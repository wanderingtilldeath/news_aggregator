// Package digestmail builds the daily email edition and sends it to the confirmed
// subscribers, at most once per subscriber per day.
package digestmail

import (
	"html/template"
	"net/url"
	"strings"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/mail"
	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

// maxPerSource keeps one busy site from filling the whole edition.
const maxPerSource = 3

// gistRunes caps a fallback description when an article has no summary bullets.
const gistRunes = 220

type entry struct {
	Title, URL, Source, Category string
	Gist                         []string
}

type edition struct {
	Date      string
	Entries   []entry
	UnsubLink string
	Footer    string
}

var editionHTML = template.Must(template.New("edition").Parse(`<!doctype html>
<html><body style="font-family:system-ui,-apple-system,sans-serif;max-width:600px;margin:0 auto;padding:16px;color:#111;line-height:1.5">
<h1 style="font-family:Georgia,serif;margin:0">The Daily Digest</h1>
<p style="color:#5f6670;margin:4px 0 20px">{{.Date}}</p>
{{range .Entries}}<div style="margin:0 0 20px;padding-bottom:16px;border-bottom:1px solid #dcdfe3">
<div style="color:#c8102e;font-size:12px;font-weight:700;text-transform:uppercase">{{.Category}}</div>
<h2 style="font-family:Georgia,serif;font-size:19px;margin:2px 0 6px"><a href="{{.URL}}" style="color:#111;text-decoration:none">{{.Title}}</a></h2>
{{if .Gist}}<ul style="margin:0 0 6px;padding-left:18px">{{range .Gist}}<li>{{.}}</li>{{end}}</ul>{{end}}
<div style="color:#5f6670;font-size:12px">{{.Source}}</div>
</div>
{{end}}<p style="color:#5f6670;font-size:13px">That is today's edition. You are caught up.</p>
<p style="color:#5f6670;font-size:12px">You get this because you subscribed to The Daily Digest. Every headline links to the original publisher.<br>
<a href="{{.UnsubLink}}">Unsubscribe</a></p>
{{if .Footer}}<p style="color:#5f6670;font-size:12px">{{.Footer}}</p>{{end}}
</body></html>`))

// pick chooses up to top articles, newest first (items arrive newest first),
// at most maxPerSource per site, and only those whose link is http(s).
func pick(items []store.Item, top int) []entry {
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

		e := entry{Title: strings.TrimSpace(it.Title), URL: it.URL, Source: strings.TrimPrefix(it.Source, "www."), Category: it.Category}
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

// render builds one subscriber's email. api is the public API address for the
// one-click unsubscribe header; empty leaves the header out.
func render(entries []entry, day, to, token, web, api, footer string) mail.Message {
	date := day
	if t, err := time.Parse("2006-01-02", day); err == nil {
		date = t.Format("Monday 2 January 2006")
	}
	unsub := strings.TrimRight(web, "/") + "/unsubscribe?token=" + url.QueryEscape(token)

	ed := edition{Date: date, Entries: entries, UnsubLink: unsub, Footer: footer}
	var html strings.Builder
	_ = editionHTML.Execute(&html, ed)

	var text strings.Builder
	text.WriteString("THE DAILY DIGEST\n" + date + "\n\n")
	for _, e := range entries {
		text.WriteString(strings.ToUpper(e.Category) + "\n" + e.Title + "\n")
		for _, g := range e.Gist {
			text.WriteString("- " + g + "\n")
		}
		text.WriteString(e.Source + "\n" + e.URL + "\n\n")
	}
	text.WriteString("That is today's edition. You are caught up.\n\n")
	text.WriteString("You get this because you subscribed to The Daily Digest.\nUnsubscribe: " + unsub + "\n")
	if footer != "" {
		text.WriteString("\n" + footer + "\n")
	}

	m := mail.Message{To: to, Subject: "The Daily Digest, " + date, HTML: html.String(), Text: text.String()}
	if api != "" {
		m.Headers = map[string]string{
			"List-Unsubscribe":      "<" + strings.TrimRight(api, "/") + "/unsubscribe?token=" + url.QueryEscape(token) + ">",
			"List-Unsubscribe-Post": "List-Unsubscribe=One-Click",
		}
	}

	return m
}
