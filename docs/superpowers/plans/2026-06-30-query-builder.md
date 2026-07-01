# DDL Query Builder (querybuilder) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `querybuilder`, a fluent Postgres DDL builder producing SQL that executes against a pgx executor — a standalone package migrations use instead of raw SQL.

**Architecture:** Each builder (CreateTable, DropTable, AlterTable, indexes, enums, schemas, Raw) is a struct with fluent methods returning itself, a pure `ToSQL() (string, error)`, and `Execute(ctx, Execer) error`. Pure SQL generation → the vast majority of tests are DB-free string assertions. Identifier quoting is centralized.

**Tech Stack:** Go 1.22+, `github.com/jackc/pgx/v5` (only for the `Execer` interface + `pgconn.CommandTag`).

## Global Constraints

- Module path: `github.com/jorgejr568/pigration` (verbatim); package `querybuilder`.
- Postgres dialect only.
- `go.mod` and dependencies are pre-created by the orchestrator. **Do not run `go mod tidy` or `go get`.** Build/test only this package: `go test ./querybuilder/...`.
- **Zero dependency on the `migrator` package** — do not import it. The only shared contract is the `Execer` interface below, which must remain a subset of `migrator.Executor` (i.e. exactly `Exec`, do not widen).
- Every builder exposes `ToSQL() (string, error)` and `Execute(ctx context.Context, exec Execer) error`.
- Quote all identifiers with double quotes; escape embedded double quotes by doubling them.
- Do NOT touch files outside `querybuilder/`.

## Shared Interface Seam (fixed — copy verbatim)

```go
// querybuilder/execer.go
package querybuilder

import (
	"context"

	"github.com/jackc/pgx/v5/pgconn"
)

// Execer is the narrow executor every builder's Execute accepts. It is a subset of
// migrator.Executor, so a migration's tx parameter satisfies it directly.
// Satisfied by pgx.Tx, *pgxpool.Pool, *pgx.Conn.
type Execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}
```

---

## File Structure

- `querybuilder/execer.go` — `Execer` (above).
- `querybuilder/ident.go` — `quoteIdent`, `quoteQualified`, `quoteLiteral`, `execStatements` helper.
- `querybuilder/raw.go` — `Raw` (statement + expression wrapper).
- `querybuilder/types.go` — `ColumnType` and constructors.
- `querybuilder/column.go` — `columnDef`, `ColumnModifier`, all modifiers, `FKOption`, referential actions.
- `querybuilder/create_table.go` — `CreateTable` + helpers.
- `querybuilder/drop_table.go` — `DropTable`.
- `querybuilder/alter_table.go` — `AlterTable` + actions.
- `querybuilder/index.go` — `CreateIndex`, `DropIndex`.
- `querybuilder/type_enum.go` — `CreateType`, `AlterTypeAddValue`, `DropType`.
- `querybuilder/schema.go` — `CreateSchema`, `DropSchema`.
- Tests colocated as `*_test.go`.

Testing convention: `ToSQL` output is compared after **normalizing whitespace** (collapse runs of spaces/newlines to a single space, trim). Provide `func norm(s string) string` in a test helper so assertions are robust to formatting.

---

## Task 1: Identifiers, Execer, Raw, exec helper

**Files:**
- Create: `querybuilder/execer.go` (verbatim above), `querybuilder/ident.go`, `querybuilder/raw.go`
- Test: `querybuilder/ident_test.go`, `querybuilder/raw_test.go`

**Interfaces:**
- Produces:
  ```go
  func quoteIdent(name string) string            // `email` -> "email"; `a"b` -> "a""b"
  func quoteQualified(name string) string        // `billing.users` -> "billing"."users"
  func quoteLiteral(s string) string             // 'O''Brien'
  func execStatements(ctx context.Context, exec Execer, stmts []string) error
  type Expr interface{ expr() string }           // marker for raw expressions
  func Raw(sql string, args ...any) RawStmt       // statement-level (Execute runs it)
  func (RawStmt) ToSQL() (string, error)
  func (RawStmt) Execute(ctx context.Context, exec Execer) error
  func RawExpr(sql string) Expr                   // expression wrapper for Default/etc.
  ```

- [ ] **Step 1: Write failing tests**

```go
func TestQuoteIdent(t *testing.T) {
	if quoteIdent("email") != `"email"` { t.Fatal("basic") }
	if quoteIdent(`we"ird`) != `"we""ird"` { t.Fatal("escape") }
	if quoteQualified("billing.users") != `"billing"."users"` { t.Fatal("qualified") }
}
func TestQuoteLiteral(t *testing.T) {
	if quoteLiteral("O'Brien") != `'O''Brien'` { t.Fatal("literal escape") }
}
func TestRawToSQL(t *testing.T) {
	s, err := Raw("CREATE EXTENSION pgcrypto;").ToSQL()
	if err != nil || s != "CREATE EXTENSION pgcrypto;" { t.Fatalf("got %q %v", s, err) }
}
```

- [ ] **Step 2: Run to verify failure** — `go test ./querybuilder/ -run 'TestQuote|TestRaw' -v` → FAIL.

- [ ] **Step 3: Implement** — `quoteIdent`: wrap in `"`, replace `"`→`""`. `quoteQualified`: split on `.`, quote each part. `quoteLiteral`: wrap in `'`, replace `'`→`''`. `execStatements`: loop, `exec.Exec(ctx, s)`, wrap error with the SQL. `RawStmt{sql string; args []any}` with `ToSQL` returning sql and `Execute` calling `exec.Exec(ctx, sql, args...)`. `RawExpr` returns an `Expr` whose `expr()` returns the sql verbatim.

- [ ] **Step 4: Run** → PASS.

- [ ] **Step 5: Commit** — `git commit -m "feat(qb): identifiers, Execer, Raw, exec helper"`

---

## Task 2: Column types

**Files:**
- Create: `querybuilder/types.go`
- Test: `querybuilder/types_test.go`

**Interfaces:**
- Produces:
  ```go
  type ColumnType struct{ sql string }
  func (t ColumnType) String() string
  // value types:
  var SmallInt, Int, BigInt, Serial, BigSerial, Real, Double, Text, Bool,
      Date, Time, Timestamp, Timestamptz, UUID, JSON, JSONB, Bytea, Inet ColumnType
  // parameterized:
  func Varchar(n int) ColumnType
  func Char(n int) ColumnType
  func Numeric(precision, scale int) ColumnType
  ```

- [ ] **Step 1: Write failing tests**

```go
func TestColumnTypes(t *testing.T) {
	cases := map[string]ColumnType{
		"bigint": BigInt, "text": Text, "boolean": Bool, "timestamptz": Timestamptz,
		"uuid": UUID, "jsonb": JSONB, "varchar(255)": Varchar(255), "numeric(10,2)": Numeric(10, 2),
	}
	for want, ct := range cases {
		if ct.String() != want { t.Errorf("got %q want %q", ct.String(), want) }
	}
}
```

- [ ] **Step 2: Run to verify failure** → FAIL.

- [ ] **Step 3: Implement** — declare the value `ColumnType`s with their SQL (`Bool` → `boolean`, `Double` → `double precision`, `Timestamptz` → `timestamptz`, etc.); `Varchar`/`Char`/`Numeric` format with args (`Varchar(0)` is allowed here but flagged invalid at builder validation, not construction).

- [ ] **Step 4: Run** → PASS.

- [ ] **Step 5: Commit** — `git commit -m "feat(qb): column types"`

---

## Task 3: Columns & modifiers

**Files:**
- Create: `querybuilder/column.go`
- Test: `querybuilder/column_test.go`

**Interfaces:**
- Produces:
  ```go
  type columnDef struct {
      name string; typ ColumnType
      notNull, unique, primaryKey, autoIncrement, unsigned bool
      def *string; check *string; comment *string; generated *string
      fk *fkRef
  }
  type ColumnModifier func(*columnDef)
  func NotNull() ColumnModifier
  func Unique() ColumnModifier
  func PrimaryKey() ColumnModifier
  func WithAutoIncrement() ColumnModifier
  func WithUnsigned() ColumnModifier
  func Default(v any) ColumnModifier          // string->quoted literal; Expr->verbatim; number/bool->verbatim
  func Check(expr string) ColumnModifier
  func Comment(text string) ColumnModifier
  func GeneratedAs(expr string) ColumnModifier
  type Action string
  const (Cascade Action="CASCADE"; SetNull Action="SET NULL"; Restrict Action="RESTRICT";
         NoAction Action="NO ACTION"; SetDefault Action="SET DEFAULT")
  type FKOption func(*fkRef)
  func WithOnDelete(a Action) FKOption
  func WithOnUpdate(a Action) FKOption
  func References(table, column string, opts ...FKOption) ColumnModifier
  // internal: func (c columnDef) definitionSQL() (string, error)  // "\"age\" integer NOT NULL CHECK (\"age\" >= 0)"
  ```

- [ ] **Step 1: Write failing tests**

```go
func TestColumnDefinitionSQL(t *testing.T) {
	c := columnDef{name: "email", typ: Text}
	NotNull()(&c); Unique()(&c)
	got, _ := c.definitionSQL()
	if norm(got) != `"email" text NOT NULL UNIQUE` { t.Fatalf("got %q", norm(got)) }
}
func TestUnsignedEmitsCheck(t *testing.T) {
	c := columnDef{name: "age", typ: Int}; WithUnsigned()(&c)
	got, _ := c.definitionSQL()
	if norm(got) != `"age" integer CHECK ("age" >= 0)` { t.Fatalf("got %q", norm(got)) }
}
func TestDefaultQuotingAndExpr(t *testing.T) {
	c := columnDef{name: "status", typ: Text}; Default("active")(&c)
	got, _ := c.definitionSQL()
	if norm(got) != `"status" text DEFAULT 'active'` { t.Fatalf("got %q", norm(got)) }
	c2 := columnDef{name: "created_at", typ: Timestamptz}; Default(RawExpr("now()"))(&c2)
	got2, _ := c2.definitionSQL()
	if norm(got2) != `"created_at" timestamptz DEFAULT now()` { t.Fatalf("got %q", norm(got2)) }
}
func TestReferencesWithOnDelete(t *testing.T) {
	c := columnDef{name: "org_id", typ: BigInt}
	References("orgs", "id", WithOnDelete(Cascade))(&c)
	got, _ := c.definitionSQL()
	if norm(got) != `"org_id" bigint REFERENCES "orgs" ("id") ON DELETE CASCADE` {
		t.Fatalf("got %q", norm(got))
	}
}
func TestAutoIncrementUsesSerial(t *testing.T) {
	c := columnDef{name: "id", typ: BigInt}; WithAutoIncrement()(&c); PrimaryKey()(&c)
	got, _ := c.definitionSQL()
	if norm(got) != `"id" bigserial PRIMARY KEY` { t.Fatalf("got %q", norm(got)) }
}
```

- [ ] **Step 2: Run to verify failure** → FAIL.

- [ ] **Step 3: Implement** — modifiers set fields. `Default(v)`: `string`→`quoteLiteral`; `Expr`→`v.expr()`; other (`int`,`bool`,`float`)→`fmt.Sprint`. `definitionSQL` assembles: `quoteIdent(name)` + type (if `autoIncrement`, map `Int`→`serial`, `BigInt`→`bigserial`, else keep type) + `PRIMARY KEY`/`NOT NULL`/`UNIQUE`/`DEFAULT ...`/`REFERENCES tbl (col) ON DELETE/UPDATE ...`/`GENERATED ALWAYS AS (expr) STORED`/`CHECK (...)`. `WithUnsigned` appends `CHECK ("<name>" >= 0)` (combine with an explicit `Check` if both set). Order clauses deterministically to match tests.

- [ ] **Step 4: Run** → PASS.

- [ ] **Step 5: Commit** — `git commit -m "feat(qb): columns and modifiers"`

---

## Task 4: CreateTable

**Files:**
- Create: `querybuilder/create_table.go`
- Test: `querybuilder/create_table_test.go`

**Interfaces:**
- Produces:
  ```go
  type CreateTableBuilder struct{ /* name, ifNotExists, cols []columnDef, tableConstraints []string */ }
  func CreateTable(name string) *CreateTableBuilder
  func (b *CreateTableBuilder) IfNotExists() *CreateTableBuilder
  func (b *CreateTableBuilder) Column(name string, t ColumnType, mods ...ColumnModifier) *CreateTableBuilder
  func (b *CreateTableBuilder) ID(name string, t ColumnType, mods ...ColumnModifier) *CreateTableBuilder // adds PrimaryKey()
  func (b *CreateTableBuilder) UUIDPrimary(name string) *CreateTableBuilder // uuid PK default gen_random_uuid()
  func (b *CreateTableBuilder) Timestamps() *CreateTableBuilder // created_at, updated_at
  func (b *CreateTableBuilder) PrimaryKeyColumns(cols ...string) *CreateTableBuilder
  func (b *CreateTableBuilder) UniqueColumns(cols ...string) *CreateTableBuilder
  func (b *CreateTableBuilder) CheckConstraint(name, expr string) *CreateTableBuilder
  func (b *CreateTableBuilder) ToSQL() (string, error)
  func (b *CreateTableBuilder) Execute(ctx context.Context, exec Execer) error
  ```

- [ ] **Step 1: Write failing tests**

```go
func TestCreateTableBasic(t *testing.T) {
	sql, err := CreateTable("users").
		ID("id", BigInt, WithAutoIncrement()).
		Column("email", Text, NotNull(), Unique()).
		Column("age", Int, WithUnsigned()).
		Timestamps().
		ToSQL()
	if err != nil { t.Fatal(err) }
	want := `CREATE TABLE "users" ( ` +
		`"id" bigserial PRIMARY KEY, ` +
		`"email" text NOT NULL UNIQUE, ` +
		`"age" integer CHECK ("age" >= 0), ` +
		`"created_at" timestamptz NOT NULL DEFAULT now(), ` +
		`"updated_at" timestamptz NOT NULL DEFAULT now() )`
	if norm(sql) != norm(want) { t.Fatalf("\n got %q\nwant %q", norm(sql), norm(want)) }
}
func TestCreateTableIfNotExistsAndTableConstraints(t *testing.T) {
	sql, _ := CreateTable("t").IfNotExists().
		Column("a", Int).Column("b", Int).
		PrimaryKeyColumns("a", "b").
		ToSQL()
	if !strings.Contains(norm(sql), `CREATE TABLE IF NOT EXISTS "t"`) { t.Fatal("if not exists") }
	if !strings.Contains(norm(sql), `PRIMARY KEY ("a", "b")`) { t.Fatal("composite pk") }
}
func TestCreateTableNoColumnsErrors(t *testing.T) {
	if _, err := CreateTable("t").ToSQL(); err == nil { t.Fatal("expected error for no columns") }
}
```

- [ ] **Step 2: Run to verify failure** → FAIL.

- [ ] **Step 3: Implement** — accumulate columns/constraints. `ID` = `Column` + `PrimaryKey()`. `UUIDPrimary` = `Column(name, UUID, PrimaryKey(), Default(RawExpr("gen_random_uuid()")))`. `Timestamps` adds two `timestamptz NOT NULL DEFAULT now()` columns. `ToSQL`: error if no columns; emit `CREATE TABLE [IF NOT EXISTS] "name" ( <col defs>, <table constraints> )`; table constraints render as `PRIMARY KEY (...)`, `UNIQUE (...)`, `CONSTRAINT "name" CHECK (expr)`. `Execute` = `execStatements(ctx, exec, []string{sql})`.

- [ ] **Step 4: Run** → PASS.

- [ ] **Step 5: Commit** — `git commit -m "feat(qb): CreateTable builder"`

---

## Task 5: DropTable

**Files:**
- Create: `querybuilder/drop_table.go`
- Test: `querybuilder/drop_table_test.go`

**Interfaces:**
- Produces: `func DropTable(name string) *DropTableBuilder` with `IfExists()`, `Cascade()`, `ToSQL`, `Execute`.

- [ ] **Step 1: Write failing test**

```go
func TestDropTable(t *testing.T) {
	sql, _ := DropTable("users").IfExists().Cascade().ToSQL()
	if norm(sql) != `DROP TABLE IF EXISTS "users" CASCADE` { t.Fatalf("got %q", norm(sql)) }
	sql2, _ := DropTable("users").ToSQL()
	if norm(sql2) != `DROP TABLE "users"` { t.Fatalf("got %q", norm(sql2)) }
}
```

- [ ] **Step 2: Run to verify failure** → FAIL.
- [ ] **Step 3: Implement** the builder.
- [ ] **Step 4: Run** → PASS.
- [ ] **Step 5: Commit** — `git commit -m "feat(qb): DropTable builder"`

---

## Task 6: AlterTable

**Files:**
- Create: `querybuilder/alter_table.go`
- Test: `querybuilder/alter_table_test.go`

**Interfaces:**
- Produces:
  ```go
  func AlterTable(name string) *AlterTableBuilder
  // fluent, each appends an action; return *AlterTableBuilder:
  AddColumn(name string, t ColumnType, mods ...ColumnModifier)
  DropColumn(name string)
  RenameColumn(from, to string)
  AlterColumnType(name string, t ColumnType)
  SetDefault(name string, expr string)   // expr verbatim
  DropDefault(name string)
  SetNotNull(name string)
  DropNotNull(name string)
  AddConstraint(name, expr string)       // expr is full constraint body e.g. "CHECK (age >= 0)"
  DropConstraint(name string)
  AddForeignKey(column, refTable, refColumn string, opts ...FKOption)
  RenameTo(newName string)
  ToSQL() (string, error); Execute(ctx, Execer) error
  ```

- [ ] **Step 1: Write failing tests**

```go
func TestAlterTableActions(t *testing.T) {
	sql, err := AlterTable("users").
		AddColumn("phone", Varchar(32), NotNull()).
		DropColumn("legacy").
		AlterColumnType("age", BigInt).
		SetNotNull("phone").
		AddConstraint("chk_age", "CHECK (age >= 0)").
		ToSQL()
	if err != nil { t.Fatal(err) }
	n := norm(sql)
	for _, want := range []string{
		`ALTER TABLE "users"`,
		`ADD COLUMN "phone" varchar(32) NOT NULL`,
		`DROP COLUMN "legacy"`,
		`ALTER COLUMN "age" TYPE bigint`,
		`ALTER COLUMN "phone" SET NOT NULL`,
		`ADD CONSTRAINT "chk_age" CHECK (age >= 0)`,
	} {
		if !strings.Contains(n, want) { t.Errorf("missing %q in %q", want, n) }
	}
}
func TestRenameEmitsSeparateStatement(t *testing.T) {
	sql, _ := AlterTable("users").RenameColumn("email", "email_address").ToSQL()
	if !strings.Contains(norm(sql), `RENAME COLUMN "email" TO "email_address"`) { t.Fatal("rename column") }
	sql2, _ := AlterTable("users").RenameTo("app_users").ToSQL()
	if !strings.Contains(norm(sql2), `ALTER TABLE "users" RENAME TO "app_users"`) { t.Fatal("rename table") }
}
```

- [ ] **Step 2: Run to verify failure** → FAIL.

- [ ] **Step 3: Implement** — store actions as a slice of typed entries. `ToSQL`: combine comma-separable actions (ADD/DROP COLUMN, ALTER COLUMN, ADD/DROP CONSTRAINT, ADD FOREIGN KEY) into one `ALTER TABLE "name" <a>, <b>, ...;` statement; emit `RENAME COLUMN` and `RENAME TO` as their own separate `ALTER TABLE` statements (Postgres requires it), joined with `;` and newline. `AddForeignKey` renders `ADD CONSTRAINT "fk_<table>_<column>" FOREIGN KEY ("column") REFERENCES "refTable" ("refColumn") [ON DELETE ...][ON UPDATE ...]`. Error if no actions.

- [ ] **Step 4: Run** → PASS.

- [ ] **Step 5: Commit** — `git commit -m "feat(qb): AlterTable builder"`

---

## Task 7: Indexes

**Files:**
- Create: `querybuilder/index.go`
- Test: `querybuilder/index_test.go`

**Interfaces:**
- Produces:
  ```go
  func CreateIndex(name string) *CreateIndexBuilder
  // On(table), Columns(cols...), Unique(), Using(method string), Where(pred string), Concurrently()
  func DropIndex(name string) *DropIndexBuilder // IfExists(), Concurrently()
  ```

- [ ] **Step 1: Write failing tests**

```go
func TestCreateIndexFull(t *testing.T) {
	sql, err := CreateIndex("idx_users_email").
		On("users").Columns("email").Unique().Using("btree").
		Where("deleted_at IS NULL").Concurrently().ToSQL()
	if err != nil { t.Fatal(err) }
	want := `CREATE UNIQUE INDEX CONCURRENTLY "idx_users_email" ON "users" USING btree ("email") WHERE deleted_at IS NULL`
	if norm(sql) != norm(want) { t.Fatalf("\n got %q\nwant %q", norm(sql), norm(want)) }
}
func TestCreateIndexComposite(t *testing.T) {
	sql, _ := CreateIndex("idx_ab").On("t").Columns("a", "b").ToSQL()
	if norm(sql) != `CREATE INDEX "idx_ab" ON "t" ("a", "b")` { t.Fatalf("got %q", norm(sql)) }
}
func TestCreateIndexNeedsTableAndColumns(t *testing.T) {
	if _, err := CreateIndex("x").ToSQL(); err == nil { t.Fatal("expected error") }
}
func TestDropIndex(t *testing.T) {
	sql, _ := DropIndex("idx_ab").IfExists().Concurrently().ToSQL()
	if norm(sql) != `DROP INDEX CONCURRENTLY IF EXISTS "idx_ab"` { t.Fatalf("got %q", norm(sql)) }
}
```

- [ ] **Step 2: Run to verify failure** → FAIL.
- [ ] **Step 3: Implement** the two builders; clause order: `CREATE [UNIQUE] INDEX [CONCURRENTLY] "name" ON "table" [USING method] (cols) [WHERE pred]`. Error if no table or no columns.
- [ ] **Step 4: Run** → PASS.
- [ ] **Step 5: Commit** — `git commit -m "feat(qb): index builders"`

---

## Task 8: Enums / types

**Files:**
- Create: `querybuilder/type_enum.go`
- Test: `querybuilder/type_enum_test.go`

**Interfaces:**
- Produces:
  ```go
  func CreateType(name string) *CreateTypeBuilder  // .AsEnum(vals ...string)
  func AlterTypeAddValue(typeName, value string) *AlterTypeBuilder // .Before(v)/.After(v) optional
  func DropType(name string) *DropTypeBuilder      // IfExists(), Cascade()
  ```

- [ ] **Step 1: Write failing tests**

```go
func TestCreateEnum(t *testing.T) {
	sql, err := CreateType("user_role").AsEnum("admin", "member", "guest").ToSQL()
	if err != nil { t.Fatal(err) }
	if norm(sql) != `CREATE TYPE "user_role" AS ENUM ('admin', 'member', 'guest')` {
		t.Fatalf("got %q", norm(sql))
	}
}
func TestAlterTypeAddValue(t *testing.T) {
	sql, _ := AlterTypeAddValue("user_role", "owner").ToSQL()
	if norm(sql) != `ALTER TYPE "user_role" ADD VALUE 'owner'` { t.Fatalf("got %q", norm(sql)) }
	sql2, _ := AlterTypeAddValue("user_role", "owner").Before("admin").ToSQL()
	if norm(sql2) != `ALTER TYPE "user_role" ADD VALUE 'owner' BEFORE 'admin'` { t.Fatalf("got %q", norm(sql2)) }
}
func TestDropType(t *testing.T) {
	sql, _ := DropType("user_role").IfExists().Cascade().ToSQL()
	if norm(sql) != `DROP TYPE IF EXISTS "user_role" CASCADE` { t.Fatalf("got %q", norm(sql)) }
}
```

- [ ] **Step 2: Run to verify failure** → FAIL.
- [ ] **Step 3: Implement** — `AsEnum` quotes each value with `quoteLiteral`; `CreateType` errors if no values. `AlterTypeAddValue` optional `Before`/`After` (mutually exclusive; last one wins or error if both).
- [ ] **Step 4: Run** → PASS.
- [ ] **Step 5: Commit** — `git commit -m "feat(qb): enum/type builders"`

---

## Task 9: Schemas + package README section

**Files:**
- Create: `querybuilder/schema.go`
- Test: `querybuilder/schema_test.go`

**Interfaces:**
- Produces: `func CreateSchema(name string) *CreateSchemaBuilder` (`IfNotExists()`); `func DropSchema(name string) *DropSchemaBuilder` (`IfExists()`, `Cascade()`).

- [ ] **Step 1: Write failing tests**

```go
func TestSchemas(t *testing.T) {
	s1, _ := CreateSchema("billing").IfNotExists().ToSQL()
	if norm(s1) != `CREATE SCHEMA IF NOT EXISTS "billing"` { t.Fatalf("got %q", norm(s1)) }
	s2, _ := DropSchema("billing").IfExists().Cascade().ToSQL()
	if norm(s2) != `DROP SCHEMA IF EXISTS "billing" CASCADE` { t.Fatalf("got %q", norm(s2)) }
}
```

- [ ] **Step 2: Run to verify failure** → FAIL.
- [ ] **Step 3: Implement** the two builders.
- [ ] **Step 4: Run** → PASS.
- [ ] **Step 5: Commit** — `git commit -m "feat(qb): schema builders"`

---

## Task 10: Optional integration smoke test

**Files:**
- Create: `querybuilder/integration_test.go`

- [ ] **Step 1: Write a guarded test** that skips unless `TEST_DATABASE_URL` is set, connects with `pgxpool`, and `Execute`s a representative chain: `CreateSchema`, `CreateType(...).AsEnum`, `CreateTable` (with FK, unsigned check, timestamps), `CreateIndex`, `AlterTable().AddColumn`, then drops them — asserting no error. This confirms the generated SQL is valid Postgres.

```go
func TestIntegrationRoundtrip(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" { t.Skip("set TEST_DATABASE_URL") }
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil { t.Fatal(err) }
	defer pool.Close()
	ctx := context.Background()
	must := func(err error) { if err != nil { t.Fatal(err) } }
	must(CreateTable("qb_users").
		ID("id", BigInt, WithAutoIncrement()).
		Column("email", Text, NotNull(), Unique()).
		Column("age", Int, WithUnsigned()).
		Timestamps().Execute(ctx, pool))
	must(CreateIndex("qb_idx_email").On("qb_users").Columns("email").Unique().Execute(ctx, pool))
	must(AlterTable("qb_users").AddColumn("phone", Varchar(32)).Execute(ctx, pool))
	must(DropTable("qb_users").IfExists().Cascade().Execute(ctx, pool))
}
```

- [ ] **Step 2: Run** — `TEST_DATABASE_URL=postgres://... go test ./querybuilder/ -run TestIntegration -v` → PASS (or SKIP without the env).
- [ ] **Step 3: Commit** — `git commit -m "test(qb): integration roundtrip against Postgres"`

---

## Self-Review Notes

- Spec coverage: types (T2), columns+modifiers incl. FK/unsigned/generated/check/comment (T3), CreateTable+helpers (T4), DropTable (T5), AlterTable all actions (T6), indexes incl. partial/concurrent/composite/method (T7), enums (T8), schemas (T9), Raw + identifiers (T1), integration (T10). All spec sections mapped.
- `Execer` is defined once (T1), exactly `Exec` — never widened.
- No imports of `migrator`; nothing touched outside `querybuilder/`.
- All `ToSQL` assertions normalize whitespace via `norm`, so formatting is free to vary.
