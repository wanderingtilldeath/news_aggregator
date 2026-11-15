// Package digestmail builds the daily email edition and sends it to the confirmed
// subscribers, at most once per subscriber per day. A subscriber chooses tracks
// (see package track) and a reading length: a one-minute read of headlines with a
// "why it matters" line, or a seven-minute read with longer summaries.
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
	// briefMax is the most stories per track in the one-minute read.
	briefMax = 5
	// gistRunes caps a fallback description when an article has no summary bullets.
	gistRunes = 220
	// unrankedScore is what an article the enrichment pass has not scored counts as.
	unrankedScore = 2
)

type entry struct {
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
	Day      string
	Mode     string
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
	sort.SliceStable(items, func(i, j int) bool {
		if si, sj := score(items[i]), score(items[j]); si != sj {
			return si > sj
		}
		return items[i].Time.After(items[j].Time)
	})

	perSource := map[string]int{}
	var out []entry
	for _, it := range items {
		if len(out) >= top {
			break
		}
		u, err := url.Parse(it.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || strings.TrimSpace(it.Title) == "" {
			continue
		}
		if perSource[it.Source] >= maxPerSource {
			continue
		}
		perSource[it.Source]++

		e := entry{
			Title: strings.TrimSpace(it.Title), URL: it.URL, Source: strings.TrimPrefix(it.Source, "www."), Category: it.Category,
			Why: strings.TrimSpace(it.Why), Tags: it.Tags, Long: strings.TrimSpace(it.Long),
		}
		if e.Category == "" {
			e.Category = "other"
		}
		switch {
		case len(it.Summary) > 0:
			e.Gist = it.Summary
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

type quizView struct {
	Link      string
	Questions []store.QuizQuestion
}

type sectionView struct {
	Title   string
	Intro   string
	Entries []entry
	Quiz    *quizView
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
	Sponsor   sponsorLine
	Date      string
	Deep      bool
	Sections  []sectionView
	UnsubLink string
	Footer    string
}

var editionHTML = template.Must(template.New("edition").Parse(`<!doctype html>
<html><body style="font-family:system-ui,-apple-system,sans-serif;max-width:600px;margin:0 auto;padding:16px;color:#111;line-height:1.5">
<h1 style="font-family:Georgia,serif;margin:0">Postpaper</h1>
<p style="color:#5f6670;margin:4px 0 20px">{{.Date}} · {{if .Deep}}7-minute read{{else}}1-minute read{{end}}</p>
{{range .Sections}}<h2 style="font-size:13px;text-transform:uppercase;letter-spacing:.6px;border-bottom:2px solid #111;padding-bottom:4px;margin:24px 0 12px">{{.Title}}</h2>
{{with .Intro}}<p style="margin:0 0 16px;padding:10px 12px;background:#f4f5f7;border-left:3px solid #c8213a"><b>Today in 60 seconds.</b> {{.}}</p>
{{end}}{{$deep := $.Deep}}{{range .Entries}}<div style="margin:0 0 18px;padding-bottom:14px;border-bottom:1px solid #dcdfe3">
<h3 style="font-family:Georgia,serif;font-size:18px;margin:0 0 4px"><a href="{{.URL}}" style="color:#111;text-decoration:none">{{.Title}}</a></h3>
{{if .Why}}<p style="margin:0 0 6px;color:#c8213a"><b>Why it matters:</b> {{.Why}}</p>{{end}}
{{if $deep}}{{if .Long}}<p style="margin:0 0 6px">{{.Long}}</p>{{else if .Gist}}<ul style="margin:0 0 6px;padding-left:18px">{{range .Gist}}<li>{{.}}</li>{{end}}</ul>{{end}}{{else if not .Why}}{{if .Gist}}<p style="margin:0 0 6px">{{index .Gist 0}}</p>{{end}}{{end}}
<div style="color:#5f6670;font-size:12px">{{.Source}}{{range .Tags}} · #{{.}}{{end}}</div>
</div>
{{end}}{{with .Quiz}}<div style="background:#f4f5f7;padding:12px 14px;border-radius:4px;margin:0 0 18px">
<p style="margin:0 0 6px"><b>Today's quiz</b> · {{len .Questions}} questions · <a href="{{.Link}}">check your answers</a></p>
{{if $deep}}<ol style="margin:0;padding-left:20px">{{range .Questions}}<li style="margin:0 0 8px">{{.Question}}<br><span style="color:#5f6670;font-size:13px">{{range $i, $o := .Options}}{{if $i}} · {{end}}{{$o}}{{end}}</span></li>{{end}}</ol>{{end}}
</div>{{end}}{{end}}{{with .Sponsor.Text}}<p style="margin:0 0 14px;padding:8px 12px;border:1px dashed #b8bdc4;font-size:13px"><span style="text-transform:uppercase;letter-spacing:.6px;font-size:11px;color:#5f6670">Sponsored</span> &nbsp;<a href="{{$.Sponsor.URL}}" rel="sponsored" style="color:#111">{{.}}</a></p>
{{end}}<p style="color:#5f6670;font-size:13px">That is today's edition. You are caught up.</p>
<p style="color:#5f6670;font-size:12px">You get this because you subscribed to Postpaper. Every headline links to the original publisher.<br>
<a href="{{.UnsubLink}}">Unsubscribe</a></p>
{{if .Footer}}<p style="color:#5f6670;font-size:12px">{{.Footer}}</p>{{end}}
</body></html>`))

// render builds one subscriber's email. api is the public API address for the
// one-click unsubscribe header; empty leaves the header out.
func render(ed edition, to, token, web, api, footer string, sponsor sponsorLine) mail.Message {
	date := ed.Day
	if t, err := time.Parse("2006-01-02", ed.Day); err == nil {
		date = t.Format("Monday 2 January 2006")
	}
	deep := ed.Mode == track.Mode7
	unsub := strings.TrimRight(web, "/") + "/unsubscribe?token=" + url.QueryEscape(token)

	quizLink := func(sec section) string {
		return strings.TrimRight(web, "/") + "/quiz?track=" + url.QueryEscape(sec.Track) + "&day=" + url.QueryEscape(ed.Day)
	}
	views := make([]sectionView, len(ed.Sections))
	for i, sec := range ed.Sections {
		views[i] = sectionView{Title: sec.Title, Intro: sec.Intro, Entries: sec.Entries}
		if sec.Quiz != nil {
			views[i].Quiz = &quizView{Link: quizLink(sec), Questions: sec.Quiz.Questions}
		}
	}

	var html strings.Builder
	_ = editionHTML.Execute(&html, page{Sponsor: sponsor, Date: date, Deep: deep, Sections: views, UnsubLink: unsub, Footer: footer})

	var text strings.Builder
	read := "1-minute read"
	if deep {
		read = "7-minute read"
	}
	text.WriteString("POSTPAPER\n" + date + " · " + read + "\n\n")
	for _, sec := range ed.Sections {
		text.WriteString(strings.ToUpper(sec.Title) + "\n\n")
		if sec.Intro != "" {
			text.WriteString("Today in 60 seconds: " + sec.Intro + "\n\n")
		}
		for _, e := range sec.Entries {
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
			text.WriteString(e.Source + tags + "\n" + e.URL + "\n\n")
		}
		if sec.Quiz != nil {
			text.WriteString(fmt.Sprintf("TODAY'S QUIZ (%d questions): %s\n", len(sec.Quiz.Questions), quizLink(sec)))
			if deep {
				for i, q := range sec.Quiz.Questions {
					text.WriteString(fmt.Sprintf("%d. %s\n   %s\n", i+1, q.Question, strings.Join(q.Options, " | ")))
				}
			}
			text.WriteString("\n")
		}
	}
	if sponsor.Text != "" {
		text.WriteString("SPONSORED: " + sponsor.Text + " " + sponsor.URL + "\n\n")
	}
	text.WriteString("That is today's edition. You are caught up.\n\n")
	text.WriteString("You get this because you subscribed to Postpaper.\nUnsubscribe: " + unsub + "\n")
	if footer != "" {
		text.WriteString("\n" + footer + "\n")
	}

	m := mail.Message{To: to, Subject: "Postpaper, " + date, HTML: html.String(), Text: text.String()}
	if api != "" {
		m.Headers = map[string]string{
			"List-Unsubscribe":      "<" + strings.TrimRight(api, "/") + "/unsubscribe?token=" + url.QueryEscape(token) + ">",
			"List-Unsubscribe-Post": "List-Unsubscribe=One-Click",
		}
	}

	return m
}
