// Package frontier is the crawler's work queue and visited set in one: a URL
// is "visited" exactly when a row for it exists. Because the state lives in
// the store, not in goroutines or channels, enqueueing never blocks (no
// producer/consumer deadlock) and a crashed process loses nothing.
package frontier

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"
	"time"
)

// Status is the lifecycle state of a frontier row.
type Status string

const (
	// Pending rows are waiting to be claimed (possibly until NextFetchAt).
	Pending Status = "pending"
	// Leased rows are held by a worker until their lease expires.
	Leased Status = "leased"
	// Done rows were fetched successfully.
	Done Status = "done"
	// Failed rows hit a permanent error (4xx, bad content, fetch rejected).
	Failed Status = "failed"
	// Blocked rows are disallowed by robots.txt.
	Blocked Status = "blocked"
	// Skipped rows were fetched but intentionally not indexed (non-HTML,
	// redirected elsewhere, noindex).
	Skipped Status = "skipped"
	// Dead rows exhausted their retry budget (the dead-letter state).
	Dead Status = "dead"
)

// Terminal reports whether no further work will happen on a row in s.
func (s Status) Terminal() bool {
	return s != Pending && s != Leased
}

// AllStatuses lists every status, in display order.
var AllStatuses = []Status{Pending, Leased, Done, Failed, Blocked, Skipped, Dead}

// Crawl is one crawl job: a seed, a depth limit and the hosts it may touch.
type Crawl struct {
	ID           int64    `json:"id"`
	Seed         string   `json:"seed"`
	MaxDepth     int      `json:"max_depth"`
	AllowedHosts []string `json:"allowed_hosts"`
	// PathPrefixes limits links to these URL path prefixes; empty means any path.
	PathPrefixes []string `json:"path_prefixes"`
	// ArticlePrefixes, when set, marks only pages whose path starts with one
	// as articles for the digest, whatever their dates say.
	ArticlePrefixes []string `json:"article_prefixes"`
	// SplitArticles stores each <article id> block of a multi-topic page as
	// its own digest item.
	SplitArticles bool `json:"split_articles"`
	// ImageFromSource makes the digest fetch each article's cited source page
	// for its lead image (for sites whose own og:image is a generic logo).
	ImageFromSource bool `json:"image_from_source"`
	// Category groups the crawl's articles in the digest (for example "tech").
	Category  string    `json:"category"`
	CreatedAt time.Time `json:"created_at"`
}

// CrawlSpec describes a crawl to create. Seed must be canonical.
type CrawlSpec struct {
	Seed            string
	MaxDepth        int
	AllowedHosts    []string
	PathPrefixes    []string
	ArticlePrefixes []string
	SplitArticles   bool
	ImageFromSource bool
	Category        string
}

// IsArticlePath reports whether path counts as an article under the crawl's
// ArticlePrefixes; ok is false when the crawl sets none, so the caller falls
// back to the page's own signals.
func (c Crawl) IsArticlePath(path string) (isArticle, ok bool) {
	if len(c.ArticlePrefixes) == 0 {
		return false, false
	}
	// The page at a prefix itself (/blog/) is the section index, not an article.
	for _, prefix := range c.ArticlePrefixes {
		if p := strings.TrimPrefix(prefix, "!"); path == p || path == strings.TrimSuffix(p, "/") {
			return false, true
		}
	}
	return PathMatches(path, c.ArticlePrefixes), true
}

// AllowsHost reports whether the crawl may fetch from host (host[:port]).
func (c Crawl) AllowsHost(host string) bool {
	for _, allowed := range c.AllowedHosts {
		if strings.EqualFold(allowed, host) {
			return true
		}
	}
	return false
}

// AllowsURL reports whether the crawl may fetch the canonical URL u: its host
// is allowed and, when path prefixes are set, its path starts with one.
func (c Crawl) AllowsURL(u string) bool {
	parsed, err := url.Parse(u)
	if err != nil || !c.AllowsHost(parsed.Host) {
		return false
	}
	return PathMatches(parsed.Path, c.PathPrefixes)
}

// PathMatches reports whether path is selected by prefixes. A prefix starting
// with "!" excludes: "!/blog/tag/" removes everything under /blog/tag/ even if
// "/blog/" is listed. With no plain prefixes, everything not excluded matches.
func PathMatches(path string, prefixes []string) bool {
	plain := false
	for _, p := range prefixes {
		if !strings.HasPrefix(p, "!") {
			plain = true
		}
	}

	matched := !plain
	for _, p := range prefixes {
		if ex, ok := strings.CutPrefix(p, "!"); ok {
			if strings.HasPrefix(path, ex) {
				return false
			}
			continue
		}
		if strings.HasPrefix(path, p) {
			matched = true
		}
	}

	return matched
}

// normalizePrefixes trims prefixes, drops empties and ensures a leading "/"
// (after the "!" of an exclusion).
func normalizePrefixes(prefixes []string) []string {
	var out []string
	for _, p := range prefixes {
		p = strings.TrimSpace(p)
		neg := strings.HasPrefix(p, "!")
		p = strings.TrimSpace(strings.TrimPrefix(p, "!"))
		if p == "" {
			continue
		}
		if !strings.HasPrefix(p, "/") {
			p = "/" + p
		}
		if neg {
			p = "!" + p
		}
		out = append(out, p)
	}
	return out
}

// NewItem is a URL to add to a crawl. URL must already be canonical.
type NewItem struct {
	URL   string
	Depth int
}

// Item is a claimed unit of work.
type Item struct {
	CrawlID int64
	URLHash string
	URL     string
	Host    string
	Depth   int
	// Attempts counts failed attempts and lease expiries so far.
	Attempts int
}

// Stats is the row count per status for a crawl.
type Stats map[Status]int64

// Active is the number of rows that may still produce work.
func (s Stats) Active() int64 { return s[Pending] + s[Leased] }

// Total is the number of distinct URLs discovered.
func (s Stats) Total() int64 {
	var n int64
	for _, v := range s {
		n += v
	}
	return n
}

// DepthRow is the row count per status at one link depth.
type DepthRow struct {
	Depth int              `json:"depth"`
	Stats map[string]int64 `json:"stats"`
}

// ErrNotFound is returned when a crawl does not exist.
var ErrNotFound = errors.New("frontier: not found")

// Frontier is the durable work queue. Implementations must be safe for
// concurrent use by many workers, and, for Postgres, by many processes.
type Frontier interface {
	// CreateCrawl registers a crawl. An empty allowed list defaults to the
	// seed's own host.
	CreateCrawl(ctx context.Context, spec CrawlSpec) (Crawl, error)
	GetCrawl(ctx context.Context, id int64) (Crawl, error)
	// ListCrawls returns the most recent crawls, newest first.
	ListCrawls(ctx context.Context, limit int) ([]Crawl, error)

	// Enqueue adds URLs to a crawl, ignoring ones already present, and
	// returns how many were new. A URL that is still pending and is enqueued
	// again at a smaller depth has its depth lowered.
	Enqueue(ctx context.Context, crawlID int64, items []NewItem) (int, error)

	// Claim leases up to n due rows for lease, shallowest first (so the crawl
	// is breadth-first). A leased row whose lease has
	// expired is claimable again, which is crash recovery; reclaiming it
	// counts as a failed attempt.
	Claim(ctx context.Context, n int, lease time.Duration) ([]Item, error)

	// Finish moves a leased row to a terminal status. It is a no-op if the
	// caller no longer holds the lease.
	Finish(ctx context.Context, it Item, status Status, note string) error

	// Reschedule returns a leased row to pending, due at `at`. countAttempt
	// records a failed attempt; false is used for "not yet, host is busy".
	Reschedule(ctx context.Context, it Item, at time.Time, countAttempt bool, note string) error

	// Release returns leased rows to pending immediately (graceful shutdown).
	Release(ctx context.Context, items []Item) error

	// Stats counts a crawl's rows by status.
	Stats(ctx context.Context, crawlID int64) (Stats, error)

	// DepthStats counts a crawl's rows by link depth and status, shallowest
	// depth first.
	DepthStats(ctx context.Context, crawlID int64) ([]DepthRow, error)

	// QueueDepth counts pending rows across all crawls.
	QueueDepth(ctx context.Context) (int64, error)
}

// Hash is the dedupe key of a canonical URL.
func Hash(canonicalURL string) string {
	sum := sha256.Sum256([]byte(canonicalURL))
	return hex.EncodeToString(sum[:])
}

// hostOf returns the lowercase host[:port] of a canonical URL.
func hostOf(canonicalURL string) string {
	u, err := url.Parse(canonicalURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Host)
}

// normalizeAllowed returns allowedHosts lowercased, or the seed's host when
// the list is empty.
func normalizeAllowed(seed string, allowedHosts []string) ([]string, error) {
	var out []string
	for _, h := range allowedHosts {
		if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
			out = append(out, h)
		}
	}
	if len(out) > 0 {
		return out, nil
	}

	host := hostOf(seed)
	if host == "" {
		return nil, errors.New("frontier: seed URL has no host")
	}
	return []string{host}, nil
}

// StartCrawl creates a crawl and enqueues its seed at depth 0. seed must be
// a canonical URL.
func StartCrawl(ctx context.Context, f Frontier, seed string, maxDepth int, allowedHosts, pathPrefixes []string) (Crawl, error) {
	return StartCrawlSpec(ctx, f, CrawlSpec{Seed: seed, MaxDepth: maxDepth, AllowedHosts: allowedHosts, PathPrefixes: pathPrefixes})
}

// StartCrawlSpec creates the crawl described by spec and enqueues its seed at
// depth 0.
func StartCrawlSpec(ctx context.Context, f Frontier, spec CrawlSpec) (Crawl, error) {
	crawl, err := f.CreateCrawl(ctx, spec)
	if err != nil {
		return Crawl{}, err
	}

	if _, err := f.Enqueue(ctx, crawl.ID, []NewItem{{URL: spec.Seed, Depth: 0}}); err != nil {
		return Crawl{}, err
	}

	return crawl, nil
}

// newCrawl builds the Crawl for spec with its lists normalised and never nil
// (so they marshal as [] and scan from NOT NULL columns).
func newCrawl(spec CrawlSpec, allowed []string) Crawl {
	c := Crawl{
		Seed:            spec.Seed,
		MaxDepth:        spec.MaxDepth,
		AllowedHosts:    allowed,
		PathPrefixes:    normalizePrefixes(spec.PathPrefixes),
		ArticlePrefixes: normalizePrefixes(spec.ArticlePrefixes),
		SplitArticles:   spec.SplitArticles,
		ImageFromSource: spec.ImageFromSource,
		Category:        spec.Category,
	}
	if c.PathPrefixes == nil {
		c.PathPrefixes = []string{}
	}
	if c.ArticlePrefixes == nil {
		c.ArticlePrefixes = []string{}
	}
	return c
}
