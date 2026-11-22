package store

import (
	"context"
	"fmt"
	"time"
)

func (s *Postgres) RecordSourceRuns(ctx context.Context, runs []SourceRun) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("store: record source runs: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, r := range runs {
		if _, err := tx.Exec(ctx, `INSERT INTO source_runs (at, source, host, pages, articles, failed, outcome) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			r.At, r.Source, r.Host, r.Pages, r.Articles, r.Failed, r.Outcome); err != nil {
			return fmt.Errorf("store: record source runs: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM source_runs WHERE at < now() - interval '60 days'`); err != nil {
		return fmt.Errorf("store: record source runs: %w", err)
	}

	return tx.Commit(ctx)
}

func (s *Postgres) SourceHealth(ctx context.Context, since, recent time.Time) ([]SourceHealth, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT source, (array_agg(host ORDER BY at DESC))[1], max(at), (array_agg(outcome ORDER BY at DESC))[1],
		       COALESCE(max(at) FILTER (WHERE articles > 0), 'epoch'::timestamptz), count(*), sum(articles),
		       COALESCE(sum(articles) FILTER (WHERE at >= $2), 0), sum(failed)
		FROM source_runs WHERE at >= $1 GROUP BY source ORDER BY source`, since, recent)
	if err != nil {
		return nil, fmt.Errorf("store: source health: %w", err)
	}
	defer rows.Close()

	out := []SourceHealth{}
	for rows.Next() {
		var h SourceHealth
		if err := rows.Scan(&h.Source, &h.Host, &h.LastRun, &h.LastOutcome, &h.LastOK, &h.Runs, &h.Articles, &h.RecentArticles, &h.Failed); err != nil {
			return nil, fmt.Errorf("store: source health scan: %w", err)
		}
		if h.LastOK.Unix() == 0 {
			h.LastOK = time.Time{}
		}
		out = append(out, h)
	}

	return out, rows.Err()
}

func (s *Postgres) ArticlesByCrawl(ctx context.Context, crawlIDs []int64) (map[int64]int, error) {
	out := map[int64]int{}
	if len(crawlIDs) == 0 {
		return out, nil
	}
	rows, err := s.pool.Query(ctx, `SELECT crawl_id, count(*) FROM pages WHERE is_article AND crawl_id = ANY($1) GROUP BY crawl_id`, crawlIDs)
	if err != nil {
		return nil, fmt.Errorf("store: articles by crawl: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, fmt.Errorf("store: articles by crawl scan: %w", err)
		}
		out[id] = n
	}

	return out, rows.Err()
}
