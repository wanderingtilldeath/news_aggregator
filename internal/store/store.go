// Package store persists crawled pages and serves full-text search over them.
package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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
	// Words is the article length in words, counted before Content is capped; 0 when unknown.
	Words int
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
	// Entities are named by the enrichment pass; set only by Store.Item.
	Entities []string `json:"entities,omitempty"`
	// Words is the article length in words (0 when unknown); the page turns it into a reading time.
	Words int `json:"words,omitempty"`
	// Lead is the opening of the article in whole sentences (see package prose), set only by
	// Store.Item. The story page shows it when the gist bullets are cut off.
	Lead string `json:"lead,omitempty"`
}

// Lead sizes: the story page's opening is at most this many sentences and about this many words.
const (
	leadSentences = 3
	leadWords     = 70
)

// Related is another source's story about the same event.
type Related struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Source string `json:"source"`
	// SourceType is the kind of publisher (see package sourcetype); the API fills it in.
	SourceType string `json:"source_type,omitempty"`
	// Time is when the story was published, so the page can lay a thread out as a timeline.
	Time time.Time `json:"time"`
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
	// Cards are question and answer flashcards for Prelims revision.
	Cards []Card `json:"cards,omitempty"`
}

// Card is one flashcard: a short question and its answer.
type Card struct {
	Q string `json:"q"`
	A string `json:"a"`
}

// StoryCards is what the flashcard deck is made from: a story with its cards, or, for a story
// written before cards existed, its Prelims facts.
type StoryCards struct {
	ID         string
	Title      string
	Importance int
	Cards      []Card
	Facts      []string
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
	Why  string
	Tags []string
	// Entities are the people, organisations, places and laws the story is about (at most four).
	Entities   []string
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
	// Concepts are the terms the story depends on ("repo rate"); the caller queues them for an
	// explainer, they are not stored with the story.
	Concepts []string
}

// ImageCoverage is how many articles show a picture (Shown), fall back to a tile because every
// candidate image is a shared logo (Logo), or have no image at all (None).
type ImageCoverage struct {
	Shown, Logo, None int
}

// Concept is a term and its plain-language explainer.
type Concept struct {
	Term string `json:"term"`
	Body string `json:"body"`
}

// EntityStory is a story with the entities it names, for building the entity pages and the graph.
type EntityStory struct {
	ID       string
	Title    string
	Source   string
	Why      string
	Time     time.Time
	Entities []string
}

// TeamHook is a Slack or Discord channel that gets the daily edition. URL is its incoming webhook
// (a secret) and Token the key of the link that stops the posts.
type TeamHook struct {
	ID    int64
	Kind  string
	URL   string
	Track string
	Token string
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

// ProMember is a paying reader. Token is the secret in their links; Until is when the paid period
// ends (a member is active while Until is in the future).
type ProMember struct {
	Email string
	Token string
	Until time.Time
}

// ProNote is one saved sentence of a Pro member's notebook, with their own note and tags. At is when it was
// saved, in milliseconds since the epoch, as the browser counts it.
type ProNote struct {
	ID, Story, Title, Text, Note string
	Tags                         []string
	At                           int64
}

// CompItem is one story of a monthly compilation. It keeps what the printed page needs, so the
// compilation stays whole after the page itself has been deleted.
type CompItem struct {
	ID         string    `json:"id"`
	URL        string    `json:"url"`
	Title      string    `json:"title"`
	Source     string    `json:"source"`
	Why        string    `json:"why,omitempty"`
	Bullets    []string  `json:"bullets,omitempty"`
	Tags       []string  `json:"tags,omitempty"`
	Importance int       `json:"importance"`
	Time       time.Time `json:"time"`
}

// CompArea is one syllabus area of a compilation, its stories most important first.
type CompArea struct {
	Name  string     `json:"name"`
	Items []CompItem `json:"items"`
}

// Compilation is the month's most important stories of one edition, by area. Month is "2006-01".
type Compilation struct {
	Month string     `json:"month"`
	Track string     `json:"track"`
	Areas []CompArea `json:"areas"`
	Built time.Time  `json:"built"`
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
	// ImageStats counts the articles dated at or after since by what their card shows: a picture,
	// nothing because the only candidates are shared logos, or nothing because the page named none.
	ImageStats(ctx context.Context, since time.Time) (ImageCoverage, error)
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
	// exam lens yet: editorials first, then most important first (newest first among equals).
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
	// ProExtend records a payment and extends the member's period by days from the later of now and
	// the current end, creating the member (with token) when new. A payment id seen before changes
	// nothing and reports fresh false, so a repeated webhook cannot extend twice.
	ProExtend(ctx context.Context, paymentID, email string, days int, token string) (m ProMember, fresh bool, err error)
	// ProByToken returns the member with this token, active or not; ok is false for an unknown one.
	ProByToken(ctx context.Context, token string) (m ProMember, ok bool, err error)
	// ProNotes returns the member's notes, newest first.
	ProNotes(ctx context.Context, email string) ([]ProNote, error)
	// SaveProNote adds a note or replaces the one with the same id. It reports false, and changes nothing, when the
	// note is new and the member already has max notes.
	SaveProNote(ctx context.Context, email string, n ProNote, max int) (bool, error)
	// AddProNotes adds the notes the member does not have yet, leaving existing ones as they are, until the member
	// has max notes. It returns how many it added.
	AddProNotes(ctx context.Context, email string, notes []ProNote, max int) (int, error)
	// DeleteProNote removes one note and reports whether there was one; with an empty id it removes them all.
	DeleteProNote(ctx context.Context, email, id string) (int, error)
	// ActiveProTokens returns the token of every member whose period has not ended, by lowercased email.
	ActiveProTokens(ctx context.Context) (map[string]string, error)
	// Cards returns the stories dated at or after since that have an exam lens with cards or facts,
	// most important first (newest first among equals), at most limit of them.
	Cards(ctx context.Context, since time.Time, limit int) ([]StoryCards, error)
	// GetCompilation returns the compilation of a month ("2006-01") and track; ok is false when none exists.
	GetCompilation(ctx context.Context, month, track string) (c Compilation, ok bool, err error)
	// SaveCompilation stores a compilation, replacing one for the same month and track.
	SaveCompilation(ctx context.Context, c Compilation) error
	// CompilationMonths returns the months that have a compilation, newest first.
	CompilationMonths(ctx context.Context) ([]string, error)
	// AddConceptTerms queues terms that have no explainer yet; a term already known is left as it is.
	AddConceptTerms(ctx context.Context, terms []string) error
	// PendingConcepts returns up to limit queued terms that still need an explainer, oldest first.
	PendingConcepts(ctx context.Context, limit int) ([]string, error)
	// SetConcept stores the explainer of a queued term.
	SetConcept(ctx context.Context, term, body string) error
	// Concepts returns the terms that have an explainer, in alphabetical order, at most limit.
	Concepts(ctx context.Context, limit int) ([]Concept, error)
	// EntityStories returns the articles dated at or after since that name at least one entity, newest first.
	EntityStories(ctx context.Context, since time.Time) ([]EntityStory, error)
	// AddTeamHook registers a webhook; false means the address was already registered (nothing changes).
	AddTeamHook(ctx context.Context, kind, url, track, token string) (bool, error)
	// RemoveTeamHook deletes the webhook with this token and reports whether there was one.
	RemoveTeamHook(ctx context.Context, token string) (bool, error)
	// TeamHooks returns every registered webhook, oldest first.
	TeamHooks(ctx context.Context) ([]TeamHook, error)
	// SetPreferences stores the choices of the subscriber with this token. It
	// reports false for an unknown token.
	SetPreferences(ctx context.Context, token string, p Preferences) (bool, error)
	// ManageToken returns the token of the active subscriber with this address (confirmed, not unsubscribed); ok is
	// false for anyone else.
	ManageToken(ctx context.Context, email string) (token string, ok bool, err error)
	// Subscription returns the reader behind a token and what they chose; ok is false for an unknown token.
	Subscription(ctx context.Context, token string) (sub Subscription, ok bool, err error)
	// AudienceStats counts the subscribers and, for each of the last days days in the time zone tz (an IANA
	// name), the signups, confirmations, unsubscribes and mails sent.
	AudienceStats(ctx context.Context, days int, tz string) (Audience, error)
	// ContentHealth counts the articles of each of the last days days in the time zone tz, and the articles of the
	// last week still waiting for each enrichment stage.
	ContentHealth(ctx context.Context, days int, tz string) (Content, error)
	// Subscribers lists subscribers whose address contains q (all when empty), newest first, with the total
	// that match. It never returns a token.
	Subscribers(ctx context.Context, q string, limit, offset int) (rows []SubscriberRow, total int, err error)
	// RemoveSubscriber deletes the subscriber and their delivery records; it reports whether there was one.
	RemoveSubscriber(ctx context.Context, id int64) (bool, error)
	// SaveAdminToken stores an admin sign-in link or session (see AdminToken) and removes expired ones.
	SaveAdminToken(ctx context.Context, t AdminToken) error
	// UseAdminToken finds the unexpired token of this kind and returns its email. With consume the token is
	// deleted as it is found (a sign-in link works once).
	UseAdminToken(ctx context.Context, hash, kind string, consume bool, now time.Time) (email string, ok bool, err error)
	// DeleteAdminToken removes a token (signing out); an unknown one is not an error.
	DeleteAdminToken(ctx context.Context, hash string) error
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
	// Source is the campaign tag of the signup link; empty leaves the stored one as it is.
	Source string
}

// Content is what the admin dashboard shows about the news side: how many articles came in each day, and how
// many of the last week's are still waiting for each stage of the pipeline.
type Content struct {
	// Daily has one entry per day of the window, oldest first, zeros included.
	Daily []ContentDay
	// Waiting are the articles of the last 7 days not yet through each stage.
	Waiting ContentWaiting
	// Latest is when the newest article was first seen; zero when there is none.
	Latest time.Time
}

type ContentDay struct {
	Day      string
	Articles int
}

// ContentWaiting counts articles by the stage they wait for: a gist, a source image, enrichment (why it matters,
// importance, tags) and the long summary.
type ContentWaiting struct {
	Summary, Image, Enrichment, Detail int
}

// SubscriberRow is one line of the admin's subscriber list. It carries no token.
type SubscriberRow struct {
	ID        int64
	Email     string
	Status    string
	Mode      string
	Topics    []string
	Source    string
	Created   time.Time
	Confirmed time.Time
}

// AdminToken is a short-lived admin credential: a sign-in link (Kind "login", used once) or a signed-in session
// (Kind "session"). Hash is the hex SHA-256 of the secret; the secret itself is never stored.
type AdminToken struct {
	Hash, Kind, Email string
	Expires           time.Time
}

// Subscription is one reader's record, found by their token. Status is "pending" (not confirmed yet),
// "active" or "unsubscribed".
type Subscription struct {
	Email  string
	Status string
	Tracks []string
	Mode   string
	Topics []string
}

// Audience is what the analytics page shows: the readers now, and a count per day.
type Audience struct {
	Pending, Active, Unsubscribed int
	// Modes counts active readers by reading length ("1min", "7min").
	Modes map[string]int
	// Topics are the most followed words among active readers, most followed first.
	Topics []TopicCount
	// Daily has one entry per day of the window, oldest first, zeros included.
	Daily []AudienceDay
	// Sources counts every signup by campaign tag, busiest first; "" is a direct visit.
	Sources []AudienceSource
}

type TopicCount struct {
	Topic string
	Count int
}

// AudienceDay is one day ("2006-01-02") of signups, confirmations, unsubscribes and daily mails sent.
type AudienceDay struct {
	Day                                    string
	Signups, Confirmed, Unsubscribed, Sent int
}

// AudienceSource is the signups that carried one campaign tag and how many of them confirmed.
type AudienceSource struct {
	Source             string
	Signups, Confirmed int
}

// fillDays turns per-day counts into the window's days, oldest first, with zeros for quiet days. The
// days are counted back from now in loc.
func fillDays(now time.Time, days int, loc *time.Location, counts map[string]*AudienceDay) []AudienceDay {
	out := make([]AudienceDay, 0, days)
	for i := days - 1; i >= 0; i-- {
		day := now.In(loc).AddDate(0, 0, -i).Format("2006-01-02")
		d := AudienceDay{Day: day}
		if c := counts[day]; c != nil {
			d = *c
			d.Day = day
		}
		out = append(out, d)
	}

	return out
}

// audienceWindow checks the window and zone a caller asked for.
func audienceWindow(days int, tz string) (int, *time.Location, error) {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return 0, nil, fmt.Errorf("store: audience: %w", err)
	}

	return min(max(days, 1), 365), loc, nil
}

// ContentHash is the dedupe fingerprint of a page body.
func ContentHash(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func subscriptionStatus(confirmed, unsubscribed bool) string {
	switch {
	case unsubscribed:
		return "unsubscribed"
	case confirmed:
		return "active"
	default:
		return "pending"
	}
}
