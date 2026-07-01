package migrator

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DefaultTable is the default tracking-table name.
const DefaultTable = "schema_migrations"

// advisoryLockKey is a fixed application-specific key for the session-level
// advisory lock guarding migration runs. Derived from "pigration".
const advisoryLockKey int64 = 0x7069677261746e00

// Options configures the tracking table used by the runner.
type Options struct {
	// Table is the tracking-table name; defaults to DefaultTable when empty.
	Table string
}

// Result is returned by Up/Down/Fresh.
type Result struct {
	Applied    []string // migrations applied (Up/Fresh)
	RolledBack []string // migrations rolled back (Down)
	Batch      int      // batch number assigned to this run (Up/Fresh)
}

// MigrationStatus describes the state of a single migration.
type MigrationStatus struct {
	Name      string
	Batch     *int
	AppliedAt *time.Time
	Pending   bool
}

// runConfig is the resolved configuration for a single runner invocation.
type runConfig struct {
	table      string
	steps      int // 0 = all
	batch      *int
	allowFresh bool
}

// RunOption configures a single runner invocation.
type RunOption func(*runConfig)

// Steps limits Up/Down to n migrations.
func Steps(n int) RunOption { return func(rc *runConfig) { rc.steps = n } }

// Batch targets a specific batch number for Down.
func Batch(k int) RunOption {
	return func(rc *runConfig) {
		v := k
		rc.batch = &v
	}
}

// AllowFresh opts in to the destructive Fresh operation.
func AllowFresh() RunOption { return func(rc *runConfig) { rc.allowFresh = true } }

// WithOptions applies an Options value (table name) as a RunOption.
func WithOptions(o Options) RunOption {
	return func(rc *runConfig) {
		if o.Table != "" {
			rc.table = o.Table
		}
	}
}

func resolveConfig(opts []RunOption) runConfig {
	rc := runConfig{table: DefaultTable}
	for _, opt := range opts {
		opt(&rc)
	}
	if rc.table == "" {
		rc.table = DefaultTable
	}
	return rc
}

// acquireLock takes the session-level advisory lock, failing with ErrLocked if
// another runner holds it. The returned release func must be called to release.
func acquireLock(ctx context.Context, pool *pgxpool.Pool) (func(), error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquiring connection for lock: %w", err)
	}
	var ok bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, advisoryLockKey).Scan(&ok); err != nil {
		conn.Release()
		return nil, fmt.Errorf("acquiring advisory lock: %w", err)
	}
	if !ok {
		conn.Release()
		return nil, ErrLocked
	}
	release := func() {
		_, _ = conn.Exec(ctx, `SELECT pg_advisory_unlock($1)`, advisoryLockKey)
		conn.Release()
	}
	return release, nil
}

// ensureTable creates the tracking table if it does not exist.
func ensureTable(ctx context.Context, pool *pgxpool.Pool, table string) error {
	_, err := pool.Exec(ctx, fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
	id         bigserial   PRIMARY KEY,
	name       text        NOT NULL UNIQUE,
	batch      int         NOT NULL,
	applied_at timestamptz NOT NULL DEFAULT now()
);`, table))
	if err != nil {
		return fmt.Errorf("ensuring tracking table %q: %w", table, err)
	}
	return nil
}

// appliedSet returns the set of applied migration names.
func appliedSet(ctx context.Context, pool *pgxpool.Pool, table string) (map[string]struct{}, error) {
	rows, err := pool.Query(ctx, fmt.Sprintf(`SELECT name FROM %s`, table))
	if err != nil {
		return nil, fmt.Errorf("loading applied migrations: %w", err)
	}
	defer rows.Close()
	set := make(map[string]struct{})
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		set[name] = struct{}{}
	}
	return set, rows.Err()
}

// Up applies pending migrations, grouping them into a single new batch.
func Up(ctx context.Context, pool *pgxpool.Pool, opts ...RunOption) (Result, error) {
	rc := resolveConfig(opts)
	if err := validateRegistry(); err != nil {
		return Result{}, err
	}
	release, err := acquireLock(ctx, pool)
	if err != nil {
		return Result{}, err
	}
	defer release()

	return applyPending(ctx, pool, rc)
}

// Fresh drops the public schema (CASCADE), recreates it, then applies all
// migrations from scratch. It returns ErrFreshNotAllowed unless AllowFresh() is
// passed. This is destructive: everything in the public schema is wiped.
func Fresh(ctx context.Context, pool *pgxpool.Pool, opts ...RunOption) (Result, error) {
	rc := resolveConfig(opts)
	if !rc.allowFresh {
		return Result{}, ErrFreshNotAllowed
	}
	if err := validateRegistry(); err != nil {
		return Result{}, err
	}
	release, err := acquireLock(ctx, pool)
	if err != nil {
		return Result{}, err
	}
	defer release()

	if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		return Result{}, fmt.Errorf("dropping public schema: %w", err)
	}

	// steps must not carry over into Fresh; apply everything.
	rc.steps = 0
	return applyPending(ctx, pool, rc)
}

// applyPending ensures the table, computes pending migrations, and applies them
// (up to rc.steps) in a fresh batch. Must be called while holding the lock.
func applyPending(ctx context.Context, pool *pgxpool.Pool, rc runConfig) (Result, error) {
	if err := ensureTable(ctx, pool, rc.table); err != nil {
		return Result{}, err
	}
	applied, err := appliedSet(ctx, pool, rc.table)
	if err != nil {
		return Result{}, err
	}

	var pending []migration
	for _, m := range Registered() {
		if _, ok := applied[m.name]; !ok {
			pending = append(pending, m)
		}
	}

	var nextBatch int
	if err := pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT COALESCE(MAX(batch),0)+1 FROM %s`, rc.table)).Scan(&nextBatch); err != nil {
		return Result{}, fmt.Errorf("computing next batch: %w", err)
	}

	if rc.steps > 0 && rc.steps < len(pending) {
		pending = pending[:rc.steps]
	}

	res := Result{Batch: nextBatch}
	for _, m := range pending {
		if err := applyOne(ctx, pool, rc.table, m, nextBatch); err != nil {
			return res, err
		}
		res.Applied = append(res.Applied, m.name)
	}
	return res, nil
}

// applyOne runs a single migration's up function and records it.
func applyOne(ctx context.Context, pool *pgxpool.Pool, table string, m migration, batch int) error {
	insert := fmt.Sprintf(`INSERT INTO %s (name, batch) VALUES ($1, $2)`, table)

	if m.nonTransactional {
		if err := m.up(ctx, pool); err != nil {
			return fmt.Errorf("migration %q failed (non-transactional, not rolled back): %w", m.name, err)
		}
		if _, err := pool.Exec(ctx, insert, m.name, batch); err != nil {
			return fmt.Errorf("recording migration %q: %w", m.name, err)
		}
		return nil
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning tx for migration %q: %w", m.name, err)
	}
	if err := m.up(ctx, tx); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("migration %q failed (rolled back): %w", m.name, err)
	}
	if _, err := tx.Exec(ctx, insert, m.name, batch); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("recording migration %q (rolled back): %w", m.name, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing migration %q: %w", m.name, err)
	}
	return nil
}

// Status returns the state of every registered migration and every
// applied-but-unregistered row, ordered by name.
func Status(ctx context.Context, pool *pgxpool.Pool, opts ...RunOption) ([]MigrationStatus, error) {
	rc := resolveConfig(opts)
	if err := ensureTable(ctx, pool, rc.table); err != nil {
		return nil, err
	}

	type appliedRow struct {
		batch     int
		appliedAt time.Time
	}
	applied := make(map[string]appliedRow)
	rows, err := pool.Query(ctx, fmt.Sprintf(`SELECT name, batch, applied_at FROM %s`, rc.table))
	if err != nil {
		return nil, fmt.Errorf("loading status: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var r appliedRow
		if err := rows.Scan(&name, &r.batch, &r.appliedAt); err != nil {
			return nil, err
		}
		applied[name] = r
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	var out []MigrationStatus
	for _, m := range Registered() {
		seen[m.name] = struct{}{}
		st := MigrationStatus{Name: m.name, Pending: true}
		if r, ok := applied[m.name]; ok {
			b := r.batch
			at := r.appliedAt
			st.Batch = &b
			st.AppliedAt = &at
			st.Pending = false
		}
		out = append(out, st)
	}
	// applied-but-unregistered rows
	for name, r := range applied {
		if _, ok := seen[name]; ok {
			continue
		}
		b := r.batch
		at := r.appliedAt
		out = append(out, MigrationStatus{Name: name, Batch: &b, AppliedAt: &at, Pending: false})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Down rolls back migrations. Target selection:
//   - default: all migrations in the highest batch.
//   - Batch(k): all migrations in batch k.
//   - Steps(n): the last n applied migrations (across batches).
//
// Targets are reverted in reverse applied order (most recent first), each in
// its own transaction unless marked NonTransactional. Execution stops on the
// first error.
func Down(ctx context.Context, pool *pgxpool.Pool, opts ...RunOption) (Result, error) {
	rc := resolveConfig(opts)
	if err := validateRegistry(); err != nil {
		return Result{}, err
	}
	release, err := acquireLock(ctx, pool)
	if err != nil {
		return Result{}, err
	}
	defer release()

	if err := ensureTable(ctx, pool, rc.table); err != nil {
		return Result{}, err
	}

	targets, err := downTargets(ctx, pool, rc)
	if err != nil {
		return Result{}, err
	}

	var res Result
	for _, name := range targets {
		m, ok := registeredByName(name)
		if !ok {
			return res, fmt.Errorf("cannot roll back %q: no longer registered", name)
		}
		if err := rollbackOne(ctx, pool, rc.table, m); err != nil {
			return res, err
		}
		res.RolledBack = append(res.RolledBack, name)
	}
	return res, nil
}

// downTargets returns the applied migration names to roll back, ordered most
// recent first (by descending id).
func downTargets(ctx context.Context, pool *pgxpool.Pool, rc runConfig) ([]string, error) {
	var query string
	var args []any
	switch {
	case rc.batch != nil:
		query = fmt.Sprintf(`SELECT name FROM %s WHERE batch = $1 ORDER BY id DESC`, rc.table)
		args = []any{*rc.batch}
	case rc.steps > 0:
		query = fmt.Sprintf(`SELECT name FROM %s ORDER BY id DESC LIMIT $1`, rc.table)
		args = []any{rc.steps}
	default:
		query = fmt.Sprintf(`SELECT name FROM %s WHERE batch = (SELECT MAX(batch) FROM %s) ORDER BY id DESC`, rc.table, rc.table)
	}

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("selecting rollback targets: %w", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// rollbackOne runs a single migration's down function and removes its tracking
// row.
func rollbackOne(ctx context.Context, pool *pgxpool.Pool, table string, m migration) error {
	del := fmt.Sprintf(`DELETE FROM %s WHERE name = $1`, table)

	if m.nonTransactional {
		if err := m.down(ctx, pool); err != nil {
			return fmt.Errorf("rollback of %q failed (non-transactional, not rolled back): %w", m.name, err)
		}
		if _, err := pool.Exec(ctx, del, m.name); err != nil {
			return fmt.Errorf("removing tracking row for %q: %w", m.name, err)
		}
		return nil
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning tx for rollback of %q: %w", m.name, err)
	}
	if err := m.down(ctx, tx); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("rollback of %q failed (rolled back): %w", m.name, err)
	}
	if _, err := tx.Exec(ctx, del, m.name); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("removing tracking row for %q (rolled back): %w", m.name, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing rollback of %q: %w", m.name, err)
	}
	return nil
}

// registeredByName returns the registered migration with the given name.
func registeredByName(name string) (migration, bool) {
	for _, m := range registry {
		if m.name == name {
			return m, true
		}
	}
	return migration{}, false
}
