package parse

import (
	"reflect"
	"testing"
)

func TestSections(t *testing.T) {
	doc := `<html><head><title>T</title></head><body>
<nav>Home Docs Blog</nav>
<main>
  <p>Intro text.</p>
  <h1>Install</h1><p>Run the installer.</p>
  <h2>Linux</h2><p>Use apt.</p><pre>apt install x
apt update</pre>
  <h2>Mac</h2><p>Use brew.</p>
  <h1>Usage</h1><ul><li>one</li><li>two</li></ul>
  <script>var x = 1;</script>
</main>
<footer>Copyright</footer></body></html>`

	want := []Section{
		{Heading: "", Text: "Intro text."},
		{Heading: "Install", Text: "Run the installer."},
		{Heading: "Install > Linux", Text: "Use apt.\n\napt install x\napt update"},
		{Heading: "Install > Mac", Text: "Use brew."},
		{Heading: "Usage", Text: "one\n\ntwo"},
	}

	if got := parseString(t, doc, "http://x.test/").Sections; !reflect.DeepEqual(got, want) {
		t.Errorf("Sections = %#v\nwant %#v", got, want)
	}
}

func TestSections_FallsBackToBodyAndSkipsChrome(t *testing.T) {
	doc := `<body><header>Top</header><p>Body text</p><aside>Side</aside><form>Search</form></body>`
	want := []Section{{Text: "Body text"}}

	if got := parseString(t, doc, "http://x.test/").Sections; !reflect.DeepEqual(got, want) {
		t.Errorf("Sections = %#v, want %#v", got, want)
	}
}
