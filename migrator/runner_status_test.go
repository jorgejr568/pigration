package migrator

import (
	"context"
	"testing"
)

func TestStatusMarksPending(t *testing.T) {
	pool := testPool(t)
	resetRegistry()
	Register("100_a", makeCreate("a"), makeDrop("a"))
	Register("200_b", makeCreate("b"), makeDrop("b"))
	Up(context.Background(), pool, Steps(1)) // apply only 100_a
	st, err := Status(context.Background(), pool)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]MigrationStatus{}
	for _, s := range st {
		byName[s.Name] = s
	}
	if byName["100_a"].Pending {
		t.Fatal("100_a should be applied")
	}
	if !byName["200_b"].Pending {
		t.Fatal("200_b should be pending")
	}
}
