# Development helpers. Integration tests are guarded by TEST_DATABASE_URL and
# skip without it; `make db-up` + `make test-all` runs everything locally.

DB_CONTAINER := pigration-pg
DB_PORT      := 5433
DB_URL       := postgres://postgres:pw@127.0.0.1:$(DB_PORT)/pigration_test?sslmode=disable

.PHONY: test test-all lint cover db-up db-down

## test: unit tests only (integration tests skip without TEST_DATABASE_URL)
test:
	go test ./...

## test-all: full suite (unit + integration) against the db-up container.
## -p 1 serializes package test binaries: they share one database and the
## migrator/CLI tests DROP SCHEMA public for isolation.
test-all:
	TEST_DATABASE_URL="$(DB_URL)" go test -p 1 -count=1 -race ./...

## lint: gofmt + go vet
lint:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	go vet ./...
	@echo lint OK

## cover: full-suite coverage report (opens the per-function breakdown)
cover:
	TEST_DATABASE_URL="$(DB_URL)" go test -p 1 -count=1 -coverprofile=cover.out ./...
	go tool cover -func=cover.out | tail -1

## db-up: throwaway Postgres 16 for the integration tests
db-up:
	docker run -d --name $(DB_CONTAINER) -e POSTGRES_PASSWORD=pw \
		-e POSTGRES_DB=pigration_test -p $(DB_PORT):5432 postgres:16
	@until docker exec $(DB_CONTAINER) pg_isready -U postgres >/dev/null 2>&1; do sleep 1; done
	@echo "postgres ready on :$(DB_PORT)"

## db-down: remove the throwaway Postgres
db-down:
	docker rm -f $(DB_CONTAINER)
