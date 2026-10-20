# Integration tests need a disposable Postgres: TEST_DATABASE_URL=postgres://... make test
export TEST_DATABASE_URL ?=

.PHONY: build build-linux test test-short lint fuzz

build:
	go build ./...

# Static binary for a Linux server: scp bin/crawlerx and sources.json to it.
# GOARCH=arm64 make build-linux for an ARM server.
build-linux:
	GOOS=linux GOARCH=$${GOARCH:-amd64} CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/crawlerx ./cmd/crawlerx

test-short:
	go test -short -race ./...

# -p 1: integration packages share one database and must not run in parallel.
test:
	go test -race -p 1 ./...

lint:
	go vet ./...
	go run honnef.co/go/tools/cmd/staticcheck@latest ./...

fuzz:
	go test ./internal/urlnorm -run xxx -fuzz FuzzCanonicalize -fuzztime 30s
