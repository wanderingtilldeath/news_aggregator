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

func TestMemoryDigestHidesFutureDatedPages(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	now := time.Now()
	for url, when := range map[string]time.Time{"http://a.test/ok": now, "http://a.test/future": now.Add(96 * time.Hour)} {
		_ = m.Save(ctx, Page{URLHash: url, URL: url, Title: "T", IsArticle: true, Published: when})
	}
	q := DigestQuery{Since: now.Add(-time.Hour)}
	if _, total, _ := m.Digest(ctx, q); total != 1 {
		t.Errorf("digest has %d items, want 1 (the future-dated page hidden)", total)
	}
	if src, _ := m.DigestSources(ctx, q); len(src) != 1 || src[0].Count != 1 {
		t.Errorf("sources = %+v, want one row with 1", src)
	}
}

func TestMemoryDigestFiltersByCategory(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	now := time.Now()
	for i, cat := range []string{"tech", "tech", "upsc", ""} {
		_ = m.Save(ctx, Page{
			URLHash: fmt.Sprintf("h%d", i), URL: fmt.Sprintf("http://a.test/%d", i), Title: "T", IsArticle: true,
			Published: now.Add(-time.Duration(i) * time.Minute), Category: cat,
		})
	}

	since := now.Add(-time.Hour)
	for cat, want := range map[string]int{"": 4, "tech": 2, "upsc": 1, "other": 1, "ai": 0} {
		if _, total, _ := m.Digest(ctx, DigestQuery{Since: since, Category: cat}); total != want {
			t.Errorf("category %q: %d items, want %d", cat, total, want)
		}
	}

	src, _ := m.DigestSources(ctx, DigestQuery{Since: since})
	if len(src) != 3 || src[0].Category != "tech" || src[0].Count != 2 {
		t.Errorf("sources = %+v, want tech first with 2 and 3 rows", src)
	}
}

func TestMemorySubscribeAndConfirm(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()

	tok, confirmed, _ := m.Subscribe(ctx, "a@x.test", "t1")
	if tok != "t1" || confirmed {
		t.Fatalf("new subscriber = %q, %v", tok, confirmed)
	}
	if tok, _, _ = m.Subscribe(ctx, "a@x.test", "t2"); tok != "t1" {
		t.Fatalf("a repeat signup must keep the first token, got %q", tok)
	}
	if ok, _ := m.Confirm(ctx, "nope"); ok {
		t.Fatal("unknown token confirmed")
	}
	if ok, _ := m.Confirm(ctx, "t1"); !ok {
		t.Fatal("known token not confirmed")
	}
	if ok, _ := m.Confirm(ctx, "t1"); !ok {
		t.Fatal("Confirm must be safe to repeat")
	}
	if _, confirmed, _ = m.Subscribe(ctx, "a@x.test", "t3"); !confirmed {
		t.Fatal("a confirmed subscriber must stay confirmed")
	}

	// Someone who unsubscribed starts over with the new token.
	m.subscribers["a@x.test"].unsubscribed = true
	if ok, _ := m.Confirm(ctx, "t1"); ok {
		t.Fatal("an unsubscribed token confirmed")
	}
	if tok, confirmed, _ = m.Subscribe(ctx, "a@x.test", "t4"); tok != "t4" || confirmed {
		t.Fatalf("resubscribe = %q, %v, want t4, false", tok, confirmed)
	}
}

func TestMemoryUnsubscribe(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()

	_, _, _ = m.Subscribe(ctx, "a@x.test", "t1")
	if ok, _ := m.Unsubscribe(ctx, "nope"); ok {
		t.Fatal("unknown token unsubscribed")
	}
	if ok, _ := m.Unsubscribe(ctx, "t1"); !ok {
		t.Fatal("known token not unsubscribed")
	}
	if ok, _ := m.Unsubscribe(ctx, "t1"); !ok {
		t.Fatal("Unsubscribe must be safe to repeat")
	}
	if ok, _ := m.Confirm(ctx, "t1"); ok {
		t.Fatal("an unsubscribed token must not confirm")
	}
}

func TestMemoryConfirmedSubscribersAndDeliveries(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()

	for _, e := range []string{"a@x.test", "b@x.test", "c@x.test"} {
		_, _, _ = m.Subscribe(ctx, e, "t-"+e)
	}
	_, _ = m.Confirm(ctx, "t-a@x.test")
	_, _ = m.Confirm(ctx, "t-b@x.test")
	_, _ = m.Unsubscribe(ctx, "t-b@x.test")

	subs, _ := m.ConfirmedSubscribers(ctx)
	if len(subs) != 1 || subs[0].Email != "a@x.test" || subs[0].Token != "t-a@x.test" || subs[0].ID == 0 {
		t.Fatalf("ConfirmedSubscribers = %+v, want only a@x.test", subs)
	}

	_ = m.MarkDelivered(ctx, subs[0].ID, "2026-11-14")
	_ = m.MarkDelivered(ctx, subs[0].ID, "2026-11-14") // twice is fine
	if got, _ := m.DeliveredOn(ctx, "2026-11-14"); !got[subs[0].ID] || len(got) != 1 {
		t.Fatalf("DeliveredOn = %v", got)
	}
	if got, _ := m.DeliveredOn(ctx, "2026-11-15"); len(got) != 0 {
		t.Fatalf("another day = %v, want none", got)
	}
}
