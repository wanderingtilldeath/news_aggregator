// Package compile builds the monthly current-affairs compilation: the month's most important
// stories of an edition, grouped by syllabus area. It is built a little each day, by merging that
// day's top stories into what the month already holds, so it needs no model and survives the
// deletion of old pages.
package compile

import (
	"net/url"
	"sort"
	"strings"
	"unicode"

	"github.com/wanderingtilldeath/news-aggregator/internal/store"
	"github.com/wanderingtilldeath/news-aggregator/internal/track"
)

const (
	// MinImportance is the lowest score that makes a story a candidate: a compilation holds the
	// stories worth revising, not every story.
	MinImportance = 3
	// PerArea caps one area, so a busy area cannot crowd out the rest.
	PerArea = 12
	// maxBullets keeps each story to the first few gist bullets.
	maxBullets = 3
)

// upscAreas are the syllabus areas the enrichment pass puts first in a UPSC story's tags, in the
// order a compilation lists them, with the names readers see.
var upscAreas = []struct{ tag, name string }{
	{"gs1-history-society", "History and society (GS1)"},
	{"gs2-polity-governance", "Polity and governance (GS2)"},
	{"gs2-international-relations", "International relations (GS2)"},
	{"gs3-economy", "Economy (GS3)"},
	{"gs3-environment", "Environment (GS3)"},
	{"gs3-science-tech", "Science and technology (GS3)"},
	{"gs3-security", "Security (GS3)"},
	{"gs4-ethics", "Ethics (GS4)"},
	{"essay", "Essay"},
}

const other = "Other"

// areaOf names the area a story belongs to in an edition: a syllabus area for the UPSC edition
// (from its first tag), the first tag as a topic name for the others, else "Other".
func areaOf(trackName string, tags []string) string {
	if len(tags) == 0 {
		return other
	}
	if trackName == track.UPSC {
		for _, a := range upscAreas {
			if tags[0] == a.tag {
				return a.name
			}
		}

		return other
	}

	words := strings.Fields(strings.ReplaceAll(tags[0], "-", " "))
	if len(words) == 0 {
		return other
	}
	r := []rune(words[0])
	words[0] = string(unicode.ToUpper(r[0])) + string(r[1:])

	return strings.Join(words, " ")
}

// order ranks area names for listing: the syllabus order for the UPSC edition, then alphabetical,
// with "Other" last.
func order(trackName, name string) (int, string) {
	if name == other {
		return 1 << 20, name
	}
	if trackName == track.UPSC {
		for i, a := range upscAreas {
			if a.name == name {
				return i, name
			}
		}
	}

	return 1 << 10, name
}

// item turns a story into its compilation entry; false when it should not be listed (not scored
// high enough, no title, or a link that is not http or https).
func item(it store.Item) (store.CompItem, bool) {
	u, err := url.Parse(it.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || strings.TrimSpace(it.Title) == "" || it.Importance < MinImportance {
		return store.CompItem{}, false
	}
	bullets := it.Summary
	if len(bullets) > maxBullets {
		bullets = bullets[:maxBullets]
	}

	return store.CompItem{
		ID: it.ID, URL: it.URL, Title: strings.TrimSpace(it.Title), Source: strings.TrimPrefix(it.Source, "www."),
		Why: strings.TrimSpace(it.Why), Bullets: bullets, Tags: it.Tags, Importance: it.Importance, Time: it.Time,
	}, true
}

// Merge adds the candidate stories to the areas the month already has and returns the result: a
// story already listed is kept once (the new copy wins, as it may have been enriched since), each
// area keeps its perArea most important stories (newest first among equals), and the areas come in
// reading order.
func Merge(existing []store.CompArea, items []store.Item, trackName string, perArea int) []store.CompArea {
	byArea := map[string]map[string]store.CompItem{}
	add := func(area string, c store.CompItem) {
		if byArea[area] == nil {
			byArea[area] = map[string]store.CompItem{}
		}
		byArea[area][c.ID] = c
	}
	for _, a := range existing {
		for _, c := range a.Items {
			add(a.Name, c)
		}
	}
	for _, it := range items {
		if c, ok := item(it); ok {
			area := areaOf(trackName, it.Tags)
			// A story that moved area (its tags changed) must not be listed twice.
			for name, set := range byArea {
				if name != area {
					delete(set, c.ID)
				}
			}
			add(area, c)
		}
	}

	var out []store.CompArea
	for name, set := range byArea {
		list := make([]store.CompItem, 0, len(set))
		for _, c := range set {
			list = append(list, c)
		}
		sort.Slice(list, func(i, j int) bool {
			if list[i].Importance != list[j].Importance {
				return list[i].Importance > list[j].Importance
			}
			if !list[i].Time.Equal(list[j].Time) {
				return list[i].Time.After(list[j].Time)
			}
			return list[i].ID < list[j].ID
		})
		if perArea > 0 && len(list) > perArea {
			list = list[:perArea]
		}
		if len(list) > 0 {
			out = append(out, store.CompArea{Name: name, Items: list})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		oi, ni := order(trackName, out[i].Name)
		oj, nj := order(trackName, out[j].Name)
		if oi != oj {
			return oi < oj
		}
		return ni < nj
	})

	return out
}
