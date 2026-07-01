package querybuilder

import (
	"strings"
	"testing"
)

// Regression: ToSQL joined statements with "; " and Execute re-split on "; ",
// corrupting any clause whose body contains that substring (e.g. a default value).
func TestAlterTableStatementsDoNotSplitOnClauseSemicolons(t *testing.T) {
	stmts, err := AlterTable("t").
		SetDefault("note", "'a; b'").
		RenameTo("t2").
		statements()
	if err != nil {
		t.Fatal(err)
	}
	if len(stmts) != 2 {
		t.Fatalf("expected 2 statements, got %d: %#v", len(stmts), stmts)
	}
	if !strings.Contains(stmts[0], `SET DEFAULT 'a; b'`) {
		t.Fatalf("default clause was corrupted: %q", stmts[0])
	}
	if !strings.Contains(stmts[1], `RENAME TO "t2"`) {
		t.Fatalf("rename statement wrong: %q", stmts[1])
	}
}

// Regression: WithAutoIncrement on SmallInt must yield smallserial, not serial.
func TestAutoIncrementSmallIntUsesSmallSerial(t *testing.T) {
	c := columnDef{name: "id", typ: SmallInt}
	WithAutoIncrement()(&c)
	got, err := c.definitionSQL()
	if err != nil {
		t.Fatal(err)
	}
	if norm(got) != `"id" smallserial` {
		t.Fatalf("got %q, want %q", norm(got), `"id" smallserial`)
	}
	// integer and bigint mappings must remain unchanged.
	ci := columnDef{name: "a", typ: Int}
	WithAutoIncrement()(&ci)
	gi, _ := ci.definitionSQL()
	if norm(gi) != `"a" serial` {
		t.Fatalf("int: got %q", norm(gi))
	}
	cb := columnDef{name: "b", typ: BigInt}
	WithAutoIncrement()(&cb)
	gb, _ := cb.definitionSQL()
	if norm(gb) != `"b" bigserial` {
		t.Fatalf("bigint: got %q", norm(gb))
	}
}
