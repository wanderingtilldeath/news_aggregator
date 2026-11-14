package summarize

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

func TestParseEnrichmentCleansTheModelsOutput(t *testing.T) {
	text := `[
	 {"id":2,"why":"  Because   it changes the budget. ","tags":["GS3 Economy","Not A Valid Tag!!","rbi","extra","more"],"importance":9,"long":"` + strings.Repeat("word ", 300) + `"},
	 {"id":1,"why":"w","tags":[],"importance":0,"long":"l"},
	 {"id":7,"why":"out of range","tags":["x"],"importance":3,"long":"x"}
	]`
	out, err := parseEnrichment(text, 3)
	if err != nil {
		t.Fatal(err)
	}
	if out[2] != nil {
		t.Fatal("an article the model skipped must be nil")
	}
	if out[1].Why != "Because it changes the budget." || out[1].Importance != 5 {
		t.Fatalf("why/importance = %q / %d, want whitespace collapsed and score clamped to 5", out[1].Why, out[1].Importance)
	}
	if got := strings.Join(out[1].Tags, ","); got != "gs3-economy,rbi,extra" {
		t.Fatalf("tags = %q, want valid ones only, hyphenated, at most 3", got)
	}
	if n := len(strings.Fields(out[1].Long)); n > maxLongWords+1 {
		t.Fatalf("long has %d words, want at most %d", n, maxLongWords)
	}
	if out[0].Importance != 1 {
		t.Fatalf("a missing score must become 1, got %d", out[0].Importance)
	}
	if _, err := parseEnrichment("not json", 1); err == nil {
		t.Fatal("garbage accepted")
	}
}

func fakeGeminiReply(t *testing.T, status int, items []map[string]any) *Gemini {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		raw, _ := json.Marshal(items)
		_ = json.NewEncoder(w).Encode(map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"parts": []any{map[string]string{"text": string(raw)}}}}}})
	}))
	t.Cleanup(srv.Close)

	return &Gemini{APIKey: "k", Model: "m", BaseURL: srv.URL, HTTP: srv.Client(), Backoff: time.Millisecond}
}

func seedSummarised(t *testing.T, m *store.Memory, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		h := string(rune('a' + i))
		_ = m.Save(context.Background(), store.Page{
			URLHash: h, URL: "https://a.test/" + h, Title: "Title " + h, IsArticle: true, Content: strings.Repeat("Body text. ", 50),
			Published: time.Now().Add(-time.Duration(i) * time.Minute), Category: "world",
		})
		_ = m.SetSummary(context.Background(), h, []string{"gist"})
	}
}

func TestEnrichPassStoresResultsAndLeavesSkippedPending(t *testing.T) {
	m := store.NewMemory()
	seedSummarised(t, m, 2)
	g := fakeGeminiReply(t, 200, []map[string]any{{"id": 1, "why": "Matters.", "tags": []string{"gs2-polity-governance"}, "importance": 4, "long": "Longer text."}})

	n, err := EnrichPass(context.Background(), m, g, time.Now().Add(-time.Hour), EnrichOptions{Limit: 10}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil || n != 1 {
		t.Fatalf("EnrichPass = %d, %v, want 1 stored", n, err)
	}
	if left, _ := m.PendingEnrichment(context.Background(), time.Now().Add(-time.Hour), 10); len(left) != 1 {
		t.Fatalf("pending after the pass = %d, want the skipped article", len(left))
	}

	items, _, _ := m.Digest(context.Background(), store.DigestQuery{Since: time.Now().Add(-time.Hour), ArticlesOnly: true})
	var found bool
	for _, it := range items {
		if it.Why == "Matters." {
			found = true
			if it.Importance != 4 || it.Tags[0] != "gs2-polity-governance" || it.Long != "" {
				t.Fatalf("item = %+v, want importance, tags, and no long summary without Detail", it)
			}
		}
	}
	if !found {
		t.Fatal("enrichment not visible in the digest")
	}
	detail, _, _ := m.Digest(context.Background(), store.DigestQuery{Since: time.Now().Add(-time.Hour), ArticlesOnly: true, Detail: true})
	var long string
	for _, it := range detail {
		long += it.Long
	}
	if long != "Longer text." {
		t.Fatalf("long summary with Detail = %q", long)
	}
}

func TestEnrichPassStopsWhenRateLimited(t *testing.T) {
	m := store.NewMemory()
	seedSummarised(t, m, 3)
	g := fakeGeminiReply(t, http.StatusTooManyRequests, nil)

	n, err := EnrichPass(context.Background(), m, g, time.Now().Add(-time.Hour), EnrichOptions{Limit: 10}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if !errors.Is(err, ErrRateLimited) || n != 0 {
		t.Fatalf("EnrichPass = %d, %v, want 0 and ErrRateLimited", n, err)
	}
}
