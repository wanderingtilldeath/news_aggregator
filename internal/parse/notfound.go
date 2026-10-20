package parse

import (
	"net/url"
	"path"
	"strings"
)

// notFoundTitles are titles an error page gives itself while the server answers
// 200 (a "soft 404"). A title matches when it equals one of these or starts with
// one followed by a separator, so "Not Found | Drishti IAS" counts and
// "Why features are not found by users" does not.
var notFoundTitles = []string{
	"not found",
	"page not found",
	"404",
	"404 not found",
	"oops",
	"we couldn't find",
	"we couldn’t find",
	"page unavailable",
}

// notFoundImages are the picture file names error pages show.
var notFoundImages = map[string]bool{"error.png": true, "404.png": true}

// LooksLikeNotFound reports whether the page is an error page served with a 200
// status: the title or first heading says "not found" (or similar), or the
// article picture is an error graphic. Short content alone is not a signal,
// because infographic pages carry their content in the image.
func (p Page) LooksLikeNotFound() bool {
	return notFoundTitle(p.Title) || notFoundTitle(p.H1) || notFoundImage(p.BodyImage)
}

func notFoundTitle(title string) bool {
	t := strings.ToLower(strings.TrimSpace(title))
	if t == "" {
		return false
	}

	for _, want := range notFoundTitles {
		if t == want {
			return true
		}

		if rest, ok := strings.CutPrefix(t, want); ok && strings.ContainsRune(" |-–—:!.", rune(rest[0])) {
			return true
		}
	}

	return false
}

func notFoundImage(raw string) bool {
	u, err := url.Parse(raw)
	if raw == "" || err != nil {
		return false
	}

	return notFoundImages[strings.ToLower(path.Base(u.Path))]
}
