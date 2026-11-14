package parse

import (
	"regexp"
	"strings"
)

// liveBlogTitle matches the titles of live blogs and minute-by-minute pages,
// whose body is a stream of short updates that make no readable gist.
var liveBlogTitle = regexp.MustCompile(`(?i)\bas it happened\b|\blive (updates?|blog|report|coverage)\b|\bminute[- ]by[- ]minute\b|^live\b|\blive:`)

// LooksLikeLiveBlog reports whether the page is a live blog: its title or first
// heading says so, or its text says the blog is closed. Such pages stay out of
// the digest.
func (p Page) LooksLikeLiveBlog() bool {
	if liveBlogTitle.MatchString(p.Title) || liveBlogTitle.MatchString(p.H1) {
		return true
	}

	return strings.Contains(strings.ToLower(p.Snippet+" "+p.Description), "this blog is now closed")
}
