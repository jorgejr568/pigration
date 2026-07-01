package migrator

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DefaultTable is the default tracking-table name.
const DefaultTable = "schema_migrations"

// advisoryLockKey is a fixed application-specific key for the session-level
// advisory lock guarding migration runs. Derived from "pigration".
const advisoryLockKey int64 = 0x7069677261746e00

// Result is returned by Up/Down/Fresh.
type Result struct {
	Applied    []string // migrations applied (Up/Fresh)
	RolledBack []string // migrations rolled back (Down)
	Batch      int      // batch number assigned to this run (Up/Fresh)
}

// MigrationStatus describes the state of a single migration. Batch and AppliedAt
// are zero values (0 and the zero Time) when Pending is true.
type MigrationStatus struct {
	Name      string
	Batch     int
	AppliedAt time.Time
	Pending   bool
}

// runConfig is the resolved configuration for a single runner invocation.
type runConfig struct {
	table      string
	steps      int // 0 = all
	batch      int // 0 = default/latest
	allowFresh bool
}

// RunOption configures a single runner invocation.
type RunOption func(*runConfig)

// Steps limits Up/Down to n migrations.
func Steps(n int) RunOption { return func(rc *runConfig) { rc.steps = n } }

// Batch targets a specific batch number for Down. k must be >= 1; values <= 0
// are ignored and Down falls back to the default (most recent batch).
func Batch(k int) RunOption { return func(rc *runConfig) { rc.batch = k } }

// AllowFresh opts in to the destructive Fresh operation.
func AllowFresh() RunOption { return func(rc *runConfig) { rc.allowFresh = true } }

// Table overrides the tracking-table name. An empty name is ignored: the table
// stays DefaultTable, since an empty identifier would produce malformed SQL at
// runtime.
func Table(name string) RunOption {
	return func(rc *runConfig) {
		if name != "" {
			rc.table = name
		}
	}
}

func resolveConfig(opts []RunOption) runConfig {
	rc := runConfig{table: DefaultTable}
	for _, opt := range opts {
		opt(&rc)
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
		// Use a cancellation-immune context: if the caller's ctx was cancelled or
		// timed out during the run, pgx would return before sending the UNLOCK,
		// stranding this session-level advisory lock on the pooled connection.
		unlockCtx := context.WithoutCancel(ctx)
		_, _ = conn.Exec(unlockCtx, `SELECT pg_advisory_unlock($1)`, advisoryLockKey)
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
	rows, _ := pool.Query(ctx, fmt.Sprintf(`SELECT name FROM %s`, table))
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("loading applied migrations: %w", err)
	}
	set := make(map[string]struct{}, len(names))
	for _, name := range names {
		set[name] = struct{}{}
	}
	return set, nil
}

// runLocked enforces the invariant that every mutating entrypoint (Up/Down/Fresh)
// validates the registry and holds the advisory lock for the entire duration of
// fn. fn's Result must be propagated verbatim: Up and Down return partially
// populated Results alongside errors, so this helper never rewrites it.
func runLocked(ctx context.Context, pool *pgxpool.Pool, rc runConfig, fn func(runConfig) (Result, error)) (Result, error) {
	if err := validateRegistry(); err != nil {
		return Result{}, err
	}
	release, err := acquireLock(ctx, pool)
	if err != nil {
		return Result{}, err
	}
	defer release()

	return fn(rc)
}

// Up applies pending migrations, grouping them into a single new batch.
func Up(ctx context.Context, pool *pgxpool.Pool, opts ...RunOption) (Result, error) {
	return runLocked(ctx, pool, resolveConfig(opts), func(rc runConfig) (Result, error) {
		return applyPending(ctx, pool, rc.table, rc.steps)
	})
}

// Fresh drops the public schema (CASCADE), recreates it, then applies all
// migrations from scratch. It returns ErrFreshNotAllowed unless AllowFresh() is
// passed. This is destructive: everything in the public schema is wiped.
func Fresh(ctx context.Context, pool *pgxpool.Pool, opts ...RunOption) (Result, error) {
	rc := resolveConfig(opts)
	// ErrFreshNotAllowed must take precedence over ErrLocked, so this check sits
	// outside runLocked and precedes lock acquisition.
	if !rc.allowFresh {
		return Result{}, ErrFreshNotAllowed
	}
	return runLocked(ctx, pool, rc, func(rc runConfig) (Result, error) {
		if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
			return Result{}, fmt.Errorf("dropping public schema: %w", err)
		}
		// Fresh always applies everything; steps must not carry over.
		return applyPending(ctx, pool, rc.table, 0)
	})
}

// applyPending ensures the table, computes pending migrations, and applies them
// (up to steps, 0 = all) in a fresh batch. Must be called while holding the lock.
func applyPending(ctx context.Context, pool *pgxpool.Pool, table string, steps int) (Result, error) {
	if err := ensureTable(ctx, pool, table); err != nil {
		return Result{}, err
	}
	applied, err := appliedSet(ctx, pool, table)
	if err != nil {
		return Result{}, err
	}

	var pending []migration
	for _, m := range registered() {
		if _, ok := applied[m.name]; !ok {
			pending = append(pending, m)
		}
	}

	var nextBatch int
	if err := pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT COALESCE(MAX(batch),0)+1 FROM %s`, table)).Scan(&nextBatch); err != nil {
		return Result{}, fmt.Errorf("computing next batch: %w", err)
	}

	if steps > 0 && steps < len(pending) {
		pending = pending[:steps]
	}

	res := Result{Batch: nextBatch}
	for _, m := range pending {
		if err := applyOne(ctx, pool, table, m, nextBatch); err != nil {
			return res, err
		}
		res.Applied = append(res.Applied, m.name)
	}
	return res, nil
}

// runStep runs one migration function and its bookkeeping statement, either
// inside a transaction (default) or directly against the pool (nonTransactional).
// opLabel/bookLabel carry each caller's exact error wording so no message is
// genericized.
func runStep(ctx context.Context, pool *pgxpool.Pool, m migration, fn MigrationFunc, opLabel, bookLabel, bookkeepSQL string, args ...any) error {
	if m.nonTransactional {
		if err := fn(ctx, pool); err != nil {
			return fmt.Errorf("%s %q failed (non-transactional, not rolled back): %w", opLabel, m.name, err)
		}
		if _, err := pool.Exec(ctx, bookkeepSQL, args...); err != nil {
			return fmt.Errorf("%s %q: %w", bookLabel, m.name, err)
		}
		return nil
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning tx for %s %q: %w", opLabel, m.name, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := fn(ctx, tx); err != nil {
		return fmt.Errorf("%s %q failed (rolled back): %w", opLabel, m.name, err)
	}
	if _, err := tx.Exec(ctx, bookkeepSQL, args...); err != nil {
		return fmt.Errorf("%s %q (rolled back): %w", bookLabel, m.name, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing %s %q: %w", opLabel, m.name, err)
	}
	return nil
}

// applyOne runs a single migration's up function and records it.
func applyOne(ctx context.Context, pool *pgxpool.Pool, table string, m migration, batch int) error {
	insert := fmt.Sprintf(`INSERT INTO %s (name, batch) VALUES ($1, $2)`, table)
	return runStep(ctx, pool, m, m.up, "migration", "recording migration", insert, m.name, batch)
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
	for _, m := range registered() {
		seen[m.name] = struct{}{}
		st := MigrationStatus{Name: m.name, Pending: true}
		if r, ok := applied[m.name]; ok {
			st.Batch = r.batch
			st.AppliedAt = r.appliedAt
			st.Pending = false
		}
		out = append(out, st)
	}
	// applied-but-unregistered rows
	for name, r := range applied {
		if _, ok := seen[name]; ok {
			continue
		}
		out = append(out, MigrationStatus{Name: name, Batch: r.batch, AppliedAt: r.appliedAt, Pending: false})
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
	if rc.batch < 0 {
		return Result{}, fmt.Errorf("batch must be >= 1, got %d", rc.batch)
	}
	return runLocked(ctx, pool, rc, func(rc runConfig) (Result, error) {
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
	})
}

// downTargets returns the applied migration names to roll back, ordered most
// recent first (by descending id).
func downTargets(ctx context.Context, pool *pgxpool.Pool, rc runConfig) ([]string, error) {
	var query string
	var args []any
	switch {
	case rc.batch > 0:
		query = fmt.Sprintf(`SELECT name FROM %s WHERE batch = $1 ORDER BY id DESC`, rc.table)
		args = []any{rc.batch}
	case rc.steps > 0:
		query = fmt.Sprintf(`SELECT name FROM %s ORDER BY id DESC LIMIT $1`, rc.table)
		args = []any{rc.steps}
	default:
		query = fmt.Sprintf(`SELECT name FROM %s WHERE batch = (SELECT MAX(batch) FROM %s) ORDER BY id DESC`, rc.table, rc.table)
	}

	rows, _ := pool.Query(ctx, query, args...)
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("selecting rollback targets: %w", err)
	}
	return names, nil
}

// rollbackOne runs a single migration's down function and removes its tracking
// row.
func rollbackOne(ctx context.Context, pool *pgxpool.Pool, table string, m migration) error {
	del := fmt.Sprintf(`DELETE FROM %s WHERE name = $1`, table)
	return runStep(ctx, pool, m, m.down, "rollback of", "removing tracking row for", del, m.name)
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
