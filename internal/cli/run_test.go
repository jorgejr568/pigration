package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/jorgejr568/pigration/internal/config"
)

func loadTestConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load(DefaultConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestRunGeneratesEntrypoint(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	os.WriteFile("go.mod", []byte("module github.com/me/app\n\ngo 1.22\n"), 0o644)
	if err := runCmd("init"); err != nil {
		t.Fatal(err)
	}
	// prepareRunner should write .db-migration/runner/main.go without executing go run
	path, err := prepareRunner(loadTestConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("runner main.go not generated")
	}
	src, _ := os.ReadFile(path)
	if !strings.Contains(string(src), "github.com/me/app/migrations") {
		t.Fatal("wrong import")
	}
}
