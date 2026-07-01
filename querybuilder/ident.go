package querybuilder

import (
	"context"
	"fmt"
	"strings"
)

// quoteIdent wraps an identifier in double quotes, doubling any embedded
// double quotes. e.g. `email` -> "email"; `a"b` -> "a""b".
// Dotted names are quoted verbatim as a single identifier: schema-qualified
// names (e.g. "billing.users") are not currently supported.
func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// quoteLiteral wraps a string in single quotes, doubling any embedded single
// quote to keep the SQL string literal safe (standard Postgres escaping).
func quoteLiteral(s string) string {
	return `'` + strings.ReplaceAll(s, `'`, `''`) + `'`
}

// execStatements runs each statement in order against exec, wrapping any error
// with the offending SQL.
func execStatements(ctx context.Context, exec Execer, stmts []string) error {
	for _, s := range stmts {
		if _, err := exec.Exec(ctx, s); err != nil {
			return fmt.Errorf("querybuilder: executing %q: %w", s, err)
		}
	}
	return nil
}
