package querybuilder

import "testing"

func TestColumnDefinitionSQL(t *testing.T) {
	c := columnDef{name: "email", typ: Text}
	NotNull()(&c)
	Unique()(&c)
	got := c.definitionSQL()
	if norm(got) != `"email" text NOT NULL UNIQUE` {
		t.Fatalf("got %q", norm(got))
	}
}

func TestUnsignedEmitsCheck(t *testing.T) {
	c := columnDef{name: "age", typ: Int}
	WithUnsigned()(&c)
	got := c.definitionSQL()
	if norm(got) != `"age" integer CHECK ("age" >= 0)` {
		t.Fatalf("got %q", norm(got))
	}
}

func TestDefaultQuotingAndExpr(t *testing.T) {
	c := columnDef{name: "status", typ: Text}
	Default("active")(&c)
	got := c.definitionSQL()
	if norm(got) != `"status" text DEFAULT 'active'` {
		t.Fatalf("got %q", norm(got))
	}
	c2 := columnDef{name: "created_at", typ: Timestamptz}
	Default(RawExpr("now()"))(&c2)
	got2 := c2.definitionSQL()
	if norm(got2) != `"created_at" timestamptz DEFAULT now()` {
		t.Fatalf("got %q", norm(got2))
	}
}

func TestReferencesWithOnDelete(t *testing.T) {
	c := columnDef{name: "org_id", typ: BigInt}
	References("orgs", "id", WithOnDelete(Cascade))(&c)
	got := c.definitionSQL()
	if norm(got) != `"org_id" bigint REFERENCES "orgs" ("id") ON DELETE CASCADE` {
		t.Fatalf("got %q", norm(got))
	}
}

func TestGeneratedAndCheck(t *testing.T) {
	c := columnDef{name: "full", typ: Text}
	GeneratedAs("first || ' ' || last")(&c)
	got := c.definitionSQL()
	if norm(got) != `"full" text GENERATED ALWAYS AS (first || ' ' || last) STORED` {
		t.Fatalf("got %q", norm(got))
	}

	c2 := columnDef{name: "age", typ: Int}
	Check("age >= 0")(&c2)
	got2 := c2.definitionSQL()
	if norm(got2) != `"age" integer CHECK (age >= 0)` {
		t.Fatalf("got %q", norm(got2))
	}
}
