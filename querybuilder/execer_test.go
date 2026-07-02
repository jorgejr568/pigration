package querybuilder

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// fakeExecer is a tiny Execer that records every statement it is asked to run
// and optionally fails on the call whose SQL contains failOn. It lets the tests
// drive both the success and error paths of every builder's Execute without a
// live database.
type fakeExecer struct {
	calls  []string
	args   [][]any
	err    error  // returned unconditionally when non-nil and failOn is empty
	failOn string // if set, err is returned only when the SQL contains this substring
}

var errExec = errors.New("fake exec failed")

func (f *fakeExecer) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	f.calls = append(f.calls, sql)
	f.args = append(f.args, args)
	if f.err != nil && (f.failOn == "" || strings.Contains(sql, f.failOn)) {
		return pgconn.CommandTag{}, f.err
	}
	return pgconn.CommandTag{}, nil
}

func TestCreateIndexExecute(t *testing.T) {
	f := &fakeExecer{}
	err := CreateIndex("idx_users_email").On("users").Columns("email").Execute(context.Background(), f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.calls) != 1 {
		t.Fatalf("want 1 call, got %d: %v", len(f.calls), f.calls)
	}
	if norm(f.calls[0]) != `CREATE INDEX "idx_users_email" ON "users" ("email")` {
		t.Fatalf("got %q", norm(f.calls[0]))
	}
}

// TestExecBuilderToSQLError covers execBuilder's ToSQL-error branch: the
// builder fails validation before any Exec happens, so nothing is executed.
func TestExecBuilderToSQLError(t *testing.T) {
	f := &fakeExecer{}
	// CreateIndex with no table fails in ToSQL.
	err := CreateIndex("x").Execute(context.Background(), f)
	if err == nil {
		t.Fatal("expected error from ToSQL validation")
	}
	if len(f.calls) != 0 {
		t.Fatalf("expected no Exec calls, got %v", f.calls)
	}
}

// TestExecStatementsExecError covers execStatements' Exec-error branch: the
// error is wrapped with the offending SQL.
func TestExecStatementsExecError(t *testing.T) {
	f := &fakeExecer{err: errExec}
	err := CreateIndex("idx_x").On("t").Columns("a").Execute(context.Background(), f)
	if err == nil {
		t.Fatal("expected error from Exec")
	}
	if !errors.Is(err, errExec) {
		t.Fatalf("want wrapped errExec, got %v", err)
	}
	if want := `querybuilder: executing`; !strings.Contains(err.Error(), want) {
		t.Fatalf("want error to contain %q, got %q", want, err.Error())
	}
	if !strings.Contains(err.Error(), `CREATE INDEX`) || !strings.Contains(err.Error(), `idx_x`) {
		t.Fatalf("want error to include offending SQL, got %q", err.Error())
	}
}

func TestDropIndexExecute(t *testing.T) {
	f := &fakeExecer{}
	if err := DropIndex("idx_ab").IfExists().Execute(context.Background(), f); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.calls) != 1 || norm(f.calls[0]) != `DROP INDEX IF EXISTS "idx_ab"` {
		t.Fatalf("got %v", f.calls)
	}

	f2 := &fakeExecer{err: errExec}
	if err := DropIndex("idx_ab").Execute(context.Background(), f2); !errors.Is(err, errExec) {
		t.Fatalf("want errExec, got %v", err)
	}
}

func TestRawStmtExecute(t *testing.T) {
	f := &fakeExecer{}
	err := Raw("SELECT set_config($1, $2, false)", "k", "v").Execute(context.Background(), f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.calls) != 1 || f.calls[0] != "SELECT set_config($1, $2, false)" {
		t.Fatalf("got calls %v", f.calls)
	}
	if len(f.args[0]) != 2 || f.args[0][0] != "k" || f.args[0][1] != "v" {
		t.Fatalf("args not passed through: %v", f.args[0])
	}

	// Error path: Execute returns the raw error verbatim (no wrapping).
	f2 := &fakeExecer{err: errExec}
	if err := Raw("SELECT 1").Execute(context.Background(), f2); !errors.Is(err, errExec) {
		t.Fatalf("want errExec, got %v", err)
	}
}

func TestCreateTypeExecute(t *testing.T) {
	f := &fakeExecer{}
	err := CreateType("role").AsEnum("admin", "member").Execute(context.Background(), f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.calls) != 1 || norm(f.calls[0]) != `CREATE TYPE "role" AS ENUM ('admin', 'member')` {
		t.Fatalf("got %v", f.calls)
	}

	// ToSQL-error path: no enum values.
	f2 := &fakeExecer{}
	if err := CreateType("role").Execute(context.Background(), f2); err == nil {
		t.Fatal("expected ToSQL error for missing enum values")
	}
	if len(f2.calls) != 0 {
		t.Fatalf("expected no Exec calls, got %v", f2.calls)
	}
}

func TestAlterTypeExecute(t *testing.T) {
	f := &fakeExecer{}
	err := AlterTypeAddValue("role", "owner").After("admin").Execute(context.Background(), f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.calls) != 1 || norm(f.calls[0]) != `ALTER TYPE "role" ADD VALUE 'owner' AFTER 'admin'` {
		t.Fatalf("got %v", f.calls)
	}

	f2 := &fakeExecer{err: errExec}
	if err := AlterTypeAddValue("role", "owner").Execute(context.Background(), f2); !errors.Is(err, errExec) {
		t.Fatalf("want errExec, got %v", err)
	}
}

func TestCreateSchemaExecute(t *testing.T) {
	f := &fakeExecer{}
	if err := CreateSchema("billing").IfNotExists().Execute(context.Background(), f); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.calls) != 1 || norm(f.calls[0]) != `CREATE SCHEMA IF NOT EXISTS "billing"` {
		t.Fatalf("got %v", f.calls)
	}
}

// TestAlterTableExecuteMultiStatement covers AlterTableBuilder.Execute for the
// happy path where a standalone action (RENAME) forces two separate
// statements, and the error path where an empty builder fails validation.
func TestAlterTableExecuteMultiStatement(t *testing.T) {
	f := &fakeExecer{}
	err := AlterTable("users").
		AddColumn("phone", Text).
		RenameColumn("phone", "phone_number").
		Execute(context.Background(), f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.calls) != 2 {
		t.Fatalf("want 2 statements, got %d: %v", len(f.calls), f.calls)
	}
	if norm(f.calls[0]) != `ALTER TABLE "users" ADD COLUMN "phone" text` {
		t.Fatalf("stmt0 = %q", norm(f.calls[0]))
	}
	if norm(f.calls[1]) != `ALTER TABLE "users" RENAME COLUMN "phone" TO "phone_number"` {
		t.Fatalf("stmt1 = %q", norm(f.calls[1]))
	}

	// Error path: no actions -> statements() returns an error, nothing runs.
	f2 := &fakeExecer{}
	if err := AlterTable("users").Execute(context.Background(), f2); err == nil {
		t.Fatal("expected error for AlterTable with no actions")
	}
	if len(f2.calls) != 0 {
		t.Fatalf("expected no Exec calls, got %v", f2.calls)
	}

	// Exec-error path: the second statement fails, error is wrapped with its SQL.
	f3 := &fakeExecer{err: errExec, failOn: "RENAME"}
	err = AlterTable("users").
		AddColumn("phone", Text).
		RenameColumn("phone", "phone_number").
		Execute(context.Background(), f3)
	if !errors.Is(err, errExec) {
		t.Fatalf("want errExec, got %v", err)
	}
	if !strings.Contains(err.Error(), "RENAME COLUMN") {
		t.Fatalf("want error to name the failing statement, got %q", err.Error())
	}
}

// TestReferencesWithOnUpdate covers WithOnUpdate and the onUpdate branch of
// referencesSQL.
func TestReferencesWithOnUpdate(t *testing.T) {
	c := columnDef{name: "org_id", typ: BigInt}
	References("orgs", "id", WithOnUpdate(SetNull))(&c)
	got := c.definitionSQL()
	if norm(got) != `"org_id" bigint REFERENCES "orgs" ("id") ON UPDATE SET NULL` {
		t.Fatalf("got %q", norm(got))
	}

	// Both actions together to exercise onDelete and onUpdate in one render.
	c2 := columnDef{name: "org_id", typ: BigInt}
	References("orgs", "id", WithOnDelete(Cascade), WithOnUpdate(Restrict))(&c2)
	got2 := c2.definitionSQL()
	want2 := `"org_id" bigint REFERENCES "orgs" ("id") ON DELETE CASCADE ON UPDATE RESTRICT`
	if norm(got2) != want2 {
		t.Fatalf("got %q", norm(got2))
	}
}

// TestDefaultNonStringNonExpr covers the default arm of Default's type switch
// (numbers and bools rendered with their Go representation).
func TestDefaultNonStringNonExpr(t *testing.T) {
	cInt := columnDef{name: "n", typ: Int}
	Default(42)(&cInt)
	if norm(cInt.definitionSQL()) != `"n" integer DEFAULT 42` {
		t.Fatalf("int default: got %q", norm(cInt.definitionSQL()))
	}

	cBool := columnDef{name: "active", typ: Bool}
	Default(true)(&cBool)
	if norm(cBool.definitionSQL()) != `"active" boolean DEFAULT true` {
		t.Fatalf("bool default: got %q", norm(cBool.definitionSQL()))
	}
}
