// Package fetch isolates the network boundary of the crawler: one polite HTTP
// GET with a bounded body, no automatic redirects and a content-type filter.
// Keeping it behind an interface lets the crawler's tests substitute a fake.
package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"strings"
	"time"
)

// DefaultUserAgent identifies the crawler honestly. Operators should override
// it with their own contact URL (see -user-agent).
const DefaultUserAgent = "CrawlerX/1.0 (+https://github.com/wanderingtilldeath/news-aggregator)"

// ErrUnsupportedContentType is returned for a successful response whose body
// is not HTML. It is permanent: retrying will not change the content type.
var ErrUnsupportedContentType = errors.New("fetch: unsupported content type")

// Request describes one GET. ETag and LastModified, when set, make it a
// conditional request.
type Request struct {
	URL          string
	ETag         string
	LastModified string
}

// Result is a fully-read HTTP response. The body is capped at the fetcher's
// MaxBytes, so a Result is always safe to hold in memory.
type Result struct {
	Status      int
	ContentType string
	Body        []byte
	// Truncated reports that the body was longer than MaxBytes.
	Truncated    bool
	ETag         string
	LastModified string
	// Location is the absolute redirect target for 3xx responses.
	Location string
	// RetryAfter is the server's requested delay, 0 if none.
	RetryAfter time.Duration
	// Latency covers request start to the end of the (capped) body read.
	Latency time.Duration
}

// Fetcher retrieves a page over HTTP.
type Fetcher interface {
	Fetch(ctx context.Context, req Request) (*Result, error)
}

// Options configures an HTTPFetcher. Zero values pick safe defaults.
type Options struct {
	UserAgent string
	Timeout   time.Duration
	// MaxBytes caps the response body (default 2 MiB).
	MaxBytes int64
	// Transport overrides the tuned default; tests use it.
	Transport http.RoundTripper
}

// HTTPFetcher is the production Fetcher.
//
// Redirects are NOT followed here: a 3xx is returned with its Location and
// the crawler enqueues the target like any other link, so robots.txt, host
// politeness, scope and depth limits apply to every hop and redirect loops
// end at the dedupe check.
type HTTPFetcher struct {
	client    *http.Client
	userAgent string
	maxBytes  int64
}

// New builds an HTTPFetcher.
func New(opts Options) *HTTPFetcher {
	if opts.UserAgent == "" {
		opts.UserAgent = DefaultUserAgent
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 15 * time.Second
	}
	if opts.MaxBytes <= 0 {
		opts.MaxBytes = 2 << 20
	}
	if opts.Transport == nil {
		opts.Transport = newTransport()
	}

	return &HTTPFetcher{
		client: &http.Client{
			Timeout:   opts.Timeout,
			Transport: opts.Transport,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		userAgent: opts.UserAgent,
		maxBytes:  opts.MaxBytes,
	}
}

func newTransport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          256,
		MaxIdleConnsPerHost:   4, // few connections per host: we are polite
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
}

// Fetch issues a GET for req.URL and reads at most MaxBytes of a 2xx HTML
// body. Non-2xx responses are returned as a Result, not an error, so the
// caller can classify them; only transport failures and unsupported content
// types return an error.
func (f *HTTPFetcher) Fetch(ctx context.Context, req Request) (*Result, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, req.URL, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("User-Agent", f.userAgent)
	httpReq.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.1")
	if req.ETag != "" {
		httpReq.Header.Set("If-None-Match", req.ETag)
	}
	if req.LastModified != "" {
		httpReq.Header.Set("If-Modified-Since", req.LastModified)
	}

	start := time.Now()

	resp, err := f.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	res := &Result{
		Status:       resp.StatusCode,
		ContentType:  resp.Header.Get("Content-Type"),
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
		RetryAfter:   ParseRetryAfter(resp.Header.Get("Retry-After"), time.Now()),
	}

	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		if loc, err := resp.Location(); err == nil {
			res.Location = loc.String()
		}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		res.Latency = time.Since(start)
		return res, nil
	}

	if !isHTML(res.ContentType) {
		return res, fmt.Errorf("%w: %q", ErrUnsupportedContentType, res.ContentType)
	}

	// Read one byte past the cap so truncation is detectable.
	body, err := io.ReadAll(io.LimitReader(resp.Body, f.maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > f.maxBytes {
		body = body[:f.maxBytes]
		res.Truncated = true
	}
	res.Body = body
	res.Latency = time.Since(start)

	return res, nil
}

func isHTML(contentType string) bool {
	if contentType == "" {
		return true // no header: let the parser decide
	}

	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = strings.ToLower(strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0]))
	}

	return mediaType == "text/html" || mediaType == "application/xhtml+xml"
}
