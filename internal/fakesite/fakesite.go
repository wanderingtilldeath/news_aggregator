// Package fakesite generates a deterministic synthetic website for testing
// and benchmarking the crawler without touching anyone else's servers.
//
// Page "/" and "/p/{i}" form a link graph with Pages nodes: a Fanout-ary
// tree (so every page is reachable) plus a few pseudo-random cross links.
// With Chaos set, page 0 also links to endpoints that misbehave on purpose
// (flaky, rate-limited, redirecting, huge, non-HTML), which the integration
// tests and the demo use.
package fakesite

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Options configures a Site. Zero values give a small, fast site.
type Options struct {
	// Pages is the number of pages in the graph (default 100).
	Pages int
	// Fanout is the number of tree children per page (default 5).
	Fanout int
	// Seed varies the generated text and cross links (default 1).
	Seed uint64
	// Latency is added to every page response.
	Latency time.Duration
	// CrawlDelay, when > 0, is advertised in robots.txt (seconds).
	CrawlDelay int
	// Chaos adds the misbehaving endpoints and links to them from page 0.
	Chaos bool
	// Log, when set, receives one JSON object per request.
	Log io.Writer
}

// Request is one recorded request.
type Request struct {
	Time   time.Time `json:"time"`
	Host   string    `json:"host"`
	Path   string    `json:"path"`
	Query  string    `json:"query,omitempty"`
	Status int       `json:"status"`
	UA     string    `json:"ua,omitempty"`
}

// Site is an http.Handler serving the synthetic site.
type Site struct {
	opts Options

	mu       sync.Mutex
	requests []Request
	flaky    map[string]int
	logMu    sync.Mutex

	rateLimited atomic.Bool
}

// New builds a Site.
func New(opts Options) *Site {
	if opts.Pages <= 0 {
		opts.Pages = 100
	}
	if opts.Fanout <= 0 {
		opts.Fanout = 5
	}
	if opts.Seed == 0 {
		opts.Seed = 1
	}

	return &Site{opts: opts, flaky: make(map[string]int)}
}

// Pages returns the number of pages in the graph.
func (s *Site) Pages() int { return s.opts.Pages }

// Requests returns a copy of every request seen so far.
func (s *Site) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]Request(nil), s.requests...)
}

// PageRequests counts requests to /p/* and "/" per path+host key.
func (s *Site) PageRequests() map[string]int {
	counts := make(map[string]int)
	for _, r := range s.Requests() {
		if r.Path == "/" || strings.HasPrefix(r.Path, "/p/") {
			counts[r.Host+r.Path]++
		}
	}
	return counts
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (s *Site) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
	defer func() {
		s.record(Request{Time: time.Now(), Host: r.Host, Path: r.URL.Path, Query: r.URL.RawQuery, Status: sw.status, UA: r.UserAgent()})
	}()

	switch {
	case r.URL.Path == "/robots.txt":
		s.robots(sw)
	case r.URL.Path == "/":
		s.page(sw, 0)
	case strings.HasPrefix(r.URL.Path, "/p/"):
		i, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/p/"))
		if err != nil || i < 0 || i >= s.opts.Pages {
			http.NotFound(sw, r)
			return
		}
		s.page(sw, i)
	case strings.HasPrefix(r.URL.Path, "/private/"):
		writeHTML(sw, "<title>Private</title><p>robots.txt disallows this</p>")
	case strings.HasPrefix(r.URL.Path, "/x/") && s.opts.Chaos:
		s.chaos(sw, r)
	default:
		http.NotFound(sw, r)
	}
}

func (s *Site) record(req Request) {
	s.mu.Lock()
	s.requests = append(s.requests, req)
	s.mu.Unlock()

	if s.opts.Log != nil {
		s.logMu.Lock()
		_ = json.NewEncoder(s.opts.Log).Encode(req)
		s.logMu.Unlock()
	}
}

func (s *Site) robots(w http.ResponseWriter) {
	var b strings.Builder
	b.WriteString("User-agent: *\nDisallow: /private/\n")
	if s.opts.CrawlDelay > 0 {
		fmt.Fprintf(&b, "Crawl-delay: %d\n", s.opts.CrawlDelay)
	}
	w.Header().Set("Content-Type", "text/plain")
	_, _ = io.WriteString(w, b.String())
}

func (s *Site) page(w http.ResponseWriter, i int) {
	if s.opts.Latency > 0 {
		// 75%..125% of the configured latency, deterministic per page.
		jitter := 0.75 + float64(mix(s.opts.Seed, uint64(i), 99)%500)/1000
		time.Sleep(time.Duration(float64(s.opts.Latency) * jitter))
	}

	var b strings.Builder
	fmt.Fprintf(&b, "<!doctype html><html><head><title>Page %d %s</title>", i, word(s.opts.Seed, i, 0))
	fmt.Fprintf(&b, `<meta name="description" content="Synthetic page %d about %s and %s">`, i, word(s.opts.Seed, i, 1), word(s.opts.Seed, i, 2))
	fmt.Fprintf(&b, "</head><body><h1>%s %s %d</h1><p>", word(s.opts.Seed, i, 3), word(s.opts.Seed, i, 4), i)
	for k := 5; k < 45; k++ {
		b.WriteString(word(s.opts.Seed, i, k))
		b.WriteByte(' ')
	}
	b.WriteString("</p><ul>")

	for _, target := range s.links(i) {
		fmt.Fprintf(&b, `<li><a href="%s">%s</a></li>`, target, target)
	}
	b.WriteString("</ul></body></html>")

	writeHTML(w, b.String())
}

// links returns the hrefs on page i.
func (s *Site) links(i int) []string {
	var out []string
	href := func(n int) string {
		if n == 0 {
			return "/"
		}
		return "/p/" + strconv.Itoa(n)
	}

	// Tree edges guarantee every page is reachable from page 0.
	for k := 1; k <= s.opts.Fanout; k++ {
		if child := i*s.opts.Fanout + k; child < s.opts.Pages {
			out = append(out, href(child))
		}
	}

	// Two pseudo-random cross links (these create cycles and duplicates).
	for k := 0; k < 2; k++ {
		out = append(out, href(int(mix(s.opts.Seed, uint64(i), uint64(100+k))%uint64(s.opts.Pages))))
	}

	// Spellings of an existing link that canonicalise to the same URL.
	if i%7 == 0 && s.opts.Pages > 1 {
		out = append(out, "/p/1?utm_source=newsletter", "/p/1#top")
	}

	if i == 0 {
		out = append(out, "/private/secret")
		if s.opts.Chaos {
			out = append(out,
				"/x/flaky/1", "/x/flaky/2", "/x/ratelimit", "/x/redirect", "/x/loop/a", "/x/huge",
				"/x/pdf", "/x/gone", "/x/error", "/x/noindex", "/x/nofollow")
		}
	}

	return out
}

func (s *Site) chaos(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/x/")

	switch {
	case strings.HasPrefix(path, "flaky/"):
		// Fails twice with 503, then works: exercises retry with backoff.
		s.mu.Lock()
		s.flaky[path]++
		n := s.flaky[path]
		s.mu.Unlock()
		if n <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writeHTML(w, "<title>Flaky but fine</title><p>recovered after retries</p>")

	case path == "ratelimit":
		// 429 with Retry-After the first time only.
		if s.rateLimited.CompareAndSwap(false, true) {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		writeHTML(w, "<title>Rate limited once</title>")

	case path == "redirect":
		http.Redirect(w, r, "/p/1", http.StatusMovedPermanently)

	case path == "loop/a":
		http.Redirect(w, r, "/x/loop/b", http.StatusFound)
	case path == "loop/b":
		http.Redirect(w, r, "/x/loop/a", http.StatusFound)

	case path == "huge":
		// ~8 MB of HTML, far over the crawler's size cap.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, `<html><head><title>Huge page</title></head><body><a href="/p/2">early link</a>`)
		chunk := strings.Repeat("lorem ipsum dolor sit amet ", 400)
		for written := 0; written < 8<<20; written += len(chunk) {
			if _, err := io.WriteString(w, chunk); err != nil {
				return
			}
		}

	case path == "pdf":
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = io.WriteString(w, "%PDF-1.4 not really")

	case path == "gone":
		http.Error(w, "gone", http.StatusGone)
	case path == "error":
		http.Error(w, "always broken", http.StatusInternalServerError)

	case path == "noindex":
		writeHTML(w, `<meta name="robots" content="noindex"><title>Hidden</title><a href="/p/3">still followed</a>`)
	case path == "nofollow":
		writeHTML(w, `<meta name="robots" content="nofollow"><title>Leaf</title><a href="/p/4">not followed from here</a>`)

	default:
		http.NotFound(w, r)
	}
}

func writeHTML(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, body)
}

var vocabulary = strings.Fields(`alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu nu xi omicron pi
rho sigma tau upsilon phi chi psi omega river mountain forest ocean desert valley island meadow canyon glacier
harbor lantern compass anchor falcon otter badger heron willow cedar maple birch quartz copper amber cobalt
ember frost thunder breeze orbit comet nebula quasar photon vector matrix kernel socket thread mutex channel`)

func word(seed uint64, page, slot int) string {
	return vocabulary[mix(seed, uint64(page), uint64(slot))%uint64(len(vocabulary))]
}

// mix is a small deterministic hash (splitmix64 finaliser) so the same seed
// always generates the same site.
func mix(a, b, c uint64) uint64 {
	x := a*0x9E3779B97F4A7C15 ^ b*0xBF58476D1CE4E5B9 ^ c*0x94D049BB133111EB
	x ^= x >> 30
	x *= 0xBF58476D1CE4E5B9
	x ^= x >> 27
	x *= 0x94D049BB133111EB
	x ^= x >> 31
	return x
}
