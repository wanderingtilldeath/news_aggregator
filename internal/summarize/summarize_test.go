package summarize

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func TestExtractiveGist(t *testing.T) {
	text := "Short. The finance ministry announced a new tax slab for salaried workers on Monday. " +
		"The change takes effect from April and affects about two crore taxpayers. A third sentence follows here with detail."

	got := ExtractiveGist("", text)
	if len(got) != 3 || !strings.HasPrefix(got[0], "The finance ministry") {
		t.Fatalf("no description: %q (want 3 bullets, the too-short sentence skipped)", got)
	}

	got = ExtractiveGist("Ministry announces new tax slab", text)
	if got[0] != "Ministry announces new tax slab" || len(got) != 3 {
		t.Fatalf("with description: %q", got)
	}

	if got := ExtractiveGist("", ""); len(got) != 0 {
		t.Fatalf("empty input gave %q", got)
	}
}

func TestTruncateWords(t *testing.T) {
	long := "A foundational strategic document issued by the Indian Army outlining authoritative operational principles, " +
		"organizational concepts, force-structuring priorities, and warfare tenets to guide military planning and execution."
	tests := []struct{ name, in, want string }{
		{"short is unchanged", "Context: a short bullet.", "Context: a short bullet."},
		{"cut after the last comma inside the limit", long,
			"A foundational strategic document issued by the Indian Army outlining authoritative operational principles, organizational concepts, force-structuring priorities…"},
		{"no comma: cut at the limit, never on a dangling word",
			"one two three four five six seven eight nine ten eleven twelve thirteen fourteen fifteen sixteen seventeen eighteen nineteen and twenty-one more",
			"one two three four five six seven eight nine ten eleven twelve thirteen fourteen fifteen sixteen seventeen eighteen nineteen…"},
		{"a cut that lands on a full stop keeps it",
			"one two three four five six seven eight nine ten eleven twelve thirteen fourteen fifteen sixteen seventeen eighteen nineteen twenty. Extra words follow",
			"one two three four five six seven eight nine ten eleven twelve thirteen fourteen fifteen sixteen seventeen eighteen nineteen twenty."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateWords(tt.in, maxBulletWords)
			if got != tt.want {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
			if n := len(strings.Fields(strings.TrimSuffix(got, "…"))); n > maxBulletWords {
				t.Errorf("%d words, limit %d", n, maxBulletWords)
			}
		})
	}
}

func TestExtractiveGistBulletsStayWithinTheWordLimit(t *testing.T) {
	text := strings.Repeat("The ministry announced a new policy for farmers across several states this week. ", 3) +
		strings.Repeat("word ", 60) + "end."
	for _, b := range ExtractiveGist(strings.Repeat("long description ", 30), text) {
		if n := len(strings.Fields(strings.TrimSuffix(b, "…"))); n > maxBulletWords {
			t.Errorf("bullet has %d words: %q", n, b)
		}
	}
}

func geminiReply(text string) string {
	return `{"candidates":[{"content":{"parts":[{"text":` + quote(text) + `}]}}]}`
}

func quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func newTestGemini(url string) *Gemini {
	g := NewGemini("test-key", "")
	g.BaseURL, g.MinInterval, g.Backoff = url, 0, time.Millisecond
	return g
}

func TestGeminiSummarize(t *testing.T) {
	var gotKey, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey, gotPath = r.Header.Get("x-goog-api-key"), r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		_, _ = io.WriteString(w, geminiReply(`["One.","Two.","Three.","Four ignored."]`))
	}))
	defer srv.Close()

	got, err := newTestGemini(srv.URL).Summarize(context.Background(), Article{Title: "Title", Content: "Body text here."})
	if err != nil || len(got) != 3 || got[0] != "One." {
		t.Fatalf("Summarize = %q, %v", got, err)
	}
	if gotKey != "test-key" || !strings.HasSuffix(gotPath, "/models/gemini-3.5-flash-lite:generateContent") {
		t.Errorf("request key=%q path=%q", gotKey, gotPath)
	}
	if !strings.Contains(gotBody, "Body text here.") || strings.Contains(gotBody, "test-key") {
		t.Errorf("body should carry the article and never the key: %s", gotBody)
	}
}

func TestGeminiNonJSONReplyFallsBackToLines(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, geminiReply("- first point\n* second point\n3. third point"))
	}))
	defer srv.Close()

	got, err := newTestGemini(srv.URL).Summarize(context.Background(), Article{Title: "T", Content: "text"})
	if err != nil || len(got) != 3 || got[1] != "second point" {
		t.Fatalf("Summarize = %q, %v", got, err)
	}
}

func TestGeminiRateLimitAndRetry(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = io.WriteString(w, geminiReply(`["ok"]`))
	}))
	defer srv.Close()

	got, err := newTestGemini(srv.URL).Summarize(context.Background(), Article{Title: "T", Content: "text"})
	if err != nil || len(got) != 1 || calls.Load() != 2 {
		t.Fatalf("after one 429: %q, %v, calls=%d", got, err, calls.Load())
	}

	always := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer always.Close()

	if _, err := newTestGemini(always.URL).Summarize(context.Background(), Article{Title: "T", Content: "text"}); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("persistent 429 gave %v, want ErrRateLimited", err)
	}
}

func TestGeminiClientErrorIsNotRetried(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	_, err := newTestGemini(srv.URL).Summarize(context.Background(), Article{Title: "T", Content: "text"})
	if err == nil || errors.Is(err, ErrRateLimited) || calls.Load() != 1 {
		t.Fatalf("err=%v calls=%d, want one non-rate-limit failure", err, calls.Load())
	}
}

type fakeSummarizer func(title string) ([]string, error)

func (f fakeSummarizer) Summarize(_ context.Context, a Article) ([]string, error) {
	return f(a.Title)
}

func seed(t *testing.T, st *store.Memory, urls ...string) {
	t.Helper()
	for _, u := range urls {
		p := store.Page{
			URLHash: u, URL: u, Title: u, IsArticle: true, Description: "meta for " + u,
			Content: "A long enough sentence about " + u + " so the extractive gist has something to use.",
		}
		if err := st.Save(context.Background(), p); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPassStoresSummariesAndSkipsDone(t *testing.T) {
	st := store.NewMemory()
	seed(t, st, "http://a.test/1", "http://a.test/2")

	llm := fakeSummarizer(func(title string) ([]string, error) { return []string{"gist of " + title}, nil })
	n, err := Pass(context.Background(), st, llm, time.Now().Add(-time.Hour), Options{Limit: 10, Workers: 1}, quiet)
	if err != nil || n != 2 {
		t.Fatalf("Pass = %d, %v", n, err)
	}

	items, _, _ := st.Digest(context.Background(), store.DigestQuery{Since: time.Now().Add(-time.Hour)})
	for _, it := range items {
		if len(it.Summary) != 1 || it.Summary[0] != "gist of "+it.URL {
			t.Errorf("%s summary = %q", it.URL, it.Summary)
		}
	}

	if n, _ := Pass(context.Background(), st, llm, time.Now().Add(-time.Hour), Options{Limit: 10, Workers: 1}, quiet); n != 0 {
		t.Errorf("second Pass summarised %d, want 0", n)
	}
}

func TestPassFallsBackOnErrorAndStopsOnRateLimit(t *testing.T) {
	st := store.NewMemory()
	seed(t, st, "http://a.test/1", "http://a.test/2", "http://a.test/3")

	calls := 0
	llm := fakeSummarizer(func(string) ([]string, error) {
		calls++
		switch calls {
		case 1:
			return nil, errors.New("boom")
		default:
			return nil, ErrRateLimited
		}
	})

	n, err := Pass(context.Background(), st, llm, time.Now().Add(-time.Hour), Options{Limit: 10, Workers: 1}, quiet)
	if !errors.Is(err, ErrRateLimited) || n != 1 {
		t.Fatalf("Pass = %d, %v; want 1 extractive summary then ErrRateLimited", n, err)
	}

	pending, _ := st.PendingSummaries(context.Background(), time.Now().Add(-time.Hour), 10)
	if len(pending) != 2 {
		t.Fatalf("%d still pending, want 2 left for the next run", len(pending))
	}

	items, _, _ := st.Digest(context.Background(), store.DigestQuery{Since: time.Now().Add(-time.Hour)})
	summarised := 0
	for _, it := range items {
		if len(it.Summary) > 0 {
			summarised++
			if !strings.HasPrefix(it.Summary[0], "meta for ") {
				t.Errorf("extractive fallback should lead with the description, got %q", it.Summary)
			}
		}
	}
	if summarised != 1 {
		t.Errorf("%d items summarised, want 1", summarised)
	}
}

func TestExtractiveProviderUsesTheDescription(t *testing.T) {
	got, err := Extractive{}.Summarize(context.Background(), Article{
		Description: "Ministry announces new tax slab",
		Content:     "The finance ministry announced a new tax slab for salaried workers on Monday morning.",
	})
	if err != nil || len(got) < 2 || got[0] != "Ministry announces new tax slab" {
		t.Fatalf("Summarize = %q, %v; want the description first", got, err)
	}
}

func TestGeminiIgnoresThoughtParts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"reasoning...","thought":true},{"text":"[\"a\",\"b\"]"}]}}]}`)
	}))
	defer srv.Close()

	got, err := newTestGemini(srv.URL).Summarize(context.Background(), Article{Title: "T", Content: "text"})
	if err != nil || len(got) != 2 || got[0] != "a" {
		t.Fatalf("Summarize = %q, %v", got, err)
	}
}

// TestGeminiLive calls the real API; it runs only with GEMINI_API_KEY set.
func TestGeminiLive(t *testing.T) {
	key := os.Getenv("GEMINI_API_KEY")
	if key == "" {
		t.Skip("GEMINI_API_KEY not set")
	}

	got, err := NewGemini(key, os.Getenv("SUMMARY_MODEL")).Summarize(context.Background(), Article{
		Title: "New tax slab announced",
		Content: "The finance ministry announced a new tax slab for salaried workers on Monday. " +
			"The change takes effect from April and affects about two crore taxpayers. " +
			"Opposition parties said the relief was too small, while industry bodies welcomed the move.",
	})
	if err != nil || len(got) == 0 {
		t.Fatalf("live Summarize = %q, %v", got, err)
	}
	t.Logf("live gist: %q", got)
}

func TestPassRunsWorkersConcurrently(t *testing.T) {
	st := store.NewMemory()
	var urls []string
	for i := 0; i < 12; i++ {
		urls = append(urls, "http://a.test/"+string(rune('a'+i)))
	}
	seed(t, st, urls...)

	var inFlight, peak atomic.Int32
	llm := fakeSummarizer(func(string) ([]string, error) {
		n := inFlight.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		inFlight.Add(-1)
		return []string{"gist"}, nil
	})

	n, err := Pass(context.Background(), st, llm, time.Now().Add(-time.Hour), Options{Limit: 100, Workers: 4}, quiet)
	if err != nil || n != 12 {
		t.Fatalf("Pass = %d, %v; want all 12", n, err)
	}
	if peak.Load() < 2 || peak.Load() > 4 {
		t.Errorf("peak concurrency %d, want between 2 and 4", peak.Load())
	}
}

func TestPassStopsAllWorkersOnRateLimit(t *testing.T) {
	st := store.NewMemory()
	var urls []string
	for i := 0; i < 20; i++ {
		urls = append(urls, "http://a.test/"+string(rune('a'+i)))
	}
	seed(t, st, urls...)

	var calls atomic.Int32
	llm := fakeSummarizer(func(string) ([]string, error) {
		calls.Add(1)
		time.Sleep(5 * time.Millisecond)
		return nil, ErrRateLimited
	})

	n, err := Pass(context.Background(), st, llm, time.Now().Add(-time.Hour), Options{Limit: 100, Workers: 3}, quiet)
	if !errors.Is(err, ErrRateLimited) || n != 0 {
		t.Fatalf("Pass = %d, %v; want 0 and ErrRateLimited", n, err)
	}
	if calls.Load() > 6 {
		t.Errorf("%d calls after the first rate limit, want the pass to stop quickly", calls.Load())
	}
	if pending, _ := st.PendingSummaries(context.Background(), time.Now().Add(-time.Hour), 100); len(pending) != 20 {
		t.Errorf("%d pending, want all 20 left for the next run", len(pending))
	}
}

func TestGeminiBatchOneRequestManyArticles(t *testing.T) {
	var calls atomic.Int32
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		// Article 2 is missing from the reply; ids are out of order; id 9 is bogus.
		_, _ = io.WriteString(w, geminiReply(`[{"id":3,"bullets":["c1","c2"]},{"id":1,"bullets":["a1"]},{"id":9,"bullets":["x"]}]`))
	}))
	defer srv.Close()

	long := strings.Repeat("word ", 2000) // 10,000 characters: must be cut per article in a batch
	got, err := newTestGemini(srv.URL).SummarizeBatch(context.Background(), []Article{
		{Title: "First", Content: "alpha text"}, {Title: "Second", Content: long}, {Title: "Third", Content: "gamma text"},
	})
	if err != nil || len(got) != 3 {
		t.Fatalf("SummarizeBatch = %q, %v", got, err)
	}
	if calls.Load() != 1 {
		t.Errorf("%d HTTP calls for 3 articles, want 1", calls.Load())
	}
	if len(got[0]) != 1 || got[0][0] != "a1" || got[1] != nil || len(got[2]) != 2 || got[2][1] != "c2" {
		t.Errorf("results mapped by id wrongly: %q", got)
	}
	for _, want := range []string{"[1] Title: First", "[2] Title: Second", "[3] Title: Third", "alpha text", "gamma text"} {
		if !strings.Contains(body, want) {
			t.Errorf("request is missing %q", want)
		}
	}
	if strings.Count(body, "word word") > 1 && strings.Count(body, "word ") > 3500 {
		t.Errorf("a long article must be capped inside a batch")
	}
}

func TestGeminiBatchGarbageIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, geminiReply(`I cannot do that`))
	}))
	defer srv.Close()

	if _, err := newTestGemini(srv.URL).SummarizeBatch(context.Background(), []Article{{Content: "a"}, {Content: "b"}}); err == nil {
		t.Fatal("a reply that is not the expected JSON must be an error")
	}
}

// batchLLM records the size of every batch it is asked for.
type batchLLM struct {
	mu     sync.Mutex
	sizes  []int
	reply  func(arts []Article) ([][]string, error)
	single atomic.Int32
}

func (b *batchLLM) Summarize(_ context.Context, a Article) ([]string, error) {
	b.single.Add(1)
	return []string{"single " + a.Title}, nil
}

func (b *batchLLM) SummarizeBatch(_ context.Context, arts []Article) ([][]string, error) {
	b.mu.Lock()
	b.sizes = append(b.sizes, len(arts))
	b.mu.Unlock()
	return b.reply(arts)
}

func echoReply(arts []Article) ([][]string, error) {
	out := make([][]string, len(arts))
	for i, a := range arts {
		out[i] = []string{"gist of " + a.Title}
	}
	return out, nil
}

func seedLong(t *testing.T, st *store.Memory, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		u := fmt.Sprintf("http://a.test/%02d", i)
		err := st.Save(context.Background(), store.Page{
			URLHash: u, URL: u, Title: u, IsArticle: true, Description: "d " + u,
			Content: strings.Repeat("A sentence long enough to be real article text. ", 20),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestPassBatchesArticlesIntoFewCalls(t *testing.T) {
	st := store.NewMemory()
	seedLong(t, st, 14)

	llm := &batchLLM{reply: echoReply}
	n, err := Pass(context.Background(), st, llm, time.Now().Add(-time.Hour), Options{Limit: 100, Workers: 2, BatchSize: 5, MinLLMRunes: 200}, quiet)
	if err != nil || n != 14 {
		t.Fatalf("Pass = %d, %v; want 14", n, err)
	}

	if len(llm.sizes) != 3 {
		t.Fatalf("%d LLM calls (%v) for 14 articles at batch size 5, want 3", len(llm.sizes), llm.sizes)
	}
	total := 0
	for _, s := range llm.sizes {
		total += s
		if s > 5 {
			t.Errorf("batch of %d exceeds the batch size", s)
		}
	}
	if total != 14 || llm.single.Load() != 0 {
		t.Errorf("batched %d articles with %d single calls, want 14 and 0", total, llm.single.Load())
	}

	items, _, _ := st.Digest(context.Background(), store.DigestQuery{Since: time.Now().Add(-time.Hour), Limit: 100})
	for _, it := range items {
		if len(it.Summary) != 1 || it.Summary[0] != "gist of "+it.URL {
			t.Errorf("%s got %q: each article must get its own gist back", it.URL, it.Summary)
		}
	}
}

func TestPassSkipsTheLLMForThinArticles(t *testing.T) {
	st := store.NewMemory()
	seedLong(t, st, 3)
	for _, u := range []string{"http://a.test/thin1", "http://a.test/thin2"} {
		_ = st.Save(context.Background(), store.Page{URLHash: u, URL: u, Title: u, IsArticle: true, Description: "A short description.", Content: "Tiny."})
	}

	llm := &batchLLM{reply: echoReply}
	n, err := Pass(context.Background(), st, llm, time.Now().Add(-time.Hour), Options{Limit: 100, Workers: 1, BatchSize: 10, MinLLMRunes: 200}, quiet)
	if err != nil || n != 5 {
		t.Fatalf("Pass = %d, %v; want all 5 summarised", n, err)
	}
	if len(llm.sizes) != 1 || llm.sizes[0] != 3 {
		t.Errorf("LLM batches %v, want one batch of the 3 real articles", llm.sizes)
	}

	items, _, _ := st.Digest(context.Background(), store.DigestQuery{Since: time.Now().Add(-time.Hour), Limit: 100})
	for _, it := range items {
		if strings.Contains(it.URL, "thin") && (len(it.Summary) == 0 || it.Summary[0] != "A short description.") {
			t.Errorf("thin article %s should get its description, got %q", it.URL, it.Summary)
		}
	}
}

func TestPassMissingEntryAndFailedBatchFallBackWithoutRetry(t *testing.T) {
	st := store.NewMemory()
	seedLong(t, st, 6)

	call := 0
	llm := &batchLLM{reply: func(arts []Article) ([][]string, error) {
		call++
		if call == 1 {
			out := make([][]string, len(arts))
			out[0] = []string{"only the first"} // the model dropped the rest
			return out, nil
		}
		return nil, errors.New("boom")
	}}

	n, err := Pass(context.Background(), st, llm, time.Now().Add(-time.Hour), Options{Limit: 100, Workers: 1, BatchSize: 3, MinLLMRunes: 200}, quiet)
	if err != nil || n != 6 {
		t.Fatalf("Pass = %d, %v; want all 6 stored", n, err)
	}
	if len(llm.sizes) != 2 {
		t.Errorf("%d LLM calls, want exactly 2: a failure must not trigger retries", len(llm.sizes))
	}

	items, _, _ := st.Digest(context.Background(), store.DigestQuery{Since: time.Now().Add(-time.Hour), Limit: 100})
	llmGists := 0
	for _, it := range items {
		if len(it.Summary) == 0 {
			t.Errorf("%s has no gist at all", it.URL)
		}
		if len(it.Summary) == 1 && it.Summary[0] == "only the first" {
			llmGists++
		}
	}
	if llmGists != 1 {
		t.Errorf("%d LLM gists, want 1; every other article falls back to its extractive gist", llmGists)
	}
}

func TestPassWaitsForAFullBatchWhenAskedTo(t *testing.T) {
	st := store.NewMemory()
	seedLong(t, st, 4)

	llm := &batchLLM{reply: echoReply}
	since := time.Now().Add(-time.Hour)

	n, err := Pass(context.Background(), st, llm, since, Options{Limit: 100, Workers: 1, BatchSize: 6, MinLLMRunes: 200, MinPending: 6}, quiet)
	if err != nil || n != 0 || len(llm.sizes) != 0 {
		t.Fatalf("with 4 pending and MinPending 6: Pass = %d, %v, calls %v; want nothing yet", n, err, llm.sizes)
	}

	seedLong(t, st, 8) // 8 distinct urls, 4 already pending: 8 pending in total
	n, err = Pass(context.Background(), st, llm, since, Options{Limit: 100, Workers: 1, BatchSize: 6, MinLLMRunes: 200, MinPending: 6}, quiet)
	if err != nil || n != 8 {
		t.Fatalf("with enough pending: Pass = %d, %v; want 8", n, err)
	}
}

func TestRetryDelayReadsHeaderAndBody(t *testing.T) {
	body := []byte(`{"error":{"code":429,"message":"Quota exceeded for metric X","details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"34s"}]}}`)

	for name, tt := range map[string]struct {
		header string
		raw    []byte
		want   time.Duration
	}{
		"body only":          {"", body, 34 * time.Second},
		"header only":        {"12", []byte(`{}`), 12 * time.Second},
		"the longer of both": {"60", body, 60 * time.Second},
		"capped":             {"900", []byte(`{}`), maxRetryHint},
		"nothing":            {"", []byte(`not json`), 0},
		"fractional seconds": {"", []byte(`{"error":{"details":[{"retryDelay":"1.5s"}]}}`), 1500 * time.Millisecond},
	} {
		if got := retryDelay(tt.header, tt.raw); got != tt.want {
			t.Errorf("%s: retryDelay = %v, want %v", name, got, tt.want)
		}
	}

	if got := apiMessage(body); got != "Quota exceeded for metric X" {
		t.Errorf("apiMessage = %q", got)
	}
}

func TestGeminiWaitsForTheServersRetryHint(t *testing.T) {
	var calls atomic.Int32
	var first, second time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			first = time.Now()
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"error":{"message":"slow down","details":[{"retryDelay":"1s"}]}}`)
			return
		}
		second = time.Now()
		_, _ = io.WriteString(w, geminiReply(`["ok"]`))
	}))
	defer srv.Close()

	g := newTestGemini(srv.URL) // Backoff is 1ms, so only the hint can explain a longer wait
	got, err := g.Summarize(context.Background(), Article{Title: "T", Content: "text"})
	if err != nil || len(got) != 1 {
		t.Fatalf("Summarize = %q, %v", got, err)
	}
	if gap := second.Sub(first); gap < 900*time.Millisecond {
		t.Errorf("retried after %v, want at least the server's 1s hint", gap)
	}
}

func TestRateLimitErrorSaysWhy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"message":"Quota exceeded for generate_content_free_tier_requests"}}`)
	}))
	defer srv.Close()

	_, err := newTestGemini(srv.URL).Summarize(context.Background(), Article{Title: "T", Content: "text"})
	if !errors.Is(err, ErrRateLimited) || !strings.Contains(err.Error(), "Quota exceeded for generate_content_free_tier_requests") {
		t.Errorf("err = %v, want ErrRateLimited that includes the API's reason", err)
	}
}
