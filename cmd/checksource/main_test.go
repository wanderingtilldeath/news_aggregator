package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/robots"
)

type robotsStub struct{ allow bool }

func (r robotsStub) Check(context.Context, string) (robots.Decision, error) {
	return robots.Decision{Allowed: r.allow}, nil
}

const articleBody = `<p>The council announced a new plan on Monday that covers transport and housing across the whole region, officials said.</p>` +
	`<p>It will take effect next year and affect forty thousand residents, according to a statement released this morning.</p>` +
	`<p>Critics argued the proposal does not go far enough on cost, while supporters welcomed the clear timeline for delivery.</p>`

// blog serves a listing with dated, illustrated articles under /blog/ plus
// navigation, and a second kind of site pieces as the tests need them.
func blog(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><body><a href="/about">About</a><a href="/blog/">Blog</a>`)
		for i := 1; i <= 8; i++ {
			fmt.Fprintf(w, `<a href="/blog/story-number-%d-about-the-council-plan">story %d</a>`, i, i)
		}
		fmt.Fprint(w, `</body></html>`)
	})
	mux.HandleFunc("/blog/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<html><head><meta property="article:published_time" content="2026-10-19T08:00:00Z">`+
			`<meta property="og:image" content="/img%s.jpg"><title>t</title></head><body><article>%s</article></body></html>`,
			strings.TrimSuffix(r.URL.Path, "/"), articleBody)
	})
	mux.HandleFunc("/nodate/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<html><body><article>%s</article></body></html>`, articleBody)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func run(t *testing.T, url string, o options) (report, string) {
	t.Helper()
	if o.Robots == nil {
		o.Robots = robotsStub{allow: true}
	}
	o.Delay = time.Millisecond
	var out bytes.Buffer
	return check(context.Background(), url, o, &out), out.String()
}

func TestWorksForADatedIllustratedBlog(t *testing.T) {
	srv := blog(t)

	rep, out := run(t, srv.URL+"/", options{})
	if rep.Verdict != verdictWorks {
		t.Fatalf("verdict %s (%v)\n%s", rep.Verdict, rep.Reasons, out)
	}
	if len(rep.Prefixes) != 1 || rep.Prefixes[0] != "/blog/" {
		t.Errorf("suggested prefixes = %v, want [/blog/]", rep.Prefixes)
	}
	for _, s := range rep.Samples {
		if !s.Article || s.Date != "2026-10-19" || s.Runes < 300 || !strings.Contains(s.Image, "/img/blog/") {
			t.Errorf("sample = %+v", s)
		}
	}
	if !strings.Contains(out, `"path_prefixes"`) || !strings.Contains(out, "/blog/") {
		t.Errorf("output should end with a sources.json entry:\n%s", out)
	}
}

func TestSamplesAreArticlesNotIndexPages(t *testing.T) {
	srv := blog(t)

	rep, _ := run(t, srv.URL+"/", options{Prefixes: []string{"/blog/"}, Samples: 3})
	for _, s := range rep.Samples {
		if strings.HasSuffix(s.URL, "/blog/") {
			t.Errorf("sampled the section index %s", s.URL)
		}
	}
}

func TestUndatedPagesNeedArticlePrefixes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><body>`)
		for i := 1; i <= 6; i++ {
			fmt.Fprintf(w, `<a href="/nodate/story-number-%d-about-the-council">s</a>`, i)
		}
		fmt.Fprint(w, `</body></html>`)
	})
	mux.HandleFunc("/nodate/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<html><body><article>%s</article></body></html>`, articleBody)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	rep, _ := run(t, srv.URL+"/", options{})
	if rep.Verdict != verdictPartial || !strings.Contains(strings.Join(rep.Reasons, " "), "article_prefixes") {
		t.Errorf("without a rule: %s %v, want PARTIAL asking for article_prefixes", rep.Verdict, rep.Reasons)
	}

	rep, _ = run(t, srv.URL+"/", options{ArticlePrefixes: []string{"/nodate/"}})
	if rep.Verdict == verdictNo || strings.Contains(strings.Join(rep.Reasons, " "), "no machine-readable date") {
		t.Errorf("with -article-prefix: %s %v, want the date complaint gone", rep.Verdict, rep.Reasons)
	}
}

func TestNoWhenRobotsBlocksOrTheSiteChallenges(t *testing.T) {
	srv := blog(t)
	if rep, out := run(t, srv.URL+"/", options{Robots: robotsStub{allow: false}}); rep.Verdict != verdictNo || !strings.Contains(out, "robots.txt disallows") {
		t.Errorf("robots-blocked: %s\n%s", rep.Verdict, out)
	}

	challenge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer challenge.Close()
	if rep, out := run(t, challenge.URL+"/", options{}); rep.Verdict != verdictNo || !strings.Contains(out, "HTTP 403") {
		t.Errorf("403: %s\n%s", rep.Verdict, out)
	}

	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `<html><head><title>Just a moment...</title></head><body>Checking your browser before accessing</body></html>`)
	}))
	defer page.Close()
	if rep, out := run(t, page.URL+"/", options{}); rep.Verdict != verdictNo || !strings.Contains(out, "bot-challenge") {
		t.Errorf("challenge page: %s\n%s", rep.Verdict, out)
	}

	empty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `<html><body><div id="app"></div></body></html>`)
	}))
	defer empty.Close()
	if rep, out := run(t, empty.URL+"/", options{}); rep.Verdict != verdictNo || !strings.Contains(out, "JavaScript") {
		t.Errorf("JS-only page: %s\n%s", rep.Verdict, out)
	}
}

func TestSeedRedirectIsReported(t *testing.T) {
	srv := blog(t)
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL+"/", http.StatusMovedPermanently)
	}))
	defer redirect.Close()

	_, out := run(t, redirect.URL+"/", options{})
	if !strings.Contains(out, "redirects to") {
		t.Errorf("a redirecting seed must be called out so the right URL goes in sources.json:\n%s", out)
	}
}

func TestSuggestPrefixes(t *testing.T) {
	links := []string{
		"https://x.test/blog/a-long-slug-with-many-words", "https://x.test/blog/another-long-slug-here-too", "https://x.test/blog/third-slug-with-hyphens-in-it",
		"https://x.test/news/2026/10/19/something", "https://x.test/news/2026/10/18/other", "https://x.test/news/2026/10/17/third",
		"https://x.test/about", "https://x.test/contact",
	}
	got := suggestPrefixes(links)
	if len(got) != 2 || got[0] != "/blog/" || got[1] != "/news/" {
		t.Errorf("suggestPrefixes = %v, want [/blog/ /news/]", got)
	}

	root := []string{"https://x.test/a-long-slug-with-many-words", "https://x.test/another-long-slug-here-too", "https://x.test/third-slug-with-hyphens-in-it"}
	if got := suggestPrefixes(root); len(got) != 1 || got[0] != "/" {
		t.Errorf("root-level slugs = %v, want [/]", got)
	}

	if got := suggestPrefixes([]string{"https://x.test/about", "https://x.test/contact"}); got != nil {
		t.Errorf("no article-like links = %v, want none", got)
	}
}

func TestJudgeReasons(t *testing.T) {
	good := sample{Article: true, Runes: 2000, Image: "https://x.test/a.jpg"}
	other := sample{Article: true, Runes: 2000, Image: "https://x.test/b.jpg"}

	if v, _ := judge([]sample{good, other, good}, false); v != verdictWorks {
		t.Errorf("good samples = %s", v)
	}
	if v, r := judge([]sample{{Runes: 2000}, {Runes: 2000}, {Runes: 2000}}, false); v != verdictPartial || !strings.Contains(strings.Join(r, " "), "article_prefixes") {
		t.Errorf("undated = %s %v", v, r)
	}
	if v, r := judge([]sample{{Article: true, Runes: 50}, {Article: true, Runes: 80}, {Article: true, Runes: 40}}, false); v != verdictPartial || !strings.Contains(strings.Join(r, " "), "thin") {
		t.Errorf("thin = %s %v", v, r)
	}
	logo := sample{Article: true, Runes: 2000, Image: "https://x.test/logo.png"}
	if _, r := judge([]sample{logo, logo, logo}, false); !strings.Contains(strings.Join(r, " "), "same image") {
		t.Errorf("shared image not flagged: %v", r)
	}
	if v, _ := judge([]sample{{Note: "boom"}, {Note: "boom"}}, false); v != verdictNo {
		t.Errorf("nothing fetched = %s", v)
	}
}

func TestAIPolicyOf(t *testing.T) {
	robotsTxt := `
User-agent: *
Disallow: /private/

# AI crawlers
User-agent: GPTBot
User-agent: ClaudeBot
Disallow: /

User-agent: Googlebot
Allow: /

User-agent: CCBot
Disallow: /archive/

Content-Signal: ai-train=no, ai-input=yes
`
	got := aiPolicyOf(robotsTxt)
	if !strings.Contains(got, "claudebot, gptbot") {
		t.Errorf("policy = %q, want ClaudeBot and GPTBot reported as disallowed from the whole site", got)
	}
	if strings.Contains(got, "ccbot") {
		t.Errorf("CCBot is only kept out of /archive/, not the whole site: %q", got)
	}
	if !strings.Contains(got, "Content-Signal: ai-train=no, ai-input=yes") {
		t.Errorf("policy = %q, want the Content-Signal line", got)
	}

	if got := aiPolicyOf("User-agent: *\nAllow: /\n"); got != "" {
		t.Errorf("an open robots.txt gave %q", got)
	}
	// A rule for a bot does not leak into the next group.
	if got := aiPolicyOf("User-agent: GPTBot\nAllow: /\nUser-agent: Foo\nDisallow: /\n"); got != "" {
		t.Errorf("GPTBot is allowed here, got %q", got)
	}
}

func TestVerdictCarriesTheAIPolicyNote(t *testing.T) {
	srv := blog(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "User-agent: GPTBot\nDisallow: /\n")
	})
	mux.Handle("/", srv.Config.Handler)
	front := httptest.NewServer(mux)
	defer front.Close()

	rep, out := run(t, front.URL+"/", options{})
	if rep.AINote == "" || !strings.Contains(out, "gptbot") || !strings.Contains(strings.Join(rep.Reasons, " "), "AI policy") {
		t.Errorf("AI note missing: %q\n%s", rep.AINote, out)
	}
}
