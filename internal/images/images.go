// Package images finds a lead image for articles whose own pages only have a
// generic logo: it fetches the original story the article cites and reads
// that page's og:image. One small request per new article, politely.
package images

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/fetch"
	"github.com/wanderingtilldeath/news-aggregator/internal/parse"
	"github.com/wanderingtilldeath/news-aggregator/internal/robots"
	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

const maxRedirects = 3

// BlockedError means the host refuses us (401, 403, 429 or 451): it is blocking
// bots, which will not change by asking again. The crawler does not try to get
// around it.
type BlockedError struct {
	Host   string
	Status int
}

func (e *BlockedError) Error() string {
	return fmt.Sprintf("images: %s blocked us (HTTP %d)", e.Host, e.Status)
}

func isBlockStatus(code int) bool {
	return code == http.StatusUnauthorized || code == http.StatusForbidden ||
		code == http.StatusTooManyRequests || code == http.StatusUnavailableForLegalReasons
}

// Resolver turns a cited source URL into an absolute image URL; "" means the
// page has none (or may not be fetched), which is a normal answer, not an error.
type Resolver interface {
	Image(ctx context.Context, sourceURL string) (string, error)
}

// Fetching is the production Resolver.
type Fetching struct {
	Fetcher fetch.Fetcher
	Robots  robots.Checker
	Parser  parse.Parser
}

// NewFetching builds a Resolver whose HTTP client refuses to connect to
// loopback, private and link-local addresses: source URLs come from crawled
// pages, so they must not be able to point this process at internal services.
//
// allowPrivate lifts that restriction; it exists for tests against a local site.
func NewFetching(userAgent string, timeout time.Duration, maxBytes int64, rb robots.Checker, allowPrivate bool) *Fetching {
	var transport http.RoundTripper // nil: the fetcher's tuned default
	if !allowPrivate {
		transport = publicOnlyTransport()
	}

	return &Fetching{
		Fetcher: fetch.New(fetch.Options{UserAgent: userAgent, Timeout: timeout, MaxBytes: maxBytes, Transport: transport}),
		Robots:  rb,
		Parser:  parse.HTMLParser{},
	}
}

func (f *Fetching) Image(ctx context.Context, sourceURL string) (string, error) {
	target := sourceURL

	for hop := 0; hop <= maxRedirects; hop++ {
		if u, err := url.Parse(target); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			return "", nil
		}

		if f.Robots != nil {
			d, err := f.Robots.Check(ctx, target)
			if err != nil {
				return "", fmt.Errorf("robots %s: %w", target, err)
			}
			if !d.Allowed {
				return "", nil
			}
		}

		res, err := f.Fetcher.Fetch(ctx, fetch.Request{URL: target})
		switch {
		case errors.Is(err, fetch.ErrUnsupportedContentType):
			return "", nil
		case err != nil:
			return "", err
		}

		switch {
		case res.Status >= 300 && res.Status < 400 && res.Location != "":
			target = res.Location
			continue
		case isBlockStatus(res.Status):
			return "", &BlockedError{Host: hostOf(target), Status: res.Status}
		case res.Status != http.StatusOK:
			return "", nil
		}

		page, err := f.Parser.Parse(bytes.NewReader(res.Body), target)
		if err != nil {
			return "", nil
		}
		return page.Image, nil
	}

	return "", nil
}

// publicOnlyTransport is the default HTTP transport with a dialer that rejects
// non-public addresses at connect time (so DNS tricks cannot bypass it).
func publicOnlyTransport() http.RoundTripper {
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip := net.ParseIP(host)
			if ip == nil || !isPublic(ip) {
				return fmt.Errorf("images: refusing to connect to non-public address %s", host)
			}
			return nil
		},
	}

	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DialContext = dialer.DialContext
	return t
}

func isPublic(ip net.IP) bool {
	return !(ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified())
}

// blockedLimit is how many refusals from one host end the lookups for that host
// in a run.
const blockedLimit = 2

// Blocklist remembers, for the length of one daily run, how many times in a row
// each host has refused us. The run shares one across its passes, so a host that
// blocks bots costs two requests a day, not two per pass.
type Blocklist struct {
	mu sync.Mutex
	n  map[string]int
}

// NewBlocklist returns an empty Blocklist.
func NewBlocklist() *Blocklist { return &Blocklist{n: map[string]int{}} }

func (b *Blocklist) blocked(host string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.n[host] >= blockedLimit
}

// refused records a refusal and reports whether it just reached the limit.
func (b *Blocklist) refused(host string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.n[host]++
	return b.n[host] == blockedLimit
}

func (b *Blocklist) answered(host string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.n[host] = 0
}

// Pass looks up the image for articles in the digest window that cite a source
// and have not been looked up yet. Requests are made one at a time, `delay`
// apart. A definitive answer (an image, or "none") marks the article looked up;
// a transient failure leaves it pending for the next run. A host that refuses
// us (HTTP 403 and the like) twice in a row is not asked again this run: its
// remaining articles are marked looked up without a request, since asking a
// bot-blocking site sixteen times only earns sixteen refusals. bl carries that
// memory across passes (nil means just this pass). It returns how many articles
// got an image.
func Pass(ctx context.Context, st store.Store, r Resolver, since time.Time, limit int, delay time.Duration, bl *Blocklist, logger *slog.Logger) (int, error) {
	if bl == nil {
		bl = NewBlocklist()
	}
	jobs, err := st.PendingImages(ctx, since, limit)
	if err != nil {
		return 0, err
	}

	var (
		found int
		asked bool // whether a request has been made yet, for spacing
	)
	for _, j := range jobs {
		if ctx.Err() != nil {
			return found, ctx.Err()
		}

		host := hostOf(j.SourceURL)
		if bl.blocked(host) {
			if err := st.SetImage(ctx, j.URLHash, ""); err != nil {
				return found, err
			}
			continue
		}

		if asked && delay > 0 {
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return found, ctx.Err()
			}
		}
		asked = true

		image, err := r.Image(ctx, j.SourceURL)

		var be *BlockedError
		switch {
		case errors.As(err, &be):
			if bl.refused(host) {
				logger.Info("source blocks us, skipping the rest of its articles this run", "host", host, "status", be.Status)
			}
			image = "" // a definitive "no": mark it looked up
		case err != nil:
			logger.Warn("image lookup failed, will retry next run", "url", j.URL, "source", j.SourceURL, "error", err)
			continue
		default:
			bl.answered(host)
		}

		if err := st.SetImage(ctx, j.URLHash, image); err != nil {
			return found, err
		}
		if image != "" {
			found++
		}
	}

	return found, nil
}

func hostOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}
