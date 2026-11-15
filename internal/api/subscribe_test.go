package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/frontier"
	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

// notified records the calls to Deps.Notify.
type notified struct{ emails, tokens []string }

func subscribeFixture(t *testing.T, readOnly bool) (*httptest.Server, *notified) {
	t.Helper()

	n := &notified{}
	srv := httptest.NewServer(NewHandler(Deps{
		Frontier: frontier.NewMemory(nil),
		Store:    store.NewMemory(),
		ReadOnly: readOnly,
		Notify: func(_ context.Context, email, token string) {
			n.emails, n.tokens = append(n.emails, email), append(n.tokens, token)
		},
	}))
	t.Cleanup(srv.Close)

	return srv, n
}

func post(t *testing.T, srv *httptest.Server, path, body string) int {
	t.Helper()

	resp, err := http.Post(srv.URL+path, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	return resp.StatusCode
}

func TestSubscribeThenConfirm(t *testing.T) {
	srv, n := subscribeFixture(t, true) // a ReadOnly box still takes signups

	if got := post(t, srv, "/subscribe", `{"email":" Reader@Example.COM "}`); got != http.StatusAccepted {
		t.Fatalf("subscribe = %d, want 202", got)
	}
	if len(n.tokens) != 1 || n.emails[0] != "reader@example.com" || len(n.tokens[0]) < 40 {
		t.Fatalf("Notify = %v %v, want one call with a lowercased address and a long token", n.emails, n.tokens)
	}

	// Signing up again re-sends the same token (a lost email) with the same answer.
	if got := post(t, srv, "/subscribe", `{"email":"reader@example.com"}`); got != http.StatusAccepted {
		t.Fatalf("repeat subscribe = %d, want 202", got)
	}
	if len(n.tokens) != 2 || n.tokens[1] != n.tokens[0] {
		t.Fatalf("repeat signup must re-send the first token: %v", n.tokens)
	}

	if got := post(t, srv, "/confirm", `{"token":"wrong"}`); got != http.StatusNotFound {
		t.Fatalf("confirm with a bad token = %d, want 404", got)
	}
	if got := post(t, srv, "/confirm", `{"token":"`+n.tokens[0]+`"}`); got != http.StatusOK {
		t.Fatalf("confirm = %d, want 200", got)
	}

	// Confirmed: same answer, but no further mail.
	if got := post(t, srv, "/subscribe", `{"email":"reader@example.com"}`); got != http.StatusAccepted {
		t.Fatalf("subscribe after confirm = %d, want 202", got)
	}
	if len(n.tokens) != 2 {
		t.Fatalf("a confirmed subscriber must not be mailed again: %v", n.tokens)
	}
}

func TestSubscribeRejectsBadInput(t *testing.T) {
	srv, n := subscribeFixture(t, false)

	for _, body := range []string{
		`{"email":""}`, `{"email":"nope"}`, `{"email":"a@b"}`, `{"email":"Name <a@b.co>"}`,
		`{"email":"` + strings.Repeat("a", 250) + `@b.co"}`, `not json`, `{}`,
		`{"email":"` + strings.Repeat("x", 5000) + `"}`,
	} {
		if got := post(t, srv, "/subscribe", body); got != http.StatusBadRequest {
			t.Errorf("subscribe %.40q = %d, want 400", body, got)
		}
	}
	if len(n.tokens) != 0 {
		t.Fatalf("bad input must not notify: %v", n.tokens)
	}

	for _, body := range []string{`{}`, `{"token":""}`, `not json`, `{"token":"` + strings.Repeat("x", 200) + `"}`} {
		if got := post(t, srv, "/confirm", body); got != http.StatusBadRequest {
			t.Errorf("confirm %.40q = %d, want 400", body, got)
		}
	}
}

func TestSubscribeIsRateLimited(t *testing.T) {
	srv, _ := subscribeFixture(t, false)

	for i := 0; i < signupsPerMinute; i++ {
		if got := post(t, srv, "/subscribe", `{"email":"a@example.com"}`); got != http.StatusAccepted {
			t.Fatalf("signup %d = %d, want 202", i, got)
		}
	}
	if got := post(t, srv, "/subscribe", `{"email":"a@example.com"}`); got != http.StatusTooManyRequests {
		t.Fatalf("signup over the limit = %d, want 429", got)
	}
}

func TestUnsubscribe(t *testing.T) {
	srv, n := subscribeFixture(t, true)
	post(t, srv, "/subscribe", `{"email":"reader@example.com"}`)
	tok := n.tokens[0]

	if got := post(t, srv, "/unsubscribe", `{"token":"wrong"}`); got != http.StatusNotFound {
		t.Fatalf("unsubscribe with a bad token = %d, want 404", got)
	}
	if got := post(t, srv, "/unsubscribe", `{}`); got != http.StatusBadRequest {
		t.Fatalf("unsubscribe without a token = %d, want 400", got)
	}

	// The web page posts JSON; a mail provider's one-click POST (RFC 8058) sends
	// form data with the token in the URL.
	resp, err := http.Post(srv.URL+"/unsubscribe?token="+tok, "application/x-www-form-urlencoded", strings.NewReader("List-Unsubscribe=One-Click"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("one-click unsubscribe = %d, want 200", resp.StatusCode)
	}
	if got := post(t, srv, "/unsubscribe", `{"token":"`+tok+`"}`); got != http.StatusOK {
		t.Fatalf("repeat unsubscribe = %d, want 200", got)
	}

	// An unsubscribed reader can no longer confirm, and signing up again starts over.
	if got := post(t, srv, "/confirm", `{"token":"`+tok+`"}`); got != http.StatusNotFound {
		t.Fatalf("confirm after unsubscribe = %d, want 404", got)
	}
	post(t, srv, "/subscribe", `{"email":"reader@example.com"}`)
	if len(n.tokens) != 2 || n.tokens[1] == tok {
		t.Fatalf("resubscribe must mail a new token: %v", n.tokens)
	}
}

func TestDigestDayAndDetailParams(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	today := time.Now().In(editionZone)
	noon := time.Date(today.Year(), today.Month(), today.Day(), 12, 0, 0, 0, editionZone).AddDate(0, 0, -1)
	_ = f.store.Save(ctx, store.Page{URLHash: "h1", URL: "https://a.test/1", Title: "Yesterday", IsArticle: true, Published: noon})
	_ = f.store.Save(ctx, store.Page{URLHash: "h2", URL: "https://a.test/2", Title: "Older", IsArticle: true, Published: noon.AddDate(0, 0, -3)})
	_ = f.store.SetEnrichment(ctx, "h1", store.Enrichment{Why: "Because.", Tags: []string{"rbi"}, Importance: 4, Long: "A long summary."})

	titles := func(path string) (string, string) {
		_, body, _ := f.do(t, "GET", path, "")
		items, _ := body["items"].([]any)
		var got, long string
		for _, it := range items {
			m := it.(map[string]any)
			got += m["title"].(string) + ";"
			if l, ok := m["long"].(string); ok {
				long += l
			}
		}
		return got, long
	}

	day := noon.Format("2006-01-02")
	if got, _ := titles("/digest?days=30&day=" + day); got != "Yesterday;" {
		t.Fatalf("day=%s returned %q, want only that edition day", day, got)
	}
	if got, _ := titles("/digest?days=30"); got != "Yesterday;Older;" {
		t.Fatalf("no day filter returned %q", got)
	}
	if status, _, _ := f.do(t, "GET", "/digest?day=not-a-date", ""); status != http.StatusBadRequest {
		t.Fatalf("bad day = %d, want 400", status)
	}

	if _, long := titles("/digest?days=30"); long != "" {
		t.Fatalf("long summary leaked without detail=1: %q", long)
	}
	if _, long := titles("/digest?days=30&detail=1"); long != "A long summary." {
		t.Fatalf("detail=1 long = %q", long)
	}
}

func TestSubscribeStoresPreferencesOnlyWhileUnconfirmed(t *testing.T) {
	st := store.NewMemory()
	srv := httptest.NewServer(NewHandler(Deps{Frontier: frontier.NewMemory(nil), Store: st}))
	t.Cleanup(srv.Close)
	ctx := context.Background()

	if got := post(t, srv, "/subscribe", `{"email":"a@example.com","tracks":["engineering"],"mode":"7min","topics":[" Monsoon ","monsoon"]}`); got != http.StatusAccepted {
		t.Fatalf("subscribe = %d", got)
	}
	tok, _, _ := st.Subscribe(ctx, "a@example.com", "unused")
	_, _ = st.Confirm(ctx, tok)
	subs, _ := st.ConfirmedSubscribers(ctx)
	if len(subs) != 1 || len(subs[0].Tracks) != 1 || subs[0].Tracks[0] != "engineering" || subs[0].Mode != "7min" || len(subs[0].Topics) != 1 || subs[0].Topics[0] != "monsoon" {
		t.Fatalf("subscribers = %+v, want engineering, 7min and the topic monsoon", subs)
	}

	// A later form for a confirmed address must not change what that reader gets.
	post(t, srv, "/subscribe", `{"email":"a@example.com","tracks":["upsc"],"mode":"1min"}`)
	subs, _ = st.ConfirmedSubscribers(ctx)
	if subs[0].Tracks[0] != "engineering" || subs[0].Mode != "7min" {
		t.Fatalf("a confirmed reader's choices changed: %+v", subs[0])
	}

	// No choices means every edition and the one-minute read.
	post(t, srv, "/subscribe", `{"email":"b@example.com"}`)
	tok, _, _ = st.Subscribe(ctx, "b@example.com", "unused")
	_, _ = st.Confirm(ctx, tok)
	subs, _ = st.ConfirmedSubscribers(ctx)
	if len(subs) != 2 || len(subs[1].Tracks) != 2 || subs[1].Mode != "1min" {
		t.Fatalf("defaults = %+v", subs)
	}

	for _, body := range []string{`{"email":"c@example.com","tracks":["nope"]}`, `{"email":"c@example.com","mode":"3min"}`} {
		if got := post(t, srv, "/subscribe", body); got != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", body, got)
		}
	}
}

func TestQuizEndpoint(t *testing.T) {
	f := newFixture(t)
	_ = f.store.SaveQuiz(context.Background(), store.Quiz{Day: "2026-11-14", Track: "upsc", Questions: []store.QuizQuestion{
		{Question: "Q?", Options: []string{"a", "b", "c", "d"}, Answer: 3, Explanation: "because"},
	}})

	status, body, _ := f.do(t, "GET", "/quiz?track=upsc&day=2026-11-14", "")
	qs, _ := body["questions"].([]any)
	if status != 200 || body["day"] != "2026-11-14" || len(qs) != 1 || qs[0].(map[string]any)["answer"] != float64(3) {
		t.Fatalf("quiz = %d %v", status, body)
	}
	if status, body, _ = f.do(t, "GET", "/quiz?track=upsc", ""); status != 200 || body["day"] != "2026-11-14" {
		t.Fatalf("latest quiz = %d %v", status, body)
	}
	for path, want := range map[string]int{
		"/quiz?track=upsc&day=2026-11-15": 404, "/quiz?track=engineering": 404,
		"/quiz?track=bogus": 400, "/quiz": 400, "/quiz?track=upsc&day=nope": 400,
	} {
		if got, _, _ := f.do(t, "GET", path, ""); got != want {
			t.Errorf("%s = %d, want %d", path, got, want)
		}
	}
}

func TestStoryEndpoint(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_ = f.store.Save(ctx, store.Page{URLHash: "abc123", URL: "https://www.site.test/story", Title: "A story", IsArticle: true, Category: "tech", Published: time.Now().Add(-time.Hour)})
	_ = f.store.Save(ctx, store.Page{URLHash: "listing", URL: "https://www.site.test/section/", Title: "A listing"})
	_ = f.store.SetEnrichment(ctx, "abc123", store.Enrichment{Why: "Because.", Tags: []string{"rbi"}, Importance: 3, Long: "Long text."})

	status, body, hdr := f.do(t, "GET", "/story/abc123", "")
	if status != 200 || body["title"] != "A story" || body["why"] != "Because." || body["source"] != "www.site.test" {
		t.Fatalf("story = %d %v", status, body)
	}
	if _, has := body["long"]; has {
		t.Fatalf("the long summary must not be in a story: %v", body)
	}
	if hdr.Get("Cache-Control") == "" {
		t.Fatal("a story must be cacheable")
	}
	for path, want := range map[string]int{"/story/nope": 404, "/story/listing": 404, "/story/" + strings.Repeat("x", 200): 400} {
		if got, _, _ := f.do(t, "GET", path, ""); got != want {
			t.Errorf("%s = %d, want %d", path[:min(len(path), 20)], got, want)
		}
	}
}

func TestDigestAndStoryCarryTheSourceTypeAndTheLens(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_ = f.store.Save(ctx, store.Page{URLHash: "up1", URL: "https://www.orfonline.org/research/x", Title: "A think tank report", IsArticle: true, Category: "upsc", Published: time.Now().Add(-time.Hour)})
	_ = f.store.SetEnrichment(ctx, "up1", store.Enrichment{Why: "w", Importance: 4, Exam: &store.ExamLens{Prelims: 2, Mains: 3, Facts: []string{"A fact."}, Skeleton: []string{"Intro"}}})

	_, list, _ := f.do(t, "GET", "/digest?days=2", "")
	item := list["items"].([]any)[0].(map[string]any)
	if item["source_type"] != "think-tank" {
		t.Fatalf("digest item source_type = %v, want think-tank", item["source_type"])
	}
	if rel, _ := item["rel"].([]any); len(rel) != 4 || rel[1] != float64(3) {
		t.Fatalf("digest item rel = %v, want four scores", item["rel"])
	}
	if _, has := item["exam"]; has {
		t.Fatal("the digest list must not carry the full exam lens")
	}

	_, story, _ := f.do(t, "GET", "/story/up1", "")
	exam, _ := story["exam"].(map[string]any)
	if story["source_type"] != "think-tank" || exam == nil || exam["mains"] != float64(3) || len(exam["facts"].([]any)) != 1 {
		t.Fatalf("story = %v, want source_type and the full exam lens", story)
	}
}
