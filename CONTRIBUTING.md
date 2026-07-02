# Contributing to pigration

Thanks for your interest! This document covers everything you need to hack on
pigration locally and get a PR merged.

## Prerequisites

- **Go** — the version pinned in [`go.mod`](go.mod)
- **Docker** (or any local Postgres) — only needed for the integration tests

## Repository layout

| Path | What lives there |
|---|---|
| `migrator/` | The migration engine: registry, transactional runner, batches, advisory lock, public API (`Up`/`Down`/`Status`/`Fresh`) |
| `querybuilder/` | Fluent Postgres DDL builder (standalone — never imports `migrator`) |
| `internal/config/` | `.db-migration.yaml` loading, `${VAR:-default}` interpolation, DSN assembly |
| `internal/codegen/` | Migration-file template, generated `go run` entrypoint, the `PIGRATION_*` env protocol |
| `internal/cli/` | Cobra commands (`init`, `make`, `migrate`, `rollback`, `status`, `fresh`) |
| `cmd/pigration/` | CLI entrypoint |
| `docs/superpowers/` | Design specs and implementation plans (historical record) |

One architectural invariant to preserve: `querybuilder.Execer` must remain a
strict subset of `migrator.Executor` (exactly `Exec`), and the two packages must
never import each other. That structural-interface seam is what lets a migration
pass its `tx` straight into `querybuilder...Execute(ctx, tx)`.

## Running the tests

Unit tests need nothing:

```sh
make test          # or: go test ./...
```

Integration tests are guarded by `TEST_DATABASE_URL` and **skip** when it is
unset. To run everything against a throwaway Postgres:

```sh
make db-up         # starts a postgres:16 container on port 5433
make test-all      # full suite with -race, serialized packages
make db-down       # tears the container down
```

> **Why `-p 1`?** The migrator and CLI e2e tests `DROP SCHEMA public CASCADE`
> on the test database for isolation. Package test binaries must therefore not
> run concurrently against the same database — `make test-all` and CI both pass
> `-p 1`.

Never point `TEST_DATABASE_URL` at a database you care about.

## Before you open a PR

1. `make lint` — `gofmt -l` must be empty and `go vet` clean.
2. `make test-all` — the full suite must pass. CI runs it against Postgres 14
   and 16 with `-race` and enforces a **95% coverage floor**, so add tests with
   your change. Tests assert real behavior (exact SQL, sentinel errors, rows in
   the database) — coverage-padding tests will be rejected in review.
3. Keep commits in the conventional style used by the history:
   `feat(migrator): ...`, `fix(cli): ...`, `test: ...`, `docs: ...`,
   `refactor: ...`.

## Design conventions

- **TDD** — every change lands with the test that proves it (failing first).
- **Direct over clever** — no speculative abstraction; zero-value sentinels over
  pointer-presence fields; one canonical helper over near-duplicates.
- **Fail loud** — explicit user input that can't be honored is an error, never a
  silent fallback (e.g. `rollback --batch 0` refuses rather than guessing).
- **Raw SQL is always available** — the query builder is a convenience layer,
  never a cage. Builders expose `ToSQL()` so SQL generation stays pure and
  testable without a database.
- New Postgres DDL support goes in `querybuilder` with pure `ToSQL` tests plus
  a line in the integration round-trip test.

## Reporting bugs / proposing features

Open a GitHub issue with a minimal reproduction (for the engine: the migration
code + expected vs. actual tracking-table state; for the builder: the builder
chain + expected vs. actual SQL). Feature proposals are welcome — pigration
deliberately stays Postgres-only, so cross-database portability requests are
out of scope.
