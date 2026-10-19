// Package testutil provides helpers for integration tests that need real
// Postgres and Redis servers. Point TEST_DATABASE_URL and TEST_REDIS_URL at
// disposable instances (CI uses service containers, `make test-integration`
// uses docker compose); without them the dependent tests are skipped.
package testutil

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/wanderingtilldeath/news-aggregator/internal/db"
)

// Postgres returns a pool on a freshly created, migrated schema that is
// dropped when the test ends, so tests never see each other's rows.
func Postgres(t *testing.T) *pgxpool.Pool {
	t.Helper()

	pool, _ := PostgresWithURL(t)
	return pool
}

// PostgresWithURL is Postgres plus a connection URL for the same schema, for
// tests that start crawlerx as a subprocess.
func PostgresWithURL(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()

	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	ctx := context.Background()
	schema := fmt.Sprintf("t_%d", time.Now().UnixNano())

	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect with schema: %v", err)
	}

	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
		admin.Close()
	})

	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}

	return pool, url + sep + "search_path=" + schema
}

// Redis returns a client on the test Redis. Every "crawlerx:*" key is removed
// when the test ends, so run packages that use it one at a time (go test -p 1,
// as the Makefile does) so they do not delete each other's keys.
func Redis(t *testing.T) *redis.Client {
	t.Helper()

	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		t.Skip("TEST_REDIS_URL not set")
	}

	opts, err := redis.ParseURL(url)
	if err != nil {
		t.Fatal(err)
	}

	client := redis.NewClient(opts)
	t.Cleanup(func() {
		ctx := context.Background()
		keys, _ := client.Keys(ctx, "crawlerx:*").Result()
		if len(keys) > 0 {
			client.Del(ctx, keys...)
		}
		client.Close()
	})

	return client
}

// IsLocalURL reports whether a database URL points at this machine; tests
// that create and drop schemas refuse to run elsewhere.
func IsLocalURL(url string) bool {
	return strings.Contains(url, "localhost") || strings.Contains(url, "127.0.0.1") || strings.Contains(url, "@postgres")
}
