// Package parse extracts everything the crawler needs from an HTML document
// (title, description, h1, a text snippet, links and robots directives) in a
// single pass over one parsed tree.
package parse

import (
	"encoding/json"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/net/html"

	"github.com/wanderingtilldeath/news-aggregator/internal/urlnorm"
)

const (
	maxSnippetRunes = 500
	maxFieldRunes   = 300
	// maxContentRunes caps the main text kept for summarisation.
	maxContentRunes = 8000
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
	// Published is the article date from page metadata; zero when absent.
	Published time.Time
	// Sections is the main content (no nav/header/footer) split at h1-h3.
	Sections []Section
	// Image is the absolute URL of the page's lead image (og:image or
	// twitter:image); empty when absent.
	Image string
	// IsArticle is true when the page declares og:type=article or carries a
	// published date, which separates articles from listing pages.
	IsArticle bool
	// Content is the main text, whitespace-collapsed and capped, for summaries.
	Content string
	// Blocks are the separate topics of a page that holds several
	// <article id="..."> elements; empty for an ordinary page.
	Blocks []Block
	// SourceURL is the first outside link inside the article's paragraphs: the
	// original story a summary site cites. Empty when there is none.
	SourceURL string
	// BodyImage is the first real picture inside the article itself (not the
	// page chrome, logos, icons, avatars, banners or SVG): the fallback when
	// the page's og:image is missing or a shared site logo.
	BodyImage string
}

// Block is one topic inside a multi-article page, addressable as pageURL#ID.
type Block struct {
	ID      string
	Title   string
	Content string
	// Image is the topic's own picture (first <img> inside it); empty if none.
	Image string
	// SourceURL is the original news article the topic cites, if any.
	SourceURL string
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
	// Prefer the article text over the page start, which is mostly menus.
	snippet := w.text.String()
	page.Sections = extractSections(doc)
	if sections := page.Sections; len(sections) > 0 {
		var b strings.Builder
		for _, s := range sections {
			if b.Len() > maxSnippetRunes*4 {
				break
			}
			b.WriteString(s.Text)
			b.WriteByte(' ')
		}
		snippet = b.String()
	}
	page.Snippet = truncate(collapse(snippet), maxSnippetRunes)
	page.Content = truncate(collapse(articleText(doc, page.Sections, w.text.String())), maxContentRunes)
	page.NoIndex = w.noindex
	page.NoFollow = w.nofollow
	ldDate := ldPublished(w.ldJSON)
	page.Published = parseDate(w.published)
	if page.Published.IsZero() {
		page.Published = ldDate
	}
	if page.Published.IsZero() {
		page.Published = dateFromURL(pageURL)
	}
	// A bare <time> is the weakest source: pages often carry an events or
	// "trending" widget before the article's own date.
	if page.Published.IsZero() {
		page.Published = parseDate(w.timeTag)
	}
	// No article is dated tomorrow; a later date is a scraped widget, not a publish date.
	if page.Published.After(time.Now().Add(maxFutureSkew)) {
		page.Published = time.Time{}
	}

	base := pageURL
	if w.baseHref != "" {
		if resolved, ok := urlnorm.Resolve(w.baseHref, pageURL); ok {
			base = resolved
		}
	}

	page.Blocks = extractBlocks(doc, base)
	page.SourceURL = sourceURL(articleRoot(doc), pageURL)
	page.BodyImage = firstImage(articleRoot(doc), base)

	if w.image != "" {
		if img, ok := urlnorm.Resolve(w.image, base); ok {
			page.Image = img
		}
	}
	// An article has its own published date, from <meta> or the URL. A bare
	// <time> is not enough (listing pages date every teaser), and neither is
	// og:type=article, which WordPress also puts on tag and filter pages.
	page.IsArticle = !parseDate(w.publishedMeta).IsZero() || !ldDate.IsZero() || !dateFromURL(pageURL).IsZero()

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
	published   string
	// timeTag is the first <time datetime> on the page, used only when nothing better dates it.
	timeTag string
	// publishedMeta is the date from <meta> only, a stronger article signal.
	publishedMeta string
	// ldJSON holds the raw <script type="application/ld+json"> blocks.
	ldJSON   []string
	image    string
	noindex  bool
	nofollow bool

	haveTitle, haveH1 bool
}

func (w *walker) walk(n *html.Node) {
	if n.Type == html.ElementNode {
		switch n.Data {
		case "script":
			if t, _ := attr(n, "type"); strings.Contains(strings.ToLower(t), "ld+json") {
				var b strings.Builder
				collectRaw(n, &b)
				w.ldJSON = append(w.ldJSON, b.String())
			}
			return
		case "style", "noscript", "template":
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
		case "time":
			if dt, ok := attr(n, "datetime"); ok && w.timeTag == "" {
				w.timeTag = dt
			}
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

	// Article date: Open Graph property, schema.org itemprop, or a plain name.
	prop, _ := attr(n, "property")
	itemprop, _ := attr(n, "itemprop")
	for _, key := range []string{prop, itemprop, name} {
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "article:published_time", "og:article:published_time", "datepublished", "date", "pubdate":
			if w.published == "" {
				w.published = content
			}
			if w.publishedMeta == "" {
				w.publishedMeta = content
			}
		}
	}

	switch strings.ToLower(strings.TrimSpace(prop)) {
	case "og:image", "og:image:url", "og:image:secure_url":
		if w.image == "" {
			w.image = strings.TrimSpace(content)
		}
	}

	switch strings.ToLower(strings.TrimSpace(name)) {
	case "twitter:image", "twitter:image:src":
		if w.image == "" {
			w.image = strings.TrimSpace(content)
		}
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

// minParagraphRunes drops buttons, captions and ad labels from article text;
// minListItemRunes is the (lower) bar for list items, which carry much of the
// substance of explainer pages but are often short.
const (
	minParagraphRunes = 50
	minListItemRunes  = 30
)

// articleText is the article body for summaries: the real paragraphs (<p> of
// at least minParagraphRunes) inside <article>, else <main>, else <body>,
// skipping page chrome. It falls back to the extracted sections, then to the
// raw body text, for pages that do not use <p>.
func articleText(doc *html.Node, sections []Section, fallback string) string {
	root := find(doc, "article")
	if root == nil {
		root = find(doc, "main")
	}
	if root == nil {
		root = find(doc, "body")
	}

	if root != nil {
		if text := paragraphText(root); text != "" {
			return text
		}
	}

	return mainText(sections, fallback)
}

// paragraphText joins the real paragraphs (<p> of at least minParagraphRunes)
// and list items (<li> of at least minListItemRunes) under root, skipping page
// chrome.
func paragraphText(root *html.Node) string {
	var b strings.Builder

	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if skipped[n.Data] {
				return
			}
			if n.Data == "p" || n.Data == "li" {
				min := minParagraphRunes
				if n.Data == "li" {
					min = minListItemRunes
				}
				var p strings.Builder
				collectText(n, &p)
				if text := collapse(p.String()); utf8.RuneCountInString(text) >= min {
					b.WriteString(text)
					b.WriteByte(' ')
				}
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)

	return b.String()
}

// extractBlocks returns the <article id="..."> elements that each have a
// headline (h1-h3) and some real text: the topics of a digest page that
// carries several. Blocks nest in practice (WordPress wraps the whole post in
// its own <article id="post-123">), so the innermost ones win and an outer
// wrapper is ignored. Fewer than two blocks means the page is a single
// article, so none are returned.
func extractBlocks(doc *html.Node, base string) []Block {
	var blocks []Block

	// walk reports whether n's subtree holds any <article id>.
	var walk func(n *html.Node) bool
	walk = func(n *html.Node) bool {
		isArticle := false
		id := ""
		if n.Type == html.ElementNode && n.Data == "article" {
			if v, ok := attr(n, "id"); ok && strings.TrimSpace(v) != "" {
				isArticle, id = true, strings.TrimSpace(v)
			}
		}

		nested := false
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if walk(c) {
				nested = true
			}
		}

		if isArticle && !nested {
			if b, ok := blockOf(n, id, base); ok {
				blocks = append(blocks, b)
			}
		}
		return isArticle || nested
	}
	walk(doc)

	if len(blocks) < 2 {
		return nil
	}
	return blocks
}

func blockOf(n *html.Node, id, base string) (Block, bool) {
	var title strings.Builder
	var findTitle func(*html.Node) bool
	findTitle = func(n *html.Node) bool {
		if n.Type == html.ElementNode && (n.Data == "h1" || n.Data == "h2" || n.Data == "h3") {
			collectText(n, &title)
			return true
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if findTitle(c) {
				return true
			}
		}
		return false
	}
	findTitle(n)

	b := Block{
		ID:        id,
		Title:     truncate(collapse(title.String()), maxFieldRunes),
		Content:   truncate(collapse(paragraphText(n)), maxContentRunes),
		Image:     firstImage(n, base),
		SourceURL: sourceURL(n, base),
	}
	return b, b.Title != "" && b.Content != ""
}

// articleRoot is the element holding the article body: the first <article>,
// else <main>, else <body>.
func articleRoot(doc *html.Node) *html.Node {
	if r := find(doc, "article"); r != nil {
		return r
	}
	if r := find(doc, "main"); r != nil {
		return r
	}
	return find(doc, "body")
}

// firstImage returns the first usable <img> under root as an absolute http(s)
// URL: it reads data-src when src is a lazy-load placeholder and skips
// tracking pixels. Empty when there is none.
func firstImage(root *html.Node, base string) string {
	var found string

	var walk func(n *html.Node) bool
	walk = func(n *html.Node) bool {
		// Page chrome (menus, headers, sidebars) holds logos and avatars, not the story's picture.
		if n.Type == html.ElementNode && skipped[n.Data] {
			return false
		}
		if n.Type == html.ElementNode && n.Data == "img" {
			src, _ := attr(n, "src")
			if src == "" || strings.HasPrefix(src, "data:") {
				for _, k := range []string{"data-src", "data-lazy-src", "data-original"} {
					if v, ok := attr(n, k); ok && v != "" {
						src = v
						break
					}
				}
			}
			if isTinyImage(n) || strings.TrimSpace(src) == "" {
				return false
			}
			if u, ok := urlnorm.Resolve(strings.TrimSpace(src), base); ok && !strings.HasPrefix(strings.TrimSpace(src), "data:") && usableImageURL(u) {
				found = u
				return true
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if walk(c) {
				return true
			}
		}
		return false
	}
	if root != nil {
		walk(root)
	}

	return found
}

// junkImageWords mark images that decorate a page rather than illustrate the
// story: site logos and mascots, icons, author photos, buttons, ad banners.
var junkImageWords = []string{
	"logo", "icon", "avatar", "gravatar", "author", "headshot", "sprite", "emoji", "badge",
	"button", "pixel", "spacer", "banner", "advert", "/ads/", "placeholder",
}

// usableImageURL accepts raster pictures that are not decoration. SVG is out
// (the image proxy refuses it, and it is nearly always a logo or diagram
// scaffold), as are ICO files and the junk words above.
func usableImageURL(u string) bool {
	parsed, err := url.Parse(u)
	if err != nil {
		return false
	}
	path := strings.ToLower(parsed.Path)
	if strings.HasSuffix(path, ".svg") || strings.HasSuffix(path, ".ico") {
		return false
	}
	// The host counts too: avatars.githubusercontent.com serves profile photos.
	where := strings.ToLower(parsed.Host) + path
	for _, w := range junkImageWords {
		if strings.Contains(where, w) {
			return false
		}
	}
	// A size in the file name ("paul-200x200.jpg") marks a thumbnail or portrait.
	if m := thumbSize.FindStringSubmatch(path); m != nil {
		w, _ := strconv.Atoi(m[1])
		h, _ := strconv.Atoi(m[2])
		if w <= 320 && h <= 320 {
			return false
		}
	}
	return true
}

var thumbSize = regexp.MustCompile(`[-_](\d{2,4})x(\d{2,4})\.(?:jpe?g|png|webp|gif|avif)$`)

// isTinyImage spots tracking pixels and icons by their declared size.
func isTinyImage(n *html.Node) bool {
	for _, k := range []string{"width", "height"} {
		if v, ok := attr(n, k); ok {
			if px, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSpace(v), "px")); err == nil && px > 0 && px <= 32 {
				return true
			}
		}
	}
	return false
}

// shareHosts are never "the original story": sharing, comment and social
// widgets that sit in article paragraphs.
var shareHosts = []string{
	"addtoany.com", "disqus.com", "facebook.com", "twitter.com", "x.com", "whatsapp.com", "wa.me",
	"telegram.me", "t.me", "linkedin.com", "instagram.com", "youtube.com", "youtu.be", "pinterest.com",
	"google.com", "play.google.com", "apps.apple.com", "bit.ly", "goo.gl",
}

// sourceURL returns the first link inside root's paragraphs that leaves the
// page's own site and is not a sharing widget: the story a summary cites.
func sourceURL(root *html.Node, pageURL string) string {
	if root == nil {
		return ""
	}
	pageHost := hostOf(pageURL)

	var found string
	var walk func(n *html.Node, inParagraph bool) bool
	walk = func(n *html.Node, inParagraph bool) bool {
		if n.Type == html.ElementNode {
			if skipped[n.Data] {
				return false
			}
			if n.Data == "p" {
				inParagraph = true
			}
			if n.Data == "a" && inParagraph {
				if href, ok := attr(n, "href"); ok {
					if u, ok := urlnorm.Resolve(strings.TrimSpace(href), pageURL); ok {
						host := hostOf(u)
						if host != "" && !sameSite(host, pageHost) && !isShareHost(host) {
							found = u
							return true
						}
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if walk(c, inParagraph) {
				return true
			}
		}
		return false
	}
	walk(root, false)

	return found
}

func hostOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
}

// sameSite is true when a and b are one host or one is a subdomain of the other.
func sameSite(a, b string) bool {
	return a == b || strings.HasSuffix(a, "."+b) || strings.HasSuffix(b, "."+a)
}

func isShareHost(host string) bool {
	for _, h := range shareHosts {
		if host == h || strings.HasSuffix(host, "."+h) {
			return true
		}
	}
	return false
}

// mainText joins the extracted sections; pages with none fall back to the
// raw body text gathered during the walk.
func mainText(sections []Section, fallback string) string {
	if len(sections) == 0 {
		return fallback
	}
	var b strings.Builder
	for _, s := range sections {
		if b.Len() > maxContentRunes*4 {
			break
		}
		b.WriteString(s.Text)
		b.WriteByte(' ')
	}
	return b.String()
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

// maxFutureSkew is how far past now a page date may be (time zones, early posts).
const maxFutureSkew = 24 * time.Hour

// dateLayouts are the timestamp formats seen in page metadata: ISO 8601 with
// and without an offset, and the "2006-01-02 15:04:05 +0000 UTC" form that Go
// and some CMSs print.
var dateLayouts = []string{
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02T15:04:05-0700",
	"2006-01-02 15:04:05 -0700 MST",
	"2006-01-02 15:04:05 -0700",
	"2006-01-02 15:04:05",
	"2006-01-02",
	time.RFC1123Z,
	time.RFC1123,
}

// parseDate accepts the layouts above; anything else is "unknown".
func parseDate(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

// ldPublished returns the datePublished of the page's own article in its
// JSON-LD, or zero. Only top-level objects (and the items of a top-level array
// or @graph) are considered, so a listing page whose JSON-LD merely contains
// posts does not count as an article.
func ldPublished(blobs []string) time.Time {
	for _, raw := range blobs {
		var v any
		if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &v); err != nil {
			continue
		}
		if t := ldArticleDate(v); !t.IsZero() {
			return t
		}
	}
	return time.Time{}
}

func ldArticleDate(v any) time.Time {
	switch x := v.(type) {
	case []any:
		for _, item := range x {
			if m, ok := item.(map[string]any); ok {
				if t := ldObjectDate(m); !t.IsZero() {
					return t
				}
			}
		}
	case map[string]any:
		if t := ldObjectDate(x); !t.IsZero() {
			return t
		}
		if graph, ok := x["@graph"].([]any); ok {
			return ldArticleDate(graph)
		}
	}
	return time.Time{}
}

func ldObjectDate(m map[string]any) time.Time {
	if !ldIsArticle(m["@type"]) {
		return time.Time{}
	}
	if s, ok := m["datePublished"].(string); ok {
		return parseDate(s)
	}
	return time.Time{}
}

// ldIsArticle matches schema.org Article and its subtypes (NewsArticle,
// BlogPosting, TechArticle, ...), SocialMediaPosting and Report; a type may be
// a string or a list. A bare WebPage is deliberately not accepted: listing
// pages carry one too.
func ldIsArticle(t any) bool {
	check := func(s string) bool {
		s = strings.ToLower(s)
		return strings.HasSuffix(s, "article") || s == "blogposting" || s == "socialmediaposting" || s == "report"
	}
	switch x := t.(type) {
	case string:
		return check(x)
	case []any:
		for _, e := range x {
			if s, ok := e.(string); ok && check(s) {
				return true
			}
		}
	}
	return false
}

var (
	urlYMD = regexp.MustCompile(`/(20\d{2})/(\d{2})/(\d{2})(?:/|$)`) // /2026/10/07/
	urlDMY = regexp.MustCompile(`/(\d{2})-(\d{2})-(20\d{2})(?:/|$)`) // /15-10-2026
)

// dateFromURL reads a date out of the URL path when the page has no metadata,
// which is how Insights (/yyyy/mm/dd/) and Drishti (/dd-mm-yyyy) both date posts.
func dateFromURL(u string) time.Time {
	if m := urlYMD.FindStringSubmatch(u); m != nil {
		return parseDate(m[1] + "-" + m[2] + "-" + m[3])
	}
	if m := urlDMY.FindStringSubmatch(u); m != nil {
		return parseDate(m[3] + "-" + m[2] + "-" + m[1])
	}
	return time.Time{}
}
