package fakesite

import (
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

var hrefRE = regexp.MustCompile(`href="([^"]+)"`)

func get(t *testing.T, ts *httptest.Server, path string) (int, string) {
	t.Helper()

	resp, err := http.Get(ts.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func TestEveryPageIsReachableFromTheRoot(t *testing.T) {
	site := New(Options{Pages: 300, Fanout: 4})
	ts := httptest.NewServer(site)
	defer ts.Close()

	seen := map[string]bool{"/": true}
	queue := []string{"/"}

	for len(queue) > 0 {
		path := queue[0]
		queue = queue[1:]

		status, body := get(t, ts, path)
		if status != 200 {
			t.Fatalf("GET %s = %d", path, status)
		}

		for _, m := range hrefRE.FindAllStringSubmatch(body, -1) {
			link := m[1]
			if i := strings.IndexAny(link, "?#"); i >= 0 {
				link = link[:i]
			}
			if (link == "/" || strings.HasPrefix(link, "/p/")) && !seen[link] {
				seen[link] = true
				queue = append(queue, link)
			}
		}
	}

	if len(seen) != 300 {
		t.Fatalf("reached %d pages from /, want all 300", len(seen))
	}
}

func TestSiteIsDeterministic(t *testing.T) {
	a, b := httptest.NewServer(New(Options{Seed: 7})), httptest.NewServer(New(Options{Seed: 7}))
	defer a.Close()
	defer b.Close()

	_, bodyA := get(t, a, "/p/5")
	_, bodyB := get(t, b, "/p/5")
	if bodyA != bodyB {
		t.Fatal("same seed produced different pages")
	}

	c := httptest.NewServer(New(Options{Seed: 8}))
	defer c.Close()
	if _, bodyC := get(t, c, "/p/5"); bodyC == bodyA {
		t.Fatal("different seeds produced identical pages")
	}
}

func TestRobotsAndPrivate(t *testing.T) {
	ts := httptest.NewServer(New(Options{CrawlDelay: 3}))
	defer ts.Close()

	_, robots := get(t, ts, "/robots.txt")
	if !strings.Contains(robots, "Disallow: /private/") || !strings.Contains(robots, "Crawl-delay: 3") {
		t.Fatalf("robots.txt = %q", robots)
	}
}

func TestOutOfRangeAndChaosGatedByOption(t *testing.T) {
	ts := httptest.NewServer(New(Options{Pages: 10}))
	defer ts.Close()

	if status, _ := get(t, ts, "/p/10"); status != 404 {
		t.Errorf("/p/10 = %d, want 404", status)
	}
	if status, _ := get(t, ts, "/x/flaky/1"); status != 404 {
		t.Errorf("chaos endpoint without Chaos = %d, want 404", status)
	}
}

func TestChaosEndpoints(t *testing.T) {
	ts := httptest.NewServer(New(Options{Chaos: true}))
	defer ts.Close()

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	status := func(path string) int {
		resp, err := client.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	for i, want := range []int{503, 503, 200} {
		if got := status("/x/flaky/9"); got != want {
			t.Errorf("flaky call %d = %d, want %d", i+1, got, want)
		}
	}
	if got := status("/x/ratelimit"); got != 429 {
		t.Errorf("first ratelimit = %d, want 429", got)
	}
	if got := status("/x/ratelimit"); got != 200 {
		t.Errorf("second ratelimit = %d, want 200", got)
	}
	for path, want := range map[string]int{"/x/redirect": 301, "/x/loop/a": 302, "/x/pdf": 200, "/x/gone": 410, "/x/error": 500} {
		if got := status(path); got != want {
			t.Errorf("%s = %d, want %d", path, got, want)
		}
	}

	resp, _ := client.Get(ts.URL + "/x/huge")
	n, _ := io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if n < 8<<20 {
		t.Errorf("/x/huge sent %d bytes, want >= 8 MiB", n)
	}
}

func TestRequestsAreRecorded(t *testing.T) {
	site := New(Options{})
	ts := httptest.NewServer(site)
	defer ts.Close()

	get(t, ts, "/")
	get(t, ts, "/p/1")
	get(t, ts, "/p/1")

	if got := site.PageRequests(); got[ts.Listener.Addr().String()+"/p/1"] != 2 {
		t.Fatalf("PageRequests = %v", got)
	}
}

func TestNewsSection(t *testing.T) {
	ts := httptest.NewServer(New(Options{}))
	defer ts.Close()

	status, listing := get(t, ts, "/news/")
	if status != 200 || strings.Count(listing, `href="/news/a/`) != NewsArticles || !strings.Contains(listing, "/news/archive/") {
		t.Fatalf("listing = %d %s", status, listing)
	}

	status, article := get(t, ts, "/news/a/1")
	for _, want := range []string{`og:type" content="article"`, `og:image" content="` + ts.URL + `/img/1.jpg"`, "article:published_time", "<p>The council"} {
		if status != 200 || !strings.Contains(article, want) {
			t.Errorf("article missing %q (status %d)", want, status)
		}
	}

	_, archive := get(t, ts, "/news/archive/")
	if !strings.Contains(archive, "<time datetime") || strings.Contains(archive, "og:type") {
		t.Errorf("archive should be a dated listing, not an article: %s", archive)
	}

	for _, p := range []string{"/news/a/0", "/news/a/6", "/news/a/x"} {
		if status, _ := get(t, ts, p); status != 404 {
			t.Errorf("%s = %d, want 404", p, status)
		}
	}
}

func TestDigestPages(t *testing.T) {
	ts := httptest.NewServer(New(Options{}))
	defer ts.Close()

	_, day := get(t, ts, "/day/19-10-2026")
	if strings.Count(day, `href="/items/`) != DayItems {
		t.Errorf("day page should link to %d items: %s", DayItems, day)
	}

	status, item := get(t, ts, "/items/2")
	if status != 200 || !strings.Contains(item, `og:type" content="article"`) || strings.Contains(item, "published_time") || !strings.Contains(item, "<p>The council") {
		t.Errorf("item should be an undated article (status %d): %s", status, item)
	}

	if !strings.Contains(item, `href="http://localhost:`) || !strings.Contains(item, "/source/2") {
		t.Errorf("item should cite a source on a different host name: %s", item)
	}
	if status, source := get(t, ts, "/source/2"); status != 200 || !strings.Contains(source, `og:image" content="/img/source-2.jpg"`) {
		t.Errorf("source page = %d %s", status, source)
	}

	_, topics := get(t, ts, "/topics/")
	if strings.Count(topics, "<article id=") != TopicCount || !strings.Contains(topics, "article:published_time") || !strings.Contains(topics, `<img src="/img/topic-1.png">`) {
		t.Errorf("topics page: %s", topics)
	}

	for _, p := range []string{"/items/0", "/items/9", "/items/x", "/source/0", "/source/9"} {
		if status, _ := get(t, ts, p); status != 404 {
			t.Errorf("%s = %d, want 404", p, status)
		}
	}
}
