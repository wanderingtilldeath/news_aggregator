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

func TestStoryAlsoCarriesTheKindOfEachSource(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()
	for _, p := range []store.Page{
		{URLHash: "n1", URL: "https://www.theguardian.com/world/a", Title: "Donbas plan floated", IsArticle: true, Published: now.Add(-2 * time.Hour)},
		{URLHash: "o1", URL: "https://news.un.org/en/story/b", Title: "Donbas plan reported", IsArticle: true, Published: now.Add(-time.Hour)},
	} {
		if err := f.store.Save(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	_ = f.store.SetStories(ctx, now.Add(-24*time.Hour), map[string]store.StoryGroup{"n1": {ID: "n1", Sources: 2}, "o1": {ID: "n1", Sources: 2}})

	_, body, _ := f.do(t, "GET", "/story/n1", "")
	also := body["also"].([]any)
	if len(also) != 1 || also[0].(map[string]any)["source_type"] != "official" || body["source_type"] != "news" {
		t.Fatalf("story = %v, want the UN story as official and this one as news", body)
	}
}

func TestCardsEndpointBuildsTheDeck(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()
	for hash, exam := range map[string]*store.ExamLens{
		"s1": {Prelims: 2, Cards: []store.Card{{Q: "Q1?", A: "A1"}, {Q: "Q2?", A: "A2"}}},
		"s2": {Prelims: 2, Facts: []string{"A fact"}},
	} {
		_ = f.store.Save(ctx, store.Page{URLHash: hash, URL: "https://site.test/" + hash, Title: "Story " + hash + " with a long enough headline | Site", IsArticle: true, Published: now.Add(-time.Hour)})
		_ = f.store.SetEnrichment(ctx, hash, store.Enrichment{Importance: 3, Exam: exam})
	}

	status, body, hdr := f.do(t, "GET", "/cards", "")
	cards, _ := body["cards"].([]any)
	if status != 200 || len(cards) != 3 || hdr.Get("Cache-Control") == "" {
		t.Fatalf("cards = %d %v", status, body)
	}
	ids := map[string]bool{}
	for _, c := range cards {
		m := c.(map[string]any)
		ids[m["id"].(string)] = true
		if m["story"] == nil || m["title"] == "" || m["q"] == "" || m["a"] == "" {
			t.Fatalf("card = %v", m)
		}
		if title := m["title"].(string); title[len(title)-4:] == "Site" {
			t.Fatalf("the site tail stayed in %q", title)
		}
	}
	for _, want := range []string{"s1:0", "s1:1", "s2:0"} {
		if !ids[want] {
			t.Fatalf("ids = %v, want %s (positions count per story)", ids, want)
		}
	}
	for _, q := range []string{"days=0", "days=31", "limit=0", "limit=9999", "limit=x"} {
		if status, _, _ := f.do(t, "GET", "/cards?"+q, ""); status != 400 {
			t.Errorf("/cards?%s = %d, want 400", q, status)
		}
	}
}

func TestMonthEndpoints(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_ = f.store.SaveCompilation(ctx, store.Compilation{Month: "2026-11", Track: "upsc", Areas: []store.CompArea{{Name: "Economy (GS3)", Items: []store.CompItem{{ID: "a", Title: "T"}}}}})

	status, body, hdr := f.do(t, "GET", "/month?month=2026-11&track=upsc", "")
	areas, _ := body["areas"].([]any)
	if status != 200 || len(areas) != 1 || body["month"] != "2026-11" || hdr.Get("Cache-Control") == "" {
		t.Fatalf("month = %d %v", status, body)
	}
	for path, want := range map[string]int{
		"/month?month=2026-10&track=upsc": 404, "/month?month=2026-13&track=upsc": 400, "/month?month=nope&track=upsc": 400,
		"/month?track=upsc": 400, "/month?month=2026-11&track=bogus": 400, "/month?month=2026-11": 400,
	} {
		if status, _, _ := f.do(t, "GET", path, ""); status != want {
			t.Errorf("%s = %d, want %d", path, status, want)
		}
	}

	_, body, _ = f.do(t, "GET", "/months", "")
	if months, _ := body["months"].([]any); len(months) != 1 || months[0] != "2026-11" {
		t.Fatalf("months = %v", body)
	}
}
