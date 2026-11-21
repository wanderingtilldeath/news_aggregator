package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wanderingtilldeath/news-aggregator/internal/frontier"
	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

func TestManageLinkIsMailedOnlyToAnActiveSubscriber(t *testing.T) {
	st := store.NewMemory()
	ctx := context.Background()
	for _, e := range []string{"active", "pending", "gone"} {
		_, _, _ = st.Subscribe(ctx, e+"@x.test", "t-"+e)
	}
	_, _ = st.Confirm(ctx, "t-active")
	_, _ = st.Confirm(ctx, "t-gone")
	_, _ = st.Unsubscribe(ctx, "t-gone")

	var emails, tokens []string
	srv := httptest.NewServer(NewHandler(Deps{
		Frontier: frontier.NewMemory(nil),
		Store:    st,
		ManageNotify: func(_ context.Context, email, token string) {
			emails, tokens = append(emails, email), append(tokens, token)
		},
	}))
	defer srv.Close()

	// Anyone gets the same answer, so it does not say who is subscribed.
	for _, e := range []string{"pending@x.test", "gone@x.test", "nobody@x.test", "ACTIVE@x.test"} {
		if code, _ := getJSONBody(t, mustReq("POST", srv.URL+"/subscription/link", `{"email":"`+e+`"}`)); code != http.StatusAccepted {
			t.Fatalf("link for %s = %d, want the same 202", e, code)
		}
	}
	if len(emails) != 1 || emails[0] != "active@x.test" || tokens[0] != "t-active" {
		t.Fatalf("mailed %v %v; want only the active subscriber, with their token", emails, tokens)
	}

	// Asking again within the gap mails nothing.
	_, _ = getJSONBody(t, mustReq("POST", srv.URL+"/subscription/link", `{"email":"active@x.test"}`))
	if len(emails) != 1 {
		t.Fatalf("a second request within the gap mailed again: %v", emails)
	}
	if code, _ := getJSONBody(t, mustReq("POST", srv.URL+"/subscription/link", `{"email":"not an address"}`)); code != http.StatusBadRequest {
		t.Fatalf("a malformed address = %d, want 400", code)
	}
}

func TestTheGeneralIntroIsServed(t *testing.T) {
	st := store.NewMemory()
	_ = st.SaveIntro(context.Background(), store.Intro{Day: "2026-11-14", Track: "top", Text: "A day of big stories."})
	srv := httptest.NewServer(NewHandler(Deps{Frontier: frontier.NewMemory(nil), Store: st}))
	defer srv.Close()

	code, body := getJSONBody(t, mustReq("GET", srv.URL+"/intro?track=top&day=2026-11-14", ""))
	if code != 200 || body["text"] != "A day of big stories." {
		t.Fatalf("GET /intro?track=top = %d %v", code, body)
	}
}
