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
