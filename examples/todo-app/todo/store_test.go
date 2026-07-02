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

func TestMigrationsAddDueDateAndPendingIndex(t *testing.T) {
	pool := testPool(t)
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM information_schema.columns
		 WHERE table_name = 'todos' AND column_name = 'due_date'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatal("due_date column missing")
	}
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM pg_indexes WHERE indexname = 'idx_todos_pending'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatal("idx_todos_pending missing")
	}
}

func TestStoreCRUD(t *testing.T) {
	pool := testPool(t)
	s := NewStore(pool)
	ctx := context.Background()

	created, err := s.Create(ctx, "write the example")
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == 0 || created.Title != "write the example" || created.Done {
		t.Fatalf("unexpected created todo: %+v", created)
	}

	toggled, err := s.Toggle(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !toggled.Done {
		t.Fatal("toggle did not flip done")
	}

	list, err := s.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("unexpected list: %+v", list)
	}

	if err := s.Delete(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Toggle(ctx, created.ID); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
	if err := s.Delete(ctx, created.ID); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound on double delete, got %v", err)
	}
}
