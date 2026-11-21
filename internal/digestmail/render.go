// Package digestmail builds the daily email edition and sends it to the confirmed
// subscribers, at most once per subscriber per day. The email is one edition of the top news
// across every section, whatever editions a subscriber once chose: "Today in 60 seconds", the top
// stories, then a short heading per section, each story with a "why it matters" line and a link to
// its page on Postpaper. A subscriber chooses a reading length: headlines with the "why it matters"
// line, or longer summaries. The Telegram and Teams posts still go per track (see package track).
package digestmail

import (
	"context"
	"fmt"
	"html/template"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/mail"
	"github.com/wanderingtilldeath/news-aggregator/internal/store"
	"github.com/wanderingtilldeath/news-aggregator/internal/track"
)

const (
	// maxPerSource keeps one busy site from filling a track.
	maxPerSource = 3
	// maxPerSourceEmail is the most stories one site may have across the whole general edition.
	maxPerSourceEmail = 4
	// briefMax is the most stories per track in the one-minute read.
	briefMax = 5
	// gistRunes caps a fallback description when an article has no summary bullets.
	gistRunes = 220
	// maxEmailBullets keeps the 7-minute read of each story to the first few gist bullets.
	maxEmailBullets = 3
	// unrankedScore is what an article the enrichment pass has not scored counts as.
	unrankedScore = 2
)

type entry struct {
	// ID is the story's id: the email links to its page on Postpaper (/story/{id}), not to the publisher.
	ID                                string
	Title, URL, Source, Category, Why string
	Tags                              []string
	Gist                              []string
	// Long is the 7-minute summary; empty in the one-minute read or when missing.
	Long string
}

type section struct {
	Track, Title string
	Entries      []entry
	// Quiz is the day's quiz of the track, nil when none was written.
	Quiz *store.Quiz
	// Intro is the day's "Today in 60 seconds" paragraph of the track, empty when none was written.
	Intro string
}

type edition struct {
	Day  string
	Mode string
	// Intro is the day's "Today in 60 seconds" paragraph of the general edition, empty when none was written.
	Intro    string
	Sections []section
}

// count is the number of stories in the edition.
func (e edition) count() int {
	n := 0
	for _, s := range e.Sections {
		n += len(s.Entries)
	}

	return n
}

// builder assembles editions from the store, caching the stories of each track.
type builder struct {
	st    store.Store
	since time.Time
	top   int
	cache map[string][]entry
	// topics caches the stories found for each followed word.
	topics map[string][]store.Item
	// generals caches the general edition of each reading length.
	generals map[string]edition
}

// edition builds the edition for these tracks and reading length.
func (b *builder) edition(ctx context.Context, tracks []string, mode, day string, topics []string) (edition, error) {
	ed := edition{Day: day, Mode: mode}
	perTrack := b.top
	if mode == track.Mode1 {
		perTrack = min(perTrack, briefMax)
	}

	for _, tr := range tracks {
		key := fmt.Sprintf("%s|%v", tr, mode == track.Mode7)
		entries, ok := b.cache[key]
		if !ok {
			var items []store.Item
			for _, cat := range track.Categories(tr) {
				got, _, err := b.st.Digest(ctx, store.DigestQuery{
					Since: b.since, ArticlesOnly: true, Category: cat, Limit: 100, Detail: mode == track.Mode7,
				})
				if err != nil {
					return edition{}, fmt.Errorf("digestmail: load %s articles: %w", cat, err)
				}
				items = append(items, got...)
			}
			entries = pick(items, perTrack)
			b.cache[key] = entries
		}
		if len(entries) > 0 {
			sec := section{Track: tr, Title: track.Title(tr), Entries: entries}
			if q, ok, err := b.st.GetQuiz(ctx, day, tr); err != nil {
				return edition{}, fmt.Errorf("digestmail: load %s quiz: %w", tr, err)
			} else if ok {
				sec.Quiz = &q
			}
			if in, ok, err := b.st.GetIntro(ctx, day, tr); err != nil {
				return edition{}, fmt.Errorf("digestmail: load %s intro: %w", tr, err)
			} else if ok {
				sec.Intro = in.Text
			}
			ed.Sections = append(ed.Sections, sec)
		}
	}

	if sec, err := b.topicSection(ctx, ed, topics, mode == track.Mode7); err != nil {
		return edition{}, err
	} else if len(sec.Entries) > 0 {
		ed.Sections = append(ed.Sections, sec)
	}

	return ed, nil
}

const (
	// The general edition: how many stories lead it and how many each section adds, in the one-minute read
	// (headlines and a line each) and in the longer read (summaries), and how many sections it has at most.
	topBrief, perSectionBrief = 10, 4
	topDeep, perSectionDeep   = 6, 3
	maxSections               = 6
)

// sectionTitles are the headings of the general edition. They name the kind of news, never an exam or a
// profession: the email is for everyone.
var sectionTitles = map[string]string{
	"india": "India", "world": "World", "tech": "Technology", "ai": "AI", "funding": "Startups and funding",
	"editorial": "Editorials", "upsc": "Policy and governance", "other": "More news",
}

// sectionOrder is the order those sections appear in; a category not listed follows, by name.
var sectionOrder = []string{"india", "world", "tech", "ai", "funding", "editorial", "upsc", "other"}

func sectionTitle(cat string) string {
	if t, ok := sectionTitles[cat]; ok {
		return t
	}
	return strings.ToUpper(cat[:1]) + cat[1:]
}

// general builds the one edition every subscriber gets in this reading length (plus their own topics): the
// day's intro, a "Top stories" section of the most important stories across everything, then a section for
// each category with the best of what the top stories left out. Built once per reading length and reused.
func (b *builder) general(ctx context.Context, mode, day string, topics []string) (edition, error) {
	deep := mode == track.Mode7
	key := "general|" + mode
	base, ok := b.generals[key]
	if !ok {
		var err error
		if base, err = b.buildGeneral(ctx, mode, day, deep); err != nil {
			return edition{}, err
		}
		if b.generals == nil {
			b.generals = map[string]edition{}
		}
		b.generals[key] = base
	}

	ed := base
	ed.Sections = append([]section(nil), base.Sections...)
	if sec, err := b.topicSection(ctx, ed, topics, deep); err != nil {
		return edition{}, err
	} else if len(sec.Entries) > 0 {
		ed.Sections = append(ed.Sections, sec)
	}

	return ed, nil
}

func (b *builder) buildGeneral(ctx context.Context, mode, day string, deep bool) (edition, error) {
	ed := edition{Day: day, Mode: mode}

	cats, err := b.st.DigestSources(ctx, store.DigestQuery{Since: b.since, ArticlesOnly: true})
	if err != nil {
		return edition{}, fmt.Errorf("digestmail: load categories: %w", err)
	}
	present := map[string]bool{}
	for _, c := range cats {
		present[c.Category] = true
	}
	var order []string
	for _, c := range sectionOrder {
		if present[c] {
			order = append(order, c)
			delete(present, c)
		}
	}
	rest := make([]string, 0, len(present))
	for c := range present {
		rest = append(rest, c)
	}
	sort.Strings(rest)
	order = append(order, rest...)

	byCat := map[string][]store.Item{}
	var all []store.Item
	for _, cat := range order {
		got, _, err := b.st.Digest(ctx, store.DigestQuery{Since: b.since, ArticlesOnly: true, Category: cat, Limit: 100, Detail: deep})
		if err != nil {
			return edition{}, fmt.Errorf("digestmail: load %s articles: %w", cat, err)
		}
		byCat[cat] = got
		all = append(all, got...)
	}

	topN, perSection := topBrief, perSectionBrief
	if deep {
		topN, perSection = topDeep, perSectionDeep
	}
	used := map[string]bool{}
	perSource := map[string]int{}
	lead := pickShared(all, topN, perSource, maxPerSourceEmail)
	for _, e := range lead {
		used[e.URL] = true
	}
	if len(lead) > 0 {
		ed.Sections = append(ed.Sections, section{Track: track.General, Title: "Top stories", Entries: lead})
	}
	for _, cat := range order {
		if len(ed.Sections) > maxSections {
			break
		}
		fresh := make([]store.Item, 0, len(byCat[cat]))
		for _, it := range byCat[cat] {
			if !used[it.URL] {
				fresh = append(fresh, it)
			}
		}
		entries := pickShared(fresh, perSection, perSource, maxPerSourceEmail)
		if len(entries) == 0 {
			continue
		}
		for _, e := range entries {
			used[e.URL] = true
		}
		ed.Sections = append(ed.Sections, section{Track: cat, Title: sectionTitle(cat), Entries: entries})
	}

	if len(ed.Sections) > 0 {
		in, ok, err := b.st.GetIntro(ctx, day, track.General)
		if err != nil {
			return edition{}, fmt.Errorf("digestmail: load intro: %w", err)
		}
		if ok {
			ed.Intro = in.Text
		} else {
			// No general intro yet (the model was busy): the two edition intros, joined, say the same things.
			var parts []string
			for _, tr := range track.All() {
				if tin, ok, err := b.st.GetIntro(ctx, day, tr); err == nil && ok {
					parts = append(parts, tin.Text)
				}
			}
			ed.Intro = strings.Join(parts, " ")
		}
	}

	return ed, nil
}

const (
	// topicsPerWord and topicsMax keep the "Your topics" section short.
	topicsPerWord = 2
	topicsMax     = 6
)

// topicSection lists the newest stories that mention each followed word, leaving out the ones the
// edition already has. The search is the same full-text query the site uses.
func (b *builder) topicSection(ctx context.Context, ed edition, topics []string, deep bool) (section, error) {
	sec := section{Track: "topics", Title: "Your topics"}
	have := map[string]bool{}
	for _, s := range ed.Sections {
		for _, e := range s.Entries {
			have[e.URL] = true
		}
	}
	for _, topic := range topics {
		items, ok := b.topics[topic]
		if !ok {
			var err error
			items, _, err = b.st.Digest(ctx, store.DigestQuery{Since: b.since, ArticlesOnly: true, Query: topic, Limit: 30, Detail: deep})
			if err != nil {
				return section{}, fmt.Errorf("digestmail: load topic %q: %w", topic, err)
			}
			b.topics[topic] = items
		}
		fresh := make([]store.Item, 0, len(items))
		for _, it := range items {
			if !have[it.URL] {
				fresh = append(fresh, it)
			}
		}
		for _, e := range pick(fresh, topicsPerWord) {
			if len(sec.Entries) >= topicsMax {
				return sec, nil
			}
			have[e.URL] = true
			e.Category = topic
			sec.Entries = append(sec.Entries, e)
		}
	}

	return sec, nil
}

func score(it store.Item) int {
	if it.Importance == 0 {
		return unrankedScore
	}

	return it.Importance
}

// pick chooses up to top stories: the most important first (newest first among
// equals), at most maxPerSource per site, and only those whose link is http(s).
func pick(items []store.Item, top int) []entry {
	return pickShared(items, top, map[string]int{}, maxPerSource)
}

// pickShared is pick with the per-site count and its limit supplied, so several calls can share one cap.
func pickShared(items []store.Item, top int, perSource map[string]int, limit int) []entry {
	sort.SliceStable(items, func(i, j int) bool {
		if si, sj := score(items[i]), score(items[j]); si != sj {
			return si > sj
		}
		return items[i].Time.After(items[j].Time)
	})

	var out []entry
	for _, it := range items {
		if len(out) >= top {
			break
		}
		u, err := url.Parse(it.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || strings.TrimSpace(it.Title) == "" {
			continue
		}
		if perSource[it.Source] >= limit {
			continue
		}
		perSource[it.Source]++

		e := entry{
			ID: it.ID, Title: strings.TrimSpace(it.Title), URL: it.URL, Source: strings.TrimPrefix(it.Source, "www."), Category: it.Category,
			Why: strings.TrimSpace(it.Why), Tags: it.Tags, Long: strings.TrimSpace(it.Long),
		}
		if e.Category == "" {
			e.Category = "other"
		}
		switch {
		case len(it.Summary) > 0:
			e.Gist = it.Summary[:min(len(it.Summary), maxEmailBullets)]
		case strings.TrimSpace(it.Description) != "":
			e.Gist = []string{clip(strings.TrimSpace(it.Description))}
		case strings.TrimSpace(it.Snippet) != "":
			e.Gist = []string{clip(strings.TrimSpace(it.Snippet))}
		}
		out = append(out, e)
	}

	return out
}

func clip(s string) string {
	r := []rune(s)
	if len(r) <= gistRunes {
		return s
	}

	return strings.TrimSpace(string(r[:gistRunes])) + "…"
}

// entryView is an entry as the email shows it, with the link to its page on Postpaper.
type entryView struct {
	entry
	Link string
}

type sectionView struct {
	Title   string
	Entries []entryView
}

// sponsorLine is the optional sponsor line of a mail.
type sponsorLine struct{ Text, URL string }

// sponsorOf returns the line from the options, empty unless text and an http(s) link are both set.
func sponsorOf(o Options) sponsorLine {
	if u, err := url.Parse(o.SponsorURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || strings.TrimSpace(o.SponsorText) == "" {
		return sponsorLine{}
	}

	return sponsorLine{Text: strings.TrimSpace(o.SponsorText), URL: o.SponsorURL}
}

type page struct {
	// ProDay and ProWeek link a Pro member's printable editions; empty for everyone else.
	ProDay, ProWeek string
	Sponsor         sponsorLine
	Date            string
	Label           string
	Intro           string
	Deep            bool
	Sections        []sectionView
	UnsubLink       string
	ManageLink      string
	SwitchLink      string
	SwitchLabel     string
	Footer          string
}

var editionHTML = template.Must(template.New("edition").Parse(`<!doctype html>
<html><body style="font-family:system-ui,-apple-system,sans-serif;max-width:600px;margin:0 auto;padding:16px;color:#111;line-height:1.5">
<h1 style="font-family:Georgia,serif;margin:0">Postpaper</h1>
<p style="color:#5f6670;margin:4px 0 20px">{{.Date}} · {{.Label}}</p>
{{with .Intro}}<p style="margin:0 0 20px;padding:10px 12px;background:#f4f5f7;border-left:3px solid #c8213a"><b>Today in 60 seconds.</b> {{.}}</p>
{{end}}{{$deep := $.Deep}}{{range .Sections}}<h2 style="font-size:13px;text-transform:uppercase;letter-spacing:.6px;border-bottom:2px solid #111;padding-bottom:4px;margin:24px 0 12px">{{.Title}}</h2>
{{range .Entries}}<div style="margin:0 0 18px;padding-bottom:14px;border-bottom:1px solid #dcdfe3">
<h3 style="font-family:Georgia,serif;font-size:18px;margin:0 0 4px"><a href="{{.Link}}" style="color:#111;text-decoration:none">{{.Title}}</a></h3>
{{if .Why}}<p style="margin:0 0 6px;color:#c8213a"><b>Why it matters:</b> {{.Why}}</p>{{end}}
{{if $deep}}{{if .Long}}<p style="margin:0 0 6px">{{.Long}}</p>{{else if .Gist}}<ul style="margin:0 0 6px;padding-left:18px">{{range .Gist}}<li>{{.}}</li>{{end}}</ul>{{end}}{{else if not .Why}}{{if .Gist}}<p style="margin:0 0 6px">{{index .Gist 0}}</p>{{end}}{{end}}
<div style="color:#5f6670;font-size:12px">{{.Source}}{{range .Tags}} · #{{.}}{{end}}</div>
</div>
{{end}}{{end}}{{with .Sponsor.Text}}<p style="margin:0 0 14px;padding:8px 12px;border:1px dashed #b8bdc4;font-size:13px"><span style="text-transform:uppercase;letter-spacing:.6px;font-size:11px;color:#5f6670">Sponsored</span> &nbsp;<a href="{{$.Sponsor.URL}}" rel="sponsored" style="color:#111">{{.}}</a></p>
{{end}}{{with .ProDay}}<p style="margin:0 0 14px;padding:8px 12px;background:#f4f5f7;font-size:13px"><b>Pro.</b> Save as PDF: <a href="{{.}}">today's edition</a> · <a href="{{$.ProWeek}}">the week</a></p>
{{end}}<p style="color:#5f6670;font-size:13px">That is today's edition. You are caught up.</p>
<p style="color:#5f6670;font-size:12px">You get this because you subscribed to Postpaper. Every headline opens the full story on Postpaper, with a link on to the publisher.<br>
<a href="{{.SwitchLink}}">{{.SwitchLabel}}</a> · <a href="{{.ManageLink}}">Manage your subscription</a> · <a href="{{.UnsubLink}}">Unsubscribe</a></p>
{{if .Footer}}<p style="color:#5f6670;font-size:12px">{{.Footer}}</p>{{end}}
</body></html>`))

// storyLink is the address of a story's page on the site; it falls back to the publisher's link for an entry
// without an id.
func storyLink(web string, e entry) string {
	if e.ID == "" {
		return e.URL
	}

	return strings.TrimRight(web, "/") + "/story/" + url.PathEscape(e.ID)
}

// render builds one subscriber's email. api is the public API address for the
// one-click unsubscribe header; empty leaves the header out.
func render(ed edition, to, token, frequency, web, api, footer string, sponsor sponsorLine, proToken string) mail.Message {
	date := ed.Day
	if t, err := time.Parse("2006-01-02", ed.Day); err == nil {
		date = t.Format("Monday 2 January 2006")
	}
	deep := ed.Mode == track.Mode7
	label := "Top news"
	if deep {
		label = "Top news, the longer read"
	}
	if frequency == track.Weekly {
		label = "The week's top news"
		if deep {
			label = "The week's top news, the longer read"
		}
	}
	unsub := strings.TrimRight(web, "/") + "/unsubscribe?token=" + url.QueryEscape(token)
	manage := strings.TrimRight(web, "/") + "/manage?token=" + url.QueryEscape(token)
	// One link to the other rhythm. It opens the manage page with that choice already picked, and nothing changes until
	// the reader saves, so a mail scanner that opens the link cannot switch anyone.
	switchTo, switchLabel := track.Weekly, "Get it weekly instead"
	if frequency == track.Weekly {
		switchTo, switchLabel = track.Daily, "Get it daily instead"
	}
	switchLink := manage + "&frequency=" + switchTo

	views := make([]sectionView, len(ed.Sections))
	for i, sec := range ed.Sections {
		views[i] = sectionView{Title: sec.Title, Entries: make([]entryView, len(sec.Entries))}
		for j, e := range sec.Entries {
			views[i].Entries[j] = entryView{entry: e, Link: storyLink(web, e)}
		}
	}

	var proDay, proWeek string
	if proToken != "" {
		base := strings.TrimRight(web, "/") + "/pro/edition?token=" + url.QueryEscape(proToken)
		proDay, proWeek = base+"&day="+url.QueryEscape(ed.Day), base+"&scope=week"
	}

	var html strings.Builder
	_ = editionHTML.Execute(&html, page{ProDay: proDay, ProWeek: proWeek, Sponsor: sponsor, Date: date, Label: label, Intro: ed.Intro, Deep: deep, Sections: views, UnsubLink: unsub, ManageLink: manage, SwitchLink: switchLink, SwitchLabel: switchLabel, Footer: footer})

	var text strings.Builder
	text.WriteString("POSTPAPER\n" + date + " · " + label + "\n\n")
	if ed.Intro != "" {
		text.WriteString("Today in 60 seconds: " + ed.Intro + "\n\n")
	}
	for i, sec := range ed.Sections {
		text.WriteString(strings.ToUpper(sec.Title) + "\n\n")
		for j, e := range sec.Entries {
			text.WriteString(e.Title + "\n")
			if e.Why != "" {
				text.WriteString("Why it matters: " + e.Why + "\n")
			}
			switch {
			case deep && e.Long != "":
				text.WriteString(e.Long + "\n")
			case deep:
				for _, g := range e.Gist {
					text.WriteString("- " + g + "\n")
				}
			case e.Why == "" && len(e.Gist) > 0:
				text.WriteString(e.Gist[0] + "\n")
			}
			tags := ""
			for _, t := range e.Tags {
				tags += " #" + t
			}
			text.WriteString(e.Source + tags + "\n" + views[i].Entries[j].Link + "\n\n")
		}
	}
	if proDay != "" {
		text.WriteString("PRO. Save as PDF: today's edition " + proDay + "\nthe week " + proWeek + "\n\n")
	}
	if sponsor.Text != "" {
		text.WriteString("SPONSORED: " + sponsor.Text + " " + sponsor.URL + "\n\n")
	}
	text.WriteString("That is today's edition. You are caught up.\n\n")
	text.WriteString("You get this because you subscribed to Postpaper.\n" + switchLabel + ": " + switchLink + "\nManage your subscription (reading length, topics): " + manage + "\nUnsubscribe: " + unsub + "\n")
	if footer != "" {
		text.WriteString("\n" + footer + "\n")
	}

	subject := "Postpaper, " + date
	if frequency == track.Weekly {
		subject = "Postpaper this week, " + date
	}
	m := mail.Message{To: to, Subject: subject, HTML: html.String(), Text: text.String()}
	if api != "" {
		m.Headers = map[string]string{
			"List-Unsubscribe":      "<" + strings.TrimRight(api, "/") + "/unsubscribe?token=" + url.QueryEscape(token) + ">",
			"List-Unsubscribe-Post": "List-Unsubscribe=One-Click",
		}
	}

	return m
}
