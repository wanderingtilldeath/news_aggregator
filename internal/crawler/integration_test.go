package crawler

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/fakesite"
	"github.com/wanderingtilldeath/news-aggregator/internal/fetch"
	"github.com/wanderingtilldeath/news-aggregator/internal/frontier"
	"github.com/wanderingtilldeath/news-aggregator/internal/politeness"
	"github.com/wanderingtilldeath/news-aggregator/internal/robots"
	"github.com/wanderingtilldeath/news-aggregator/internal/store"
	"github.com/wanderingtilldeath/news-aggregator/internal/testutil"
)

// TestIntegration_ChaosSiteOnRealPostgres crawls the fake site's
// misbehaving endpoints with the production Postgres frontier, Postgres store
// and in-process gate, and checks every failure mode lands
// in the right terminal state.
func TestIntegration_ChaosSiteOnRealPostgres(t *testing.T) {
	pool := testutil.Postgres(t)

	site := fakesite.New(fakesite.Options{Pages: 30, Fanout: 3, Chaos: true})
	ts := httptest.NewServer(site)
	defer ts.Close()

	front := frontier.NewPostgres(pool)
	pages := store.NewPostgres(pool)
	defer pages.Close()

	c := New(Config{
		Workers:       8,
		PollInterval:  5 * time.Millisecond,
		HostDelay:     5 * time.Millisecond,
		MaxAttempts:   4,
		Backoff:       fetch.Backoff{Base: 5 * time.Millisecond, Max: 50 * time.Millisecond},
		ShutdownGrace: time.Second,
	}, Deps{
		Frontier: front,
		Fetcher:  fetch.New(fetch.Options{Timeout: 5 * time.Second, MaxBytes: 1 << 20}),
		Store:    pages,
		Gate:     politeness.NewMemory(nil),
		Robots:   robots.New(robots.Options{Product: "CrawlerX"}),
		Logger:   discardLogger(),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	crawl, err := c.Crawl(ctx, ts.URL+"/", 6, nil)
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}

	status := func(path string) string {
		var s string
		err := pool.QueryRow(ctx, `SELECT status FROM frontier WHERE crawl_id=$1 AND url_hash=$2`,
			crawl.ID, frontier.Hash(ts.URL+path)).Scan(&s)
		if err != nil {
			t.Fatalf("no frontier row for %s: %v", path, err)
		}
		return s
	}

	want := map[string]string{
		"/":               "done",
		"/p/1":            "done",
		"/private/secret": "blocked", // robots.txt Disallow
		"/x/flaky/1":      "done",    // 503, 503, then 200
		"/x/flaky/2":      "done",
		"/x/ratelimit":    "done",    // 429 + Retry-After, then 200
		"/x/redirect":     "skipped", // 301 -> /p/1 (already crawled)
		"/x/loop/a":       "skipped", // redirect loop terminates
		"/x/loop/b":       "skipped",
		"/x/huge":         "done",    // truncated at the size cap
		"/x/pdf":          "skipped", // not HTML
		"/x/gone":         "failed",  // 410 is permanent
		"/x/error":        "dead",    // 500 forever -> dead letter
		"/x/noindex":      "skipped",
		"/x/nofollow":     "done",
	}
	for path, wantStatus := range want {
		if got := status(path); got != wantStatus {
			t.Errorf("%s: status %q, want %q", path, got, wantStatus)
		}
	}

	// Pages: noindex and non-HTML are not stored, the huge page is capped.
	var size int
	if err := pool.QueryRow(ctx, `SELECT size_bytes FROM pages WHERE url_hash=$1`, frontier.Hash(ts.URL+"/x/huge")).Scan(&size); err != nil {
		t.Fatalf("huge page not stored: %v", err)
	}
	if size != 1<<20 {
		t.Errorf("huge page stored %d bytes, want the 1 MiB cap", size)
	}

	var n int
	pool.QueryRow(ctx, `SELECT count(*) FROM pages WHERE url_hash = ANY($1)`,
		[]string{frontier.Hash(ts.URL + "/x/noindex"), frontier.Hash(ts.URL + "/x/pdf")}).Scan(&n)
	if n != 0 {
		t.Errorf("%d noindex/non-HTML pages were stored", n)
	}

	// /x/error: exactly MaxAttempts requests, then dead-lettered.
	errRequests := 0
	for _, r := range site.Requests() {
		if r.Path == "/x/error" {
			errRequests++
		}
	}
	if errRequests != 4 {
		t.Errorf("/x/error requested %d times, want MaxAttempts=4", errRequests)
	}

	// robots.txt fetched once, then served from the cache.
	robotsRequests := 0
	for _, r := range site.Requests() {
		if r.Path == "/robots.txt" {
			robotsRequests++
		}
	}
	if robotsRequests != 1 {
		t.Errorf("robots.txt fetched %d times, want 1", robotsRequests)
	}

	// Everything published is searchable.
	if hits, _, err := pages.Search(ctx, "synthetic page", 5, 0); err != nil || len(hits) == 0 {
		t.Errorf("search returned %v, %v", hits, err)
	}
}
