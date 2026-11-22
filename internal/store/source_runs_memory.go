package store

import (
	"context"
	"sort"
	"time"
)

func (m *Memory) RecordSourceRuns(_ context.Context, runs []SourceRun) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.sourceRuns = append(m.sourceRuns, runs...)
	keep := m.sourceRuns[:0]
	for _, r := range m.sourceRuns {
		if r.At.After(time.Now().AddDate(0, 0, -60)) {
			keep = append(keep, r)
		}
	}
	m.sourceRuns = keep

	return nil
}

func (m *Memory) SourceHealth(_ context.Context, since, recent time.Time) ([]SourceHealth, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	by := map[string]*SourceHealth{}
	for _, r := range m.sourceRuns {
		if r.At.Before(since) {
			continue
		}
		h := by[r.Source]
		if h == nil {
			h = &SourceHealth{Source: r.Source}
			by[r.Source] = h
		}
		if r.At.After(h.LastRun) || h.LastRun.IsZero() {
			h.LastRun, h.LastOutcome, h.Host = r.At, r.Outcome, r.Host
		}
		if r.Articles > 0 && r.At.After(h.LastOK) {
			h.LastOK = r.At
		}
		h.Runs++
		h.Articles += r.Articles
		h.Failed += r.Failed
		if !r.At.Before(recent) {
			h.RecentArticles += r.Articles
		}
	}
	out := make([]SourceHealth, 0, len(by))
	for _, h := range by {
		out = append(out, *h)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Source < out[j].Source })

	return out, nil
}

func (m *Memory) ArticlesByCrawl(_ context.Context, crawlIDs []int64) (map[int64]int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	want := map[int64]bool{}
	for _, id := range crawlIDs {
		want[id] = true
	}
	out := map[int64]int{}
	for _, p := range m.pages {
		if p.IsArticle && want[p.CrawlID] {
			out[p.CrawlID]++
		}
	}

	return out, nil
}
