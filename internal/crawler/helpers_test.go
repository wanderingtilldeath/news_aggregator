package crawler

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/fetch"
	"github.com/wanderingtilldeath/news-aggregator/internal/frontier"
	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

// harness bundles a crawler with the in-memory backends it runs on.
type harness struct {
	c        *Crawler
	frontier *frontier.Memory
	store    *store.Memory
}

func testConfig() Config {
	return Config{
		Workers:       4,
		PollInterval:  2 * time.Millisecond,
		ShutdownGrace: 200 * time.Millisecond,
		MaxAttempts:   3,
		Backoff:       fetch.Backoff{Base: time.Millisecond, Max: 5 * time.Millisecond},
	}
}

// newHarness builds a crawler on in-memory backends. tweak may adjust the
// dependencies before the crawler is created.
func newHarness(t *testing.T, cfg Config, tweak func(*Deps)) *harness {
	t.Helper()

	h := &harness{frontier: frontier.NewMemory(nil), store: store.NewMemory()}

	deps := Deps{
		Frontier: h.frontier,
		Fetcher:  fetch.New(fetch.Options{Timeout: 2 * time.Second}),
		Store:    h.store,
		Logger:   discardLogger(),
	}
	if tweak != nil {
		tweak(&deps)
	}

	h.c = New(cfg, deps)

	return h
}

// crawl runs a crawl to completion, failing the test if it takes too long.
func (h *harness) crawl(t *testing.T, seed string, maxDepth int) frontier.Crawl {
	t.Helper()

	type result struct {
		crawl frontier.Crawl
		err   error
	}

	done := make(chan result, 1)
	go func() {
		crawl, err := h.c.Crawl(context.Background(), seed, maxDepth, nil)
		done <- result{crawl, err}
	}()

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("Crawl() returned error: %v", r.err)
		}
		return r.crawl
	case <-time.After(10 * time.Second):
		t.Fatal("crawl did not finish in time")
		return frontier.Crawl{}
	}
}

// status returns the terminal status recorded for a path of server.
func (h *harness) status(crawl frontier.Crawl, serverURL, path string) frontier.Status {
	canonical := serverURL + path
	return h.frontier.Rows(crawl.ID)[canonical]
}

// hits counts requests per path.
type hits struct {
	mu     sync.Mutex
	counts map[string]int
	times  map[string][]time.Time
}

func newHits() *hits {
	return &hits{counts: map[string]int{}, times: map[string][]time.Time{}}
}

func (h *hits) record(path string) int {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.counts[path]++
	h.times[path] = append(h.times[path], time.Now())

	return h.counts[path]
}

func (h *hits) count(path string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.counts[path]
}

func (h *hits) total() int {
	h.mu.Lock()
	defer h.mu.Unlock()

	n := 0
	for _, c := range h.counts {
		n += c
	}
	return n
}

// allTimes returns every request time, sorted.
func (h *hits) allTimes() []time.Time {
	h.mu.Lock()
	defer h.mu.Unlock()

	var all []time.Time
	for _, ts := range h.times {
		all = append(all, ts...)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Before(all[j]) })

	return all
}

// site serves static HTML pages keyed by path and records hits. Paths not in
// the map return 404.
func site(t *testing.T, pages map[string]string) (*httptest.Server, *hits) {
	t.Helper()

	counter := newHits()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counter.record(r.URL.Path)

		body, ok := pages[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(body))
	}))
	t.Cleanup(ts.Close)

	return ts, counter
}

// discardLogger keeps `go test` output focused on results, not crawl logs.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
