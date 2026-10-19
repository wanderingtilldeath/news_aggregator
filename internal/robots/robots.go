// Package robots decides whether a URL may be crawled according to the
// host's robots.txt, following RFC 9309: a 2xx file is parsed (Allow,
// Disallow, wildcards, Crawl-delay), a 4xx means "no restrictions", and a 5xx
// (or an unparsable file) means "assume everything is disallowed" until the
// short error TTL expires.
package robots

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/temoto/robotstxt"
	"golang.org/x/sync/singleflight"
)

const (
	// maxRobotsBytes is the parsing limit RFC 9309 asks crawlers to support
	// at minimum (500 KiB).
	maxRobotsBytes = 500 << 10
	// maxRedirects is the number of redirects RFC 9309 asks us to follow.
	maxRedirects = 5
)

// Decision is the verdict for one URL.
type Decision struct {
	Allowed bool
	// CrawlDelay is the host's Crawl-delay for our user agent, 0 if none.
	CrawlDelay time.Duration
}

// Checker answers robots.txt questions. A nil error with Allowed=false means
// "blocked"; an error means robots.txt could not be fetched right now (a
// transient condition the caller should retry later).
type Checker interface {
	Check(ctx context.Context, rawURL string) (Decision, error)
}

// Cache is a shared second-level cache for raw robots.txt responses, so many
// crawler processes fetch each host's file once per TTL instead of once each.
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
}

// Options configures a Client. Zero values pick defaults.
type Options struct {
	// UserAgent is the full header value sent when fetching robots.txt.
	UserAgent string
	// Product is the token matched against User-agent groups (e.g. "CrawlerX").
	Product string
	// HTTP is the client used to fetch robots.txt.
	HTTP *http.Client
	// Cache is an optional shared cache (see NewRedisCache).
	Cache Cache
	// TTL is how long a good (2xx/4xx) result is cached; default 24h.
	TTL time.Duration
	// ErrorTTL is how long a 5xx / unparsable result is cached; default 5m.
	ErrorTTL time.Duration
	// Now is the clock, for tests.
	Now func() time.Time
}

// Client is the production Checker.
type Client struct {
	opts Options

	mu    sync.Mutex
	local map[string]localEntry
	group singleflight.Group
}

type localEntry struct {
	data    *robotstxt.RobotsData
	expires time.Time
}

// cachedResponse is the form stored in the shared cache. We keep the raw
// status and body, not the parsed result, so the cache format does not depend
// on the parser's internals.
type cachedResponse struct {
	Status int    `json:"status"`
	Body   string `json:"body"`
}

// New builds a Client.
func New(opts Options) *Client {
	if opts.Product == "" {
		opts.Product = "CrawlerX"
	}
	if opts.UserAgent == "" {
		opts.UserAgent = opts.Product
	}
	if opts.TTL <= 0 {
		opts.TTL = 24 * time.Hour
	}
	if opts.ErrorTTL <= 0 {
		opts.ErrorTTL = 5 * time.Minute
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.HTTP == nil {
		opts.HTTP = &http.Client{Timeout: 10 * time.Second}
	}

	// Follow at most maxRedirects hops, per RFC 9309. A copy keeps the
	// caller's client untouched.
	httpClient := *opts.HTTP
	httpClient.CheckRedirect = func(_ *http.Request, via []*http.Request) error {
		if len(via) > maxRedirects {
			return http.ErrUseLastResponse
		}
		return nil
	}
	opts.HTTP = &httpClient

	return &Client{opts: opts, local: make(map[string]localEntry)}
}

// Check reports whether rawURL may be fetched.
func (c *Client) Check(ctx context.Context, rawURL string) (Decision, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return Decision{}, fmt.Errorf("robots: bad URL %q", rawURL)
	}

	origin := strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host)

	data, err := c.rulesFor(ctx, origin)
	if err != nil {
		return Decision{}, err
	}

	group := data.FindGroup(c.opts.Product)

	return Decision{
		Allowed:    data.TestAgent(u.RequestURI(), c.opts.Product),
		CrawlDelay: group.CrawlDelay,
	}, nil
}

func (c *Client) rulesFor(ctx context.Context, origin string) (*robotstxt.RobotsData, error) {
	now := c.opts.Now()

	c.mu.Lock()
	entry, ok := c.local[origin]
	c.mu.Unlock()
	if ok && now.Before(entry.expires) {
		return entry.data, nil
	}

	// One fetch per origin at a time, however many workers ask at once.
	v, err, _ := c.group.Do(origin, func() (any, error) {
		return c.load(ctx, origin)
	})
	if err != nil {
		return nil, err
	}

	return v.(*robotstxt.RobotsData), nil
}

func (c *Client) load(ctx context.Context, origin string) (*robotstxt.RobotsData, error) {
	cacheKey := "crawlerx:robots:" + origin

	if c.opts.Cache != nil {
		if raw, ok, err := c.opts.Cache.Get(ctx, cacheKey); err == nil && ok {
			var cached cachedResponse
			if json.Unmarshal(raw, &cached) == nil {
				data, ttl := c.interpret(cached.Status, []byte(cached.Body))
				c.remember(origin, data, ttl)
				return data, nil
			}
		}
	}

	status, body, err := c.fetch(ctx, origin+"/robots.txt")
	if err != nil {
		return nil, err
	}

	data, ttl := c.interpret(status, body)
	c.remember(origin, data, ttl)

	if c.opts.Cache != nil {
		if raw, err := json.Marshal(cachedResponse{Status: status, Body: string(body)}); err == nil {
			_ = c.opts.Cache.Set(ctx, cacheKey, raw, ttl) // best effort
		}
	}

	return data, nil
}

func (c *Client) remember(origin string, data *robotstxt.RobotsData, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.local[origin] = localEntry{data: data, expires: c.opts.Now().Add(ttl)}
}

// interpret turns a robots.txt response into rules and a cache lifetime.
func (c *Client) interpret(status int, body []byte) (*robotstxt.RobotsData, time.Duration) {
	// 3xx here means we gave up after maxRedirects: RFC 9309 says to treat
	// that as "unavailable", the same as a 404.
	if status >= 300 && status < 400 {
		status = http.StatusNotFound
	}

	data, err := robotstxt.FromStatusAndBytes(status, body)
	if err != nil {
		// Unparsable, or a status we do not understand: be conservative.
		return disallowAll(), c.opts.ErrorTTL
	}

	if status >= 500 {
		return data, c.opts.ErrorTTL
	}

	return data, c.opts.TTL
}

func disallowAll() *robotstxt.RobotsData {
	data, _ := robotstxt.FromStatusAndBytes(http.StatusServiceUnavailable, nil)
	return data
}

func (c *Client) fetch(ctx context.Context, robotsURL string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, robotsURL, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("User-Agent", c.opts.UserAgent)

	resp, err := c.opts.HTTP.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("robots: fetch %s: %w", robotsURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRobotsBytes))
	if err != nil {
		return 0, nil, fmt.Errorf("robots: read %s: %w", robotsURL, err)
	}

	return resp.StatusCode, body, nil
}

// RedisCache is a Cache backed by Redis.
type RedisCache struct {
	client redis.Cmdable
}

// NewRedisCache returns a shared robots.txt cache.
func NewRedisCache(client redis.Cmdable) *RedisCache {
	return &RedisCache{client: client}
}

func (r *RedisCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	val, err := r.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return val, true, nil
}

func (r *RedisCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return r.client.Set(ctx, key, value, ttl).Err()
}

var (
	_ Checker = (*Client)(nil)
	_ Cache   = (*RedisCache)(nil)
)
