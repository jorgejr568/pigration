package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runCmd builds a fresh root command, sets args, and executes it.
func runCmd(args ...string) error {
	root := newRootCmd()
	root.SetArgs(args)
	return root.Execute()
}

func TestInitScaffolds(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := runCmd("init"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(".db-migration.yaml"); err != nil {
		t.Fatal("config not created")
	}
	if _, err := os.Stat("migrations"); err != nil {
		t.Fatal("migrations dir not created")
	}
	gi, _ := os.ReadFile(".gitignore")
	if !strings.Contains(string(gi), "/.db-migration/") {
		t.Fatal(".gitignore not updated")
	}
	// idempotent: second run without --force does not error and does not overwrite
	if err := runCmd("init"); err != nil {
		t.Fatalf("re-init errored: %v", err)
	}
}

func TestInitDoesNotOverwriteConfig(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile(".db-migration.yaml", []byte("custom: content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runCmd("init"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(".db-migration.yaml")
	if string(data) != "custom: content\n" {
		t.Fatalf("init overwrote existing config: %q", data)
	}
}

func TestMakeCreatesTimestampedFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := runCmd("init"); err != nil {
		t.Fatal(err)
	}
	if err := runCmd("make", "create users"); err != nil {
		t.Fatal(err)
	}
	matches, _ := filepath.Glob("migrations/*_create_users.go")
	if len(matches) != 1 {
		t.Fatalf("expected 1 migration file, got %v", matches)
	}
}
