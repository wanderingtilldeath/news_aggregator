package frontier

import (
	"context"
	"sort"
	"sync"
	"time"
)

type memKey struct {
	crawlID int64
	hash    string
}

type memRow struct {
	item        Item
	status      Status
	nextFetchAt time.Time
	leaseUntil  time.Time
	note        string
}

// Memory is an in-process Frontier. It is unbounded and never blocks on
// Enqueue, so it is the fast backend for tests and has the same semantics as
// the Postgres implementation. State is lost on exit.
type Memory struct {
	mu     sync.Mutex
	now    func() time.Time
	nextID int64
	crawls map[int64]Crawl
	rows   map[memKey]*memRow
}

// NewMemory returns an empty in-memory frontier. now may be nil (time.Now).
func NewMemory(now func() time.Time) *Memory {
	if now == nil {
		now = time.Now
	}
	return &Memory{
		now:    now,
		crawls: make(map[int64]Crawl),
		rows:   make(map[memKey]*memRow),
	}
}

func (m *Memory) CreateCrawl(_ context.Context, seed string, maxDepth int, allowedHosts, pathPrefixes []string) (Crawl, error) {
	allowed, err := normalizeAllowed(seed, allowedHosts)
	if err != nil {
		return Crawl{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.nextID++
	crawl := Crawl{ID: m.nextID, Seed: seed, MaxDepth: maxDepth, AllowedHosts: allowed, PathPrefixes: normalizePrefixes(pathPrefixes), CreatedAt: m.now()}
	m.crawls[crawl.ID] = crawl

	return crawl, nil
}

func (m *Memory) GetCrawl(_ context.Context, id int64) (Crawl, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	crawl, ok := m.crawls[id]
	if !ok {
		return Crawl{}, ErrNotFound
	}
	return crawl, nil
}

func (m *Memory) ListCrawls(_ context.Context, limit int) ([]Crawl, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := make([]Crawl, 0, len(m.crawls))
	for _, c := range m.crawls {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}

	return out, nil
}

func (m *Memory) DepthStats(_ context.Context, crawlID int64) ([]DepthRow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	byDepth := map[int]map[string]int64{}
	for key, row := range m.rows {
		if key.crawlID != crawlID {
			continue
		}
		if byDepth[row.item.Depth] == nil {
			byDepth[row.item.Depth] = map[string]int64{}
		}
		byDepth[row.item.Depth][string(row.status)]++
	}

	out := make([]DepthRow, 0, len(byDepth))
	for depth, stats := range byDepth {
		out = append(out, DepthRow{Depth: depth, Stats: stats})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Depth < out[j].Depth })

	return out, nil
}

func (m *Memory) Enqueue(_ context.Context, crawlID int64, items []NewItem) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.crawls[crawlID]; !ok {
		return 0, ErrNotFound
	}

	added := 0
	for _, it := range items {
		hash := Hash(it.URL)
		key := memKey{crawlID, hash}
		if row, exists := m.rows[key]; exists {
			// Rediscovered by a shorter path while still waiting: keep the
			// shallowest depth so the page is expanded as far as it should be.
			if row.status == Pending && it.Depth < row.item.Depth {
				row.item.Depth = it.Depth
			}
			continue
		}

		m.rows[key] = &memRow{
			item:        Item{CrawlID: crawlID, URLHash: hash, URL: it.URL, Host: hostOf(it.URL), Depth: it.Depth},
			status:      Pending,
			nextFetchAt: m.now(),
		}
		added++
	}

	return added, nil
}

func (m *Memory) Claim(_ context.Context, n int, lease time.Duration) ([]Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.now()

	var due []*memRow
	for _, row := range m.rows {
		switch {
		case row.status == Pending && !row.nextFetchAt.After(now):
			due = append(due, row)
		case row.status == Leased && row.leaseUntil.Before(now) && !row.nextFetchAt.After(now):
			due = append(due, row)
		}
	}

	// Shallowest first keeps the crawl breadth-first.
	sort.Slice(due, func(i, j int) bool {
		if due[i].item.Depth != due[j].item.Depth {
			return due[i].item.Depth < due[j].item.Depth
		}
		if !due[i].nextFetchAt.Equal(due[j].nextFetchAt) {
			return due[i].nextFetchAt.Before(due[j].nextFetchAt)
		}
		return due[i].item.URL < due[j].item.URL
	})
	if len(due) > n {
		due = due[:n]
	}

	claimed := make([]Item, 0, len(due))
	for _, row := range due {
		if row.status == Leased {
			row.item.Attempts++ // the previous holder never finished
		}
		row.status = Leased
		row.leaseUntil = now.Add(lease)
		claimed = append(claimed, row.item)
	}

	return claimed, nil
}

func (m *Memory) Finish(_ context.Context, it Item, status Status, note string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	row, ok := m.rows[memKey{it.CrawlID, it.URLHash}]
	if !ok || row.status != Leased {
		return nil
	}

	row.status = status
	row.note = note
	row.leaseUntil = time.Time{}
	row.item.Attempts = max(row.item.Attempts, it.Attempts)

	return nil
}

func (m *Memory) Reschedule(_ context.Context, it Item, at time.Time, countAttempt bool, note string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	row, ok := m.rows[memKey{it.CrawlID, it.URLHash}]
	if !ok || row.status != Leased {
		return nil
	}

	row.status = Pending
	row.nextFetchAt = at
	row.leaseUntil = time.Time{}
	row.note = note
	if countAttempt {
		row.item.Attempts++
	}

	return nil
}

func (m *Memory) Release(_ context.Context, items []Item) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, it := range items {
		if row, ok := m.rows[memKey{it.CrawlID, it.URLHash}]; ok && row.status == Leased {
			row.status = Pending
			row.leaseUntil = time.Time{}
		}
	}

	return nil
}

func (m *Memory) Stats(_ context.Context, crawlID int64) (Stats, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	stats := make(Stats)
	for key, row := range m.rows {
		if key.crawlID == crawlID {
			stats[row.status]++
		}
	}

	return stats, nil
}

func (m *Memory) QueueDepth(context.Context) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var n int64
	for _, row := range m.rows {
		if row.status == Pending {
			n++
		}
	}

	return n, nil
}

// Rows returns a snapshot of (url, status) for tests.
func (m *Memory) Rows(crawlID int64) map[string]Status {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := make(map[string]Status)
	for key, row := range m.rows {
		if key.crawlID == crawlID {
			out[row.item.URL] = row.status
		}
	}

	return out
}

var _ Frontier = (*Memory)(nil)
