# Spec 1 — Migration Engine + CLI + Config + Library API

**Date:** 2026-06-30
**Module:** `github.com/jorgejr568/pigration`
**Status:** Approved for implementation

## Overview

A database migration system for Go + Postgres where migrations are **Go code** that
self-register via `init()`. Delivers a Cobra CLI (`pigration`) for local/dev use and a
first-class public library API (`migrator.Up/Down/Status`) usable directly from a
production binary or app boot — the CLI commands are thin wrappers over the same
library code path.

This spec covers the engine, CLI, config, and library API. The DDL query builder is a
separate spec (Spec 2) that plugs in via a shared structural interface and has **zero
import dependency** on this spec.

## Goals

- Migrations are compiled Go, registered via `init()`, receiving an explicit executor.
- Per-migration Postgres transactions with rollback on failure; opt-out for operations
  that cannot run in a transaction.
- Laravel-style **batch** tracking: `migrate` groups applied migrations into a batch;
  `rollback` reverts the most recent batch (or N steps, or a specific batch).
- Config via `.db-migration.yaml` supporting either a full DB URL or discrete params,
  each value sourced from an env var (with optional default) or a literal.
- CLI that "starts the repo" (`init`), scaffolds migrations (`make`), and runs
  `migrate` / `rollback` / `status`.
- Same engine runs in dev (CLI) and prod (library).

## Non-Goals

- The DDL query builder (Spec 2). Migrations in this spec use raw SQL via `tx.Exec`.
- Databases other than Postgres.
- A GUI or web dashboard.

## Architecture

### Driver & connections

- `pgx` v5. Connections via `*pgxpool.Pool`; transactions via `pgx.Tx`.

### Integration seam (shared with Spec 2)

Both packages depend only on `pgx`, never on each other. Go interfaces are structural:

```go
// migrator.Executor — the type of the `tx` parameter passed to migration functions.
// Superset interface. Satisfied by pgx.Tx (and pgxpool.Pool / pgx.Conn).
type Executor interface {
    Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
    Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
    QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}
```

Spec 2's `querybuilder.Execer` is a narrow subset requiring only `Exec`. Because a
value of interface type `migrator.Executor` has a method set that is a superset of
`querybuilder.Execer`, a migration can call `qb...Execute(ctx, tx)` where
`tx migrator.Executor` and it compiles with no cross-package import. **This contract is
fixed and must not change without updating both specs.**

### Packages

- `migrator` — registry, runner, transaction handling, tracking table, public API,
  config loading, `Executor` interface, `MigrationFunc`, `RegisterOption`, `RunOption`.
- `cmd/pigration` + `internal/cli` — Cobra commands.
- `internal/config` — `.db-migration.yaml` loading + `${}` interpolation + DSN assembly.
- `internal/codegen` — templates for `init`, `make`, and the `go run` runner entrypoint.

## Configuration

### File: `.db-migration.yaml`

Default template written by `init`:

```yaml
database:
  url: ${DATABASE_URL}          # ${VAR} reads env; ${VAR:-default} gives a fallback
migrations:
  dir: ./migrations
  package: migrations           # package name used in generated files
  table: schema_migrations      # tracking table name
```

Discrete-params alternative (any value may be `${ENV}`, `${ENV:-default}`, or a literal):

```yaml
database:
  host: ${DB_HOST:-localhost}
  port: 5432
  user: ${DB_USER}
  password: ${DB_PASSWORD}
  name: myapp
  sslmode: disable
migrations:
  dir: ./migrations
  package: migrations
  table: schema_migrations
fresh:
  allow: false                  # must be true (or PIGRATION_ALLOW_FRESH=1) for `fresh`
```

### Loading rules

- Parse with `yaml.v3` into a struct.
- **Interpolation pass** on every string value:
  - `${VAR}` → value of env var `VAR` (empty string if unset).
  - `${VAR:-default}` → env var `VAR`, or `default` if unset/empty.
  - Any other string → used literally.
- **DSN precedence:** if `database.url` is non-empty after interpolation, use it. Else
  assemble a DSN from the discrete params (`host`, `port`, `user`, `password`, `name`,
  `sslmode`). If both `url` and discrete params are present, use `url` and log a warning.
- `sslmode` defaults to `disable` if omitted in discrete mode.
- Config file path defaults to `./.db-migration.yaml`; override with `--config`.

## Tracking Table

```sql
CREATE TABLE schema_migrations (
    id         bigserial PRIMARY KEY,
    name       text        NOT NULL UNIQUE,   -- "<unix_ts>_<snake_name>"
    batch      int         NOT NULL,
    applied_at timestamptz NOT NULL DEFAULT now()
);
```

Table name comes from config (`migrations.table`). Created automatically on first run if
absent (`CREATE TABLE IF NOT EXISTS`).

## Registry & Migration Identity

- Registered name format: `"<unixTimestamp>_<snake_name>"`, e.g. `1719800000_create_users`.
  This sorts chronologically as a plain string.
- Package-level ordered registry inside `migrator`. `Register` appends an entry.
- The runner lexically sorts registered names to determine order.
- Duplicate names → error at run start (fail fast).

### API

```go
type MigrationFunc func(ctx context.Context, tx Executor) error

type RegisterOption func(*migration)

// NonTransactional marks a migration to run WITHOUT a wrapping transaction
// (for CREATE INDEX CONCURRENTLY, ALTER TYPE ... ADD VALUE, VACUUM, etc.).
func NonTransactional() RegisterOption

func Register(name string, up, down MigrationFunc, opts ...RegisterOption)
```

## Runner

### Concurrency safety

Wrap each run in a session-level `pg_advisory_lock` (a fixed app-specific key) so two
concurrent runners cannot race; release on completion. Use `pg_try_advisory_lock` and
fail with a clear "another migration is in progress" error if not acquired.

### Up (`migrator.Up`)

1. Acquire advisory lock.
2. `CREATE TABLE IF NOT EXISTS` the tracking table.
3. Load applied names into a set.
4. `pending = sort(registeredNames − applied)`.
5. `batch = SELECT COALESCE(MAX(batch),0)+1` — one batch number for this invocation.
6. For each pending migration (up to `--steps` if provided), in order:
   - If transactional: `BEGIN`; run `up(ctx, tx)`; `INSERT` tracking row; `COMMIT`.
     On error: `ROLLBACK`, stop, return error identifying the migration.
   - If `NonTransactional`: run `up(ctx, pool)` directly; on success `INSERT` tracking
     row (separately); on error stop and return.
7. Release lock. Return `Result` (applied names, batch).

### Down / rollback (`migrator.Down`)

Target selection via `RunOption`:
- **default:** the highest batch — all its migrations.
- `Steps(n)`: the last `n` applied migrations (across batches), most recent first.
- `Batch(k)`: all migrations in batch `k`.

Execute each target's `down` in **reverse applied order**, each in its own transaction
(unless `NonTransactional`), deleting its tracking row on success. Stop on first error.

### Status (`migrator.Status`)

Returns, for every registered migration and every applied-but-unregistered row: name,
batch (nil if pending), applied_at (nil if pending), and pending flag. Ordered by name.

## Public Library API

```go
func Up(ctx context.Context, pool *pgxpool.Pool, opts ...RunOption) (Result, error)
func Down(ctx context.Context, pool *pgxpool.Pool, opts ...RunOption) (Result, error)
func Status(ctx context.Context, pool *pgxpool.Pool) ([]MigrationStatus, error)

// Fresh drops the public schema (CASCADE), recreates it, then applies all migrations.
// It returns ErrFreshNotAllowed unless the caller opts in (see Fresh section). The CLI
// gates this on PIGRATION_ALLOW_FRESH=1; the library requires AllowFresh() explicitly.
func Fresh(ctx context.Context, pool *pgxpool.Pool, opts ...RunOption) (Result, error)
func AllowFresh() RunOption

type RunOption func(*runConfig)
func Steps(n int) RunOption
func Batch(k int) RunOption

type Result struct {
    Applied    []string // or RolledBack for Down
    Batch      int
}

type MigrationStatus struct {
    Name      string
    Batch     *int
    AppliedAt *time.Time
    Pending   bool
}
```

Table name and DSN are supplied by the caller when embedding directly; the CLI supplies
them from config. (Provide a `migrator.Options{Table string}` or a package-level setter —
implementer's choice, but the table name must be configurable and default to
`schema_migrations`.)

## CLI (`pigration`, Cobra)

Global flag: `--config` (default `./.db-migration.yaml`).

| Command | Needs user code? | Behavior |
|---|---|---|
| `pigration init` | no | Write `.db-migration.yaml` (default URL template) if absent; create `migrations/` dir; add `.db-migration/` to `.gitignore`. Idempotent; never overwrite an existing config without `--force`. |
| `pigration make <name>` | no | Render a timestamped migration file into `migrations/<ts>_<snake_name>.go` using the configured package name. |
| `pigration migrate [--steps N]` | yes | Generate runner entrypoint, `go run` it calling `migrator.Up`. |
| `pigration rollback [--steps N \| --batch K]` | yes | Same entrypoint calling `migrator.Down`. |
| `pigration status` | yes | Same entrypoint calling `migrator.Status`; print a table. |
| `pigration fresh [--force]` | yes | **Destructive.** Drop the `public` schema (CASCADE), recreate it, then apply all migrations from scratch. Guarded — see below. |

### Running user Go code (`migrate`/`rollback`/`status`)

1. Read `go.mod` module path; compute the migrations import path as
   `<module>/<migrations.dir cleaned>` (e.g. module `github.com/me/app` + `./migrations`
   → `github.com/me/app/migrations`).
2. Generate `./.db-migration/runner/main.go` that:
   - blank-imports the migrations package (`_ "<import path>"`) to trigger `init()`
     registrations,
   - imports `migrator`,
   - reads DSN + table name + command + flags from environment,
   - opens a `pgxpool.Pool` and calls the requested `migrator` function,
   - prints results / errors and exits non-zero on failure.
3. `go run ./.db-migration/runner` with config passed via environment variables.
4. The `./.db-migration/` dir is regenerated each run and is gitignored.

### `fresh` (destructive reset)

Behavior: `DROP SCHEMA public CASCADE; CREATE SCHEMA public;` then run `migrator.Up` for
all migrations. This wipes **everything** in the public schema (tables, types, indexes,
the tracking table itself), so it fully resets the database.

**Double safety guard — both required to proceed:**

1. **Hard prod refusal:** `fresh` refuses to run unless the environment variable
   `PIGRATION_ALLOW_FRESH=1` is set (or `fresh.allow: true` in `.db-migration.yaml`).
   Without it, the command exits non-zero with a message explaining the guard. This makes
   accidental production wipes impossible without a deliberate opt-in. In the library, the
   equivalent gate is the `migrator.AllowFresh()` run option; `Fresh` returns
   `ErrFreshNotAllowed` otherwise.
2. **Interactive confirmation:** even with the opt-in set, the CLI prompts
   `This will DROP ALL data in schema "public". Type the database name to continue:` and
   requires the user to type the target database name. `--force` skips only this prompt
   (the env/config opt-in is still mandatory).

`fresh` uses the same `go run` runner entrypoint as `migrate`, calling `migrator.Fresh`.

### Generated migration template (`make`)

```go
package migrations

import (
    "context"

    "github.com/jorgejr568/pigration/migrator"
)

func CreateUsers1719800000Up(ctx context.Context, tx migrator.Executor) error {
    _, err := tx.Exec(ctx, `CREATE TABLE users (
        id         bigserial PRIMARY KEY,
        email      text NOT NULL UNIQUE,
        created_at timestamptz NOT NULL DEFAULT now()
    );`)
    return err
}

func CreateUsers1719800000Down(ctx context.Context, tx migrator.Executor) error {
    _, err := tx.Exec(ctx, `DROP TABLE users;`)
    return err
}

func init() {
    migrator.Register("1719800000_create_users",
        CreateUsers1719800000Up, CreateUsers1719800000Down)
}
```

Function names are CamelCase `<Name><Timestamp>Up/Down`; the registered identity is the
timestamp-prefixed snake name. For a non-transactional migration the template shows the
`migrator.NonTransactional()` option in a comment.

## Branding & README

The project is **pigration** (π-gration — a pun on **pig** + migration and **π** + gration).
Deliver a `README.md` featuring:

- A hero with a **pig SVG** logo and the wordmark styled as **π**-gration (the Greek π
  glyph leading "gration").
- Quickstart: `go install .../cmd/pigration`, `pigration init`, `pigration make`,
  `pigration migrate`, `pigration rollback`, `pigration status`, `pigration fresh`.
- The config format (URL and discrete forms), the library-embedding example
  (`migrator.Up(ctx, pool)`), and the transactional / `NonTransactional` semantics.
- A short callout on `fresh` and its safety guards.

The SVG should live in the repo (e.g. `docs/assets/pig.svg`) and render in the README.

## Error Handling

- Config: missing file → clear message telling the user to run `pigration init`. Missing DSN
  after interpolation → explicit error naming the empty field/env var.
- Runner: any migration error is wrapped with the migration name and whether the tx was
  rolled back. Advisory-lock contention → "another migration is in progress".
- `go run` failures (compile errors in user migrations) surface the compiler output
  verbatim.
- Duplicate registered name, or an applied name that is no longer registered during
  rollback → explicit errors.

## Testing

- **Config:** table-driven tests for interpolation (`${VAR}`, `${VAR:-default}`,
  literals), URL-vs-discrete precedence, DSN assembly, defaults.
- **Registry:** registration ordering, duplicate detection, sorting.
- **Runner (integration, against a real Postgres):** first-run table bootstrap; apply
  pending; batch numbering; failure rolls back a transactional migration and stops;
  `NonTransactional` runs without a wrapping tx; rollback of last batch; `--steps`;
  `--batch`; status output. Use a disposable Postgres (testcontainers or a
  `TEST_DATABASE_URL` guard that skips when unset).
- **CLI:** `init` idempotency and `--force`; `make` filename + content; import-path
  computation from `go.mod` + dir; runner-entrypoint generation renders and compiles.

## Open Implementation Choices (implementer's discretion)

- Exact advisory-lock key constant.
- Whether table name is passed via `migrator.Options` or a package setter.
- testcontainers vs `TEST_DATABASE_URL` for integration tests.
