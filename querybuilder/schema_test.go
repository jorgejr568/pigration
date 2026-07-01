package querybuilder

import "testing"

func TestSchemas(t *testing.T) {
	s1, _ := CreateSchema("billing").IfNotExists().ToSQL()
	if norm(s1) != `CREATE SCHEMA IF NOT EXISTS "billing"` {
		t.Fatalf("got %q", norm(s1))
	}
	s2, _ := DropSchema("billing").IfExists().Cascade().ToSQL()
	if norm(s2) != `DROP SCHEMA IF EXISTS "billing" CASCADE` {
		t.Fatalf("got %q", norm(s2))
	}
}

func TestSchemasPlain(t *testing.T) {
	s1, _ := CreateSchema("billing").ToSQL()
	if norm(s1) != `CREATE SCHEMA "billing"` {
		t.Fatalf("got %q", norm(s1))
	}
	s2, _ := DropSchema("billing").ToSQL()
	if norm(s2) != `DROP SCHEMA "billing"` {
		t.Fatalf("got %q", norm(s2))
	}
}
