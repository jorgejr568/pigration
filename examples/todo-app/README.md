# todo-app — the pigration example

A minimal, fully functional todo JSON API that shows every pigration feature in
~300 lines: migrations in Go (query builder, raw SQL, and a non-transactional
`CREATE INDEX CONCURRENTLY`), the CLI workflow, and library embedding — the app
migrates its own database on boot with `migrator.Up`.

## Run it

```sh
# from the repo root: throwaway Postgres on :5433
make db-up

cd examples/todo-app
export DATABASE_URL="postgres://postgres:pw@127.0.0.1:5433/pigration_test?sslmode=disable"
go run .          # applies the 3 migrations, serves on :8080
```

## Try it

```sh
curl localhost:8080/todos
curl -X POST localhost:8080/todos -d '{"title":"try pigration"}'
curl -X POST localhost:8080/todos/1/toggle
curl -X DELETE localhost:8080/todos/1
```

## The CLI, from this directory

The pigration CLI reads *this* module's `go.mod` — run it from here:

```sh
go install github.com/jorgejr568/pigration/cmd/pigration@latest
pigration status
pigration make "add priority"     # scaffold a new migration
pigration rollback                # revert the latest batch
```

## What to look at

| File | Shows |
|---|---|
| `migrations/1782900000_create_todos.go` | query-builder migration (`CreateTable`, `Timestamps`) |
| `migrations/1782900100_add_due_date.go` | plain raw-SQL migration |
| `migrations/1782900200_add_pending_index.go` | `NonTransactional()` + `CreateIndex(...).Concurrently()` |
| `main.go` | production-style boot: `migrator.Up(ctx, pool)` before serving |
| `todo/store_test.go` | `migrator.Fresh` as a test fixture |
