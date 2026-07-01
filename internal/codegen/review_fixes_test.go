package codegen

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func mustParse(t *testing.T, content string) {
	t.Helper()
	if _, err := parser.ParseFile(token.NewFileSet(), "m.go", content, 0); err != nil {
		t.Fatalf("generated file does not parse: %v\n---\n%s", err, content)
	}
}

// Regression: a name whose first word is numeric produced `func 2faTokens...`,
// which is not a valid Go identifier and breaks the whole migrations package.
func TestRenderMigrationLeadingDigitProducesValidIdentifier(t *testing.T) {
	fname, content, err := RenderMigration("migrations", "2fa tokens", 1719800000)
	if err != nil {
		t.Fatal(err)
	}
	mustParse(t, content)
	if !strings.Contains(content, "1719800000Up(") {
		t.Fatalf("missing Up func:\n%s", content)
	}
	if strings.Contains(content, "func 2") {
		t.Fatalf("function identifier starts with a digit:\n%s", content)
	}
	if fname == "" || strings.ContainsAny(fname, `"`+"`"+`;/\ `) {
		t.Fatalf("bad filename: %q", fname)
	}
}

// Regression: an unescaped name with quotes/semicolons/dots was injected into the
// generated Go source (function names, filename, and the Register string literal).
func TestRenderMigrationSanitizesInjectionChars(t *testing.T) {
	fname, content, err := RenderMigration("migrations", `add"; DROP`, 1719800000)
	if err != nil {
		t.Fatal(err)
	}
	mustParse(t, content)
	if strings.ContainsAny(strings.TrimSuffix(fname, ".go"), `"`+"`"+`;/.`) {
		t.Fatalf("filename not sanitized: %q", fname)
	}
	if !strings.Contains(content, `migrator.Register("1719800000_add_drop"`) {
		t.Fatalf("register identity not sanitized:\n%s", content)
	}
	// dotted name likewise must not corrupt the file.
	_, content2, err := RenderMigration("migrations", "add.column", 1719800001)
	if err != nil {
		t.Fatal(err)
	}
	mustParse(t, content2)
}

// A name that reduces to nothing usable is still an error (unchanged behavior).
func TestRenderMigrationEmptyAfterSanitizeErrors(t *testing.T) {
	if _, _, err := RenderMigration("migrations", `!!! ...`, 1719800000); err == nil {
		t.Fatal("expected error for a name with no identifier characters")
	}
}
