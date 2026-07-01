package migrator

import (
	"context"
	"fmt"
	"testing"
)

func TestUpAppliesPendingAndRecordsBatch(t *testing.T) {
	pool := testPool(t)
	resetRegistry()
	Register("100_a", makeCreate("a"), makeDrop("a"))
	Register("200_b", makeCreate("b"), makeDrop("b"))

	res, err := Up(context.Background(), pool)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Applied) != 2 || res.Batch != 1 {
		t.Fatalf("got applied=%v batch=%d", res.Applied, res.Batch)
	}
	// second run applies nothing
	res2, _ := Up(context.Background(), pool)
	if len(res2.Applied) != 0 {
		t.Fatalf("expected 0 applied, got %v", res2.Applied)
	}
	// both tables exist
	for _, tbl := range []string{"a", "b"} {
		assertTableExists(t, pool, tbl)
	}
}

func TestUpFailureRollsBackAndStops(t *testing.T) {
	pool := testPool(t)
	resetRegistry()
	Register("100_a", makeCreate("a"), makeDrop("a"))
	Register("200_bad", func(ctx context.Context, tx Executor) error {
		tx.Exec(ctx, `CREATE TABLE will_rollback (id int);`)
		return fmt.Errorf("boom")
	}, func(ctx context.Context, tx Executor) error { return nil })

	_, err := Up(context.Background(), pool)
	if err == nil {
		t.Fatal("expected error")
	}
	assertTableExists(t, pool, "a")              // committed before failure
	assertTableMissing(t, pool, "will_rollback") // rolled back
}
