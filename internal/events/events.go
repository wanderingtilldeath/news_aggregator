// Package events finds upcoming dates in article text ("the Reserve Bank meets on 4 December") with
// plain rules: no model and no network, so it costs nothing to run on every crawl. It is a hint for
// a calendar page, not a source of record: each event links back to the story it came from.
package events

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/prose"
)

// Found is a dated event in an article: the day it falls on and the sentence that mentions it.
type Found struct {
	Day  time.Time
	Text string
}

const (
	// maxPerArticle keeps one article from filling the calendar.
	maxPerArticle = 3
	// maxText is the longest sentence kept whole; a longer one is cut at a word.
	maxText = 260
)

const monthNames = `Jan(?:uary)?|Feb(?:ruary)?|Mar(?:ch)?|Apr(?:il)?|May|Jun(?:e)?|Jul(?:y)?|Aug(?:ust)?|Sep(?:t(?:ember)?)?|Oct(?:ober)?|Nov(?:ember)?|Dec(?:ember)?`

// A date is "4 December", "4th Dec" or "December 4" with an optional year. The month must be
// capitalised, so the verbs "may" and "march" do not count.
var dateRE = regexp.MustCompile(`\b(?:(\d{1,2})(?:st|nd|rd|th)?\s+(` + monthNames + `)\b\.?|(` + monthNames + `)\b\.?\s+(\d{1,2})(?:st|nd|rd|th)?\b)(?:,?\s+(20\d{2}))?`)

// pastCue marks sentences about something that already happened or a person's dates, which an
// upcoming date inside them would only be a coincidence of.
var pastCue = regexp.MustCompile(`(?i)\b(born|died|birthday|anniversary)\b`)

var months = map[string]time.Month{
	"jan": time.January, "feb": time.February, "mar": time.March, "apr": time.April, "may": time.May, "jun": time.June,
	"jul": time.July, "aug": time.August, "sep": time.September, "oct": time.October, "nov": time.November, "dec": time.December,
}

func monthOf(name string) time.Month { return months[strings.ToLower(name)[:3]] }

// Extract returns the dates in content that fall within horizonDays after the day the article was
// published (the article's own day counts), with the sentence that holds each. A date with no year
// is the next time it comes round after the publication day, so "12 November" in an article of 15
// November is next year and is left out, and "5 January" in one of 20 December is kept.
func Extract(content string, published time.Time, horizonDays int) []Found {
	day := time.Date(published.Year(), published.Month(), published.Day(), 0, 0, 0, 0, time.UTC)
	last := day.AddDate(0, 0, horizonDays)

	var out []Found
	seen := map[string]bool{}
	for _, s := range prose.Sentences(prose.DropBoilerplate(content)) {
		if len(s) < 30 || prose.Fragment(s) || pastCue.MatchString(s) {
			continue
		}
		for _, m := range dateRE.FindAllStringSubmatch(s, -1) {
			dd, mm := m[1], m[2]
			if dd == "" {
				dd, mm = m[4], m[3]
			}
			d, _ := strconv.Atoi(dd)
			when, ok := resolve(d, monthOf(mm), m[5], day)
			if !ok || when.Before(day) || when.After(last) {
				continue
			}
			key := when.Format("2006-01-02") + "|" + s
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, Found{Day: when, Text: clip(s)})
			if len(out) == maxPerArticle {
				return out
			}
		}
	}

	return out
}

// resolve turns day, month and an optional year into a date: the given year, or else the first
// time that day comes round on or after the publication day. ok is false for a day that does not
// exist (30 February).
func resolve(d int, month time.Month, year string, published time.Time) (time.Time, bool) {
	valid := func(y int) (time.Time, bool) {
		t := time.Date(y, month, d, 0, 0, 0, 0, time.UTC)
		return t, d >= 1 && t.Month() == month && t.Day() == d
	}
	if year != "" {
		y, _ := strconv.Atoi(year)
		return valid(y)
	}
	t, ok := valid(published.Year())
	if ok && t.Before(published) {
		t, ok = valid(published.Year() + 1)
	}

	return t, ok
}

func clip(s string) string {
	if len(s) <= maxText {
		return s
	}
	cut := strings.LastIndex(s[:maxText], " ")
	if cut < maxText/2 {
		cut = maxText
	}

	return strings.TrimRight(s[:cut], ",;: ") + "…"
}
