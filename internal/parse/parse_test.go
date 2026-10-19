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
		{"garbage", `<head><meta property="article:published_time" content="yesterday"></head>`, ""},
		{"absent", `<body>hi</body>`, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseString(t, tt.doc, "http://x.test/").Published
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
