package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/frontier"
	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

type fixture struct {
	srv      *httptest.Server
	frontier *frontier.Memory
	store    *store.Memory
	healthy  bool
}

func newFixture(t *testing.T) *fixture { return newFixtureWith(t, nil) }

// newFixtureWith is newFixture with an image proxy handler mounted at /img/{id}.
func newFixtureWith(t *testing.T, images http.Handler) *fixture {
	t.Helper()

	f := &fixture{frontier: frontier.NewMemory(nil), store: store.NewMemory(), healthy: true}

	f.srv = httptest.NewServer(NewHandler(Deps{
		Frontier: f.frontier,
		Store:    f.store,
		Images:   images,
		Checks: map[string]func(context.Context) error{
			"db": func(context.Context) error {
				if f.healthy {
					return nil
				}
				return errors.New("connection refused")
			},
		},
	}))
	t.Cleanup(f.srv.Close)

	return f
}

func (f *fixture) do(t *testing.T, method, path, body string) (int, map[string]any, http.Header) {
	t.Helper()

	req, _ := http.NewRequest(method, f.srv.URL+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)

	return resp.StatusCode, out, resp.Header
}

func TestCreateAndGetCrawl(t *testing.T) {
	f := newFixture(t)

	status, body, hdr := f.do(t, "POST", "/crawls", `{"seed":"HTTP://Example.com:80/a/../b?utm_source=x","max_depth":3}`)
	if status != http.StatusCreated {
		t.Fatalf("status = %d, body %v", status, body)
	}
	if body["seed"] != "http://example.com/b" || body["max_depth"] != float64(3) {
		t.Fatalf("seed should be canonicalised: %v", body)
	}
	if hdr.Get("Location") != "/crawls/1" {
		t.Fatalf("Location = %q", hdr.Get("Location"))
	}
	stats := body["stats"].(map[string]any)
	if stats["pending"] != float64(1) || body["total"] != float64(1) || body["done"] != false {
		t.Fatalf("fresh crawl should have the seed pending: %v", body)
	}

	// Finish the seed and re-read.
	items, _ := f.frontier.Claim(context.Background(), 1, 60e9)
	f.frontier.Finish(context.Background(), items[0], frontier.Done, "")

	status, body, _ = f.do(t, "GET", "/crawls/1", "")
	if status != 200 || body["done"] != true || body["stats"].(map[string]any)["done"] != float64(1) {
		t.Fatalf("GET /crawls/1 = %d %v", status, body)
	}
}

func TestCreateCrawlDefaultsAndValidation(t *testing.T) {
	f := newFixture(t)

	if status, body, _ := f.do(t, "POST", "/crawls", `{"seed":"http://a.test/"}`); status != 201 || body["max_depth"] != float64(2) {
		t.Fatalf("default depth: %d %v", status, body)
	}

	bad := map[string]string{
		"not json":      `nope`,
		"unknown field": `{"seed":"http://a.test/","bogus":1}`,
		"relative seed": `{"seed":"/just/a/path"}`,
		"ftp seed":      `{"seed":"ftp://a.test/"}`,
		"empty seed":    `{}`,
		"depth too big": `{"seed":"http://a.test/","max_depth":50}`,
		"negative":      `{"seed":"http://a.test/","max_depth":-1}`,
	}
	for name, body := range bad {
		if status, resp, _ := f.do(t, "POST", "/crawls", body); status != http.StatusBadRequest || resp["error"] == nil {
			t.Errorf("%s: status %d, body %v; want 400 with an error", name, status, resp)
		}
	}
}

func TestGetCrawlErrors(t *testing.T) {
	f := newFixture(t)

	if status, _, _ := f.do(t, "GET", "/crawls/999", ""); status != http.StatusNotFound {
		t.Errorf("unknown crawl: %d, want 404", status)
	}
	if status, _, _ := f.do(t, "GET", "/crawls/abc", ""); status != http.StatusBadRequest {
		t.Errorf("non-numeric id: %d, want 400", status)
	}
}

func TestSearch(t *testing.T) {
	f := newFixture(t)
	f.store.Save(context.Background(), store.Page{URLHash: "1", URL: "http://a.test/go", Title: "Go crawler", Snippet: "polite"})

	status, body, _ := f.do(t, "GET", "/search?q=crawler", "")
	hits, _ := body["hits"].([]any)
	if status != 200 || len(hits) != 1 || hits[0].(map[string]any)["url"] != "http://a.test/go" {
		t.Fatalf("search = %d %v", status, body)
	}

	if status, _, _ := f.do(t, "GET", "/search", ""); status != 400 {
		t.Errorf("missing q: %d, want 400", status)
	}
	if status, _, _ := f.do(t, "GET", "/search?q=x&limit=0", ""); status != 400 {
		t.Errorf("bad limit: %d, want 400", status)
	}

	// No matches is an empty list, not null.
	_, body, _ = f.do(t, "GET", "/search?q=zzz", "")
	if hits, ok := body["hits"].([]any); !ok || len(hits) != 0 {
		t.Errorf("no-match hits = %#v, want []", body["hits"])
	}
}

func TestHealthz(t *testing.T) {
	f := newFixture(t)

	if status, body, _ := f.do(t, "GET", "/healthz", ""); status != 200 || body["status"] != "ok" {
		t.Fatalf("healthy: %d %v", status, body)
	}

	f.healthy = false
	status, body, _ := f.do(t, "GET", "/healthz", "")
	if status != http.StatusServiceUnavailable || body["failing"].(map[string]any)["db"] == nil {
		t.Fatalf("unhealthy: %d %v", status, body)
	}
}

func TestStaticPage(t *testing.T) {
	f := newFixture(t)

	resp, err := http.Get(f.srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	page, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(page), "CrawlerX") {
		t.Fatalf("/ = %d", resp.StatusCode)
	}
}

func TestListCrawlsAndDepths(t *testing.T) {
	f := newFixture(t)

	_, body, _ := f.do(t, "GET", "/crawls", "")
	if list, ok := body["crawls"].([]any); !ok || len(list) != 0 {
		t.Fatalf("empty list = %#v, want []", body["crawls"])
	}

	f.do(t, "POST", "/crawls", `{"seed":"http://a.test/","max_depth":3}`)
	f.do(t, "POST", "/crawls", `{"seed":"http://b.test/","max_depth":3}`)

	_, body, _ = f.do(t, "GET", "/crawls", "")
	list := body["crawls"].([]any)
	if len(list) != 2 || list[0].(map[string]any)["seed"] != "http://b.test/" {
		t.Fatalf("crawls = %v, want newest first", list)
	}
	if list[0].(map[string]any)["stats"].(map[string]any)["pending"] != float64(1) {
		t.Fatalf("list entries should carry stats: %v", list[0])
	}

	status, body, _ := f.do(t, "GET", "/crawls/1/depths", "")
	depths := body["depths"].([]any)
	if status != 200 || len(depths) != 1 || depths[0].(map[string]any)["depth"] != float64(0) {
		t.Fatalf("depths = %d %v", status, body)
	}

	if status, _, _ := f.do(t, "GET", "/crawls/99/depths", ""); status != http.StatusNotFound {
		t.Errorf("unknown crawl depths: %d, want 404", status)
	}
}

func TestDigest(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	for _, p := range []store.Page{
		{URLHash: "1", URL: "http://a.test/new", Title: "Exam dates announced", Published: time.Now().Add(-time.Hour), IsArticle: true, Image: "http://a.test/new.jpg"},
		{URLHash: "2", URL: "http://a.test/old", Title: "Old exam news", Published: time.Now().AddDate(0, 0, -5), IsArticle: true},
		{URLHash: "3", URL: "http://a.test/other", Title: "Budget", Published: time.Now().Add(-2 * time.Hour), IsArticle: true},
		{URLHash: "4", URL: "http://b.test/news/", Title: "News index", Published: time.Now().Add(-time.Hour)},
	} {
		if err := f.store.Save(ctx, p); err != nil {
			t.Fatal(err)
		}
	}

	status, body, _ := f.do(t, "GET", "/digest", "")
	if status != http.StatusOK || len(body["items"].([]any)) != 2 {
		t.Fatalf("default digest = %d %v, want 2 items from the last day", status, body)
	}

	_, body, _ = f.do(t, "GET", "/digest?days=7&q=exam", "")
	items := body["items"].([]any)
	if len(items) != 2 || items[0].(map[string]any)["url"] != "http://a.test/new" {
		t.Fatalf("exam digest = %v, want new first", items)
	}
	first := items[0].(map[string]any)
	if first["image"] != "http://a.test/new.jpg" || first["source"] != "a.test" {
		t.Errorf("item should carry image and source: %v", first)
	}

	_, body, _ = f.do(t, "GET", "/digest?articles_only=false", "")
	if n := len(body["items"].([]any)); n != 3 {
		t.Errorf("articles_only=false returned %d items, want 3 including the index page", n)
	}

	_, body, _ = f.do(t, "GET", "/digest?days=7&source=A.test", "")
	if n := len(body["items"].([]any)); n != 3 {
		t.Errorf("source filter returned %d items, want the 3 articles on a.test", n)
	}

	for _, q := range []string{"days=0", "days=31", "days=x", "limit=0"} {
		if status, _, _ := f.do(t, "GET", "/digest?"+q, ""); status != http.StatusBadRequest {
			t.Errorf("/digest?%s status = %d, want 400", q, status)
		}
	}
}

func TestDigestPageIsServed(t *testing.T) {
	f := newFixture(t)

	resp, err := http.Get(f.srv.URL + "/digest.html")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "Daily news digest") {
		t.Fatalf("GET /digest.html = %d, want the digest page", resp.StatusCode)
	}
}
