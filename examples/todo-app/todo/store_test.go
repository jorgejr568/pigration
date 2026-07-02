package todo

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jorgejr568/pigration/migrator"

	_ "github.com/jorgejr568/pigration/examples/todo-app/migrations"
)

// testPool connects to TEST_DATABASE_URL (skipping without it) and resets the
// database with migrator.Fresh so every test starts from freshly applied
// migrations — dogfooding the engine as the test fixture.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run integration tests")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := migrator.Fresh(context.Background(), pool, migrator.AllowFresh()); err != nil {
		t.Fatalf("fresh: %v", err)
	}
	return pool
}

func TestMigrationsCreateTodosTable(t *testing.T) {
	pool := testPool(t)
	var n int
	err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM information_schema.tables WHERE table_name = 'todos'`).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("todos table not created (count=%d)", n)
	}
}
