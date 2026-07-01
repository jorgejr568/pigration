package querybuilder

import (
	"strings"
	"testing"
)

func TestCreateTableBasic(t *testing.T) {
	sql, err := CreateTable("users").
		ID("id", BigSerial).
		Column("email", Text, NotNull(), Unique()).
		Column("age", Int, WithUnsigned()).
		Timestamps().
		ToSQL()
	if err != nil {
		t.Fatal(err)
	}
	want := `CREATE TABLE "users" ( ` +
		`"id" bigserial PRIMARY KEY, ` +
		`"email" text NOT NULL UNIQUE, ` +
		`"age" integer CHECK ("age" >= 0), ` +
		`"created_at" timestamptz NOT NULL DEFAULT now(), ` +
		`"updated_at" timestamptz NOT NULL DEFAULT now() )`
	if norm(sql) != norm(want) {
		t.Fatalf("\n got %q\nwant %q", norm(sql), norm(want))
	}
}

func TestCreateTableIfNotExistsAndTableConstraints(t *testing.T) {
	sql, _ := CreateTable("t").IfNotExists().
		Column("a", Int).Column("b", Int).
		PrimaryKeyColumns("a", "b").
		ToSQL()
	if !strings.Contains(norm(sql), `CREATE TABLE IF NOT EXISTS "t"`) {
		t.Fatal("if not exists")
	}
	if !strings.Contains(norm(sql), `PRIMARY KEY ("a", "b")`) {
		t.Fatal("composite pk")
	}
}

func TestCreateTableNoColumnsErrors(t *testing.T) {
	if _, err := CreateTable("t").ToSQL(); err == nil {
		t.Fatal("expected error for no columns")
	}
}

func TestCreateTableUniqueAndCheckConstraintAndUUID(t *testing.T) {
	sql, err := CreateTable("t").
		UUIDPrimary("id").
		Column("a", Int).
		UniqueColumns("a").
		CheckConstraint("chk_a", "a > 0").
		ToSQL()
	if err != nil {
		t.Fatal(err)
	}
	n := norm(sql)
	if !strings.Contains(n, `"id" uuid PRIMARY KEY DEFAULT gen_random_uuid()`) {
		t.Fatalf("uuid primary: %q", n)
	}
	if !strings.Contains(n, `UNIQUE ("a")`) {
		t.Fatalf("unique columns: %q", n)
	}
	if !strings.Contains(n, `CONSTRAINT "chk_a" CHECK (a > 0)`) {
		t.Fatalf("check constraint: %q", n)
	}
}
