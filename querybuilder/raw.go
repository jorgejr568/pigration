package querybuilder

import "context"

// Expr is a marker for raw SQL expressions emitted verbatim (e.g. now()).
type Expr interface {
	expr() string
}

type rawExpr struct{ sql string }

func (e rawExpr) expr() string { return e.sql }

// RawExpr wraps a SQL expression for use as a column default or other
// expression position; it is emitted verbatim.
func RawExpr(sql string) Expr {
	return rawExpr{sql: sql}
}

// RawStmt is a statement-level raw SQL escape hatch.
type RawStmt struct {
	sql  string
	args []any
}

// Raw builds a statement-level raw SQL builder. Execute runs it verbatim.
func Raw(sql string, args ...any) RawStmt {
	return RawStmt{sql: sql, args: args}
}

// ToSQL returns the raw SQL string.
func (r RawStmt) ToSQL() (string, error) {
	return r.sql, nil
}

// Execute runs the raw SQL against exec.
func (r RawStmt) Execute(ctx context.Context, exec Execer) error {
	_, err := exec.Exec(ctx, r.sql, r.args...)
	return err
}
