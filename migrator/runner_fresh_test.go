package migrator

import (
	"context"
	"errors"
	"testing"
)

func TestFreshRequiresAllow(t *testing.T) {
	pool := testPool(t)
	if _, err := Fresh(context.Background(), pool); !errors.Is(err, ErrFreshNotAllowed) {
		t.Fatalf("expected ErrFreshNotAllowed, got %v", err)
	}
}

func TestFreshWipesAndReMigrates(t *testing.T) {
	pool := testPool(t)
	resetRegistry()
	Register("100_a", makeCreate("a"), makeDrop("a"))
	Up(context.Background(), pool)
	// create a stray table not managed by migrations
	pool.Exec(context.Background(), `CREATE TABLE stray (id int);`)

	res, err := Fresh(context.Background(), pool, AllowFresh())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Applied) != 1 {
		t.Fatalf("expected 1 applied, got %v", res.Applied)
	}
	assertTableExists(t, pool, "a")      // re-created
	assertTableMissing(t, pool, "stray") // wiped by DROP SCHEMA
}
