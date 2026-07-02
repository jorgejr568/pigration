// Package migrator is a Postgres migration engine where migrations are
// ordinary Go code, registered via init() and compiled into your binary.
//
// A migration is a pair of functions with the signature
//
//	func(ctx context.Context, tx migrator.Executor) error
//
// registered under a chronologically sortable identity:
//
//	func init() {
//		migrator.Register("1719800000_create_users", up, down)
//	}
//
// [Up] applies all pending migrations, grouping them into a single batch;
// [Down] reverts the most recent batch (or [Steps] / [Batch] targets);
// [Status] reports applied and pending migrations; [Fresh] drops the public
// schema and re-applies everything (guarded by [AllowFresh]).
//
// Each migration runs inside its own transaction and is rolled back if it
// returns an error. Register with [NonTransactional] for statements Postgres
// cannot run inside a transaction (CREATE INDEX CONCURRENTLY, VACUUM, ...);
// such a migration runs directly against the pool and is not rolled back on
// failure.
//
// Every run takes a session-level Postgres advisory lock, so concurrent
// runners fail fast with [ErrLocked] instead of racing.
//
// The pigration CLI drives this same package via a generated `go run`
// entrypoint; embedding it in an application is a direct call:
//
//	pool, _ := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
//	res, err := migrator.Up(ctx, pool)
package migrator
