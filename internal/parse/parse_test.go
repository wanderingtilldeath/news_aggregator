package parse

import (
	"reflect"
	"strings"
	"testing"
)

func parseString(t *testing.T, doc, pageURL string) Page {
	t.Helper()

	page, err := HTMLParser{}.Parse(strings.NewReader(doc), pageURL)
	if err != nil {
		t.Fatalf("Parse() returned error: %v", err)
	}
	return page
}

func TestHTMLParser_Links(t *testing.T) {
	tests := []struct {
		name    string
		html    string
		baseURL string
		want    []string
	}{
		{
			name:    "absolute and relative links resolved against page",
			html:    `<a href="/about">about</a><a href="https://other.example/page">other</a>`,
			baseURL: "https://example.com/",
			want:    []string{"https://example.com/about", "https://other.example/page"},
		},
		{
			name:    "fragment is stripped",
			html:    `<a href="/docs#section-2">docs</a>`,
			baseURL: "https://example.com/",
			want:    []string{"https://example.com/docs"},
		},
		{
			name:    "non-http schemes are filtered out",
			html:    `<a href="mailto:a@example.com">mail</a><a href="javascript:void(0)">js</a><a href="/ok">ok</a>`,
			baseURL: "https://example.com/",
			want:    []string{"https://example.com/ok"},
		},
		{
			name:    "no anchors yields no links",
			html:    `<p>nothing to see here</p>`,
			baseURL: "https://example.com/",
			want:    nil,
		},
		{
			name:    "duplicates within a page are collapsed",
			html:    `<a href="/a">1</a><a href="/a#x">2</a><a href="/b">3</a><a href="/a?utm_source=z">4</a>`,
			baseURL: "https://example.com/",
			want:    []string{"https://example.com/a", "https://example.com/b"},
		},
		{
			name:    "base href overrides the page URL",
			html:    `<html><head><base href="https://cdn.example.com/x/"></head><body><a href="y">y</a></body></html>`,
			baseURL: "https://example.com/page",
			want:    []string{"https://cdn.example.com/x/y"},
		},
		{
			name:    "relative base href resolves against the page URL",
			html:    `<html><head><base href="/sub/"></head><body><a href="y">y</a></body></html>`,
			baseURL: "https://example.com/a/b",
			want:    []string{"https://example.com/sub/y"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseString(t, tt.html, tt.baseURL).Links
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Links = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHTMLParser_Metadata(t *testing.T) {
	doc := `<html><head>
		<title>  Hello
		 World </title>
		<meta name="description" content="A  short   description">
		<style>.x{color:red}</style>
		<script>var hidden = 1;</script>
	</head><body>
		<h1>Main   heading</h1><h1>second</h1>
		<p>Some body text.</p>
		<script>var alsoHidden = 2;</script>
	</body></html>`

	page := parseString(t, doc, "https://example.com/")

	if page.Title != "Hello World" {
		t.Errorf("Title = %q", page.Title)
	}
	if page.Description != "A short description" {
		t.Errorf("Description = %q", page.Description)
	}
	if page.H1 != "Main heading" {
		t.Errorf("H1 = %q", page.H1)
	}
	if !strings.Contains(page.Snippet, "Some body text.") || strings.Contains(page.Snippet, "hidden") || strings.Contains(page.Snippet, "color") {
		t.Errorf("Snippet = %q", page.Snippet)
	}
}

func TestHTMLParser_SnippetIsCapped(t *testing.T) {
	doc := "<body>" + strings.Repeat("word ", 5000) + "</body>"

	page := parseString(t, doc, "https://example.com/")

	if got := len([]rune(page.Snippet)); got > maxSnippetRunes {
		t.Fatalf("snippet has %d runes, want <= %d", got, maxSnippetRunes)
	}
}

func TestHTMLParser_RobotsMeta(t *testing.T) {
	tests := []struct {
		content           string
		noindex, nofollow bool
	}{
		{"", false, false},
		{"noindex", true, false},
		{"nofollow", false, true},
		{"NoIndex, NoFollow", true, true},
		{"none", true, true},
		{"index, follow", false, false},
	}

	for _, tt := range tests {
		doc := `<head><meta name="robots" content="` + tt.content + `"></head><body><a href="/x">x</a></body>`
		page := parseString(t, doc, "https://example.com/")

		if page.NoIndex != tt.noindex || page.NoFollow != tt.nofollow {
			t.Errorf("content %q: noindex=%v nofollow=%v, want %v %v",
				tt.content, page.NoIndex, page.NoFollow, tt.noindex, tt.nofollow)
		}
	}
}

func TestHTMLParser_Published(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		want string // RFC 3339, empty = unknown
	}{
		{"open graph", `<head><meta property="article:published_time" content="2026-03-04T10:00:00+05:30"></head>`, "2026-03-04T04:30:00Z"},
		{"itemprop", `<head><meta itemprop="datePublished" content="2026-03-04"></head>`, "2026-03-04T00:00:00Z"},
		{"time element", `<body><time datetime="2026-03-05T08:00:00Z">5 Mar</time></body>`, "2026-03-05T08:00:00Z"},
		{"meta beats time", `<head><meta property="article:published_time" content="2026-01-01"></head><body><time datetime="2026-02-02">x</time></body>`, "2026-01-01T00:00:00Z"},
		{"json-ld beats time", `<head><script type="application/ld+json">{"@type":"NewsArticle","datePublished":"2026-01-01T00:00:00Z"}</script></head><body><time datetime="2026-02-02">events widget</time></body>`, "2026-01-01T00:00:00Z"},
		{"url date beats time", `<body><time datetime="2026-02-02">events widget</time></body>`, "2026-01-01T00:00:00Z"},
		{"future time is unknown", `<body><time datetime="2099-01-01">upcoming</time></body>`, ""},
		{"future meta is unknown", `<head><meta property="article:published_time" content="2099-01-01"></head>`, ""},
		{"garbage", `<head><meta property="article:published_time" content="yesterday"></head>`, ""},
		{"absent", `<body>hi</body>`, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pageURL := "http://x.test/"
			if tt.name == "url date beats time" {
				pageURL = "http://x.test/2026/01/01/post/"
			}
			got := parseString(t, tt.doc, pageURL).Published
			if tt.want == "" {
				if !got.IsZero() {
					t.Errorf("Published = %v, want unknown", got)
				}
				return
			}
			if got.Format("2006-01-02T15:04:05Z07:00") != tt.want {
				t.Errorf("Published = %v, want %s", got, tt.want)
			}
		})
	}
}

func TestHTMLParser_PublishedFromURLAndSnippetFromMainContent(t *testing.T) {
	tests := []struct{ url, want string }{
		{"https://i.test/2026/10/07/upsc-current-affairs/", "2026-10-07"},
		{"https://d.test/news-analysis/15-10-2026", "2026-10-15"},
		{"https://d.test/news-analysis/31-02-2026", ""}, // not a real date
		{"https://d.test/mind-map/", ""},
	}
	for _, tt := range tests {
		got := parseString(t, `<body>x</body>`, tt.url).Published
		if tt.want == "" && !got.IsZero() || tt.want != "" && got.Format("2006-01-02") != tt.want {
			t.Errorf("%s: Published = %v, want %q", tt.url, got, tt.want)
		}
	}

	page := parseString(t, `<body><nav>Buy our course</nav><main><p>Real article text.</p></main></body>`, "http://x.test/")
	if page.Snippet != "Real article text." {
		t.Errorf("Snippet = %q, want article text only", page.Snippet)
	}
}

func TestHTMLParser_ImageAndArticle(t *testing.T) {
	tests := []struct {
		name        string
		html        string
		pageURL     string
		wantImage   string
		wantArticle bool
	}{
		{
			name:        "og:image with a published date",
			html:        `<head><meta property="og:image" content="https://cdn.example/a.jpg"><meta property="article:published_time" content="2026-10-01T08:00:00Z"></head><body><p>x</p></body>`,
			pageURL:     "https://example.com/story",
			wantImage:   "https://cdn.example/a.jpg",
			wantArticle: true,
		},
		{
			name:      "og:type article alone (a WordPress tag page) is not an article",
			html:      `<head><meta property="og:type" content="article"></head><body><a href="/a">a</a></body>`,
			pageURL:   "https://example.com/tag/economy/",
			wantImage: "",
		},
		{
			name:      "relative og:image is resolved",
			html:      `<head><meta property="og:image" content="/img/a.jpg"></head><body></body>`,
			pageURL:   "https://example.com/story",
			wantImage: "https://example.com/img/a.jpg",
		},
		{
			name:      "twitter:image is the fallback and og:image wins",
			html:      `<head><meta name="twitter:image" content="https://cdn.example/t.jpg"><meta property="og:image" content="https://cdn.example/o.jpg"></head>`,
			pageURL:   "https://example.com/",
			wantImage: "https://cdn.example/t.jpg",
		},
		{
			name:        "published date makes it an article",
			html:        `<head><meta property="article:published_time" content="2026-10-01T08:00:00Z"></head>`,
			pageURL:     "https://example.com/p",
			wantArticle: true,
		},
		{
			name:        "dated URL makes it an article",
			html:        `<body>hello</body>`,
			pageURL:     "https://example.com/2026/10/07/story",
			wantArticle: true,
		},
		{
			name:    "bare <time> on a listing page is not an article",
			html:    `<body><time datetime="2026-10-01">Oct 1</time><a href="/a">a</a></body>`,
			pageURL: "https://example.com/news/",
		},
		{
			name:    "plain listing page is not an article",
			html:    `<body><a href="/a">a</a></body>`,
			pageURL: "https://example.com/news/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page := parseString(t, tt.html, tt.pageURL)
			if page.Image != tt.wantImage {
				t.Errorf("Image = %q, want %q", page.Image, tt.wantImage)
			}
			if page.IsArticle != tt.wantArticle {
				t.Errorf("IsArticle = %v, want %v", page.IsArticle, tt.wantArticle)
			}
		})
	}
}

func TestHTMLParser_Content(t *testing.T) {
	long := strings.Repeat("word ", 5000)
	page := parseString(t, `<body><nav>menu</nav><article><h1>T</h1><p>Hello there, this opening paragraph is long enough to be kept as real text.</p><p>`+long+`</p></article></body>`, "https://example.com/")

	if !strings.HasPrefix(page.Content, "Hello there, this opening") || strings.Contains(page.Content, "menu") {
		t.Errorf("Content should start with the article text and omit chrome, got %.40q", page.Content)
	}
	if n := len([]rune(page.Content)); n > maxContentRunes {
		t.Errorf("Content has %d runes, want at most %d", n, maxContentRunes)
	}
}

func TestHTMLParser_ContentKeepsRealParagraphsOnly(t *testing.T) {
	page := parseString(t, `<body>
		<header>Skip to content Advertisement</header>
		<article>
		  <div>AI Narration Listen 00:00 0.5x 1x</div>
		  <p>Share</p>
		  <p>The finance ministry announced a new tax slab for salaried workers on Monday morning.</p>
		  <aside><p>Related: an unrelated story that should not be part of this article text at all.</p></aside>
		  <p>The change takes effect from April and affects about two crore taxpayers across the country.</p>
		</article></body>`, "https://example.com/story")

	want := "The finance ministry announced a new tax slab for salaried workers on Monday morning. " +
		"The change takes effect from April and affects about two crore taxpayers across the country."
	if page.Content != want {
		t.Errorf("Content = %q\nwant      %q", page.Content, want)
	}
}

const topicPage = `<body><nav>Menu</nav>
<div class="gs-label">GS Paper 3</div>
<article class="ca-topic-block" id="gs3-ca1"><h2 class="topic-title">Stampede Deaths in India</h2>
  <p class="ca-meta-row">Source: NDTV</p>
  <p>A deadly crowd surge at a festival in Kolkata killed nineteen people and injured dozens of others on Sunday night.</p>
  <aside><p>Related reading that is long enough to count as a paragraph but sits in a sidebar, so it must be skipped.</p></aside>
  <p>Experts say crowd-density monitoring and regulated entry could prevent such disasters at religious gatherings.</p>
</article>
<article class="ca-topic-block" id="cme-ca3"><h2 class="topic-title">IMF Report on AI and Employment</h2>
  <p>The International Monetary Fund said artificial intelligence could affect close to forty per cent of jobs across Asia.</p>
</article>
<article class="ca-topic-block" id="ffp-ca4"><h2 class="topic-title">Kufri Jamunia</h2><p>Short.</p></article>
</body>`

func TestHTMLParser_Blocks(t *testing.T) {
	page := parseString(t, topicPage, "https://example.com/2026/10/19/daily/")

	if len(page.Blocks) != 2 {
		t.Fatalf("got %d blocks, want 2 (the one with no real text is dropped): %+v", len(page.Blocks), page.Blocks)
	}
	first := page.Blocks[0]
	if first.ID != "gs3-ca1" || first.Title != "Stampede Deaths in India" {
		t.Errorf("first block = %+v", first)
	}
	if !strings.HasPrefix(first.Content, "A deadly crowd surge") || strings.Contains(first.Content, "Related reading") || strings.Contains(first.Content, "Source: NDTV") {
		t.Errorf("block content should be the body paragraphs only, got %q", first.Content)
	}
	if page.Blocks[1].ID != "cme-ca3" {
		t.Errorf("second block = %+v", page.Blocks[1])
	}
}

func TestHTMLParser_NoBlocksForOrdinaryPages(t *testing.T) {
	for name, doc := range map[string]string{
		"single article with an id": `<body><article id="main"><h1>Only one</h1><p>` + strings.Repeat("word ", 20) + `</p></article></body>`,
		"articles without ids":      `<body><article><h2>A</h2><p>` + strings.Repeat("word ", 20) + `</p></article><article><h2>B</h2><p>` + strings.Repeat("word ", 20) + `</p></article></body>`,
		"no articles":               `<body><p>` + strings.Repeat("word ", 20) + `</p></body>`,
	} {
		if blocks := parseString(t, doc, "https://example.com/").Blocks; len(blocks) != 0 {
			t.Errorf("%s: got blocks %+v, want none", name, blocks)
		}
	}
}

// wordpressPost mirrors the real InsightsIAS daily post: the whole post sits in
// its own <article id="post-...">, with one <article id> per topic inside it.
const wordpressPost = `<body><nav>Menu</nav>
<article id="post-362408" class="post type-post"><h1 class="entry-title">UPSC CURRENT AFFAIRS - 19 OCTOBER 2026</h1>
 <nav class="toc"><a href="#gs3-ca1">Stampede Deaths in India</a></nav>
 <article class="ca-topic-block" id="gs3-ca1" itemscope><div class="ca-topic-header"><h2 class="topic-title">Stampede Deaths in India</h2></div>
  <img src="/wp-content/uploads/2026/10/Stampede.png" alt="">
  <div class="ca-meta"><p class="ca-meta-row"><span>Source:</span> <a href="https://www.ndtv.com/india-news/stampede-case-123">NDTV</a></p></div>
  <div class="article-body-column"><p><strong>Context:</strong> A deadly crowd surge at a festival in Kolkata killed nineteen people and injured dozens.</p>
  <p>Crowd-density monitoring and regulated entry could prevent such disasters at religious gatherings, experts said.</p></div>
 </article>
 <article class="ca-topic-block" id="ffp-ca4" itemscope><h2 class="topic-title">Kufri Jamunia</h2>
  <img data-src="https://cdn.example/jamunia.png" src="data:image/gif;base64,R0lGOD">
  <p class="ca-meta-row"><span>Source:</span> <a href="https://www.pib.gov.in/PressRelease?PRID=1">PIB</a></p>
  <p>Kufri Jamunia is a biofortified purple potato variety released by the Central Potato Research Institute this year.</p>
 </article>
 <article class="ca-topic-block" id="cme-ca3"><h2 class="topic-title">IMF on AI</h2>
  <img src="/pixel.gif" width="1" height="1"><p>The International Monetary Fund said artificial intelligence could affect forty per cent of jobs across Asia.</p>
 </article>
</article></body>`

func TestHTMLParser_NestedBlocksUseTheInnermostArticles(t *testing.T) {
	page := parseString(t, wordpressPost, "https://www.insightsonindia.com/2026/10/19/upsc-current-affairs-19-october-2026/")

	if len(page.Blocks) != 3 {
		t.Fatalf("got %d blocks, want the 3 topics (not the outer post wrapper): %+v", len(page.Blocks), page.Blocks)
	}
	ids := []string{page.Blocks[0].ID, page.Blocks[1].ID, page.Blocks[2].ID}
	if ids[0] != "gs3-ca1" || ids[1] != "ffp-ca4" || ids[2] != "cme-ca3" {
		t.Errorf("block ids = %v", ids)
	}

	a, b, c := page.Blocks[0], page.Blocks[1], page.Blocks[2]
	if a.Title != "Stampede Deaths in India" || !strings.HasPrefix(a.Content, "Context: A deadly crowd surge") {
		t.Errorf("first block = %+v", a)
	}
	if a.Image != "https://www.insightsonindia.com/wp-content/uploads/2026/10/Stampede.png" {
		t.Errorf("relative <img> should resolve against the page, got %q", a.Image)
	}
	if a.SourceURL != "https://www.ndtv.com/india-news/stampede-case-123" {
		t.Errorf("source link = %q", a.SourceURL)
	}
	if b.Image != "https://cdn.example/jamunia.png" {
		t.Errorf("a lazy-loaded image should come from data-src, got %q", b.Image)
	}
	if b.SourceURL != "https://www.pib.gov.in/PressRelease?PRID=1" {
		t.Errorf("second block source = %q", b.SourceURL)
	}
	if c.Image != "" || c.SourceURL != "" {
		t.Errorf("a tracking pixel is not an image and no link means no source: %+v", c)
	}
}

func TestHTMLParser_SourceURL(t *testing.T) {
	longP := func(inner string) string {
		return `<p>The commission took cognisance of the allegations. ` + inner + ` It asked for a report within two weeks.</p>`
	}
	tests := []struct {
		name string
		html string
		want string
	}{
		{"outside link in a paragraph", `<article>` + longP(`<a href="https://www.thehindu.com/news/national/story.ece">Source</a>`) + `</article>`, "https://www.thehindu.com/news/national/story.ece"},
		{"same-site link is not a source", `<article>` + longP(`<a href="/other/page">more</a><a href="https://sub.drishtiias.com/x">sub</a>`) + `</article>`, ""},
		{"share and comment widgets are skipped", `<article>` + longP(`<a href="https://www.addtoany.com/share">share</a><a href="https://disqus.com/?ref">c</a><a href="https://t.me/x">t</a>`) + `</article>`, ""},
		{"the first real source wins over a later one", `<article>` + longP(`<a href="https://www.addtoany.com/s">s</a><a href="https://www.pib.gov.in/a">PIB</a><a href="https://other.example/b">o</a>`) + `</article>`, "https://www.pib.gov.in/a"},
		{"a link outside any paragraph is ignored", `<article><div><a href="https://www.thehindu.com/x">nav</a></div></article>`, ""},
		{"links in page chrome are ignored", `<article><aside><p>x <a href="https://ads.example/a">ad</a></p></aside></article>`, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseString(t, tt.html, "https://www.drishtiias.com/daily-updates/news/x").SourceURL; got != tt.want {
				t.Errorf("SourceURL = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHTMLParser_ListItemsCountAsContent(t *testing.T) {
	page := parseString(t, `<body><article><h1>T</h1>
		<p>Context: a short intro paragraph that is long enough to be kept as real text.</p>
		<ul><li>Share</li><li>Biofortified purple potato with high anthocyanin content for the plains.</li><li>Needs cool nights.</li></ul>
	</article></body>`, "https://example.com/story")

	want := "Context: a short intro paragraph that is long enough to be kept as real text. " +
		"Biofortified purple potato with high anthocyanin content for the plains."
	if page.Content != want {
		t.Errorf("Content = %q\nwant      %q", page.Content, want)
	}
}

func TestHTMLParser_JSONLDDate(t *testing.T) {
	ld := func(body string) string {
		return `<head><script type="application/ld+json">` + body + `</script></head><body><p>x</p></body>`
	}
	tests := []struct {
		name        string
		html        string
		wantArticle bool
		wantDate    string // YYYY-MM-DD, "" for none
	}{
		{"Article", ld(`{"@context":"https://schema.org","@type":"Article","datePublished":"2026-10-16T09:30:00+05:30"}`), true, "2026-10-16"},
		{"BlogPosting in @graph", ld(`{"@graph":[{"@type":"WebSite"},{"@type":"BlogPosting","datePublished":"2026-10-15T10:00:00Z"}]}`), true, "2026-10-15"},
		{"type list", ld(`{"@type":["Thing","NewsArticle"],"datePublished":"2026-10-14"}`), true, "2026-10-14"},
		{"top-level array", ld(`[{"@type":"BreadcrumbList"},{"@type":"TechArticle","datePublished":"2026-10-13"}]`), true, "2026-10-13"},
		{"a listing that merely contains posts is not an article",
			ld(`{"@type":"Blog","blogPost":[{"@type":"BlogPosting","datePublished":"2026-10-12"}]}`), false, ""},
		{"SocialMediaPosting (Hugging Face)", ld(`{"@type":"SocialMediaPosting","datePublished":"2026-10-15T08:05:40.843Z"}`), true, "2026-10-15"},
		{"a bare WebPage is not an article (listing pages have one too)", ld(`{"@graph":[{"@type":"WebPage","datePublished":"2026-08-24T15:32:15+00:00"}]}`), false, ""},
		{"a non-article type with a date is not an article", ld(`{"@type":"Event","datePublished":"2026-10-11"}`), false, ""},
		{"invalid JSON is ignored", ld(`{not json`), false, ""},
		{"unparseable date is ignored", ld(`{"@type":"Article","datePublished":"last Tuesday"}`), false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page := parseString(t, tt.html, "https://example.com/blog/post")
			if page.IsArticle != tt.wantArticle {
				t.Errorf("IsArticle = %v, want %v", page.IsArticle, tt.wantArticle)
			}
			got := ""
			if !page.Published.IsZero() {
				got = page.Published.Format("2006-01-02")
			}
			if got != tt.wantDate {
				t.Errorf("Published = %q, want %q", got, tt.wantDate)
			}
		})
	}
}

func TestParseDateLayouts(t *testing.T) {
	for in, want := range map[string]string{
		"2026-10-16T09:30:00Z":            "2026-10-16 09:30",
		"2026-10-16T09:30:00.123+05:30":   "2026-10-16 04:00",
		"2026-10-16T09:30:00+0530":        "2026-10-16 04:00",
		"2026-10-16 10:00:00 +0000 UTC":   "2026-10-16 10:00", // Smashing Magazine
		"2026-10-16 10:00:00 +0530 IST":   "2026-10-16 04:30",
		"2026-10-16 10:00:00":             "2026-10-16 10:00",
		"2026-10-16":                      "2026-10-16 00:00",
		"Fri, 16 Oct 2026 10:00:00 +0000": "2026-10-16 10:00",
		"  2026-10-16T09:30:00Z  ":        "2026-10-16 09:30",
	} {
		got := parseDate(in)
		if got.IsZero() || got.Format("2006-01-02 15:04") != want {
			t.Errorf("parseDate(%q) = %v, want %s", in, got, want)
		}
	}
	for _, bad := range []string{"", "yesterday", "16/10/2026", "2026-13-45"} {
		if got := parseDate(bad); !got.IsZero() {
			t.Errorf("parseDate(%q) = %v, want zero", bad, got)
		}
	}
}

func TestHTMLParser_BodyImage(t *testing.T) {
	const base = "https://blog.example/post/slug"
	text := `<p>` + strings.Repeat("A sentence long enough to count as article text. ", 3) + `</p>`

	tests := []struct {
		name string
		html string
		want string
	}{
		{"a real figure inside the article", `<body><article><h1>T</h1>` + text + `<img src="/img/post/diagram-1.png"></article></body>`, "https://blog.example/img/post/diagram-1.png"},
		{"lazy-loaded figure", `<body><article>` + text + `<img src="data:image/gif;base64,R0lG" data-src="/img/fig.jpg"></article></body>`, "https://blog.example/img/fig.jpg"},
		{"chrome images are skipped", `<body><header><img src="/img/team.png"></header><nav><img src="/img/menu.png"></nav><article>` + text + `<img src="/img/real.png"></article></body>`, "https://blog.example/img/real.png"},
		{"svg is never used", `<body><article>` + text + `<img src="/images/diagram.svg"><img src="/images/photo.webp"></article></body>`, "https://blog.example/images/photo.webp"},
		{"logos, mascots, avatars, banners and buttons are skipped",
			`<body><article>` + text +
				`<img src="/images/rust-logo-blk.png"><img src="/u/avatar-12.jpg"><img src="/uploads/Mobile Banner 320.gif">` +
				`<img src="/b/printfriendly-button.png"><img src="/x/author-photo.jpg"><img src="/figs/benchmark.png"></article></body>`,
			"https://blog.example/figs/benchmark.png"},
		{"tiny images are skipped", `<body><article>` + text + `<img src="/dot.png" width="1" height="1"><img src="/figs/chart.png"></article></body>`, "https://blog.example/figs/chart.png"},
		{"nothing usable gives empty", `<body><article>` + text + `<img src="/images/logo.png"><img src="/a.svg"></article></body>`, ""},
		{"an empty src is not the page itself", `<body><article>` + text + `<img src=""><img src="  "><img src="/figs/real.png"></article></body>`, "https://blog.example/figs/real.png"},
		{"a profile photo from an avatar host is skipped", `<body><article>` + text + `<img src="https://avatars.githubusercontent.com/u/81577787?s=200&v=4"><img src="/figs/arch.png"></article></body>`, "https://blog.example/figs/arch.png"},
		{"a thumbnail-sized file name is skipped", `<body><article>` + text + `<img src="/uploads/paulboag-200x200.jpg"><img src="/uploads/hero-1200x630.jpg"></article></body>`, "https://blog.example/uploads/hero-1200x630.jpg"},
		{"no images at all", `<body><article>` + text + `</article></body>`, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseString(t, tt.html, base).BodyImage; got != tt.want {
				t.Errorf("BodyImage = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUsableImageURL(t *testing.T) {
	for u, want := range map[string]bool{
		"https://x.test/wp-content/uploads/2026/10/Stampede-Deaths-in-India.png": true,
		"https://x.test/photo.jpg":                        true,
		"https://x.test/chart.webp":                       true,
		"https://x.test/mascot.svg":                       false,
		"https://x.test/favicon.ico":                      false,
		"https://x.test/site-logo.png":                    false,
		"https://x.test/Authors/jo.jpg":                   false,
		"https://x.test/ads/promo.png":                    false,
		"https://x.test/pixel.gif":                        false,
		"https://avatars.githubusercontent.com/u/1?s=200": false,
		"https://x.test/team/jo-200x200.jpg":              false,
		"https://x.test/hero-1200x630.jpg":                true,
		"https://x.test/chart-640x300.png":                true, // only when both sides are 320 or less is it a thumbnail
		"https://x.test/chart-300x300.png":                false,
	} {
		if got := usableImageURL(u); got != want {
			t.Errorf("usableImageURL(%q) = %v, want %v", u, got, want)
		}
	}
}
