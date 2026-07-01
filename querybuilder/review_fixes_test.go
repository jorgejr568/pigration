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
