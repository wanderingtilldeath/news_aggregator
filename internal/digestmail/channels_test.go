package digestmail

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/store"
	"github.com/wanderingtilldeath/news-aggregator/internal/telegram"
)

type poster struct {
	posts map[string]string
	order []string
	err   error
}

func (p *poster) Post(_ context.Context, chat, text string) error {
	if p.err != nil {
		return p.err
	}
	if p.posts == nil {
		p.posts = map[string]string{}
	}
	p.posts[chat] = text
	p.order = append(p.order, chat)

	return nil
}

func chanOpts() ChannelOptions {
	return ChannelOptions{
		Day: "2026-11-14", Window: 24 * time.Hour, Top: 8, MinArticles: 2, WebBase: "https://digest.test",
		Chats: map[string]string{"upsc": "@postpaper_upsc", "engineering": "@postpaper_eng"},
	}
}

func seedTwoTracks(t *testing.T) *store.Memory {
	t.Helper()
	m := store.NewMemory()
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		_ = m.Save(ctx, store.Page{URLHash: fmt.Sprintf("u%d", i), URL: fmt.Sprintf("https://n%d.test/%d", i, i), Title: fmt.Sprintf("India <story> & %d", i), IsArticle: true, Category: "india", Published: time.Now().Add(-time.Duration(i+1) * time.Minute)})
		_ = m.SetEnrichment(ctx, fmt.Sprintf("u%d", i), store.Enrichment{Why: fmt.Sprintf("Why <%d> matters.", i), Importance: 3})
		_ = m.Save(ctx, store.Page{URLHash: fmt.Sprintf("t%d", i), URL: fmt.Sprintf("https://e%d.test/%d", i, i), Title: fmt.Sprintf("Tech %d", i), IsArticle: true, Category: "tech", Published: time.Now().Add(-time.Duration(i+1) * time.Minute)})
	}

	return m
}

func TestChannelPostsOncePerChannelPerDayAndEscapes(t *testing.T) {
	m := seedTwoTracks(t)
	_ = m.SaveQuiz(context.Background(), store.Quiz{Day: "2026-11-14", Track: "upsc", Questions: []store.QuizQuestion{{Question: "Q", Options: []string{"a", "b", "c", "d"}}}})
	p := &poster{}

	n, err := RunChannels(context.Background(), m, p, chanOpts(), quiet)
	if err != nil || n != 2 || strings.Join(p.order, ",") != "@postpaper_upsc,@postpaper_eng" {
		t.Fatalf("first run = %d, %v, posts to %v", n, err, p.order)
	}
	up := p.posts["@postpaper_upsc"]
	if !strings.Contains(up, "India &lt;story&gt; &amp; 0") || strings.Contains(up, "<story>") || !strings.Contains(up, "<i>Why &lt;0&gt; matters.</i>") {
		t.Fatalf("feed text must be HTML-escaped:\n%s", up)
	}
	if !strings.Contains(up, "https://digest.test/quiz?track=upsc&amp;day=2026-11-14") || strings.Contains(p.posts["@postpaper_eng"], "quiz") {
		t.Fatalf("only the track with a quiz links it:\n%s", up)
	}
	if len(up) > telegram.MaxMessage {
		t.Fatalf("message is %d bytes", len(up))
	}

	if n, _ = RunChannels(context.Background(), m, p, chanOpts(), quiet); n != 0 {
		t.Fatalf("second run the same day posted %d, want 0", n)
	}
	next := chanOpts()
	next.Day = "2026-11-15"
	if n, _ = RunChannels(context.Background(), m, p, next, quiet); n != 2 {
		t.Fatalf("next day posted %d, want 2", n)
	}
}

func TestChannelsSkipUnconfiguredThinAndDryRun(t *testing.T) {
	m := seedTwoTracks(t)
	p := &poster{}

	o := chanOpts()
	o.Chats = map[string]string{"engineering": "@postpaper_eng"}
	if n, _ := RunChannels(context.Background(), m, p, o, quiet); n != 1 || len(p.posts) != 1 {
		t.Fatalf("a track without a channel must not post: %v", p.order)
	}

	o = chanOpts()
	o.MinArticles = 5
	if n, _ := RunChannels(context.Background(), m, p, o, quiet); n != 0 {
		t.Fatalf("too few stories posted %d", n)
	}

	o = chanOpts()
	o.DryRun = true
	RunChannels(context.Background(), m, p, o, quiet)
	if done, _ := m.ChannelPosted(context.Background(), "2026-11-14", "telegram:@postpaper_upsc"); done {
		t.Fatal("a dry run must record nothing")
	}
}

func TestChannelRateLimitStopsQuietlyAndFailuresDoNotBlock(t *testing.T) {
	m := seedTwoTracks(t)

	p := &poster{err: telegram.ErrRateLimited}
	if n, err := RunChannels(context.Background(), m, p, chanOpts(), quiet); err != nil || n != 0 {
		t.Fatalf("rate limited = %d, %v, want a clean stop", n, err)
	}
	p = &poster{err: errors.New("boom")}
	if n, err := RunChannels(context.Background(), m, p, chanOpts(), quiet); err != nil || n != 0 {
		t.Fatalf("failing = %d, %v", n, err)
	}
	// Nothing was recorded, so a later run posts.
	if n, _ := RunChannels(context.Background(), m, &poster{}, chanOpts(), quiet); n != 2 {
		t.Fatalf("retry posted %d, want 2", n)
	}
}

func TestTelegramTextStaysUnderTheLimit(t *testing.T) {
	var entries []entry
	for i := 0; i < 5; i++ {
		entries = append(entries, entry{Title: strings.Repeat("T", 900), URL: "https://a.test/" + fmt.Sprint(i), Why: strings.Repeat("w", 900)})
	}
	text := telegramText(section{Track: "upsc", Title: "UPSC", Entries: entries}, "2026-11-14", "https://x.test")
	if len(text) > telegram.MaxMessage {
		t.Fatalf("text is %d bytes, over the %d limit", len(text), telegram.MaxMessage)
	}
}

func TestTelegramLinksOpenTheStoryOnTheSite(t *testing.T) {
	text := telegramText(section{Track: "upsc", Title: "India", Entries: []entry{{ID: "abc123", Title: "A story", URL: "https://publisher.test/a", Why: "Because."}}}, "2026-11-14", "https://x.test")
	if !strings.Contains(text, `href="https://x.test/story/abc123"`) || strings.Contains(text, "publisher.test") {
		t.Fatalf("the post must link to the story on the site, not the publisher:\n%s", text)
	}
}
