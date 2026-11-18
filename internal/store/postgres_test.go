package store_test

import (
	"context"
	"fmt"
	"os"
	"strings"
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

	hits, _, err := s.Search(ctx, "concurrent goroutines", 10, 0)
	if err != nil || len(hits) != 1 || hits[0].URL != "http://a.test/go" {
		t.Fatalf("Search = %+v, %v", hits, err)
	}

	// Stemming: "queues" matches "queue".
	hits, _, _ = s.Search(ctx, "queue", 10, 0)
	if len(hits) != 1 || hits[0].URL != "http://a.test/pg" {
		t.Fatalf("stemmed Search = %+v", hits)
	}

	if hits, _, _ := s.Search(ctx, "nonexistentword", 10, 0); len(hits) != 0 {
		t.Fatalf("unexpected hits: %+v", hits)
	}

	// websearch syntax: a leading minus excludes.
	hits, _, _ = s.Search(ctx, "database -queue", 10, 0)
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

	hits, _, err := s.Search(ctx, "kubernetes", 10, 0)
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

func TestPostgresDigest(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	ctx := context.Background()

	old, recent := page("http://a.test/old", "Old budget news", "budget"), page("http://a.test/new", "New budget news", "budget")
	old.Published = time.Now().AddDate(0, 0, -10)
	recent.Published = time.Now().Add(-time.Hour)
	undated := page("http://a.test/undated", "Undated exam notice", "exam")

	for _, p := range []store.Page{old, recent, undated} {
		if err := s.Save(ctx, p); err != nil {
			t.Fatal(err)
		}
	}

	since := time.Now().AddDate(0, 0, -1)
	items, _, err := s.Digest(ctx, store.DigestQuery{Since: since, Limit: 10})
	if err != nil || len(items) != 2 {
		t.Fatalf("Digest = %+v, %v; want recent + undated (first_seen is now)", items, err)
	}
	if items[0].URL != "http://a.test/undated" && items[0].URL != "http://a.test/new" {
		t.Errorf("unexpected first item %s", items[0].URL)
	}

	items, _, _ = s.Digest(ctx, store.DigestQuery{Since: since, Query: "exam", Limit: 10})
	if len(items) != 1 || items[0].URL != "http://a.test/undated" {
		t.Fatalf("filtered Digest = %+v", items)
	}

	items, _, _ = s.Digest(ctx, store.DigestQuery{Since: time.Now().AddDate(0, 0, -30), Query: "budget", Limit: 10})
	if len(items) != 2 || items[0].URL != "http://a.test/new" {
		t.Fatalf("30-day Digest = %+v, want new before old", items)
	}
}

func TestPostgresDigestFieldsAndSummaries(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	ctx := context.Background()

	article := page("http://news.test/2026/10/07/story", "Story", "body")
	article.IsArticle, article.Image, article.Content = true, "http://news.test/s.jpg", "full story text"
	listing := page("http://news.test/section/", "Section", "links")
	other := page("http://other.test/a", "Other", "body")
	other.IsArticle = true

	for _, p := range []store.Page{article, listing, other} {
		if err := s.Save(ctx, p); err != nil {
			t.Fatal(err)
		}
	}

	since := time.Now().AddDate(0, 0, -1)

	items, _, err := s.Digest(ctx, store.DigestQuery{Since: since, ArticlesOnly: true, Source: "news.test", Limit: 10})
	if err != nil || len(items) != 1 {
		t.Fatalf("Digest = %+v, %v; want only the news.test article", items, err)
	}
	if got := items[0]; got.Image != "http://news.test/s.jpg" || got.Source != "news.test" || !got.IsArticle || len(got.Summary) != 0 {
		t.Errorf("item = %+v", got)
	}

	if all, _, _ := s.Digest(ctx, store.DigestQuery{Since: since, Limit: 10}); len(all) != 3 {
		t.Errorf("unfiltered Digest has %d items, want 3", len(all))
	}

	pending, err := s.PendingSummaries(ctx, since, 10)
	if err != nil || len(pending) != 2 {
		t.Fatalf("PendingSummaries = %+v, %v; want the 2 articles", pending, err)
	}
	for _, u := range pending {
		if u.URL == article.URL && u.Content != "full story text" {
			t.Errorf("pending content = %q", u.Content)
		}
	}

	if err := s.SetSummary(ctx, article.URLHash, []string{"one", "two"}); err != nil {
		t.Fatal(err)
	}
	if pending, _ := s.PendingSummaries(ctx, since, 10); len(pending) != 1 || pending[0].URL != other.URL {
		t.Errorf("after SetSummary pending = %+v, want only other.test", pending)
	}

	// Re-crawling with changed content must not discard the stored summary.
	article.ContentHash = store.ContentHash([]byte("changed"))
	if err := s.Save(ctx, article); err != nil {
		t.Fatal(err)
	}
	items, _, _ = s.Digest(ctx, store.DigestQuery{Since: since, ArticlesOnly: true, Source: "news.test", Limit: 10})
	if len(items) != 1 || len(items[0].Summary) != 2 || items[0].Summary[0] != "one" {
		t.Errorf("summary after re-save = %+v", items)
	}
}

func TestPostgresPendingSummariesFollowTheDigestWindow(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	ctx := context.Background()

	fresh := page("http://a.test/fresh", "Fresh", "body")
	fresh.IsArticle, fresh.Published = true, time.Now().Add(-time.Hour)
	undated := page("http://a.test/undated", "Undated", "body")
	undated.IsArticle = true // first seen now
	old := page("http://a.test/old", "Old promo", "body")
	old.IsArticle, old.Published = true, time.Now().AddDate(-1, 0, 0) // crawled today, published last year

	for _, p := range []store.Page{fresh, undated, old} {
		if err := s.Save(ctx, p); err != nil {
			t.Fatal(err)
		}
	}

	pending, err := s.PendingSummaries(ctx, time.Now().Add(-48*time.Hour), 10)
	if err != nil || len(pending) != 2 {
		t.Fatalf("PendingSummaries = %+v, %v; want fresh and undated, not the year-old page", pending, err)
	}
	for _, u := range pending {
		if u.URL == old.URL {
			t.Errorf("old page was queued for a summary: %+v", u)
		}
	}
}

func TestPostgresSourceImages(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	ctx := context.Background()

	a := page("http://a.test/daily-updates/a", "A", "body")
	a.IsArticle, a.Image, a.SourceURL = true, "http://a.test/logo.jpg", "http://news.test/story-a"
	b := page("http://a.test/daily-updates/b", "B", "body")
	b.IsArticle, b.Image = true, "http://a.test/logo.jpg" // cites nothing
	for _, p := range []store.Page{a, b} {
		if err := s.Save(ctx, p); err != nil {
			t.Fatal(err)
		}
	}

	since := time.Now().Add(-time.Hour)
	jobs, err := s.PendingImages(ctx, since, 10)
	if err != nil || len(jobs) != 1 || jobs[0].URL != a.URL || jobs[0].SourceURL != "http://news.test/story-a" {
		t.Fatalf("PendingImages = %+v, %v; want only the article that cites a source", jobs, err)
	}

	if err := s.SetImage(ctx, a.URLHash, "http://news.test/story-a.jpg"); err != nil {
		t.Fatal(err)
	}
	if jobs, _ := s.PendingImages(ctx, since, 10); len(jobs) != 0 {
		t.Errorf("after SetImage still pending: %+v", jobs)
	}
	items, _, _ := s.Digest(ctx, store.DigestQuery{Since: since, Source: "a.test", Limit: 10})
	imgs := map[string]string{}
	for _, it := range items {
		imgs[it.URL] = it.Image
	}
	if imgs[a.URL] != "http://news.test/story-a.jpg" {
		t.Errorf("fetched image not served: %v", imgs)
	}

	// A re-crawl whose content changed must not put the logo back.
	a.ContentHash = store.ContentHash([]byte("changed"))
	if err := s.Save(ctx, a); err != nil {
		t.Fatal(err)
	}
	items, _, _ = s.Digest(ctx, store.DigestQuery{Since: since, Source: "a.test", Limit: 10})
	for _, it := range items {
		if it.URL == a.URL && it.Image != "http://news.test/story-a.jpg" {
			t.Errorf("re-crawl replaced the fetched image with %q", it.Image)
		}
	}

	// An empty result only marks the lookup done; the page's own image stays.
	if err := s.SetImage(ctx, b.URLHash, ""); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresDigestPaginationSourcesAndGenericImages(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	ctx := context.Background()

	// 12 articles on a.test: 6 share one logo, the rest have their own picture.
	for i := 0; i < 12; i++ {
		p := page(fmt.Sprintf("http://a.test/%02d", i), fmt.Sprintf("Story %02d", i), "body")
		p.IsArticle, p.Published = true, time.Now().Add(-time.Duration(i)*time.Minute)
		p.Image = fmt.Sprintf("http://a.test/pic-%02d.jpg", i)
		if i%2 == 0 {
			p.Image = "http://a.test/logo.png"
		}
		if err := s.Save(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	other := page("http://b.test/x", "Other", "body")
	other.IsArticle, other.Published, other.Image = true, time.Now(), "http://b.test/unique.jpg"
	if err := s.Save(ctx, other); err != nil {
		t.Fatal(err)
	}

	since := time.Now().Add(-time.Hour)

	items, total, err := s.Digest(ctx, store.DigestQuery{Since: since, ArticlesOnly: true, Limit: 5})
	if err != nil || total != 13 || len(items) != 5 {
		t.Fatalf("page 1 = %d items, total %d, %v; want 5 of 13", len(items), total, err)
	}
	items3, total3, _ := s.Digest(ctx, store.DigestQuery{Since: since, ArticlesOnly: true, Limit: 5, Offset: 10})
	if total3 != 13 || len(items3) != 3 {
		t.Errorf("page 3 = %d items, total %d; want 3 of 13", len(items3), total3)
	}
	past, totalPast, _ := s.Digest(ctx, store.DigestQuery{Since: since, ArticlesOnly: true, Limit: 5, Offset: 50})
	if len(past) != 0 || totalPast != 13 {
		t.Errorf("past the end = %d items, total %d; want 0 items and the real total", len(past), totalPast)
	}

	// The logo shared by 6 articles is hidden; unique pictures are kept.
	all, _, _ := s.Digest(ctx, store.DigestQuery{Since: since, ArticlesOnly: true, Limit: 50})
	for _, it := range all {
		if strings.HasSuffix(it.URL, "/00") || strings.HasSuffix(it.URL, "/02") {
			if it.Image != "" {
				t.Errorf("%s still shows the shared logo %q", it.URL, it.Image)
			}
		}
		if strings.HasSuffix(it.URL, "/01") && it.Image != "http://a.test/pic-01.jpg" {
			t.Errorf("%s lost its own picture: %q", it.URL, it.Image)
		}
		if it.ID == "" {
			t.Errorf("%s has no id", it.URL)
		}
	}

	src, err := s.DigestSources(ctx, store.DigestQuery{Since: since, ArticlesOnly: true})
	if err != nil || len(src) != 2 || src[0].Source != "a.test" || src[0].Count != 12 || src[1].Source != "b.test" || src[1].Count != 1 {
		t.Errorf("DigestSources = %+v, %v", src, err)
	}
	// The source list ignores the source filter itself, so the dropdown keeps every option.
	src, _ = s.DigestSources(ctx, store.DigestQuery{Since: since, ArticlesOnly: true, Source: "b.test"})
	if len(src) != 2 {
		t.Errorf("DigestSources with a source filter = %+v, want both hosts", src)
	}

	img, err := s.ImageURL(ctx, other.URLHash)
	if err != nil || img != "http://b.test/unique.jpg" {
		t.Errorf("ImageURL = %q, %v", img, err)
	}
	if img, _ := s.ImageURL(ctx, "nope"); img != "" {
		t.Errorf("ImageURL(missing) = %q", img)
	}
}

func TestPostgresSearchPagination(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	ctx := context.Background()

	for i := 0; i < 23; i++ {
		if err := s.Save(ctx, page(fmt.Sprintf("http://a.test/%02d", i), fmt.Sprintf("Budget story %02d", i), "budget")); err != nil {
			t.Fatal(err)
		}
	}

	hits, total, err := s.Search(ctx, "budget", 10, 0)
	if err != nil || total != 23 || len(hits) != 10 {
		t.Fatalf("page 1 = %d hits, total %d, %v", len(hits), total, err)
	}
	hits3, total3, _ := s.Search(ctx, "budget", 10, 20)
	if total3 != 23 || len(hits3) != 3 {
		t.Errorf("page 3 = %d hits, total %d", len(hits3), total3)
	}
	past, totalPast, _ := s.Search(ctx, "budget", 10, 100)
	if len(past) != 0 || totalPast != 23 {
		t.Errorf("past the end = %d hits, total %d", len(past), totalPast)
	}

	seen := map[string]bool{}
	for off := 0; off < 23; off += 10 {
		page, _, _ := s.Search(ctx, "budget", 10, off)
		for _, h := range page {
			if seen[h.URL] {
				t.Errorf("%s appears on two pages", h.URL)
			}
			seen[h.URL] = true
		}
	}
	if len(seen) != 23 {
		t.Errorf("pages covered %d hits, want 23", len(seen))
	}
}

func TestPostgresBodyImageFallback(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	ctx := context.Background()
	now := time.Now()

	// "real": own og:image. "fallback": shared logo as og:image but a real body picture.
	// "bothshared": shared logo and a shared body picture. "nobody": nothing at all.
	// "same": og:image and body image are the same file (must count once, not twice).
	mk := func(name, og, body string) store.Page {
		p := page("http://a.test/"+name, name, "text")
		p.IsArticle, p.Published, p.Image, p.BodyImage = true, now, og, body
		return p
	}
	pages := []store.Page{
		mk("real", "http://a.test/real.jpg", "http://a.test/real-body.jpg"),
		mk("fallback", "http://a.test/logo.png", "http://a.test/figure-fallback.png"),
		mk("bothshared", "http://a.test/logo.png", "http://a.test/mascot.png"),
		mk("nobody", "", ""),
		mk("same", "http://a.test/same.jpg", "http://a.test/same.jpg"),
	}
	// Four more articles share the logo and the mascot, so each is used by 5 or more.
	for i := 0; i < 4; i++ {
		pages = append(pages, mk(fmt.Sprintf("filler%d", i), "http://a.test/logo.png", "http://a.test/mascot.png"))
	}
	// Three more articles whose og:image equals their body image: 4 articles, not 8 uses.
	for i := 0; i < 3; i++ {
		pages = append(pages, mk(fmt.Sprintf("same%d", i), "http://a.test/same.jpg", "http://a.test/same.jpg"))
	}
	for _, p := range pages {
		if err := s.Save(ctx, p); err != nil {
			t.Fatal(err)
		}
	}

	items, _, err := s.Digest(ctx, store.DigestQuery{Since: now.Add(-time.Hour), ArticlesOnly: true, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, it := range items {
		got[it.Title] = it.Image
		// The proxy must serve exactly what the digest advertises.
		if img, _ := s.ImageURL(ctx, it.ID); img != it.Image {
			t.Errorf("%s: ImageURL = %q but the digest shows %q", it.Title, img, it.Image)
		}
	}

	for name, want := range map[string]string{
		"real":       "http://a.test/real.jpg",
		"fallback":   "http://a.test/figure-fallback.png", // the logo is shared, the figure is not
		"bothshared": "",                                  // both pictures are shared filler
		"nobody":     "",
		"same":       "http://a.test/same.jpg", // 4 articles use it: below the threshold of 5
	} {
		if got[name] != want {
			t.Errorf("%s: image = %q, want %q", name, got[name], want)
		}
	}
}

func TestPostgresDigestFiltersByCategoryAndBackfillsIt(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	ctx := context.Background()

	save := func(url, category string) {
		p := page(url, "Story", "text")
		p.IsArticle, p.Published, p.Category = true, time.Now(), category
		if err := s.Save(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	save("http://a.test/1", "tech")
	save("http://a.test/2", "upsc")
	save("http://a.test/3", "")

	since := time.Now().Add(-time.Hour)
	for cat, want := range map[string]int{"": 3, "tech": 1, "upsc": 1, "other": 1, "ai": 0} {
		if _, total, err := s.Digest(ctx, store.DigestQuery{Since: since, Category: cat, ArticlesOnly: true}); err != nil || total != want {
			t.Errorf("category %q: %d items, %v; want %d", cat, total, err, want)
		}
	}

	// Same content, new category: the row is updated, so old rows pick one up.
	save("http://a.test/3", "ai")
	if _, total, _ := s.Digest(ctx, store.DigestQuery{Since: since, Category: "ai", ArticlesOnly: true}); total != 1 {
		t.Errorf("recrawled page not moved to ai: %d", total)
	}

	sources, err := s.DigestSources(ctx, store.DigestQuery{Since: since, ArticlesOnly: true})
	if err != nil || len(sources) != 3 {
		t.Errorf("DigestSources = %+v, %v; want one row per category", sources, err)
	}
}

func TestPostgresDigestHidesFutureDatedPages(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	ctx := context.Background()

	for url, when := range map[string]time.Time{"http://a.test/ok": time.Now(), "http://a.test/future": time.Now().Add(96 * time.Hour)} {
		p := page(url, "Story", "text")
		p.IsArticle, p.Published = true, when
		if err := s.Save(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	q := store.DigestQuery{Since: time.Now().Add(-time.Hour), ArticlesOnly: true}
	if _, total, err := s.Digest(ctx, q); err != nil || total != 1 {
		t.Errorf("digest: %d items, %v; want 1", total, err)
	}
	if src, err := s.DigestSources(ctx, q); err != nil || len(src) != 1 || src[0].Count != 1 {
		t.Errorf("sources = %+v, %v; want one row with 1", src, err)
	}
}

func TestPostgresRecrawlCorrectsAWrongPublishDate(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	ctx := context.Background()

	p := page("http://a.test/1", "Story", "text")
	p.IsArticle, p.Published = true, time.Now().Add(96*time.Hour)
	if err := s.Save(ctx, p); err != nil {
		t.Fatal(err)
	}

	// Same content, corrected date: the row must be rewritten.
	p.Published = time.Now().Add(-time.Hour)
	if err := s.Save(ctx, p); err != nil {
		t.Fatal(err)
	}
	if _, total, err := s.Digest(ctx, store.DigestQuery{Since: time.Now().Add(-2 * time.Hour), ArticlesOnly: true}); err != nil || total != 1 {
		t.Errorf("corrected page: %d items, %v; want 1", total, err)
	}
}

func TestPostgresDigestSurvivesAPageWithoutAHost(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	ctx := context.Background()

	for _, url := range []string{"http://a.test/ok", "mailto:someone@a.test"} {
		p := page(url, "Story", "text")
		p.IsArticle, p.Published = true, time.Now()
		if err := s.Save(ctx, p); err != nil {
			t.Fatal(err)
		}
	}

	q := store.DigestQuery{Since: time.Now().Add(-time.Hour), ArticlesOnly: true}
	if items, total, err := s.Digest(ctx, q); err != nil || total != 2 || len(items) != 2 {
		t.Fatalf("Digest = %d items of %d, %v; want both pages", len(items), total, err)
	}
	if sources, err := s.DigestSources(ctx, q); err != nil || len(sources) != 2 {
		t.Fatalf("DigestSources = %+v, %v; want a row per host, one of them empty", sources, err)
	}
}

func TestPostgresSubscribeAndConfirm(t *testing.T) {
	pool := testutil.Postgres(t)
	s := store.NewPostgres(pool)
	defer s.Close()
	ctx := context.Background()

	tok, confirmed, err := s.Subscribe(ctx, "a@x.test", "t1")
	if err != nil || tok != "t1" || confirmed {
		t.Fatalf("new subscriber = %q, %v, %v", tok, confirmed, err)
	}
	if tok, _, _ = s.Subscribe(ctx, "a@x.test", "t2"); tok != "t1" {
		t.Fatalf("a repeat signup must keep the first token, got %q", tok)
	}
	if ok, _ := s.Confirm(ctx, "nope"); ok {
		t.Fatal("unknown token confirmed")
	}
	if ok, _ := s.Confirm(ctx, "t1"); !ok {
		t.Fatal("known token not confirmed")
	}
	if ok, _ := s.Confirm(ctx, "t1"); !ok {
		t.Fatal("Confirm must be safe to repeat")
	}
	if _, confirmed, _ = s.Subscribe(ctx, "a@x.test", "t3"); !confirmed {
		t.Fatal("a confirmed subscriber must stay confirmed")
	}

	// Someone who unsubscribed starts over with the new token.
	if _, err := pool.Exec(ctx, `UPDATE subscribers SET unsubscribed_at = now() WHERE email = 'a@x.test'`); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.Confirm(ctx, "t1"); ok {
		t.Fatal("an unsubscribed token confirmed")
	}
	if tok, confirmed, _ = s.Subscribe(ctx, "a@x.test", "t4"); tok != "t4" || confirmed {
		t.Fatalf("resubscribe = %q, %v, want t4, false", tok, confirmed)
	}
	if ok, _ := s.Confirm(ctx, "t4"); !ok {
		t.Fatal("new token after resubscribe not confirmed")
	}
}

func TestPostgresSubscribeConcurrentSameAddress(t *testing.T) {
	pool := testutil.Postgres(t)
	s := store.NewPostgres(pool)
	defer s.Close()
	ctx := context.Background()

	var wg sync.WaitGroup
	toks := make([]string, 20)
	for i := range toks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			toks[i], _, _ = s.Subscribe(ctx, "race@x.test", fmt.Sprintf("tok-%d", i))
		}()
	}
	wg.Wait()

	for _, tok := range toks {
		if tok != toks[0] || tok == "" {
			t.Fatalf("concurrent signups disagree on the token: %v", toks)
		}
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM subscribers`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("rows = %d, %v, want 1", n, err)
	}
}

func TestPostgresUnsubscribe(t *testing.T) {
	pool := testutil.Postgres(t)
	s := store.NewPostgres(pool)
	defer s.Close()
	ctx := context.Background()

	_, _, _ = s.Subscribe(ctx, "a@x.test", "t1")
	if ok, _ := s.Unsubscribe(ctx, "nope"); ok {
		t.Fatal("unknown token unsubscribed")
	}
	if ok, err := s.Unsubscribe(ctx, "t1"); err != nil || !ok {
		t.Fatalf("known token = %v, %v", ok, err)
	}
	if ok, _ := s.Unsubscribe(ctx, "t1"); !ok {
		t.Fatal("Unsubscribe must be safe to repeat")
	}
	if ok, _ := s.Confirm(ctx, "t1"); ok {
		t.Fatal("an unsubscribed token must not confirm")
	}
	var at *time.Time
	if err := pool.QueryRow(ctx, `SELECT unsubscribed_at FROM subscribers WHERE email = 'a@x.test'`).Scan(&at); err != nil || at == nil {
		t.Fatalf("unsubscribed_at = %v, %v", at, err)
	}
}

func TestPostgresConfirmedSubscribersAndDeliveries(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	ctx := context.Background()

	for _, e := range []string{"a@x.test", "b@x.test", "c@x.test"} {
		_, _, _ = s.Subscribe(ctx, e, "t-"+e)
	}
	_, _ = s.Confirm(ctx, "t-a@x.test")
	_, _ = s.Confirm(ctx, "t-b@x.test")
	_, _ = s.Unsubscribe(ctx, "t-b@x.test")

	subs, err := s.ConfirmedSubscribers(ctx)
	if err != nil || len(subs) != 1 || subs[0].Email != "a@x.test" || subs[0].Token != "t-a@x.test" || subs[0].ID == 0 {
		t.Fatalf("ConfirmedSubscribers = %+v, %v, want only a@x.test", subs, err)
	}

	if err := s.MarkDelivered(ctx, subs[0].ID, "2026-11-14"); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkDelivered(ctx, subs[0].ID, "2026-11-14"); err != nil { // twice is fine
		t.Fatal(err)
	}
	if got, err := s.DeliveredOn(ctx, "2026-11-14"); err != nil || !got[subs[0].ID] || len(got) != 1 {
		t.Fatalf("DeliveredOn = %v, %v", got, err)
	}
	if got, _ := s.DeliveredOn(ctx, "2026-11-15"); len(got) != 0 {
		t.Fatalf("another day = %v, want none", got)
	}
}

func TestPostgresEnrichmentUntilAndDetail(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	ctx := context.Background()

	now := time.Now()
	for i, d := range []time.Duration{time.Hour, 30 * time.Hour} {
		p := page(fmt.Sprintf("http://a.test/%d", i), fmt.Sprintf("Story %d", i), "body")
		p.IsArticle, p.Published, p.Category, p.Content = true, now.Add(-d), "world", "article text"
		if err := s.Save(ctx, p); err != nil {
			t.Fatal(err)
		}
		if err := s.SetSummary(ctx, p.URLHash, []string{"gist"}); err != nil {
			t.Fatal(err)
		}
	}

	pending, err := s.PendingEnrichment(ctx, now.Add(-48*time.Hour), 10)
	if err != nil || len(pending) != 2 || pending[0].Title != "Story 0" || pending[0].Category != "world" {
		t.Fatalf("PendingEnrichment = %+v, %v, want 2 newest first with category", pending, err)
	}
	if err := s.SetEnrichment(ctx, pending[0].URLHash, store.Enrichment{Why: "Why.", Tags: []string{"rbi", "gs3-economy"}, Importance: 5, Long: "Long text."}); err != nil {
		t.Fatal(err)
	}
	if left, _ := s.PendingEnrichment(ctx, now.Add(-48*time.Hour), 10); len(left) != 1 || left[0].Title != "Story 1" {
		t.Fatalf("pending after enriching one = %+v", left)
	}

	items, total, err := s.Digest(ctx, store.DigestQuery{Since: now.Add(-48 * time.Hour), ArticlesOnly: true})
	if err != nil || total != 2 || items[0].Why != "Why." || items[0].Importance != 5 || len(items[0].Tags) != 2 || items[0].Long != "" {
		t.Fatalf("Digest = %+v, %d, %v, want enrichment without the long summary", items, total, err)
	}
	items, _, _ = s.Digest(ctx, store.DigestQuery{Since: now.Add(-48 * time.Hour), ArticlesOnly: true, Detail: true})
	if items[0].Long != "Long text." {
		t.Fatalf("Detail long = %q", items[0].Long)
	}

	// Until keeps one day's edition: only the older story is before 24 hours ago.
	items, total, _ = s.Digest(ctx, store.DigestQuery{Since: now.Add(-48 * time.Hour), Until: now.Add(-24 * time.Hour), ArticlesOnly: true})
	if total != 1 || items[0].Title != "Story 1" {
		t.Fatalf("Until digest = %+v (%d), want only Story 1", items, total)
	}
	srcs, _ := s.DigestSources(ctx, store.DigestQuery{Since: now.Add(-48 * time.Hour), Until: now.Add(-24 * time.Hour), ArticlesOnly: true})
	if len(srcs) != 1 || srcs[0].Count != 1 {
		t.Fatalf("DigestSources with Until = %+v", srcs)
	}
}

func TestPostgresSubscriberPreferences(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	ctx := context.Background()

	for _, e := range []string{"a@x.test", "b@x.test"} {
		tok, _, _ := s.Subscribe(ctx, e, "tok-"+e)
		_, _ = s.Confirm(ctx, tok)
	}
	if ok, err := s.SetPreferences(ctx, "tok-a@x.test", store.Preferences{Tracks: []string{"engineering", "bogus"}, Mode: "7min", Topics: []string{" Monsoon ", "RBI"}}); err != nil || !ok {
		t.Fatalf("SetPreferences = %v, %v", ok, err)
	}
	if ok, _ := s.SetPreferences(ctx, "nope", store.Preferences{}); ok {
		t.Fatal("an unknown token was accepted")
	}

	subs, err := s.ConfirmedSubscribers(ctx)
	if err != nil || len(subs) != 2 {
		t.Fatalf("subscribers = %+v, %v", subs, err)
	}
	if len(subs[0].Tracks) != 1 || subs[0].Tracks[0] != "engineering" || subs[0].Mode != "7min" {
		t.Fatalf("chosen preferences = %+v", subs[0])
	}
	if len(subs[0].Topics) != 2 || subs[0].Topics[0] != "monsoon" || subs[0].Topics[1] != "rbi" {
		t.Fatalf("topics = %q, want them cleaned", subs[0].Topics)
	}
	if len(subs[1].Tracks) != 2 || subs[1].Mode != "1min" || len(subs[1].Topics) != 0 {
		t.Fatalf("defaults = %+v, want every track, 1min and no topics", subs[1])
	}
}

func TestPostgresQuizzes(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	ctx := context.Background()

	if _, ok, err := s.GetQuiz(ctx, "", "upsc"); ok || err != nil {
		t.Fatalf("empty store: ok=%v err=%v", ok, err)
	}
	q := store.Quiz{Day: "2026-11-14", Track: "upsc", Questions: []store.QuizQuestion{{Question: "Q1?", Options: []string{"a", "b", "c", "d"}, Answer: 2, Explanation: "e", URL: "https://a.test/1"}}}
	if err := s.SaveQuiz(ctx, q); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.GetQuiz(ctx, "2026-11-14", "upsc")
	if err != nil || !ok || got.Day != "2026-11-14" || len(got.Questions) != 1 || got.Questions[0].Answer != 2 || got.Questions[0].URL != "https://a.test/1" {
		t.Fatalf("GetQuiz = %+v, %v, %v", got, ok, err)
	}

	// Saving again replaces; the latest day wins when no day is given.
	q.Questions[0].Question = "Q1 changed?"
	_ = s.SaveQuiz(ctx, q)
	_ = s.SaveQuiz(ctx, store.Quiz{Day: "2026-11-16", Track: "upsc", Questions: q.Questions})
	if got, _, _ = s.GetQuiz(ctx, "2026-11-14", "upsc"); got.Questions[0].Question != "Q1 changed?" {
		t.Fatalf("replace failed: %+v", got)
	}
	if got, _, _ = s.GetQuiz(ctx, "", "upsc"); got.Day != "2026-11-16" {
		t.Fatalf("latest = %s, want 2026-11-16", got.Day)
	}
	if _, ok, _ = s.GetQuiz(ctx, "", "engineering"); ok {
		t.Fatal("another track's quiz leaked")
	}
}

func TestPostgresChannelPosts(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	ctx := context.Background()

	if ok, err := s.ChannelPosted(ctx, "2026-11-14", "telegram:@a"); ok || err != nil {
		t.Fatalf("nothing posted yet: %v %v", ok, err)
	}
	if err := s.MarkChannelPosted(ctx, "2026-11-14", "telegram:@a"); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkChannelPosted(ctx, "2026-11-14", "telegram:@a"); err != nil { // twice is fine
		t.Fatal(err)
	}
	if ok, _ := s.ChannelPosted(ctx, "2026-11-14", "telegram:@a"); !ok {
		t.Fatal("post not recorded")
	}
	if ok, _ := s.ChannelPosted(ctx, "2026-11-15", "telegram:@a"); ok {
		t.Fatal("another day counted")
	}
}

func TestPostgresItem(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	ctx := context.Background()

	p := page("https://www.site.test/story", "A story", "body")
	p.IsArticle, p.Category, p.Description, p.Published = true, "world", "Desc", time.Now().Add(-time.Hour)
	if err := s.Save(ctx, p); err != nil {
		t.Fatal(err)
	}
	listing := page("https://www.site.test/section/", "A listing", "body")
	if err := s.Save(ctx, listing); err != nil {
		t.Fatal(err)
	}
	_ = s.SetSummary(ctx, p.URLHash, []string{"Gist."})
	_ = s.SetEnrichment(ctx, p.URLHash, store.Enrichment{Why: "Why.", Tags: []string{"rbi"}, Importance: 4, Long: "Long text."})

	it, ok, err := s.Item(ctx, p.URLHash)
	if err != nil || !ok || it.Title != "A story" || it.Source != "www.site.test" || it.Category != "world" || it.Why != "Why." || it.Importance != 4 || len(it.Summary) != 1 || len(it.Tags) != 1 {
		t.Fatalf("Item = %+v, %v, %v", it, ok, err)
	}
	if it.Long != "" {
		t.Fatalf("a story page must not carry the long summary, got %q", it.Long)
	}
	if _, ok, _ := s.Item(ctx, listing.URLHash); ok {
		t.Fatal("a listing page is not a story")
	}
	if _, ok, err := s.Item(ctx, "missing"); ok || err != nil {
		t.Fatalf("unknown id: ok=%v err=%v", ok, err)
	}
}

func TestPostgresExamLens(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	ctx := context.Background()

	now := time.Now()
	a := page("https://x.test/a", "A", "body")
	a.IsArticle, a.Category, a.Published = true, "india", now.Add(-time.Hour)
	tech := page("https://x.test/t", "T", "body")
	tech.IsArticle, tech.Category, tech.Published = true, "tech", now.Add(-2*time.Hour)
	for _, p := range []store.Page{a, tech} {
		if err := s.Save(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	lens := &store.ExamLens{Prelims: 3, Mains: 2, Essay: 1, Dimensions: []string{"political"}, Facts: []string{"f1", "f2"}, For: []string{"for"}, Against: []string{"against"}, Skeleton: []string{"s"}}
	if err := s.SetEnrichment(ctx, a.URLHash, store.Enrichment{Why: "w", Importance: 3, Exam: lens}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetEnrichment(ctx, tech.URLHash, store.Enrichment{Why: "w", Importance: 3}); err != nil {
		t.Fatal(err)
	}

	items, _, err := s.Digest(ctx, store.DigestQuery{Since: now.Add(-24 * time.Hour), ArticlesOnly: true})
	if err != nil || len(items) != 2 {
		t.Fatalf("Digest = %+v, %v", items, err)
	}
	for _, it := range items {
		switch it.ID {
		case a.URLHash:
			if fmt.Sprint(it.Rel) != "[3 2 1 0]" || it.Exam != nil {
				t.Fatalf("UPSC item = rel %v exam %v, want [3 2 1 0] and no full lens in a list", it.Rel, it.Exam)
			}
		case tech.URLHash:
			if it.Rel != nil {
				t.Fatalf("tech item has rel %v", it.Rel)
			}
		}
	}

	got, ok, err := s.Item(ctx, a.URLHash)
	if err != nil || !ok || got.Exam == nil || len(got.Exam.Facts) != 2 || got.Exam.Dimensions[0] != "political" || got.Exam.Against[0] != "against" || fmt.Sprint(got.Rel) != "[3 2 1 0]" {
		t.Fatalf("Item = %+v, %v, %v, want the full lens", got, ok, err)
	}
	if plain, _, _ := s.Item(ctx, tech.URLHash); plain.Exam != nil || plain.Rel != nil {
		t.Fatalf("a story without a lens: %+v", plain)
	}
}

func TestPostgresDetailStage(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	ctx := context.Background()

	now := time.Now()
	var hashes []string
	for i, imp := range []int{2, 5, 4} {
		p := page(fmt.Sprintf("https://x.test/%d", i), fmt.Sprintf("Story %d", i), "body")
		p.IsArticle, p.Category, p.Published = true, "india", now.Add(-time.Duration(i)*time.Minute)
		if err := s.Save(ctx, p); err != nil {
			t.Fatal(err)
		}
		if err := s.SetEnrichment(ctx, p.URLHash, store.Enrichment{Why: "w", Importance: imp}); err != nil {
			t.Fatal(err)
		}
		hashes = append(hashes, p.URLHash)
	}

	pending, err := s.PendingDetail(ctx, now.Add(-time.Hour), 2)
	if err != nil || len(pending) != 2 || pending[0].URLHash != hashes[1] || pending[1].URLHash != hashes[2] {
		t.Fatalf("PendingDetail = %+v, %v, want importance 5 then 4", pending, err)
	}
	if err := s.SetDetail(ctx, hashes[1], store.Detail{Long: "Long text.", Exam: &store.ExamLens{Prelims: 3, Facts: []string{"f"}}}); err != nil {
		t.Fatal(err)
	}
	if left, _ := s.PendingDetail(ctx, now.Add(-time.Hour), 10); len(left) != 2 {
		t.Fatalf("pending after one detail = %d, want 2", len(left))
	}

	items, _, _ := s.Digest(ctx, store.DigestQuery{Since: now.Add(-time.Hour), ArticlesOnly: true, Detail: true})
	for _, it := range items {
		if it.ID == hashes[1] && (it.Long != "Long text." || fmt.Sprint(it.Rel) != "[3 0 0 0]") {
			t.Fatalf("detailed item = long %q rel %v", it.Long, it.Rel)
		}
	}
	if got, _, _ := s.Item(ctx, hashes[1]); got.Exam == nil || got.Exam.Facts[0] != "f" {
		t.Fatalf("Item = %+v, want the lens", got)
	}
}

// The migration that sends recent articles with a cut-off gist back to be summarised again.
func TestPostgresResummariseTruncatedMigration(t *testing.T) {
	pool := testutil.Postgres(t)
	s := store.NewPostgres(pool)
	defer s.Close()
	ctx := context.Background()

	raw, err := os.ReadFile("../db/migrations/00022_resummarise_truncated.sql")
	if err != nil {
		t.Fatal(err)
	}
	var stmt strings.Builder
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "-- +goose Down") {
			break
		}
		if !strings.HasPrefix(line, "--") {
			stmt.WriteString(line + "\n")
		}
	}

	now := time.Now()
	for _, p := range []struct {
		hash    string
		age     time.Duration
		body    string
		summary []string
	}{
		{"cut", time.Hour, "Body text.", []string{"A whole one.", "A cut one…"}},
		{"dots", time.Hour, "Body text.", []string{"Another cut one..."}},
		{"whole", time.Hour, "Body text.", []string{"All whole.", "Also whole."}},
		{"old", 5 * 24 * time.Hour, "Body text.", []string{"Cut but old…"}},
		{"notext", time.Hour, "", []string{"Cut but nothing to rebuild from…"}},
	} {
		pg := page("https://site.test/"+p.hash, "Title "+p.hash, p.body)
		pg.URLHash, pg.IsArticle, pg.Published, pg.Content = p.hash, true, now.Add(-p.age), p.body
		if err := s.Save(ctx, pg); err != nil {
			t.Fatal(err)
		}
		if err := s.SetSummary(ctx, p.hash, p.summary); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE pages SET content = '' WHERE url_hash = 'notext'`); err != nil {
		t.Fatal(err)
	}

	if _, err := pool.Exec(ctx, stmt.String()); err != nil {
		t.Fatalf("migration: %v", err)
	}
	pending, err := s.PendingSummaries(ctx, now.AddDate(0, 0, -30), 50)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, u := range pending {
		got[u.URLHash] = true
	}
	if len(got) != 2 || !got["cut"] || !got["dots"] {
		t.Fatalf("pending = %v, want only the two recent articles that have a cut bullet and their text", got)
	}
}

// One page the database rejects must not fail, roll back or poison the pages saved in the same
// batch; and text that is not valid UTF-8 (a mis-encoded site) or holds a NUL is cleaned, not fatal.
func TestPostgresSaveIsolatesABadPageFromItsBatch(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	ctx := context.Background()

	mk := func(n string) store.Page {
		p := page("https://x.test/"+n, "Title "+n, "body "+n)
		p.IsArticle, p.Category, p.Published = true, "tech", time.Now().Add(-time.Minute)
		return p
	}
	mis := mk("misencoded")
	mis.Title = "caf\xe9 \x00 menu" // windows-1252 byte and a NUL
	mis.Content = "text \xff\xfe more"
	bad := mk("bad")
	bad.Published = time.Date(300000, 1, 1, 0, 0, 0, 0, time.UTC) // out of range for timestamptz

	pages := []store.Page{mk("a"), mis, bad, mk("b"), mk("c")}
	errs := make([]error, len(pages))
	var wg sync.WaitGroup
	for i := range pages {
		wg.Add(1)
		go func() { defer wg.Done(); errs[i] = s.Save(ctx, pages[i]) }()
	}
	wg.Wait()

	for i, p := range pages {
		if p.URL == bad.URL {
			if errs[i] == nil {
				t.Error("the page with an impossible date saved without an error")
			}
			continue
		}
		if errs[i] != nil {
			t.Errorf("%s failed because of another page in its batch: %v", p.URL, errs[i])
		}
	}
	items, _, err := s.Digest(ctx, store.DigestQuery{Since: time.Now().Add(-time.Hour), ArticlesOnly: true, Limit: 50})
	if err != nil || len(items) != 4 {
		t.Fatalf("Digest = %d items, %v; want the 4 good pages (the mis-encoded one cleaned)", len(items), err)
	}
}
