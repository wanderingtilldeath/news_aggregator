package crawler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/frontier"
	"github.com/wanderingtilldeath/news-aggregator/internal/parse"
	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

const para = "The council announced a new plan on Monday that covers transport and housing across the whole region."

// newsSite serves a dated day page that links to two undated article pages, and
// a multi-topic page whose topics are <article id> blocks.
func newsSite() *httptest.Server {
	mux := http.NewServeMux()
	page := func(body string) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, "<!doctype html><html><head><title>t</title></head><body>"+body+"</body></html>")
		}
	}
	mux.HandleFunc("/day/19-10-2026", page(`<h1>Day</h1><a href="/items/one">one</a><a href="/items/two">two</a>`))
	mux.HandleFunc("/items/one", page(`<article><h1>Item one</h1><p>`+para+` <a href="https://news.example/story-one">Source</a></p></article>`))
	mux.HandleFunc("/items/two", page(`<article><h1>Item two</h1><p>`+para+`</p></article>`))
	mux.HandleFunc("/digest/19-10-2026", page(
		`<h1>Daily</h1>`+
			`<article id="gs3-ca1"><h2>Topic A</h2><img src="/img/a.png"><p>`+para+` <a href="https://news.example/topic-a">Source</a></p></article>`+
			`<article id="cme-ca2"><h2>Topic B</h2><p>`+para+` Second.</p></article>`))
	return httptest.NewServer(mux)
}

func TestArticlePrefixesMarkOnlyMatchingPagesAsArticles(t *testing.T) {
	ts := newsSite()
	defer ts.Close()

	h := newHarness(t, testConfig(), func(d *Deps) { d.Parser = parse.HTMLParser{} })
	spec := frontier.CrawlSpec{Seed: ts.URL + "/day/19-10-2026", MaxDepth: 1, ArticlePrefixes: []string{"/items/"}}

	done := make(chan error, 1)
	go func() { _, err := h.c.CrawlSpec(t.Context(), spec); done <- err }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("crawl did not finish")
	}

	pages := h.store.Pages()
	for _, path := range []string{"/items/one", "/items/two"} {
		if p, ok := pages[ts.URL+path]; !ok || !p.IsArticle {
			t.Errorf("%s = %+v, %v; want a stored article even though it has no date", path, p, ok)
		}
	}
	// The day page has a date in its URL, which would normally make it an article.
	if p, ok := pages[ts.URL+"/day/19-10-2026"]; !ok || p.IsArticle {
		t.Errorf("day page = %+v, %v; want a stored non-article", p, ok)
	}
}

func TestWithoutArticlePrefixesThePageSignalsDecide(t *testing.T) {
	ts := newsSite()
	defer ts.Close()

	h := newHarness(t, testConfig(), func(d *Deps) { d.Parser = parse.HTMLParser{} })
	h.crawl(t, ts.URL+"/day/19-10-2026", 1)

	pages := h.store.Pages()
	if !pages[ts.URL+"/day/19-10-2026"].IsArticle {
		t.Error("with no rules, a page dated by its URL counts as an article (the default)")
	}
	if pages[ts.URL+"/items/one"].IsArticle {
		t.Error("with no rules, an undated page is not an article")
	}
}

func TestSplitArticlesStoresEachTopicAsItsOwnItem(t *testing.T) {
	ts := newsSite()
	defer ts.Close()

	h := newHarness(t, testConfig(), func(d *Deps) { d.Parser = parse.HTMLParser{} })
	spec := frontier.CrawlSpec{Seed: ts.URL + "/digest/19-10-2026", MaxDepth: 0, SplitArticles: true}

	done := make(chan error, 1)
	go func() { _, err := h.c.CrawlSpec(t.Context(), spec); done <- err }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("crawl did not finish")
	}

	pages := h.store.Pages()
	for id, title := range map[string]string{"gs3-ca1": "Topic A", "cme-ca2": "Topic B"} {
		p, ok := pages[ts.URL+"/digest/19-10-2026#"+id]
		if !ok || !p.IsArticle || p.Title != title || !strings.HasPrefix(p.Content, "The council announced") {
			t.Errorf("topic %s = %+v, %v", id, p, ok)
		}
		if p.URLHash != frontier.Hash(p.URL) || p.URLHash == "" {
			t.Errorf("topic %s has hash %q, want the hash of its own URL", id, p.URLHash)
		}
	}
	if p := pages[ts.URL+"/digest/19-10-2026"]; p.IsArticle {
		t.Errorf("the container page must not be an article: %+v", p)
	}
	if len(pages) != 3 {
		t.Errorf("%d stored pages, want the container plus 2 topics", len(pages))
	}

	// Without split_articles the same page is one ordinary item.
	h2 := newHarness(t, testConfig(), func(d *Deps) { d.Parser = parse.HTMLParser{} })
	h2.crawl(t, ts.URL+"/digest/19-10-2026", 0)
	if n := len(h2.store.Pages()); n != 1 {
		t.Errorf("without splitting, %d pages stored, want 1", n)
	}
}

func TestImageFromSourceStoresTheCitedURLOnlyWhenAsked(t *testing.T) {
	ts := newsSite()
	defer ts.Close()

	run := func(imageFromSource bool) map[string]store.Page {
		h := newHarness(t, testConfig(), func(d *Deps) { d.Parser = parse.HTMLParser{} })
		spec := frontier.CrawlSpec{Seed: ts.URL + "/day/19-10-2026", MaxDepth: 1, ArticlePrefixes: []string{"/items/"}, ImageFromSource: imageFromSource}

		done := make(chan error, 1)
		go func() { _, err := h.c.CrawlSpec(t.Context(), spec); done <- err }()
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("crawl did not finish")
		}
		return h.store.Pages()
	}

	if got := run(true)[ts.URL+"/items/one"].SourceURL; got != "https://news.example/story-one" {
		t.Errorf("SourceURL = %q, want the cited story", got)
	}
	if got := run(false)[ts.URL+"/items/one"].SourceURL; got != "" {
		t.Errorf("SourceURL = %q, want none when the crawl did not ask for source images", got)
	}
}

func TestSplitTopicsKeepTheirOwnImageAndSource(t *testing.T) {
	ts := newsSite()
	defer ts.Close()

	h := newHarness(t, testConfig(), func(d *Deps) { d.Parser = parse.HTMLParser{} })
	spec := frontier.CrawlSpec{Seed: ts.URL + "/digest/19-10-2026", MaxDepth: 0, SplitArticles: true, ImageFromSource: true}

	done := make(chan error, 1)
	go func() { _, err := h.c.CrawlSpec(t.Context(), spec); done <- err }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("crawl did not finish")
	}

	a := h.store.Pages()[ts.URL+"/digest/19-10-2026#gs3-ca1"]
	if a.Image != ts.URL+"/img/a.png" {
		t.Errorf("topic image = %q, want the topic's own <img>", a.Image)
	}
	if a.SourceURL != "https://news.example/topic-a" {
		t.Errorf("topic source = %q", a.SourceURL)
	}
	if b := h.store.Pages()[ts.URL+"/digest/19-10-2026#cme-ca2"]; b.Image != "" || b.SourceURL != "" {
		t.Errorf("a topic with no <img> and no link has neither: %+v", b)
	}
}
