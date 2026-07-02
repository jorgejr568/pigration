<div align="center">

<img src="docs/assets/pig.svg" alt="pigration pig mascot" width="180" />

# **π**gration

**Go + Postgres database migrations as self-registering Go code.**

[![CI](https://github.com/jorgejr568/pigration/actions/workflows/ci.yml/badge.svg)](https://github.com/jorgejr568/pigration/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/jorgejr568/pigration.svg)](https://pkg.go.dev/github.com/jorgejr568/pigration)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

</div>

---

`pigration` (π-gration — **pig** + migration, and **π** + gration) is a migration
engine for Go and Postgres where migrations are ordinary **Go code** that
self-register via `init()`. It ships both a Cobra CLI (`pigration`) for dev use and
a first-class library API (`migrator.Up/Down/Status/Fresh`) you can call straight
from your production binary or app boot. The CLI commands are thin wrappers over the
exact same library code path.

- **Migrations are compiled Go**, registered via `init()`, receiving an explicit
  `Executor` — run raw SQL with `tx.Exec`, or plug in a query builder.
- **Per-migration transactions** with automatic rollback on failure, and an opt-out
  for operations that cannot run in a transaction.
- **Laravel-style batches:** `migrate` groups applied migrations into a batch;
  `rollback` reverts the most recent batch (or `--steps N`, or `--batch K`).
- **One engine, dev and prod.** The CLI shells out to `go run` so your migration code
  is compiled in; the library runs the identical logic in-process.

## Install

```sh
go install github.com/jorgejr568/pigration/cmd/pigration@latest
```

## Quickstart

```sh
# 1. Scaffold config, the migrations dir, and .gitignore
pigration init

# 2. Create a timestamped migration file
pigration make create users
#   → migrations/1719800000_create_users.go

# 3. Edit the generated Up/Down bodies, then apply
pigration migrate                 # apply all pending (one new batch)
pigration migrate --steps 1       # apply just the next one

# 4. Inspect and revert
pigration status                  # applied vs pending
pigration rollback                # revert the most recent batch
pigration rollback --steps 2      # revert the last 2 migrations
pigration rollback --batch 3      # revert everything in batch 3

# 5. Reset from scratch (destructive — guarded, see below)
PIGRATION_ALLOW_FRESH=1 pigration fresh
```

## Configuration

`pigration init` writes `.db-migration.yaml`. Every string value may be a literal,
`${VAR}` (env var, empty if unset), or `${VAR:-default}` (env var with a fallback).

**URL form** (default):

```yaml
database:
  url: ${DATABASE_URL}
migrations:
  dir: ./migrations
  package: migrations           # package name used in generated files
  table: schema_migrations      # tracking table name
```

**Discrete-params form** (any value may be `${ENV}`, `${ENV:-default}`, or literal):

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

If `database.url` is set it wins; otherwise a DSN is assembled from the discrete
params (`sslmode` defaults to `disable`). Override the config path with `--config`.

## A generated migration

```go
package migrations

import (
	"context"

	"github.com/jorgejr568/pigration/migrator"
)

func CreateUsers1719800000Up(ctx context.Context, tx migrator.Executor) error {
	_, err := tx.Exec(ctx, `CREATE TABLE users (
		id         bigserial   PRIMARY KEY,
		email      text        NOT NULL UNIQUE,
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

The registered identity is `"<unixTimestamp>_<snake_name>"`, which sorts
chronologically as a plain string and determines run order.

## Transactional vs. non-transactional

By default each migration runs inside its own transaction and is rolled back if it
returns an error. For statements Postgres cannot run inside a transaction — e.g.
`CREATE INDEX CONCURRENTLY`, `ALTER TYPE ... ADD VALUE`, `VACUUM` — register with
`migrator.NonTransactional()`:

```go
func init() {
	migrator.Register("1719800100_idx_users_email",
		AddIndex1719800100Up, AddIndex1719800100Down,
		migrator.NonTransactional())
}
```

A non-transactional migration runs directly against the pool; its tracking row is
recorded separately after success (it is **not** rolled back on failure).

## Using it as a library

The same engine embeds directly in your app — no CLI, no `go run`:

```go
import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jorgejr568/pigration/migrator"

	_ "yourapp/migrations" // blank import triggers init() registrations
)

func migrate(ctx context.Context, pool *pgxpool.Pool) error {
	res, err := migrator.Up(ctx, pool)
	if err != nil {
		return err
	}
	log.Printf("applied %d migrations in batch %d", len(res.Applied), res.Batch)
	return nil
}
```

`Up`, `Down`, `Fresh`, and `Status` all take `...migrator.RunOption`
(`Steps`, `Batch`, `AllowFresh`, `Table` for a custom table name). Runs are
guarded by a Postgres advisory lock so two runners cannot race.

## The query builder

The `querybuilder` package is a fluent Postgres DDL builder you can use inside
migrations instead of raw SQL. Because its one-method `Execer` interface is a
subset of `migrator.Executor`, the migration's `tx` passes straight in:

```go
import "github.com/jorgejr568/pigration/querybuilder"

func CreateUsers1719800000Up(ctx context.Context, tx migrator.Executor) error {
	return querybuilder.CreateTable("users").
		ID("id", querybuilder.BigSerial).
		Column("email", querybuilder.Text, querybuilder.NotNull(), querybuilder.Unique()).
		Column("age", querybuilder.Int, querybuilder.WithUnsigned()). // CHECK (age >= 0)
		Column("org_id", querybuilder.BigInt,
			querybuilder.References("orgs", "id", querybuilder.WithOnDelete(querybuilder.Cascade))).
		Timestamps(). // created_at + updated_at
		Execute(ctx, tx)
}
```

It covers tables (`CreateTable`, `AlterTable`, `DropTable`), indexes — including
unique, partial, composite, and `Concurrently()` (pair that one with
`migrator.NonTransactional()`) — enum types, schemas, and column modifiers
(defaults, checks, generated columns, foreign keys with referential actions).
Every builder exposes `ToSQL()` so SQL generation is pure and testable, and
`querybuilder.Raw(sql, args...)` executes anything the builders don't cover:
the builder is a convenience, never a cage. See the
[package docs](https://pkg.go.dev/github.com/jorgejr568/pigration/querybuilder)
for runnable examples.

## `fresh` — destructive reset (double-guarded)

`fresh` runs `DROP SCHEMA public CASCADE; CREATE SCHEMA public;` and then re-applies
every migration from scratch. This wipes **everything** in the public schema. Two
independent guards must both pass:

1. **Hard opt-in.** `fresh` refuses unless `PIGRATION_ALLOW_FRESH=1` (or
   `fresh.allow: true` in config). In the library the equivalent is the
   `migrator.AllowFresh()` option; `migrator.Fresh` returns `ErrFreshNotAllowed`
   otherwise.
2. **Interactive confirmation.** Even with the opt-in, the CLI prompts you to type
   the target database name before proceeding. `--force` skips only this prompt (the
   opt-in from guard #1 is still mandatory).

## Contributing

Contributions are welcome — see [CONTRIBUTING.md](CONTRIBUTING.md) for the
repository layout, how to run the test suite against a throwaway Postgres
(`make db-up && make test-all`), and the conventions CI enforces (gofmt, vet,
`-race`, and a 95% coverage floor against Postgres 14 and 16).

## License

[MIT](LICENSE)
