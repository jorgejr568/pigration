package migrator

import (
	"context"
	"testing"
)

func TestRegisterOrdersByName(t *testing.T) {
	resetRegistry()
	noop := func(ctx context.Context, tx Executor) error { return nil }
	Register("200_b", noop, noop)
	Register("100_a", noop, noop)
	got := Registered()
	if got[0].name != "100_a" || got[1].name != "200_b" {
		t.Fatalf("not sorted: %v", []string{got[0].name, got[1].name})
	}
}

func TestNonTransactionalOption(t *testing.T) {
	resetRegistry()
	noop := func(ctx context.Context, tx Executor) error { return nil }
	Register("100_x", noop, noop, NonTransactional())
	if !Registered()[0].nonTransactional {
		t.Fatal("expected nonTransactional=true")
	}
}

func TestDuplicateDetection(t *testing.T) {
	resetRegistry()
	noop := func(ctx context.Context, tx Executor) error { return nil }
	Register("100_x", noop, noop)
	Register("100_x", noop, noop)
	if validateRegistry() == nil {
		t.Fatal("expected duplicate error")
	}
}
