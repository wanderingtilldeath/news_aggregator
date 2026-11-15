package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/store"
	"github.com/wanderingtilldeath/news-aggregator/internal/testutil"
)

// storiesContract runs the same checks on every Store: grouping is written and read the same way.
func storiesContract(t *testing.T, s store.Store) {
	ctx := context.Background()
	now := time.Now()
	for i, p := range []store.Page{
		{URLHash: "a", URL: "https://one.test/a", Title: "Donbas plan floated", IsArticle: true, Published: now.Add(-3 * time.Hour)},
		{URLHash: "b", URL: "https://two.test/b", Title: "Donbas plan reported", IsArticle: true, Published: now.Add(-2 * time.Hour)},
		{URLHash: "c", URL: "https://three.test/c", Title: "Budget day", IsArticle: true, Published: now.Add(-time.Hour)},
		{URLHash: "d", URL: "https://three.test/section/", Title: "Listing page", Published: now.Add(-time.Hour)},
	} {
		p.ContentHash = string(rune('a' + i))
		if err := s.Save(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	since := now.Add(-24 * time.Hour)

	cands, err := s.GroupCandidates(ctx, since, 10)
	if err != nil || len(cands) != 3 || cands[0].ID != "c" || cands[0].Source != "three.test" || cands[0].Title != "Budget day" {
		t.Fatalf("GroupCandidates = %+v, %v; want the 3 articles newest first", cands, err)
	}
	if few, _ := s.GroupCandidates(ctx, since, 2); len(few) != 2 {
		t.Fatalf("limit ignored: %d candidates", len(few))
	}

	groups := map[string]store.StoryGroup{"a": {ID: "a", Sources: 2}, "b": {ID: "a", Sources: 2}}
	if err := s.SetStories(ctx, since, groups); err != nil {
		t.Fatal(err)
	}

	it, ok, err := s.Item(ctx, "b")
	if err != nil || !ok || it.Coverage != 2 || len(it.Also) != 1 || it.Also[0].ID != "a" || it.Also[0].Source != "one.test" || it.Also[0].Title != "Donbas plan floated" {
		t.Fatalf("Item b = %+v, %v, %v; want coverage 2 and a as the other story", it, ok, err)
	}
	if lone, _, _ := s.Item(ctx, "c"); lone.Coverage != 0 || len(lone.Also) != 0 {
		t.Fatalf("a story with no match must carry no coverage: %+v", lone)
	}
	items, _, _ := s.Digest(ctx, store.DigestQuery{Since: since, ArticlesOnly: true, Limit: 10})
	coverage := map[string]int{}
	for _, it := range items {
		coverage[it.ID] = it.Coverage
	}
	if coverage["a"] != 2 || coverage["b"] != 2 || coverage["c"] != 0 {
		t.Fatalf("digest coverage = %v", coverage)
	}

	// A later run that no longer finds the group clears it.
	if err := s.SetStories(ctx, since, map[string]store.StoryGroup{}); err != nil {
		t.Fatal(err)
	}
	if it, _, _ := s.Item(ctx, "b"); it.Coverage != 0 || len(it.Also) != 0 {
		t.Fatalf("stale group kept: %+v", it)
	}
}

func TestMemoryStories(t *testing.T) { storiesContract(t, store.NewMemory()) }

func TestPostgresStories(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	storiesContract(t, s)
}

func introContract(t *testing.T, s store.Store) {
	ctx := context.Background()
	if _, ok, err := s.GetIntro(ctx, "", "upsc"); ok || err != nil {
		t.Fatalf("empty store: ok=%v err=%v", ok, err)
	}
	for _, in := range []store.Intro{
		{Day: "2026-11-14", Track: "upsc", Text: "Yesterday."},
		{Day: "2026-11-15", Track: "upsc", Text: "Today."},
		{Day: "2026-11-15", Track: "engineering", Text: "Code."},
	} {
		if err := s.SaveIntro(ctx, in); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SaveIntro(ctx, store.Intro{Day: "2026-11-15", Track: "upsc", Text: "Today, revised."}); err != nil {
		t.Fatal(err)
	}

	if in, ok, _ := s.GetIntro(ctx, "", "upsc"); !ok || in.Day != "2026-11-15" || in.Text != "Today, revised." {
		t.Fatalf("latest = %+v", in)
	}
	if in, ok, _ := s.GetIntro(ctx, "2026-11-14", "upsc"); !ok || in.Text != "Yesterday." {
		t.Fatalf("by day = %+v", in)
	}
	if _, ok, _ := s.GetIntro(ctx, "2026-11-13", "upsc"); ok {
		t.Fatal("a day with no intro must not fall back to another")
	}
}

func TestMemoryIntros(t *testing.T) { introContract(t, store.NewMemory()) }

func TestPostgresIntros(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	introContract(t, s)
}
