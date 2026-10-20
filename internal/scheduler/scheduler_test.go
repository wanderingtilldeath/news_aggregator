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

	if llm.calls != 3 { // the first try plus two retries
		t.Errorf("summariser called %d times, want 3 (1 + 2 retries)", llm.calls)
	}
	if pending, _ := st.PendingSummaries(context.Background(), time.Now().Add(-time.Hour), 10); len(pending) != 1 {
		t.Errorf("an article that never got a gist must stay pending for the next run")
	}
}
