# Migration Engine (pigration) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the `pigration` migration engine, config loader, public library API, and Cobra CLI for Go + Postgres, where migrations are self-registering Go code.

**Architecture:** A `migrator` package holds the registry, transactional runner, tracking table, and public API (`Up/Down/Fresh/Status`). An `internal/config` loader parses `.db-migration.yaml` with `${}` env interpolation. `internal/codegen` renders migration files and a throwaway `go run` entrypoint. `cmd/pigration` + `internal/cli` provide the Cobra commands; the DB-touching commands shell out to `go run` so user migration code is compiled in.

**Tech Stack:** Go 1.22+, `github.com/jackc/pgx/v5` (+ `pgxpool`, `pgconn`), `github.com/spf13/cobra`, `gopkg.in/yaml.v3`.

## Global Constraints

- Module path: `github.com/jorgejr568/pigration` (verbatim).
- Postgres only; driver is pgx v5. Connections are `*pgxpool.Pool`; transactions are `pgx.Tx`.
- `go.mod` and its dependencies are pre-created by the orchestrator before this plan runs. **Do not run `go mod tidy` or `go get`** — build/test only your own packages (`go test ./migrator/...`, etc.). The orchestrator runs `go mod tidy` after all work lands.
- The tracking-table name is configurable, default `schema_migrations`.
- Registered migration identity format: `"<unixTimestamp>_<snake_name>"` (sorts chronologically as a string).
- Integration tests requiring Postgres read `TEST_DATABASE_URL` and **skip** (not fail) when it is unset: `if os.Getenv("TEST_DATABASE_URL") == "" { t.Skip("set TEST_DATABASE_URL") }`.
- Do NOT edit files under `querybuilder/` — that is a separate parallel plan. The only shared contract is the pgx interface (Executor is a structural superset of querybuilder's Execer); do not import `querybuilder`.

## Shared Interface Seam (fixed — copy verbatim)

```go
// migrator/executor.go
package migrator

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Executor is the database handle passed to every migration function. It is a
// superset of querybuilder.Execer, so a migration may call
// qb...Execute(ctx, tx) directly. Satisfied by pgx.Tx, *pgxpool.Pool, *pgx.Conn.
type Executor interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// MigrationFunc is the signature of an Up or Down function.
type MigrationFunc func(ctx context.Context, tx Executor) error
```

---

## File Structure

- `migrator/executor.go` — `Executor` interface, `MigrationFunc` (above).
- `migrator/registry.go` — package registry, `Register`, `RegisterOption`, `NonTransactional`, ordered lookup, duplicate detection.
- `migrator/errors.go` — sentinel errors (`ErrFreshNotAllowed`, `ErrLocked`, wrappers).
- `migrator/runner.go` — `Up`, `Down`, `Fresh`, `Status`, `RunOption` (`Steps`, `Batch`, `AllowFresh`), `Result`, `MigrationStatus`, `Options`, tracking-table bootstrap, advisory lock.
- `internal/config/config.go` — `Config`, `Load`, `${}` interpolation, DSN assembly.
- `internal/codegen/codegen.go` — migration-file template + render, runner-entrypoint template + render, import-path resolution from `go.mod`.
- `internal/cli/root.go` — root command + `--config`.
- `internal/cli/init.go`, `make.go`, `run.go` (migrate/rollback/status/fresh share the go-run helper).
- `cmd/pigration/main.go` — entrypoint calling `cli.Execute()`.
- `README.md`, `docs/assets/pig.svg` — branding.
- Tests colocated as `*_test.go`.

---

## Task 1: Config loader — interpolation & DSN

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces:
  ```go
  type Config struct {
      Database struct {
          URL, Host, Port, User, Password, Name, SSLMode string
      }
      Migrations struct{ Dir, Package, Table string }
      Fresh struct{ Allow bool }
  }
  func Load(path string) (*Config, error)          // reads file, interpolates, applies defaults
  func Interpolate(s string, getenv func(string) string) string
  func (c *Config) DSN() (string, error)           // url wins; else assemble; error if empty
  ```
- Defaults applied in `Load`: `Migrations.Dir=./migrations`, `Migrations.Package=migrations`, `Migrations.Table=schema_migrations`, `Database.SSLMode=disable` (discrete mode only).

- [ ] **Step 1: Write failing tests for interpolation**

```go
func TestInterpolate(t *testing.T) {
	env := map[string]string{"DB_HOST": "db.local", "EMPTY": ""}
	get := func(k string) string { return env[k] }
	cases := []struct{ in, want string }{
		{"${DB_HOST}", "db.local"},
		{"${MISSING}", ""},
		{"${MISSING:-localhost}", "localhost"},
		{"${EMPTY:-fallback}", "fallback"}, // empty triggers default
		{"${DB_HOST:-x}", "db.local"},
		{"literal", "literal"},
		{"postgres://${DB_HOST}/app", "postgres://db.local/app"},
	}
	for _, c := range cases {
		if got := Interpolate(c.in, get); got != c.want {
			t.Errorf("Interpolate(%q)=%q want %q", c.in, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run to verify failure** — `go test ./internal/config/ -run TestInterpolate -v` → FAIL (undefined `Interpolate`).

- [ ] **Step 3: Implement `Interpolate`** — regexp `\$\{([A-Za-z_][A-Za-z0-9_]*)(:-([^}]*))?\}`; for each match, look up env; if value is empty and a `:-default` group exists, use the default; replace literally otherwise.

- [ ] **Step 4: Run** — same command → PASS.

- [ ] **Step 5: Write failing tests for `DSN` precedence + `Load` defaults**

```go
func TestDSN(t *testing.T) {
	var c Config
	c.Database.URL = "postgres://u:p@h/db"
	if got, _ := c.DSN(); got != "postgres://u:p@h/db" { t.Fatalf("url should win, got %q", got) }

	c = Config{}
	c.Database.Host, c.Database.Port = "h", "5432"
	c.Database.User, c.Database.Password, c.Database.Name = "u", "p", "db"
	c.Database.SSLMode = "disable"
	got, err := c.DSN()
	if err != nil { t.Fatal(err) }
	want := "postgres://u:p@h:5432/db?sslmode=disable"
	if got != want { t.Fatalf("DSN=%q want %q", got, want) }

	if _, err := (&Config{}).DSN(); err == nil { t.Fatal("empty config must error") }
}

func TestLoadDefaults(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, ".db-migration.yaml")
	os.WriteFile(p, []byte("database:\n  url: ${TESTURL}\n"), 0o644)
	t.Setenv("TESTURL", "postgres://x/y")
	c, err := Load(p)
	if err != nil { t.Fatal(err) }
	if c.Migrations.Dir != "./migrations" || c.Migrations.Table != "schema_migrations" || c.Migrations.Package != "migrations" {
		t.Fatalf("defaults not applied: %+v", c.Migrations)
	}
	if got, _ := c.DSN(); got != "postgres://x/y" { t.Fatalf("interpolated DSN wrong: %q", got) }
}
```

- [ ] **Step 6: Run to verify failure** — `go test ./internal/config/ -v` → FAIL.

- [ ] **Step 7: Implement `Load` + `DSN`** — `Load`: `os.ReadFile` → `yaml.Unmarshal` into a raw struct, walk every string field through `Interpolate(_, os.Getenv)`, apply defaults, return. Missing file → error mentioning `pigration init`. `DSN`: if `URL != ""` return it (log a warning to stderr if discrete fields also set); else require host/user/name non-empty (error naming the first empty one) and build `postgres://user:password@host:port/name?sslmode=...`, URL-escaping user/password.

- [ ] **Step 8: Run** — `go test ./internal/config/ -v` → PASS.

- [ ] **Step 9: Commit** — `git add internal/config && git commit -m "feat(config): yaml loader with env interpolation and DSN"`

---

## Task 2: Registry & registration

**Files:**
- Create: `migrator/executor.go` (verbatim from Shared Interface Seam), `migrator/registry.go`
- Test: `migrator/registry_test.go`

**Interfaces:**
- Produces:
  ```go
  type migration struct {
      name             string
      up, down         MigrationFunc
      nonTransactional bool
  }
  type RegisterOption func(*migration)
  func NonTransactional() RegisterOption
  func Register(name string, up, down MigrationFunc, opts ...RegisterOption)
  func Registered() []migration      // sorted by name; internal accessor for runner/tests
  func resetRegistry()               // test helper
  func validateRegistry() error      // duplicate-name detection
  ```

- [ ] **Step 1: Write failing tests**

```go
func TestRegisterOrdersByName(t *testing.T) {
	resetRegistry()
	noop := func(ctx context.Context, tx Executor) error { return nil }
	Register("200_b", noop, noop)
	Register("100_a", noop, noop)
	got := Registered()
	if got[0].name != "100_a" || got[1].name != "200_b" {
		t.Fatalf("not sorted: %v", []string{got[0].name, got[1].name})
	}
}

func TestNonTransactionalOption(t *testing.T) {
	resetRegistry()
	noop := func(ctx context.Context, tx Executor) error { return nil }
	Register("100_x", noop, noop, NonTransactional())
	if !Registered()[0].nonTransactional { t.Fatal("expected nonTransactional=true") }
}

func TestDuplicateDetection(t *testing.T) {
	resetRegistry()
	noop := func(ctx context.Context, tx Executor) error { return nil }
	Register("100_x", noop, noop)
	Register("100_x", noop, noop)
	if validateRegistry() == nil { t.Fatal("expected duplicate error") }
}
```

- [ ] **Step 2: Run to verify failure** — `go test ./migrator/ -run 'TestRegister|TestNon|TestDuplicate' -v` → FAIL.

- [ ] **Step 3: Implement** — package-level `var registry []migration`. `Register` appends; options mutate the `migration`. `Registered` returns a copy sorted by `name` (`sort.Slice`). `validateRegistry` scans for duplicate names. `resetRegistry` sets `registry = nil`.

- [ ] **Step 4: Run** → PASS.

- [ ] **Step 5: Commit** — `git commit -am "feat(migrator): registry, Register, NonTransactional"`

---

## Task 3: Tracking table + Up runner

**Files:**
- Create: `migrator/runner.go`, `migrator/errors.go`
- Test: `migrator/runner_up_test.go`

**Interfaces:**
- Produces:
  ```go
  type Options struct{ Table string }        // Table default "schema_migrations"
  type Result struct{ Applied, RolledBack []string; Batch int }
  type RunOption func(*runConfig)
  func Steps(n int) RunOption
  func Batch(k int) RunOption
  func AllowFresh() RunOption
  func Up(ctx context.Context, pool *pgxpool.Pool, opts ...RunOption) (Result, error)
  // internal: ensureTable, appliedSet, acquireLock/releaseLock, tableName(runConfig)
  ```
- `runConfig` carries `table string`, `steps int` (0 = all), `batch *int`, `allowFresh bool`. `RunOption`s and `Options` both feed it; add `func WithOptions(Options) RunOption`.

- [ ] **Step 1: Write failing integration test**

```go
func TestUpAppliesPendingAndRecordsBatch(t *testing.T) {
	pool := testPool(t) // helper: skips if TEST_DATABASE_URL unset; drops schema public first
	resetRegistry()
	Register("100_a", makeCreate("a"), makeDrop("a"))
	Register("200_b", makeCreate("b"), makeDrop("b"))

	res, err := Up(context.Background(), pool)
	if err != nil { t.Fatal(err) }
	if len(res.Applied) != 2 || res.Batch != 1 {
		t.Fatalf("got applied=%v batch=%d", res.Applied, res.Batch)
	}
	// second run applies nothing
	res2, _ := Up(context.Background(), pool)
	if len(res2.Applied) != 0 { t.Fatalf("expected 0 applied, got %v", res2.Applied) }
	// both tables exist
	for _, tbl := range []string{"a", "b"} { assertTableExists(t, pool, tbl) }
}

func TestUpFailureRollsBackAndStops(t *testing.T) {
	pool := testPool(t)
	resetRegistry()
	Register("100_a", makeCreate("a"), makeDrop("a"))
	Register("200_bad", func(ctx context.Context, tx Executor) error {
		tx.Exec(ctx, `CREATE TABLE will_rollback (id int);`)
		return fmt.Errorf("boom")
	}, func(ctx context.Context, tx Executor) error { return nil })

	_, err := Up(context.Background(), pool)
	if err == nil { t.Fatal("expected error") }
	assertTableExists(t, pool, "a")         // committed before failure
	assertTableMissing(t, pool, "will_rollback") // rolled back
}
```

Add a `migrator/testutil_test.go` with `testPool`, `makeCreate`, `makeDrop`, `assertTableExists`, `assertTableMissing`. `testPool` skips when `TEST_DATABASE_URL` unset, connects, runs `DROP SCHEMA public CASCADE; CREATE SCHEMA public;` for isolation, returns the pool.

- [ ] **Step 2: Run to verify failure** — `go test ./migrator/ -run TestUp -v` → FAIL.

- [ ] **Step 3: Implement `ensureTable`, lock, and `Up`** — `tableName` from runConfig (default `schema_migrations`). `ensureTable`: `CREATE TABLE IF NOT EXISTS <table> (id bigserial primary key, name text not null unique, batch int not null, applied_at timestamptz not null default now())`. `Up`: acquire advisory lock (`SELECT pg_advisory_lock(<const int64>)`, release with `pg_advisory_unlock` in defer); `ensureTable`; load applied names (`SELECT name FROM <table>`); compute pending = `Registered()` minus applied, already sorted; `nextBatch = SELECT COALESCE(MAX(batch),0)+1`; iterate pending (limited by `steps`), for transactional ones `pool.Begin` → `up(ctx, tx)` → `INSERT` row → `Commit` (rollback+return on error, wrapping with migration name); for `nonTransactional`, run `up(ctx, pool)` then `INSERT`. Return `Result{Applied, Batch: nextBatch}`.

- [ ] **Step 4: Run** — `TEST_DATABASE_URL=postgres://... go test ./migrator/ -run TestUp -v` → PASS.

- [ ] **Step 5: Commit** — `git commit -am "feat(migrator): tracking table bootstrap and Up runner with per-migration tx"`

---

## Task 4: Down / rollback

**Files:**
- Modify: `migrator/runner.go`
- Test: `migrator/runner_down_test.go`

**Interfaces:**
- Produces: `func Down(ctx context.Context, pool *pgxpool.Pool, opts ...RunOption) (Result, error)`

- [ ] **Step 1: Write failing tests**

```go
func TestDownRollsBackLastBatch(t *testing.T) {
	pool := testPool(t)
	resetRegistry()
	Register("100_a", makeCreate("a"), makeDrop("a"))
	Register("200_b", makeCreate("b"), makeDrop("b"))
	Up(context.Background(), pool)                 // batch 1: a, b
	Register("300_c", makeCreate("c"), makeDrop("c"))
	Up(context.Background(), pool)                 // batch 2: c

	res, err := Down(context.Background(), pool)   // default: last batch (c)
	if err != nil { t.Fatal(err) }
	if len(res.RolledBack) != 1 || res.RolledBack[0] != "300_c" {
		t.Fatalf("expected rollback of 300_c, got %v", res.RolledBack)
	}
	assertTableMissing(t, pool, "c")
	assertTableExists(t, pool, "a")
}

func TestDownStepsReverseOrder(t *testing.T) {
	pool := testPool(t)
	resetRegistry()
	Register("100_a", makeCreate("a"), makeDrop("a"))
	Register("200_b", makeCreate("b"), makeDrop("b"))
	Up(context.Background(), pool)
	res, _ := Down(context.Background(), pool, Steps(2)) // both, most recent first
	if res.RolledBack[0] != "200_b" || res.RolledBack[1] != "100_a" {
		t.Fatalf("reverse order wrong: %v", res.RolledBack)
	}
}
```

- [ ] **Step 2: Run to verify failure** → FAIL.

- [ ] **Step 3: Implement `Down`** — acquire lock, ensureTable. Determine targets: default → names where `batch = MAX(batch)`; `Batch(k)` → names where `batch=k`; `Steps(n)` → last `n` rows ordered by `id DESC`. Load target names ordered most-recent-first. For each, find its registered `migration` (error if not registered), run `down` in its own tx (unless nonTransactional), `DELETE FROM <table> WHERE name=$1` on success. Stop on first error. Return `Result{RolledBack}`.

- [ ] **Step 4: Run** → PASS.

- [ ] **Step 5: Commit** — `git commit -am "feat(migrator): Down/rollback by batch or steps"`

---

## Task 5: Status

**Files:**
- Modify: `migrator/runner.go`
- Test: `migrator/runner_status_test.go`

**Interfaces:**
- Produces:
  ```go
  type MigrationStatus struct {
      Name string; Batch *int; AppliedAt *time.Time; Pending bool
  }
  func Status(ctx context.Context, pool *pgxpool.Pool, opts ...RunOption) ([]MigrationStatus, error)
  ```

- [ ] **Step 1: Write failing test**

```go
func TestStatusMarksPending(t *testing.T) {
	pool := testPool(t)
	resetRegistry()
	Register("100_a", makeCreate("a"), makeDrop("a"))
	Register("200_b", makeCreate("b"), makeDrop("b"))
	Up(context.Background(), pool, Steps(1)) // apply only 100_a
	st, err := Status(context.Background(), pool)
	if err != nil { t.Fatal(err) }
	byName := map[string]MigrationStatus{}
	for _, s := range st { byName[s.Name] = s }
	if byName["100_a"].Pending { t.Fatal("100_a should be applied") }
	if !byName["200_b"].Pending { t.Fatal("200_b should be pending") }
}
```

- [ ] **Step 2: Run to verify failure** → FAIL.

- [ ] **Step 3: Implement `Status`** — ensureTable; `SELECT name,batch,applied_at FROM <table>` into a map; for each registered name emit a `MigrationStatus` (Pending if absent from map, else fill Batch/AppliedAt); also emit rows applied-but-not-registered (Pending=false). Sort by Name.

- [ ] **Step 4: Run** → PASS.

- [ ] **Step 5: Commit** — `git commit -am "feat(migrator): Status with pending detection"`

---

## Task 6: Fresh + safety guard

**Files:**
- Modify: `migrator/runner.go`, `migrator/errors.go`
- Test: `migrator/runner_fresh_test.go`

**Interfaces:**
- Produces: `func Fresh(ctx context.Context, pool *pgxpool.Pool, opts ...RunOption) (Result, error)`; `var ErrFreshNotAllowed = errors.New("fresh not allowed: set AllowFresh() / PIGRATION_ALLOW_FRESH=1")`. `AllowFresh()` sets `runConfig.allowFresh=true` (from Task 3).

- [ ] **Step 1: Write failing tests**

```go
func TestFreshRequiresAllow(t *testing.T) {
	pool := testPool(t)
	if _, err := Fresh(context.Background(), pool); !errors.Is(err, ErrFreshNotAllowed) {
		t.Fatalf("expected ErrFreshNotAllowed, got %v", err)
	}
}

func TestFreshWipesAndReMigrates(t *testing.T) {
	pool := testPool(t)
	resetRegistry()
	Register("100_a", makeCreate("a"), makeDrop("a"))
	Up(context.Background(), pool)
	// create a stray table not managed by migrations
	pool.Exec(context.Background(), `CREATE TABLE stray (id int);`)

	res, err := Fresh(context.Background(), pool, AllowFresh())
	if err != nil { t.Fatal(err) }
	if len(res.Applied) != 1 { t.Fatalf("expected 1 applied, got %v", res.Applied) }
	assertTableExists(t, pool, "a")        // re-created
	assertTableMissing(t, pool, "stray")   // wiped by DROP SCHEMA
}
```

- [ ] **Step 2: Run to verify failure** → FAIL.

- [ ] **Step 3: Implement `Fresh`** — if `!runConfig.allowFresh` return `ErrFreshNotAllowed`. Acquire lock; `DROP SCHEMA public CASCADE; CREATE SCHEMA public;` (single Exec); then call the same core apply-all logic as `Up` (refactor `Up`'s body into `applyPending` so `Fresh` reuses it) and return its `Result`.

- [ ] **Step 4: Run** → PASS.

- [ ] **Step 5: Commit** — `git commit -am "feat(migrator): Fresh with DROP SCHEMA + AllowFresh guard"`

---

## Task 7: Codegen — migration template & make naming

**Files:**
- Create: `internal/codegen/codegen.go`
- Test: `internal/codegen/codegen_test.go`

**Interfaces:**
- Produces:
  ```go
  type MigrationData struct{ Package, FuncBase, Name string } // Name = "<ts>_<snake>"
  func RenderMigration(pkg, rawName string, ts int64) (filename string, content string, err error)
  func SnakeCase(s string) string
  func CamelCase(s string) string
  ```

- [ ] **Step 1: Write failing tests**

```go
func TestRenderMigrationNaming(t *testing.T) {
	fname, content, err := RenderMigration("migrations", "Create Users", 1719800000)
	if err != nil { t.Fatal(err) }
	if fname != "1719800000_create_users.go" { t.Fatalf("filename=%q", fname) }
	for _, want := range []string{
		"package migrations",
		"func CreateUsers1719800000Up(ctx context.Context, tx migrator.Executor) error",
		"func CreateUsers1719800000Down(ctx context.Context, tx migrator.Executor) error",
		`migrator.Register("1719800000_create_users",`,
		`"github.com/jorgejr568/pigration/migrator"`,
	} {
		if !strings.Contains(content, want) { t.Errorf("missing %q", want) }
	}
}
```

- [ ] **Step 2: Run to verify failure** → FAIL.

- [ ] **Step 3: Implement** — `SnakeCase`/`CamelCase` helpers; a `text/template` for the migration file (matching the spec's template, with example `CREATE TABLE`/`DROP TABLE` bodies and a commented `migrator.NonTransactional()` hint). `RenderMigration` builds `FuncBase = CamelCase(name)+strconv(ts)`, `Name = ts+"_"+snake`, filename `Name+".go"`.

- [ ] **Step 4: Run** → PASS.

- [ ] **Step 5: Commit** — `git commit -am "feat(codegen): migration file template and naming"`

---

## Task 8: Codegen — runner entrypoint & import path

**Files:**
- Modify: `internal/codegen/codegen.go`
- Test: `internal/codegen/runner_test.go`

**Interfaces:**
- Produces:
  ```go
  func ModuleImportPath(goModPath, migrationsDir string) (string, error) // module + cleaned dir
  func RenderRunner(migrationsImport string) (content string, err error) // main.go source
  ```

- [ ] **Step 1: Write failing tests**

```go
func TestModuleImportPath(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module github.com/me/app\n\ngo 1.22\n"), 0o644)
	got, err := ModuleImportPath(filepath.Join(dir, "go.mod"), "./migrations")
	if err != nil { t.Fatal(err) }
	if got != "github.com/me/app/migrations" { t.Fatalf("got %q", got) }
}

func TestRenderRunnerCompiles(t *testing.T) {
	src, err := RenderRunner("github.com/me/app/migrations")
	if err != nil { t.Fatal(err) }
	for _, want := range []string{
		`_ "github.com/me/app/migrations"`,
		`"github.com/jorgejr568/pigration/migrator"`,
		"pgxpool.New", "os.Getenv",
	} {
		if !strings.Contains(src, want) { t.Errorf("runner missing %q", want) }
	}
}
```

- [ ] **Step 2: Run to verify failure** → FAIL.

- [ ] **Step 3: Implement** — `ModuleImportPath`: read `go.mod`, parse the `module` line (`golang.org/x/mod/modfile` or a simple prefix scan), join with `path.Clean` of the migrations dir (strip leading `./`). `RenderRunner`: `text/template` producing a `package main` that blank-imports the migrations package, opens `pgxpool.New(ctx, os.Getenv("PIGRATION_DSN"))`, reads `PIGRATION_CMD` (`up`/`down`/`status`/`fresh`), `PIGRATION_TABLE`, `PIGRATION_STEPS`, `PIGRATION_BATCH`, `PIGRATION_ALLOW_FRESH`, dispatches to the matching `migrator.*` call with the right `RunOption`s, prints results, `os.Exit(1)` on error.

- [ ] **Step 4: Run** → PASS.

- [ ] **Step 5: Commit** — `git commit -am "feat(codegen): go-run runner entrypoint and import-path resolution"`

---

## Task 9: CLI — root, init, make

**Files:**
- Create: `internal/cli/root.go`, `internal/cli/init.go`, `internal/cli/make.go`, `cmd/pigration/main.go`
- Test: `internal/cli/cli_test.go`

**Interfaces:**
- Produces: `func Execute() error` (root); `newInitCmd`, `newMakeCmd`, `newRunCmd` returning `*cobra.Command`. Root persistent flag `--config` (default `./.db-migration.yaml`).

- [ ] **Step 1: Write failing tests**

```go
func TestInitScaffolds(t *testing.T) {
	dir := t.TempDir(); chdir(t, dir)
	if err := runCmd("init"); err != nil { t.Fatal(err) }
	if _, err := os.Stat(".db-migration.yaml"); err != nil { t.Fatal("config not created") }
	if _, err := os.Stat("migrations"); err != nil { t.Fatal("migrations dir not created") }
	gi, _ := os.ReadFile(".gitignore")
	if !strings.Contains(string(gi), "/.db-migration/") { t.Fatal(".gitignore not updated") }
	// idempotent: second run without --force does not error and does not overwrite
	if err := runCmd("init"); err != nil { t.Fatalf("re-init errored: %v", err) }
}

func TestMakeCreatesTimestampedFile(t *testing.T) {
	dir := t.TempDir(); chdir(t, dir)
	runCmd("init")
	if err := runCmd("make", "create users"); err != nil { t.Fatal(err) }
	matches, _ := filepath.Glob("migrations/*_create_users.go")
	if len(matches) != 1 { t.Fatalf("expected 1 migration file, got %v", matches) }
}
```

Helpers `runCmd(args...)` (builds root cmd, sets args, executes) and `chdir` (t.Chdir).

- [ ] **Step 2: Run to verify failure** → FAIL.

- [ ] **Step 3: Implement** — root cobra command wiring subcommands and `--config`. `init`: write default config template (URL form) unless present (or `--force`), `os.MkdirAll` the migrations dir, append `/.db-migration/` to `.gitignore` if not already there. `make`: load config, get `time.Now().Unix()`, call `codegen.RenderMigration(cfg.Migrations.Package, name, ts)`, write into `cfg.Migrations.Dir`. `main.go` calls `cli.Execute()`.

- [ ] **Step 4: Run** → PASS.

- [ ] **Step 5: Commit** — `git commit -am "feat(cli): root, init, make commands"`

---

## Task 10: CLI — migrate/rollback/status/fresh (go run)

**Files:**
- Create: `internal/cli/run.go`
- Test: `internal/cli/run_test.go`

**Interfaces:**
- Produces: `newRunCmd` builds `migrate`, `rollback`, `status`, `fresh` subcommands sharing `runViaGoRun(cfg, cmd string, env map[string]string) error`.

- [ ] **Step 1: Write failing test (unit — no DB)**

```go
func TestRunGeneratesEntrypoint(t *testing.T) {
	dir := t.TempDir(); chdir(t, dir)
	os.WriteFile("go.mod", []byte("module github.com/me/app\n\ngo 1.22\n"), 0o644)
	runCmd("init")
	// prepare() should write .db-migration/runner/main.go without executing go run
	path, err := prepareRunner(loadTestConfig(t))
	if err != nil { t.Fatal(err) }
	if _, err := os.Stat(path); err != nil { t.Fatal("runner main.go not generated") }
	src, _ := os.ReadFile(path)
	if !strings.Contains(string(src), "github.com/me/app/migrations") { t.Fatal("wrong import") }
}
```

Split the command into `prepareRunner(cfg) (mainPath string, err error)` (pure filesystem, testable) and `runViaGoRun` (executes `go run`). Test only `prepareRunner` here.

- [ ] **Step 2: Run to verify failure** → FAIL.

- [ ] **Step 3: Implement** — `prepareRunner`: resolve import path via `codegen.ModuleImportPath("go.mod", cfg.Migrations.Dir)`, `RenderRunner`, `os.MkdirAll(".db-migration/runner")`, write `main.go`. `runViaGoRun`: build env (`PIGRATION_DSN` from `cfg.DSN()`, `PIGRATION_TABLE`, `PIGRATION_CMD`, plus `--steps`/`--batch`/allow-fresh flags), `exec.Command("go", "run", "./.db-migration/runner")` with `Stdout/Stderr` wired through, run. Each subcommand sets `PIGRATION_CMD` and its flags. `fresh` additionally: refuse unless `PIGRATION_ALLOW_FRESH=1` env or `cfg.Fresh.Allow`; then prompt for the DB name (skip prompt with `--force`) before running.

- [ ] **Step 4: Run** — `go test ./internal/cli/ -run TestRun -v` → PASS.

- [ ] **Step 5: (Optional) manual e2e** — with `TEST_DATABASE_URL` and a scratch module, run `pigration migrate`/`status`/`rollback`/`fresh`. Document in commit body.

- [ ] **Step 6: Commit** — `git commit -am "feat(cli): migrate/rollback/status/fresh via go run"`

---

## Task 11: README + pig SVG branding

**Files:**
- Create: `README.md`, `docs/assets/pig.svg`

- [ ] **Step 1: Create `docs/assets/pig.svg`** — a simple, clean pig mascot as inline SVG (head + snout + ears), single accent color, viewBox `0 0 240 200`.

- [ ] **Step 2: Write `README.md`** — hero with the pig SVG and a `π`-gration wordmark (Greek π glyph leading "gration"); a one-paragraph pitch; Install (`go install github.com/jorgejr568/pigration/cmd/pigration@latest`); Quickstart walking `init` → `make` → `migrate` → `rollback` → `status`; the config format (both URL and discrete forms); a library-embedding snippet (`migrator.Up(ctx, pool)`); transactional vs `NonTransactional` note; and a `fresh` safety callout.

- [ ] **Step 3: Commit** — `git commit -am "docs: README with pigration branding and pig SVG"`

---

## Self-Review Notes

- Spec coverage: config (T1), registry/NonTransactional (T2), Up/tx/batch (T3), Down (T4), Status (T5), Fresh/guard (T6), templates (T7), runner/import path (T8), init/make (T9), migrate/rollback/status/fresh CLI (T10), README/SVG (T11), advisory lock (T3). All spec sections mapped.
- The pgx `Executor` superset is defined once (Task 2) and consumed by templates (T7) and migrations; never widened.
- Do not touch `querybuilder/` or `go.mod`.
