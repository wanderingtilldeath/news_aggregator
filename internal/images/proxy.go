package images

import (
	"container/list"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	defaultCacheBytes = 64 << 20
	maxImageBytes     = 5 << 20
	browserCacheSecs  = 86400
	failureTTL        = 10 * time.Minute
	// maxConcurrentFetches is how many different images are fetched at once.
	maxConcurrentFetches = 4
)

// Lookup finds the stored image URL of a page.
type Lookup interface {
	ImageURL(ctx context.Context, urlHash string) (string, error)
}

// Proxy serves article images from our own origin: GET /img/{id} looks the
// image URL up by page id (never from the request, so it is not an open
// proxy), fetches it once, keeps it in a bounded in-memory cache, and tells
// browsers to cache it for a day. That fixes the usual ways hotlinked images
// break (referrer checks, mixed content, slow or flaky third-party hosts) and
// makes repeat views instant.
type Proxy struct {
	Lookup Lookup
	HTTP   *http.Client
	// UserAgent identifies us to the image host.
	UserAgent string
	// MaxCacheBytes bounds the in-memory cache (default 64 MiB).
	MaxCacheBytes int64

	group singleflight.Group
	// sem bounds the fetches in flight: each one buffers up to maxImageBytes, and a burst of
	// different ids would otherwise run a small server out of memory. nil means no bound.
	sem chan struct{}

	mu    sync.Mutex
	items map[string]*list.Element
	order *list.List // front = most recently used
	bytes int64
	// failed remembers recent failures (and why) so a broken image is not refetched on
	// every page view.
	failed map[string]failure
	// Logger, when set, gets one warning per fresh fetch failure: the host and the reason.
	Logger *slog.Logger
	// urls remembers the image URL of an article id for a few minutes: the lookup is a database
	// query that scans the shared-image counts, and a page of cards asks for 24 of them.
	urls map[string]urlEntry
}

type failure struct {
	at     time.Time
	reason string
}

// How an image can fail to be served. reasonOf turns one of these into the short text sent in the
// X-Image-Error header and logged, so a missing picture can be explained without guessing.
var (
	errTooLarge = errors.New("image too large")
	errNotImage = errors.New("not a supported image")
)

// statusError is an upstream answer other than 200.
type statusError struct{ code int }

func (e *statusError) Error() string { return fmt.Sprintf("upstream status %d", e.code) }

// failedBefore is returned for an image that failed recently; it carries the earlier reason.
type failedBefore struct{ reason string }

func (e *failedBefore) Error() string { return "failed recently: " + e.reason }

// reasonOf is the short public reason an image could not be served. It never includes the address.
func reasonOf(err error) string {
	var se *statusError
	var fb *failedBefore
	switch {
	case errors.As(err, &fb):
		return fb.reason
	case errors.As(err, &se):
		return se.Error()
	case errors.Is(err, errTooLarge):
		return "too large"
	case errors.Is(err, errNotImage):
		return "not a supported image"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	default:
		return "fetch failed"
	}
}

type urlEntry struct {
	url string
	at  time.Time
}

type cached struct {
	key         string
	data        []byte
	contentType string
	etag        string
}

// NewProxy builds a Proxy whose client refuses non-public addresses unless
// allowPrivate (tests against a local site).
func NewProxy(l Lookup, userAgent string, allowPrivate bool) *Proxy {
	var transport http.RoundTripper = http.DefaultTransport
	if !allowPrivate {
		transport = publicOnlyTransport()
	}

	return &Proxy{
		Lookup:    l,
		HTTP:      &http.Client{Timeout: 15 * time.Second, Transport: transport},
		UserAgent: userAgent,
		sem:       make(chan struct{}, maxConcurrentFetches),
	}
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" || len(id) > 128 {
		http.NotFound(w, r)
		return
	}

	imageURL, ok := p.cachedURL(id)
	if !ok {
		var err error
		imageURL, err = p.Lookup.ImageURL(r.Context(), id)
		if err != nil {
			http.Error(w, "lookup failed", http.StatusInternalServerError)
			return
		}
		p.rememberURL(id, imageURL)
	}
	if imageURL == "" {
		http.NotFound(w, r)
		return
	}

	img, err := p.get(r.Context(), imageURL)
	if err != nil {
		w.Header().Set("X-Image-Error", reasonOf(err))
		http.NotFound(w, r)
		return
	}

	h := w.Header()
	h.Set("Content-Type", img.contentType)
	h.Set("Cache-Control", fmt.Sprintf("public, max-age=%d", browserCacheSecs))
	h.Set("ETag", img.etag)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")

	if r.Header.Get("If-None-Match") == img.etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	h.Set("Content-Length", fmt.Sprint(len(img.data)))
	_, _ = w.Write(img.data)
}

const (
	urlTTL      = 5 * time.Minute // an article's image URL
	urlMissTTL  = time.Minute     // "no image" or an unknown id, so scanners cannot make a query per request
	maxURLCache = 10000
)

func (p *Proxy) cachedURL(id string) (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	e, ok := p.urls[id]
	if !ok {
		return "", false
	}
	ttl := urlTTL
	if e.url == "" {
		ttl = urlMissTTL
	}
	if time.Since(e.at) > ttl {
		delete(p.urls, id)
		return "", false
	}
	return e.url, true
}

func (p *Proxy) rememberURL(id, url string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.urls == nil || len(p.urls) >= maxURLCache {
		p.urls = map[string]urlEntry{} // simplest bound: start over
	}
	p.urls[id] = urlEntry{url: url, at: time.Now()}
}

// get returns the cached image or fetches it, once per URL at a time.
func (p *Proxy) get(ctx context.Context, imageURL string) (*cached, error) {
	if img := p.cacheGet(imageURL); img != nil {
		return img, nil
	}
	if reason, ok := p.recentlyFailed(imageURL); ok {
		return nil, &failedBefore{reason: reason}
	}

	v, err, _ := p.group.Do(imageURL, func() (any, error) {
		// Not tied to one viewer's request: others wait on this fetch too.
		fetchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
		defer cancel()

		if p.sem != nil {
			select {
			case p.sem <- struct{}{}:
				defer func() { <-p.sem }()
			case <-fetchCtx.Done():
				return nil, fetchCtx.Err() // too busy, not a bad image: do not remember it as failed
			}
		}

		img, err := p.fetch(fetchCtx, imageURL)
		if err != nil {
			p.markFailed(imageURL, reasonOf(err))
			if p.Logger != nil {
				p.Logger.Warn("image fetch failed", "host", hostOf(imageURL), "reason", reasonOf(err))
			}
			return nil, err
		}
		p.cachePut(img)
		return img, nil
	})
	if err != nil {
		return nil, err
	}

	return v.(*cached), nil
}

// allowedTypes are served as-is. SVG is left out on purpose: it can carry script.
var allowedTypes = map[string]bool{
	"image/jpeg": true, "image/png": true, "image/gif": true, "image/webp": true, "image/avif": true,
}

func (p *Proxy) fetch(ctx context.Context, imageURL string) (*cached, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", p.UserAgent)
	req.Header.Set("Accept", "image/avif,image/webp,image/png,image/jpeg,image/*;q=0.8")

	resp, err := p.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("images: %s: %w", imageURL, &statusError{code: resp.StatusCode})
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxImageBytes {
		return nil, fmt.Errorf("images: %s is larger than %d bytes: %w", imageURL, maxImageBytes, errTooLarge)
	}

	// Trust the bytes over the header: a mislabelled HTML error page is not an image.
	contentType := http.DetectContentType(data)
	if i := strings.IndexByte(contentType, ';'); i >= 0 {
		contentType = contentType[:i]
	}
	if !allowedTypes[contentType] {
		return nil, fmt.Errorf("images: %s is %s: %w", imageURL, contentType, errNotImage)
	}

	sum := sha1.Sum(data)
	return &cached{key: imageURL, data: data, contentType: contentType, etag: `"` + hex.EncodeToString(sum[:8]) + `"`}, nil
}

func (p *Proxy) maxBytes() int64 {
	if p.MaxCacheBytes > 0 {
		return p.MaxCacheBytes
	}
	return defaultCacheBytes
}

func (p *Proxy) cacheGet(key string) *cached {
	p.mu.Lock()
	defer p.mu.Unlock()

	if el, ok := p.items[key]; ok {
		p.order.MoveToFront(el)
		return el.Value.(*cached)
	}
	return nil
}

func (p *Proxy) cachePut(img *cached) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.items == nil {
		p.items, p.order = map[string]*list.Element{}, list.New()
	}
	if _, ok := p.items[img.key]; ok {
		return
	}

	p.items[img.key] = p.order.PushFront(img)
	p.bytes += int64(len(img.data))

	for p.bytes > p.maxBytes() && p.order.Len() > 1 {
		last := p.order.Back()
		old := last.Value.(*cached)
		p.order.Remove(last)
		delete(p.items, old.key)
		p.bytes -= int64(len(old.data))
	}
}

func (p *Proxy) markFailed(key, reason string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.failed == nil {
		p.failed = map[string]failure{}
	}
	// Keep the map from growing without bound.
	if len(p.failed) > 5000 {
		p.failed = map[string]failure{}
	}
	p.failed[key] = failure{at: time.Now(), reason: reason}
}

func (p *Proxy) recentlyFailed(key string) (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	f, ok := p.failed[key]
	if ok && time.Since(f.at) > failureTTL {
		delete(p.failed, key)
		return "", false
	}
	return f.reason, ok
}
