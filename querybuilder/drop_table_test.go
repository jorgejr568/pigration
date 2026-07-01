package querybuilder

import "testing"

func TestDropTable(t *testing.T) {
	sql, _ := DropTable("users").IfExists().Cascade().ToSQL()
	if norm(sql) != `DROP TABLE IF EXISTS "users" CASCADE` {
		t.Fatalf("got %q", norm(sql))
	}
	sql2, _ := DropTable("users").ToSQL()
	if norm(sql2) != `DROP TABLE "users"` {
		t.Fatalf("got %q", norm(sql2))
	}
}
