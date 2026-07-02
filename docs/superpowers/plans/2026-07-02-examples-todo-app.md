# examples/todo-app Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A fully functional todo HTTP API under `examples/todo-app` that dogfoods pigration end-to-end: library-embedded `migrator.Up` on boot, querybuilder and raw-SQL migrations, a `NonTransactional` concurrent index, the CLI driven from the example directory, and CI coverage.

**Architecture:** The example is its own Go module (`replace`d to the repo root) exactly like a real consumer. `main.go` opens a pgxpool, runs `migrator.Up` (Laravel-style boot migration), and serves a stdlib `net/http` JSON API backed by a small `todo` package (store + handlers). Three migrations showcase the three styles: querybuilder `CreateTable`, raw-SQL `ALTER TABLE`, and a `NonTransactional` `CREATE INDEX CONCURRENTLY`.

**Tech Stack:** Go (version from root `go.mod`), `github.com/jackc/pgx/v5` (pgxpool), stdlib `net/http` (Go 1.22 method+path routing), pigration (`migrator`, `querybuilder`).

## Global Constraints

- Example module path: `github.com/jorgejr568/pigration/examples/todo-app`, with `replace github.com/jorgejr568/pigration => ../..` — never edit the ROOT `go.mod`/`go.sum`; the example's own `go.mod`/`go.sum` are yours to manage (`go mod tidy` inside `examples/todo-app` is allowed and required).
- **Bug-fix rule (from the user, verbatim): "Changes needed or bugs catched you should fix right away."** If any pigration behavior blocks a task or produces wrong results, STOP the example work, write a failing test in the affected library package, fix the library, run the library suite, commit that fix separately (`fix(<pkg>): ...`), then resume the example task.
- Integration tests follow the repo convention: guard on `TEST_DATABASE_URL`, `t.Skip` when unset. All DB-backed steps below assume `make db-up` has been run at the repo root (Postgres 16 on `127.0.0.1:5433`) and use `TEST_DATABASE_URL=postgres://postgres:pw@127.0.0.1:5433/pigration_test?sslmode=disable`.
- No new third-party dependencies: pgx + stdlib only.
- Every commit leaves the tree green: `gofmt -l .` empty, `go vet` clean in both modules, tests pass.
- Migration identities use fixed timestamps `1782900000`, `1782900100`, `1782900200` (chronological, ~2026-07-02) so filenames and registered names in this plan are exact.

## File Structure

```
examples/todo-app/
  go.mod                     own module + replace to ../..
  go.sum                     committed (CI needs it)
  .gitignore                 /.db-migration/ (runner dir; root .gitignore is root-anchored)
  .db-migration.yaml         DATABASE_URL config
  README.md                  run instructions + curl walkthrough
  main.go                    pool + migrator.Up + http.ListenAndServe
  migrations/
    1782900000_create_todos.go     querybuilder CreateTable
    1782900100_add_due_date.go     raw SQL ALTER TABLE
    1782900200_add_pending_index.go NonTransactional + CreateIndex Concurrently
  todo/
    store.go                 Store: List/Create/Toggle/Delete (+ Todo, ErrNotFound)
    store_test.go            guarded integration tests + shared testPool helper
    handlers.go              NewServer(pool) http.Handler, JSON handlers
    handlers_test.go         httptest CRUD walkthrough
.github/workflows/ci.yml     + vet/test steps for the example module
Makefile                     test-all/lint extended to the example module
CONTRIBUTING.md              + examples/ row in the layout table
```

---

## Task 1: Module scaffold + first migration (querybuilder)

**Files:**
- Create: `examples/todo-app/go.mod`, `examples/todo-app/.gitignore`, `examples/todo-app/.db-migration.yaml`, `examples/todo-app/migrations/1782900000_create_todos.go`, `examples/todo-app/todo/store_test.go` (testPool helper + first test only)
- Generated: `examples/todo-app/go.sum` (via `go mod tidy`)

**Interfaces:**
- Consumes: `migrator.Register`, `migrator.Executor`, `migrator.Fresh`, `migrator.AllowFresh`, `querybuilder.CreateTable/ID/Column/Timestamps/BigSerial/Text/Bool/NotNull/Default`.
- Produces: package `migrations` (import path `github.com/jorgejr568/pigration/examples/todo-app/migrations`) registering `1782900000_create_todos`; test helper `testPool(t *testing.T) *pgxpool.Pool` in package `todo` used by every later test.

- [ ] **Step 1: Write the module scaffold**

`examples/todo-app/go.mod`:

```go
module github.com/jorgejr568/pigration/examples/todo-app

go 1.26.4

require (
	github.com/jackc/pgx/v5 v5.10.0
	github.com/jorgejr568/pigration v0.0.0
)

replace github.com/jorgejr568/pigration => ../..
```

`examples/todo-app/.gitignore`:

```
/.db-migration/
```

`examples/todo-app/.db-migration.yaml`:

```yaml
database:
  url: ${DATABASE_URL}
migrations:
  dir: ./migrations
  package: migrations
  table: schema_migrations
fresh:
  allow: false
```

- [ ] **Step 2: Write the first migration (querybuilder showcase)**

`examples/todo-app/migrations/1782900000_create_todos.go`:

```go
package migrations

import (
	"context"

	"github.com/jorgejr568/pigration/migrator"
	"github.com/jorgejr568/pigration/querybuilder"
)

func CreateTodos1782900000Up(ctx context.Context, tx migrator.Executor) error {
	return querybuilder.CreateTable("todos").
		ID("id", querybuilder.BigSerial).
		Column("title", querybuilder.Text, querybuilder.NotNull()).
		Column("done", querybuilder.Bool, querybuilder.NotNull(), querybuilder.Default(false)).
		Timestamps().
		Execute(ctx, tx)
}

func CreateTodos1782900000Down(ctx context.Context, tx migrator.Executor) error {
	return querybuilder.DropTable("todos").IfExists().Execute(ctx, tx)
}

func init() {
	migrator.Register("1782900000_create_todos",
		CreateTodos1782900000Up, CreateTodos1782900000Down)
}
```

- [ ] **Step 3: Write the failing test (testPool helper + table exists)**

`examples/todo-app/todo/store_test.go`:

```go
package todo

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jorgejr568/pigration/migrator"

	_ "github.com/jorgejr568/pigration/examples/todo-app/migrations"
)

// testPool connects to TEST_DATABASE_URL (skipping without it) and resets the
// database with migrator.Fresh so every test starts from freshly applied
// migrations — dogfooding the engine as the test fixture.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run integration tests")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := migrator.Fresh(context.Background(), pool, migrator.AllowFresh()); err != nil {
		t.Fatalf("fresh: %v", err)
	}
	return pool
}

func TestMigrationsCreateTodosTable(t *testing.T) {
	pool := testPool(t)
	var n int
	err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM information_schema.tables WHERE table_name = 'todos'`).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("todos table not created (count=%d)", n)
	}
}
```

- [ ] **Step 4: Tidy and run to verify it fails without the migration applied... then passes**

```sh
cd examples/todo-app && go mod tidy
TEST_DATABASE_URL="postgres://postgres:pw@127.0.0.1:5433/pigration_test?sslmode=disable" \
  go test ./todo/ -run TestMigrationsCreateTodosTable -v
```

Expected: PASS (the test drives Fresh which applies the migration — the "failing first" evidence here is running it BEFORE writing Step 2's migration file if you want strict TDD ordering; acceptable either way since the assertion is on real DB state). Also run `go vet ./...` and `gofmt -l .` inside the example — clean.

- [ ] **Step 5: Commit**

```sh
git add examples/todo-app
git commit -m "feat(examples): todo-app scaffold, first migration, test fixture"
```

---

## Task 2: Raw-SQL and NonTransactional migrations

**Files:**
- Create: `examples/todo-app/migrations/1782900100_add_due_date.go`, `examples/todo-app/migrations/1782900200_add_pending_index.go`
- Modify: `examples/todo-app/todo/store_test.go` (add one test)

**Interfaces:**
- Consumes: `migrator.NonTransactional`, `querybuilder.CreateIndex/DropIndex`.
- Produces: registered `1782900100_add_due_date` (adds nullable `due_date date`) and `1782900200_add_pending_index` (partial index `idx_todos_pending`).

- [ ] **Step 1: Write the failing test**

Append to `store_test.go`:

```go
func TestMigrationsAddDueDateAndPendingIndex(t *testing.T) {
	pool := testPool(t)
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM information_schema.columns
		 WHERE table_name = 'todos' AND column_name = 'due_date'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatal("due_date column missing")
	}
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM pg_indexes WHERE indexname = 'idx_todos_pending'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatal("idx_todos_pending missing")
	}
}
```

- [ ] **Step 2: Run to verify it fails** — same `go test` command, `-run TestMigrationsAddDueDate` → FAIL ("due_date column missing").

- [ ] **Step 3: Write the two migrations**

`examples/todo-app/migrations/1782900100_add_due_date.go` (raw-SQL style):

```go
package migrations

import (
	"context"

	"github.com/jorgejr568/pigration/migrator"
)

func AddDueDate1782900100Up(ctx context.Context, tx migrator.Executor) error {
	_, err := tx.Exec(ctx, `ALTER TABLE todos ADD COLUMN due_date date;`)
	return err
}

func AddDueDate1782900100Down(ctx context.Context, tx migrator.Executor) error {
	_, err := tx.Exec(ctx, `ALTER TABLE todos DROP COLUMN due_date;`)
	return err
}

func init() {
	migrator.Register("1782900100_add_due_date",
		AddDueDate1782900100Up, AddDueDate1782900100Down)
}
```

`examples/todo-app/migrations/1782900200_add_pending_index.go` (NonTransactional + Concurrently):

```go
package migrations

import (
	"context"

	"github.com/jorgejr568/pigration/migrator"
	"github.com/jorgejr568/pigration/querybuilder"
)

func AddPendingIndex1782900200Up(ctx context.Context, tx migrator.Executor) error {
	return querybuilder.CreateIndex("idx_todos_pending").
		On("todos").Columns("done").
		Where("done = false").
		Concurrently().
		Execute(ctx, tx)
}

func AddPendingIndex1782900200Down(ctx context.Context, tx migrator.Executor) error {
	return querybuilder.DropIndex("idx_todos_pending").IfExists().Concurrently().Execute(ctx, tx)
}

func init() {
	// CREATE INDEX CONCURRENTLY cannot run inside a transaction.
	migrator.Register("1782900200_add_pending_index",
		AddPendingIndex1782900200Up, AddPendingIndex1782900200Down,
		migrator.NonTransactional())
}
```

- [ ] **Step 4: Run to verify it passes** — both tests PASS. **If `Fresh` + NonTransactional + Concurrently misbehaves here, that is a pigration bug: apply the Global Constraints bug-fix rule.**

- [ ] **Step 5: Commit** — `git add examples/todo-app/migrations examples/todo-app/todo && git commit -m "feat(examples): due_date and concurrent partial index migrations"`

---

## Task 3: Todo store

**Files:**
- Create: `examples/todo-app/todo/store.go`
- Modify: `examples/todo-app/todo/store_test.go`

**Interfaces:**
- Produces (used verbatim by Task 4):

```go
type Todo struct {
	ID        int64      `json:"id"`
	Title     string     `json:"title"`
	Done      bool       `json:"done"`
	DueDate   *time.Time `json:"due_date"`
	CreatedAt time.Time  `json:"created_at"`
}
var ErrNotFound = errors.New("todo not found")
type Store struct{ pool *pgxpool.Pool }
func NewStore(pool *pgxpool.Pool) *Store
func (s *Store) List(ctx context.Context) ([]Todo, error)          // ordered by id
func (s *Store) Create(ctx context.Context, title string) (Todo, error)
func (s *Store) Toggle(ctx context.Context, id int64) (Todo, error) // ErrNotFound if absent
func (s *Store) Delete(ctx context.Context, id int64) error         // ErrNotFound if absent
```

- [ ] **Step 1: Write the failing test**

Append to `store_test.go`:

```go
func TestStoreCRUD(t *testing.T) {
	pool := testPool(t)
	s := NewStore(pool)
	ctx := context.Background()

	created, err := s.Create(ctx, "write the example")
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == 0 || created.Title != "write the example" || created.Done {
		t.Fatalf("unexpected created todo: %+v", created)
	}

	toggled, err := s.Toggle(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !toggled.Done {
		t.Fatal("toggle did not flip done")
	}

	list, err := s.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("unexpected list: %+v", list)
	}

	if err := s.Delete(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Toggle(ctx, created.ID); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
	if err := s.Delete(ctx, created.ID); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound on double delete, got %v", err)
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `-run TestStoreCRUD` → FAIL (undefined `NewStore`).

- [ ] **Step 3: Implement the store**

`examples/todo-app/todo/store.go`:

```go
// Package todo is the storage and HTTP layer of the pigration example app.
package todo

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Todo struct {
	ID        int64      `json:"id"`
	Title     string     `json:"title"`
	Done      bool       `json:"done"`
	DueDate   *time.Time `json:"due_date"`
	CreatedAt time.Time  `json:"created_at"`
}

var ErrNotFound = errors.New("todo not found")

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

const todoColumns = `id, title, done, due_date, created_at`

func (s *Store) List(ctx context.Context) ([]Todo, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+todoColumns+` FROM todos ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var todos []Todo
	for rows.Next() {
		var t Todo
		if err := rows.Scan(&t.ID, &t.Title, &t.Done, &t.DueDate, &t.CreatedAt); err != nil {
			return nil, err
		}
		todos = append(todos, t)
	}
	return todos, rows.Err()
}

func (s *Store) Create(ctx context.Context, title string) (Todo, error) {
	var t Todo
	err := s.pool.QueryRow(ctx,
		`INSERT INTO todos (title) VALUES ($1) RETURNING `+todoColumns, title).
		Scan(&t.ID, &t.Title, &t.Done, &t.DueDate, &t.CreatedAt)
	return t, err
}

func (s *Store) Toggle(ctx context.Context, id int64) (Todo, error) {
	var t Todo
	err := s.pool.QueryRow(ctx,
		`UPDATE todos SET done = NOT done WHERE id = $1 RETURNING `+todoColumns, id).
		Scan(&t.ID, &t.Title, &t.Done, &t.DueDate, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Todo{}, ErrNotFound
	}
	return t, err
}

func (s *Store) Delete(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM todos WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
```

- [ ] **Step 4: Run to verify it passes** — `-run TestStoreCRUD` → PASS.

- [ ] **Step 5: Commit** — `git commit -am "feat(examples): todo store with CRUD"`

---

## Task 4: HTTP handlers + main.go

**Files:**
- Create: `examples/todo-app/todo/handlers.go`, `examples/todo-app/todo/handlers_test.go`, `examples/todo-app/main.go`

**Interfaces:**
- Consumes: Task 3's `Store`/`Todo`/`ErrNotFound`.
- Produces: `func NewServer(pool *pgxpool.Pool) http.Handler` (used by `main.go` and tests).

- [ ] **Step 1: Write the failing test**

`examples/todo-app/todo/handlers_test.go`:

```go
package todo

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlersCRUDWalkthrough(t *testing.T) {
	pool := testPool(t)
	srv := httptest.NewServer(NewServer(pool))
	defer srv.Close()

	post := func(path, body string) *http.Response {
		t.Helper()
		resp, err := http.Post(srv.URL+path, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	// create
	resp := post("/todos", `{"title":"ship the example"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: status %d", resp.StatusCode)
	}
	var created Todo
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	if created.Title != "ship the example" || created.Done {
		t.Fatalf("create: %+v", created)
	}

	// empty title rejected
	resp = post("/todos", `{"title":"  "}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty title: status %d", resp.StatusCode)
	}
	resp.Body.Close()

	// toggle
	resp = post("/todos/"+jsonID(created.ID)+"/toggle", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("toggle: status %d", resp.StatusCode)
	}
	var toggled Todo
	json.NewDecoder(resp.Body).Decode(&toggled)
	resp.Body.Close()
	if !toggled.Done {
		t.Fatal("toggle did not flip done")
	}

	// list
	resp, err := http.Get(srv.URL + "/todos")
	if err != nil {
		t.Fatal(err)
	}
	var list []Todo
	json.NewDecoder(resp.Body).Decode(&list)
	resp.Body.Close()
	if len(list) != 1 {
		t.Fatalf("list: %+v", list)
	}

	// delete + 404s
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/todos/"+jsonID(created.ID), nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: status %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = post("/todos/"+jsonID(created.ID)+"/toggle", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("toggle after delete: status %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = post("/todos/not-a-number/toggle", "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad id: status %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func jsonID(id int64) string { return strconv.FormatInt(id, 10) }
```

(Add `"strconv"` to the imports.)

- [ ] **Step 2: Run to verify it fails** — `-run TestHandlersCRUD` → FAIL (undefined `NewServer`).

- [ ] **Step 3: Implement handlers**

`examples/todo-app/todo/handlers.go`:

```go
package todo

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewServer wires the todo API onto a stdlib mux (Go 1.22 method+path routing).
func NewServer(pool *pgxpool.Pool) http.Handler {
	h := &handlers{store: NewStore(pool)}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /todos", h.list)
	mux.HandleFunc("POST /todos", h.create)
	mux.HandleFunc("POST /todos/{id}/toggle", h.toggle)
	mux.HandleFunc("DELETE /todos/{id}", h.delete)
	return mux
}

type handlers struct{ store *Store }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be an integer")
		return 0, false
	}
	return id, true
}

func (h *handlers) list(w http.ResponseWriter, r *http.Request) {
	todos, err := h.store.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if todos == nil {
		todos = []Todo{}
	}
	writeJSON(w, http.StatusOK, todos)
}

func (h *handlers) create(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	todo, err := h.store.Create(r.Context(), in.Title)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, todo)
}

func (h *handlers) toggle(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	todo, err := h.store.Toggle(r.Context(), id)
	switch {
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "todo not found")
	case err != nil:
		writeError(w, http.StatusInternalServerError, err.Error())
	default:
		writeJSON(w, http.StatusOK, todo)
	}
}

func (h *handlers) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	err := h.store.Delete(r.Context(), id)
	switch {
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "todo not found")
	case err != nil:
		writeError(w, http.StatusInternalServerError, err.Error())
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
```

- [ ] **Step 4: Run to verify it passes** — `-run TestHandlersCRUD` → PASS.

- [ ] **Step 5: Write `main.go` (library-embedding showcase)**

`examples/todo-app/main.go`:

```go
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

	"github.com/jorgejr568/pigration/examples/todo-app/todo"
	_ "github.com/jorgejr568/pigration/examples/todo-app/migrations" // register migrations
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
```

- [ ] **Step 6: Verify the whole example module** —

```sh
cd examples/todo-app && go build ./... && go vet ./... && \
TEST_DATABASE_URL="postgres://postgres:pw@127.0.0.1:5433/pigration_test?sslmode=disable" \
  go test -p 1 -count=1 ./...
```

Expected: build/vet clean, all tests PASS.

- [ ] **Step 7: Commit** — `git add examples/todo-app && git commit -m "feat(examples): HTTP handlers and boot-migrating main"`

---

## Task 5: CLI dogfood pass (and fix pigration if anything breaks)

No new files — this task exercises the **pigration CLI from inside `examples/todo-app`** exactly as a user would, because the example's own `go.mod` is what the CLI reads. Prereq: `make db-up` at repo root; build the CLI once: `go build -o /tmp/pigration ./cmd/pigration` (repo root).

- [ ] **Step 1: `make` scaffolds into the example** —

```sh
cd examples/todo-app
/tmp/pigration make "add priority"
ls migrations/*_add_priority.go       # exists
```

Then delete it: `rm migrations/*_add_priority.go` (scaffold-only check).

- [ ] **Step 2: migrate / status / rollback round-trip** —

```sh
export DATABASE_URL="postgres://postgres:pw@127.0.0.1:5433/pigration_test?sslmode=disable"
/tmp/pigration migrate            # applies all 3 (or "nothing to apply" if tests left them)
/tmp/pigration status             # 3 applied, 0 pending
/tmp/pigration rollback           # reverts the last batch
/tmp/pigration status             # shows pending again
/tmp/pigration migrate            # re-apply
```

Expected: each command exits 0 with sensible output; rollback of the NonTransactional index works (`DROP INDEX CONCURRENTLY`).

- [ ] **Step 3: run the app and curl it** —

```sh
go run . &         # uses DATABASE_URL; runs migrator.Up on boot
sleep 2
curl -s localhost:8080/todos                                   # []
curl -s -X POST localhost:8080/todos -d '{"title":"try pigration"}'   # 201 + JSON
curl -s -X POST localhost:8080/todos/1/toggle                  # done:true
curl -s -X DELETE -o /dev/null -w '%{http_code}' localhost:8080/todos/1   # 204
kill %1
```

- [ ] **Step 4: Bug gate.** Any failure in Steps 1–3 caused by pigration (not the example) → apply the Global Constraints bug-fix rule: failing library test, fix, library suite green, separate `fix(...)` commit, then re-run this task.

- [ ] **Step 5: Commit** (only if this task produced changes — e.g. library fixes): library fixes are committed separately in Step 4; otherwise nothing to commit.

---

## Task 6: README, CI, Makefile, CONTRIBUTING

**Files:**
- Create: `examples/todo-app/README.md`
- Modify: `.github/workflows/ci.yml` (vet + test steps for the example), `Makefile` (`test-all`, `lint`), `CONTRIBUTING.md` (layout table)

- [ ] **Step 1: Write `examples/todo-app/README.md`**

```markdown
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
```

- [ ] **Step 2: Extend CI** — in `.github/workflows/ci.yml`, add to the `go vet` step and after the coverage-floor step:

```yaml
      - name: go vet (example)
        run: go vet -C examples/todo-app ./...

      - name: example todo-app tests
        run: go test -C examples/todo-app -p 1 -count=1 -race ./...
```

(Example tests reuse the same service container and `TEST_DATABASE_URL`; they run in their own step, after the root suite, so the schema drops cannot race.)

- [ ] **Step 3: Extend Makefile** — `test-all` and `lint` gain the example module:

```make
test-all:
	TEST_DATABASE_URL="$(DB_URL)" go test -p 1 -count=1 -race ./...
	TEST_DATABASE_URL="$(DB_URL)" go test -C examples/todo-app -p 1 -count=1 -race ./...

lint:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	go vet ./...
	go vet -C examples/todo-app ./...
	@echo lint OK
```

- [ ] **Step 4: CONTRIBUTING layout table** — add the row:

```markdown
| `examples/todo-app/` | Runnable example app (own module) dogfooding the CLI, library boot-migration, and all three migration styles |
```

- [ ] **Step 5: Verify** — `actionlint` clean; from repo root: `make lint`, `make test-all` (both modules green against db-up).

- [ ] **Step 6: Commit & push** —

```sh
git add examples/todo-app/README.md .github/workflows/ci.yml Makefile CONTRIBUTING.md
git commit -m "feat(examples): README, CI and Makefile coverage for todo-app"
git push && gh run watch $(gh run list --limit 1 --json databaseId --jq '.[0].databaseId') --exit-status
```

Expected: CI green on both Postgres matrix jobs including the new example steps.

---

## Self-Review Notes

- Spec coverage: functional app (T3/T4), all three migration styles (T1/T2), CLI usage from the example (T5), "fix bugs right away" (Global Constraints + T5 Step 4 gate), CI keeps the example green (T6). Root `go.mod` untouched; example is its own module with `replace`.
- Type consistency: `NewServer(pool)` produced in T4 and consumed by `main.go`/tests; `testPool` defined in T1, used in T2–T4; store method signatures in T3's interface block match T4's handler usage; migration names/timestamps consistent across T1/T2/T5/README.
- No placeholders: every file's full content is inline.
- Known risk called out where it lives: `Fresh`/NonTransactional interplay (T2 Step 4) and CLI-from-subdirectory module resolution (T5) are the two most likely places to surface a real pigration bug — both have explicit bug gates.
