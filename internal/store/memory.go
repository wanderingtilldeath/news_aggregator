package store

import (
	"context"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

// Memory is an in-process Store for tests. Search is a naive substring
// match, not real full-text search.
type Memory struct {
	mu    sync.Mutex
	pages map[string]Page
	seen  map[string]time.Time // first Save per URL hash
	saves int
	// summaries holds SetSummary results; a key means "summarised".
	summaries map[string][]string
	// imageChecked marks articles whose source image was looked up.
	imageChecked map[string]bool
}

// NewMemory returns an empty in-memory store.
func NewMemory() *Memory {
	return &Memory{
		pages:     make(map[string]Page),
		seen:      make(map[string]time.Time),
		summaries: make(map[string][]string),

		imageChecked: make(map[string]bool),
	}
}

func (m *Memory) Save(_ context.Context, p Page) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Like the Postgres upsert: a re-crawl keeps an image fetched from the source.
	if old, ok := m.pages[p.URLHash]; ok && m.imageChecked[p.URLHash] && old.SourceURL != "" && old.Image != "" {
		p.Image = old.Image
	}
	m.pages[p.URLHash] = p
	if _, ok := m.seen[p.URLHash]; !ok {
		m.seen[p.URLHash] = time.Now().UTC()
	}
	m.saves++

	return nil
}

func (m *Memory) Validators(_ context.Context, urlHash string) (Validators, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.pages[urlHash]
	if !ok {
		return Validators{}, false, nil
	}

	return Validators{ETag: p.ETag, LastModified: p.LastModified}, p.ETag != "" || p.LastModified != "", nil
}

func (m *Memory) Search(_ context.Context, q string, limit, offset int) ([]Hit, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	q = strings.ToLower(q)

	var hits []Hit
	for _, p := range m.pages {
		hay := strings.ToLower(p.Title + " " + p.Description + " " + p.H1 + " " + p.Snippet)
		if strings.Contains(hay, q) {
			hits = append(hits, Hit{URL: p.URL, Title: p.Title, Description: p.Description, Snippet: p.Snippet})
		}
	}

	sort.Slice(hits, func(i, j int) bool { return hits[i].URL < hits[j].URL })

	total := len(hits)
	offset = max(offset, 0)
	if offset > len(hits) {
		offset = len(hits)
	}
	hits = hits[offset:]
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}

	return hits, total, nil
}

func (m *Memory) Digest(_ context.Context, q DigestQuery) ([]Item, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	items := m.digestLocked(q, q.Source)

	total := len(items)
	offset := max(q.Offset, 0)
	if offset > len(items) {
		offset = len(items)
	}
	items = items[offset:]
	if q.Limit > 0 && len(items) > q.Limit {
		items = items[:q.Limit]
	}

	return items, total, nil
}

// digestLocked returns every matching item, newest first, hiding images that
// many articles share (site logos), like the Postgres query does.
func (m *Memory) digestLocked(q DigestQuery, source string) []Item {
	needle := strings.ToLower(q.Query)

	shared := m.sharedImagesLocked()

	var items []Item
	for hash, p := range m.pages {
		ts := p.Published
		if ts.IsZero() {
			ts = m.seen[hash]
		}
		hay := strings.ToLower(p.Title + " " + p.Description + " " + p.H1 + " " + p.Snippet)
		host := pageHost(p.URL)
		if ts.Before(q.Since) || !strings.Contains(hay, needle) ||
			(source != "" && host != source) || (q.ArticlesOnly && !p.IsArticle) {
			continue
		}

		image := effectiveImage(p, shared)
		items = append(items, Item{
			ID: hash, URL: p.URL, Title: p.Title, Description: p.Description, Snippet: p.Snippet,
			Source: host, Image: image, Summary: m.summaries[hash], IsArticle: p.IsArticle, Time: ts,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		if !items[i].Time.Equal(items[j].Time) {
			return items[i].Time.After(items[j].Time)
		}
		return items[i].URL < items[j].URL
	})

	return items
}

func (m *Memory) DigestSources(_ context.Context, q DigestQuery) ([]SourceCount, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	counts := map[string]int{}
	for _, it := range m.digestLocked(q, "") {
		counts[it.Source]++
	}

	out := []SourceCount{}
	for host, n := range counts {
		out = append(out, SourceCount{Source: host, Count: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Source < out[j].Source
	})

	return out, nil
}

func (m *Memory) ImageURL(_ context.Context, urlHash string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	return effectiveImage(m.pages[urlHash], m.sharedImagesLocked()), nil
}

// sharedImagesLocked counts, per image URL, the distinct articles using it as
// their og:image or their body image.
func (m *Memory) sharedImagesLocked() map[string]int {
	shared := map[string]int{}
	for _, p := range m.pages {
		if !p.IsArticle {
			continue
		}
		if p.Image != "" {
			shared[p.Image]++
		}
		if p.BodyImage != "" && p.BodyImage != p.Image {
			shared[p.BodyImage]++
		}
	}
	return shared
}

// effectiveImage mirrors effectiveImageSQL: the og:image unless it is a shared
// logo, else the body image unless that is shared too, else none.
func effectiveImage(p Page, shared map[string]int) string {
	if p.Image != "" && shared[p.Image] < genericImageMin {
		return p.Image
	}
	if p.BodyImage != "" && shared[p.BodyImage] < genericImageMin {
		return p.BodyImage
	}
	return ""
}

func (m *Memory) PendingSummaries(_ context.Context, since time.Time, limit int) ([]Unsummarized, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var out []Unsummarized
	for hash, p := range m.pages {
		ts := p.Published
		if ts.IsZero() {
			ts = m.seen[hash]
		}
		if _, done := m.summaries[hash]; done || !p.IsArticle || ts.Before(since) {
			continue
		}
		out = append(out, Unsummarized{URLHash: hash, URL: p.URL, Title: p.Title, Description: p.Description, Content: p.Content})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].URL < out[j].URL })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}

	return out, nil
}

func (m *Memory) PendingImages(_ context.Context, since time.Time, limit int) ([]ImageJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var out []ImageJob
	for hash, p := range m.pages {
		ts := p.Published
		if ts.IsZero() {
			ts = m.seen[hash]
		}
		if !p.IsArticle || p.SourceURL == "" || m.imageChecked[hash] || ts.Before(since) {
			continue
		}
		out = append(out, ImageJob{URLHash: hash, URL: p.URL, SourceURL: p.SourceURL})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].URL < out[j].URL })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}

	return out, nil
}

func (m *Memory) SetImage(_ context.Context, urlHash, image string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if p, ok := m.pages[urlHash]; ok && image != "" {
		p.Image = image
		m.pages[urlHash] = p
	}
	m.imageChecked[urlHash] = true

	return nil
}

func (m *Memory) SetSummary(_ context.Context, urlHash string, bullets []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if bullets == nil {
		bullets = []string{}
	}
	m.summaries[urlHash] = bullets

	return nil
}

// pageHost returns the lowercase host[:port] of rawURL, "" when unparsable.
func pageHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Host)
}

func (m *Memory) Close() error { return nil }

// Pages returns a copy of everything saved, keyed by URL.
func (m *Memory) Pages() map[string]Page {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := make(map[string]Page, len(m.pages))
	for _, p := range m.pages {
		out[p.URL] = p
	}

	return out
}

var _ Store = (*Memory)(nil)
