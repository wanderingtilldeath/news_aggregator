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

func TestBrevoWebhookSuppressesBouncesAndComplaints(t *testing.T) {
	st := store.NewMemory()
	ctx := context.Background()
	for _, e := range []string{"gone@x.test", "angry@x.test", "fine@x.test"} {
		tok, _, _ := st.Subscribe(ctx, e, "tok-"+e)
		st.Confirm(ctx, tok)
	}
	srv := httptest.NewServer(NewHandler(Deps{Frontier: frontier.NewMemory(nil), Store: st, BrevoSecret: "s3cret"}))
	t.Cleanup(srv.Close)

	send := func(query, body string) int {
		t.Helper()
		resp, err := http.Post(srv.URL+"/brevo/webhook"+query, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()

		return resp.StatusCode
	}

	if got := send("", `{"event":"hard_bounce","email":"gone@x.test"}`); got != http.StatusUnauthorized {
		t.Fatalf("no secret = %d, want 401", got)
	}
	if got := send("?secret=wrong", `{"event":"hard_bounce","email":"gone@x.test"}`); got != http.StatusUnauthorized {
		t.Fatalf("wrong secret = %d, want 401", got)
	}
	if subs, _ := st.ConfirmedSubscribers(ctx); len(subs) != 3 {
		t.Fatalf("a rejected call changed something: %d confirmed", len(subs))
	}

	if got := send("?secret=s3cret", `{"event":"hard_bounce","email":"Gone@x.test","reason":"mailbox full"}`); got != http.StatusOK {
		t.Fatalf("hard bounce = %d", got)
	}
	if got := send("?secret=s3cret", `{"event":"spam","email":"angry@x.test"}`); got != http.StatusOK {
		t.Fatalf("spam = %d", got)
	}
	// Events that are not a failure are acknowledged and change nothing.
	if got := send("?secret=s3cret", `{"event":"delivered","email":"fine@x.test"}`); got != http.StatusOK {
		t.Fatalf("delivered = %d", got)
	}
	if got := send("?secret=s3cret", `{"event":"soft_bounce","email":"fine@x.test"}`); got != http.StatusOK {
		t.Fatalf("soft bounce = %d", got)
	}
	if got := send("?secret=s3cret", `not json`); got != http.StatusBadRequest {
		t.Fatalf("bad body = %d, want 400", got)
	}

	subs, _ := st.ConfirmedSubscribers(ctx)
	if len(subs) != 1 || subs[0].Email != "fine@x.test" {
		t.Fatalf("still mailed: %+v, want only fine@x.test", subs)
	}
}

func TestBrevoWebhookIsOffWithoutASecret(t *testing.T) {
	srv := httptest.NewServer(NewHandler(Deps{Frontier: frontier.NewMemory(nil), Store: store.NewMemory()}))
	t.Cleanup(srv.Close)

	resp, err := http.Post(srv.URL+"/brevo/webhook?secret=", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("an unconfigured webhook answered 200")
	}
}
