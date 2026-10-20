// Package testutil provides helpers for integration tests that need a real
// Postgres server. Point TEST_DATABASE_URL at a disposable instance; without
// it the dependent tests are skipped.
package testutil

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

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

// IsLocalURL reports whether a database URL points at this machine; tests
// that create and drop schemas refuse to run elsewhere.
func IsLocalURL(url string) bool {
	return strings.Contains(url, "localhost") || strings.Contains(url, "127.0.0.1") || strings.Contains(url, "@postgres")
}
