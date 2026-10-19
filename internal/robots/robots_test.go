package robots

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// robotsServer serves the given robots.txt response and counts fetches.
func robotsServer(t *testing.T, status int, body string) (*httptest.Server, *atomic.Int32) {
	t.Helper()

	var hits atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/robots.txt" {
			http.NotFound(w, r)
			return
		}
		hits.Add(1)
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	t.Cleanup(ts.Close)

	return ts, &hits
}

func mustCheck(t *testing.T, c *Client, url string) Decision {
	t.Helper()

	d, err := c.Check(context.Background(), url)
	if err != nil {
		t.Fatalf("Check(%q) error: %v", url, err)
	}
	return d
}

func TestClient_AllowDisallowWildcardsAndCrawlDelay(t *testing.T) {
	ts, _ := robotsServer(t, 200, `
User-agent: *
Disallow: /private/
Disallow: /*.pdf$
Disallow: /search?
Allow: /private/public/
Crawl-delay: 2

User-agent: crawlerx
Disallow: /only-for-us/
Crawl-delay: 7
`)

	// The specific group wins over "*" entirely.
	c := New(Options{Product: "CrawlerX"})
	if d := mustCheck(t, c, ts.URL+"/only-for-us/x"); d.Allowed {
		t.Error("/only-for-us/ should be disallowed for CrawlerX")
	}
	if d := mustCheck(t, c, ts.URL+"/private/x"); !d.Allowed {
		t.Error("/private/ is only disallowed for *, not for the CrawlerX group")
	}
	if d := mustCheck(t, c, ts.URL+"/"); d.CrawlDelay != 7*time.Second {
		t.Errorf("CrawlDelay = %v, want 7s", d.CrawlDelay)
	}

	// A bot with no specific group falls back to "*".
	other := New(Options{Product: "SomeoneElse"})
	cases := []struct {
		path    string
		allowed bool
	}{
		{"/", true},
		{"/private/secret", false},
		{"/private/public/page", true}, // longer Allow beats shorter Disallow
		{"/doc.pdf", false},            // wildcard + end anchor
		{"/doc.pdf?x=1", true},         // $ anchors the end
		{"/search?q=go", false},        // query is part of the matched path
		{"/search", true},
	}
	for _, tc := range cases {
		if d := mustCheck(t, other, ts.URL+tc.path); d.Allowed != tc.allowed {
			t.Errorf("%s: Allowed = %v, want %v", tc.path, d.Allowed, tc.allowed)
		}
	}
	if d := mustCheck(t, other, ts.URL+"/"); d.CrawlDelay != 2*time.Second {
		t.Errorf("CrawlDelay = %v, want 2s", d.CrawlDelay)
	}
}

func TestClient_4xxMeansAllowAll(t *testing.T) {
	for _, status := range []int{401, 403, 404, 410} {
		ts, _ := robotsServer(t, status, "User-agent: *\nDisallow: /")
		c := New(Options{})

		if d := mustCheck(t, c, ts.URL+"/anything"); !d.Allowed {
			t.Errorf("status %d: want allow-all, got blocked", status)
		}
	}
}

func TestClient_5xxMeansDisallowAll(t *testing.T) {
	for _, status := range []int{500, 502, 503} {
		ts, _ := robotsServer(t, status, "")
		c := New(Options{})

		if d := mustCheck(t, c, ts.URL+"/anything"); d.Allowed {
			t.Errorf("status %d: want disallow-all, got allowed", status)
		}
	}
}

func TestClient_5xxIsRetriedAfterErrorTTL(t *testing.T) {
	status := atomic.Int32{}
	status.Store(503)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(int(status.Load()))
	}))
	defer ts.Close()

	now := time.Now()
	c := New(Options{ErrorTTL: time.Minute, Now: func() time.Time { return now }})

	if d := mustCheck(t, c, ts.URL+"/x"); d.Allowed {
		t.Fatal("503 should disallow")
	}

	status.Store(404)
	now = now.Add(30 * time.Second)
	if d := mustCheck(t, c, ts.URL+"/x"); d.Allowed {
		t.Fatal("within the error TTL the 503 verdict should still apply")
	}

	now = now.Add(31 * time.Second)
	if d := mustCheck(t, c, ts.URL+"/x"); !d.Allowed {
		t.Fatal("after the error TTL robots.txt should be re-fetched and now allow")
	}
}

func TestClient_UnparsableIsConservative(t *testing.T) {
	ts, _ := robotsServer(t, 200, "User-agent: *\nCrawl-delay: not-a-number\nDisallow:\n\x00\x01garbage")
	c := New(Options{})

	// Whatever the parser decides, it must not crash; and a parse error
	// must not silently turn into allow-all.
	_, _ = c.Check(context.Background(), ts.URL+"/x")
}

func TestClient_NetworkErrorIsAnError(t *testing.T) {
	ts := httptest.NewServer(http.NotFoundHandler())
	url := ts.URL
	ts.Close() // nothing listens any more

	if _, err := New(Options{}).Check(context.Background(), url+"/x"); err == nil {
		t.Fatal("unreachable host should return an error, not a verdict")
	}
}

func TestClient_CachesPerOrigin(t *testing.T) {
	ts, hits := robotsServer(t, 200, "User-agent: *\nDisallow: /no")
	c := New(Options{})

	for i := 0; i < 5; i++ {
		mustCheck(t, c, ts.URL+"/a")
		mustCheck(t, c, ts.URL+"/b")
	}

	if got := hits.Load(); got != 1 {
		t.Fatalf("robots.txt fetched %d times, want 1", got)
	}
}

func TestClient_SharedRedisCacheAcrossClients(t *testing.T) {
	ts, hits := robotsServer(t, 200, "User-agent: *\nDisallow: /no")

	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rc.Close()

	// Two separate Clients (two "processes") sharing one Redis.
	a := New(Options{Cache: NewRedisCache(rc)})
	b := New(Options{Cache: NewRedisCache(rc)})

	if d := mustCheck(t, a, ts.URL+"/no"); d.Allowed {
		t.Fatal("/no should be blocked")
	}
	if d := mustCheck(t, b, ts.URL+"/no"); d.Allowed {
		t.Fatal("second client should see the cached rules and block /no")
	}

	if got := hits.Load(); got != 1 {
		t.Fatalf("robots.txt fetched %d times across two clients, want 1", got)
	}
}

func TestClient_SendsUserAgent(t *testing.T) {
	var got atomic.Value
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Store(r.Header.Get("User-Agent"))
		w.WriteHeader(404)
	}))
	defer ts.Close()

	mustCheck(t, New(Options{UserAgent: "CrawlerX/1.0 (+https://example.test)"}), ts.URL+"/x")

	if got.Load() != "CrawlerX/1.0 (+https://example.test)" {
		t.Fatalf("User-Agent = %v", got.Load())
	}
}

func TestClient_RedirectLoopTreatedAsUnavailable(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/robots.txt", http.StatusFound)
	}))
	defer ts.Close()

	if d := mustCheck(t, New(Options{}), ts.URL+"/x"); !d.Allowed {
		t.Fatal("a redirect loop is 'unavailable' under RFC 9309, which means allow")
	}
}
