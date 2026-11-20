// Package track defines Postpaper's two editions, which readers choose between
// at signup, and the two reading modes. A track is a set of source categories.
package track

import "strings"

const (
	UPSC        = "upsc"
	Engineering = "engineering"
	// General is the one edition of all the top news, which the daily email carries. It is not a track a
	// reader chooses (Valid is false for it): it has a title and an intro, and no categories of its own.
	General = "top"

	// Mode1 is the one-minute read: headlines with a one-line "why it matters".
	// Mode7 is the seven-minute read: gist bullets, the longer summary and topic tags.
	Mode1 = "1min"
	Mode7 = "7min"
)

var categories = map[string][]string{
	UPSC:        {"india", "world", "upsc", "editorial"},
	Engineering: {"tech", "ai"},
}

var titles = map[string]string{
	UPSC:        "UPSC current affairs",
	Engineering: "Engineering",
	General:     "Top news",
}

// All lists the tracks in the order they appear.
func All() []string { return []string{UPSC, Engineering} }

// Valid reports whether t is a track.
func Valid(t string) bool { _, ok := categories[t]; return ok }

// Categories returns the source categories of a track (nil for an unknown one).
func Categories(t string) []string { return append([]string(nil), categories[t]...) }

// Title is the track's heading in emails and pages.
func Title(t string) string { return titles[t] }

// ValidMode reports whether m is a reading mode.
func ValidMode(m string) bool { return m == Mode1 || m == Mode7 }

const (
	// MaxTopics is how many topics a subscriber can follow, and MaxTopicLen the longest one.
	MaxTopics   = 5
	MaxTopicLen = 40
)

// NormalizeTopics returns the topics trimmed, lower-cased, without blanks, duplicates or
// the ones over MaxTopicLen, and at most MaxTopics of them.
func NormalizeTopics(topics []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, t := range topics {
		t = strings.ToLower(strings.Join(strings.Fields(t), " "))
		if t == "" || len([]rune(t)) > MaxTopicLen || seen[t] || len(out) >= MaxTopics {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}

	return out
}

// Normalize returns the tracks without duplicates or unknown names, in the
// standard order, and the mode; an empty or invalid choice becomes every track
// and the one-minute read.
func Normalize(tracks []string, mode string) ([]string, string) {
	want := map[string]bool{}
	for _, t := range tracks {
		if Valid(t) {
			want[t] = true
		}
	}

	var out []string
	for _, t := range All() {
		if want[t] {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		out = All()
	}
	if !ValidMode(mode) {
		mode = Mode1
	}

	return out, mode
}
