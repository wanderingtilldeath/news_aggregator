package store

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestMemoryPaginationMatchesTheContract(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	for i := 0; i < 12; i++ {
		img := fmt.Sprintf("http://a.test/pic-%02d.jpg", i)
		if i < 6 {
			img = "http://a.test/logo.png" // shared by 6 articles: a logo
		}
		_ = m.Save(ctx, Page{
			URLHash: fmt.Sprintf("h%02d", i), URL: fmt.Sprintf("http://a.test/%02d", i), Title: "Budget", IsArticle: true,
			Published: time.Now().Add(-time.Duration(i) * time.Minute), Image: img,
		})
	}

	since := time.Now().Add(-time.Hour)
	items, total, _ := m.Digest(ctx, DigestQuery{Since: since, Limit: 5, Offset: 10})
	if total != 12 || len(items) != 2 {
		t.Errorf("last page = %d items of %d, want 2 of 12", len(items), total)
	}
	past, total, _ := m.Digest(ctx, DigestQuery{Since: since, Limit: 5, Offset: 40})
	if len(past) != 0 || total != 12 {
		t.Errorf("past the end = %d items, total %d", len(past), total)
	}

	all, _, _ := m.Digest(ctx, DigestQuery{Since: since, Limit: 50})
	for _, it := range all {
		if it.URL <= "http://a.test/05" && it.Image != "" {
			t.Errorf("%s still shows the shared logo", it.URL)
		}
		if it.URL > "http://a.test/05" && it.Image == "" {
			t.Errorf("%s lost its own picture", it.URL)
		}
	}

	hits, total, _ := m.Search(ctx, "budget", 5, 10)
	if total != 12 || len(hits) != 2 {
		t.Errorf("search last page = %d hits of %d", len(hits), total)
	}
	if src, _ := m.DigestSources(ctx, DigestQuery{Since: since}); len(src) != 1 || src[0].Count != 12 {
		t.Errorf("sources = %+v", src)
	}
}

func TestMemoryBodyImageFallbackMatchesPostgres(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	now := time.Now()
	save := func(name, og, body string) {
		_ = m.Save(ctx, Page{URLHash: name, URL: "http://a.test/" + name, Title: name, IsArticle: true, Published: now, Image: og, BodyImage: body})
	}

	save("real", "http://a.test/real.jpg", "")
	save("fallback", "http://a.test/logo.png", "http://a.test/figure.png")
	save("bothshared", "http://a.test/logo.png", "http://a.test/mascot.png")
	save("nobody", "", "")
	for i := 0; i < 4; i++ {
		save(fmt.Sprintf("filler%d", i), "http://a.test/logo.png", "http://a.test/mascot.png")
	}
	for i := 0; i < 4; i++ { // same file as og:image and body image: counts once per article
		save(fmt.Sprintf("same%d", i), "http://a.test/same.jpg", "http://a.test/same.jpg")
	}

	items, _, _ := m.Digest(ctx, DigestQuery{Since: now.Add(-time.Hour), Limit: 50})
	got := map[string]string{}
	for _, it := range items {
		got[it.Title] = it.Image
		if img, _ := m.ImageURL(ctx, it.ID); img != it.Image {
			t.Errorf("%s: ImageURL %q != digest %q", it.Title, img, it.Image)
		}
	}
	for name, want := range map[string]string{
		"real": "http://a.test/real.jpg", "fallback": "http://a.test/figure.png",
		"bothshared": "", "nobody": "", "same0": "http://a.test/same.jpg",
	} {
		if got[name] != want {
			t.Errorf("%s: image = %q, want %q", name, got[name], want)
		}
	}
}
