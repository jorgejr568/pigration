package querybuilder

import "testing"

func TestRawToSQL(t *testing.T) {
	s, err := Raw("CREATE EXTENSION pgcrypto;").ToSQL()
	if err != nil || s != "CREATE EXTENSION pgcrypto;" {
		t.Fatalf("got %q %v", s, err)
	}
}

func TestRawExpr(t *testing.T) {
	e := RawExpr("now()")
	if e.expr() != "now()" {
		t.Fatalf("got %q", e.expr())
	}
}
