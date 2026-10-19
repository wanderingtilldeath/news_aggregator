# Integration tests need Postgres + Redis: `make up` first, or point these elsewhere.
export TEST_DATABASE_URL ?= postgres://crawlerx:crawlerx@localhost:5432/crawlerx?sslmode=disable
export TEST_REDIS_URL ?= redis://localhost:6379/0

.PHONY: up down build test test-short test-integration lint bench fuzz

up:
	docker compose up --build -d
	@echo "API: http://localhost:8081  (start a crawl: curl -XPOST localhost:8081/crawls -d '{\"seed\":\"http://fakesite:9000/\",\"max_depth\":6}')"

down:
	docker compose down -v

build:
	go build ./...

test-short:
	go test -short -race ./...

# -p 1: integration packages share one Redis and must not run in parallel.
test test-integration:
	go test -race -p 1 ./...

lint:
	go vet ./...
	go run honnef.co/go/tools/cmd/staticcheck@latest ./...

fuzz:
	go test ./internal/urlnorm -run xxx -fuzz FuzzCanonicalize -fuzztime 30s

bench:
	go run ./cmd/bench -database-url "$(TEST_DATABASE_URL)" -redis-url "$(TEST_REDIS_URL)" -procs 1,2 -pages 500 -hosts 32 -out docs/BENCHMARK.table.md
