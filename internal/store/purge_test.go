package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/store"
	"github.com/wanderingtilldeath/news-aggregator/internal/testutil"
)

// takedownScenario saves a page and a compilation story for each of two sites, takes one down by a spelling that
// differs from the stored host (case, www., blanks) and checks only the other site is left, then repeats it.
func takedownScenario(t *testing.T, s store.Store) {
	t.Helper()
	ctx := context.Background()
	now := time.Now()

	for hash, u := range map[string]string{"pa": "https://www.a.test/x", "pb": "https://b.test/y"} {
		if err := s.Save(ctx, store.Page{URLHash: hash, URL: u, Title: "T " + hash, IsArticle: true, Category: "india", Published: now.Add(-time.Minute)}); err != nil {
			t.Fatal(err)
		}
	}
	err := s.SaveCompilation(ctx, store.Compilation{Month: "2026-11", Track: "upsc", Areas: []store.CompArea{
		{Name: "Only A", Items: []store.CompItem{{ID: "pa", URL: "https://www.a.test/x", Title: "A"}}},
		{Name: "Mixed", Items: []store.CompItem{{ID: "pa2", URL: "https://a.test/z", Title: "A2"}, {ID: "pb", URL: "https://b.test/y", Title: "B"}}},
	}})
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.PurgeHosts(ctx, []string{" WWW.A.test ", ""})
	if err != nil || got.Pages != 1 || got.Compiled != 2 {
		t.Fatalf("PurgeHosts = %+v, %v; want 1 page and 2 compiled stories", got, err)
	}
	items, _, _ := s.Digest(ctx, store.DigestQuery{Since: now.Add(-time.Hour)})
	if len(items) != 1 || items[0].ID == "" || items[0].URL != "https://b.test/y" {
		t.Fatalf("after the takedown the digest holds %+v, want only the b.test page", items)
	}
	c, ok, err := s.GetCompilation(ctx, "2026-11", "upsc")
	if err != nil || !ok || len(c.Areas) != 1 || c.Areas[0].Name != "Mixed" || len(c.Areas[0].Items) != 1 || c.Areas[0].Items[0].ID != "pb" {
		t.Fatalf("compilation = %+v, %v, %v; want the empty area gone and only B left", c, ok, err)
	}

	// Safe to repeat, and no hosts means nothing happens.
	if got, err := s.PurgeHosts(ctx, []string{"a.test"}); err != nil || got != (store.Purged{}) {
		t.Fatalf("second PurgeHosts = %+v, %v; want nothing left to remove", got, err)
	}
	if got, err := s.PurgeHosts(ctx, nil); err != nil || got != (store.Purged{}) {
		t.Fatalf("PurgeHosts(nil) = %+v, %v", got, err)
	}
}

func TestMemoryPurgeHostsRemovesPagesAndCompiledStories(t *testing.T) {
	takedownScenario(t, store.NewMemory())
}

func TestPostgresPurgeHostsRemovesPagesAndCompiledStories(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	takedownScenario(t, s)
}
