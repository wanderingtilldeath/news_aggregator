package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/images"
	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

// saveArticles stores n dated articles on host, newest first by index.
func saveArticles(t *testing.T, f *fixture, host string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		p := store.Page{
			URLHash: fmt.Sprintf("%s-%02d", host, i), URL: fmt.Sprintf("http://%s/a/%02d", host, i),
			Title: fmt.Sprintf("Story %02d about budget", i), IsArticle: true,
			Published: time.Now().Add(-time.Duration(i) * time.Minute),
			Image:     fmt.Sprintf("http://%s/img/%02d.jpg", host, i),
		}
		if err := f.store.Save(context.Background(), p); err != nil {
			t.Fatal(err)
		}
	}
}

func asInt(t *testing.T, body map[string]any, key string) int {
	t.Helper()
	v, ok := body[key].(float64)
	if !ok {
		t.Fatalf("%s missing or not a number in %v", key, body)
	}
	return int(v)
}

func TestDigestPagination(t *testing.T) {
	f := newFixture(t)
	saveArticles(t, f, "a.test", 25)

	status, body, _ := f.do(t, "GET", "/digest?page_size=10", "")
	if status != 200 || len(body["items"].([]any)) != 10 || asInt(t, body, "total") != 25 ||
		asInt(t, body, "total_pages") != 3 || asInt(t, body, "page") != 1 || asInt(t, body, "page_size") != 10 {
		t.Fatalf("page 1 = %d %v", status, body)
	}
	first := body["items"].([]any)[0].(map[string]any)["url"]

	_, body, _ = f.do(t, "GET", "/digest?page_size=10&page=3", "")
	items := body["items"].([]any)
	if len(items) != 5 || asInt(t, body, "page") != 3 {
		t.Fatalf("last page has %d items, want 5: %v", len(items), body)
	}
	if items[0].(map[string]any)["url"] == first {
		t.Error("page 3 repeats page 1")
	}

	// Past the end: no items, but the total is still reported so a client can recover.
	_, body, _ = f.do(t, "GET", "/digest?page_size=10&page=9", "")
	if len(body["items"].([]any)) != 0 || asInt(t, body, "total") != 25 || asInt(t, body, "total_pages") != 3 {
		t.Errorf("page 9 = %v", body)
	}

	// Pages do not overlap and cover everything exactly once.
	seen := map[string]bool{}
	for page := 1; page <= 3; page++ {
		_, body, _ = f.do(t, "GET", fmt.Sprintf("/digest?page_size=10&page=%d", page), "")
		for _, it := range body["items"].([]any) {
			u := it.(map[string]any)["url"].(string)
			if seen[u] {
				t.Errorf("%s appears on two pages", u)
			}
			seen[u] = true
		}
	}
	if len(seen) != 25 {
		t.Errorf("pages covered %d items, want 25", len(seen))
	}

	// `limit` still works as the page size, and the default is 24.
	_, body, _ = f.do(t, "GET", "/digest?limit=7", "")
	if len(body["items"].([]any)) != 7 {
		t.Errorf("limit alias gave %d items, want 7", len(body["items"].([]any)))
	}
	_, body, _ = f.do(t, "GET", "/digest", "")
	if asInt(t, body, "page_size") != 24 || len(body["items"].([]any)) != 24 {
		t.Errorf("default page size = %v", body["page_size"])
	}

	for _, q := range []string{"page=0", "page=x", "page_size=0", "page_size=201", "page=-1"} {
		if status, _, _ := f.do(t, "GET", "/digest?"+q, ""); status != http.StatusBadRequest {
			t.Errorf("/digest?%s = %d, want 400", q, status)
		}
	}
}

func TestDigestFiltersCombineWithPagination(t *testing.T) {
	f := newFixture(t)
	saveArticles(t, f, "a.test", 12)
	saveArticles(t, f, "b.test", 5)

	_, body, _ := f.do(t, "GET", "/digest?source=a.test&page_size=5&page=3", "")
	if asInt(t, body, "total") != 12 || len(body["items"].([]any)) != 2 {
		t.Errorf("a.test page 3 = %v", body)
	}

	_, body, _ = f.do(t, "GET", "/digest?source=b.test&page_size=5", "")
	if asInt(t, body, "total") != 5 || asInt(t, body, "total_pages") != 1 {
		t.Errorf("b.test = %v", body)
	}
}

func TestDigestSources(t *testing.T) {
	f := newFixture(t)
	saveArticles(t, f, "a.test", 3)
	saveArticles(t, f, "b.test", 5)
	// A listing page does not count.
	_ = f.store.Save(context.Background(), store.Page{URLHash: "l", URL: "http://c.test/list", Published: time.Now()})

	status, body, hdr := f.do(t, "GET", "/digest/sources?days=2", "")
	sources := body["sources"].([]any)
	if status != 200 || len(sources) != 2 {
		t.Fatalf("sources = %d %v", status, body)
	}
	first := sources[0].(map[string]any)
	if first["source"] != "b.test" || first["count"] != float64(5) {
		t.Errorf("sources must be ordered by count: %v", sources)
	}
	if hdr.Get("Cache-Control") == "" {
		t.Error("sources should be briefly cacheable")
	}
	if status, _, _ := f.do(t, "GET", "/digest/sources?days=99", ""); status != http.StatusBadRequest {
		t.Errorf("days=99 = %d, want 400", status)
	}
}

func TestSearchPagination(t *testing.T) {
	f := newFixture(t)
	saveArticles(t, f, "a.test", 23)

	_, body, _ := f.do(t, "GET", "/search?q=budget&page_size=10", "")
	if len(body["hits"].([]any)) != 10 || asInt(t, body, "total") != 23 || asInt(t, body, "total_pages") != 3 {
		t.Fatalf("page 1 = %v", body)
	}

	_, body, _ = f.do(t, "GET", "/search?q=budget&page_size=10&page=3", "")
	if len(body["hits"].([]any)) != 3 || asInt(t, body, "page") != 3 {
		t.Errorf("page 3 = %v", body)
	}

	_, body, _ = f.do(t, "GET", "/search?q=nothing-matches", "")
	if len(body["hits"].([]any)) != 0 || asInt(t, body, "total") != 0 || asInt(t, body, "total_pages") != 0 {
		t.Errorf("empty result = %v", body)
	}

	for _, q := range []string{"q=budget&page=0", "q=budget&page_size=101", "q=budget&limit=0"} {
		if status, _, _ := f.do(t, "GET", "/search?"+q, ""); status != http.StatusBadRequest {
			t.Errorf("/search?%s = %d, want 400", q, status)
		}
	}
}

// pngBytes is a valid 1x1 PNG.
var pngBytes = []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00, 0x00,
	0x00, 0x0a, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00, 0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d,
	0xb4, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82}

func TestDigestServesImagesThroughTheProxy(t *testing.T) {
	var upstream int
	img := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstream++
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngBytes)
	}))
	defer img.Close()

	// The proxy looks images up in the same store the fixture serves.
	st := store.NewMemory()
	proxy := images.NewProxy(st, "test-agent", true)
	f := newFixtureWith(t, proxy)
	f.store = st
	f.srv.Config.Handler = NewHandler(Deps{Frontier: f.frontier, Store: st, Images: proxy})

	_ = st.Save(context.Background(), store.Page{
		URLHash: "abc123", URL: "http://a.test/story", Title: "Story", IsArticle: true,
		Published: time.Now(), Image: img.URL + "/photo.png",
	})
	_ = st.Save(context.Background(), store.Page{URLHash: "noimg", URL: "http://a.test/plain", Title: "Plain", IsArticle: true, Published: time.Now()})

	_, body, _ := f.do(t, "GET", "/digest", "")
	got := map[string]string{}
	for _, it := range body["items"].([]any) {
		m := it.(map[string]any)
		got[m["id"].(string)] = m["image"].(string)
	}
	if got["abc123"] != "/img/abc123" {
		t.Errorf("image = %q, want it served from our own origin", got["abc123"])
	}
	if got["noimg"] != "" {
		t.Errorf("an item with no image keeps an empty image, got %q", got["noimg"])
	}

	resp, err := http.Get(f.srv.URL + "/img/abc123")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/png" || string(data) != string(pngBytes) {
		t.Fatalf("GET /img/abc123 = %d %q (%d bytes)", resp.StatusCode, resp.Header.Get("Content-Type"), len(data))
	}
	if cc := resp.Header.Get("Cache-Control"); cc == "" {
		t.Error("images must be browser-cacheable")
	}

	resp2, _ := http.Get(f.srv.URL + "/img/abc123")
	resp2.Body.Close()
	if upstream != 1 {
		t.Errorf("upstream was hit %d times for 2 views, want 1 (cached)", upstream)
	}

	for _, id := range []string{"missing", "noimg"} {
		r, _ := http.Get(f.srv.URL + "/img/" + id)
		r.Body.Close()
		if r.StatusCode != http.StatusNotFound {
			t.Errorf("/img/%s = %d, want 404", id, r.StatusCode)
		}
	}
}
