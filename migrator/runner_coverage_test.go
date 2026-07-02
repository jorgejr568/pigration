package migrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// rowCount returns the number of rows in an arbitrary table (test helper).
func rowCount(t *testing.T, pool *pgxpool.Pool, table string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		fmt.Sprintf(`SELECT count(*) FROM %s`, table)).Scan(&n); err != nil {
		t.Fatalf("counting %s: %v", table, err)
	}
	return n
}

// nameInTable reports whether a tracking row with the given name exists.
func nameInTable(t *testing.T, pool *pgxpool.Pool, table, name string) bool {
	t.Helper()
	var exists bool
	if err := pool.QueryRow(context.Background(),
		fmt.Sprintf(`SELECT EXISTS (SELECT 1 FROM %s WHERE name = $1)`, table), name).Scan(&exists); err != nil {
		t.Fatalf("checking name %q in %s: %v", name, table, err)
	}
	return exists
}

// TestNonTransactionalUpAndDown drives the NonTransactional path in runStep
// end-to-end. CREATE INDEX CONCURRENTLY cannot run inside a transaction, so it
// only succeeds via the non-transactional branch; if runStep wrapped it in a tx
// the Up would error. Down likewise drops the index concurrently.
func TestNonTransactionalUpAndDown(t *testing.T) {
	pool := testPool(t)
	resetRegistry()
	// Base table the concurrent index will be built on.
	Register("100_base", makeCreate("idx_base"), makeDrop("idx_base"))
	Register("200_concidx",
		func(ctx context.Context, tx Executor) error {
			_, err := tx.Exec(ctx, `CREATE INDEX CONCURRENTLY concidx ON idx_base (id);`)
			return err
		},
		func(ctx context.Context, tx Executor) error {
			_, err := tx.Exec(ctx, `DROP INDEX CONCURRENTLY concidx;`)
			return err
		},
		NonTransactional(),
	)

	res, err := Up(context.Background(), pool)
	if err != nil {
		t.Fatalf("non-transactional Up failed: %v", err)
	}
	if len(res.Applied) != 2 {
		t.Fatalf("expected 2 applied, got %v", res.Applied)
	}
	// Index exists and the tracking row was written.
	var idxExists bool
	if err := pool.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'concidx')`).Scan(&idxExists); err != nil {
		t.Fatal(err)
	}
	if !idxExists {
		t.Fatal("expected concurrent index to exist after Up")
	}
	if !nameInTable(t, pool, DefaultTable, "200_concidx") {
		t.Fatal("expected tracking row for 200_concidx")
	}

	// Roll back the non-transactional migration.
	dres, err := Down(context.Background(), pool, Steps(1))
	if err != nil {
		t.Fatalf("non-transactional Down failed: %v", err)
	}
	if len(dres.RolledBack) != 1 || dres.RolledBack[0] != "200_concidx" {
		t.Fatalf("expected rollback of 200_concidx, got %v", dres.RolledBack)
	}
	if err := pool.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'concidx')`).Scan(&idxExists); err != nil {
		t.Fatal(err)
	}
	if idxExists {
		t.Fatal("expected concurrent index to be dropped after Down")
	}
	if nameInTable(t, pool, DefaultTable, "200_concidx") {
		t.Fatal("expected tracking row for 200_concidx to be removed")
	}
}

// TestNonTransactionalUpFailure covers the error branch of the non-transactional
// op in runStep: the migration fn returns an error and the loud, "not rolled
// back" message is surfaced.
func TestNonTransactionalUpFailure(t *testing.T) {
	pool := testPool(t)
	resetRegistry()
	Register("100_boom",
		func(ctx context.Context, tx Executor) error { return fmt.Errorf("kaboom") },
		func(ctx context.Context, tx Executor) error { return nil },
		NonTransactional(),
	)
	_, err := Up(context.Background(), pool)
	if err == nil {
		t.Fatal("expected error from non-transactional migration")
	}
	if nameInTable(t, pool, DefaultTable, "100_boom") {
		t.Fatal("failed non-transactional migration must not record a tracking row")
	}
}

// TestNonTransactionalBookkeepFailure covers the bookkeeping error branch of the
// non-transactional path in runStep: the migration fn succeeds but the tracking
// INSERT fails. Driving this through Up is impossible (a pre-existing tracking
// row makes the migration non-pending, so Up skips it), so we invoke applyOne
// directly (white-box) after planting a conflicting UNIQUE(name) row.
func TestNonTransactionalBookkeepFailure(t *testing.T) {
	pool := testPool(t)
	resetRegistry()
	if err := ensureTable(context.Background(), pool, DefaultTable); err != nil {
		t.Fatal(err)
	}
	// Plant a conflicting row so the bookkeeping INSERT violates UNIQUE(name).
	if _, err := pool.Exec(context.Background(),
		fmt.Sprintf(`INSERT INTO %s (name, batch) VALUES ('100_dupname', 99)`, DefaultTable)); err != nil {
		t.Fatal(err)
	}
	m := migration{
		name:             "100_dupname",
		up:               func(ctx context.Context, tx Executor) error { return nil }, // succeeds
		down:             func(ctx context.Context, tx Executor) error { return nil },
		nonTransactional: true,
	}
	err := applyOne(context.Background(), pool, DefaultTable, m, 1)
	if err == nil {
		t.Fatal("expected bookkeeping insert to fail on unique-name conflict")
	}
	if want := `recording migration "100_dupname"`; !strings.Contains(err.Error(), want) {
		t.Fatalf("expected error to mention %q, got %q", want, err.Error())
	}
	// The pre-existing row is untouched (batch 99), proving the fn ran but the
	// bookkeeping insert was rejected without a transaction to roll it back.
	var batch int
	if err := pool.QueryRow(context.Background(),
		fmt.Sprintf(`SELECT batch FROM %s WHERE name = '100_dupname'`, DefaultTable)).Scan(&batch); err != nil {
		t.Fatal(err)
	}
	if batch != 99 {
		t.Fatalf("expected untouched batch 99, got %d", batch)
	}
}

// TestBatchTargeting proves Down(..., Batch(k)) removes exactly batch k's
// migrations, in reverse order, leaving other batches applied. This covers the
// Batch RunOption, downTargets' batch>0 branch, and Down's batch targeting.
func TestBatchTargeting(t *testing.T) {
	pool := testPool(t)
	resetRegistry()
	Register("100_a", makeCreate("a"), makeDrop("a"))
	Register("200_b", makeCreate("b"), makeDrop("b"))
	if _, err := Up(context.Background(), pool); err != nil { // batch 1: a, b
		t.Fatal(err)
	}
	Register("300_c", makeCreate("c"), makeDrop("c"))
	Register("400_d", makeCreate("d"), makeDrop("d"))
	if _, err := Up(context.Background(), pool); err != nil { // batch 2: c, d
		t.Fatal(err)
	}

	res, err := Down(context.Background(), pool, Batch(1))
	if err != nil {
		t.Fatal(err)
	}
	// Batch 1 is a,b; reverse applied order => 200_b then 100_a.
	if len(res.RolledBack) != 2 || res.RolledBack[0] != "200_b" || res.RolledBack[1] != "100_a" {
		t.Fatalf("expected [200_b 100_a], got %v", res.RolledBack)
	}
	assertTableMissing(t, pool, "a")
	assertTableMissing(t, pool, "b")
	// Batch 2 untouched.
	assertTableExists(t, pool, "c")
	assertTableExists(t, pool, "d")
	if !nameInTable(t, pool, DefaultTable, "300_c") || !nameInTable(t, pool, DefaultTable, "400_d") {
		t.Fatal("batch 2 tracking rows should remain")
	}
}

// TestBatchEmptyTarget covers Down(..., Batch(k)) for a batch that has no rows:
// downTargets returns an empty slice and Down completes with nothing rolled back.
func TestBatchEmptyTarget(t *testing.T) {
	pool := testPool(t)
	resetRegistry()
	Register("100_a", makeCreate("a"), makeDrop("a"))
	if _, err := Up(context.Background(), pool); err != nil { // batch 1
		t.Fatal(err)
	}
	res, err := Down(context.Background(), pool, Batch(5)) // no such batch
	if err != nil {
		t.Fatal(err)
	}
	if len(res.RolledBack) != 0 {
		t.Fatalf("expected nothing rolled back, got %v", res.RolledBack)
	}
	assertTableExists(t, pool, "a") // untouched
}

// TestNegativeBatchLoudError covers Down's rc.batch < 0 guard.
func TestNegativeBatchLoudError(t *testing.T) {
	pool := testPool(t)
	resetRegistry()
	_, err := Down(context.Background(), pool, Batch(-1))
	if err == nil {
		t.Fatal("expected loud error for negative batch")
	}
	if got := err.Error(); got != "batch must be >= 1, got -1" {
		t.Fatalf("unexpected error message: %q", got)
	}
}

// TestCustomTableFullCycle drives a full Up/Status/Down cycle against a custom
// tracking table and asserts, via direct SQL, that the custom table holds the
// rows while the default schema_migrations table is never created.
func TestCustomTableFullCycle(t *testing.T) {
	pool := testPool(t)
	resetRegistry()
	const custom = "custom_migrations"
	Register("100_a", makeCreate("a"), makeDrop("a"))
	Register("200_b", makeCreate("b"), makeDrop("b"))

	if _, err := Up(context.Background(), pool, Table(custom)); err != nil {
		t.Fatal(err)
	}
	// Custom table exists with both rows; default table does not exist.
	assertTableExists(t, pool, custom)
	assertTableMissing(t, pool, DefaultTable)
	if n := rowCount(t, pool, custom); n != 2 {
		t.Fatalf("expected 2 rows in %s, got %d", custom, n)
	}

	// Status reads from the custom table.
	st, err := Status(context.Background(), pool, Table(custom))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range st {
		if s.Pending {
			t.Fatalf("expected %s applied per custom table", s.Name)
		}
	}
	// Status must not have created the default table either.
	assertTableMissing(t, pool, DefaultTable)

	// Down against the custom table clears batch 1.
	res, err := Down(context.Background(), pool, Table(custom))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.RolledBack) != 2 {
		t.Fatalf("expected 2 rolled back, got %v", res.RolledBack)
	}
	if n := rowCount(t, pool, custom); n != 0 {
		t.Fatalf("expected custom table empty after Down, got %d", n)
	}
}

// TestTableEmptyNameKeepsDefault covers the empty-name branch of the Table
// option: an empty string is ignored and DefaultTable is used.
func TestTableEmptyNameKeepsDefault(t *testing.T) {
	pool := testPool(t)
	resetRegistry()
	Register("100_a", makeCreate("a"), makeDrop("a"))
	if _, err := Up(context.Background(), pool, Table("")); err != nil {
		t.Fatal(err)
	}
	assertTableExists(t, pool, DefaultTable)
	if !nameInTable(t, pool, DefaultTable, "100_a") {
		t.Fatal("expected default table to record 100_a")
	}
}

// TestUpReturnsErrLocked acquires the advisory lock on a separate connection,
// then expects Up to return ErrLocked (acquireLock's !ok branch + runLocked's
// lock-failure branch).
func TestUpReturnsErrLocked(t *testing.T) {
	pool := testPool(t)
	resetRegistry()
	Register("100_a", makeCreate("a"), makeDrop("a"))

	// Hold the lock on a dedicated connection outside the pool's normal churn.
	ctx := context.Background()
	holder, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Release()
	var ok bool
	if err := holder.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, advisoryLockKey).Scan(&ok); err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("test setup: could not acquire advisory lock")
	}
	defer holder.Exec(ctx, `SELECT pg_advisory_unlock($1)`, advisoryLockKey)

	_, err = Up(ctx, pool)
	if !errors.Is(err, ErrLocked) {
		t.Fatalf("expected ErrLocked, got %v", err)
	}
}

// TestDuplicateRegistrationBlocksUp covers runLocked's validateRegistry-failure
// branch: two migrations with the same name make Up return the duplicate error
// before any lock is taken.
func TestDuplicateRegistrationBlocksUp(t *testing.T) {
	pool := testPool(t)
	resetRegistry()
	Register("100_dup", makeCreate("x"), makeDrop("x"))
	Register("100_dup", makeCreate("y"), makeDrop("y"))
	_, err := Up(context.Background(), pool)
	if err == nil {
		t.Fatal("expected duplicate-registration error")
	}
	if got := err.Error(); got != `duplicate migration name registered: "100_dup"` {
		t.Fatalf("unexpected error: %q", got)
	}
}

// TestDownUnregisteredAppliedMigration inserts a tracking row for a name that is
// not registered, then expects Down to fail with the not-registered error
// (Down's registeredByName-miss branch + registeredByName's not-found return).
func TestDownUnregisteredAppliedMigration(t *testing.T) {
	pool := testPool(t)
	resetRegistry()
	Register("100_a", makeCreate("a"), makeDrop("a"))
	if _, err := Up(context.Background(), pool); err != nil { // batch 1: 100_a
		t.Fatal(err)
	}
	// Plant an applied-but-unregistered row in the same (latest) batch.
	if _, err := pool.Exec(context.Background(),
		fmt.Sprintf(`INSERT INTO %s (name, batch) VALUES ('999_ghost', 1)`, DefaultTable)); err != nil {
		t.Fatal(err)
	}
	_, err := Down(context.Background(), pool) // targets batch 1, ghost first (higher id)
	if err == nil {
		t.Fatal("expected not-registered error")
	}
	if got := err.Error(); got != `cannot roll back "999_ghost": no longer registered` {
		t.Fatalf("unexpected error: %q", got)
	}
	// Ghost row is untouched; 100_a still applied.
	if !nameInTable(t, pool, DefaultTable, "999_ghost") {
		t.Fatal("ghost row should remain after failed Down")
	}
	if !nameInTable(t, pool, DefaultTable, "100_a") {
		t.Fatal("100_a should remain applied")
	}
}

// TestDownMigrationFailureStopsAndKeepsRow covers the mid-run Down error path:
// a failing down fn (inside its transaction) stops the run and its tracking row
// survives. This exercises rollbackOne -> runStep transactional error branches
// and Down's rollbackOne-error return.
func TestDownMigrationFailureStopsAndKeepsRow(t *testing.T) {
	pool := testPool(t)
	resetRegistry()
	Register("100_a", makeCreate("a"), makeDrop("a"))
	// down deliberately fails.
	Register("200_bad",
		makeCreate("b"),
		func(ctx context.Context, tx Executor) error { return fmt.Errorf("down boom") },
	)
	if _, err := Up(context.Background(), pool); err != nil {
		t.Fatal(err)
	}
	_, err := Down(context.Background(), pool, Steps(2)) // 200_bad first
	if err == nil {
		t.Fatal("expected down failure")
	}
	// Failed down keeps its tracking row and its table.
	if !nameInTable(t, pool, DefaultTable, "200_bad") {
		t.Fatal("200_bad tracking row should survive a failed down (tx rolled back)")
	}
	assertTableExists(t, pool, "b")
	// 100_a untouched because the run stopped before reaching it.
	if !nameInTable(t, pool, DefaultTable, "100_a") {
		t.Fatal("100_a should still be applied")
	}
}

// TestStatusIncludesUnregisteredApplied covers Status's applied-but-unregistered
// output branch: a tracking row whose name is not in the registry is surfaced as
// a non-pending MigrationStatus.
func TestStatusIncludesUnregisteredApplied(t *testing.T) {
	pool := testPool(t)
	resetRegistry()
	Register("100_a", makeCreate("a"), makeDrop("a"))
	if _, err := Up(context.Background(), pool); err != nil {
		t.Fatal(err)
	}
	// Plant an applied row for a name no longer registered.
	if _, err := pool.Exec(context.Background(),
		fmt.Sprintf(`INSERT INTO %s (name, batch) VALUES ('900_gone', 7)`, DefaultTable)); err != nil {
		t.Fatal(err)
	}
	st, err := Status(context.Background(), pool)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]MigrationStatus{}
	for _, s := range st {
		byName[s.Name] = s
	}
	gone, ok := byName["900_gone"]
	if !ok {
		t.Fatal("expected unregistered applied row 900_gone in status")
	}
	if gone.Pending || gone.Batch != 7 {
		t.Fatalf("unexpected status for 900_gone: %+v", gone)
	}
}

// TestTransactionalBookkeepFailure covers runStep's transactional bookkeeping
// error branch: the migration fn succeeds but the tracking INSERT fails inside
// the tx (unique-name conflict), so the whole tx rolls back. Driven via applyOne
// directly since Up would skip an already-applied name.
func TestTransactionalBookkeepFailure(t *testing.T) {
	pool := testPool(t)
	resetRegistry()
	if err := ensureTable(context.Background(), pool, DefaultTable); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(),
		fmt.Sprintf(`INSERT INTO %s (name, batch) VALUES ('100_txdup', 42)`, DefaultTable)); err != nil {
		t.Fatal(err)
	}
	m := migration{
		name: "100_txdup",
		up: func(ctx context.Context, tx Executor) error {
			_, err := tx.Exec(ctx, `CREATE TABLE tx_side_effect (id int);`)
			return err
		},
		down: func(ctx context.Context, tx Executor) error { return nil },
	}
	err := applyOne(context.Background(), pool, DefaultTable, m, 1)
	if err == nil {
		t.Fatal("expected transactional bookkeeping insert to fail")
	}
	if want := `recording migration "100_txdup" (rolled back)`; !strings.Contains(err.Error(), want) {
		t.Fatalf("expected %q, got %q", want, err.Error())
	}
	// The whole tx rolled back: the side-effect table must not exist.
	assertTableMissing(t, pool, "tx_side_effect")
}

// TestApplyPendingNextBatchError covers applyPending's next-batch query error
// branch. A tracking table missing the batch column passes ensureTable (created
// externally, IF NOT EXISTS no-op) but fails the SELECT MAX(batch) query.
func TestApplyPendingNextBatchError(t *testing.T) {
	pool := testPool(t)
	resetRegistry()
	Register("100_a", makeCreate("a"), makeDrop("a"))
	// name column present so appliedSet succeeds; batch column absent so the
	// next-batch computation fails.
	if _, err := pool.Exec(context.Background(),
		fmt.Sprintf(`CREATE TABLE %s (id bigserial PRIMARY KEY, name text);`, DefaultTable)); err != nil {
		t.Fatal(err)
	}
	_, err := Up(context.Background(), pool)
	if err == nil {
		t.Fatal("expected next-batch computation error")
	}
	if !strings.Contains(err.Error(), "computing next batch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestStatusQueryError covers Status's pool.Query error branch (distinct from
// ensureTable): a table lacking the batch/applied_at columns passes ensureTable
// but fails the SELECT name, batch, applied_at query.
func TestStatusQueryError(t *testing.T) {
	pool := testPool(t)
	resetRegistry()
	if _, err := pool.Exec(context.Background(),
		fmt.Sprintf(`CREATE TABLE %s (id bigserial PRIMARY KEY, name text);`, DefaultTable)); err != nil {
		t.Fatal(err)
	}
	_, err := Status(context.Background(), pool)
	if err == nil {
		t.Fatal("expected Status query error for missing columns")
	}
	if !strings.Contains(err.Error(), "loading status") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestStatusScanError covers Status's rows.Scan error branch. The query
// succeeds but a column whose declared type is incompatible with the scan
// destination (batch declared text, holding a non-integer) makes Scan fail.
func TestStatusScanError(t *testing.T) {
	pool := testPool(t)
	resetRegistry()
	// Columns named as expected so the SELECT succeeds, but batch is text with a
	// non-numeric value so Scan(&int) fails.
	if _, err := pool.Exec(context.Background(),
		fmt.Sprintf(`CREATE TABLE %s (name text, batch text, applied_at timestamptz);`, DefaultTable)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(),
		fmt.Sprintf(`INSERT INTO %s (name, batch, applied_at) VALUES ('x', 'not_a_number', now());`, DefaultTable)); err != nil {
		t.Fatal(err)
	}
	_, err := Status(context.Background(), pool)
	if err == nil {
		t.Fatal("expected Status scan error for non-numeric batch")
	}
}

// TestDownEnsureTableError covers Down's ensureTable error branch (inside
// runLocked) via a malformed table name that fails CREATE TABLE.
func TestDownEnsureTableError(t *testing.T) {
	pool := testPool(t)
	resetRegistry()
	_, err := Down(context.Background(), pool, Table("bad name!"))
	if err == nil {
		t.Fatal("expected ensureTable error inside Down")
	}
	if !strings.Contains(err.Error(), "ensuring tracking table") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestCancelledContextErrorBranches covers the acquire/ensure error wraps by
// running Up with an already-cancelled context. Pool.Acquire fails, so
// acquireLock's error-wrap branch and runLocked's acquire-failure branch run.
func TestCancelledContextErrorBranches(t *testing.T) {
	pool := testPool(t)
	resetRegistry()
	Register("100_a", makeCreate("a"), makeDrop("a"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Up(ctx, pool)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

// TestUpEnsureTableError covers applyPending's ensureTable error branch (and
// Up's propagation of it) via a malformed table name that fails CREATE TABLE.
func TestUpEnsureTableError(t *testing.T) {
	pool := testPool(t)
	resetRegistry()
	Register("100_a", makeCreate("a"), makeDrop("a"))
	_, err := Up(context.Background(), pool, Table("bad name!"))
	if err == nil {
		t.Fatal("expected ensureTable error inside Up")
	}
	if !strings.Contains(err.Error(), "ensuring tracking table") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestStatusEnsureTableErrorBranch covers ensureTable's error branch (and
// Status's propagation of it) by passing a syntactically invalid table
// identifier so the CREATE TABLE statement fails.
func TestStatusEnsureTableErrorBranch(t *testing.T) {
	pool := testPool(t)
	resetRegistry()
	// A quoted identifier with an embedded space is legal to Table() (non-empty)
	// but produces invalid unquoted SQL, so CREATE TABLE errors.
	_, err := Status(context.Background(), pool, Table("bad name!"))
	if err == nil {
		t.Fatal("expected ensureTable error for malformed table name")
	}
}

// TestAppliedSetErrorBranch covers appliedSet's error branch. We register a
// migration and pre-create the tracking table WITHOUT a name column, then run
// Up: ensureTable's IF NOT EXISTS is a no-op on the malformed table and the
// SELECT name query in appliedSet fails.
func TestAppliedSetErrorBranch(t *testing.T) {
	pool := testPool(t)
	resetRegistry()
	Register("100_a", makeCreate("a"), makeDrop("a"))
	// Pre-create a conflicting table missing the "name" column.
	if _, err := pool.Exec(context.Background(),
		fmt.Sprintf(`CREATE TABLE %s (id bigserial PRIMARY KEY, wrong int);`, DefaultTable)); err != nil {
		t.Fatal(err)
	}
	_, err := Up(context.Background(), pool)
	if err == nil {
		t.Fatal("expected appliedSet error selecting missing column")
	}
}

// TestDownTargetsErrorBranch covers downTargets' error branch via a malformed
// tracking table (missing batch column) so the SELECT fails. Down's ensureTable
// is a no-op (IF NOT EXISTS), and downTargets' query errors.
func TestDownTargetsErrorBranch(t *testing.T) {
	pool := testPool(t)
	resetRegistry()
	Register("100_a", makeCreate("a"), makeDrop("a"))
	if _, err := pool.Exec(context.Background(),
		fmt.Sprintf(`CREATE TABLE %s (id bigserial PRIMARY KEY, name text);`, DefaultTable)); err != nil {
		t.Fatal(err)
	}
	// Default Down selects by MAX(batch); the batch column is missing -> error.
	_, err := Down(context.Background(), pool)
	if err == nil {
		t.Fatal("expected downTargets error selecting missing batch column")
	}
}
