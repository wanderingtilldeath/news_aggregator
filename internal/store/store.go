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
	// Why, Tags and Importance come from the enrichment pass: a one-line "why
	// it matters", topic tags, and a score from 1 (minor) to 5 (major); all are
	// empty or 0 until the pass has run. Long is the 7-minute summary, set only
	// when DigestQuery.Detail asks for it.
	Why        string   `json:"why,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	Importance int      `json:"importance,omitempty"`
	Long       string   `json:"long,omitempty"`
	// Rel is the exam relevance of a UPSC story: Prelims, Mains, Essay, Ethics, each 0 to 3
	// (nil when the story has none). Exam is the full lens and is set only by Store.Item.
	Rel  []int     `json:"rel,omitempty"`
	Exam *ExamLens `json:"exam,omitempty"`
	// SourceType is the kind of publisher (see package sourcetype); the API fills it in.
	SourceType string `json:"source_type,omitempty"`
	// Kind is "opinion", "analysis" or "explainer" (see package kind); the API fills it in.
	Kind string `json:"kind,omitempty"`
	// Coverage is how many sources cover the same event (0 when this story stands alone). Also
	// lists the other stories of that event, and is set only by Store.Item.
	Coverage int       `json:"coverage,omitempty"`
	Also     []Related `json:"also,omitempty"`
}

// Related is another source's story about the same event.
type Related struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Source string `json:"source"`
}

// Candidate is what story grouping compares: an article's headline, source and date.
type Candidate struct {
	ID     string
	Source string
	Title  string
	Time   time.Time
}

// StoryGroup places an article in a story: ID names the group, Sources counts its distinct hosts.
type StoryGroup struct {
	ID      string
	Sources int
}

// ExamLens is what a UPSC aspirant can take from a story, written by a model from the article
// text alone: relevance scores (0 to 3), the dimensions it touches, arguments for and against,
// testable facts for Prelims and a short Mains answer skeleton.
type ExamLens struct {
	Prelims    int      `json:"prelims"`
	Mains      int      `json:"mains"`
	Essay      int      `json:"essay"`
	Ethics     int      `json:"ethics"`
	Dimensions []string `json:"dimensions,omitempty"`
	For        []string `json:"for,omitempty"`
	Against    []string `json:"against,omitempty"`
	Facts      []string `json:"facts,omitempty"`
	Skeleton   []string `json:"skeleton,omitempty"`
}

// Rel returns the four scores in the order Prelims, Mains, Essay, Ethics.
func (e *ExamLens) Rel() []int {
	if e == nil {
		return nil
	}

	return []int{e.Prelims, e.Mains, e.Essay, e.Ethics}
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
	// Until, when set, keeps only items dated before it (with Since: one day's edition).
	Until time.Time
	// Detail includes the long summary of each item.
	Detail bool
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

// Unenriched is a summarised article waiting for the enrichment pass.
type Unenriched struct {
	URLHash     string
	URL         string
	Title       string
	Description string
	Content     string
	Category    string
}

// Enrichment is what the enrichment pass learns about one article.
type Enrichment struct {
	Why        string
	Tags       []string
	Importance int
	Long       string
	// Exam is the UPSC lens; nil for stories outside the UPSC categories.
	Exam *ExamLens
}

// Detail is what the second enrichment stage writes for a top story: the long summary and, for
// UPSC categories, the exam lens.
type Detail struct {
	Long string
	Exam *ExamLens
}

// QuizQuestion is one multiple-choice question: Answer is the index of the correct
// option, and URL is the article it was written from.
type QuizQuestion struct {
	Question    string   `json:"question"`
	Options     []string `json:"options"`
	Answer      int      `json:"answer"`
	Explanation string   `json:"explanation"`
	URL         string   `json:"url,omitempty"`
}

// Intro is the short "Today in 60 seconds" briefing of one track for an edition day.
type Intro struct {
	Day   string `json:"day"`
	Track string `json:"track"`
	Text  string `json:"text"`
}

// Quiz is the day's quiz for one track.
type Quiz struct {
	Day       string         `json:"day"`
	Track     string         `json:"track"`
	Questions []QuizQuestion `json:"questions"`
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
	// Item returns one article by its id (the page's URL hash), the same way Digest
	// shows it but without the long summary. ok is false for an unknown id or a page
	// that is not an article.
	Item(ctx context.Context, id string) (it Item, ok bool, err error)
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
	// PendingEnrichment returns summarised articles dated at or after since that
	// the enrichment pass has not seen, newest first.
	PendingEnrichment(ctx context.Context, since time.Time, limit int) ([]Unenriched, error)
	// PendingDetail returns enriched articles dated at or after since that have no long summary or
	// exam lens yet, most important first (newest first among equals).
	PendingDetail(ctx context.Context, since time.Time, limit int) ([]Unenriched, error)
	// SetDetail stores an article's long summary and exam lens and marks it done.
	SetDetail(ctx context.Context, urlHash string, d Detail) error
	// GroupCandidates returns the articles dated at or after since, newest first, for story grouping.
	GroupCandidates(ctx context.Context, since time.Time, limit int) ([]Candidate, error)
	// SetStories replaces the story grouping of the articles dated at or after since: those in groups
	// get their group, every other one in the window is ungrouped.
	SetStories(ctx context.Context, since time.Time, groups map[string]StoryGroup) error
	// SetEnrichment stores an article's enrichment and marks it done.
	SetEnrichment(ctx context.Context, urlHash string, e Enrichment) error
	// Validators returns the validators from the last stored fetch of a URL.
	Validators(ctx context.Context, urlHash string) (Validators, bool, error)
	// Search returns one page of the pages matching q, best first (limit rows
	// after skipping offset), plus the total number of matches.
	Search(ctx context.Context, q string, limit, offset int) ([]Hit, int, error)
	// Subscribe records an email address (already lowercased and validated) as
	// an unconfirmed subscriber with the given token, and returns the token
	// the confirmation link must carry plus whether the address is already
	// confirmed. An existing subscriber keeps their token; one who had
	// unsubscribed gets the new token and must confirm again.
	Subscribe(ctx context.Context, email, token string) (string, bool, error)
	// Confirm marks the subscriber with this token confirmed. It reports false
	// for an unknown or unsubscribed token, and is safe to repeat.
	Confirm(ctx context.Context, token string) (bool, error)
	// Unsubscribe marks the subscriber with this token unsubscribed. It reports
	// false for an unknown token, and is safe to repeat.
	Unsubscribe(ctx context.Context, token string) (bool, error)
	// ChannelPosted reports whether the edition of day was already posted to the
	// channel (for example "telegram:@postpaper_upsc").
	ChannelPosted(ctx context.Context, day, channel string) (bool, error)
	// MarkChannelPosted records a post; it is safe to repeat.
	MarkChannelPosted(ctx context.Context, day, channel string) error
	// GetQuiz returns the quiz of a track for an edition day ("2006-01-02"); an
	// empty day means the latest one. ok is false when there is none.
	GetQuiz(ctx context.Context, day, track string) (q Quiz, ok bool, err error)
	// SaveQuiz stores a quiz, replacing one for the same day and track.
	SaveQuiz(ctx context.Context, q Quiz) error
	// GetIntro returns the intro of a track for an edition day ("2006-01-02"); an empty day means
	// the latest one. ok is false when there is none.
	GetIntro(ctx context.Context, day, track string) (in Intro, ok bool, err error)
	// SaveIntro stores an intro, replacing one for the same day and track.
	SaveIntro(ctx context.Context, in Intro) error
	// SetPreferences stores the choices of the subscriber with this token. It
	// reports false for an unknown token.
	SetPreferences(ctx context.Context, token string, p Preferences) (bool, error)
	// ConfirmedSubscribers returns everyone who confirmed and has not
	// unsubscribed, oldest first.
	ConfirmedSubscribers(ctx context.Context) ([]Subscriber, error)
	// DeliveredOn returns the IDs of subscribers already mailed the edition of
	// day ("2006-01-02").
	DeliveredOn(ctx context.Context, day string) (map[int64]bool, error)
	// MarkDelivered records that a subscriber got the edition of day; it is
	// safe to repeat.
	MarkDelivered(ctx context.Context, subscriberID int64, day string) error
	// Close flushes and releases resources.
	Close() error
}

// Subscriber is a confirmed mail recipient. Token is also their unsubscribe key.
// Tracks and Mode are what they chose at signup (see package track).
type Subscriber struct {
	ID     int64
	Email  string
	Token  string
	Tracks []string
	Mode   string
	// Topics are words they follow (see track.NormalizeTopics).
	Topics []string
}

// Preferences are a subscriber's choices; empty values mean every track and the
// one-minute read, and no topics.
type Preferences struct {
	Tracks []string
	Mode   string
	Topics []string
}

// ContentHash is the dedupe fingerprint of a page body.
func ContentHash(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
