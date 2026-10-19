package parse

import (
	"strings"

	"golang.org/x/net/html"
)

// Section is the text that sits under one heading in the main content.
type Section struct {
	// Heading is the h1 > h2 > h3 path, e.g. "Install > Linux". Empty for text
	// before the first heading.
	Heading string
	// Text is paragraphs separated by a blank line.
	Text string
}

// skipped never contributes text: page chrome and non-content elements.
var skipped = map[string]bool{
	"script": true, "style": true, "noscript": true, "template": true, "svg": true,
	"nav": true, "header": true, "footer": true, "aside": true, "form": true,
}

// blocks end the current paragraph.
var blocks = map[string]bool{
	"p": true, "li": true, "div": true, "section": true, "article": true,
	"tr": true, "br": true, "blockquote": true, "dt": true, "dd": true,
	"table": true, "ul": true, "ol": true, "h4": true, "h5": true, "h6": true,
}

type sectionBuilder struct {
	path     [3]string // headings for h1..h3
	heading  string
	para     strings.Builder
	paras    []string
	sections []Section
}

// extractSections returns the main content split at h1-h3. The root is the
// first <main>, else the first <article>, else <body>.
func extractSections(doc *html.Node) []Section {
	root := find(doc, "main")
	if root == nil {
		root = find(doc, "article")
	}
	if root == nil {
		root = find(doc, "body")
	}
	if root == nil {
		return nil
	}
	b := &sectionBuilder{}
	b.walk(root)
	b.closeSection()
	return b.sections
}

func (b *sectionBuilder) walk(n *html.Node) {
	switch n.Type {
	case html.TextNode:
		b.para.WriteString(n.Data)
		b.para.WriteByte(' ')
		return
	case html.ElementNode:
		if skipped[n.Data] {
			return
		}
		switch n.Data {
		case "h1", "h2", "h3":
			b.closeSection()
			var text strings.Builder
			collectText(n, &text)
			b.setHeading(int(n.Data[1]-'1'), collapse(text.String()))
			return
		case "pre":
			// Code keeps its line breaks; collapsing would destroy it.
			b.endPara()
			var text strings.Builder
			collectRaw(n, &text)
			if code := strings.TrimSpace(text.String()); code != "" {
				b.paras = append(b.paras, code)
			}
			return
		}
	}

	for child := n.FirstChild; child != nil; child = child.NextSibling {
		b.walk(child)
	}
	if n.Type == html.ElementNode && blocks[n.Data] {
		b.endPara()
	}
}

func (b *sectionBuilder) setHeading(level int, text string) {
	b.path[level] = text
	for i := level + 1; i < len(b.path); i++ {
		b.path[i] = ""
	}
	var parts []string
	for _, p := range b.path {
		if p != "" {
			parts = append(parts, p)
		}
	}
	b.heading = strings.Join(parts, " > ")
}

func (b *sectionBuilder) endPara() {
	if p := collapse(b.para.String()); p != "" {
		b.paras = append(b.paras, p)
	}
	b.para.Reset()
}

func (b *sectionBuilder) closeSection() {
	b.endPara()
	if len(b.paras) > 0 {
		b.sections = append(b.sections, Section{Heading: b.heading, Text: strings.Join(b.paras, "\n\n")})
	}
	b.paras = nil
}

func find(n *html.Node, tag string) *html.Node {
	if n.Type == html.ElementNode && n.Data == tag {
		return n
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if f := find(child, tag); f != nil {
			return f
		}
	}
	return nil
}

func collectRaw(n *html.Node, b *strings.Builder) {
	if n.Type == html.TextNode {
		b.WriteString(n.Data)
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		collectRaw(child, b)
	}
}
