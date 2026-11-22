package scheduler

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/frontier"
	"github.com/wanderingtilldeath/news-aggregator/internal/store"
	"github.com/wanderingtilldeath/news-aggregator/internal/summarize"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// longText is long enough that the summariser sends it to the LLM.
var longText = strings.Repeat("The council announced a new plan for transport and housing. ", 12)

func TestParseSources(t *testing.T) {
	got, err := ParseSources([]byte(`[
		{"name":"Drishti","seed":"https://www.drishtiias.com/current-affairs/","path_prefixes":["/current-affairs/"]},
		{"seed":"https://www.insightsonindia.com/","max_depth":2}
	]`))
	if err != nil || len(got) != 2 {
		t.Fatalf("ParseSources = %+v, %v", got, err)
	}
	if got[0].Depth() != 1 || got[1].Depth() != 2 {
		t.Errorf("depths = %d, %d; want the default 1 and the explicit 2", got[0].Depth(), got[1].Depth())
	}
	if got[1].Name != "www.insightsonindia.com" {
		t.Errorf("unnamed source should be named after its host, got %q", got[1].Name)
	}

	for name, bad := range map[string]string{
		"not json":  `{}`,
		"empty":     `[]`,
		"bad seed":  `[{"seed":"ftp://x"}]`,
		"no seed":   `[{"name":"x"}]`,
		"too deep":  `[{"seed":"https://a.test/","max_depth":11}]`,
		"neg depth": `[{"seed":"https://a.test/","max_depth":-1}]`,
	} {
		if _, err := ParseSources([]byte(bad)); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestSourceSpecsExpandDatesAndCarryRules(t *testing.T) {
	ist, _ := time.LoadLocation("Asia/Kolkata")
	now := time.Date(2026, 10, 19, 6, 0, 0, 0, ist)

	src := Source{
		Name:            "Drishti",
		Seed:            "https://www.drishtiias.com/news/{date:02-01-2006}",
		DaysBack:        1,
		PathPrefixes:    []string{"/news/", "/daily-updates/"},
		ArticlePrefixes: []string{"/daily-updates/"},
		SplitArticles:   true,
		ImageFromSource: true,
	}
	specs, err := src.Specs(now)
	if err != nil || len(specs) != 2 {
		t.Fatalf("Specs = %+v, %v; want today and yesterday", specs, err)
	}
	if specs[0].Seed != "https://www.drishtiias.com/news/19-10-2026" || specs[1].Seed != "https://www.drishtiias.com/news/18-10-2026" {
		t.Errorf("seeds = %s, %s", specs[0].Seed, specs[1].Seed)
	}
	for _, sp := range specs {
		if sp.MaxDepth != 1 || !sp.SplitArticles || !sp.ImageFromSource || len(sp.ArticlePrefixes) != 1 || len(sp.PathPrefixes) != 2 {
			t.Errorf("spec lost its rules: %+v", sp)
		}
	}

	// Month and year rollover.
	specs, _ = src.Specs(time.Date(2026, 3, 1, 6, 0, 0, 0, ist))
	if specs[1].Seed != "https://www.drishtiias.com/news/28-02-2026" {
		t.Errorf("rollover seed = %s", specs[1].Seed)
	}

	// No placeholder: exactly one crawl.
	plain := Source{Name: "x", Seed: "https://a.test/section/"}
	if specs, err := plain.Specs(now); err != nil || len(specs) != 1 || specs[0].Seed != "https://a.test/section/" {
		t.Errorf("plain Specs = %+v, %v", specs, err)
	}

	if _, err := (Source{Name: "x", Seed: "https://a.test/{date:nonsense}"}).Specs(now); err == nil {
		t.Error("a layout that formats to itself should be rejected")
	}
}

func TestParseSourcesValidatesTemplatesAndDaysBack(t *testing.T) {
	good := `[{"name":"d","seed":"https://a.test/n/{date:02-01-2006}","days_back":1,"article_prefixes":["/x/"],"split_articles":true}]`
	got, err := ParseSources([]byte(good))
	if err != nil || got[0].DaysBack != 1 || !got[0].SplitArticles || got[0].ArticlePrefixes[0] != "/x/" {
		t.Fatalf("ParseSources = %+v, %v", got, err)
	}
	if got[0].Seed != "https://a.test/n/{date:02-01-2006}" {
		t.Errorf("a templated seed must be kept as written, got %q", got[0].Seed)
	}

	for name, bad := range map[string]string{
		"days_back without a placeholder": `[{"seed":"https://a.test/","days_back":1}]`,
		"negative days_back":              `[{"seed":"https://a.test/{date:02-01-2006}","days_back":-1}]`,
		"huge days_back":                  `[{"seed":"https://a.test/{date:02-01-2006}","days_back":99}]`,
		"bad layout":                      `[{"seed":"https://a.test/{date:abc}"}]`,
		"not a URL after expansion":       `[{"seed":"{date:02-01-2006}"}]`,
	} {
		if _, err := ParseSources([]byte(bad)); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func newSched(t *testing.T, cfg Config, deps Deps) *Scheduler {
	t.Helper()
	if cfg.At == "" {
		cfg.At = "06:00"
	}
	if len(cfg.Sources) == 0 {
		cfg.Sources = []Source{{Name: "a", Seed: "http://a.test/"}}
	}
	deps.Logger = quiet
	s, err := New(cfg, deps)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestNewRejectsBadTimeAndNoSources(t *testing.T) {
	for _, at := range []string{"", "6", "25:00", "06:61", "ab:cd"} {
		if _, err := New(Config{At: at, Sources: []Source{{Seed: "http://a.test/"}}}, Deps{}); err == nil {
			t.Errorf("At=%q: want an error", at)
		}
	}
	if _, err := New(Config{At: "06:00"}, Deps{}); err == nil {
		t.Error("no sources: want an error")
	}
}

func TestNextRun(t *testing.T) {
	ist, _ := time.LoadLocation("Asia/Kolkata")
	s := newSched(t, Config{At: "06:00", Location: ist}, Deps{})

	for _, tt := range []struct {
		name, now, want string
	}{
		{"before the time today", "2026-10-19 05:00", "2026-10-19 06:00"},
		{"exactly at the time", "2026-10-19 06:00", "2026-10-20 06:00"},
		{"after the time", "2026-10-19 09:30", "2026-10-20 06:00"},
	} {
		now, _ := time.ParseInLocation("2006-01-02 15:04", tt.now, ist)
		if got := s.nextRun(now).In(ist).Format("2006-01-02 15:04"); got != tt.want {
			t.Errorf("%s: nextRun = %s, want %s", tt.name, got, tt.want)
		}
	}
}

// finisher completes every claimed URL so crawls report done, standing in for
// the crawler workers.
func finisher(ctx context.Context, f *frontier.Memory) {
	go func() {
		for ctx.Err() == nil {
			items, _ := f.Claim(ctx, 10, time.Minute)
			for _, it := range items {
				_ = f.Finish(ctx, it, frontier.Done, "")
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()
}

type countingSummarizer struct {
	mu    sync.Mutex
	calls int
}

func (c *countingSummarizer) Summarize(context.Context, summarize.Article) ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	return []string{"gist"}, nil
}

func TestRunOnceCrawlsThenSummarises(t *testing.T) {
	front := frontier.NewMemory(nil)
	st := store.NewMemory()
	sum := &countingSummarizer{}

	// An article the "crawl" is deemed to have found.
	_ = st.Save(context.Background(), store.Page{URLHash: "1", URL: "http://a.test/story", Title: "Story", IsArticle: true, Content: longText})

	s := newSched(t, Config{Poll: 5 * time.Millisecond, CrawlTimeout: 5 * time.Second,
		Sources: []Source{{Name: "a", Seed: "http://a.test/"}, {Name: "b", Seed: "http://b.test/"}}},
		Deps{Frontier: front, Store: st, Summarizer: sum, Claims: &MemoryClaims{}})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finisher(ctx, front)

	s.runOnce(ctx)

	crawls, _ := front.ListCrawls(ctx, 10)
	if len(crawls) != 2 {
		t.Fatalf("%d crawls started, want one per source", len(crawls))
	}
	for _, c := range crawls {
		if stats, _ := front.Stats(ctx, c.ID); stats.Active() != 0 {
			t.Errorf("crawl %d still active: %v", c.ID, stats)
		}
	}
	if sum.calls != 1 {
		t.Errorf("summariser called %d times, want 1 (after the crawls)", sum.calls)
	}
}

func TestRunOnceSummarisesEvenIfCrawlsTimeOut(t *testing.T) {
	front := frontier.NewMemory(nil) // no finisher: crawls never complete
	st := store.NewMemory()
	sum := &countingSummarizer{}
	_ = st.Save(context.Background(), store.Page{URLHash: "1", URL: "http://a.test/story", IsArticle: true, Content: longText})

	s := newSched(t, Config{Poll: 5 * time.Millisecond, CrawlTimeout: 40 * time.Millisecond},
		Deps{Frontier: front, Store: st, Summarizer: sum, Claims: &MemoryClaims{}})

	s.runOnce(context.Background())

	if sum.calls != 1 {
		t.Errorf("summariser called %d times, want 1 despite the crawl timeout", sum.calls)
	}
}

func TestRunClaimsOncePerDay(t *testing.T) {
	ist, _ := time.LoadLocation("Asia/Kolkata")
	front := frontier.NewMemory(nil)
	claims := &MemoryClaims{}

	// 09:00 IST: past 06:00 with nothing run yet, so the first start catches up.
	now := time.Date(2026, 10, 19, 9, 0, 0, 0, ist)
	mk := func() *Scheduler {
		return newSched(t, Config{Location: ist, Poll: 5 * time.Millisecond, CrawlTimeout: time.Second},
			Deps{Frontier: front, Store: store.NewMemory(), Summarizer: &countingSummarizer{}, Claims: claims,
				Now: func() time.Time { return now }})
	}

	ctx, cancel := context.WithCancel(context.Background())
	finisher(ctx, front)

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ { // two "processes"
		s := mk()
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.runIfClaimed(ctx, "test")
		}()
	}
	wg.Wait()
	cancel()

	if crawls, _ := front.ListCrawls(context.Background(), 10); len(crawls) != 1 {
		t.Fatalf("%d crawls after two racing processes, want 1", len(crawls))
	}
}

func TestRunStopsOnContextCancel(t *testing.T) {
	s := newSched(t, Config{}, Deps{Frontier: frontier.NewMemory(nil), Store: store.NewMemory(),
		Summarizer: &countingSummarizer{}, Claims: &MemoryClaims{},
		Now: func() time.Time { return time.Date(2026, 10, 19, 1, 0, 0, 0, time.UTC) }}) // before 06:00: no catch-up

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after the context was cancelled")
	}
}

func TestMemoryClaims(t *testing.T) {
	c := &MemoryClaims{}
	d := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	if ok, _ := c.Claim(context.Background(), "j", d); !ok {
		t.Fatal("first claim should win")
	}
	if ok, _ := c.Claim(context.Background(), "j", d.Add(5*time.Hour)); ok {
		t.Fatal("same day should not win twice")
	}
	if ok, _ := c.Claim(context.Background(), "j", d.AddDate(0, 0, 1)); !ok {
		t.Fatal("next day should win again")
	}
}

type fakeImages func(source string) (string, error)

func (f fakeImages) Image(_ context.Context, source string) (string, error) { return f(source) }

func TestRunOnceFetchesSourceImagesBeforeSummarising(t *testing.T) {
	front := frontier.NewMemory(nil)
	st := store.NewMemory()
	_ = st.Save(context.Background(), store.Page{
		URLHash: "1", URL: "http://a.test/story", Title: "Story", IsArticle: true, Content: longText,
		Image: "http://a.test/logo.jpg", SourceURL: "http://news.test/story",
	})

	var looked []string
	imgs := fakeImages(func(src string) (string, error) {
		looked = append(looked, src)
		return "http://news.test/lead.jpg", nil
	})

	s := newSched(t, Config{Poll: 5 * time.Millisecond, CrawlTimeout: time.Second, ImageDelay: time.Millisecond},
		Deps{Frontier: front, Store: st, Summarizer: &countingSummarizer{}, Images: imgs, Claims: &MemoryClaims{}})
	s.runOnce(context.Background())

	if len(looked) != 1 || looked[0] != "http://news.test/story" {
		t.Errorf("looked up %v, want the cited source once", looked)
	}
	if got := st.Pages()["http://a.test/story"].Image; got != "http://news.test/lead.jpg" {
		t.Errorf("image = %q, want the one from the source", got)
	}
}

func TestPrefixesCanCarryTheYear(t *testing.T) {
	src := Source{
		Name: "TechCrunch", Seed: "https://techcrunch.com/",
		PathPrefixes: []string{"/{date:2006}/"}, ArticlePrefixes: []string{"/{date:2006}/", "/news/"},
	}

	for year, want := range map[int]string{2026: "/2026/", 2027: "/2027/"} {
		specs, err := src.Specs(time.Date(year, 12, 31, 6, 0, 0, 0, time.UTC))
		if err != nil || len(specs) != 1 {
			t.Fatalf("Specs = %+v, %v", specs, err)
		}
		if specs[0].PathPrefixes[0] != want || specs[0].ArticlePrefixes[0] != want || specs[0].ArticlePrefixes[1] != "/news/" {
			t.Errorf("year %d: prefixes = %v / %v, want %s then /news/", year, specs[0].PathPrefixes, specs[0].ArticlePrefixes, want)
		}
	}

	bad := Source{Name: "x", Seed: "https://a.test/", PathPrefixes: []string{"/{date:nonsense}/"}}
	if _, err := bad.Specs(time.Now()); err == nil {
		t.Error("a bad layout in a prefix should be rejected")
	}
	if _, err := ParseSources([]byte(`[{"seed":"https://a.test/","path_prefixes":["/{date:nonsense}/"]}]`)); err == nil {
		t.Error("ParseSources should reject a bad prefix layout")
	}
}

func TestLoadSourcesCombinesSeveralFiles(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	one := write("one.json", `[{"name":"A","seed":"https://a.test/"}]`)
	two := write("two.json", `[{"name":"B","seed":"https://b.test/"},{"name":"C","seed":"https://c.test/"}]`)

	got, err := LoadSources(one + ", " + two)
	if err != nil || len(got) != 3 || got[0].Name != "A" || got[2].Name != "C" {
		t.Fatalf("LoadSources = %+v, %v; want A, B, C in order", got, err)
	}

	if _, err := LoadSources(one + "," + filepath.Join(dir, "missing.json")); err == nil {
		t.Error("a missing file must be an error, not silently skipped")
	}
	if _, err := LoadSources(""); err == nil {
		t.Error("no file at all must be an error")
	}
	bad := write("bad.json", `[{"seed":"ftp://nope"}]`)
	if _, err := LoadSources(one + "," + bad); err == nil || !strings.Contains(err.Error(), "bad.json") {
		t.Errorf("an invalid file's error should name it, got %v", err)
	}
}

func TestSourceNoteIsAllowedAndIgnored(t *testing.T) {
	got, err := ParseSources([]byte(`[{"name":"X","note":"robots.txt disallows gptbot","seed":"https://a.test/"}]`))
	if err != nil || got[0].Note != "robots.txt disallows gptbot" {
		t.Fatalf("ParseSources = %+v, %v", got, err)
	}
	specs, err := got[0].Specs(time.Now())
	if err != nil || len(specs) != 1 {
		t.Fatalf("Specs = %+v, %v", specs, err)
	}
}

// limitedOnce rate-limits its first batch, then works.
type limitedOnce struct {
	mu    sync.Mutex
	calls int
}

func (l *limitedOnce) Summarize(_ context.Context, a summarize.Article) ([]string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls++
	if l.calls == 1 {
		return nil, summarize.ErrRateLimited
	}
	return []string{"gist"}, nil
}

func TestFinalPassWaitsOutARateLimitAndFinishes(t *testing.T) {
	front := frontier.NewMemory(nil)
	st := store.NewMemory()
	_ = st.Save(context.Background(), store.Page{URLHash: "1", URL: "http://a.test/story", Title: "Story", IsArticle: true, Content: longText})

	llm := &limitedOnce{}
	s := newSched(t, Config{Poll: 5 * time.Millisecond, CrawlTimeout: time.Second, RateLimitCooldown: 20 * time.Millisecond, RateLimitRetries: 2},
		Deps{Frontier: front, Store: st, Summarizer: llm, Claims: &MemoryClaims{}})

	start := time.Now()
	s.runOnce(context.Background())

	if llm.calls < 2 {
		t.Fatalf("summariser called %d times, want a retry after the rate limit", llm.calls)
	}
	items, _, _ := st.Digest(context.Background(), store.DigestQuery{Since: time.Now().Add(-time.Hour)})
	if len(items) != 1 || len(items[0].Summary) == 0 || items[0].Summary[0] != "gist" {
		t.Errorf("the article should have its gist after the retry: %+v", items)
	}
	if time.Since(start) < 20*time.Millisecond {
		t.Errorf("the retry did not wait for the cooldown")
	}
}

type alwaysLimited struct{ calls int }

func (a *alwaysLimited) Summarize(context.Context, summarize.Article) ([]string, error) {
	a.calls++
	return nil, summarize.ErrRateLimited
}

func TestFinalPassGivesUpAfterTheConfiguredRetries(t *testing.T) {
	st := store.NewMemory()
	_ = st.Save(context.Background(), store.Page{URLHash: "1", URL: "http://a.test/story", IsArticle: true, Content: longText})

	llm := &alwaysLimited{}
	s := newSched(t, Config{Poll: 5 * time.Millisecond, CrawlTimeout: time.Second, RateLimitCooldown: time.Millisecond, RateLimitRetries: 2},
		Deps{Frontier: frontier.NewMemory(nil), Store: st, Summarizer: llm, Claims: &MemoryClaims{}})

	s.runOnce(context.Background())

	// A second rate limit after a pause means the quota is gone: the run stops asking instead of using up its retries.
	if llm.calls != 2 {
		t.Errorf("summariser called %d times, want 2 (the first try, and one after the pause)", llm.calls)
	}
	if pending, _ := st.PendingSummaries(context.Background(), time.Now().Add(-time.Hour), 10); len(pending) != 1 {
		t.Errorf("an article that never got a gist must stay pending for the next run")
	}
}

func TestSourceCategoryIsNormalisedAndPassedToTheCrawl(t *testing.T) {
	got, err := ParseSources([]byte(`[{"name":"X","category":" UPSC ","seed":"https://a.test/"}]`))
	if err != nil || got[0].Category != "upsc" {
		t.Fatalf("ParseSources = %+v, %v", got, err)
	}
	specs, _ := got[0].Specs(time.Now())
	if specs[0].Category != "upsc" {
		t.Errorf("spec category = %q, want upsc", specs[0].Category)
	}

	if _, err := ParseSources([]byte(`[{"name":"X","category":"not a slug","seed":"https://a.test/"}]`)); err == nil {
		t.Error("a category with spaces was accepted")
	}
}

func TestDropBlockedHonoursATakedownByHost(t *testing.T) {
	sources, err := ParseSources([]byte(`[
		{"name":"A news","category":"world","seed":"https://www.a.test/news/"},
		{"name":"A research","category":"world","seed":"https://www.a.test/research/"},
		{"name":"B","category":"tech","seed":"https://b.test/blog/"}
	]`))
	if err != nil {
		t.Fatal(err)
	}

	// The match ignores case, spaces and "www.", and every source on the host goes.
	kept, dropped := DropBlocked(sources, []string{" WWW.A.test ", ""})
	if len(kept) != 1 || kept[0].Name != "B" || len(dropped) != 1 || dropped[0] != "a.test" {
		t.Fatalf("kept %v, dropped %v; want only B kept and a.test dropped once", kept, dropped)
	}

	if kept, dropped := DropBlocked(sources, nil); len(kept) != 3 || len(dropped) != 0 {
		t.Fatalf("nothing blocked changed the list: %v %v", kept, dropped)
	}
	if kept, _ := DropBlocked(sources, []string{"c.test"}); len(kept) != 3 {
		t.Fatalf("an unknown host removed a source: %v", kept)
	}

	// A source seeded on another site but allowed onto the blocked one would still fetch it, so it goes too.
	wide, err := ParseSources([]byte(`[
		{"name":"Wide","category":"world","seed":"https://b.test/blog/","allowed_hosts":["b.test","WWW.A.test"]},
		{"name":"B","category":"tech","seed":"https://b.test/other/"}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	if kept, dropped := DropBlocked(wide, []string{"a.test"}); len(kept) != 1 || kept[0].Name != "B" || len(dropped) != 1 || dropped[0] != "a.test" {
		t.Fatalf("kept %v, dropped %v; want Wide dropped for its allowed host", kept, dropped)
	}
}

func TestCompileMonthBuildsTheMonthDayByDay(t *testing.T) {
	st := store.NewMemory()
	ctx := context.Background()
	ist := time.FixedZone("IST", 5*3600+1800)
	started := time.Now()
	month := started.In(ist).Format("2006-01")

	save := func(hash, cat string, imp int, tags ...string) {
		t.Helper()
		if err := st.Save(ctx, store.Page{URLHash: hash, URL: "https://site.test/" + hash, Title: "Story " + hash, IsArticle: true, Category: cat, Published: started.Add(-3 * time.Hour), ContentHash: hash}); err != nil {
			t.Fatal(err)
		}
		_ = st.SetEnrichment(ctx, hash, store.Enrichment{Why: "why", Tags: tags, Importance: imp})
	}
	save("e1", "india", 4, "gs3-economy")
	save("p1", "world", 5, "gs2-international-relations")
	save("minor", "india", 2, "gs3-economy")
	save("k1", "tech", 4, "kubernetes")

	s := newSched(t, Config{Location: ist}, Deps{Store: st})
	s.compileMonth(ctx, started)

	// The month is the India-time month of the run.
	if months, _ := st.CompilationMonths(ctx); len(months) != 1 || months[0] != month {
		t.Fatalf("months = %v, want [%s]", months, month)
	}
	upsc, ok, _ := st.GetCompilation(ctx, month, "upsc")
	if !ok || len(upsc.Areas) != 2 || upsc.Areas[0].Name != "International relations (GS2)" || upsc.Areas[1].Name != "Economy (GS3)" {
		t.Fatalf("upsc = %+v, want international relations then economy, the minor story left out", upsc)
	}
	if eng, ok, _ := st.GetCompilation(ctx, month, "engineering"); !ok || len(eng.Areas) != 1 || eng.Areas[0].Name != "Kubernetes" {
		t.Fatalf("engineering = %+v", eng)
	}

	// A second run the same day adds nothing twice.
	s.compileMonth(ctx, started)
	again, _, _ := st.GetCompilation(ctx, month, "upsc")
	if n := len(again.Areas[0].Items) + len(again.Areas[1].Items); n != 2 {
		t.Fatalf("a repeated run listed %d stories, want 2", n)
	}
}

type fakeExplainer struct{ asked []string }

func (f *fakeExplainer) Explain(_ context.Context, terms []string) (map[string]string, error) {
	f.asked = terms
	out := map[string]string{}
	for _, t := range terms {
		out[strings.ToLower(t)] = "An explainer of " + t + "."
	}

	return out, nil
}

func TestExplainConceptsWritesTheQueuedTermsOnceARun(t *testing.T) {
	st := store.NewMemory()
	ctx := context.Background()
	_ = st.AddConceptTerms(ctx, []string{"repo rate", "writ"})
	ex := &fakeExplainer{}

	newSched(t, Config{}, Deps{Store: st, Explainer: ex}).explainConcepts(ctx, freeQuota())
	if len(ex.asked) != 2 {
		t.Fatalf("asked for %v, want both queued terms in one call", ex.asked)
	}
	if all, _ := st.Concepts(ctx, 10); len(all) != 2 {
		t.Fatalf("concepts = %+v, want two explained", all)
	}

	// Without an explainer the step does nothing.
	_ = st.AddConceptTerms(ctx, []string{"gerrymandering"})
	newSched(t, Config{}, Deps{Store: st}).explainConcepts(ctx, freeQuota())
	if pending, _ := st.PendingConcepts(ctx, 10); len(pending) != 1 {
		t.Fatalf("pending = %v, want the new term still waiting", pending)
	}
}

// blocksUntilCancelled is a model that never answers: the run must not wait on it past its budget.
type blocksUntilCancelled struct{}

func (blocksUntilCancelled) Summarize(ctx context.Context, _ summarize.Article) ([]string, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestRunStopsAskingTheModelWhenItsBudgetIsUsed(t *testing.T) {
	st := store.NewMemory()
	_ = st.Save(context.Background(), store.Page{URLHash: "1", URL: "http://a.test/story", Title: "Story", IsArticle: true, Content: longText})

	s := newSched(t, Config{Poll: 5 * time.Millisecond, CrawlTimeout: 200 * time.Millisecond, LLMBudget: 150 * time.Millisecond},
		Deps{Frontier: frontier.NewMemory(nil), Store: st, Summarizer: blocksUntilCancelled{}, Claims: &MemoryClaims{}})

	start := time.Now()
	s.runOnce(context.Background())

	if took := time.Since(start); took > 5*time.Second {
		t.Fatalf("the run took %v with a model that never answers; its budget is 150ms", took)
	}
	if pending, _ := st.PendingSummaries(context.Background(), time.Now().Add(-time.Hour), 10); len(pending) != 1 {
		t.Errorf("%d pending, want the article left for the next run", len(pending))
	}
}

// Each source's crawl outcome is kept for the dashboard: articles found under its crawl make it "ok", pages with no
// article "empty", a crawl nobody finished "timeout".
func TestRunOnceRecordsHowEachSourceWentAndTimeouts(t *testing.T) {
	front := frontier.NewMemory(nil)
	st := store.NewMemory()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := newSched(t, Config{Poll: 5 * time.Millisecond, CrawlTimeout: 5 * time.Second,
		Sources: []Source{{Name: "has-articles", Seed: "http://a.test/news/"}, {Name: "quiet", Seed: "http://www.b.test/"}}},
		Deps{Frontier: front, Store: st, Summarizer: &countingSummarizer{}, Claims: &MemoryClaims{}})
	finisher(ctx, front)

	// The first crawl will get id 1: give it an article.
	_ = st.Save(ctx, store.Page{URLHash: "1", CrawlID: 1, URL: "http://a.test/news/story", Title: "Story", IsArticle: true, Content: longText})
	s.runOnce(ctx)

	health, err := st.SourceHealth(ctx, time.Now().Add(-time.Hour), time.Now().Add(-time.Hour))
	if err != nil || len(health) != 2 {
		t.Fatalf("SourceHealth = %+v, %v; want both sources", health, err)
	}
	got := map[string]store.SourceHealth{}
	for _, h := range health {
		got[h.Source] = h
	}
	if h := got["has-articles"]; h.LastOutcome != "ok" || h.Articles != 1 || h.Host != "a.test" || h.LastOK.IsZero() {
		t.Fatalf("has-articles = %+v, want ok with one article on a.test", h)
	}
	if h := got["quiet"]; h.LastOutcome != "empty" || h.Host != "b.test" || !h.LastOK.IsZero() {
		t.Fatalf("quiet = %+v, want empty on b.test (www stripped) and never ok", h)
	}

	// A crawl still running when the run gives up is a timeout.
	st2 := store.NewMemory()
	s2 := newSched(t, Config{Poll: 5 * time.Millisecond, CrawlTimeout: 40 * time.Millisecond, Sources: []Source{{Name: "stuck", Seed: "http://c.test/"}}},
		Deps{Frontier: frontier.NewMemory(nil), Store: st2, Summarizer: &countingSummarizer{}, Claims: &MemoryClaims{}})
	s2.runOnce(context.Background())
	if h, _ := st2.SourceHealth(context.Background(), time.Now().Add(-time.Hour), time.Now().Add(-time.Hour)); len(h) != 1 || h[0].LastOutcome != "timeout" {
		t.Fatalf("stuck = %+v, want a timeout", h)
	}
}

func freeQuota() *quota { return newQuota(time.Now, time.Second, 0, nil) }

// While the crawl runs the poll loop fires a summary pass every tick. A rate-limited model must not be asked
// again on every tick: the first limit pauses it, a second after the pause ends model work for the run.
func TestPollLoopDoesNotHammerARateLimitedModel(t *testing.T) {
	st := store.NewMemory()
	_ = st.Save(context.Background(), store.Page{URLHash: "1", URL: "http://a.test/story", IsArticle: true, Content: longText})

	llm := &alwaysLimited{}
	s := newSched(t, Config{Poll: 2 * time.Millisecond, CrawlTimeout: 120 * time.Millisecond, SummaryBatch: 1, RateLimitCooldown: 150 * time.Millisecond, RateLimitRetries: 5},
		Deps{Frontier: frontier.NewMemory(nil), Store: st, Summarizer: llm, Claims: &MemoryClaims{}}) // crawls never finish: about 60 ticks

	s.runOnce(context.Background())

	if llm.calls != 2 {
		t.Fatalf("a rate-limited model was asked %d times, want 2 (one limit, a pause, one more limit, then stop)", llm.calls)
	}
}

// callCounter is a summariser that reports its request count, as the Gemini client does.
type callCounter struct {
	mu    sync.Mutex
	calls int64
}

func (c *callCounter) Summarize(context.Context, summarize.Article) ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	return []string{"gist"}, nil
}

func (c *callCounter) Calls() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func TestRequestCapBoundsTheBulkPasses(t *testing.T) {
	st := store.NewMemory()
	for i := 0; i < 6; i++ {
		h := string(rune('a' + i))
		_ = st.Save(context.Background(), store.Page{URLHash: h, URL: "http://a.test/" + h, IsArticle: true, Content: longText + h})
	}
	llm := &callCounter{}
	// One article per request here (batch of 1 and a summariser that is not a batch one), cap of 3 requests.
	s := newSched(t, Config{Poll: 5 * time.Millisecond, CrawlTimeout: time.Second, SummaryBatch: 1, MaxRequests: 3},
		Deps{Frontier: frontier.NewMemory(nil), Store: st, Summarizer: llm, Claims: &MemoryClaims{}})

	s.runOnce(context.Background())

	if got := llm.Calls(); got != 3 {
		t.Fatalf("%d requests, want exactly the cap of 3", got)
	}
	if pending, _ := st.PendingSummaries(context.Background(), time.Now().Add(-time.Hour), 10); len(pending) != 3 {
		t.Fatalf("%d articles left pending, want the other 3 for the next run", len(pending))
	}

	// A negative cap means none.
	llm2 := &callCounter{}
	st2 := store.NewMemory()
	for i := 0; i < 4; i++ {
		h := string(rune('a' + i))
		_ = st2.Save(context.Background(), store.Page{URLHash: h, URL: "http://a.test/" + h, IsArticle: true, Content: longText + h})
	}
	newSched(t, Config{Poll: 5 * time.Millisecond, CrawlTimeout: time.Second, SummaryBatch: 1, MaxRequests: -1},
		Deps{Frontier: frontier.NewMemory(nil), Store: st2, Summarizer: llm2, Claims: &MemoryClaims{}}).runOnce(context.Background())
	if llm2.Calls() != 4 {
		t.Fatalf("%d requests with no cap, want 4", llm2.Calls())
	}
}

func TestQuotaPausesThenSpends(t *testing.T) {
	clock := time.Date(2026, 11, 22, 6, 0, 0, 0, time.UTC)
	q := newQuota(func() time.Time { return clock }, time.Minute, 0, nil)
	if !q.allow(true) || q.spent() {
		t.Fatal("a fresh quota must allow work")
	}
	q.strike()
	if q.allow(true) || q.allow(false) || q.wait() != time.Minute || q.spent() {
		t.Fatalf("after one limit every pass waits out the pause: wait=%v spent=%v", q.wait(), q.spent())
	}
	clock = clock.Add(time.Minute)
	if !q.allow(true) {
		t.Fatal("the pause is over")
	}
	q.ok()
	q.strike()
	clock = clock.Add(time.Minute)
	if !q.allow(true) {
		t.Fatal("a good pass in between makes the next limit a first one again")
	}
	q.strike() // a second limit in a row
	clock = clock.Add(time.Hour)
	if q.allow(true) || q.allow(false) || !q.spent() {
		t.Fatal("two limits in a row mean the quota is gone for the run, however long we wait")
	}
}

func TestQuotaLimitSpendsTheCapAcrossPasses(t *testing.T) {
	used := int64(0)
	q := newQuota(time.Now, time.Second, 10, func() int64 { return used })
	if got := q.limit(300, 6); got != 60 {
		t.Fatalf("limit with 10 requests left = %d, want 60 articles at 6 a request", got)
	}
	used = 9
	if got := q.limit(300, 6); got != 6 {
		t.Fatalf("limit with 1 request left = %d, want 6", got)
	}
	used = 10
	if q.allow(true) || !q.allow(false) {
		t.Fatal("at the cap bulk passes stop, the exempt ones go on")
	}
	if reached, first := q.overCap(); !reached || !first {
		t.Fatal("the first look at a reached cap reports it")
	}
	if _, first := q.overCap(); first {
		t.Fatal("the cap is reported once")
	}
	if got := newQuota(time.Now, time.Second, 0, nil).limit(300, 6); got != 300 {
		t.Fatalf("no cap must not shrink the limit, got %d", got)
	}
}
