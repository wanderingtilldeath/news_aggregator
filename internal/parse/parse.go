// Package parse extracts everything the crawler needs from an HTML document
// (title, description, h1, a text snippet, links and robots directives) in a
// single pass over one parsed tree.
package parse

import (
	"io"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"

	"github.com/wanderingtilldeath/news-aggregator/internal/urlnorm"
)

const (
	maxSnippetRunes = 500
	maxFieldRunes   = 300
)

// Page is the parsed view of one HTML document.
type Page struct {
	Title       string
	Description string
	H1          string
	// Snippet is the start of the visible body text, whitespace-collapsed.
	Snippet string
	// Links are canonical absolute URLs, deduplicated, in document order.
	Links []string
	// NoIndex and NoFollow come from <meta name="robots">.
	NoIndex  bool
	NoFollow bool
}

// Parser parses an HTML document. It is an interface so the crawler can be
// tested without parsing real HTML.
type Parser interface {
	Parse(body io.Reader, pageURL string) (Page, error)
}

// HTMLParser is the production Parser, built on golang.org/x/net/html.
type HTMLParser struct{}

// Parse reads body once. Relative links are resolved against <base href>
// when the document has one, otherwise against pageURL.
func (HTMLParser) Parse(body io.Reader, pageURL string) (Page, error) {
	doc, err := html.Parse(body)
	if err != nil {
		return Page{}, err
	}

	w := walker{}
	w.walk(doc)

	var page Page
	page.Title = truncate(collapse(w.title.String()), maxFieldRunes)
	page.H1 = truncate(collapse(w.h1.String()), maxFieldRunes)
	page.Description = truncate(collapse(w.description), maxSnippetRunes)
	page.Snippet = truncate(collapse(w.text.String()), maxSnippetRunes)
	page.NoIndex = w.noindex
	page.NoFollow = w.nofollow

	base := pageURL
	if w.baseHref != "" {
		if resolved, ok := urlnorm.Resolve(w.baseHref, pageURL); ok {
			base = resolved
		}
	}

	seen := make(map[string]struct{}, len(w.hrefs))
	for _, href := range w.hrefs {
		link, ok := urlnorm.Resolve(href, base)
		if !ok {
			continue
		}
		if _, dup := seen[link]; dup {
			continue
		}
		seen[link] = struct{}{}
		page.Links = append(page.Links, link)
	}

	return page, nil
}

type walker struct {
	title       strings.Builder
	h1          strings.Builder
	text        strings.Builder
	description string
	baseHref    string
	hrefs       []string
	noindex     bool
	nofollow    bool

	haveTitle, haveH1 bool
}

func (w *walker) walk(n *html.Node) {
	if n.Type == html.ElementNode {
		switch n.Data {
		case "script", "style", "noscript", "template":
			return
		case "a":
			if href, ok := attr(n, "href"); ok {
				w.hrefs = append(w.hrefs, href)
			}
		case "base":
			if href, ok := attr(n, "href"); ok && w.baseHref == "" {
				w.baseHref = strings.TrimSpace(href)
			}
		case "meta":
			w.meta(n)
		case "title":
			if !w.haveTitle {
				w.haveTitle = true
				collectText(n, &w.title)
				return
			}
		case "h1":
			if !w.haveH1 {
				w.haveH1 = true
				collectText(n, &w.h1)
			}
		}
	}

	if n.Type == html.TextNode && w.text.Len() < maxSnippetRunes*4 && insideBody(n) {
		w.text.WriteString(n.Data)
		w.text.WriteByte(' ')
	}

	for child := n.FirstChild; child != nil; child = child.NextSibling {
		w.walk(child)
	}
}

func (w *walker) meta(n *html.Node) {
	name, _ := attr(n, "name")
	content, _ := attr(n, "content")

	switch strings.ToLower(strings.TrimSpace(name)) {
	case "description":
		if w.description == "" {
			w.description = content
		}
	case "robots", "crawlerx":
		for _, token := range strings.Split(strings.ToLower(content), ",") {
			switch strings.TrimSpace(token) {
			case "noindex":
				w.noindex = true
			case "nofollow":
				w.nofollow = true
			case "none":
				w.noindex, w.nofollow = true, true
			}
		}
	}
}

func insideBody(n *html.Node) bool {
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Type == html.ElementNode {
			switch p.Data {
			case "body":
				return true
			case "head", "title":
				return false
			}
		}
	}
	return false
}

func collectText(n *html.Node, b *strings.Builder) {
	if n.Type == html.TextNode {
		b.WriteString(n.Data)
		b.WriteByte(' ')
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		collectText(child, b)
	}
}

func attr(n *html.Node, key string) (string, bool) {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val, true
		}
	}
	return "", false
}

func collapse(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func truncate(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes])
}
