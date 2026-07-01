package migrator

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Executor is the database handle passed to every migration function. It is a
// superset of querybuilder.Execer, so a migration may call
// qb...Execute(ctx, tx) directly. Satisfied by pgx.Tx, *pgxpool.Pool, *pgx.Conn.
type Executor interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// MigrationFunc is the signature of an Up or Down migration function.
type MigrationFunc func(ctx context.Context, tx Executor) error
