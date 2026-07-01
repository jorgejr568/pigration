package migrator

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// testPool returns a connected pool for integration tests. It skips the test
// when TEST_DATABASE_URL is unset, and resets the public schema for isolation.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connecting to test db: %v", err)
	}
	if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		pool.Close()
		t.Fatalf("resetting schema: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// makeCreate returns an up func that creates a table with the given name.
func makeCreate(name string) MigrationFunc {
	return func(ctx context.Context, tx Executor) error {
		_, err := tx.Exec(ctx, fmt.Sprintf(`CREATE TABLE %s (id int);`, name))
		return err
	}
}

// makeDrop returns a down func that drops the given table.
func makeDrop(name string) MigrationFunc {
	return func(ctx context.Context, tx Executor) error {
		_, err := tx.Exec(ctx, fmt.Sprintf(`DROP TABLE %s;`, name))
		return err
	}
}

func tableExists(t *testing.T, pool *pgxpool.Pool, name string) bool {
	t.Helper()
	var exists bool
	err := pool.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name=$1)`,
		name).Scan(&exists)
	if err != nil {
		t.Fatalf("checking table %q: %v", name, err)
	}
	return exists
}

func assertTableExists(t *testing.T, pool *pgxpool.Pool, name string) {
	t.Helper()
	if !tableExists(t, pool, name) {
		t.Fatalf("expected table %q to exist", name)
	}
}

func assertTableMissing(t *testing.T, pool *pgxpool.Pool, name string) {
	t.Helper()
	if tableExists(t, pool, name) {
		t.Fatalf("expected table %q to be missing", name)
	}
}
