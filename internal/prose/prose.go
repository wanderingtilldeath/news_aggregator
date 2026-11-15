// Package prose holds the small text helpers that read article text: splitting it into sentences
// (aware of abbreviations such as "Dr." and "U.S."), dropping page chrome that sticks to it, and
// taking its opening sentences whole. The summariser and the store both use it.
package prose

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// abbreviations end in a full stop without ending a sentence ("Mr. Rao", "Dr. Singh", "St. Louis").
var abbreviations = map[string]bool{
	"mr": true, "mrs": true, "ms": true, "dr": true, "prof": true, "sr": true, "jr": true, "st": true, "gen": true,
	"lt": true, "col": true, "capt": true, "hon": true, "no": true, "vs": true, "etc": true, "rs": true, "inc": true,
	"ltd": true, "co": true, "corp": true, "govt": true, "dept": true, "approx": true, "jan": true, "feb": true,
	"mar": true, "apr": true, "jun": true, "jul": true, "aug": true, "sep": true, "sept": true, "oct": true,
	"nov": true, "dec": true,
}

// endsSentence reports whether the full stop at runes[i] ends a sentence. It does not after an
// abbreviation ("Dr."), initials ("J." or "U.S."), or when the next word does not start like a new
// sentence (a dash, a lower-case letter).
func endsSentence(runes []rune, i int) bool {
	if i+1 == len(runes) {
		return true
	}
	if runes[i+1] != ' ' {
		return false
	}

	start := i
	for start > 0 && runes[start-1] != ' ' {
		start--
	}
	word := strings.Trim(string(runes[start:i]), "\"'([")
	lower := strings.ToLower(word)
	switch {
	case abbreviations[lower]:
		return false
	case len(word) == 1 && unicode.IsUpper([]rune(word)[0]): // an initial: "J. Jaishankar"
		return false
	case strings.Contains(word, ".") && len([]rune(word)) <= 6: // "U.S", "U.K", "e.g", "i.e"
		next := nextWordRune(runes, i+1)
		return unicode.IsUpper(next) || unicode.IsDigit(next)
	}

	next := nextWordRune(runes, i+1)

	return unicode.IsUpper(next) || unicode.IsDigit(next) || strings.ContainsRune("\"'“‘([", next) || next == 0
}

// nextWordRune is the first rune after the spaces from i, or 0 at the end of the text.
func nextWordRune(runes []rune, i int) rune {
	for i < len(runes) && runes[i] == ' ' {
		i++
	}
	if i >= len(runes) {
		return 0
	}

	return runes[i]
}

// Sentences splits on a full stop, "?" or "!" (and the Devanagari danda) that ends a sentence, which
// is good enough for a gist: see endsSentence for the full stops that do not.
func Sentences(text string) []string {
	text = strings.Join(strings.Fields(text), " ")

	var out []string
	start := 0
	runes := []rune(text)
	for i, r := range runes {
		var end bool
		switch r {
		case '.':
			end = endsSentence(runes, i)
		case '?', '!', '।':
			end = i+1 == len(runes) || runes[i+1] == ' '
		}
		if end {
			out = append(out, strings.TrimSpace(string(runes[start:i+1])))
			start = i + 1
		}
	}
	if start < len(runes) {
		out = append(out, strings.TrimSpace(string(runes[start:])))
	}

	return out
}

// boilerplate matches page chrome that article text picks up: paywall and login
// prompts, newsletter sign-ups, the navigation bar and video labels.
var boilerplate = regexp.MustCompile(`(?i)subscri(be|ption|ber)|^login\b|log in\b|sign up\b|skip links|skip to content|navigation menu|inbox|newsletter|get audio access|advertisement|video duration|read more|this article appears in the|this blog is now closed`)

// leadingLogin strips the "LOGIN " button label that sticks to the first sentence.
var leadingLogin = regexp.MustCompile(`^(?i:login)\s+`)

// DropBoilerplate removes sentences that are page chrome, not article text.
func DropBoilerplate(text string) string {
	var kept []string
	for _, s := range Sentences(text) {
		s = leadingLogin.ReplaceAllString(strings.TrimSpace(s), "")
		if s == "" || boilerplate.MatchString(s) {
			continue
		}
		kept = append(kept, s)
	}
	return strings.Join(kept, " ")
}

// Fragment reports a "sentence" that is the tail of a split one: it starts with a dash, a comma or
// a lower-case letter, as in "— are expected to meet in Manila".
func Fragment(s string) bool {
	r, _ := utf8.DecodeRuneInString(strings.TrimSpace(s))

	return strings.ContainsRune("—–-,;:)", r) || unicode.IsLower(r)
}

// Lead returns the opening sentences of an article, whole: up to maxSentences of them, and it stops
// adding sentences once maxWords words are reached (the first sentence is always kept whole). Page
// chrome, fragments and very short sentences are skipped. It is what the story page shows when a
// story has no gist worth showing, so the reader never meets a sentence cut off in the middle.
func Lead(content string, maxSentences, maxWords int) string {
	var out []string
	words := 0
	for _, s := range Sentences(DropBoilerplate(content)) {
		if len(s) < 40 || Fragment(s) {
			continue
		}
		n := len(strings.Fields(s))
		if len(out) > 0 && words+n > maxWords {
			break
		}
		out = append(out, s)
		words += n
		if len(out) == maxSentences {
			break
		}
	}

	return strings.Join(out, " ")
}
