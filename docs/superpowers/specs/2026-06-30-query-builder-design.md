# Spec 2 — Full DDL Query Builder (`querybuilder`)

**Date:** 2026-06-30
**Module:** `github.com/jorgejr568/go-migration`
**Status:** Approved for implementation

## Overview

A fluent, Postgres-focused DDL query builder that migrations use as an ergonomic
alternative to raw SQL. Every builder produces a SQL string and can execute it against a
pgx executor. This package is a **standalone SQL-generation layer**: it depends only on
`pgx` (for the `Execer` interface and `pgconn.CommandTag`) and has **zero import
dependency** on the `migrator` package (Spec 1). It can be developed and tested entirely
independently.

## Goals

- Cover the large majority of Postgres DDL through a fluent API:
  create/drop/alter tables, columns with modifiers and constraints, indexes (including
  partial, composite, method, concurrent), enums/types, and schemas.
- Every builder exposes `ToSQL() (string, error)` (pure, DB-free, fully unit-testable)
  and `Execute(ctx, exec Execer) error`.
- A `Raw(sql, args...)` escape hatch so the builder is never a cage.
- Honest Postgres semantics: e.g. `WithUnsigned()` emits a `CHECK (col >= 0)` constraint
  because Postgres has no unsigned integer types.

## Non-Goals

- DML / query building (SELECT/INSERT/UPDATE/DELETE). DDL only.
- The migration engine, registry, batches, config, CLI (Spec 1).
- Non-Postgres dialects.

## Integration Seam (shared with Spec 1)

```go
// Execer — narrow interface accepted by every builder's Execute method.
// Satisfied by pgx.Tx, pgxpool.Pool, pgx.Conn.
type Execer interface {
    Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}
```

This is intentionally a subset of `migrator.Executor` (Spec 1). Because `migrator.Executor`
has `Exec` in its method set, a migration function whose `tx` parameter is typed
`migrator.Executor` can be passed directly to `Execute(ctx, tx)` here — with no import of
`migrator`. **Do not widen `Execer` beyond `Exec`.**

## API Surface

All builders live in package `querybuilder`. Constructor functions return a builder value;
chained methods return the builder for fluent use; terminal methods are `ToSQL` and
`Execute`.

```go
func (b <Builder>) ToSQL() (string, error)
func (b <Builder>) Execute(ctx context.Context, exec Execer) error
```

### Column types

Represented by a `ColumnType` value or constructor:

```
SmallInt, Int, BigInt,
Serial, BigSerial,
Real, Double, Numeric(precision, scale),
Text, Varchar(n), Char(n),
Bool,
Date, Time, Timestamp, Timestamptz,
UUID,
JSON, JSONB,
Bytea,
Inet
```

### Column modifiers (used with `.Column(name, type, ...mods)` and `.AddColumn`)

- `WithAutoIncrement()` — for integer PKs; equivalent to using a serial type. (When
  applied to `BigInt`, produce `bigserial`/identity semantics.)
- `WithUnsigned()` — Postgres has no unsigned ints → emit `CHECK (col >= 0)`.
- `NotNull()`
- `Default(value any)` — literal or expression; strings quoted, `Raw`-wrapped values
  emitted verbatim for expressions like `now()`.
- `Unique()`
- `PrimaryKey()`
- `References(table, column string, opts ...FKOption)` with `WithOnDelete(action)`,
  `WithOnUpdate(action)` where action ∈ {`Cascade`, `SetNull`, `Restrict`, `NoAction`,
  `SetDefault`}.
- `Check(expr string)`
- `Comment(text string)`
- `GeneratedAs(expr string)` — generated stored column.

### CreateTable

```go
querybuilder.CreateTable("users").
    IfNotExists().
    ID("id", querybuilder.BigInt, querybuilder.WithAutoIncrement()).
    Column("email", querybuilder.Text, querybuilder.NotNull(), querybuilder.Unique()).
    Column("age", querybuilder.Int, querybuilder.WithUnsigned()).
    Column("org_id", querybuilder.BigInt,
        querybuilder.References("orgs", "id", querybuilder.WithOnDelete(querybuilder.Cascade))).
    Timestamps().           // created_at + updated_at timestamptz NOT NULL DEFAULT now()
    Execute(ctx, tx)
```

Helpers:
- `ID(name, type, ...mods)` — convenience for a primary-key column.
- `UUIDPrimary(name)` — `uuid PRIMARY KEY DEFAULT gen_random_uuid()`.
- `Timestamps()` — `created_at`, `updated_at`.
- `Column(name, type, ...mods)`.
- Table-level: `PrimaryKeyColumns(cols...)`, `UniqueColumns(cols...)`, `CheckConstraint(name, expr)`.

### DropTable

```go
querybuilder.DropTable("users").IfExists().Cascade().Execute(ctx, tx)
```

### AlterTable

```go
querybuilder.AlterTable("users").
    AddColumn("phone", querybuilder.Varchar(32), querybuilder.NotNull()).
    DropColumn("legacy_field").
    RenameColumn("email", "email_address").
    AlterColumnType("age", querybuilder.BigInt).
    SetDefault("status", "'active'").
    DropDefault("status").
    SetNotNull("phone").
    DropNotNull("phone").
    AddConstraint("chk_age", "CHECK (age >= 0)").
    DropConstraint("chk_age").
    AddForeignKey("org_id", "orgs", "id", querybuilder.WithOnDelete(querybuilder.Cascade)).
    RenameTo("app_users").
    Execute(ctx, tx)
```

Each `Alter*` call adds a clause; `ToSQL` emits one or more `ALTER TABLE` statements (a
single statement with comma-separated actions where Postgres allows, separate statements
where required, e.g. `RENAME`).

### Indexes

```go
querybuilder.CreateIndex("idx_users_email").
    On("users").Columns("email").Unique().Using("btree").
    Where("deleted_at IS NULL").         // partial
    Concurrently().                      // pairs with migrator.NonTransactional()
    Execute(ctx, tx)

querybuilder.DropIndex("idx_users_email").IfExists().Concurrently().Execute(ctx, tx)
```

Support composite indexes (`Columns("a","b")`) and expression columns via `Raw`.

### Enums / types

```go
querybuilder.CreateType("user_role").AsEnum("admin", "member", "guest").Execute(ctx, tx)
querybuilder.AlterTypeAddValue("user_role", "owner").Execute(ctx, tx)   // BeforeValue/AfterValue optional
querybuilder.DropType("user_role").IfExists().Cascade().Execute(ctx, tx)
```

### Schemas

```go
querybuilder.CreateSchema("billing").IfNotExists().Execute(ctx, tx)
querybuilder.DropSchema("billing").IfExists().Cascade().Execute(ctx, tx)
```

### Raw escape hatch

```go
querybuilder.Raw("CREATE EXTENSION IF NOT EXISTS pgcrypto;").Execute(ctx, tx)
```

Also usable as a column default / expression wrapper: `querybuilder.Default(querybuilder.Raw("now()"))`.

## Identifier Handling & Safety

- Quote identifiers (table/column/index/type/schema names) with double quotes and escape
  embedded quotes, so reserved words and mixed case work.
- DDL cannot be parameterized in Postgres; literal `Default` values that are strings are
  single-quoted and escaped; expressions must be passed via `Raw`/`GeneratedAs`/`Check`
  and are emitted verbatim (documented as trusted input — DDL, not user data).

## Error Handling

- `ToSQL` returns an error for structurally invalid builders (e.g. `CreateTable` with no
  columns, `CreateIndex` with no table or no columns, `Varchar(0)`, unknown FK action).
- `Execute` calls `ToSQL`, returns its error if any, then runs each generated statement
  via `exec.Exec`; a statement error is returned wrapped with the offending SQL.

## Testing

- **Pure `ToSQL` unit tests** (no DB) for every builder and every modifier/option
  combination — this is the bulk of the suite and gives near-total coverage cheaply.
  Table-driven: builder chain → expected SQL string.
- **Identifier quoting/escaping** edge cases (reserved words, mixed case, quotes).
- **`WithUnsigned` → CHECK**, FK actions, partial/concurrent indexes, enum add-value
  ordering, multi-action `AlterTable` emission.
- **Optional integration smoke tests** against a real Postgres (guarded by
  `TEST_DATABASE_URL`) that execute a representative subset to confirm the generated SQL
  is actually valid Postgres.

## Open Implementation Choices (implementer's discretion)

- Internal representation (per-builder structs vs a shared clause list).
- Whether `WithAutoIncrement()` maps to `serial`/`bigserial` vs `GENERATED ... AS IDENTITY`
  (pick one, document it; identity is the modern Postgres recommendation).
- Exact formatting/whitespace of generated SQL (tests assert normalized output).
