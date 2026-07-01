package querybuilder

import "testing"

func TestQuoteIdent(t *testing.T) {
	if quoteIdent("email") != `"email"` {
		t.Fatal("basic")
	}
	if quoteIdent(`we"ird`) != `"we""ird"` {
		t.Fatal("escape")
	}
}

func TestQuoteLiteral(t *testing.T) {
	if quoteLiteral("O'Brien") != `'O''Brien'` {
		t.Fatal("literal escape")
	}
}
