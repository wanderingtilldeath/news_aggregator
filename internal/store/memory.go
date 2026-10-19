package store

import (
	"context"
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
}

// NewMemory returns an empty in-memory store.
func NewMemory() *Memory {
	return &Memory{pages: make(map[string]Page), seen: make(map[string]time.Time)}
}

func (m *Memory) Save(_ context.Context, p Page) error {
	m.mu.Lock()
	defer m.mu.Unlock()

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

func (m *Memory) Search(_ context.Context, q string, limit int) ([]Hit, error) {
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
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}

	return hits, nil
}

func (m *Memory) Digest(_ context.Context, since time.Time, q string, limit int) ([]Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	q = strings.ToLower(q)

	var items []Item
	for hash, p := range m.pages {
		ts := p.Published
		if ts.IsZero() {
			ts = m.seen[hash]
		}
		hay := strings.ToLower(p.Title + " " + p.Description + " " + p.H1 + " " + p.Snippet)
		if ts.Before(since) || !strings.Contains(hay, q) {
			continue
		}
		items = append(items, Item{URL: p.URL, Title: p.Title, Description: p.Description, Snippet: p.Snippet, Time: ts})
	}

	sort.Slice(items, func(i, j int) bool {
		if !items[i].Time.Equal(items[j].Time) {
			return items[i].Time.After(items[j].Time)
		}
		return items[i].URL < items[j].URL
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}

	return items, nil
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
