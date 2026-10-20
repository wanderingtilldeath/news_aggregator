package e2e

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/fakesite"
	"github.com/wanderingtilldeath/news-aggregator/internal/testutil"
)

// TestDailyDigestRun runs the real binary with a sources file: the scheduler
// must crawl the news section, store image and article flags, write a summary
// for each article (extractive: no API key here), keep listing pages out of
// the article set, and, on a second run, change nothing it already had.
func TestDailyDigestRun(t *testing.T) {
	requireEnv(t)
	pool, dbURL := testutil.PostgresWithURL(t)

	ts := httptest.NewServer(fakesite.New(fakesite.Options{}))
	defer ts.Close()

	sources := filepath.Join(t.TempDir(), "sources.json")
	if err := os.WriteFile(sources, []byte(`[{"name":"fake news","seed":"`+ts.URL+`/news/","max_depth":1,"path_prefixes":["/news/"]}]`), 0o600); err != nil {
		t.Fatal(err)
	}

	count := func(where string) int {
		var n int
		if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM pages WHERE `+where).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	want := fakesite.NewsArticles

	run := func() *proc {
		return startCrawlerx(t, dbURL, "-host-delay", "5ms", "-sources", sources, "-run-on-start", "-timezone", "UTC")
	}
	stop := func(p *proc) {
		_ = p.cmd.Process.Signal(syscall.SIGTERM)
		if err := p.wait(t, 15*time.Second); err != nil {
			t.Fatalf("exit: %v\n%s", err, p.out.String())
		}
	}

	first := run()
	waitFor(t, 60*time.Second, "every article summarised", func() bool {
		return count(`is_article AND summarized_at IS NOT NULL`) == want
	}, func() string { return "output:\n" + tail(first.out.String(), 3000) })
	stop(first)

	if n := count(`is_article`); n != want {
		t.Errorf("%d articles, want %d (the listing and archive pages must not count)", n, want)
	}
	if n := count(`is_article AND image_url LIKE '%/img/%.jpg'`); n != want {
		t.Errorf("%d articles have an image, want %d", n, want)
	}
	if n := count(`is_article AND cardinality(summary) >= 1 AND content LIKE 'The council announced%'`); n != want {
		t.Errorf("%d articles have a summary and body text, want %d", n, want)
	}
	if n := count(`NOT is_article AND url LIKE '%/news/%'`); n != 2 {
		t.Errorf("%d non-article pages under /news/, want the listing and the archive", n)
	}

	var seenBefore time.Time
	if err := pool.QueryRow(context.Background(), `SELECT min(first_seen_at) FROM pages WHERE is_article`).Scan(&seenBefore); err != nil {
		t.Fatal(err)
	}

	// Second run: a new crawl over the same pages creates no duplicates and
	// leaves first_seen_at and the stored summaries alone.
	second := run()
	waitFor(t, 60*time.Second, "second crawl to finish", func() bool {
		var n int
		_ = pool.QueryRow(context.Background(), `SELECT count(*) FROM crawls`).Scan(&n)
		c := statusCounts(t, pool)
		return n == 2 && c["pending"] == 0 && c["leased"] == 0
	}, func() string { return "output:\n" + tail(second.out.String(), 3000) })
	stop(second)

	if n := count(`is_article`); n != want {
		t.Errorf("%d articles after the second run, want still %d", n, want)
	}
	var seenAfter time.Time
	if err := pool.QueryRow(context.Background(), `SELECT min(first_seen_at) FROM pages WHERE is_article`).Scan(&seenAfter); err != nil {
		t.Fatal(err)
	}
	if !seenAfter.Equal(seenBefore) {
		t.Errorf("first_seen_at moved from %v to %v", seenBefore, seenAfter)
	}
	if n := count(`is_article AND summarized_at IS NOT NULL`); n != want {
		t.Errorf("%d summarised after the second run, want %d", n, want)
	}
}

// TestDigestRulesForUndatedArticlesAndTopics covers the two layouts real news
// sites force on us: a dated listing whose article pages carry no date (only
// the article_prefixes rule can tell them apart), and one page holding several
// topics (split_articles).
func TestDigestRulesForUndatedArticlesAndTopics(t *testing.T) {
	requireEnv(t)
	pool, dbURL := testutil.PostgresWithURL(t)

	ts := httptest.NewServer(fakesite.New(fakesite.Options{}))
	defer ts.Close()

	sources := filepath.Join(t.TempDir(), "sources.json")
	body := `[
	  {"name":"undated","seed":"` + ts.URL + `/day/{date:02-01-2006}","days_back":1,"max_depth":1,
	   "path_prefixes":["/items/"],"article_prefixes":["/items/"],"image_from_source":true},
	  {"name":"topics","seed":"` + ts.URL + `/topics/","max_depth":0,"split_articles":true}
	]`
	if err := os.WriteFile(sources, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	count := func(where string) int {
		var n int
		if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM pages WHERE `+where).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	wantArticles := fakesite.DayItems + fakesite.TopicCount

	run := func() *proc {
		// The cited "sources" are on loopback here, which the image step refuses by default.
		return startCrawlerx(t, dbURL, "-host-delay", "5ms", "-sources", sources, "-run-on-start", "-timezone", "UTC", "-allow-private-sources")
	}
	stop := func(p *proc) {
		_ = p.cmd.Process.Signal(syscall.SIGTERM)
		if err := p.wait(t, 15*time.Second); err != nil {
			t.Fatalf("exit: %v\n%s", err, p.out.String())
		}
	}

	first := run()
	// Summaries and source-image lookups run side by side (lookups are spaced a
	// second apart), so wait for both.
	waitFor(t, 60*time.Second, "every article summarised and its source image looked up", func() bool {
		return count(`is_article AND summarized_at IS NOT NULL`) == wantArticles &&
			count(`is_article AND url LIKE '%/items/%' AND image_checked_at IS NOT NULL`) == fakesite.DayItems
	}, func() string { return "output:\n" + tail(first.out.String(), 3000) })
	stop(first)

	if n := count(`is_article AND url LIKE '%/items/%' AND published_at IS NULL`); n != fakesite.DayItems {
		t.Errorf("%d undated articles under /items/, want %d", n, fakesite.DayItems)
	}
	if n := count(`is_article AND url LIKE '%/topics/#t%'`); n != fakesite.TopicCount {
		t.Errorf("%d topic items, want %d (one per <article id>)", n, fakesite.TopicCount)
	}
	if n := count(`NOT is_article AND (url LIKE '%/day/%' OR url LIKE '%/topics/')`); n < 2 {
		t.Errorf("the day listing and the topics container must be stored as non-articles, found %d", n)
	}
	if n := count(`is_article`); n != wantArticles {
		t.Errorf("%d articles in total, want %d", n, wantArticles)
	}
	// Undated articles take their image from the cited source page, not the
	// page's own (generic) og:image; topics keep the <img> inside their block.
	if n := count(`is_article AND url LIKE '%/items/%' AND image_url LIKE '%/img/source-%.jpg'`); n != fakesite.DayItems {
		t.Errorf("%d articles got their image from the cited source, want %d", n, fakesite.DayItems)
	}
	if n := count(`is_article AND url LIKE '%/items/%' AND image_checked_at IS NOT NULL`); n != fakesite.DayItems {
		t.Errorf("%d articles marked as looked up, want %d", n, fakesite.DayItems)
	}
	if n := count(`is_article AND url LIKE '%/topics/#t%' AND image_url LIKE '%/img/topic-%.png'`); n != fakesite.TopicCount {
		t.Errorf("%d topics have their own image, want %d", n, fakesite.TopicCount)
	}
	// {date} with days_back=1 starts two crawls for the undated source, plus one for topics.
	var crawls int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM crawls WHERE article_prefixes <> '{}' OR split_articles`).Scan(&crawls); err != nil || crawls != 3 {
		t.Errorf("%d crawls carry article rules, want 3 (today + yesterday + topics): %v", crawls, err)
	}

	// A second run adds nothing new.
	second := run()
	waitFor(t, 60*time.Second, "second run's crawls to finish", func() bool {
		var n int
		_ = pool.QueryRow(context.Background(), `SELECT count(*) FROM crawls`).Scan(&n)
		c := statusCounts(t, pool)
		return n == 6 && c["pending"] == 0 && c["leased"] == 0
	}, func() string { return "output:\n" + tail(second.out.String(), 3000) })
	stop(second)

	if n := count(`is_article`); n != wantArticles {
		t.Errorf("%d articles after the second run, want still %d", n, wantArticles)
	}
}
