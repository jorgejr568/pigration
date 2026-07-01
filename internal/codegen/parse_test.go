package codegen

import (
	"go/parser"
	"go/token"
	"testing"
)

// TestRenderedFilesParse ensures the generated migration file and runner
// entrypoint are syntactically valid Go.
func TestRenderedFilesParse(t *testing.T) {
	_, mig, err := RenderMigration("migrations", "Create Users", 1719800000)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "migration.go", mig, parser.AllErrors); err != nil {
		t.Fatalf("generated migration does not parse: %v\n%s", err, mig)
	}

	run, err := RenderRunner("github.com/me/app/migrations")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "runner.go", run, parser.AllErrors); err != nil {
		t.Fatalf("generated runner does not parse: %v\n%s", err, run)
	}
}
