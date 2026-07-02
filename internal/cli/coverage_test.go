package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jorgejr568/pigration/internal/codegen"
	"github.com/jorgejr568/pigration/internal/config"
)

// TestExecute drives the exported Execute() entrypoint by manipulating os.Args
// in a temp working directory. `init` is benign and self-contained (no DB), so
// it exercises the real root-command construction and dispatch that Execute
// performs, and leaves observable artifacts on disk.
func TestExecute(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	saved := os.Args
	t.Cleanup(func() { os.Args = saved })
	os.Args = []string{"pigration", "init"}
	if err := Execute(); err != nil {
		t.Fatalf("Execute() init: %v", err)
	}
	if _, err := os.Stat(".db-migration.yaml"); err != nil {
		t.Fatal("Execute() init did not scaffold the config")
	}
}

// TestConfigPathFallbacks covers configPath's default-fallback arms: a command
// with no --config flag registered at all (GetString errors), and one where the
// flag is registered but explicitly empty.
func TestConfigPathFallbacks(t *testing.T) {
	t.Run("flag not registered falls back to default", func(t *testing.T) {
		// A bare make command has no --config flag until wired under root.
		cmd := newMakeCmd()
		if got := configPath(cmd); got != DefaultConfigPath {
			t.Fatalf("configPath=%q want default %q", got, DefaultConfigPath)
		}
	})

	t.Run("empty flag falls back to default", func(t *testing.T) {
		root := newRootCmd()
		// ParseFlags is how cobra populates the value configPath reads.
		if err := root.ParseFlags([]string{"--config", ""}); err != nil {
			t.Fatal(err)
		}
		if got := configPath(root); got != DefaultConfigPath {
			t.Fatalf("configPath=%q want default %q", got, DefaultConfigPath)
		}
	})

	t.Run("custom flag is honored", func(t *testing.T) {
		root := newRootCmd()
		if err := root.ParseFlags([]string{"--config", "/custom/path.yaml"}); err != nil {
			t.Fatal(err)
		}
		if got := configPath(root); got != "/custom/path.yaml" {
			t.Fatalf("configPath=%q want /custom/path.yaml", got)
		}
	})
}

// TestInitForceOverwrites covers the --force arm of runInit, which rewrites an
// existing config with the default template.
func TestInitForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile(".db-migration.yaml", []byte("custom: content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runCmd("init", "--force"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(".db-migration.yaml")
	if strings.Contains(string(data), "custom: content") {
		t.Fatalf("--force did not overwrite: %q", data)
	}
	if !strings.Contains(string(data), "${DATABASE_URL}") {
		t.Fatalf("--force wrote unexpected content: %q", data)
	}
}

// TestInitNestedConfigDir covers the MkdirAll(dir) arm of runInit where the
// config path has a non-trivial parent directory that must be created.
func TestInitNestedConfigDir(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := runCmd("--config", "nested/sub/.db-migration.yaml", "init"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat("nested/sub/.db-migration.yaml"); err != nil {
		t.Fatal("nested config not created")
	}
}

// TestEnsureGitignoreAppendsWithNewline covers ensureGitignore's prefix arm:
// when the existing .gitignore does not end in a newline, the new entry is
// prepended with one so it lands on its own line.
func TestEnsureGitignoreAppendsWithNewline(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	// No trailing newline on the existing content.
	if err := os.WriteFile(".gitignore", []byte("node_modules"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureGitignore("/.db-migration/"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(".gitignore")
	want := "node_modules\n/.db-migration/\n"
	if string(got) != want {
		t.Fatalf(".gitignore=%q want %q", got, want)
	}
	// Idempotent: a second call finds the entry and is a no-op.
	if err := ensureGitignore("/.db-migration/"); err != nil {
		t.Fatal(err)
	}
	got2, _ := os.ReadFile(".gitignore")
	if string(got2) != want {
		t.Fatalf("second ensureGitignore changed file: %q", got2)
	}
}

// TestEnsureGitignoreReadError covers the non-IsNotExist read-error arm by
// making .gitignore a directory, which ReadFile cannot read as a file.
func TestEnsureGitignoreReadError(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.Mkdir(".gitignore", 0o755); err != nil {
		t.Fatal(err)
	}
	err := ensureGitignore("/.db-migration/")
	if err == nil {
		t.Fatal("want error reading a directory as .gitignore")
	}
	if !strings.Contains(err.Error(), "reading .gitignore") {
		t.Fatalf("want reading-.gitignore wrap, got %v", err)
	}
}

// TestRunMakeErrors covers runMake's error arms: an invalid migration name
// (empty identifier from RenderMigration) and an already-existing destination.
func TestRunMakeErrors(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := runCmd("init"); err != nil {
		t.Fatal(err)
	}
	cfg := loadTestConfig(t)

	t.Run("empty identifier name", func(t *testing.T) {
		err := runMake(newMakeCmd(), cfg, "!!! ...", 1719800000)
		if err == nil || !strings.Contains(err.Error(), "empty identifier") {
			t.Fatalf("want empty-identifier error, got %v", err)
		}
	})

	t.Run("already exists", func(t *testing.T) {
		if err := runMake(newMakeCmd(), cfg, "create things", 1719800001); err != nil {
			t.Fatal(err)
		}
		// Second call with the same timestamp+name collides.
		err := runMake(newMakeCmd(), cfg, "create things", 1719800001)
		if err == nil || !strings.Contains(err.Error(), "already exists") {
			t.Fatalf("want already-exists error, got %v", err)
		}
	})
}

// TestMakeLoadConfigError covers the make RunE arm where loadConfig fails
// because no config file exists.
func TestMakeLoadConfigError(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	err := runCmd("make", "create users")
	if err == nil || !strings.Contains(err.Error(), "run `pigration init`") {
		t.Fatalf("want config-not-found error, got %v", err)
	}
}

// TestDBTouchingCommandsRequireConfig covers the loadConfig error arm of every
// DB-touching RunE closure (migrate/rollback/status/fresh) in a dir with no
// config — none of them reach `go run`.
func TestDBTouchingCommandsRequireConfig(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	for _, args := range [][]string{{"migrate"}, {"rollback"}, {"status"}, {"fresh"}} {
		err := runCmd(args...)
		if err == nil || !strings.Contains(err.Error(), "run `pigration init`") {
			t.Fatalf("%v: want config-not-found error, got %v", args, err)
		}
	}
}

// TestFreshRefusesWithoutOptIn covers newFreshCmd's refusal arm: with neither
// fresh.allow nor PIGRATION_ALLOW_FRESH=1, fresh must refuse before touching
// anything.
func TestFreshRefusesWithoutOptIn(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := runCmd("init"); err != nil { // default config has fresh.allow: false
		t.Fatal(err)
	}
	// Ensure the env opt-in is not set.
	t.Setenv(codegen.EnvAllowFresh, "")
	err := runCmd("fresh")
	if err == nil || !strings.Contains(err.Error(), "fresh refused") {
		t.Fatalf("want fresh-refused error, got %v", err)
	}
}

// TestFreshConfirmMismatch covers the confirm-mismatch path: fresh is opted in
// via config (fresh.allow: true), a determinable DB name exists, but the piped
// confirmation input does not match, so confirmFresh aborts before `go run`.
func TestFreshConfirmMismatch(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFreshOptInConfig(t, "postgres://u:p@localhost:5432/confirm_db?sslmode=disable")

	root := newRootCmd()
	root.SetArgs([]string{"fresh"})
	root.SetIn(strings.NewReader("wrong_name\n"))
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "confirmation did not match") {
		t.Fatalf("want confirmation-mismatch error, got %v", err)
	}
}

// TestFreshUndeterminableDBName covers the arm where fresh is opted in but the
// DB name cannot be determined (a kv-DSN URL), so it demands --force.
func TestFreshUndeterminableDBName(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	// A key/value DSN yields DBName() == "" (not a postgres:// URL).
	writeFreshOptInConfig(t, "host=localhost user=u dbname=kv sslmode=disable")

	err := runCmd("fresh")
	if err == nil || !strings.Contains(err.Error(), "cannot determine database name") {
		t.Fatalf("want undeterminable-name error, got %v", err)
	}
}

// TestFreshEnvOptIn covers the PIGRATION_ALLOW_FRESH=1 branch of the `allowed`
// expression (config fresh.allow is false, env grants it), combined with the
// undeterminable-name guard so no DB is touched.
func TestFreshEnvOptIn(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	// fresh.allow stays false; the env var provides the opt-in.
	writeConfig(t, "database:\n  url: host=localhost dbname=kv\n")
	t.Setenv(codegen.EnvAllowFresh, "1")
	err := runCmd("fresh")
	if err == nil || !strings.Contains(err.Error(), "cannot determine database name") {
		t.Fatalf("env opt-in should pass the allow gate and hit the name guard, got %v", err)
	}
}

// TestConfirmFresh covers confirmFresh directly across all three outcomes:
// a matching name (success), a mismatched name (error), and a read that returns
// no newline / EOF before a match (error). Input is driven through the command's
// InOrStdin so tests are deterministic.
func TestConfirmFresh(t *testing.T) {
	t.Run("match", func(t *testing.T) {
		cmd := newFreshCmd()
		cmd.SetIn(strings.NewReader("mydb\n"))
		cmd.SetOut(new(strings.Builder))
		if err := confirmFresh(cmd, "mydb"); err != nil {
			t.Fatalf("matching name should succeed, got %v", err)
		}
	})

	t.Run("match without trailing newline (EOF)", func(t *testing.T) {
		// ReadString returns the buffered "mydb" alongside io.EOF; the error is
		// ignored and the trimmed line still matches.
		cmd := newFreshCmd()
		cmd.SetIn(strings.NewReader("mydb"))
		cmd.SetOut(new(strings.Builder))
		if err := confirmFresh(cmd, "mydb"); err != nil {
			t.Fatalf("EOF-terminated match should succeed, got %v", err)
		}
	})

	t.Run("mismatch", func(t *testing.T) {
		cmd := newFreshCmd()
		cmd.SetIn(strings.NewReader("typo\n"))
		cmd.SetOut(new(strings.Builder))
		err := confirmFresh(cmd, "mydb")
		if err == nil || !strings.Contains(err.Error(), `did not match "mydb"`) {
			t.Fatalf("want mismatch error, got %v", err)
		}
	})

	t.Run("empty input aborts", func(t *testing.T) {
		cmd := newFreshCmd()
		cmd.SetIn(strings.NewReader(""))
		cmd.SetOut(new(strings.Builder))
		err := confirmFresh(cmd, "mydb")
		if err == nil || !strings.Contains(err.Error(), "did not match") {
			t.Fatalf("want mismatch error on empty input, got %v", err)
		}
	})
}

// TestRunViaGoRunDSNError covers the cfg.DSN() error arm of runViaGoRun: a
// config with no url and incomplete discrete fields passes prepareRunner (go.mod
// present) but fails to assemble a DSN, so runViaGoRun returns before `go run`.
func TestRunViaGoRunDSNError(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile("go.mod", []byte("module e2e\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll("migrations", 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.Migrations.Dir = "./migrations"
	cfg.Migrations.Table = "schema_migrations"
	// No URL, no host/user/name → DSN() errors "database.host is empty".
	err := runViaGoRun(newStatusCmd(), cfg, codegen.RunnerEnv{Cmd: "status"})
	if err == nil || !strings.Contains(err.Error(), "database.host is empty") {
		t.Fatalf("want DSN error, got %v", err)
	}
	// prepareRunner still ran: the runner main.go must be on disk.
	if _, statErr := os.Stat(filepath.Join(runnerDir, "main.go")); statErr != nil {
		t.Fatalf("prepareRunner should have written the runner: %v", statErr)
	}
}

// TestPrepareRunnerMkdirError covers prepareRunner's MkdirAll(runnerDir) error
// arm by placing a regular file where the runner directory's parent must be a
// directory (.db-migration is a file, so .db-migration/runner cannot be made).
func TestPrepareRunnerMkdirError(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile("go.mod", []byte("module e2e\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll("migrations", 0o755); err != nil {
		t.Fatal(err)
	}
	// Occupy ".db-migration" with a file so MkdirAll(".db-migration/runner") fails.
	if err := os.WriteFile(".db-migration", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.Migrations.Dir = "./migrations"
	err := prepareRunner(cfg)
	if err == nil || !strings.Contains(err.Error(), "creating runner dir") {
		t.Fatalf("want creating-runner-dir error, got %v", err)
	}
}

// TestPrepareRunnerWriteError covers prepareRunner's WriteFile(mainPath) error
// arm: the runner directory exists but is read-only, so MkdirAll is a no-op and
// the main.go write is refused.
func TestPrepareRunnerWriteError(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile("go.mod", []byte("module e2e\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll("migrations", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(runnerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(runnerDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(runnerDir, 0o755) })

	cfg := &config.Config{}
	cfg.Migrations.Dir = "./migrations"
	err := prepareRunner(cfg)
	if err == nil || !strings.Contains(err.Error(), "writing runner") {
		t.Fatalf("want writing-runner error, got %v", err)
	}
}

// TestRunInitConfigWriteError covers runInit's WriteFile(cfgPath) error arm:
// the config's parent dir exists but is read-only, so the file cannot be
// written even though the not-exist Stat guard passes and MkdirAll is a no-op.
func TestRunInitConfigWriteError(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.Mkdir("cfgdir", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod("cfgdir", 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod("cfgdir", 0o755) })
	err := runCmd("--config", "cfgdir/.db-migration.yaml", "init")
	if err == nil || !strings.Contains(err.Error(), "writing config") {
		t.Fatalf("want writing-config error, got %v", err)
	}
}

// TestRunInitConfigDirError covers the MkdirAll(config parent dir) error arm:
// the parent path is occupied by a regular file, so the dir cannot be created.
func TestRunInitConfigDirError(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	// "sub" is a file; init --config sub/x.yaml must MkdirAll("sub") and fail.
	if err := os.WriteFile("sub", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := runCmd("--config", "sub/x.yaml", "init")
	if err == nil || !strings.Contains(err.Error(), "creating config dir") {
		t.Fatalf("want creating-config-dir error, got %v", err)
	}
}

// TestRunInitMigrationsDirError covers runInit's MkdirAll("migrations") error
// arm: a regular file named "migrations" blocks the directory creation.
func TestRunInitMigrationsDirError(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile("migrations", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := runCmd("init")
	if err == nil || !strings.Contains(err.Error(), "creating migrations dir") {
		t.Fatalf("want creating-migrations-dir error, got %v", err)
	}
}

// TestRunInitGitignoreError covers the arm where ensureGitignore returns an
// error that runInit propagates: .gitignore is a directory.
func TestRunInitGitignoreError(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.Mkdir(".gitignore", 0o755); err != nil {
		t.Fatal(err)
	}
	err := runCmd("init")
	if err == nil || !strings.Contains(err.Error(), ".gitignore") {
		t.Fatalf("want .gitignore error propagated from runInit, got %v", err)
	}
}

// TestEnsureGitignoreOpenError covers ensureGitignore's OpenFile error arm.
// The parent directory is made read-only so O_CREATE cannot create .gitignore.
func TestEnsureGitignoreOpenError(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	// Read+execute but not write: OpenFile with O_CREATE fails to create.
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	err := ensureGitignore("/.db-migration/")
	if err == nil || !strings.Contains(err.Error(), "opening .gitignore") {
		t.Fatalf("want opening-.gitignore error, got %v", err)
	}
}

// TestRunMakeMkdirError covers runMake's MkdirAll(dir) error arm: the migrations
// dir path is occupied by a regular file.
func TestRunMakeMkdirError(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile("migrations", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.Migrations.Dir = "migrations"
	cfg.Migrations.Package = "migrations"
	err := runMake(newMakeCmd(), cfg, "create users", 1719800000)
	if err == nil || !strings.Contains(err.Error(), "creating migrations dir") {
		t.Fatalf("want creating-migrations-dir error, got %v", err)
	}
}

// TestRunMakeWriteError covers runMake's WriteFile(dest) error arm: the
// destination does not yet exist (so the Stat "already exists" guard passes),
// but the migrations directory is read-only, so the file write fails.
func TestRunMakeWriteError(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	// Pre-create the migrations dir, then make it read-only so MkdirAll is a
	// no-op (it exists) but WriteFile(dest) is refused.
	if err := os.MkdirAll("migrations", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod("migrations", 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod("migrations", 0o755) })

	cfg := &config.Config{}
	cfg.Migrations.Dir = "migrations"
	cfg.Migrations.Package = "migrations"
	err := runMake(newMakeCmd(), cfg, "create users", 1719800000)
	if err == nil || !strings.Contains(err.Error(), "writing migration") {
		t.Fatalf("want writing-migration error, got %v", err)
	}
}

// writeConfig writes a config file at the default path in the cwd.
func writeConfig(t *testing.T, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Base(DefaultConfigPath), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeFreshOptInConfig writes a config with fresh.allow: true and the given DB
// url, and ensures the env opt-in is not independently set.
func writeFreshOptInConfig(t *testing.T, dbURL string) {
	t.Helper()
	t.Setenv(codegen.EnvAllowFresh, "")
	writeConfig(t, "database:\n  url: "+dbURL+"\nfresh:\n  allow: true\n")
	// Sanity: the config parses and reports the intended opt-in.
	cfg, err := config.Load(DefaultConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Fresh.Allow {
		t.Fatal("fixture did not set fresh.allow: true")
	}
}
