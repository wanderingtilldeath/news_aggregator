package summarize

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

type fakeIntro struct {
	calls   int
	stories map[string][]IntroStory
	err     error
}

func (f *fakeIntro) Intro(_ context.Context, tr string, stories []IntroStory) (string, error) {
	f.calls++
	if f.stories == nil {
		f.stories = map[string][]IntroStory{}
	}
	f.stories[tr] = stories

	return "Today the biggest story came first and the rest followed in order of weight.", f.err
}

func seedForIntro(t *testing.T, st store.Store, category string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		hash := fmt.Sprintf("%s%d", category, i)
		err := st.Save(context.Background(), store.Page{
			URLHash: hash, URL: fmt.Sprintf("https://%s.test/%d", category, i), Title: fmt.Sprintf("Headline %s %d", category, i),
			IsArticle: true, Category: category, Published: time.Now().Add(-time.Duration(i+1) * time.Minute),
		})
		if err != nil {
			t.Fatal(err)
		}
		_ = st.SetEnrichment(context.Background(), hash, store.Enrichment{Why: "Because " + hash, Importance: 1 + i%5})
	}
}

func TestIntroPassWritesOnePerTrackFromTheTopHeadlines(t *testing.T) {
	st := store.NewMemory()
	seedForIntro(t, st, "india", 8)
	seedForIntro(t, st, "tech", 2) // too few stories for the engineering track
	fi := &fakeIntro{}
	since := time.Now().Add(-24 * time.Hour)

	n, err := IntroPass(context.Background(), st, fi, since, "2026-11-15", quietLog)
	if err != nil || n != 2 || fi.calls != 2 {
		t.Fatalf("IntroPass = %d, %v after %d calls; want 2 intros: upsc and the general top news", n, err, fi.calls)
	}
	if top := fi.stories["top"]; len(top) != introStories {
		t.Fatalf("top stories = %+v, want the %d most important across every category", top, introStories)
	}
	if _, ok := fi.stories["engineering"]; ok {
		t.Fatal("engineering has too few stories and must be skipped")
	}
	got := fi.stories["upsc"]
	if len(got) != introStories || got[0].Why == "" || !strings.HasPrefix(got[0].Why, "Because india") {
		t.Fatalf("stories = %+v, want the %d most important with their why lines", got, introStories)
	}
	in, ok, _ := st.GetIntro(context.Background(), "2026-11-15", "upsc")
	if !ok || in.Text == "" {
		t.Fatalf("intro not stored: %+v", in)
	}

	// A second pass the same day spends nothing.
	if n, _ := IntroPass(context.Background(), st, fi, since, "2026-11-15", quietLog); n != 0 || fi.calls != 2 {
		t.Fatalf("second pass made %d intros in %d calls", n, fi.calls)
	}
}

func TestIntroPassStopsWhenRateLimited(t *testing.T) {
	st := store.NewMemory()
	seedForIntro(t, st, "india", 6)
	_, err := IntroPass(context.Background(), st, &fakeIntro{err: ErrRateLimited}, time.Now().Add(-time.Hour*24), "2026-11-15", quietLog)
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
}

func TestCleanIntro(t *testing.T) {
	if _, err := cleanIntro("Too short."); err == nil {
		t.Fatal("a one-line reply must be refused")
	}
	long := strings.Repeat("A sentence of seven plain words here. ", 20)
	got, err := cleanIntro("**" + long + "**")
	if err != nil || len(strings.Fields(got)) > maxIntroWords || !strings.HasSuffix(got, ".") || strings.Contains(got, "*") {
		t.Fatalf("cleanIntro = %q, %v; want plain text cut at a sentence end", got, err)
	}
}
