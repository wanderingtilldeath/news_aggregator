package frontier

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Postgres is the durable Frontier. Several processes can share one database:
// rows are claimed with FOR UPDATE SKIP LOCKED, so workers never block on or
// double-claim each other's rows, and a lease timeout recovers rows held by a
// process that died.
type Postgres struct {
	pool *pgxpool.Pool
}

// NewPostgres wraps a pool whose schema has been migrated (see package db).
func NewPostgres(pool *pgxpool.Pool) *Postgres {
	return &Postgres{pool: pool}
}

func (p *Postgres) CreateCrawl(ctx context.Context, spec CrawlSpec) (Crawl, error) {
	allowed, err := normalizeAllowed(spec.Seed, spec.AllowedHosts)
	if err != nil {
		return Crawl{}, err
	}

	crawl := newCrawl(spec, allowed)

	err = p.pool.QueryRow(ctx,
		`INSERT INTO crawls (seed, max_depth, allowed_hosts, path_prefixes, article_prefixes, split_articles, image_from_source, category)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id, created_at`,
		spec.Seed, spec.MaxDepth, allowed, crawl.PathPrefixes, crawl.ArticlePrefixes, crawl.SplitArticles, crawl.ImageFromSource, crawl.Category,
	).Scan(&crawl.ID, &crawl.CreatedAt)
	if err != nil {
		return Crawl{}, fmt.Errorf("frontier: create crawl: %w", err)
	}

	return crawl, nil
}

func (p *Postgres) GetCrawl(ctx context.Context, id int64) (Crawl, error) {
	var crawl Crawl

	err := p.pool.QueryRow(ctx,
		`SELECT id, seed, max_depth, allowed_hosts, path_prefixes, article_prefixes, split_articles, image_from_source, category, created_at FROM crawls WHERE id = $1`, id,
	).Scan(&crawl.ID, &crawl.Seed, &crawl.MaxDepth, &crawl.AllowedHosts, &crawl.PathPrefixes, &crawl.ArticlePrefixes, &crawl.SplitArticles, &crawl.ImageFromSource, &crawl.Category, &crawl.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Crawl{}, ErrNotFound
	}
	if err != nil {
		return Crawl{}, fmt.Errorf("frontier: get crawl: %w", err)
	}

	return crawl, nil
}

func (p *Postgres) ListCrawls(ctx context.Context, limit int) ([]Crawl, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := p.pool.Query(ctx,
		`SELECT id, seed, max_depth, allowed_hosts, path_prefixes, article_prefixes, split_articles, image_from_source, category, created_at FROM crawls ORDER BY id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("frontier: list crawls: %w", err)
	}
	defer rows.Close()

	crawls := []Crawl{}
	for rows.Next() {
		var c Crawl
		if err := rows.Scan(&c.ID, &c.Seed, &c.MaxDepth, &c.AllowedHosts, &c.PathPrefixes, &c.ArticlePrefixes, &c.SplitArticles, &c.ImageFromSource, &c.Category, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("frontier: list crawls scan: %w", err)
		}
		crawls = append(crawls, c)
	}

	return crawls, rows.Err()
}

func (p *Postgres) DepthStats(ctx context.Context, crawlID int64) ([]DepthRow, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT depth, status, count(*) FROM frontier WHERE crawl_id = $1 GROUP BY depth, status ORDER BY depth`, crawlID)
	if err != nil {
		return nil, fmt.Errorf("frontier: depth stats: %w", err)
	}
	defer rows.Close()

	out := []DepthRow{}
	for rows.Next() {
		var (
			depth  int
			status string
			n      int64
		)
		if err := rows.Scan(&depth, &status, &n); err != nil {
			return nil, fmt.Errorf("frontier: depth stats scan: %w", err)
		}
		if len(out) == 0 || out[len(out)-1].Depth != depth {
			out = append(out, DepthRow{Depth: depth, Stats: map[string]int64{}})
		}
		out[len(out)-1].Stats[status] = n
	}

	return out, rows.Err()
}

// enqueueSQL inserts new rows and, for a URL that is still waiting in the
// queue, lowers its depth if it was just rediscovered by a shorter path. The
// returned flag is true for rows that were really inserted (xmax = 0), not
// merely updated.
const enqueueSQL = `
WITH upserted AS (
    INSERT INTO frontier (crawl_id, url_hash, url, host, depth)
    SELECT $1, h, u, o, d FROM unnest($2::text[], $3::text[], $4::text[], $5::int[]) AS t(h, u, o, d)
    ON CONFLICT (crawl_id, url_hash) DO UPDATE
        SET depth = EXCLUDED.depth, updated_at = now()
        WHERE frontier.status = 'pending' AND EXCLUDED.depth < frontier.depth
    RETURNING (xmax = 0) AS inserted
)
SELECT count(*) FILTER (WHERE inserted) FROM upserted`

func (p *Postgres) Enqueue(ctx context.Context, crawlID int64, items []NewItem) (int, error) {
	items = dedupeShallowest(items)
	if len(items) == 0 {
		return 0, nil
	}

	hashes := make([]string, len(items))
	urls := make([]string, len(items))
	hosts := make([]string, len(items))
	depths := make([]int32, len(items))
	for i, it := range items {
		hashes[i], urls[i], hosts[i], depths[i] = Hash(it.URL), it.URL, hostOf(it.URL), int32(it.Depth)
	}

	var inserted int
	if err := p.pool.QueryRow(ctx, enqueueSQL, crawlID, hashes, urls, hosts, depths).Scan(&inserted); err != nil {
		return 0, fmt.Errorf("frontier: enqueue: %w", err)
	}

	return inserted, nil
}

// dedupeShallowest collapses repeated URLs within one batch, keeping the
// smallest depth. (A single INSERT ... ON CONFLICT DO UPDATE may not touch the
// same row twice.)
func dedupeShallowest(items []NewItem) []NewItem {
	index := make(map[string]int, len(items))
	out := make([]NewItem, 0, len(items))

	for _, it := range items {
		if i, seen := index[it.URL]; seen {
			out[i].Depth = min(out[i].Depth, it.Depth)
			continue
		}
		index[it.URL] = len(out)
		out = append(out, it)
	}

	return out
}

// claimSQL picks due rows (pending, or leased with an expired lease) under
// SKIP LOCKED, then leases them in the same statement. A reclaimed row had a
// previous holder that never finished, which counts as a failed attempt.
const claimSQL = `
WITH picked AS (
    SELECT crawl_id, url_hash
    FROM frontier
    WHERE next_fetch_at <= now()
      AND (status = 'pending' OR (status = 'leased' AND lease_until < now()))
    ORDER BY depth, next_fetch_at
    LIMIT $1
    FOR UPDATE SKIP LOCKED
)
UPDATE frontier f
SET attempts    = f.attempts + CASE WHEN f.status = 'leased' THEN 1 ELSE 0 END,
    status      = 'leased',
    lease_until = now() + make_interval(secs => $2),
    updated_at  = now()
FROM picked
WHERE f.crawl_id = picked.crawl_id AND f.url_hash = picked.url_hash
RETURNING f.crawl_id, f.url_hash, f.url, f.host, f.depth, f.attempts`

func (p *Postgres) Claim(ctx context.Context, n int, lease time.Duration) ([]Item, error) {
	rows, err := p.pool.Query(ctx, claimSQL, n, lease.Seconds())
	if err != nil {
		return nil, fmt.Errorf("frontier: claim: %w", err)
	}
	defer rows.Close()

	var items []Item
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.CrawlID, &it.URLHash, &it.URL, &it.Host, &it.Depth, &it.Attempts); err != nil {
			return nil, fmt.Errorf("frontier: claim scan: %w", err)
		}
		items = append(items, it)
	}

	return items, rows.Err()
}

func (p *Postgres) Finish(ctx context.Context, it Item, status Status, note string) error {
	_, err := p.pool.Exec(ctx, `
		UPDATE frontier
		SET status = $3, last_error = NULLIF($4, ''), lease_until = NULL, updated_at = now()
		WHERE crawl_id = $1 AND url_hash = $2 AND status = 'leased'`,
		it.CrawlID, it.URLHash, string(status), note)
	if err != nil {
		return fmt.Errorf("frontier: finish: %w", err)
	}
	return nil
}

func (p *Postgres) Reschedule(ctx context.Context, it Item, at time.Time, countAttempt bool, note string) error {
	_, err := p.pool.Exec(ctx, `
		UPDATE frontier
		SET status = 'pending', next_fetch_at = $3, lease_until = NULL,
		    attempts = attempts + CASE WHEN $4 THEN 1 ELSE 0 END,
		    last_error = COALESCE(NULLIF($5, ''), last_error), updated_at = now()
		WHERE crawl_id = $1 AND url_hash = $2 AND status = 'leased'`,
		it.CrawlID, it.URLHash, at, countAttempt, note)
	if err != nil {
		return fmt.Errorf("frontier: reschedule: %w", err)
	}
	return nil
}

func (p *Postgres) Release(ctx context.Context, items []Item) error {
	if len(items) == 0 {
		return nil
	}

	crawlIDs := make([]int64, len(items))
	hashes := make([]string, len(items))
	for i, it := range items {
		crawlIDs[i], hashes[i] = it.CrawlID, it.URLHash
	}

	_, err := p.pool.Exec(ctx, `
		UPDATE frontier f
		SET status = 'pending', lease_until = NULL, updated_at = now()
		FROM unnest($1::bigint[], $2::text[]) AS t(c, h)
		WHERE f.crawl_id = t.c AND f.url_hash = t.h AND f.status = 'leased'`,
		crawlIDs, hashes)
	if err != nil {
		return fmt.Errorf("frontier: release: %w", err)
	}
	return nil
}

func (p *Postgres) Stats(ctx context.Context, crawlID int64) (Stats, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT status, count(*) FROM frontier WHERE crawl_id = $1 GROUP BY status`, crawlID)
	if err != nil {
		return nil, fmt.Errorf("frontier: stats: %w", err)
	}
	defer rows.Close()

	stats := make(Stats)
	for rows.Next() {
		var (
			status string
			n      int64
		)
		if err := rows.Scan(&status, &n); err != nil {
			return nil, fmt.Errorf("frontier: stats scan: %w", err)
		}
		stats[Status(status)] = n
	}

	return stats, rows.Err()
}

func (p *Postgres) QueueDepth(ctx context.Context) (int64, error) {
	var n int64
	if err := p.pool.QueryRow(ctx, `SELECT count(*) FROM frontier WHERE status = 'pending'`).Scan(&n); err != nil {
		return 0, fmt.Errorf("frontier: queue depth: %w", err)
	}
	return n, nil
}

var _ Frontier = (*Postgres)(nil)
