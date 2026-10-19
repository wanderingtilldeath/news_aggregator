// Package crawler is the worker loop. It claims URLs from a durable
// frontier, applies robots.txt and per-host politeness, fetches, parses,
// stores the page and feeds discovered links back into the frontier.
//
// Workers never block on each other: discovered links go into the frontier
// (a database write), not into a bounded channel, so a page with thousands of
// links cannot deadlock the pool. All state lives in the frontier, so a
// process can die at any point and another resumes the work.
package crawler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/wanderingtilldeath/news-aggregator/internal/fetch"
	"github.com/wanderingtilldeath/news-aggregator/internal/frontier"
	"github.com/wanderingtilldeath/news-aggregator/internal/metrics"
	"github.com/wanderingtilldeath/news-aggregator/internal/parse"
	"github.com/wanderingtilldeath/news-aggregator/internal/politeness"
	"github.com/wanderingtilldeath/news-aggregator/internal/robots"
	"github.com/wanderingtilldeath/news-aggregator/internal/store"
	"github.com/wanderingtilldeath/news-aggregator/internal/urlnorm"
)

// Config controls the worker pool. Zero values pick defaults.
type Config struct {
	// Workers is the number of goroutines processing URLs (default 8).
	Workers int
	// Lease is how long a claimed URL is reserved before another worker may
	// take it over (default 60s). It must exceed the fetch timeout.
	Lease time.Duration
	// PollInterval is the sleep when there is nothing to claim (default 100ms).
	PollInterval time.Duration
	// HostDelay is the minimum gap between requests to one host (default 1s).
	// A larger robots.txt Crawl-delay wins.
	HostDelay time.Duration
	// HostInflight caps concurrent requests per host in this process
	// (default 2).
	HostInflight int
	// MaxAttempts is the retry budget before a URL is dead-lettered
	// (default 5).
	MaxAttempts int
	// Backoff shapes retry delays (default fetch.DefaultBackoff).
	Backoff fetch.Backoff
	// ShutdownGrace is how long in-flight work may finish after the context
	// is cancelled before it is aborted and released (default 10s).
	ShutdownGrace time.Duration
	// Conditional sends If-None-Match / If-Modified-Since for URLs fetched
	// before. A 304 marks the URL done but its links are not re-discovered.
	Conditional bool
}

func (c Config) withDefaults() Config {
	if c.Workers <= 0 {
		c.Workers = 8
	}
	if c.Lease <= 0 {
		c.Lease = 60 * time.Second
	}
	if c.PollInterval <= 0 {
		c.PollInterval = 100 * time.Millisecond
	}
	if c.HostDelay < 0 {
		c.HostDelay = 0
	}
	if c.HostInflight <= 0 {
		c.HostInflight = 2
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 5
	}
	if c.Backoff.Base <= 0 {
		c.Backoff = fetch.DefaultBackoff
	}
	if c.ShutdownGrace <= 0 {
		c.ShutdownGrace = 10 * time.Second
	}
	return c
}

// Deps are the crawler's collaborators. Frontier and Fetcher are required;
// every other field may be nil.
type Deps struct {
	Frontier frontier.Frontier
	Fetcher  fetch.Fetcher
	// Parser defaults to parse.HTMLParser.
	Parser parse.Parser
	// Store receives parsed pages; nil discards them.
	Store store.Store
	// Gate enforces per-host delay; nil disables politeness delays.
	Gate politeness.Gate
	// Robots enforces robots.txt; nil allows everything.
	Robots  robots.Checker
	Metrics *metrics.Metrics
	Logger  *slog.Logger
	// Now and Rand are test seams.
	Now  func() time.Time
	Rand func() float64
}

// Crawler runs the worker pool.
type Crawler struct {
	cfg  Config
	deps Deps

	crawlsMu sync.Mutex
	crawls   map[int64]frontier.Crawl

	inflightMu sync.Mutex
	inflight   map[string]int
}

// New builds a Crawler.
func New(cfg Config, deps Deps) *Crawler {
	if deps.Parser == nil {
		deps.Parser = parse.HTMLParser{}
	}
	if deps.Metrics == nil {
		deps.Metrics = metrics.New(prometheus.NewRegistry())
	}
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.Rand == nil {
		deps.Rand = rand.Float64
	}

	return &Crawler{
		cfg:      cfg.withDefaults(),
		deps:     deps,
		crawls:   make(map[int64]frontier.Crawl),
		inflight: make(map[string]int),
	}
}

// Crawl starts a new crawl at seed and runs the pool until the crawl has no
// pending or leased URLs left, or ctx is cancelled. Other processes sharing
// the frontier may help; the call returns once the crawl is complete.
func (c *Crawler) Crawl(ctx context.Context, seed string, maxDepth int, allowedHosts []string) (frontier.Crawl, error) {
	canonical, err := urlnorm.Canonicalize(seed)
	if err != nil {
		return frontier.Crawl{}, fmt.Errorf("crawler: bad seed %q: %w", seed, err)
	}

	crawl, err := frontier.StartCrawl(ctx, c.deps.Frontier, canonical, maxDepth, allowedHosts, nil)
	if err != nil {
		return frontier.Crawl{}, err
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go c.cancelWhenDone(runCtx, crawl.ID, cancel)

	if err := c.Run(runCtx); err != nil {
		return crawl, err
	}

	return crawl, ctx.Err()
}

func (c *Crawler) cancelWhenDone(ctx context.Context, crawlID int64, cancel context.CancelFunc) {
	ticker := time.NewTicker(c.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			stats, err := c.deps.Frontier.Stats(ctx, crawlID)
			if err == nil && stats.Active() == 0 {
				cancel()
				return
			}
		}
	}
}

// Run starts the workers and blocks until ctx is cancelled and in-flight work
// has finished (or ShutdownGrace has passed and it was released back to the
// frontier). A killed process skips all of that; its leases simply expire.
func (c *Crawler) Run(ctx context.Context) error {
	// In-flight work runs on its own context so cancelling ctx stops
	// *claiming* immediately but lets current fetches finish.
	procCtx, procCancel := context.WithCancel(context.WithoutCancel(ctx))
	defer procCancel()

	var wg sync.WaitGroup
	for i := 1; i <= c.cfg.Workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			c.worker(ctx, procCtx, id)
		}(i)
	}

	go c.sampleQueueDepth(ctx)

	drained := make(chan struct{})
	go func() {
		wg.Wait()
		close(drained)
	}()

	select {
	case <-drained:
	case <-ctx.Done():
		grace := time.NewTimer(c.cfg.ShutdownGrace)
		defer grace.Stop()

		select {
		case <-drained:
		case <-grace.C:
			c.deps.Logger.Warn("shutdown grace expired, aborting in-flight work")
			procCancel()
			<-drained
		}
	}

	return nil
}

func (c *Crawler) sampleQueueDepth(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if depth, err := c.deps.Frontier.QueueDepth(ctx); err == nil {
				c.deps.Metrics.QueueDepth.Set(float64(depth))
			}
		}
	}
}

func (c *Crawler) worker(claimCtx, procCtx context.Context, id int) {
	log := c.deps.Logger.With("worker", id)

	// idle is how long to sleep when nothing is claimable. After deferring a
	// URL for a busy host it shrinks to the deferral, so the worker is back
	// exactly when that URL is due instead of a full poll interval later.
	idle := c.cfg.PollInterval

	for claimCtx.Err() == nil {
		items, err := c.deps.Frontier.Claim(claimCtx, 1, c.cfg.Lease)
		if err != nil {
			if claimCtx.Err() != nil {
				return
			}
			log.Error("claim failed", "error", err)
			sleep(claimCtx, 4*c.cfg.PollInterval)
			continue
		}

		if len(items) == 0 {
			sleep(claimCtx, c.jitter(idle))
			idle = c.cfg.PollInterval
			continue
		}

		for _, it := range items {
			if hint := c.handle(procCtx, log, it); hint > 0 && hint < idle {
				idle = hint
			}
		}
	}

	log.Debug("worker stopping")
}

// handle processes one claimed item and returns how soon the caller should
// look for work again (0 = no hint). If processing could not move the row to
// a new state (aborted by shutdown, database trouble), the lease is released
// so another worker can pick it up now instead of after the lease expires.
func (c *Crawler) handle(ctx context.Context, log *slog.Logger, it frontier.Item) time.Duration {
	var retryIn time.Duration
	if err := c.process(ctx, log, it, &retryIn); err != nil {
		log.Warn("item not completed, releasing", "url", it.URL, "error", err)

		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()

		if err := c.deps.Frontier.Release(releaseCtx, []frontier.Item{it}); err != nil {
			log.Warn("release failed; lease will expire", "url", it.URL, "error", err)
		}
	}

	return retryIn
}

// process returns nil once the item has been moved out of "leased" (finished,
// rescheduled), and an error if it has not. When it defers the item because
// its host is busy it stores the time until the item is due in *retryIn.
func (c *Crawler) process(ctx context.Context, log *slog.Logger, it frontier.Item, retryIn *time.Duration) error {
	crawl, err := c.crawl(ctx, it.CrawlID)
	if err != nil {
		return err
	}

	// A previous holder died on this URL too many times: stop retrying it.
	if it.Attempts >= c.cfg.MaxAttempts {
		return c.finish(ctx, it, frontier.Dead, "retry budget exhausted")
	}

	if !c.enterHost(it.Host) {
		*retryIn = c.deferDelay()
		return c.deps.Frontier.Reschedule(ctx, it, c.deps.Now().Add(*retryIn), false, "")
	}
	defer c.leaveHost(it.Host)

	crawlDelay := time.Duration(0)
	if c.deps.Robots != nil {
		decision, err := c.deps.Robots.Check(ctx, it.URL)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return c.transient(ctx, it, "robots.txt: "+err.Error(), 0)
		}
		if !decision.Allowed {
			c.deps.Metrics.RobotsBlocked.Inc()
			return c.finish(ctx, it, frontier.Blocked, "disallowed by robots.txt")
		}
		crawlDelay = decision.CrawlDelay
	}

	delay := max(c.cfg.HostDelay, crawlDelay)
	if c.deps.Gate != nil {
		wait, err := c.deps.Gate.Acquire(ctx, it.Host, delay)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Warn("politeness gate unavailable, backing off", "error", err)
			return c.deps.Frontier.Reschedule(ctx, it, c.deps.Now().Add(2*time.Second), false, "gate unavailable")
		}
		if wait > 0 {
			// Not our turn on this host. Hand the URL back instead of
			// sleeping so this worker can serve other hosts meanwhile.
			c.deps.Metrics.GateDeferred.Inc()
			*retryIn = wait + time.Millisecond
			return c.deps.Frontier.Reschedule(ctx, it, c.deps.Now().Add(*retryIn), false, "")
		}
	}

	req := fetch.Request{URL: it.URL}
	if c.cfg.Conditional && c.deps.Store != nil {
		if v, ok, err := c.deps.Store.Validators(ctx, it.URLHash); err == nil && ok {
			req.ETag, req.LastModified = v.ETag, v.LastModified
		}
	}

	c.deps.Metrics.HostInflight.WithLabelValues(it.Host).Inc()
	res, err := c.deps.Fetcher.Fetch(ctx, req)
	c.deps.Metrics.HostInflight.WithLabelValues(it.Host).Dec()

	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, fetch.ErrUnsupportedContentType) {
			return c.finish(ctx, it, frontier.Skipped, err.Error())
		}
		return c.transient(ctx, it, err.Error(), 0)
	}

	c.deps.Metrics.FetchDuration.Observe(res.Latency.Seconds())
	log.Debug("fetched", "url", it.URL, "status", res.Status, "depth", it.Depth, "latency", res.Latency)

	switch {
	case res.Status == 304:
		return c.finish(ctx, it, frontier.Done, "not modified")

	case res.Status >= 200 && res.Status < 300:
		return c.handlePage(ctx, crawl, it, res)

	case res.Status >= 300 && res.Status < 400:
		return c.handleRedirect(ctx, crawl, it, res)

	case fetch.ShouldRetryStatus(res.Status):
		if res.RetryAfter > 0 && c.deps.Gate != nil {
			// The server asked us to back off: pause the whole host,
			// not just this URL.
			_ = c.deps.Gate.Penalize(ctx, it.Host, res.RetryAfter)
		}
		return c.transient(ctx, it, fmt.Sprintf("HTTP %d", res.Status), res.RetryAfter)

	default:
		return c.finish(ctx, it, frontier.Failed, fmt.Sprintf("HTTP %d", res.Status))
	}
}

func (c *Crawler) handlePage(ctx context.Context, crawl frontier.Crawl, it frontier.Item, res *fetch.Result) error {
	page, err := c.deps.Parser.Parse(bytes.NewReader(res.Body), it.URL)
	if err != nil {
		return c.finish(ctx, it, frontier.Failed, "parse: "+err.Error())
	}

	if !page.NoFollow && it.Depth < crawl.MaxDepth {
		var items []frontier.NewItem
		for _, link := range page.Links {
			if crawl.AllowsURL(link) {
				items = append(items, frontier.NewItem{URL: link, Depth: it.Depth + 1})
			}
		}

		if len(items) > 0 {
			if _, err := c.deps.Frontier.Enqueue(ctx, it.CrawlID, items); err != nil {
				return c.databaseError(ctx, it, "enqueue links", err)
			}
		}
	}

	if page.NoIndex {
		return c.finish(ctx, it, frontier.Skipped, "noindex")
	}

	if c.deps.Store != nil {
		err := c.deps.Store.Save(ctx, store.Page{
			URLHash:      it.URLHash,
			CrawlID:      it.CrawlID,
			URL:          it.URL,
			StatusCode:   res.Status,
			ContentType:  res.ContentType,
			Title:        page.Title,
			Description:  page.Description,
			H1:           page.H1,
			Snippet:      page.Snippet,
			Published:    page.Published,
			ContentHash:  store.ContentHash(res.Body),
			ETag:         res.ETag,
			LastModified: res.LastModified,
			SizeBytes:    len(res.Body),
			Latency:      res.Latency,
		})
		if err != nil {
			return c.databaseError(ctx, it, "save page", err)
		}
	}

	note := ""
	if res.Truncated {
		note = "body truncated at size cap"
	}

	return c.finish(ctx, it, frontier.Done, note)
}

// handleRedirect does not follow the redirect inline. The target is enqueued
// like any discovered link, one hop deeper, so robots.txt, the per-host gate,
// crawl scope and the depth limit all apply to it, and a redirect loop ends
// at the dedupe check or the depth limit.
func (c *Crawler) handleRedirect(ctx context.Context, crawl frontier.Crawl, it frontier.Item, res *fetch.Result) error {
	target, ok := urlnorm.Resolve(res.Location, it.URL)
	if !ok {
		return c.finish(ctx, it, frontier.Failed, fmt.Sprintf("HTTP %d with unusable Location", res.Status))
	}

	note := fmt.Sprintf("HTTP %d redirect to %s", res.Status, target)

	switch {
	case it.Depth+1 > crawl.MaxDepth:
		note += " (beyond max depth)"
	case !crawl.AllowsURL(target):
		note += " (not in scope)"
	default:
		if _, err := c.deps.Frontier.Enqueue(ctx, it.CrawlID, []frontier.NewItem{{URL: target, Depth: it.Depth + 1}}); err != nil {
			return c.databaseError(ctx, it, "enqueue redirect", err)
		}
	}

	return c.finish(ctx, it, frontier.Skipped, note)
}

// databaseError treats a failed frontier/store write as transient so a
// persistent failure eventually dead-letters instead of looping forever.
func (c *Crawler) databaseError(ctx context.Context, it frontier.Item, what string, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return c.transient(ctx, it, what+": "+err.Error(), 0)
}

// transient records a failed attempt: reschedule with jittered exponential
// backoff (or the server's Retry-After), or dead-letter once the retry
// budget is spent.
func (c *Crawler) transient(ctx context.Context, it frontier.Item, note string, retryAfter time.Duration) error {
	if it.Attempts+1 >= c.cfg.MaxAttempts {
		return c.finish(ctx, it, frontier.Dead, note)
	}

	delay := retryAfter
	if delay <= 0 {
		delay = c.cfg.Backoff.Delay(it.Attempts, c.deps.Rand)
	}

	c.deps.Metrics.Retries.Inc()

	return c.deps.Frontier.Reschedule(ctx, it, c.deps.Now().Add(delay), true, note)
}

func (c *Crawler) finish(ctx context.Context, it frontier.Item, status frontier.Status, note string) error {
	if err := c.deps.Frontier.Finish(ctx, it, status, note); err != nil {
		return err
	}

	c.deps.Metrics.PagesTotal.WithLabelValues(string(status)).Inc()

	return nil
}

func (c *Crawler) crawl(ctx context.Context, id int64) (frontier.Crawl, error) {
	c.crawlsMu.Lock()
	crawl, ok := c.crawls[id]
	c.crawlsMu.Unlock()
	if ok {
		return crawl, nil
	}

	crawl, err := c.deps.Frontier.GetCrawl(ctx, id)
	if err != nil {
		return frontier.Crawl{}, err
	}

	c.crawlsMu.Lock()
	c.crawls[id] = crawl
	c.crawlsMu.Unlock()

	return crawl, nil
}

func (c *Crawler) enterHost(host string) bool {
	c.inflightMu.Lock()
	defer c.inflightMu.Unlock()

	if c.inflight[host] >= c.cfg.HostInflight {
		return false
	}
	c.inflight[host]++

	return true
}

func (c *Crawler) leaveHost(host string) {
	c.inflightMu.Lock()
	defer c.inflightMu.Unlock()

	if c.inflight[host]--; c.inflight[host] <= 0 {
		delete(c.inflight, host)
	}
}

func (c *Crawler) deferDelay() time.Duration {
	return max(c.cfg.HostDelay/2, 50*time.Millisecond)
}

// jitter spreads d over 75%..125% so idle workers do not poll in lockstep.
func (c *Crawler) jitter(d time.Duration) time.Duration {
	return time.Duration(float64(d) * (0.75 + 0.5*c.deps.Rand()))
}

func sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-ctx.Done():
	case <-t.C:
	}
}
