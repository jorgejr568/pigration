package querybuilder

import "testing"

func TestCreateEnum(t *testing.T) {
	sql, err := CreateType("user_role").AsEnum("admin", "member", "guest").ToSQL()
	if err != nil {
		t.Fatal(err)
	}
	if norm(sql) != `CREATE TYPE "user_role" AS ENUM ('admin', 'member', 'guest')` {
		t.Fatalf("got %q", norm(sql))
	}
}

func TestCreateEnumNoValuesErrors(t *testing.T) {
	if _, err := CreateType("user_role").ToSQL(); err == nil {
		t.Fatal("expected error for no values")
	}
	// AsEnum called with an empty slice must also error (collapsed guard).
	if _, err := CreateType("user_role").AsEnum().ToSQL(); err == nil {
		t.Fatal("expected error for AsEnum with no values")
	}
}

func TestAlterTypeAddValue(t *testing.T) {
	sql, _ := AlterTypeAddValue("user_role", "owner").ToSQL()
	if norm(sql) != `ALTER TYPE "user_role" ADD VALUE 'owner'` {
		t.Fatalf("got %q", norm(sql))
	}
	sql2, _ := AlterTypeAddValue("user_role", "owner").Before("admin").ToSQL()
	if norm(sql2) != `ALTER TYPE "user_role" ADD VALUE 'owner' BEFORE 'admin'` {
		t.Fatalf("got %q", norm(sql2))
	}
	sql3, _ := AlterTypeAddValue("user_role", "owner").After("guest").ToSQL()
	if norm(sql3) != `ALTER TYPE "user_role" ADD VALUE 'owner' AFTER 'guest'` {
		t.Fatalf("got %q", norm(sql3))
	}
}

func TestDropType(t *testing.T) {
	sql, _ := DropType("user_role").IfExists().Cascade().ToSQL()
	if norm(sql) != `DROP TYPE IF EXISTS "user_role" CASCADE` {
		t.Fatalf("got %q", norm(sql))
	}
}
