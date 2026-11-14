package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
