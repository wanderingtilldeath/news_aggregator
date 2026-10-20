// Package store persists crawled pages and serves full-text search over them.
package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// Page is one stored document.
type Page struct {
	URLHash     string
	CrawlID     int64
	URL         string
	StatusCode  int
	ContentType string
	Title       string
	Description string
	H1          string
	Snippet     string
	// Published is the article date from page metadata; zero when unknown.
	Published time.Time
	// Image is the lead image URL, IsArticle separates articles from listing
	// pages, and Content is the capped main text the summariser reads.
	Image string
	// BodyImage is the first real picture inside the article; it stands in
	// when Image is missing or a logo shared by many articles.
	BodyImage string
	IsArticle bool
	Content   string
	// SourceURL is the original story the article cites; set only for crawls
	// that fetch their image from it, so the image pass can find it.
	SourceURL string
	// Category is the crawl's category (for example "tech"); "" means other.
	Category     string
	ContentHash  string
	ETag         string
	LastModified string
	SizeBytes    int
	Latency      time.Duration
}

// Hit is a search result.
type Hit struct {
	URL         string  `json:"url"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Snippet     string  `json:"snippet"`
	Rank        float64 `json:"rank"`
}

// Item is one entry of the news digest. Time is the article's published date,
// or when we first saw the page if it has none.
type Item struct {
	// ID identifies the page (its URL hash); the image proxy is keyed by it.
	ID          string `json:"id"`
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Snippet     string `json:"snippet"`
	// Source is the host the page came from, Image its lead image, and
	// Summary the bullet gist (empty until the summarise pass has run).
	Source    string    `json:"source"`
	Image     string    `json:"image"`
	Summary   []string  `json:"summary"`
	IsArticle bool      `json:"is_article"`
	Time      time.Time `json:"time"`
	// Category groups the item ("other" when its crawl set none).
	Category string `json:"category"`
}

// DigestQuery selects the digest. Zero values mean "no filter" (Limit 0 = 50).
type DigestQuery struct {
	Since time.Time
	// Query is a full-text filter.
	Query string
	// Source restricts to one host, e.g. "www.drishtiias.com".
	Source string
	// Category restricts to one category ("" = any).
	Category string
	// ArticlesOnly drops listing and index pages.
	ArticlesOnly bool
	// Limit is the page size (0 = 50, max 200) and Offset how many matches to
	// skip, for pagination.
	Limit  int
	Offset int
}

// SourceCount is how many digest items one host has.
type SourceCount struct {
	Source   string `json:"source"`
	Category string `json:"category"`
	Count    int    `json:"count"`
}

// ImageJob is an article whose lead image should come from its cited source.
type ImageJob struct {
	URLHash   string
	URL       string
	SourceURL string
}

// Unsummarized is an article waiting for its gist.
type Unsummarized struct {
	URLHash     string
	URL         string
	Title       string
	Description string
	Content     string
}

// Validators are the cache validators saved from the last fetch of a URL.
type Validators struct {
	ETag         string
	LastModified string
}

// Store saves pages and searches them.
type Store interface {
	// Save durably stores p; it returns after the write is committed, so a
	// caller can safely mark the URL done afterwards.
	Save(ctx context.Context, p Page) error
	// Digest returns one page of the pages dated at or after q.Since, newest
	// first, narrowed by the other DigestQuery fields, plus the total number of
	// matches across all pages.
	Digest(ctx context.Context, q DigestQuery) ([]Item, int, error)
	// DigestSources counts the digest items per host for the same window and
	// filters as q (ignoring q.Source, Limit and Offset), most items first.
	DigestSources(ctx context.Context, q DigestQuery) ([]SourceCount, error)
	// ImageURL returns the stored image URL of a page, "" when it has none.
	ImageURL(ctx context.Context, urlHash string) (string, error)
	// PendingSummaries returns articles dated (published, else first seen) at
	// or after since that have no summary yet, newest first. It matches the
	// digest window, so old pages that were merely crawled are never summarised.
	PendingSummaries(ctx context.Context, since time.Time, limit int) ([]Unsummarized, error)
	// SetSummary stores an article's gist and marks it summarised.
	SetSummary(ctx context.Context, urlHash string, bullets []string) error
	// PendingImages returns articles in the digest window whose source page has
	// not been looked up for an image yet, newest first.
	PendingImages(ctx context.Context, since time.Time, limit int) ([]ImageJob, error)
	// SetImage records the outcome of an article's image lookup. A non-empty
	// image replaces the page's own; an empty one only marks it looked up.
	SetImage(ctx context.Context, urlHash, image string) error
	// Validators returns the validators from the last stored fetch of a URL.
	Validators(ctx context.Context, urlHash string) (Validators, bool, error)
	// Search returns one page of the pages matching q, best first (limit rows
	// after skipping offset), plus the total number of matches.
	Search(ctx context.Context, q string, limit, offset int) ([]Hit, int, error)
	// Close flushes and releases resources.
	Close() error
}

// ContentHash is the dedupe fingerprint of a page body.
func ContentHash(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
