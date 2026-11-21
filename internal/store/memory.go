package store

import (
	"cmp"
	"context"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/prose"
	"github.com/wanderingtilldeath/news-aggregator/internal/track"
)

// Memory is an in-process Store for tests. Search is a naive substring
// match, not real full-text search.
type Memory struct {
	mu    sync.Mutex
	pages map[string]Page
	seen  map[string]time.Time // first Save per URL hash
	saves int
	// summaries holds SetSummary results; a key means "summarised".
	summaries map[string][]string
	// imageChecked marks articles whose source image was looked up.
	imageChecked map[string]bool
	// enrichments holds SetEnrichment results; a key means "enriched". detailed marks SetDetail.
	enrichments map[string]Enrichment
	detailed    map[string]bool
	// stories holds SetStories results by article id.
	stories map[string]StoryGroup
	// posted holds channel posts by day and channel.
	posted map[[2]string]bool
	// quizzes is keyed by day and track.
	quizzes map[[2]string]Quiz
	// pro holds members by email and payments by id.
	pro map[string]ProMember
	// proNotes holds each member's notes by email, then by note id.
	proNotes map[string]map[string]ProNote
	payments map[string]string
	// accounts are keyed by username; each has its recovery codes (hash -> spent) and sessions are keyed by hash.
	accounts    map[string]*memAccount
	accountSeq  int64
	acctSession map[string]memSession
	identities  map[string]int64
	// compilations is keyed by month and track.
	compilations map[[2]string]Compilation
	// events, concepts and team hooks.
	concepts map[string]*Concept // by lower-cased term
	conceptN []string            // queue order
	hooks    []TeamHook
	nextHook int64
	// intros is keyed by day and track.
	intros map[[2]string]Intro
	// subscribers is keyed by lowercased email; subSeq is the last id given (ids are never reused).
	subscribers map[string]*subscriber
	subSeq      int64
	// adminTokens holds admin sign-in links and sessions by hash.
	adminTokens map[string]AdminToken
	// delivered[day] is the set of subscriber IDs mailed that edition day.
	delivered map[string]map[int64]bool
}

// NewMemory returns an empty in-memory store.
func NewMemory() *Memory {
	return &Memory{
		pages:     make(map[string]Page),
		seen:      make(map[string]time.Time),
		summaries: make(map[string][]string),

		imageChecked: make(map[string]bool),
	}
}

func (m *Memory) Save(_ context.Context, p Page) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Like the Postgres upsert: a re-crawl keeps an image fetched from the source.
	if old, ok := m.pages[p.URLHash]; ok && m.imageChecked[p.URLHash] && old.SourceURL != "" && old.Image != "" {
		p.Image = old.Image
	}
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

func (m *Memory) Search(_ context.Context, q string, limit, offset int) ([]Hit, int, error) {
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

	total := len(hits)
	offset = max(offset, 0)
	if offset > len(hits) {
		offset = len(hits)
	}
	hits = hits[offset:]
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}

	return hits, total, nil
}

func (m *Memory) Digest(_ context.Context, q DigestQuery) ([]Item, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	items := m.digestLocked(q, q.Source)

	total := len(items)
	offset := max(q.Offset, 0)
	if offset > len(items) {
		offset = len(items)
	}
	items = items[offset:]
	if q.Limit > 0 && len(items) > q.Limit {
		items = items[:q.Limit]
	}

	return items, total, nil
}

// digestLocked returns every matching item, newest first, hiding images that
// many articles share (site logos), like the Postgres query does.
func (m *Memory) digestLocked(q DigestQuery, source string) []Item {
	needle := strings.ToLower(q.Query)

	shared := m.sharedImagesLocked()

	var items []Item
	for hash, p := range m.pages {
		ts := p.Published
		if ts.IsZero() {
			ts = m.seen[hash]
		}
		hay := strings.ToLower(p.Title + " " + p.Description + " " + p.H1 + " " + p.Snippet)
		host := pageHost(p.URL)
		category := cmp.Or(p.Category, "other")
		if ts.Before(q.Since) || ts.After(time.Now().Add(24*time.Hour)) || !strings.Contains(hay, needle) ||
			(!q.Until.IsZero() && !ts.Before(q.Until)) ||
			(source != "" && host != source) || (q.Category != "" && category != q.Category) ||
			(q.ArticlesOnly && !p.IsArticle) {
			continue
		}

		image := effectiveImage(p, shared)
		en := m.enrichments[hash]
		it := Item{
			ID: hash, URL: p.URL, Title: p.Title, Description: p.Description, Snippet: p.Snippet,
			Source: host, Image: image, Summary: m.summaries[hash], IsArticle: p.IsArticle, Time: ts, Category: category,
			Why: en.Why, Tags: en.Tags, Importance: en.Importance, Rel: en.Exam.Rel(), Coverage: m.stories[hash].Sources, Words: p.Words,
		}
		if q.Detail {
			it.Long = en.Long
		}
		items = append(items, it)
	}

	sort.Slice(items, func(i, j int) bool {
		if !items[i].Time.Equal(items[j].Time) {
			return items[i].Time.After(items[j].Time)
		}
		return items[i].URL < items[j].URL
	})

	return items
}

func (m *Memory) Item(_ context.Context, id string) (Item, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.pages[id]
	if !ok || !p.IsArticle {
		return Item{}, false, nil
	}
	ts := p.Published
	if ts.IsZero() {
		ts = m.seen[id]
	}
	en := m.enrichments[id]

	it := Item{
		ID: id, URL: p.URL, Title: p.Title, Description: p.Description, Snippet: p.Snippet,
		Source: pageHost(p.URL), Image: effectiveImage(p, m.sharedImagesLocked()), Summary: m.summaries[id], IsArticle: true,
		Time: ts, Category: cmp.Or(p.Category, "other"), Why: en.Why, Tags: en.Tags, Importance: en.Importance,
		Rel: en.Exam.Rel(), Exam: en.Exam, Coverage: m.stories[id].Sources, Entities: en.Entities, Words: p.Words,
		Lead: prose.Lead(p.Content, leadSentences, leadWords),
	}
	if g, ok := m.stories[id]; ok {
		type rel struct {
			Related
			ts time.Time
		}
		var others []rel
		for oid, og := range m.stories {
			op := m.pages[oid]
			if oid == id || og.ID != g.ID || !op.IsArticle {
				continue
			}
			ots := op.Published
			if ots.IsZero() {
				ots = m.seen[oid]
			}
			others = append(others, rel{Related{ID: oid, Title: op.Title, Source: pageHost(op.URL), Time: ots}, ots})
		}
		sort.Slice(others, func(i, j int) bool {
			if !others[i].ts.Equal(others[j].ts) {
				return others[i].ts.After(others[j].ts)
			}
			return m.pages[others[i].ID].URL < m.pages[others[j].ID].URL
		})
		for _, o := range others[:min(len(others), maxAlso)] {
			it.Also = append(it.Also, o.Related)
		}
	}

	return it, true, nil
}

func (m *Memory) GroupCandidates(_ context.Context, since time.Time, limit int) ([]Candidate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var out []Candidate
	for hash, p := range m.pages {
		ts := p.Published
		if ts.IsZero() {
			ts = m.seen[hash]
		}
		if p.IsArticle && p.Title != "" && !ts.Before(since) {
			out = append(out, Candidate{ID: hash, Source: pageHost(p.URL), Title: p.Title, Time: ts})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Time.Equal(out[j].Time) {
			return out[i].Time.After(out[j].Time)
		}
		return m.pages[out[i].ID].URL < m.pages[out[j].ID].URL
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}

	return out, nil
}

func (m *Memory) SetStories(_ context.Context, since time.Time, groups map[string]StoryGroup) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id := range m.stories {
		ts := m.pages[id].Published
		if ts.IsZero() {
			ts = m.seen[id]
		}
		if _, keep := groups[id]; !keep && !ts.Before(since) {
			delete(m.stories, id)
		}
	}
	if m.stories == nil {
		m.stories = make(map[string]StoryGroup)
	}
	for id, g := range groups {
		m.stories[id] = g
	}

	return nil
}

func (m *Memory) DigestSources(_ context.Context, q DigestQuery) ([]SourceCount, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	type key struct{ host, category string }
	counts := map[key]int{}
	for _, it := range m.digestLocked(q, "") {
		counts[key{it.Source, it.Category}]++
	}

	out := []SourceCount{}
	for k, n := range counts {
		out = append(out, SourceCount{Source: k.host, Category: k.category, Count: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		if out[i].Source != out[j].Source {
			return out[i].Source < out[j].Source
		}
		return out[i].Category < out[j].Category
	})

	return out, nil
}

func (m *Memory) ImageURL(_ context.Context, urlHash string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	return effectiveImage(m.pages[urlHash], m.sharedImagesLocked()), nil
}

// sharedImagesLocked returns the generic images: those used as the og:image or body image of
// genericImageMin or more distinct articles on at most genericImageHosts sites.
func (m *Memory) sharedImagesLocked() map[string]bool {
	type use struct {
		articles int
		hosts    map[string]bool
	}
	uses := map[string]*use{}
	add := func(img, host string) {
		u := uses[img]
		if u == nil {
			u = &use{hosts: map[string]bool{}}
			uses[img] = u
		}
		u.articles++
		u.hosts[host] = true
	}
	for _, p := range m.pages {
		if !p.IsArticle {
			continue
		}
		host := pageHost(p.URL)
		if p.Image != "" {
			add(p.Image, host)
		}
		if p.BodyImage != "" && p.BodyImage != p.Image {
			add(p.BodyImage, host)
		}
	}
	generic := map[string]bool{}
	for img, u := range uses {
		if u.articles >= genericImageMin && len(u.hosts) <= genericImageHosts {
			generic[img] = true
		}
	}
	return generic
}

func (m *Memory) ImageStats(_ context.Context, since time.Time) (ImageCoverage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	generic := m.sharedImagesLocked()
	var c ImageCoverage
	for hash, p := range m.pages {
		if !p.IsArticle || m.pageTime(hash, p).Before(since) {
			continue
		}
		switch {
		case effectiveImage(p, generic) != "":
			c.Shown++
		case p.Image != "" || p.BodyImage != "":
			c.Logo++
		default:
			c.None++
		}
	}
	return c, nil
}

// effectiveImage mirrors effectiveImageSQL: the og:image unless it is a shared
// logo, else the body image unless that is shared too, else none.
func effectiveImage(p Page, generic map[string]bool) string {
	if p.Image != "" && !generic[p.Image] {
		return p.Image
	}
	if p.BodyImage != "" && !generic[p.BodyImage] {
		return p.BodyImage
	}
	return ""
}

func (m *Memory) PendingSummaries(_ context.Context, since time.Time, limit int) ([]Unsummarized, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var out []Unsummarized
	for hash, p := range m.pages {
		ts := p.Published
		if ts.IsZero() {
			ts = m.seen[hash]
		}
		if _, done := m.summaries[hash]; done || !p.IsArticle || ts.Before(since) {
			continue
		}
		out = append(out, Unsummarized{URLHash: hash, URL: p.URL, Title: p.Title, Description: p.Description, Content: p.Content})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].URL < out[j].URL })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}

	return out, nil
}

func (m *Memory) PendingEnrichment(_ context.Context, since time.Time, limit int) ([]Unenriched, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	type dated struct {
		u  Unenriched
		ts time.Time
	}
	var found []dated
	for hash, p := range m.pages {
		ts := p.Published
		if ts.IsZero() {
			ts = m.seen[hash]
		}
		_, summarised := m.summaries[hash]
		_, enriched := m.enrichments[hash]
		if !summarised || enriched || !p.IsArticle || ts.Before(since) {
			continue
		}
		found = append(found, dated{Unenriched{
			URLHash: hash, URL: p.URL, Title: p.Title, Description: p.Description, Content: p.Content, Category: cmp.Or(p.Category, "other"),
		}, ts})
	}

	sort.Slice(found, func(i, j int) bool {
		if !found[i].ts.Equal(found[j].ts) {
			return found[i].ts.After(found[j].ts)
		}
		return found[i].u.URL < found[j].u.URL
	})
	if limit > 0 && len(found) > limit {
		found = found[:limit]
	}
	out := make([]Unenriched, len(found))
	for i, f := range found {
		out[i] = f.u
	}

	return out, nil
}

func (m *Memory) SetEnrichment(_ context.Context, urlHash string, e Enrichment) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.enrichments == nil {
		m.enrichments = make(map[string]Enrichment)
	}
	m.enrichments[urlHash] = e

	return nil
}

func (m *Memory) PendingDetail(_ context.Context, since time.Time, limit int) ([]Unenriched, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	type scored struct {
		u          Unenriched
		importance int
		ts         time.Time
	}
	var found []scored
	for hash, p := range m.pages {
		en, enriched := m.enrichments[hash]
		ts := p.Published
		if ts.IsZero() {
			ts = m.seen[hash]
		}
		if !enriched || m.detailed[hash] || !p.IsArticle || ts.Before(since) {
			continue
		}
		found = append(found, scored{Unenriched{URLHash: hash, URL: p.URL, Title: p.Title, Description: p.Description, Content: p.Content, Category: cmp.Or(p.Category, "other")}, en.Importance, ts})
	}
	sort.Slice(found, func(i, j int) bool {
		// Editorials first: they always get the long summary and exam lens, whatever their score.
		if ei, ej := found[i].u.Category == "editorial", found[j].u.Category == "editorial"; ei != ej {
			return ei
		}
		if found[i].importance != found[j].importance {
			return found[i].importance > found[j].importance
		}
		if !found[i].ts.Equal(found[j].ts) {
			return found[i].ts.After(found[j].ts)
		}
		return found[i].u.URL < found[j].u.URL
	})
	if limit > 0 && len(found) > limit {
		found = found[:limit]
	}
	out := make([]Unenriched, len(found))
	for i, f := range found {
		out[i] = f.u
	}

	return out, nil
}

func (m *Memory) SetDetail(_ context.Context, urlHash string, d Detail) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	en := m.enrichments[urlHash]
	en.Long, en.Exam = d.Long, d.Exam
	if m.enrichments == nil {
		m.enrichments = make(map[string]Enrichment)
	}
	m.enrichments[urlHash] = en
	if m.detailed == nil {
		m.detailed = make(map[string]bool)
	}
	m.detailed[urlHash] = true

	return nil
}

func (m *Memory) PendingImages(_ context.Context, since time.Time, limit int) ([]ImageJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var out []ImageJob
	for hash, p := range m.pages {
		ts := p.Published
		if ts.IsZero() {
			ts = m.seen[hash]
		}
		if !p.IsArticle || p.SourceURL == "" || m.imageChecked[hash] || ts.Before(since) {
			continue
		}
		out = append(out, ImageJob{URLHash: hash, URL: p.URL, SourceURL: p.SourceURL})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].URL < out[j].URL })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}

	return out, nil
}

func (m *Memory) SetImage(_ context.Context, urlHash, image string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if p, ok := m.pages[urlHash]; ok && image != "" {
		p.Image = image
		m.pages[urlHash] = p
	}
	m.imageChecked[urlHash] = true

	return nil
}

func (m *Memory) SetSummary(_ context.Context, urlHash string, bullets []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if bullets == nil {
		bullets = []string{}
	}
	m.summaries[urlHash] = bullets

	return nil
}

// pageHost returns the lowercase host[:port] of rawURL, "" when unparsable.
func pageHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Host)
}

// subscriber mirrors a row of the subscribers table.
type subscriber struct {
	id           int64
	tracks       []string
	mode         string
	frequency    string
	topics       []string
	token        string
	source       string
	confirmed    bool
	unsubscribed bool
	// The moments of signup, confirmation and unsubscribing; zero when it has not happened.
	created, confirmedAt, unsubAt time.Time
}

func (m *Memory) Subscribe(_ context.Context, email, token string) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.subscribers == nil {
		m.subscribers = make(map[string]*subscriber)
	}
	sub, ok := m.subscribers[email]
	switch {
	case !ok:
		m.subSeq++
		sub = &subscriber{id: m.subSeq, token: token, created: time.Now()}
		m.subscribers[email] = sub
	case sub.unsubscribed:
		*sub = subscriber{id: sub.id, token: token, source: sub.source, created: sub.created}
	}

	return sub.token, sub.confirmed, nil
}

func (m *Memory) Confirm(_ context.Context, token string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, sub := range m.subscribers {
		if sub.token == token && !sub.unsubscribed {
			if !sub.confirmed {
				sub.confirmedAt = time.Now()
			}
			sub.confirmed = true
			return true, nil
		}
	}

	return false, nil
}

func (m *Memory) Unsubscribe(_ context.Context, token string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, sub := range m.subscribers {
		if sub.token == token {
			if !sub.unsubscribed {
				sub.unsubAt = time.Now()
			}
			sub.unsubscribed = true
			return true, nil
		}
	}

	return false, nil
}

func (m *Memory) SetPreferences(_ context.Context, token string, p Preferences) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, sub := range m.subscribers {
		if sub.token == token {
			sub.tracks, sub.mode = track.Normalize(p.Tracks, p.Mode)
			sub.topics = track.NormalizeTopics(p.Topics)
			sub.frequency = track.NormalizeFrequency(p.Frequency)
			if p.Source != "" {
				sub.source = p.Source
			}
			return true, nil
		}
	}

	return false, nil
}

func (m *Memory) ManageToken(_ context.Context, email string) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if sub, ok := m.subscribers[email]; ok && sub.confirmed && !sub.unsubscribed {
		return sub.token, true, nil
	}

	return "", false, nil
}

func (m *Memory) Subscription(_ context.Context, token string) (Subscription, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for email, sub := range m.subscribers {
		if sub.token == token {
			tracks, mode := track.Normalize(sub.tracks, sub.mode)
			return Subscription{Email: email, Status: subscriptionStatus(sub.confirmed, sub.unsubscribed), Tracks: tracks, Mode: mode, Topics: append([]string{}, sub.topics...), Frequency: track.NormalizeFrequency(sub.frequency)}, true, nil
		}
	}

	return Subscription{}, false, nil
}

func (m *Memory) AudienceStats(_ context.Context, days int, tz string) (Audience, error) {
	days, loc, err := audienceWindow(days, tz)
	if err != nil {
		return Audience{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	counts := map[string]*AudienceDay{}
	day := func(t time.Time) *AudienceDay {
		k := t.In(loc).Format("2006-01-02")
		if counts[k] == nil {
			counts[k] = &AudienceDay{}
		}
		return counts[k]
	}
	a := Audience{Modes: map[string]int{}}
	topics := map[string]int{}
	sources := map[string]*AudienceSource{}
	for _, sub := range m.subscribers {
		switch {
		case sub.unsubscribed:
			a.Unsubscribed++
		case sub.confirmed:
			a.Active++
			_, mode := track.Normalize(sub.tracks, sub.mode)
			a.Modes[mode]++
			for _, t := range sub.topics {
				topics[t]++
			}
		default:
			a.Pending++
		}
		day(sub.created).Signups++
		if sub.confirmed || !sub.confirmedAt.IsZero() {
			day(sub.confirmedAt).Confirmed++
		}
		if sub.unsubscribed {
			day(sub.unsubAt).Unsubscribed++
		}
		src := sources[sub.source]
		if src == nil {
			src = &AudienceSource{Source: sub.source}
			sources[sub.source] = src
		}
		src.Signups++
		if sub.confirmed || !sub.confirmedAt.IsZero() {
			src.Confirmed++
		}
	}
	for d, ids := range m.delivered {
		if t, err := time.ParseInLocation("2006-01-02", d, loc); err == nil {
			day(t).Sent = len(ids)
		}
	}
	a.Daily = fillDays(now, days, loc, counts)

	for t, n := range topics {
		a.Topics = append(a.Topics, TopicCount{Topic: t, Count: n})
	}
	sort.Slice(a.Topics, func(i, j int) bool {
		if a.Topics[i].Count != a.Topics[j].Count {
			return a.Topics[i].Count > a.Topics[j].Count
		}
		return a.Topics[i].Topic < a.Topics[j].Topic
	})
	if len(a.Topics) > 10 {
		a.Topics = a.Topics[:10]
	}
	for _, src := range sources {
		a.Sources = append(a.Sources, *src)
	}
	sort.Slice(a.Sources, func(i, j int) bool {
		if a.Sources[i].Signups != a.Sources[j].Signups {
			return a.Sources[i].Signups > a.Sources[j].Signups
		}
		return a.Sources[i].Source < a.Sources[j].Source
	})

	return a, nil
}

func (m *Memory) ContentHealth(_ context.Context, days int, tz string) (Content, error) {
	days, loc, err := audienceWindow(days, tz)
	if err != nil {
		return Content{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	week := now.AddDate(0, 0, -7)
	counts := map[string]int{}
	var c Content
	for hash, p := range m.pages {
		if !p.IsArticle {
			continue
		}
		when := p.Published
		if when.IsZero() {
			when = m.seen[hash]
		}
		counts[when.In(loc).Format("2006-01-02")]++
		if m.seen[hash].After(c.Latest) {
			c.Latest = m.seen[hash]
		}
		if when.Before(week) {
			continue
		}
		_, summarised := m.summaries[hash]
		_, enriched := m.enrichments[hash]
		switch {
		case !summarised:
			c.Waiting.Summary++
		case !enriched:
			c.Waiting.Enrichment++
		case !m.detailed[hash]:
			c.Waiting.Detail++
		}
		if p.SourceURL != "" && !m.imageChecked[hash] {
			c.Waiting.Image++
		}
	}
	for i := days - 1; i >= 0; i-- {
		day := now.In(loc).AddDate(0, 0, -i).Format("2006-01-02")
		c.Daily = append(c.Daily, ContentDay{Day: day, Articles: counts[day]})
	}

	return c, nil
}

func (m *Memory) Subscribers(_ context.Context, q string, limit, offset int) ([]SubscriberRow, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	q = strings.ToLower(strings.TrimSpace(q))
	var all []SubscriberRow
	for email, sub := range m.subscribers {
		if !strings.Contains(email, q) {
			continue
		}
		_, mode := track.Normalize(sub.tracks, sub.mode)
		all = append(all, SubscriberRow{
			ID: sub.id, Email: email, Status: subscriptionStatus(sub.confirmed, sub.unsubscribed), Mode: mode,
			Topics: append([]string{}, sub.topics...), Source: sub.source, Created: sub.created, Confirmed: sub.confirmedAt,
		})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID > all[j].ID })
	total := len(all)
	if offset >= total {
		return nil, total, nil
	}

	return all[offset:min(offset+limit, total)], total, nil
}

func (m *Memory) RemoveSubscriber(_ context.Context, id int64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for email, sub := range m.subscribers {
		if sub.id == id {
			delete(m.subscribers, email)
			for _, ids := range m.delivered {
				delete(ids, id)
			}
			return true, nil
		}
	}

	return false, nil
}

func (m *Memory) SaveAdminToken(_ context.Context, t AdminToken) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.adminTokens == nil {
		m.adminTokens = map[string]AdminToken{}
	}
	now := time.Now()
	for h, old := range m.adminTokens {
		if old.Expires.Before(now) {
			delete(m.adminTokens, h)
		}
	}
	m.adminTokens[t.Hash] = t

	return nil
}

func (m *Memory) UseAdminToken(_ context.Context, hash, kind string, consume bool, now time.Time) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	t, ok := m.adminTokens[hash]
	if !ok || t.Kind != kind || !t.Expires.After(now) {
		return "", false, nil
	}
	if consume {
		delete(m.adminTokens, hash)
	}

	return t.Email, true, nil
}

func (m *Memory) DeleteAdminToken(_ context.Context, hash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.adminTokens, hash)

	return nil
}

func (m *Memory) ConfirmedSubscribers(_ context.Context) ([]Subscriber, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var out []Subscriber
	for email, sub := range m.subscribers {
		if sub.confirmed && !sub.unsubscribed {
			tracks, mode := track.Normalize(sub.tracks, sub.mode)
			out = append(out, Subscriber{ID: sub.id, Email: email, Token: sub.token, Tracks: tracks, Mode: mode, Topics: sub.topics, Frequency: track.NormalizeFrequency(sub.frequency)})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })

	return out, nil
}

func (m *Memory) DeliveredOn(_ context.Context, day string) (map[int64]bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := make(map[int64]bool, len(m.delivered[day]))
	for id := range m.delivered[day] {
		out[id] = true
	}

	return out, nil
}

func (m *Memory) MarkDelivered(_ context.Context, subscriberID int64, day string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.delivered == nil {
		m.delivered = make(map[string]map[int64]bool)
	}
	if m.delivered[day] == nil {
		m.delivered[day] = make(map[int64]bool)
	}
	m.delivered[day][subscriberID] = true

	return nil
}

func (m *Memory) ChannelPosted(_ context.Context, day, channel string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.posted[[2]string{day, channel}], nil
}

func (m *Memory) MarkChannelPosted(_ context.Context, day, channel string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.posted == nil {
		m.posted = make(map[[2]string]bool)
	}
	m.posted[[2]string{day, channel}] = true

	return nil
}

func (m *Memory) GetQuiz(_ context.Context, day, track string) (Quiz, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if day != "" {
		q, ok := m.quizzes[[2]string{day, track}]
		return q, ok, nil
	}
	var best Quiz
	found := false
	for k, q := range m.quizzes {
		if k[1] == track && (!found || k[0] > best.Day) {
			best, found = q, true
		}
	}

	return best, found, nil
}

func (m *Memory) SaveQuiz(_ context.Context, q Quiz) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.quizzes == nil {
		m.quizzes = make(map[[2]string]Quiz)
	}
	m.quizzes[[2]string{q.Day, q.Track}] = q

	return nil
}

func (m *Memory) GetIntro(_ context.Context, day, track string) (Intro, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if day != "" {
		in, ok := m.intros[[2]string{day, track}]
		return in, ok, nil
	}
	var best Intro
	found := false
	for k, in := range m.intros {
		if k[1] == track && (!found || k[0] > best.Day) {
			best, found = in, true
		}
	}

	return best, found, nil
}

func (m *Memory) SaveIntro(_ context.Context, in Intro) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.intros == nil {
		m.intros = make(map[[2]string]Intro)
	}
	m.intros[[2]string{in.Day, in.Track}] = in

	return nil
}

func (m *Memory) ProExtend(_ context.Context, paymentID, email string, days int, token string) (ProMember, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.pro == nil {
		m.pro, m.payments = map[string]ProMember{}, map[string]string{}
	}
	if _, seen := m.payments[paymentID]; seen {
		return m.pro[email], false, nil
	}
	m.payments[paymentID] = email

	member, ok := m.pro[email]
	if !ok {
		member = ProMember{Email: email, Token: token}
	}
	from := time.Now()
	if member.Until.After(from) {
		from = member.Until
	}
	member.Until = from.AddDate(0, 0, days)
	m.pro[email] = member

	return member, true, nil
}

func (m *Memory) ProByToken(_ context.Context, token string) (ProMember, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, member := range m.pro {
		if member.Token == token {
			return member, true, nil
		}
	}

	return ProMember{}, false, nil
}

func (m *Memory) ProNotes(_ context.Context, email string) ([]ProNote, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := []ProNote{}
	for _, n := range m.proNotes[email] {
		n.Tags = append([]string{}, n.Tags...)
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].At != out[j].At {
			return out[i].At > out[j].At
		}
		return out[i].ID < out[j].ID
	})

	return out, nil
}

func (m *Memory) SaveProNote(_ context.Context, email string, n ProNote, max int) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.proNotes == nil {
		m.proNotes = map[string]map[string]ProNote{}
	}
	if m.proNotes[email] == nil {
		m.proNotes[email] = map[string]ProNote{}
	}
	if _, have := m.proNotes[email][n.ID]; !have && len(m.proNotes[email]) >= max {
		return false, nil
	}
	if n.Tags == nil {
		n.Tags = []string{}
	}
	m.proNotes[email][n.ID] = n

	return true, nil
}

func (m *Memory) AddProNotes(_ context.Context, email string, notes []ProNote, max int) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.proNotes == nil {
		m.proNotes = map[string]map[string]ProNote{}
	}
	if m.proNotes[email] == nil {
		m.proNotes[email] = map[string]ProNote{}
	}
	added := 0
	for _, n := range notes {
		if _, have := m.proNotes[email][n.ID]; have || len(m.proNotes[email]) >= max {
			continue
		}
		if n.Tags == nil {
			n.Tags = []string{}
		}
		m.proNotes[email][n.ID] = n
		added++
	}

	return added, nil
}

func (m *Memory) DeleteProNote(_ context.Context, email, id string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if id == "" {
		n := len(m.proNotes[email])
		delete(m.proNotes, email)
		return n, nil
	}
	if _, have := m.proNotes[email][id]; !have {
		return 0, nil
	}
	delete(m.proNotes[email], id)

	return 1, nil
}

func (m *Memory) ActiveProTokens(_ context.Context) (map[string]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := map[string]string{}
	for email, member := range m.pro {
		if member.Until.After(time.Now()) {
			out[strings.ToLower(email)] = member.Token
		}
	}

	return out, nil
}

func (m *Memory) Cards(_ context.Context, since time.Time, limit int) ([]StoryCards, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	type dated struct {
		StoryCards
		ts  time.Time
		url string
	}
	var found []dated
	for hash, p := range m.pages {
		en, ok := m.enrichments[hash]
		if !ok || en.Exam == nil || !p.IsArticle || (len(en.Exam.Cards) == 0 && len(en.Exam.Facts) == 0) {
			continue
		}
		ts := p.Published
		if ts.IsZero() {
			ts = m.seen[hash]
		}
		if ts.Before(since) || ts.After(time.Now().Add(24*time.Hour)) {
			continue
		}
		found = append(found, dated{StoryCards{ID: hash, Title: p.Title, Importance: en.Importance, Cards: en.Exam.Cards, Facts: en.Exam.Facts}, ts, p.URL})
	}
	sort.Slice(found, func(i, j int) bool {
		if found[i].Importance != found[j].Importance {
			return found[i].Importance > found[j].Importance
		}
		if !found[i].ts.Equal(found[j].ts) {
			return found[i].ts.After(found[j].ts)
		}
		return found[i].url < found[j].url
	})

	var out []StoryCards
	for _, f := range found {
		if limit > 0 && len(out) >= limit {
			break
		}
		out = append(out, f.StoryCards)
	}

	return out, nil
}

func (m *Memory) GetCompilation(_ context.Context, month, track string) (Compilation, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	c, ok := m.compilations[[2]string{month, track}]

	return c, ok, nil
}

func (m *Memory) SaveCompilation(_ context.Context, c Compilation) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.compilations == nil {
		m.compilations = make(map[[2]string]Compilation)
	}
	c.Built = time.Now()
	m.compilations[[2]string{c.Month, c.Track}] = c

	return nil
}

func (m *Memory) CompilationMonths(_ context.Context) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	seen := map[string]bool{}
	var out []string
	for k := range m.compilations {
		if !seen[k[0]] {
			seen[k[0]] = true
			out = append(out, k[0])
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(out)))

	return out, nil
}

func (m *Memory) pageTime(hash string, p Page) time.Time {
	if ts := p.Published; !ts.IsZero() {
		return ts
	}

	return m.seen[hash]
}

func (m *Memory) AddConceptTerms(_ context.Context, terms []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.concepts == nil {
		m.concepts = make(map[string]*Concept)
	}
	for _, t := range terms {
		if key := strings.ToLower(t); m.concepts[key] == nil {
			m.concepts[key] = &Concept{Term: t}
			m.conceptN = append(m.conceptN, key)
		}
	}

	return nil
}

func (m *Memory) PendingConcepts(_ context.Context, limit int) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var out []string
	for _, key := range m.conceptN {
		if c := m.concepts[key]; c.Body == "" && len(out) < limit {
			out = append(out, c.Term)
		}
	}

	return out, nil
}

func (m *Memory) SetConcept(_ context.Context, term, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if c := m.concepts[strings.ToLower(term)]; c != nil {
		c.Body = body
	}

	return nil
}

func (m *Memory) Concepts(_ context.Context, limit int) ([]Concept, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	keys := make([]string, 0, len(m.concepts))
	for key, c := range m.concepts {
		if c.Body != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	if limit > 0 && len(keys) > limit {
		keys = keys[:limit]
	}
	out := make([]Concept, 0, len(keys))
	for _, key := range keys {
		out = append(out, *m.concepts[key])
	}

	return out, nil
}

func (m *Memory) EntityStories(_ context.Context, since time.Time) ([]EntityStory, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var out []EntityStory
	for hash, p := range m.pages {
		en := m.enrichments[hash]
		if ts := m.pageTime(hash, p); p.IsArticle && len(en.Entities) > 0 && !ts.Before(since) {
			out = append(out, EntityStory{ID: hash, Title: p.Title, Source: pageHost(p.URL), Why: en.Why, Time: ts, Entities: en.Entities})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Time.Equal(out[j].Time) {
			return out[i].Time.After(out[j].Time)
		}
		return out[i].ID < out[j].ID
	})

	return out, nil
}

func (m *Memory) AddTeamHook(_ context.Context, kind, url, track, token string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, h := range m.hooks {
		if h.URL == url || h.Token == token {
			return false, nil
		}
	}
	m.nextHook++
	m.hooks = append(m.hooks, TeamHook{ID: m.nextHook, Kind: kind, URL: url, Track: track, Token: token})

	return true, nil
}

func (m *Memory) RemoveTeamHook(_ context.Context, token string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, h := range m.hooks {
		if h.Token == token {
			m.hooks = append(m.hooks[:i], m.hooks[i+1:]...)
			return true, nil
		}
	}

	return false, nil
}

func (m *Memory) TeamHooks(_ context.Context) ([]TeamHook, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	return append([]TeamHook(nil), m.hooks...), nil
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
