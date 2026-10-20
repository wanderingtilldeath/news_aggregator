package store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultBatchSize = 64
	defaultLinger    = 10 * time.Millisecond
)

// upsertSQL keeps an existing row when the content has not changed, which
// skips rewriting the row, its tsvector and its GIN entries on a re-crawl.
const upsertSQL = `
INSERT INTO pages (url_hash, crawl_id, url, status_code, content_type, title, description, h1, snippet,
                   content_hash, etag, last_modified, size_bytes, latency_ms, published_at,
                   image_url, is_article, content, source_url, body_image_url)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
ON CONFLICT (url_hash) DO UPDATE SET
    crawl_id = EXCLUDED.crawl_id, url = EXCLUDED.url, status_code = EXCLUDED.status_code,
    content_type = EXCLUDED.content_type, title = EXCLUDED.title, description = EXCLUDED.description,
    h1 = EXCLUDED.h1, snippet = EXCLUDED.snippet, content_hash = EXCLUDED.content_hash,
    etag = EXCLUDED.etag, last_modified = EXCLUDED.last_modified, size_bytes = EXCLUDED.size_bytes,
    latency_ms = EXCLUDED.latency_ms, published_at = EXCLUDED.published_at, fetched_at = now(),
    -- a re-crawl must not replace an image already fetched from the source
    image_url = CASE WHEN pages.image_checked_at IS NOT NULL AND pages.source_url <> '' AND pages.image_url <> ''
                     THEN pages.image_url ELSE EXCLUDED.image_url END,
    is_article = EXCLUDED.is_article, content = EXCLUDED.content, source_url = EXCLUDED.source_url,
    body_image_url = EXCLUDED.body_image_url
WHERE pages.content_hash IS DISTINCT FROM EXCLUDED.content_hash`

type saveRequest struct {
	page Page
	done chan error
}

// Postgres is the production Store. Writes use group commit: concurrent
// Save calls are collected into one batch (up to batchSize rows, or whatever
// arrives within the linger window) and sent in a single round trip. Every
// caller still waits for its own batch to commit before Save returns.
type Postgres struct {
	pool      *pgxpool.Pool
	batchSize int
	linger    time.Duration

	reqs chan saveRequest
	quit chan struct{}
	wg   sync.WaitGroup
	once sync.Once
}

// NewPostgres starts the batch writer. Close it to flush and stop.
func NewPostgres(pool *pgxpool.Pool) *Postgres {
	s := &Postgres{
		pool:      pool,
		batchSize: defaultBatchSize,
		linger:    defaultLinger,
		reqs:      make(chan saveRequest),
		quit:      make(chan struct{}),
	}

	s.wg.Add(1)
	go s.run()

	return s
}

var errClosed = errors.New("store: closed")

func (s *Postgres) Save(ctx context.Context, p Page) error {
	req := saveRequest{page: p, done: make(chan error, 1)}

	select {
	case s.reqs <- req:
	case <-ctx.Done():
		return ctx.Err()
	case <-s.quit:
		return errClosed
	}

	select {
	case err := <-req.done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Postgres) run() {
	defer s.wg.Done()

	for {
		var batch []saveRequest

		select {
		case req := <-s.reqs:
			batch = append(batch, req)
		case <-s.quit:
			return
		}

		timer := time.NewTimer(s.linger)
	collect:
		for len(batch) < s.batchSize {
			select {
			case req := <-s.reqs:
				batch = append(batch, req)
			case <-timer.C:
				break collect
			case <-s.quit:
				break collect
			}
		}
		timer.Stop()

		s.flush(batch)
	}
}

func (s *Postgres) flush(batch []saveRequest) {
	// Detached from any caller's context: a caller that gave up must not
	// cancel the writes of the others in its batch.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pgBatch := &pgx.Batch{}
	for _, req := range batch {
		p := req.page
		pgBatch.Queue(upsertSQL, p.URLHash, p.CrawlID, p.URL, p.StatusCode, p.ContentType, p.Title,
			p.Description, p.H1, p.Snippet, p.ContentHash, p.ETag, p.LastModified, p.SizeBytes,
			int(p.Latency.Milliseconds()), publishedArg(p.Published), p.Image, p.IsArticle, p.Content, p.SourceURL, p.BodyImage)
	}

	results := s.pool.SendBatch(ctx, pgBatch)
	defer results.Close()

	for _, req := range batch {
		_, err := results.Exec()
		if err != nil {
			err = fmt.Errorf("store: save %s: %w", req.page.URL, err)
		}
		req.done <- err
	}
}

// publishedArg maps an unknown (zero) date to SQL NULL.
func publishedArg(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

// genericImageMin is how many articles must share one image URL before it is
// treated as a site logo (or other filler) rather than a picture of the story.
const genericImageMin = 5

// genericCTE lists the images shared by genericImageMin or more distinct
// articles, counting both image columns (UNION, so an article whose og:image
// and body image are the same file counts once).
var genericCTE = `generic AS (
	SELECT u FROM (
		SELECT image_url AS u, url_hash FROM pages WHERE is_article AND image_url <> ''
		UNION
		SELECT body_image_url, url_hash FROM pages WHERE is_article AND body_image_url <> ''
	) t GROUP BY u HAVING count(*) >= ` + fmt.Sprint(genericImageMin) + `
)`

// effectiveImageSQL picks the picture to show: the page's og:image unless it
// is a shared logo, else the article's own first picture unless that is shared
// too, else none. Digest and ImageURL both use it, so the image proxy serves
// exactly what the digest advertises.
const effectiveImageSQL = `CASE
	WHEN image_url <> '' AND image_url NOT IN (SELECT u FROM generic) THEN image_url
	WHEN body_image_url <> '' AND body_image_url NOT IN (SELECT u FROM generic) THEN body_image_url
	ELSE '' END`

// digestWhere is the filter shared by Digest and DigestSources. $1 since,
// $2 full-text query, $3 source host (” = any), $4 articles only.
const digestWhere = `
	WHERE COALESCE(published_at, first_seen_at) >= $1
	  AND ($2 = '' OR search @@ websearch_to_tsquery('english', $2))
	  AND ($3 = '' OR ` + hostSQL + ` = $3)
	  AND (NOT $4 OR is_article)`

func (s *Postgres) Digest(ctx context.Context, q DigestQuery) ([]Item, int, error) {
	limit := q.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := max(q.Offset, 0)

	rows, err := s.pool.Query(ctx, `
		WITH `+genericCTE+`
		SELECT url_hash, url, title, description, snippet, `+hostSQL+`,
		       `+effectiveImageSQL+`,
		       summary, is_article, COALESCE(published_at, first_seen_at) AS ts, count(*) OVER()
		FROM pages`+digestWhere+`
		ORDER BY ts DESC, url
		LIMIT $5 OFFSET $6`, q.Since, q.Query, q.Source, q.ArticlesOnly, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("store: digest: %w", err)
	}
	defer rows.Close()

	items := []Item{}
	total := 0
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.ID, &it.URL, &it.Title, &it.Description, &it.Snippet, &it.Source, &it.Image,
			&it.Summary, &it.IsArticle, &it.Time, &total); err != nil {
			return nil, 0, fmt.Errorf("store: digest scan: %w", err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	// A page past the end has no rows to carry the total.
	if len(items) == 0 && offset > 0 {
		if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM pages`+digestWhere,
			q.Since, q.Query, q.Source, q.ArticlesOnly).Scan(&total); err != nil {
			return nil, 0, fmt.Errorf("store: digest count: %w", err)
		}
	}

	return items, total, nil
}

func (s *Postgres) DigestSources(ctx context.Context, q DigestQuery) ([]SourceCount, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+hostSQL+` AS host, count(*) FROM pages`+digestWhere+`
		GROUP BY host ORDER BY count(*) DESC, host`,
		q.Since, q.Query, "", q.ArticlesOnly)
	if err != nil {
		return nil, fmt.Errorf("store: digest sources: %w", err)
	}
	defer rows.Close()

	out := []SourceCount{}
	for rows.Next() {
		var sc SourceCount
		if err := rows.Scan(&sc.Source, &sc.Count); err != nil {
			return nil, fmt.Errorf("store: digest sources scan: %w", err)
		}
		out = append(out, sc)
	}

	return out, rows.Err()
}

func (s *Postgres) ImageURL(ctx context.Context, urlHash string) (string, error) {
	var img string
	err := s.pool.QueryRow(ctx, `WITH `+genericCTE+` SELECT `+effectiveImageSQL+` FROM pages WHERE url_hash = $1`, urlHash).Scan(&img)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("store: image url: %w", err)
	}
	return img, nil
}

// hostSQL extracts the host[:port] of a page URL.
const hostSQL = `lower(substring(url from '^https?://([^/?#]+)'))`

func (s *Postgres) PendingSummaries(ctx context.Context, since time.Time, limit int) ([]Unsummarized, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT url_hash, url, title, description, content
		FROM pages
		WHERE is_article AND summarized_at IS NULL AND COALESCE(published_at, first_seen_at) >= $1
		ORDER BY COALESCE(published_at, first_seen_at) DESC, url
		LIMIT $2`, since, limit)
	if err != nil {
		return nil, fmt.Errorf("store: pending summaries: %w", err)
	}
	defer rows.Close()

	var out []Unsummarized
	for rows.Next() {
		var u Unsummarized
		if err := rows.Scan(&u.URLHash, &u.URL, &u.Title, &u.Description, &u.Content); err != nil {
			return nil, fmt.Errorf("store: pending summaries scan: %w", err)
		}
		out = append(out, u)
	}

	return out, rows.Err()
}

func (s *Postgres) PendingImages(ctx context.Context, since time.Time, limit int) ([]ImageJob, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT url_hash, url, source_url
		FROM pages
		WHERE is_article AND source_url <> '' AND image_checked_at IS NULL
		  AND COALESCE(published_at, first_seen_at) >= $1
		ORDER BY COALESCE(published_at, first_seen_at) DESC, url
		LIMIT $2`, since, limit)
	if err != nil {
		return nil, fmt.Errorf("store: pending images: %w", err)
	}
	defer rows.Close()

	var out []ImageJob
	for rows.Next() {
		var j ImageJob
		if err := rows.Scan(&j.URLHash, &j.URL, &j.SourceURL); err != nil {
			return nil, fmt.Errorf("store: pending images scan: %w", err)
		}
		out = append(out, j)
	}

	return out, rows.Err()
}

func (s *Postgres) SetImage(ctx context.Context, urlHash, image string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE pages
		SET image_url = CASE WHEN $2 <> '' THEN $2 ELSE image_url END, image_checked_at = now()
		WHERE url_hash = $1`, urlHash, image)
	if err != nil {
		return fmt.Errorf("store: set image: %w", err)
	}
	return nil
}

func (s *Postgres) SetSummary(ctx context.Context, urlHash string, bullets []string) error {
	if bullets == nil {
		bullets = []string{}
	}
	_, err := s.pool.Exec(ctx,
		`UPDATE pages SET summary = $2, summarized_at = now() WHERE url_hash = $1`, urlHash, bullets)
	if err != nil {
		return fmt.Errorf("store: set summary: %w", err)
	}
	return nil
}

func (s *Postgres) Validators(ctx context.Context, urlHash string) (Validators, bool, error) {
	var v Validators

	err := s.pool.QueryRow(ctx,
		`SELECT etag, last_modified FROM pages WHERE url_hash = $1`, urlHash,
	).Scan(&v.ETag, &v.LastModified)
	if errors.Is(err, pgx.ErrNoRows) {
		return Validators{}, false, nil
	}
	if err != nil {
		return Validators{}, false, fmt.Errorf("store: validators: %w", err)
	}

	return v, v.ETag != "" || v.LastModified != "", nil
}

func (s *Postgres) Search(ctx context.Context, q string, limit, offset int) ([]Hit, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset = max(offset, 0)

	rows, err := s.pool.Query(ctx, `
		SELECT url, title, description, snippet, ts_rank(search, query) AS rank, count(*) OVER()
		FROM pages, websearch_to_tsquery('english', $1) AS query
		WHERE search @@ query
		ORDER BY rank DESC, url
		LIMIT $2 OFFSET $3`, q, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("store: search: %w", err)
	}
	defer rows.Close()

	hits := []Hit{}
	total := 0
	for rows.Next() {
		var h Hit
		if err := rows.Scan(&h.URL, &h.Title, &h.Description, &h.Snippet, &h.Rank, &total); err != nil {
			return nil, 0, fmt.Errorf("store: search scan: %w", err)
		}
		hits = append(hits, h)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	if len(hits) == 0 && offset > 0 {
		if err := s.pool.QueryRow(ctx, `
			SELECT count(*) FROM pages, websearch_to_tsquery('english', $1) AS query WHERE search @@ query`, q).Scan(&total); err != nil {
			return nil, 0, fmt.Errorf("store: search count: %w", err)
		}
	}

	return hits, total, nil
}

// Close stops the writer after it finishes the batch in progress.
func (s *Postgres) Close() error {
	s.once.Do(func() { close(s.quit) })
	s.wg.Wait()
	return nil
}

var _ Store = (*Postgres)(nil)
