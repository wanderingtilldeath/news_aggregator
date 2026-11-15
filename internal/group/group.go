// Package group finds stories that several sources cover, by comparing headlines. It uses no
// model: it needs no tokens and no network, so it can run on every crawl. Two stories from
// different sources are the same event when their headlines share enough distinctive words.
package group

import (
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

// Story is what the comparison needs of one article.
type Story struct {
	ID   string
	Host string
	Time time.Time
	// Title is the headline.
	Title string
}

const (
	// window is how far apart two stories can be published and still be the same event.
	window = 72 * time.Hour
	// minShared words must be shared, and at least minOverlap of the shorter headline's words, or
	// minJaccard of all the words together.
	minShared  = 3
	minOverlap = 0.5
	minJaccard = 0.45
)

var stop = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`the a an and or but of to in on at for with by from as is are was were be been it its this that
		these those has have had will would could should can may says say said after over into out up down new more most
		than then their there they them his her he she you your our we not no who what when where why how about amid against
		among across during before still just also one two`) {
		stop[w] = true
	}
}

// words returns the distinctive words of a headline: lower case, no stop words, at least three
// letters (or two for numbers), and the " | Site name" tail publishers add removed.
func words(title string) map[string]bool {
	if i := strings.Index(title, " | "); i >= 20 {
		title = title[:i]
	}
	set := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(title), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if stop[w] {
			continue
		}
		digits := strings.IndexFunc(w, unicode.IsDigit) >= 0
		if (digits && len(w) >= 2) || len([]rune(w)) >= 3 {
			set[stem(w)] = true
		}
	}

	return set
}

// stem folds the commonest English endings so "protests" and "protest" meet.
func stem(w string) string {
	for _, suf := range []string{"ing", "ed", "es", "s"} {
		if strings.HasSuffix(w, suf) && len(w)-len(suf) >= 4 {
			return strings.TrimSuffix(w, suf)
		}
	}

	return w
}

func similar(a, b map[string]bool) bool {
	shared := 0
	for w := range a {
		if b[w] {
			shared++
		}
	}
	if shared < minShared {
		return false
	}
	small := min(len(a), len(b))
	union := len(a) + len(b) - shared

	return float64(shared)/float64(small) >= minOverlap || float64(shared)/float64(union) >= minJaccard
}

// roundup matches a headline that is a digest of many events ("Ukraine war briefing: ...", "Live
// updates", a newsletter), not one event. Its headline borrows a topic of the day, so another
// outlet's story on that topic shares words with it by coincidence and the two are not the same story.
var roundup = regexp.MustCompile(`(?i)\b(briefing|round-?up|newsletter|as it happened)\b|\blive (blog|updates?|coverage)\b|^live\b`)

// Group places a story in an event: Story is the group's id (the smallest story id in it) and
// Sources the number of distinct hosts covering it.
type Group struct {
	Story   string
	Sources int
}

// Cluster returns a Group for every story that shares an event with a story from another source.
// Stories with no match are absent from the result.
func Cluster(stories []Story) map[string]Group {
	n := len(stories)
	sets := make([]map[string]bool, n)
	for i, s := range stories {
		sets[i] = words(s.Title)
	}

	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}

	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			a, b := stories[i], stories[j]
			if roundup.MatchString(a.Title) || roundup.MatchString(b.Title) || a.Host == b.Host || a.Time.Sub(b.Time).Abs() > window || !similar(sets[i], sets[j]) {
				continue
			}
			parent[find(i)] = find(j)
		}
	}

	members := map[int][]int{}
	for i := range stories {
		members[find(i)] = append(members[find(i)], i)
	}

	out := map[string]Group{}
	for _, idx := range members {
		hosts := map[string]bool{}
		ids := make([]string, 0, len(idx))
		for _, i := range idx {
			hosts[stories[i].Host] = true
			ids = append(ids, stories[i].ID)
		}
		if len(hosts) < 2 {
			continue // several stories from one source are not a story covered by several
		}
		sort.Strings(ids)
		for _, id := range ids {
			out[id] = Group{Story: ids[0], Sources: len(hosts)}
		}
	}

	return out
}
