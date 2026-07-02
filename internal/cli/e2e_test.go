package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/jorgejr568/pigration/internal/codegen"
)

// repoRoot returns the absolute path to the module root (two levels up from
// internal/cli). It is the target of the temp module's replace directive.
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// internal/cli -> internal -> repo root
	root := filepath.Dir(filepath.Dir(wd))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("could not locate repo go.mod from %q: %v", root, err)
	}
	return root
}

// TestE2EFullLifecycle drives the DB-touching commands (migrate, status,
// rollback, fresh) in-process through runCmd so their RunE closures and
// runViaGoRun are counted for coverage. It requires a live Postgres via
// TEST_DATABASE_URL and is skipped otherwise. `go run` compilation of the
// generated runner requires a real module, so the test builds a throwaway
// module whose replace directive points at this repo.
func TestE2EFullLifecycle(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping live-DB e2e")
	}
	root := repoRoot(t)

	// Reset the target database to a clean, empty public schema so assertions
	// are deterministic regardless of prior runs.
	resetSchema(t, dbURL)

	// Build the throwaway module.
	modDir := t.TempDir()
	writeE2EModule(t, modDir, root)

	t.Chdir(modDir)
	// The CLI reads the DSN from the config's ${DATABASE_URL} interpolation.
	t.Setenv("DATABASE_URL", dbURL)
	// Ensure fresh is not env-opted-in unless a subtest wants it.
	t.Setenv(codegen.EnvAllowFresh, "")

	// Scaffold config + migrations dir, then one migration.
	if err := runCmd("init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	// init's default config uses ${DATABASE_URL}; that resolves via env above.
	if err := runCmd("make", "create widgets"); err != nil {
		t.Fatalf("make: %v", err)
	}
	// Rewrite the generated migration to a deterministic table so SQL assertions
	// are unambiguous (the default template creates a table named "example").
	rewriteMigration(t, "widgets")

	ctx := context.Background()

	// status before migrate: exactly one pending migration, no tables yet.
	if err := runCmd("status"); err != nil {
		t.Fatalf("status (pre-migrate): %v", err)
	}
	if tableExists(t, ctx, dbURL, "widgets") {
		t.Fatal("widgets table should not exist before migrate")
	}

	// migrate: applies the migration, creating the widgets table.
	if err := runCmd("migrate"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !tableExists(t, ctx, dbURL, "widgets") {
		t.Fatal("widgets table should exist after migrate")
	}
	if n := appliedCount(t, ctx, dbURL, "schema_migrations"); n != 1 {
		t.Fatalf("schema_migrations rows after migrate=%d want 1", n)
	}

	// migrate again is a no-op (nothing pending).
	if err := runCmd("migrate"); err != nil {
		t.Fatalf("migrate (idempotent): %v", err)
	}

	// rollback --steps 1: drops the widgets table.
	if err := runCmd("rollback", "--steps", "1"); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if tableExists(t, ctx, dbURL, "widgets") {
		t.Fatal("widgets table should be gone after rollback")
	}
	if n := appliedCount(t, ctx, dbURL, "schema_migrations"); n != 0 {
		t.Fatalf("schema_migrations rows after rollback=%d want 0", n)
	}

	// fresh --force with env opt-in: drops the public schema and re-applies.
	t.Setenv(codegen.EnvAllowFresh, "1")
	if err := runCmd("fresh", "--force"); err != nil {
		t.Fatalf("fresh --force: %v", err)
	}
	if !tableExists(t, ctx, dbURL, "widgets") {
		t.Fatal("widgets table should exist after fresh")
	}
	if n := appliedCount(t, ctx, dbURL, "schema_migrations"); n != 1 {
		t.Fatalf("schema_migrations rows after fresh=%d want 1", n)
	}

	// fresh with confirmation typed through stdin (name matches → proceeds).
	// Use the concrete database name parsed from the DSN.
	dbName := dbNameFromURL(t, dbURL)
	freshRoot := newRootCmd()
	freshRoot.SetArgs([]string{"fresh"})
	freshRoot.SetIn(strings.NewReader(dbName + "\n"))
	if err := freshRoot.Execute(); err != nil {
		t.Fatalf("fresh (confirmed): %v", err)
	}
	if !tableExists(t, ctx, dbURL, "widgets") {
		t.Fatal("widgets table should exist after confirmed fresh")
	}
}

// TestE2ERunViaGoRunFailure covers runViaGoRun's `go run` error-wrapping arm:
// with an unreachable DSN, the runner compiles and runs but exits non-zero, so
// c.Run() returns an error that runViaGoRun wraps as "running migrations".
func TestE2ERunViaGoRunFailure(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping live-DB e2e")
	}
	root := repoRoot(t)
	modDir := t.TempDir()
	writeE2EModule(t, modDir, root)
	t.Chdir(modDir)
	// Point at a port with no server so pgxpool/connect fails inside the runner.
	t.Setenv("DATABASE_URL", "postgres://postgres:pw@127.0.0.1:1/nope?sslmode=disable&connect_timeout=2")

	if err := runCmd("init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := runCmd("make", "create widgets"); err != nil {
		t.Fatalf("make: %v", err)
	}
	err := runCmd("migrate")
	if err == nil {
		t.Fatal("migrate against a dead DSN should fail")
	}
	if !strings.Contains(err.Error(), "running migrations") {
		t.Fatalf("want runViaGoRun wrap, got %v", err)
	}
}

// TestE2EMissingGoMod covers prepareRunner's ModuleImportPath error arm reached
// through a DB-touching command: with no go.mod in the working directory,
// runViaGoRun fails before executing anything.
func TestE2EMissingGoMod(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping live-DB e2e")
	}
	dir := t.TempDir()
	t.Chdir(dir)
	// init writes a config but not a go.mod.
	if err := runCmd("init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	t.Setenv("DATABASE_URL", "postgres://postgres:pw@127.0.0.1:5433/cov_cli?sslmode=disable")
	err := runCmd("status")
	if err == nil {
		t.Fatal("status without go.mod should fail in prepareRunner")
	}
	if !strings.Contains(err.Error(), "go.mod") {
		t.Fatalf("want go.mod read error, got %v", err)
	}
}

// TestE2EURLConflictWarning covers the URLConflicts warning arm of runViaGoRun:
// a config with both url and discrete fields set emits the warning to stderr
// before running. Paired with a dead DSN so no real DB work happens; we only
// assert the warning is written.
func TestE2EURLConflictWarning(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping live-DB e2e")
	}
	root := repoRoot(t)
	modDir := t.TempDir()
	writeE2EModule(t, modDir, root)
	t.Chdir(modDir)

	// url + discrete host set → URLConflicts() is true.
	cfg := "database:\n" +
		"  url: postgres://postgres:pw@127.0.0.1:1/nope?sslmode=disable&connect_timeout=2\n" +
		"  host: ignored-host\n" +
		"migrations:\n  dir: ./migrations\n  package: migrations\n"
	if err := os.WriteFile(".db-migration.yaml", []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll("migrations", 0o755); err != nil {
		t.Fatal(err)
	}

	stderr := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetArgs([]string{"status"})
	cmd.SetErr(stderr)
	cmd.SetOut(new(strings.Builder))
	// The command will fail at `go run` (dead DSN), but the warning fires first.
	_ = cmd.Execute()
	if !strings.Contains(stderr.String(), "ignoring discrete database.* fields") {
		t.Fatalf("expected url-conflict warning on stderr, got %q", stderr.String())
	}
}

// --- e2e helpers ---

// writeE2EModule writes a go.mod for a throwaway module that replaces the
// pigration dependency with the local repo, then runs `go mod tidy` so `go run`
// can compile the generated runner against real deps from the module cache.
func writeE2EModule(t *testing.T, modDir, repoRoot string) {
	t.Helper()
	gomod := "module e2e\n\ngo 1.26\n\n" +
		"require github.com/jorgejr568/pigration v0.0.0\n\n" +
		"replace github.com/jorgejr568/pigration => " + repoRoot + "\n"
	if err := os.WriteFile(filepath.Join(modDir, "go.mod"), []byte(gomod), 0o644); err != nil {
		t.Fatal(err)
	}
	// A placeholder .go file gives `go mod tidy` a package to anchor on and
	// pulls the transitive deps (pgx, etc.) referenced by the pigration module.
	anchor := "package e2e\n\nimport (\n" +
		"\t_ \"github.com/jorgejr568/pigration/migrator\"\n" +
		"\t_ \"github.com/jackc/pgx/v5/pgxpool\"\n" +
		")\n"
	if err := os.WriteFile(filepath.Join(modDir, "anchor.go"), []byte(anchor), 0o644); err != nil {
		t.Fatal(err)
	}
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = modDir
	tidy.Env = os.Environ()
	if out, err := tidy.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy failed (module cache should have deps): %v\n%s", err, out)
	}
}

// rewriteMigration replaces the single generated migration file's up/down SQL
// with deterministic CREATE/DROP TABLE statements for the given table, keeping
// the generated function/register wiring intact.
func rewriteMigration(t *testing.T, table string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join("migrations", "*.go"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("expected exactly one migration file, got %v (err %v)", matches, err)
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	src = strings.Replace(src,
		"CREATE TABLE example (\n\tid         bigserial   PRIMARY KEY,\n\tcreated_at timestamptz NOT NULL DEFAULT now()\n);",
		"CREATE TABLE "+table+" (id bigserial PRIMARY KEY);", 1)
	src = strings.Replace(src, "DROP TABLE example;", "DROP TABLE "+table+";", 1)
	if !strings.Contains(src, "CREATE TABLE "+table) || !strings.Contains(src, "DROP TABLE "+table) {
		t.Fatalf("migration rewrite failed; content:\n%s", src)
	}
	if err := os.WriteFile(matches[0], []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

// resetSchema drops and recreates the public schema on the target DB.
func resetSchema(t *testing.T, dsn string) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect for reset: %v", err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, "DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public;"); err != nil {
		t.Fatalf("reset schema: %v", err)
	}
}

func tableExists(t *testing.T, ctx context.Context, dsn, table string) bool {
	t.Helper()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect for tableExists: %v", err)
	}
	defer conn.Close(ctx)
	var exists bool
	err = conn.QueryRow(ctx,
		"SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name=$1)",
		table).Scan(&exists)
	if err != nil {
		t.Fatalf("tableExists query: %v", err)
	}
	return exists
}

func appliedCount(t *testing.T, ctx context.Context, dsn, table string) int {
	t.Helper()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect for appliedCount: %v", err)
	}
	defer conn.Close(ctx)
	var exists bool
	if err := conn.QueryRow(ctx,
		"SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name=$1)",
		table).Scan(&exists); err != nil {
		t.Fatalf("appliedCount existence: %v", err)
	}
	if !exists {
		return 0
	}
	var n int
	if err := conn.QueryRow(ctx, "SELECT count(*) FROM "+pgx.Identifier{table}.Sanitize()).Scan(&n); err != nil {
		t.Fatalf("appliedCount: %v", err)
	}
	return n
}

func dbNameFromURL(t *testing.T, dsn string) string {
	t.Helper()
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	if cfg.Database == "" {
		t.Fatalf("no database in dsn %q", dsn)
	}
	return cfg.Database
}
