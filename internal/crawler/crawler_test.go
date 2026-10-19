package crawler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/fetch"
	"github.com/wanderingtilldeath/news-aggregator/internal/frontier"
	"github.com/wanderingtilldeath/news-aggregator/internal/politeness"
	"github.com/wanderingtilldeath/news-aggregator/internal/robots"
	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

func TestCrawler_RespectsMaxDepth(t *testing.T) {
	ts, counter := site(t, map[string]string{
		"/":  `<a href="/a">a</a><a href="/b">b</a>`,
		"/a": `<a href="/c">c</a>`,
		"/b": `no links`,
		"/c": `should not be fetched at depth 1`,
	})

	h := newHarness(t, testConfig(), nil)
	crawl := h.crawl(t, ts.URL+"/", 1)

	for _, path := range []string{"/", "/a", "/b"} {
		if counter.count(path) != 1 {
			t.Errorf("%s fetched %d times, want 1", path, counter.count(path))
		}
	}
	if counter.count("/c") != 0 {
		t.Errorf("/c fetched %d times, want 0 (beyond max depth)", counter.count("/c"))
	}
	if got := h.frontier.Rows(crawl.ID); len(got) != 3 {
		t.Errorf("frontier has %d rows, want 3: %v", len(got), got)
	}
}

func TestCrawler_DeduplicatesCycles(t *testing.T) {
	ts, counter := site(t, map[string]string{
		"/":  `<a href="/a">a</a>`,
		"/a": `<a href="/">home</a><a href="/b">b</a><a href="/a#frag">self</a>`,
		"/b": `<a href="/a">a</a><a href="/?utm_source=x">home again</a>`,
	})

	h := newHarness(t, testConfig(), nil)
	h.crawl(t, ts.URL+"/", 10)

	for _, path := range []string{"/", "/a", "/b"} {
		if counter.count(path) != 1 {
			t.Errorf("%s fetched %d times, want exactly 1", path, counter.count(path))
		}
	}
}

func TestCrawler_StaysOnAllowedHosts(t *testing.T) {
	other, otherHits := site(t, map[string]string{"/": "other site"})
	ts, _ := site(t, map[string]string{"/": fmt.Sprintf(`<a href="%s/">external</a><a href="/in">in</a>`, other.URL), "/in": "ok"})

	h := newHarness(t, testConfig(), nil)
	crawl := h.crawl(t, ts.URL+"/", 3)

	if otherHits.total() != 0 {
		t.Fatalf("crawler left the seed host: %d requests to the other site", otherHits.total())
	}
	if got := h.status(crawl, ts.URL, "/in"); got != frontier.Done {
		t.Errorf("/in status = %q, want done", got)
	}
}

func TestCrawler_PermanentErrorsAreNotRetried(t *testing.T) {
	ts, counter := site(t, map[string]string{"/": `<a href="/missing">gone</a>`})

	h := newHarness(t, testConfig(), nil)
	crawl := h.crawl(t, ts.URL+"/", 1)

	if got := h.status(crawl, ts.URL, "/missing"); got != frontier.Failed {
		t.Errorf("/missing status = %q, want failed", got)
	}
	if counter.count("/missing") != 1 {
		t.Errorf("404 fetched %d times, want 1 (4xx is permanent)", counter.count("/missing"))
	}
}

func TestCrawler_RetriesTransientErrorsThenSucceeds(t *testing.T) {
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("finally"))
	}))
	defer ts.Close()

	cfg := testConfig()
	cfg.MaxAttempts = 5

	h := newHarness(t, cfg, nil)
	crawl := h.crawl(t, ts.URL+"/", 0)

	if got := h.status(crawl, ts.URL, "/"); got != frontier.Done {
		t.Fatalf("status = %q, want done", got)
	}
	if calls.Load() != 3 {
		t.Fatalf("server saw %d requests, want 3 (two 503s then success)", calls.Load())
	}
}

func TestCrawler_DeadLettersAfterMaxAttempts(t *testing.T) {
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	cfg := testConfig()
	cfg.MaxAttempts = 3

	h := newHarness(t, cfg, nil)
	crawl := h.crawl(t, ts.URL+"/", 0)

	if got := h.status(crawl, ts.URL, "/"); got != frontier.Dead {
		t.Fatalf("status = %q, want dead", got)
	}
	if calls.Load() != 3 {
		t.Fatalf("server saw %d requests, want exactly MaxAttempts=3", calls.Load())
	}
}

func TestCrawler_RetryAfterIsHonoured(t *testing.T) {
	var (
		calls atomic.Int32
		first atomic.Int64
		gap   atomic.Int64
	)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch calls.Add(1) {
		case 1:
			first.Store(time.Now().UnixNano())
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
		default:
			gap.Store(time.Now().UnixNano() - first.Load())
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("ok"))
		}
	}))
	defer ts.Close()

	h := newHarness(t, testConfig(), nil)
	crawl := h.crawl(t, ts.URL+"/", 0)

	if got := h.status(crawl, ts.URL, "/"); got != frontier.Done {
		t.Fatalf("status = %q, want done", got)
	}
	if d := time.Duration(gap.Load()); d < 900*time.Millisecond {
		t.Fatalf("retried after %v, want >= ~1s per Retry-After (backoff is only ~ms)", d)
	}
}

func TestCrawler_RobotsDisallowIsNotFetched(t *testing.T) {
	mux := http.NewServeMux()
	counter := newHits()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		counter.record("/robots.txt")
		w.Write([]byte("User-agent: *\nDisallow: /private\n"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		counter.record(r.URL.Path)
		w.Header().Set("Content-Type", "text/html")
		if r.URL.Path == "/" {
			w.Write([]byte(`<a href="/private/x">p</a><a href="/public">q</a>`))
			return
		}
		w.Write([]byte("page"))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	h := newHarness(t, testConfig(), func(d *Deps) {
		d.Robots = robots.New(robots.Options{Product: "CrawlerX"})
	})
	crawl := h.crawl(t, ts.URL+"/", 1)

	if got := h.status(crawl, ts.URL, "/private/x"); got != frontier.Blocked {
		t.Errorf("/private/x status = %q, want blocked", got)
	}
	if counter.count("/private/x") != 0 {
		t.Error("a robots-disallowed URL was fetched")
	}
	if got := h.status(crawl, ts.URL, "/public"); got != frontier.Done {
		t.Errorf("/public status = %q, want done", got)
	}
}

func TestCrawler_RobotsServerErrorBlocksEverything(t *testing.T) {
	mux := http.NewServeMux()
	var pageHits atomic.Int32
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) })
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { pageHits.Add(1) })
	ts := httptest.NewServer(mux)
	defer ts.Close()

	h := newHarness(t, testConfig(), func(d *Deps) { d.Robots = robots.New(robots.Options{}) })
	crawl := h.crawl(t, ts.URL+"/", 1)

	if pageHits.Load() != 0 {
		t.Fatalf("pages fetched %d times though robots.txt returned 503", pageHits.Load())
	}
	if got := h.status(crawl, ts.URL, "/"); got != frontier.Blocked {
		t.Fatalf("status = %q, want blocked", got)
	}
}

func TestCrawler_RedirectsAreEnqueuedNotFollowedInline(t *testing.T) {
	ts, counter := site(t, map[string]string{"/final": "arrived"})
	_ = counter

	mux := http.NewServeMux()
	var oldHits atomic.Int32
	mux.HandleFunc("/old", func(w http.ResponseWriter, r *http.Request) {
		oldHits.Add(1)
		http.Redirect(w, r, ts.URL+"/final", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<a href="/old">old</a>`))
	})
	front := httptest.NewServer(mux)
	defer front.Close()

	// /old redirects to a different host that is not in scope.
	h := newHarness(t, testConfig(), nil)
	crawl := h.crawl(t, front.URL+"/", 3)

	if got := h.status(crawl, front.URL, "/old"); got != frontier.Skipped {
		t.Errorf("/old status = %q, want skipped", got)
	}
	if counter.count("/final") != 0 {
		t.Error("redirect to an out-of-scope host was followed")
	}
}

func TestCrawler_RedirectChainWithinScopeIsCrawled(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/middle", http.StatusFound) })
	mux.HandleFunc("/middle", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/end", http.StatusFound) })
	mux.HandleFunc("/end", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<title>End</title>"))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	h := newHarness(t, testConfig(), nil)
	crawl := h.crawl(t, ts.URL+"/start", 5)

	if got := h.status(crawl, ts.URL, "/end"); got != frontier.Done {
		t.Fatalf("/end status = %q, want done (rows: %v)", got, h.frontier.Rows(crawl.ID))
	}
}

func TestCrawler_RedirectLoopTerminates(t *testing.T) {
	var calls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Redirect(w, r, "/b", http.StatusFound)
	})
	mux.HandleFunc("/b", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Redirect(w, r, "/a", http.StatusFound)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	h := newHarness(t, testConfig(), nil)
	h.crawl(t, ts.URL+"/a", 50) // generous depth: dedupe alone must stop it

	if calls.Load() != 2 {
		t.Fatalf("redirect loop caused %d requests, want 2 (one per URL)", calls.Load())
	}
}

func TestCrawler_EndlessRedirectChainStopsAtMaxDepth(t *testing.T) {
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		http.Redirect(w, r, fmt.Sprintf("/r/%d", n), http.StatusFound)
	}))
	defer ts.Close()

	h := newHarness(t, testConfig(), nil)
	h.crawl(t, ts.URL+"/r/0", 4)

	if got := calls.Load(); got != 5 { // depths 0..4
		t.Fatalf("endless chain made %d requests, want 5 (max depth 4)", got)
	}
}

func TestCrawler_NonHTMLIsSkipped(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<a href="/doc.pdf">pdf</a>`))
	})
	mux.HandleFunc("/doc.pdf", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.Write([]byte("%PDF"))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	h := newHarness(t, testConfig(), nil)
	crawl := h.crawl(t, ts.URL+"/", 1)

	if got := h.status(crawl, ts.URL, "/doc.pdf"); got != frontier.Skipped {
		t.Fatalf("/doc.pdf status = %q, want skipped", got)
	}
	if _, stored := h.store.Pages()[ts.URL+"/doc.pdf"]; stored {
		t.Fatal("a non-HTML page was stored")
	}
}

func TestCrawler_OversizedBodyIsTruncated(t *testing.T) {
	ts, _ := site(t, map[string]string{"/": "<title>Big</title>" + strings.Repeat("x", 100_000)})

	h := newHarness(t, testConfig(), func(d *Deps) {
		d.Fetcher = fetch.New(fetch.Options{MaxBytes: 2000})
	})
	h.crawl(t, ts.URL+"/", 0)

	page, ok := h.store.Pages()[ts.URL+"/"]
	if !ok {
		t.Fatal("page not stored")
	}
	if page.SizeBytes != 2000 {
		t.Fatalf("stored %d bytes, want the 2000-byte cap", page.SizeBytes)
	}
	if page.Title != "Big" {
		t.Fatalf("Title = %q; the cap should still leave the start of the document parseable", page.Title)
	}
}

func TestCrawler_StoresParsedPage(t *testing.T) {
	ts, _ := site(t, map[string]string{"/": `<html><head><title>Home</title>
		<meta name="description" content="The home page"></head>
		<body><h1>Welcome</h1><p>hello there</p></body></html>`})

	h := newHarness(t, testConfig(), nil)
	h.crawl(t, ts.URL+"/", 0)

	page := h.store.Pages()[ts.URL+"/"]

	want := store.Page{Title: "Home", Description: "The home page", H1: "Welcome", StatusCode: 200}
	if page.Title != want.Title || page.Description != want.Description || page.H1 != want.H1 || page.StatusCode != 200 {
		t.Fatalf("stored page = %+v", page)
	}
	if page.ContentHash == "" || page.URLHash == "" {
		t.Fatalf("missing hashes: %+v", page)
	}
}

func TestCrawler_MetaRobotsNoindexNofollow(t *testing.T) {
	ts, counter := site(t, map[string]string{
		"/":             `<a href="/noindex">1</a><a href="/nofollow">2</a>`,
		"/noindex":      `<meta name="robots" content="noindex"><a href="/via-noindex">x</a>`,
		"/nofollow":     `<meta name="robots" content="nofollow"><a href="/via-nofollow">x</a>`,
		"/via-noindex":  "reachable: noindex still follows links",
		"/via-nofollow": "unreachable",
	})

	h := newHarness(t, testConfig(), nil)
	h.crawl(t, ts.URL+"/", 3)

	pages := h.store.Pages()
	if _, ok := pages[ts.URL+"/noindex"]; ok {
		t.Error("noindex page was stored")
	}
	if _, ok := pages[ts.URL+"/nofollow"]; !ok {
		t.Error("nofollow page should still be stored")
	}
	if counter.count("/via-noindex") != 1 {
		t.Error("links on a noindex page should still be followed")
	}
	if counter.count("/via-nofollow") != 0 {
		t.Error("links on a nofollow page were followed")
	}
}

func TestCrawler_PerHostDelayIsRespected(t *testing.T) {
	pages := map[string]string{"/": ""}
	var links strings.Builder
	for i := 0; i < 5; i++ {
		fmt.Fprintf(&links, `<a href="/p%d">p</a>`, i)
		pages[fmt.Sprintf("/p%d", i)] = "page"
	}
	pages["/"] = links.String()
	ts, counter := site(t, pages)

	const delay = 60 * time.Millisecond

	cfg := testConfig()
	cfg.Workers = 8
	cfg.HostDelay = delay

	h := newHarness(t, cfg, func(d *Deps) { d.Gate = politeness.NewMemory(nil) })
	h.crawl(t, ts.URL+"/", 1)

	times := counter.allTimes()
	if len(times) != 6 {
		t.Fatalf("got %d requests, want 6", len(times))
	}
	for i := 1; i < len(times); i++ {
		// Allow a little scheduler slack below the configured delay.
		if gap := times[i].Sub(times[i-1]); gap < delay-10*time.Millisecond {
			t.Errorf("requests %d and %d were %v apart, want >= %v", i-1, i, gap, delay)
		}
	}
}

func TestCrawler_DifferentHostsAreNotSerialised(t *testing.T) {
	// Two hosts, each with a 300ms delay and 3 pages. A global limiter would
	// need ~1.5s; per-host gating lets the hosts proceed in parallel.
	makeHost := func() (*httptest.Server, *hits) {
		return site(t, map[string]string{
			"/":  `<a href="/a">a</a><a href="/b">b</a>`,
			"/a": "a",
			"/b": "b",
		})
	}
	a, _ := makeHost()
	b, _ := makeHost()

	cfg := testConfig()
	cfg.HostDelay = 300 * time.Millisecond
	cfg.Workers = 8

	h := newHarness(t, cfg, func(d *Deps) { d.Gate = politeness.NewMemory(nil) })

	start := time.Now()
	done := make(chan struct{}, 2)
	for _, ts := range []*httptest.Server{a, b} {
		go func() {
			h.c.Crawl(context.Background(), ts.URL+"/", 1, nil)
			done <- struct{}{}
		}()
	}
	<-done
	<-done

	// Each host needs 2 gaps of 300ms = 600ms. Serialised across hosts it
	// would take about 1.2s.
	if elapsed := time.Since(start); elapsed > 1000*time.Millisecond {
		t.Fatalf("two hosts took %v; per-host gating should run them in parallel (~600ms)", elapsed)
	}
}

func TestCrawler_ReclaimsWorkFromACrashedWorker(t *testing.T) {
	ts, counter := site(t, map[string]string{
		"/":  `<a href="/a">a</a><a href="/b">b</a>`,
		"/a": "a",
		"/b": "b",
	})

	h := newHarness(t, testConfig(), nil)
	ctx := context.Background()

	crawl, err := frontier.StartCrawl(ctx, h.frontier, ts.URL+"/", 1, nil)
	if err != nil {
		t.Fatal(err)
	}

	// A "worker" claims the seed with a short lease and dies without
	// finishing or releasing it.
	if items, err := h.frontier.Claim(ctx, 1, 30*time.Millisecond); err != nil || len(items) != 1 {
		t.Fatalf("setup claim = %v, %v", items, err)
	}

	// A live crawler must pick the row up once the lease expires.
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go h.c.Run(runCtx)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		stats, _ := h.frontier.Stats(ctx, crawl.ID)
		if stats[frontier.Done] == 3 && stats.Active() == 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	stats, _ := h.frontier.Stats(ctx, crawl.ID)
	if stats[frontier.Done] != 3 {
		t.Fatalf("stats = %v, want 3 done after the lease expired", stats)
	}
	for _, path := range []string{"/", "/a", "/b"} {
		if counter.count(path) != 1 {
			t.Errorf("%s fetched %d times, want 1", path, counter.count(path))
		}
	}
}

func TestCrawler_DoesNotRefetchFinishedURLsAfterRestart(t *testing.T) {
	ts, counter := site(t, map[string]string{
		"/":  `<a href="/a">a</a><a href="/b">b</a>`,
		"/a": "a",
		"/b": "b",
	})

	h := newHarness(t, testConfig(), nil)
	crawl := h.crawl(t, ts.URL+"/", 1)

	// A new crawler instance over the same frontier (a "restart") must find
	// nothing left to do.
	restarted := New(testConfig(), Deps{Frontier: h.frontier, Fetcher: fetch.New(fetch.Options{}), Store: h.store, Logger: discardLogger()})
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	restarted.Run(ctx)

	if counter.total() != 3 {
		t.Fatalf("server saw %d requests after restart, want still 3", counter.total())
	}
	if stats, _ := h.frontier.Stats(context.Background(), crawl.ID); stats[frontier.Done] != 3 {
		t.Fatalf("stats = %v", stats)
	}
}

func TestCrawler_ContextCancellationStopsPromptlyAndReleasesWork(t *testing.T) {
	block := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-block:
		case <-r.Context().Done():
		}
	}))
	// Cleanups run LIFO: unblock handlers first, then wait for the server.
	t.Cleanup(ts.Close)
	t.Cleanup(func() { close(block) })

	h := newHarness(t, testConfig(), nil)

	ctx, cancel := context.WithCancel(context.Background())
	crawl, err := frontier.StartCrawl(ctx, h.frontier, ts.URL+"/", 1, nil)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		h.c.Run(ctx)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond) // let a worker start the stuck request
	cancel()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Run() did not return promptly after context cancellation")
	}

	stats, _ := h.frontier.Stats(context.Background(), crawl.ID)
	if stats[frontier.Pending] != 1 || stats[frontier.Leased] != 0 {
		t.Fatalf("stats = %v; the aborted URL should be released back to pending", stats)
	}
}

func TestCrawler_InFlightWorkFinishesDuringGracePeriod(t *testing.T) {
	var started atomic.Bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started.Store(true)
		time.Sleep(100 * time.Millisecond)
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("slow but finishes"))
	}))
	defer ts.Close()

	cfg := testConfig()
	cfg.ShutdownGrace = 2 * time.Second

	h := newHarness(t, cfg, nil)
	ctx, cancel := context.WithCancel(context.Background())
	crawl, _ := frontier.StartCrawl(ctx, h.frontier, ts.URL+"/", 0, nil)

	done := make(chan struct{})
	go func() {
		h.c.Run(ctx)
		close(done)
	}()

	for !started.Load() {
		time.Sleep(time.Millisecond)
	}
	cancel() // shutdown while the request is in flight
	<-done

	if stats, _ := h.frontier.Stats(context.Background(), crawl.ID); stats[frontier.Done] != 1 {
		t.Fatalf("stats = %v; in-flight work should finish inside the grace period", stats)
	}
}

func TestCrawler_BadSeedIsRejected(t *testing.T) {
	h := newHarness(t, testConfig(), nil)

	if _, err := h.c.Crawl(context.Background(), "ftp://example.com/", 1, nil); err == nil {
		t.Fatal("expected an error for a non-http seed")
	}
}
