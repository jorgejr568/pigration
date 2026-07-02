// Package querybuilder is a fluent Postgres DDL builder: tables, columns and
// constraints, indexes, enum types, and schemas, rendered as SQL strings.
//
// Every builder is pure until executed — [CreateTableBuilder.ToSQL] and
// friends return the SQL without touching a database, and Execute runs it
// against anything satisfying the one-method [Execer] interface (pgx.Tx,
// *pgxpool.Pool, *pgx.Conn).
//
// Execer is deliberately a subset of the migration engine's Executor
// interface, so inside a migration the transaction can be passed straight in:
//
//	func Up(ctx context.Context, tx migrator.Executor) error {
//		return querybuilder.CreateTable("users").
//			ID("id", querybuilder.BigSerial).
//			Column("email", querybuilder.Text, querybuilder.NotNull()).
//			Execute(ctx, tx)
//	}
//
// Identifiers are always double-quoted (and embedded quotes escaped), string
// literals single-quoted. DDL cannot be parameterized in Postgres, so
// expression inputs (Check, GeneratedAs, RawExpr, index Where clauses) are
// emitted verbatim and must be trusted input.
//
// The builder is a convenience layer, never a cage: [Raw] executes any
// statement with bound arguments for anything the builders don't cover.
package querybuilder
