package migrator

import (
	"context"
	"testing"
)

func TestDownRollsBackLastBatch(t *testing.T) {
	pool := testPool(t)
	resetRegistry()
	Register("100_a", makeCreate("a"), makeDrop("a"))
	Register("200_b", makeCreate("b"), makeDrop("b"))
	Up(context.Background(), pool) // batch 1: a, b
	Register("300_c", makeCreate("c"), makeDrop("c"))
	Up(context.Background(), pool) // batch 2: c

	res, err := Down(context.Background(), pool) // default: last batch (c)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.RolledBack) != 1 || res.RolledBack[0] != "300_c" {
		t.Fatalf("expected rollback of 300_c, got %v", res.RolledBack)
	}
	assertTableMissing(t, pool, "c")
	assertTableExists(t, pool, "a")
}

func TestDownStepsReverseOrder(t *testing.T) {
	pool := testPool(t)
	resetRegistry()
	Register("100_a", makeCreate("a"), makeDrop("a"))
	Register("200_b", makeCreate("b"), makeDrop("b"))
	Up(context.Background(), pool)
	res, _ := Down(context.Background(), pool, Steps(2)) // both, most recent first
	if res.RolledBack[0] != "200_b" || res.RolledBack[1] != "100_a" {
		t.Fatalf("reverse order wrong: %v", res.RolledBack)
	}
}
