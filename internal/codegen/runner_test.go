package codegen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestModuleImportPath(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module github.com/me/app\n\ngo 1.22\n"), 0o644)
	got, err := ModuleImportPath(filepath.Join(dir, "go.mod"), "./migrations")
	if err != nil {
		t.Fatal(err)
	}
	if got != "github.com/me/app/migrations" {
		t.Fatalf("got %q", got)
	}
}

func TestModuleImportPathNested(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module github.com/me/app\n\ngo 1.22\n"), 0o644)
	got, err := ModuleImportPath(filepath.Join(dir, "go.mod"), "db/migrations/")
	if err != nil {
		t.Fatal(err)
	}
	if got != "github.com/me/app/db/migrations" {
		t.Fatalf("got %q", got)
	}
}

func TestRenderRunnerCompiles(t *testing.T) {
	src, err := RenderRunner("github.com/me/app/migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`_ "github.com/me/app/migrations"`,
		`"github.com/jorgejr568/pigration/migrator"`,
		"pgxpool.New", "os.Getenv",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("runner missing %q", want)
		}
	}
}
