package migrator

import (
	"context"
	"testing"
)

// Regression: the advisory-lock release used the caller's context for the UNLOCK.
// If that context was already cancelled/expired, pgx returned before sending the
// UNLOCK, stranding a session-level advisory lock on a pooled connection.
func TestAdvisoryLockReleasedDespiteCancelledContext(t *testing.T) {
	pool := testPool(t)

	ctx, cancel := context.WithCancel(context.Background())
	release, err := acquireLock(ctx, pool)
	if err != nil {
		t.Fatalf("acquireLock: %v", err)
	}
	// Simulate the run's context being cancelled before cleanup runs.
	cancel()
	release()

	// From the pool, no advisory lock should remain held anywhere.
	var count int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM pg_locks WHERE locktype = 'advisory'`).Scan(&count); err != nil {
		t.Fatalf("querying pg_locks: %v", err)
	}
	if count != 0 {
		t.Fatalf("advisory lock leaked: %d still held after release", count)
	}

	// And a fresh acquire must succeed.
	release2, err := acquireLock(context.Background(), pool)
	if err != nil {
		t.Fatalf("second acquireLock failed (lock was not released): %v", err)
	}
	release2()
}
