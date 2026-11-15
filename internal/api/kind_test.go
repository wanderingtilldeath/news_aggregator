package api

import (
	"context"
	"testing"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

func TestDigestAndStoryCarryTheKind(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	for _, p := range []store.Page{
		{URLHash: "op", URL: "https://www.site.test/opinion/columns/why-it-matters", Title: "Why it matters", IsArticle: true, Published: time.Now().Add(-time.Hour)},
		{URLHash: "rep", URL: "https://www.site.test/news/india/a-report", Title: "A report", IsArticle: true, Published: time.Now().Add(-time.Hour)},
	} {
		if err := f.store.Save(ctx, p); err != nil {
			t.Fatal(err)
		}
	}

	_, body, _ := f.do(t, "GET", "/digest", "")
	kinds := map[string]any{}
	for _, it := range body["items"].([]any) {
		m := it.(map[string]any)
		kinds[m["id"].(string)] = m["kind"]
	}
	if kinds["op"] != "opinion" || kinds["rep"] != nil {
		t.Fatalf("kinds = %v, want opinion for the column and none for the report", kinds)
	}

	if _, body, _ := f.do(t, "GET", "/story/op", ""); body["kind"] != "opinion" {
		t.Fatalf("story kind = %v", body["kind"])
	}
}

func TestIntroEndpoint(t *testing.T) {
	f := newFixture(t)
	_ = f.store.SaveIntro(context.Background(), store.Intro{Day: "2026-11-15", Track: "upsc", Text: "A paragraph."})

	status, body, hdr := f.do(t, "GET", "/intro?track=upsc", "")
	if status != 200 || body["text"] != "A paragraph." || body["day"] != "2026-11-15" || hdr.Get("Cache-Control") == "" {
		t.Fatalf("intro = %d %v %v", status, body, hdr)
	}
	for path, want := range map[string]int{
		"/intro?track=upsc&day=2026-11-14": 404, "/intro?track=engineering": 404,
		"/intro?track=bogus": 400, "/intro?track=upsc&day=nope": 400, "/intro": 400,
	} {
		if status, _, _ := f.do(t, "GET", path, ""); status != want {
			t.Errorf("%s = %d, want %d", path, status, want)
		}
	}
}
