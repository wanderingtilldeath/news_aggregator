package store_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/frontier"
	"github.com/wanderingtilldeath/news-aggregator/internal/store"
	"github.com/wanderingtilldeath/news-aggregator/internal/testutil"
)

func page(url, title, body string) store.Page {
	return store.Page{
		URLHash: frontier.Hash(url), CrawlID: 1, URL: url, StatusCode: 200, ContentType: "text/html",
		Title: title, Snippet: body, ContentHash: store.ContentHash([]byte(title + body)),
		ETag: `"` + title + `"`, SizeBytes: len(body), Latency: 12 * time.Millisecond,
	}
}

func TestPostgresSaveAndSearch(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	ctx := context.Background()

	for _, p := range []store.Page{
		page("http://a.test/go", "Go concurrency", "goroutines and channels make concurrent programs simple"),
		page("http://a.test/pg", "Postgres queues", "SKIP LOCKED makes a work queue in a database"),
		page("http://a.test/cook", "Pancake recipe", "flour eggs and milk"),
	} {
		if err := s.Save(ctx, p); err != nil {
			t.Fatal(err)
		}
	}

	hits, err := s.Search(ctx, "concurrent goroutines", 10)
	if err != nil || len(hits) != 1 || hits[0].URL != "http://a.test/go" {
		t.Fatalf("Search = %+v, %v", hits, err)
	}

	// Stemming: "queues" matches "queue".
	hits, _ = s.Search(ctx, "queue", 10)
	if len(hits) != 1 || hits[0].URL != "http://a.test/pg" {
		t.Fatalf("stemmed Search = %+v", hits)
	}

	if hits, _ := s.Search(ctx, "nonexistentword", 10); len(hits) != 0 {
		t.Fatalf("unexpected hits: %+v", hits)
	}

	// websearch syntax: a leading minus excludes.
	hits, _ = s.Search(ctx, "database -queue", 10)
	if len(hits) != 0 {
		t.Fatalf("exclusion failed: %+v", hits)
	}
}

func TestPostgresTitleOutranksSnippet(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	ctx := context.Background()

	s.Save(ctx, page("http://a.test/body", "Unrelated", "this page mentions kubernetes once"))
	s.Save(ctx, page("http://a.test/title", "Kubernetes", "short"))

	hits, err := s.Search(ctx, "kubernetes", 10)
	if err != nil || len(hits) != 2 || hits[0].URL != "http://a.test/title" {
		t.Fatalf("hits = %+v, %v; the title match should rank first", hits, err)
	}
}

func TestPostgresValidatorsAndUnchangedContent(t *testing.T) {
	pool := testutil.Postgres(t)
	s := store.NewPostgres(pool)
	defer s.Close()
	ctx := context.Background()

	p := page("http://a.test/x", "First", "body")
	p.LastModified = "Mon, 02 Jan 2006 15:04:05 GMT"
	if err := s.Save(ctx, p); err != nil {
		t.Fatal(err)
	}

	v, ok, err := s.Validators(ctx, p.URLHash)
	if err != nil || !ok || v.ETag != p.ETag || v.LastModified != p.LastModified {
		t.Fatalf("Validators = %+v, %v, %v", v, ok, err)
	}
	if _, ok, _ := s.Validators(ctx, frontier.Hash("http://a.test/none")); ok {
		t.Fatal("Validators for an unknown URL should be !ok")
	}

	var fetchedAt time.Time
	pool.QueryRow(ctx, `SELECT fetched_at FROM pages WHERE url_hash=$1`, p.URLHash).Scan(&fetchedAt)

	// Same content again: the row is not rewritten.
	time.Sleep(20 * time.Millisecond)
	s.Save(ctx, p)
	var again time.Time
	pool.QueryRow(ctx, `SELECT fetched_at FROM pages WHERE url_hash=$1`, p.URLHash).Scan(&again)
	if !again.Equal(fetchedAt) {
		t.Fatal("unchanged content rewrote the row")
	}

	// Changed content: updated in place.
	changed := p
	changed.Title, changed.ContentHash = "Second", store.ContentHash([]byte("different"))
	s.Save(ctx, changed)

	var title string
	var count int
	pool.QueryRow(ctx, `SELECT title FROM pages WHERE url_hash=$1`, p.URLHash).Scan(&title)
	pool.QueryRow(ctx, `SELECT count(*) FROM pages`).Scan(&count)
	if title != "Second" || count != 1 {
		t.Fatalf("title=%q count=%d; want an in-place update", title, count)
	}
}

func TestPostgresConcurrentSavesAreBatched(t *testing.T) {
	pool := testutil.Postgres(t)
	s := store.NewPostgres(pool)
	defer s.Close()
	ctx := context.Background()

	const n = 200
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.Save(ctx, page(fmt.Sprintf("http://a.test/%d", i), fmt.Sprint("t", i), "body")); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()

	// Save returns only after commit, so every row is visible right away.
	var count int
	pool.QueryRow(ctx, `SELECT count(*) FROM pages`).Scan(&count)
	if count != n {
		t.Fatalf("%d rows after %d saves", count, n)
	}
}

func TestPostgresSaveAfterCloseFails(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	s.Close()

	if err := s.Save(context.Background(), page("http://a.test/x", "t", "b")); err == nil {
		t.Fatal("Save after Close should fail")
	}
}

func TestContentHashIsStable(t *testing.T) {
	first, again := store.ContentHash([]byte("a")), store.ContentHash([]byte("a"))
	if first != again || first == store.ContentHash([]byte("b")) {
		t.Fatal("ContentHash must be deterministic and distinguish inputs")
	}
}
