package querybuilder

import "testing"

func TestCreateIndexFull(t *testing.T) {
	sql, err := CreateIndex("idx_users_email").
		On("users").Columns("email").Unique().Using("btree").
		Where("deleted_at IS NULL").Concurrently().ToSQL()
	if err != nil {
		t.Fatal(err)
	}
	want := `CREATE UNIQUE INDEX CONCURRENTLY "idx_users_email" ON "users" USING btree ("email") WHERE deleted_at IS NULL`
	if norm(sql) != norm(want) {
		t.Fatalf("\n got %q\nwant %q", norm(sql), norm(want))
	}
}

func TestCreateIndexComposite(t *testing.T) {
	sql, _ := CreateIndex("idx_ab").On("t").Columns("a", "b").ToSQL()
	if norm(sql) != `CREATE INDEX "idx_ab" ON "t" ("a", "b")` {
		t.Fatalf("got %q", norm(sql))
	}
}

func TestCreateIndexNeedsTableAndColumns(t *testing.T) {
	if _, err := CreateIndex("x").ToSQL(); err == nil {
		t.Fatal("expected error")
	}
	if _, err := CreateIndex("x").On("t").ToSQL(); err == nil {
		t.Fatal("expected error for no columns")
	}
	if _, err := CreateIndex("x").Columns("a").ToSQL(); err == nil {
		t.Fatal("expected error for no table")
	}
}

func TestDropIndex(t *testing.T) {
	sql, _ := DropIndex("idx_ab").IfExists().Concurrently().ToSQL()
	if norm(sql) != `DROP INDEX CONCURRENTLY IF EXISTS "idx_ab"` {
		t.Fatalf("got %q", norm(sql))
	}
}
