package querybuilder

import (
	"context"

	"github.com/jackc/pgx/v5/pgconn"
)

// Execer is the narrow executor every builder's Execute accepts. It is a subset of
// migrator.Executor, so a migration's tx parameter satisfies it directly.
// Satisfied by pgx.Tx, *pgxpool.Pool, *pgx.Conn.
type Execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// sqler is any builder that renders a single statement.
type sqler interface{ ToSQL() (string, error) }

// execBuilder renders b and runs the resulting statement. It is only for
// arg-less single-statement builders: RawStmt keeps its own Execute because it
// passes bound args through, and AlterTableBuilder keeps its own because it may
// emit multiple statements.
func execBuilder(ctx context.Context, exec Execer, b sqler) error {
	sql, err := b.ToSQL()
	if err != nil {
		return err
	}
	return execStatements(ctx, exec, []string{sql})
}
