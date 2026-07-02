// The pigration example app: a todo JSON API that migrates its own database on
// boot via the migrator library — no CLI needed in production.
package main

import (
	"cmp"
	"context"
	"log"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jorgejr568/pigration/migrator"

	_ "github.com/jorgejr568/pigration/examples/todo-app/migrations" // register migrations
	"github.com/jorgejr568/pigration/examples/todo-app/todo"
)

func main() {
	ctx := context.Background()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("connecting: %v", err)
	}
	defer pool.Close()

	res, err := migrator.Up(ctx, pool)
	if err != nil {
		log.Fatalf("migrating: %v", err)
	}
	if len(res.Applied) > 0 {
		log.Printf("applied %d migration(s) in batch %d", len(res.Applied), res.Batch)
	}

	addr := ":" + cmp.Or(os.Getenv("PORT"), "8080")
	log.Printf("todo-app listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, todo.NewServer(pool)))
}
