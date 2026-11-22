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

var quietLog = slog.New(slog.NewTextHandler(io.Discard, nil))

func TestParseEnrichmentDropsAnAnswerWithoutAWhyLine(t *testing.T) {
	out, err := parseEnrichment(`[{"id":1,"why":"   ","tags":["rbi"],"importance":4},{"id":2,"why":"Real reason.","tags":[],"importance":3}]`, 2)
	if err != nil {
		t.Fatal(err)
	}
	if out[0] != nil || out[1] == nil {
		t.Fatalf("a blank why must stay pending (nil) and a real one be kept, got %v / %v", out[0], out[1])
	}
}

func TestParseEnrichmentCleansTheModelsOutput(t *testing.T) {
	text := `[
	 {"id":2,"why":"  Because   it changes the budget. ","tags":["GS3 Economy","Not A Valid Tag!!","rbi","extra","more"],"importance":9},
	 {"id":1,"why":"w","tags":[],"importance":0},
	 {"id":7,"why":"out of range","tags":["x"],"importance":3}
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
	if out[0].Importance != 1 {
		t.Fatalf("a missing score must become 1, got %d", out[0].Importance)
	}
	if out[1].Long != "" || out[1].Exam != nil {
		t.Fatal("stage 1 must not carry the long summary or the lens")
	}
	if _, err := parseEnrichment("not json", 1); err == nil {
		t.Fatal("garbage accepted")
	}
}

func TestParseDetailKeepsALensForUPSCStoriesOnly(t *testing.T) {
	text := `[
	 {"id":1,"long":"` + strings.Repeat("word ", 300) + `","exam":{"prelims":7,"mains":-2,"essay":1,"ethics":2,
	   "dimensions":["Political","economic","vibes","political"],
	   "for":["Cheaper loans.","Faster growth.","Third is dropped."],"against":["Inflation risk."],
	   "facts":["RBI cut the repo rate to 5.5%.","  ","The MPC met on 6 June.","c","d","Fifth is dropped."],
	   "skeleton":["Introduction","Body","Way forward"]}},
	 {"id":2,"long":"short","exam":{"prelims":3,"mains":3,"essay":3,"ethics":3,"facts":["x"]}}
	]`
	out, err := parseDetail(text, []EnrichArticle{{Category: "india"}, {Category: "tech"}})
	if err != nil {
		t.Fatal(err)
	}

	if n := len(strings.Fields(out[0].Long)); n > maxLongWords+1 {
		t.Fatalf("long has %d words, want at most %d", n, maxLongWords)
	}
	lens := out[0].Exam
	if lens == nil {
		t.Fatal("an india story must keep its exam lens")
	}
	if lens.Prelims != 3 || lens.Mains != 0 || lens.Essay != 1 || lens.Ethics != 2 {
		t.Fatalf("scores = %d %d %d %d, want 3 0 1 2 (clamped to 0..3)", lens.Prelims, lens.Mains, lens.Essay, lens.Ethics)
	}
	if got := strings.Join(lens.Dimensions, ","); got != "political,economic" {
		t.Fatalf("dimensions = %q, want known ones only, lowercased, no repeats", got)
	}
	if len(lens.For) != 2 || len(lens.Against) != 1 || len(lens.Facts) != 4 || len(lens.Skeleton) != 3 {
		t.Fatalf("lists = for %d against %d facts %d skeleton %d, want 2 1 4 3 (empties dropped, caps applied)", len(lens.For), len(lens.Against), len(lens.Facts), len(lens.Skeleton))
	}
	if out[1].Exam != nil || out[1].Long != "short" {
		t.Fatalf("a tech story keeps its long summary but must not get a lens: %+v", out[1])
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
	g := fakeGeminiReply(t, 200, []map[string]any{{"id": 1, "why": "Matters.", "tags": []string{"gs2-polity-governance"}, "importance": 4}})
	since := time.Now().Add(-time.Hour)

	n, err := EnrichPass(context.Background(), m, g, since, EnrichOptions{Limit: 10}, quietLog)
	if err != nil || n != 1 {
		t.Fatalf("EnrichPass = %d, %v, want 1 stored", n, err)
	}
	if left, _ := m.PendingEnrichment(context.Background(), since, 10); len(left) != 1 {
		t.Fatalf("pending after the pass = %d, want the skipped article", len(left))
	}

	items, _, _ := m.Digest(context.Background(), store.DigestQuery{Since: since, ArticlesOnly: true, Detail: true})
	var found bool
	for _, it := range items {
		if it.Why == "Matters." {
			found = true
			if it.Importance != 4 || it.Tags[0] != "gs2-polity-governance" || it.Long != "" {
				t.Fatalf("item = %+v, want why, tags and importance but no long summary after stage 1", it)
			}
		}
	}
	if !found {
		t.Fatal("enrichment not visible in the digest")
	}
}

func TestDetailPassSpendsTheLongRepliesOnTheMostImportantStories(t *testing.T) {
	m := store.NewMemory()
	ctx := context.Background()
	seedSummarised(t, m, 3)
	for h, imp := range map[string]int{"a": 2, "b": 5, "c": 4} {
		_ = m.SetEnrichment(ctx, h, store.Enrichment{Why: "w", Importance: imp})
	}
	since := time.Now().Add(-time.Hour)

	// The two most important stories are b (5) and c (4); the reply answers them in that order.
	g := fakeGeminiReply(t, 200, []map[string]any{
		{"id": 1, "long": "Long text for b.", "exam": map[string]any{"prelims": 3, "mains": 2, "facts": []string{"A fact."}}},
		{"id": 2, "long": "Long text for c."},
	})
	n, err := DetailPass(ctx, m, g, since, EnrichOptions{Limit: 2}, quietLog)
	if err != nil || n != 2 {
		t.Fatalf("DetailPass = %d, %v, want 2", n, err)
	}
	if left, _ := m.PendingDetail(ctx, since, 10); len(left) != 1 || left[0].URLHash != "a" {
		t.Fatalf("left pending = %+v, want only the least important story", left)
	}

	items, _, _ := m.Digest(ctx, store.DigestQuery{Since: since, ArticlesOnly: true, Detail: true})
	long := map[string]string{}
	for _, it := range items {
		long[it.ID] = it.Long
	}
	if long["b"] != "Long text for b." || long["c"] != "Long text for c." || long["a"] != "" {
		t.Fatalf("long summaries = %v", long)
	}
	if it, _, _ := m.Item(ctx, "b"); it.Exam == nil || it.Exam.Prelims != 3 {
		t.Fatalf("the world story must keep its lens: %+v", it)
	}
	// The next pass reaches the story left over (the fake's second answer has no article to match).
	if n, _ := DetailPass(ctx, m, g, since, EnrichOptions{Limit: 2}, quietLog); n != 1 {
		t.Fatalf("a second pass should finish the one remaining story, got %d", n)
	}
	if left, _ := m.PendingDetail(ctx, since, 10); len(left) != 0 {
		t.Fatalf("nothing should be left pending, got %+v", left)
	}
}

func TestPassesStopWhenRateLimited(t *testing.T) {
	m := store.NewMemory()
	seedSummarised(t, m, 3)
	since := time.Now().Add(-time.Hour)
	g := fakeGeminiReply(t, http.StatusTooManyRequests, nil)

	if n, err := EnrichPass(context.Background(), m, g, since, EnrichOptions{Limit: 10}, quietLog); !errors.Is(err, ErrRateLimited) || n != 0 {
		t.Fatalf("EnrichPass = %d, %v, want 0 and ErrRateLimited", n, err)
	}
	_ = m.SetEnrichment(context.Background(), "a", store.Enrichment{Why: "w", Importance: 3})
	if n, err := DetailPass(context.Background(), m, g, since, EnrichOptions{Limit: 5}, quietLog); !errors.Is(err, ErrRateLimited) || n != 0 {
		t.Fatalf("DetailPass = %d, %v, want 0 and ErrRateLimited", n, err)
	}
}

func TestParseDetailKeepsCleanFlashcards(t *testing.T) {
	long := strings.Repeat("word ", 40)
	text := `[{"id":1,"long":"A summary.","exam":{"prelims":2,"cards":[
		{"q":"  Which   body  sets the repo rate? ","a":"The Monetary Policy Committee."},
		{"q":"","a":"no question, dropped"},
		{"q":"No answer, dropped","a":"  "},
		{"q":"` + long + `","a":"` + long + `"},
		{"q":"Third?","a":"Yes."},
		{"q":"Fourth is over the cap?","a":"Yes."}]}},
		{"id":2,"long":"Tech story.","exam":{"prelims":2,"cards":[{"q":"Q?","a":"A."}]}}]`
	got, err := parseDetail(text, []EnrichArticle{{Title: "RBI", Category: "india"}, {Title: "Go", Category: "tech"}})
	if err != nil {
		t.Fatal(err)
	}

	cards := got[0].Exam.Cards
	if len(cards) != maxCards || cards[0].Q != "Which body sets the repo rate?" || cards[0].A != "The Monetary Policy Committee." {
		t.Fatalf("cards = %+v, want %d clean cards starting with the repo rate question", cards, maxCards)
	}
	if n := len(strings.Fields(strings.TrimSuffix(cards[1].Q, "…"))); n > maxCardQWords {
		t.Fatalf("a question has %d words, limit %d", n, maxCardQWords)
	}
	if got[1].Exam != nil {
		t.Fatalf("a tech story must carry no lens or cards: %+v", got[1].Exam)
	}
}

func TestParseEnrichmentKeepsCleanEntities(t *testing.T) {
	text := `[{"id":1,"why":"Matters.","tags":["economy"],"importance":4,"entities":["  Sheikh   Hasina ","RBI","rbi","government","Article 370.","a very long name that has far too many words in it","Narendra Modi","Dhaka","India"]}]`
	got, err := parseEnrichment(text, 1)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Sheikh Hasina", "RBI", "Article 370", "Narendra Modi"}
	if strings.Join(got[0].Entities, "|") != strings.Join(want, "|") {
		t.Fatalf("Entities = %q, want %q (capitalised, once whatever the case, short, four at most)", got[0].Entities, want)
	}
}

func TestParseDetailKeepsCleanConceptsForEveryCategory(t *testing.T) {
	text := `[{"id":1,"long":"A summary.","concepts":["  Repo   Rate ","repo rate","writ of mandamus","a fourth term that is much too long to keep"]},{"id":2,"long":"Tech.","concepts":["Kubernetes"]}]`
	got, err := parseDetail(text, []EnrichArticle{{Title: "RBI", Category: "india"}, {Title: "Go", Category: "tech"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got[0].Concepts, "|") != "repo rate|writ of mandamus" {
		t.Fatalf("concepts = %q, want two, lower case, once each", got[0].Concepts)
	}
	if len(got[1].Concepts) != 1 || got[1].Concepts[0] != "kubernetes" {
		t.Fatalf("a tech story must carry concepts too: %q", got[1].Concepts)
	}
}

func TestDetailPassQueuesTheConceptsOfTheStoriesItDetails(t *testing.T) {
	m := store.NewMemory()
	seedSummarised(t, m, 1)
	_ = m.SetEnrichment(context.Background(), "a", store.Enrichment{Why: "w", Importance: 5})
	g := fakeGeminiReply(t, 200, []map[string]any{{"id": 1, "long": "A summary.", "concepts": []string{"Repo Rate"}}})

	if n, err := DetailPass(context.Background(), m, g, time.Now().Add(-time.Hour), EnrichOptions{Limit: 5}, quietLog); err != nil || n != 1 {
		t.Fatalf("DetailPass = %d, %v", n, err)
	}
	if pending, _ := m.PendingConcepts(context.Background(), 10); len(pending) != 1 || pending[0] != "repo rate" {
		t.Fatalf("pending concepts = %v, want the term the story named", pending)
	}
}
