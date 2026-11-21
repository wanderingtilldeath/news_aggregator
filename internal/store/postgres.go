package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wanderingtilldeath/news-aggregator/internal/prose"
	"github.com/wanderingtilldeath/news-aggregator/internal/track"
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
                   image_url, is_article, content, source_url, body_image_url, category, words)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22)
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
    body_image_url = EXCLUDED.body_image_url, category = EXCLUDED.category, words = EXCLUDED.words
-- a changed category or date alone also updates the row, so old rows pick up
-- categories and a corrected publish date (image addresses too, below)
WHERE pages.content_hash IS DISTINCT FROM EXCLUDED.content_hash
   OR pages.category IS DISTINCT FROM EXCLUDED.category
   OR pages.published_at IS DISTINCT FROM EXCLUDED.published_at
   OR pages.body_image_url IS DISTINCT FROM EXCLUDED.body_image_url
   OR pages.words IS DISTINCT FROM EXCLUDED.words
   -- a changed image address also updates the row (an earlier parser rewrote signed addresses), except
   -- where the image was fetched from the source page and is kept
   OR (NOT (pages.image_checked_at IS NOT NULL AND pages.source_url <> '' AND pages.image_url <> '')
       AND pages.image_url IS DISTINCT FROM EXCLUDED.image_url)`

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
		pgBatch.Queue(upsertSQL, upsertArgs(req.page)...)
	}

	results := s.pool.SendBatch(ctx, pgBatch)
	errs := make([]error, len(batch))
	failed := false
	for i := range batch {
		if _, errs[i] = results.Exec(); errs[i] != nil {
			failed = true
		}
	}
	if results.Close() != nil {
		failed = true
	}

	// A batch runs in one implicit transaction: one bad row rolls back the rows before it that
	// already answered "ok" and fails the ones after it. So when anything failed, save the rows one
	// by one (the upsert is idempotent), and only the bad row reports its error.
	if failed {
		for i, req := range batch {
			_, errs[i] = s.pool.Exec(ctx, upsertSQL, upsertArgs(req.page)...)
		}
	}

	for i, req := range batch {
		if errs[i] != nil {
			errs[i] = fmt.Errorf("store: save %s: %w", req.page.URL, errs[i])
		}
		req.done <- errs[i]
	}
}

// upsertArgs lists a page's values in the order upsertSQL expects. Text is made valid UTF-8 and
// free of NUL bytes first: Postgres rejects both, and one page in a mis-encoded site would
// otherwise fail its whole batch.
func upsertArgs(p Page) []any {
	return []any{p.URLHash, p.CrawlID, cleanText(p.URL), p.StatusCode, cleanText(p.ContentType), cleanText(p.Title),
		cleanText(p.Description), cleanText(p.H1), cleanText(p.Snippet), p.ContentHash, cleanText(p.ETag), cleanText(p.LastModified), p.SizeBytes,
		int(p.Latency.Milliseconds()), publishedArg(p.Published), cleanText(p.Image), p.IsArticle, cleanText(p.Content),
		cleanText(p.SourceURL), cleanText(p.BodyImage), cleanText(p.Category), p.Words}
}

func cleanText(s string) string {
	return strings.ReplaceAll(strings.ToValidUTF8(s, "\uFFFD"), "\x00", "")
}

// publishedArg maps an unknown (zero) date to SQL NULL.
func publishedArg(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

// genericImageMin is how many articles must share one image URL before it is
// treated as a site logo (or other filler) rather than a picture of the story,
// and genericImageHosts the most sites that may share it: a logo belongs to one
// site, while a wire photo that many sites run is a real picture and is kept.
const (
	genericImageMin   = 5
	genericImageHosts = 2
)

// genericCTE lists the images shared by genericImageMin or more distinct
// articles on at most genericImageHosts sites, counting both image columns
// (UNION, so an article whose og:image and body image are the same file counts once).
var genericCTE = `generic AS (
	SELECT u FROM (
		SELECT image_url AS u, url_hash, ` + hostSQL + ` AS h FROM pages WHERE is_article AND image_url <> ''
		UNION
		SELECT body_image_url, url_hash, ` + hostSQL + ` FROM pages WHERE is_article AND body_image_url <> ''
	) t GROUP BY u HAVING count(DISTINCT url_hash) >= ` + fmt.Sprint(genericImageMin) + ` AND count(DISTINCT h) <= ` + fmt.Sprint(genericImageHosts) + `
)`

// effectiveImageSQL picks the picture to show: the page's og:image unless it
// is a shared logo, else the article's own first picture unless that is shared
// too, else none. Digest and ImageURL both use it, so the image proxy serves
// exactly what the digest advertises.
const effectiveImageSQL = `CASE
	WHEN image_url <> '' AND image_url NOT IN (SELECT u FROM generic) THEN image_url
	WHEN body_image_url <> '' AND body_image_url NOT IN (SELECT u FROM generic) THEN body_image_url
	ELSE '' END`

// digestWhere is the filter shared by Digest and DigestSources. $6 is the optional
// upper bound (NULL = none). $1 since,
// $2 full-text query, $3 source host (” = any), $4 articles only, $5 category
// (” = any; a page with no category counts as "other").
// A page dated more than a day ahead has a wrong date and is left out.
const digestWhere = `
	WHERE COALESCE(published_at, first_seen_at) >= $1
	  AND COALESCE(published_at, first_seen_at) <= now() + interval '1 day'
	  AND ($2 = '' OR search @@ websearch_to_tsquery('english', $2))
	  AND ($3 = '' OR ` + hostSQL + ` = $3)
	  AND (NOT $4 OR is_article)
	  AND ($5 = '' OR ` + categorySQL + ` = $5)
	  AND ($6::timestamptz IS NULL OR COALESCE(published_at, first_seen_at) < $6)`

// untilArg is the $6 argument of digestWhere: NULL when there is no upper bound.
func untilArg(t time.Time) any {
	if t.IsZero() {
		return nil
	}

	return t
}

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
		       summary, is_article, COALESCE(published_at, first_seen_at) AS ts, `+categorySQL+`,
		       why, tags, importance, CASE WHEN $7 THEN long_summary ELSE '' END,
		       rel_prelims, rel_mains, rel_essay, rel_ethics, exam IS NOT NULL, coverage, words, count(*) OVER()
		FROM pages`+digestWhere+`
		ORDER BY ts DESC, url
		LIMIT $8 OFFSET $9`, q.Since, q.Query, q.Source, q.ArticlesOnly, q.Category, untilArg(q.Until), q.Detail, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("store: digest: %w", err)
	}
	defer rows.Close()

	items := []Item{}
	total := 0
	for rows.Next() {
		var it Item
		var rel [4]int
		var hasExam bool
		if err := rows.Scan(&it.ID, &it.URL, &it.Title, &it.Description, &it.Snippet, &it.Source, &it.Image,
			&it.Summary, &it.IsArticle, &it.Time, &it.Category, &it.Why, &it.Tags, &it.Importance, &it.Long,
			&rel[0], &rel[1], &rel[2], &rel[3], &hasExam, &it.Coverage, &it.Words, &total); err != nil {
			return nil, 0, fmt.Errorf("store: digest scan: %w", err)
		}
		if hasExam {
			it.Rel = rel[:]
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	// A page past the end has no rows to carry the total.
	if len(items) == 0 && offset > 0 {
		if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM pages`+digestWhere,
			q.Since, q.Query, q.Source, q.ArticlesOnly, q.Category, untilArg(q.Until)).Scan(&total); err != nil {
			return nil, 0, fmt.Errorf("store: digest count: %w", err)
		}
	}

	return items, total, nil
}

func (s *Postgres) Item(ctx context.Context, id string) (Item, bool, error) {
	var it Item
	var examJSON []byte
	var storyID *string
	var content string
	err := s.pool.QueryRow(ctx, `
		WITH `+genericCTE+`
		SELECT url_hash, url, title, description, snippet, `+hostSQL+`,
		       `+effectiveImageSQL+`,
		       summary, is_article, COALESCE(published_at, first_seen_at), `+categorySQL+`,
		       why, tags, importance, exam, coverage, story_id, content, entities, words
		FROM pages
		WHERE url_hash = $1 AND is_article`, id).Scan(&it.ID, &it.URL, &it.Title, &it.Description, &it.Snippet, &it.Source, &it.Image,
		&it.Summary, &it.IsArticle, &it.Time, &it.Category, &it.Why, &it.Tags, &it.Importance, &examJSON, &it.Coverage, &storyID, &content, &it.Entities, &it.Words)
	if errors.Is(err, pgx.ErrNoRows) {
		return Item{}, false, nil
	}
	if err != nil {
		return Item{}, false, fmt.Errorf("store: item: %w", err)
	}
	if len(examJSON) > 0 {
		var lens ExamLens
		if err := json.Unmarshal(examJSON, &lens); err != nil {
			return Item{}, false, fmt.Errorf("store: decode exam lens: %w", err)
		}
		it.Exam, it.Rel = &lens, lens.Rel()
	}
	it.Lead = prose.Lead(content, leadSentences, leadWords)
	if storyID != nil {
		rows, err := s.pool.Query(ctx, `
			SELECT url_hash, title, `+hostSQL+`, COALESCE(published_at, first_seen_at)
			FROM pages
			WHERE story_id = $1 AND url_hash <> $2 AND is_article
			ORDER BY COALESCE(published_at, first_seen_at) DESC, url
			LIMIT $3`, *storyID, id, maxAlso)
		if err != nil {
			return Item{}, false, fmt.Errorf("store: item related: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var r Related
			if err := rows.Scan(&r.ID, &r.Title, &r.Source, &r.Time); err != nil {
				return Item{}, false, fmt.Errorf("store: item related scan: %w", err)
			}
			it.Also = append(it.Also, r)
		}
		if err := rows.Err(); err != nil {
			return Item{}, false, err
		}
	}

	return it, true, nil
}

// maxAlso caps the "also covered by" list on a story.
const maxAlso = 12

func (s *Postgres) GroupCandidates(ctx context.Context, since time.Time, limit int) ([]Candidate, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT url_hash, `+hostSQL+`, title, COALESCE(published_at, first_seen_at)
		FROM pages
		WHERE is_article AND title <> '' AND COALESCE(published_at, first_seen_at) >= $1
		ORDER BY COALESCE(published_at, first_seen_at) DESC, url
		LIMIT $2`, since, limit)
	if err != nil {
		return nil, fmt.Errorf("store: group candidates: %w", err)
	}
	defer rows.Close()

	var out []Candidate
	for rows.Next() {
		var c Candidate
		if err := rows.Scan(&c.ID, &c.Source, &c.Title, &c.Time); err != nil {
			return nil, fmt.Errorf("store: group candidates scan: %w", err)
		}
		out = append(out, c)
	}

	return out, rows.Err()
}

func (s *Postgres) SetStories(ctx context.Context, since time.Time, groups map[string]StoryGroup) error {
	ids := make([]string, 0, len(groups))
	stories := make([]string, 0, len(groups))
	counts := make([]int32, 0, len(groups))
	for id, g := range groups {
		ids, stories, counts = append(ids, id), append(stories, g.ID), append(counts, int32(g.Sources))
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("store: set stories: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after Commit

	if _, err := tx.Exec(ctx, `
		UPDATE pages SET story_id = NULL, coverage = 0
		WHERE story_id IS NOT NULL AND COALESCE(published_at, first_seen_at) >= $1 AND NOT url_hash = ANY($2)`, since, ids); err != nil {
		return fmt.Errorf("store: clear stories: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE pages SET story_id = g.story, coverage = g.n
		FROM unnest($1::text[], $2::text[], $3::int[]) AS g(id, story, n)
		WHERE pages.url_hash = g.id AND (pages.story_id IS DISTINCT FROM g.story OR pages.coverage <> g.n)`, ids, stories, counts); err != nil {
		return fmt.Errorf("store: set stories: %w", err)
	}

	return tx.Commit(ctx)
}

func (s *Postgres) DigestSources(ctx context.Context, q DigestQuery) ([]SourceCount, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+hostSQL+` AS host, `+categorySQL+` AS cat, count(*) FROM pages`+digestWhere+`
		GROUP BY host, cat ORDER BY count(*) DESC, host, cat`,
		q.Since, q.Query, "", q.ArticlesOnly, q.Category, untilArg(q.Until))
	if err != nil {
		return nil, fmt.Errorf("store: digest sources: %w", err)
	}
	defer rows.Close()

	out := []SourceCount{}
	for rows.Next() {
		var sc SourceCount
		if err := rows.Scan(&sc.Source, &sc.Category, &sc.Count); err != nil {
			return nil, fmt.Errorf("store: digest sources scan: %w", err)
		}
		out = append(out, sc)
	}

	return out, rows.Err()
}

func (s *Postgres) ImageStats(ctx context.Context, since time.Time) (ImageCoverage, error) {
	var c ImageCoverage
	err := s.pool.QueryRow(ctx, `WITH `+genericCTE+`
		SELECT count(*) FILTER (WHERE shown <> ''),
		       count(*) FILTER (WHERE shown = '' AND (image_url <> '' OR body_image_url <> '')),
		       count(*) FILTER (WHERE shown = '' AND image_url = '' AND body_image_url = '')
		FROM (SELECT image_url, body_image_url, `+effectiveImageSQL+` AS shown FROM pages
		      WHERE is_article AND COALESCE(published_at, first_seen_at) >= $1) t`, since).Scan(&c.Shown, &c.Logo, &c.None)
	if err != nil {
		return ImageCoverage{}, fmt.Errorf("store: image stats: %w", err)
	}
	return c, nil
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

// hostSQL extracts the host[:port] of a page URL, "" when it is not an http(s)
// URL (a NULL here would fail the scan and take the whole digest down).
// categorySQL is a page's category, "other" when its crawl set none.
const categorySQL = `COALESCE(NULLIF(category, ''), 'other')`

const hostSQL = `COALESCE(lower(substring(url from '^https?://([^/?#]+)')), '')`

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

func (s *Postgres) PendingEnrichment(ctx context.Context, since time.Time, limit int) ([]Unenriched, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT url_hash, url, title, description, content, `+categorySQL+`
		FROM pages
		WHERE is_article AND summarized_at IS NOT NULL AND enriched_at IS NULL
		  AND COALESCE(published_at, first_seen_at) >= $1
		ORDER BY COALESCE(published_at, first_seen_at) DESC, url
		LIMIT $2`, since, limit)
	if err != nil {
		return nil, fmt.Errorf("store: pending enrichment: %w", err)
	}
	defer rows.Close()

	var out []Unenriched
	for rows.Next() {
		var u Unenriched
		if err := rows.Scan(&u.URLHash, &u.URL, &u.Title, &u.Description, &u.Content, &u.Category); err != nil {
			return nil, fmt.Errorf("store: pending enrichment scan: %w", err)
		}
		out = append(out, u)
	}

	return out, rows.Err()
}

func (s *Postgres) SetEnrichment(ctx context.Context, urlHash string, e Enrichment) error {
	if e.Tags == nil {
		e.Tags = []string{}
	}
	if e.Entities == nil {
		e.Entities = []string{}
	}
	var exam []byte
	var rel [4]int
	if e.Exam != nil {
		raw, err := json.Marshal(e.Exam)
		if err != nil {
			return fmt.Errorf("store: encode exam lens: %w", err)
		}
		exam = raw
		copy(rel[:], e.Exam.Rel())
	}
	if _, err := s.pool.Exec(ctx, `
		UPDATE pages SET why = $2, tags = $3, importance = $4, long_summary = $5, enriched_at = now(),
		                 rel_prelims = $6, rel_mains = $7, rel_essay = $8, rel_ethics = $9, exam = $10, entities = $11
		WHERE url_hash = $1`, urlHash, e.Why, e.Tags, e.Importance, e.Long, rel[0], rel[1], rel[2], rel[3], exam, e.Entities); err != nil {
		return fmt.Errorf("store: set enrichment: %w", err)
	}

	return nil
}

func (s *Postgres) PendingDetail(ctx context.Context, since time.Time, limit int) ([]Unenriched, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT url_hash, url, title, description, content, `+categorySQL+`
		FROM pages
		WHERE is_article AND enriched_at IS NOT NULL AND detailed_at IS NULL
		  AND COALESCE(published_at, first_seen_at) >= $1
		ORDER BY (category = 'editorial') DESC, importance DESC, COALESCE(published_at, first_seen_at) DESC, url
		LIMIT $2`, since, limit)
	if err != nil {
		return nil, fmt.Errorf("store: pending detail: %w", err)
	}
	defer rows.Close()

	var out []Unenriched
	for rows.Next() {
		var u Unenriched
		if err := rows.Scan(&u.URLHash, &u.URL, &u.Title, &u.Description, &u.Content, &u.Category); err != nil {
			return nil, fmt.Errorf("store: pending detail scan: %w", err)
		}
		out = append(out, u)
	}

	return out, rows.Err()
}

func (s *Postgres) SetDetail(ctx context.Context, urlHash string, d Detail) error {
	var exam []byte
	var rel [4]int
	if d.Exam != nil {
		raw, err := json.Marshal(d.Exam)
		if err != nil {
			return fmt.Errorf("store: encode exam lens: %w", err)
		}
		exam = raw
		copy(rel[:], d.Exam.Rel())
	}
	if _, err := s.pool.Exec(ctx, `
		UPDATE pages SET long_summary = $2, exam = $3, rel_prelims = $4, rel_mains = $5, rel_essay = $6, rel_ethics = $7,
		                 detailed_at = now()
		WHERE url_hash = $1`, urlHash, d.Long, exam, rel[0], rel[1], rel[2], rel[3]); err != nil {
		return fmt.Errorf("store: set detail: %w", err)
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
func (s *Postgres) Subscribe(ctx context.Context, email, token string) (string, bool, error) {
	// One statement, so two signups for the same address cannot race: a new
	// address is inserted; an existing one keeps its token, unless it had
	// unsubscribed, in which case it starts over unconfirmed with the new token.
	var stored string
	var confirmed bool
	err := s.pool.QueryRow(ctx, `
		INSERT INTO subscribers (email, token) VALUES ($1, $2)
		ON CONFLICT (email) DO UPDATE SET
		    token = CASE WHEN subscribers.unsubscribed_at IS NOT NULL THEN EXCLUDED.token ELSE subscribers.token END,
		    confirmed_at = CASE WHEN subscribers.unsubscribed_at IS NOT NULL THEN NULL ELSE subscribers.confirmed_at END,
		    unsubscribed_at = NULL
		RETURNING token, confirmed_at IS NOT NULL`, email, token).Scan(&stored, &confirmed)
	if err != nil {
		return "", false, fmt.Errorf("store: subscribe: %w", err)
	}

	return stored, confirmed, nil
}

func (s *Postgres) Confirm(ctx context.Context, token string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE subscribers SET confirmed_at = COALESCE(confirmed_at, now())
		WHERE token = $1 AND unsubscribed_at IS NULL`, token)
	if err != nil {
		return false, fmt.Errorf("store: confirm: %w", err)
	}

	return tag.RowsAffected() > 0, nil
}

func (s *Postgres) Unsubscribe(ctx context.Context, token string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE subscribers SET unsubscribed_at = COALESCE(unsubscribed_at, now())
		WHERE token = $1`, token)
	if err != nil {
		return false, fmt.Errorf("store: unsubscribe: %w", err)
	}

	return tag.RowsAffected() > 0, nil
}

func (s *Postgres) SetPreferences(ctx context.Context, token string, p Preferences) (bool, error) {
	tracks, mode := track.Normalize(p.Tracks, p.Mode)
	tag, err := s.pool.Exec(ctx, `
		UPDATE subscribers SET tracks = $2, mode = $3, topics = $4, source = CASE WHEN $5 <> '' THEN $5 ELSE source END
		WHERE token = $1`, token, tracks, mode, track.NormalizeTopics(p.Topics), p.Source)
	if err != nil {
		return false, fmt.Errorf("store: set preferences: %w", err)
	}

	return tag.RowsAffected() > 0, nil
}

func (s *Postgres) Subscription(ctx context.Context, token string) (Subscription, bool, error) {
	var sub Subscription
	var confirmed, unsubscribed bool
	err := s.pool.QueryRow(ctx, `
		SELECT email, tracks, mode, topics, confirmed_at IS NOT NULL, unsubscribed_at IS NOT NULL
		FROM subscribers WHERE token = $1`, token).Scan(&sub.Email, &sub.Tracks, &sub.Mode, &sub.Topics, &confirmed, &unsubscribed)
	if errors.Is(err, pgx.ErrNoRows) {
		return Subscription{}, false, nil
	}
	if err != nil {
		return Subscription{}, false, fmt.Errorf("store: subscription: %w", err)
	}
	sub.Status = subscriptionStatus(confirmed, unsubscribed)

	return sub, true, nil
}

func (s *Postgres) AudienceStats(ctx context.Context, days int, tz string) (Audience, error) {
	days, loc, err := audienceWindow(days, tz)
	if err != nil {
		return Audience{}, err
	}
	now := time.Now()
	since := now.In(loc).AddDate(0, 0, -(days - 1))
	since = time.Date(since.Year(), since.Month(), since.Day(), 0, 0, 0, 0, loc)
	sinceDay := since.Format("2006-01-02")

	a := Audience{Modes: map[string]int{}}
	if err := s.pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE confirmed_at IS NULL AND unsubscribed_at IS NULL),
		       count(*) FILTER (WHERE confirmed_at IS NOT NULL AND unsubscribed_at IS NULL),
		       count(*) FILTER (WHERE unsubscribed_at IS NOT NULL)
		FROM subscribers`).Scan(&a.Pending, &a.Active, &a.Unsubscribed); err != nil {
		return Audience{}, fmt.Errorf("store: audience totals: %w", err)
	}

	counts := map[string]*AudienceDay{}
	day := func(d string) *AudienceDay {
		if counts[d] == nil {
			counts[d] = &AudienceDay{}
		}
		return counts[d]
	}
	for _, q := range []struct {
		sql string
		add func(d *AudienceDay, n int)
	}{
		{`SELECT to_char(created_at AT TIME ZONE $1, 'YYYY-MM-DD'), count(*) FROM subscribers WHERE created_at >= $2 GROUP BY 1`, func(d *AudienceDay, n int) { d.Signups = n }},
		{`SELECT to_char(confirmed_at AT TIME ZONE $1, 'YYYY-MM-DD'), count(*) FROM subscribers WHERE confirmed_at >= $2 GROUP BY 1`, func(d *AudienceDay, n int) { d.Confirmed = n }},
		{`SELECT to_char(unsubscribed_at AT TIME ZONE $1, 'YYYY-MM-DD'), count(*) FROM subscribers WHERE unsubscribed_at >= $2 GROUP BY 1`, func(d *AudienceDay, n int) { d.Unsubscribed = n }},
	} {
		rows, err := s.pool.Query(ctx, q.sql, tz, since)
		if err != nil {
			return Audience{}, fmt.Errorf("store: audience days: %w", err)
		}
		for rows.Next() {
			var d string
			var n int
			if err := rows.Scan(&d, &n); err != nil {
				rows.Close()
				return Audience{}, fmt.Errorf("store: audience days scan: %w", err)
			}
			q.add(day(d), n)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return Audience{}, fmt.Errorf("store: audience days: %w", err)
		}
	}
	rows, err := s.pool.Query(ctx, `SELECT to_char(edition_day, 'YYYY-MM-DD'), count(*) FROM deliveries WHERE edition_day >= $1::date GROUP BY 1`, sinceDay)
	if err != nil {
		return Audience{}, fmt.Errorf("store: audience sent: %w", err)
	}
	for rows.Next() {
		var d string
		var n int
		if err := rows.Scan(&d, &n); err != nil {
			rows.Close()
			return Audience{}, fmt.Errorf("store: audience sent scan: %w", err)
		}
		day(d).Sent = n
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Audience{}, fmt.Errorf("store: audience sent: %w", err)
	}
	a.Daily = fillDays(now, days, loc, counts)

	rows, err = s.pool.Query(ctx, `SELECT mode, count(*) FROM subscribers WHERE confirmed_at IS NOT NULL AND unsubscribed_at IS NULL GROUP BY mode`)
	if err != nil {
		return Audience{}, fmt.Errorf("store: audience modes: %w", err)
	}
	for rows.Next() {
		var m string
		var n int
		if err := rows.Scan(&m, &n); err != nil {
			rows.Close()
			return Audience{}, fmt.Errorf("store: audience modes scan: %w", err)
		}
		a.Modes[m] = n
	}
	rows.Close()

	rows, err = s.pool.Query(ctx, `
		SELECT t, count(*) FROM subscribers, unnest(topics) AS t
		WHERE confirmed_at IS NOT NULL AND unsubscribed_at IS NULL
		GROUP BY t ORDER BY 2 DESC, t LIMIT 10`)
	if err != nil {
		return Audience{}, fmt.Errorf("store: audience topics: %w", err)
	}
	for rows.Next() {
		var t TopicCount
		if err := rows.Scan(&t.Topic, &t.Count); err != nil {
			rows.Close()
			return Audience{}, fmt.Errorf("store: audience topics scan: %w", err)
		}
		a.Topics = append(a.Topics, t)
	}
	rows.Close()

	rows, err = s.pool.Query(ctx, `SELECT source, count(*), count(confirmed_at) FROM subscribers GROUP BY source ORDER BY 2 DESC, 1`)
	if err != nil {
		return Audience{}, fmt.Errorf("store: audience sources: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var src AudienceSource
		if err := rows.Scan(&src.Source, &src.Signups, &src.Confirmed); err != nil {
			return Audience{}, fmt.Errorf("store: audience sources scan: %w", err)
		}
		a.Sources = append(a.Sources, src)
	}

	return a, rows.Err()
}

func (s *Postgres) SaveAdminToken(ctx context.Context, t AdminToken) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM admin_tokens WHERE expires_at < now()`); err != nil {
		return fmt.Errorf("store: prune admin tokens: %w", err)
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO admin_tokens (token_hash, kind, email, expires_at) VALUES ($1, $2, $3, $4)`, t.Hash, t.Kind, t.Email, t.Expires); err != nil {
		return fmt.Errorf("store: save admin token: %w", err)
	}

	return nil
}

func (s *Postgres) UseAdminToken(ctx context.Context, hash, kind string, consume bool, now time.Time) (string, bool, error) {
	query := `SELECT email FROM admin_tokens WHERE token_hash = $1 AND kind = $2 AND expires_at > $3`
	if consume {
		query = `DELETE FROM admin_tokens WHERE token_hash = $1 AND kind = $2 AND expires_at > $3 RETURNING email`
	}
	var email string
	err := s.pool.QueryRow(ctx, query, hash, kind, now).Scan(&email)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("store: use admin token: %w", err)
	}

	return email, true, nil
}

func (s *Postgres) DeleteAdminToken(ctx context.Context, hash string) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM admin_tokens WHERE token_hash = $1`, hash); err != nil {
		return fmt.Errorf("store: delete admin token: %w", err)
	}

	return nil
}

func (s *Postgres) ConfirmedSubscribers(ctx context.Context) ([]Subscriber, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, email, token, tracks, mode, topics FROM subscribers
		WHERE confirmed_at IS NOT NULL AND unsubscribed_at IS NULL
		ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("store: confirmed subscribers: %w", err)
	}
	defer rows.Close()

	var out []Subscriber
	for rows.Next() {
		var sub Subscriber
		if err := rows.Scan(&sub.ID, &sub.Email, &sub.Token, &sub.Tracks, &sub.Mode, &sub.Topics); err != nil {
			return nil, fmt.Errorf("store: confirmed subscribers scan: %w", err)
		}
		out = append(out, sub)
	}

	return out, rows.Err()
}

func (s *Postgres) DeliveredOn(ctx context.Context, day string) (map[int64]bool, error) {
	rows, err := s.pool.Query(ctx, `SELECT subscriber_id FROM deliveries WHERE edition_day = $1::date`, day)
	if err != nil {
		return nil, fmt.Errorf("store: delivered on: %w", err)
	}
	defer rows.Close()

	out := make(map[int64]bool)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("store: delivered on scan: %w", err)
		}
		out[id] = true
	}

	return out, rows.Err()
}

func (s *Postgres) MarkDelivered(ctx context.Context, subscriberID int64, day string) error {
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO deliveries (subscriber_id, edition_day) VALUES ($1, $2::date)
		ON CONFLICT DO NOTHING`, subscriberID, day); err != nil {
		return fmt.Errorf("store: mark delivered: %w", err)
	}

	return nil
}

func (s *Postgres) ChannelPosted(ctx context.Context, day, channel string) (bool, error) {
	var ok bool
	if err := s.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM channel_posts WHERE edition_day = $1::date AND channel = $2)`, day, channel).Scan(&ok); err != nil {
		return false, fmt.Errorf("store: channel posted: %w", err)
	}

	return ok, nil
}

func (s *Postgres) MarkChannelPosted(ctx context.Context, day, channel string) error {
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO channel_posts (edition_day, channel) VALUES ($1::date, $2) ON CONFLICT DO NOTHING`, day, channel); err != nil {
		return fmt.Errorf("store: mark channel posted: %w", err)
	}

	return nil
}

func (s *Postgres) GetQuiz(ctx context.Context, day, track string) (Quiz, bool, error) {
	var raw []byte
	q := Quiz{Track: track}
	var d time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT edition_day, questions FROM quizzes
		WHERE track = $1 AND ($2 = '' OR edition_day = $2::date)
		ORDER BY edition_day DESC LIMIT 1`, track, day).Scan(&d, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return Quiz{}, false, nil
	}
	if err != nil {
		return Quiz{}, false, fmt.Errorf("store: get quiz: %w", err)
	}
	q.Day = d.Format("2006-01-02")
	if err := json.Unmarshal(raw, &q.Questions); err != nil {
		return Quiz{}, false, fmt.Errorf("store: decode quiz: %w", err)
	}

	return q, true, nil
}

func (s *Postgres) SaveQuiz(ctx context.Context, q Quiz) error {
	raw, err := json.Marshal(q.Questions)
	if err != nil {
		return fmt.Errorf("store: encode quiz: %w", err)
	}
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO quizzes (edition_day, track, questions) VALUES ($1::date, $2, $3)
		ON CONFLICT (edition_day, track) DO UPDATE SET questions = EXCLUDED.questions, created_at = now()`,
		q.Day, q.Track, raw); err != nil {
		return fmt.Errorf("store: save quiz: %w", err)
	}

	return nil
}

func (s *Postgres) GetIntro(ctx context.Context, day, track string) (Intro, bool, error) {
	in := Intro{Track: track}
	var d time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT edition_day, body FROM intros
		WHERE track = $1 AND ($2 = '' OR edition_day = $2::date)
		ORDER BY edition_day DESC LIMIT 1`, track, day).Scan(&d, &in.Text)
	if errors.Is(err, pgx.ErrNoRows) {
		return Intro{}, false, nil
	}
	if err != nil {
		return Intro{}, false, fmt.Errorf("store: get intro: %w", err)
	}
	in.Day = d.Format("2006-01-02")

	return in, true, nil
}

func (s *Postgres) SaveIntro(ctx context.Context, in Intro) error {
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO intros (edition_day, track, body) VALUES ($1::date, $2, $3)
		ON CONFLICT (edition_day, track) DO UPDATE SET body = EXCLUDED.body, created_at = now()`,
		in.Day, in.Track, in.Text); err != nil {
		return fmt.Errorf("store: save intro: %w", err)
	}

	return nil
}

func (s *Postgres) ProExtend(ctx context.Context, paymentID, email string, days int, token string) (ProMember, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ProMember{}, false, fmt.Errorf("store: pro extend: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after Commit

	var m ProMember
	tag, err := tx.Exec(ctx, `INSERT INTO pro_payments (payment_id, email) VALUES ($1, $2) ON CONFLICT DO NOTHING`, paymentID, email)
	if err != nil {
		return ProMember{}, false, fmt.Errorf("store: record payment: %w", err)
	}
	fresh := tag.RowsAffected() > 0
	if fresh {
		err = tx.QueryRow(ctx, `
			INSERT INTO pro_members (email, token, until) VALUES ($1, $2, now() + make_interval(days => $3))
			ON CONFLICT (email) DO UPDATE SET until = GREATEST(pro_members.until, now()) + make_interval(days => $3)
			RETURNING email, token, until`, email, token, days).Scan(&m.Email, &m.Token, &m.Until)
	} else {
		err = tx.QueryRow(ctx, `SELECT email, token, until FROM pro_members WHERE email = $1`, email).Scan(&m.Email, &m.Token, &m.Until)
	}
	if err != nil {
		return ProMember{}, false, fmt.Errorf("store: pro member: %w", err)
	}

	return m, fresh, tx.Commit(ctx)
}

func (s *Postgres) ProByToken(ctx context.Context, token string) (ProMember, bool, error) {
	var m ProMember
	err := s.pool.QueryRow(ctx, `SELECT email, token, until FROM pro_members WHERE token = $1`, token).Scan(&m.Email, &m.Token, &m.Until)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProMember{}, false, nil
	}
	if err != nil {
		return ProMember{}, false, fmt.Errorf("store: pro by token: %w", err)
	}

	return m, true, nil
}

func (s *Postgres) ActiveProTokens(ctx context.Context) (map[string]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT lower(email), token FROM pro_members WHERE until > now()`)
	if err != nil {
		return nil, fmt.Errorf("store: active pro: %w", err)
	}
	defer rows.Close()

	out := map[string]string{}
	for rows.Next() {
		var email, token string
		if err := rows.Scan(&email, &token); err != nil {
			return nil, fmt.Errorf("store: active pro scan: %w", err)
		}
		out[email] = token
	}

	return out, rows.Err()
}

func (s *Postgres) Cards(ctx context.Context, since time.Time, limit int) ([]StoryCards, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT url_hash, title, importance, exam
		FROM pages
		WHERE is_article AND exam IS NOT NULL
		  AND (jsonb_array_length(COALESCE(exam->'cards', '[]'::jsonb)) > 0 OR jsonb_array_length(COALESCE(exam->'facts', '[]'::jsonb)) > 0)
		  AND COALESCE(published_at, first_seen_at) >= $1
		  AND COALESCE(published_at, first_seen_at) <= now() + interval '1 day'
		ORDER BY importance DESC, COALESCE(published_at, first_seen_at) DESC, url
		LIMIT $2`, since, limit)
	if err != nil {
		return nil, fmt.Errorf("store: cards: %w", err)
	}
	defer rows.Close()

	var out []StoryCards
	for rows.Next() {
		var sc StoryCards
		var raw []byte
		if err := rows.Scan(&sc.ID, &sc.Title, &sc.Importance, &raw); err != nil {
			return nil, fmt.Errorf("store: cards scan: %w", err)
		}
		var lens ExamLens
		if err := json.Unmarshal(raw, &lens); err != nil {
			return nil, fmt.Errorf("store: decode exam lens: %w", err)
		}
		sc.Cards, sc.Facts = lens.Cards, lens.Facts
		out = append(out, sc)
	}

	return out, rows.Err()
}

func (s *Postgres) GetCompilation(ctx context.Context, month, track string) (Compilation, bool, error) {
	c := Compilation{Month: month, Track: track}
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT areas, built_at FROM compilations WHERE month = $1 AND track = $2`, month, track).Scan(&raw, &c.Built)
	if errors.Is(err, pgx.ErrNoRows) {
		return Compilation{}, false, nil
	}
	if err != nil {
		return Compilation{}, false, fmt.Errorf("store: get compilation: %w", err)
	}
	if err := json.Unmarshal(raw, &c.Areas); err != nil {
		return Compilation{}, false, fmt.Errorf("store: decode compilation: %w", err)
	}

	return c, true, nil
}

func (s *Postgres) SaveCompilation(ctx context.Context, c Compilation) error {
	raw, err := json.Marshal(c.Areas)
	if err != nil {
		return fmt.Errorf("store: encode compilation: %w", err)
	}
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO compilations (month, track, areas) VALUES ($1, $2, $3)
		ON CONFLICT (month, track) DO UPDATE SET areas = EXCLUDED.areas, built_at = now()`, c.Month, c.Track, raw); err != nil {
		return fmt.Errorf("store: save compilation: %w", err)
	}

	return nil
}

func (s *Postgres) CompilationMonths(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT month FROM compilations ORDER BY month DESC`)
	if err != nil {
		return nil, fmt.Errorf("store: compilation months: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var m string
		if err := rows.Scan(&m); err != nil {
			return nil, fmt.Errorf("store: compilation months scan: %w", err)
		}
		out = append(out, m)
	}

	return out, rows.Err()
}

func (s *Postgres) AddConceptTerms(ctx context.Context, terms []string) error {
	for _, t := range terms {
		if _, err := s.pool.Exec(ctx, `INSERT INTO concepts (term_key, term) VALUES (lower($1), $1) ON CONFLICT DO NOTHING`, t); err != nil {
			return fmt.Errorf("store: add concept term: %w", err)
		}
	}

	return nil
}

func (s *Postgres) PendingConcepts(ctx context.Context, limit int) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT term FROM concepts WHERE body = '' ORDER BY created_at, term_key LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("store: pending concepts: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, fmt.Errorf("store: pending concepts scan: %w", err)
		}
		out = append(out, t)
	}

	return out, rows.Err()
}

func (s *Postgres) SetConcept(ctx context.Context, term, body string) error {
	if _, err := s.pool.Exec(ctx, `UPDATE concepts SET body = $2 WHERE term_key = lower($1)`, term, body); err != nil {
		return fmt.Errorf("store: set concept: %w", err)
	}

	return nil
}

func (s *Postgres) Concepts(ctx context.Context, limit int) ([]Concept, error) {
	rows, err := s.pool.Query(ctx, `SELECT term, body FROM concepts WHERE body <> '' ORDER BY term_key LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("store: concepts: %w", err)
	}
	defer rows.Close()

	out := []Concept{}
	for rows.Next() {
		var c Concept
		if err := rows.Scan(&c.Term, &c.Body); err != nil {
			return nil, fmt.Errorf("store: concepts scan: %w", err)
		}
		out = append(out, c)
	}

	return out, rows.Err()
}

func (s *Postgres) EntityStories(ctx context.Context, since time.Time) ([]EntityStory, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT url_hash, title, `+hostSQL+`, why, COALESCE(published_at, first_seen_at), entities
		FROM pages
		WHERE is_article AND entities <> '{}' AND COALESCE(published_at, first_seen_at) >= $1
		ORDER BY COALESCE(published_at, first_seen_at) DESC, url
		LIMIT 10000`, since)
	if err != nil {
		return nil, fmt.Errorf("store: entity stories: %w", err)
	}
	defer rows.Close()

	var out []EntityStory
	for rows.Next() {
		var e EntityStory
		if err := rows.Scan(&e.ID, &e.Title, &e.Source, &e.Why, &e.Time, &e.Entities); err != nil {
			return nil, fmt.Errorf("store: entity stories scan: %w", err)
		}
		out = append(out, e)
	}

	return out, rows.Err()
}

func (s *Postgres) AddTeamHook(ctx context.Context, kind, url, track, token string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `INSERT INTO team_hooks (kind, url, track, token) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`, kind, url, track, token)
	if err != nil {
		return false, fmt.Errorf("store: add team hook: %w", err)
	}

	return tag.RowsAffected() > 0, nil
}

func (s *Postgres) RemoveTeamHook(ctx context.Context, token string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM team_hooks WHERE token = $1`, token)
	if err != nil {
		return false, fmt.Errorf("store: remove team hook: %w", err)
	}

	return tag.RowsAffected() > 0, nil
}

func (s *Postgres) TeamHooks(ctx context.Context) ([]TeamHook, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, kind, url, track, token FROM team_hooks ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("store: team hooks: %w", err)
	}
	defer rows.Close()

	var out []TeamHook
	for rows.Next() {
		var h TeamHook
		if err := rows.Scan(&h.ID, &h.Kind, &h.URL, &h.Track, &h.Token); err != nil {
			return nil, fmt.Errorf("store: team hooks scan: %w", err)
		}
		out = append(out, h)
	}

	return out, rows.Err()
}

func (s *Postgres) Close() error {
	s.once.Do(func() { close(s.quit) })
	s.wg.Wait()
	return nil
}

var _ Store = (*Postgres)(nil)
