package codegen

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
)

// TestEnviron covers RunnerEnv.Environ across the full field matrix: the always-
// emitted DSN/table/cmd triple, the >0 guards on Steps and Batch (both emitted
// and both omitted), and the AllowFresh flag (set and unset). It asserts on the
// exact appended entries so a protocol drift is caught.
func TestEnviron(t *testing.T) {
	base := []string{"PATH=/usr/bin", "HOME=/home/x"}

	t.Run("all fields set", func(t *testing.T) {
		e := RunnerEnv{Cmd: "up", Steps: 3, Batch: 2, AllowFresh: true}
		got := e.Environ(base, "postgres://h/db", "schema_migrations")
		want := []string{
			"PATH=/usr/bin", "HOME=/home/x",
			EnvDSN + "=postgres://h/db",
			EnvTable + "=schema_migrations",
			EnvCmd + "=up",
			EnvSteps + "=3",
			EnvBatch + "=2",
			EnvAllowFresh + "=1",
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("Environ()=%v\nwant %v", got, want)
		}
	})

	t.Run("zero-valued steps and batch omitted, fresh off", func(t *testing.T) {
		e := RunnerEnv{Cmd: "status"} // Steps=0, Batch=0, AllowFresh=false
		got := e.Environ(nil, "dsn", "tbl")
		want := []string{
			EnvDSN + "=dsn",
			EnvTable + "=tbl",
			EnvCmd + "=status",
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("Environ()=%v\nwant %v", got, want)
		}
	})

	t.Run("negative steps and batch are omitted", func(t *testing.T) {
		// The guard is strictly > 0, so negatives are treated as unset.
		e := RunnerEnv{Cmd: "down", Steps: -1, Batch: -5}
		got := e.Environ(nil, "dsn", "tbl")
		for _, entry := range got {
			if entry == EnvSteps+"="+strconv.Itoa(-1) || entry == EnvBatch+"="+strconv.Itoa(-5) {
				t.Fatalf("negative value leaked into env: %v", got)
			}
		}
		if len(got) != 3 {
			t.Fatalf("want only DSN/table/cmd, got %v", got)
		}
	})

	t.Run("only steps set", func(t *testing.T) {
		e := RunnerEnv{Cmd: "up", Steps: 1}
		got := e.Environ(nil, "dsn", "tbl")
		want := []string{EnvDSN + "=dsn", EnvTable + "=tbl", EnvCmd + "=up", EnvSteps + "=1"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("Environ()=%v\nwant %v", got, want)
		}
	})

	t.Run("only batch set", func(t *testing.T) {
		e := RunnerEnv{Cmd: "down", Batch: 7}
		got := e.Environ(nil, "dsn", "tbl")
		want := []string{EnvDSN + "=dsn", EnvTable + "=tbl", EnvCmd + "=down", EnvBatch + "=7"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("Environ()=%v\nwant %v", got, want)
		}
	})
}

// TestModuleImportPathErrors covers the read-error and no-module-line arms.
func TestModuleImportPathErrors(t *testing.T) {
	t.Run("missing go.mod", func(t *testing.T) {
		_, err := ModuleImportPath(filepath.Join(t.TempDir(), "nope", "go.mod"), "./migrations")
		if err == nil {
			t.Fatal("want error for missing go.mod")
		}
	})

	t.Run("no module line", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "go.mod")
		// Valid-ish go.mod content but with no `module` directive.
		if err := os.WriteFile(p, []byte("go 1.22\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := ModuleImportPath(p, "./migrations")
		if err == nil {
			t.Fatal("want error when no module path is present")
		}
	})
}

// TestModuleImportPathRootDir covers the branch where the migrations dir cleans
// down to "" or "." — the import path is then the bare module path.
func TestModuleImportPathRootDir(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "go.mod")
	if err := os.WriteFile(p, []byte("module github.com/me/app\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, migDir := range []string{".", "./", ""} {
		got, err := ModuleImportPath(p, migDir)
		if err != nil {
			t.Fatalf("migDir %q: %v", migDir, err)
		}
		if got != "github.com/me/app" {
			t.Fatalf("migDir %q: got %q want bare module path", migDir, got)
		}
	}
}

// TestCamelCaseEdgeCases exercises CamelCase paths the naming tests don't:
// leading digits (kept as-is; leading char stays a digit here — the M-prefix
// fix lives in RenderMigration, not CamelCase), all-separator input (empty
// result), and mixed camel/acronym boundaries.
func TestCamelCaseEdgeCases(t *testing.T) {
	cases := map[string]string{
		"":                 "",
		"!!! ...":          "", // all separators → no words → empty
		"2fa tokens":       "2faTokens",
		"add HTTP proxy":   "AddHttpProxy",
		"createUsersTable": "CreateUsersTable",
	}
	for in, want := range cases {
		if got := CamelCase(in); got != want {
			t.Errorf("CamelCase(%q)=%q want %q", in, got, want)
		}
	}
}
