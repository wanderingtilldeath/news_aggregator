package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wanderingtilldeath/news-aggregator/internal/frontier"
	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

// An account manages its newsletter only through a CONFIRMED email: the token comes back for that address's
// subscriber and for nobody else's.
func TestAccountNewsletterNeedsAConfirmedEmail(t *testing.T) {
	srv, sent := emailFixture(t)
	if code, _ := getJSONBody(t, mustReq("GET", srv.URL+"/account/newsletter", "")); code != http.StatusUnauthorized {
		t.Fatalf("logged out = %d, want 401", code)
	}
	session, _ := signup(t, srv, "alice", "a long password here")

	// No email: nothing to manage, no token.
	code, body := getJSONBody(t, authed("GET", srv.URL+"/account/newsletter", session, ""))
	if code != 200 || body["email_confirmed"] != false || body["subscribed"] != false || body["token"] != nil {
		t.Fatalf("no email = %d %v, want nothing to manage", code, body)
	}

	// Somebody subscribes with the address first; an UNCONFIRMED account email must not reveal their token.
	if got := post(t, srv, "/subscribe", `{"email":"alice@x.test"}`); got != http.StatusAccepted {
		t.Fatalf("subscribe = %d", got)
	}
	_, _ = getJSONBody(t, authed("POST", srv.URL+"/account/email", session, `{"email":"alice@x.test","current_password":"a long password here"}`))
	_, body = getJSONBody(t, authed("GET", srv.URL+"/account/newsletter", session, ""))
	if body["email_confirmed"] != false || body["token"] != nil || body["subscribed"] != false {
		t.Fatalf("an unconfirmed email = %v, want no token", body)
	}

	// Confirmed: the token of that address's subscriber comes back, whatever its state (here: pending).
	if code, _ := acct(t, srv, "/account/email/verify", `{"token":"`+sent()[0].token+`"}`); code != http.StatusNoContent {
		t.Fatalf("verify = %d", code)
	}
	_, body = getJSONBody(t, authed("GET", srv.URL+"/account/newsletter", session, ""))
	token, _ := body["token"].(string)
	if body["email"] != "alice@x.test" || body["email_confirmed"] != true || body["subscribed"] != true || token == "" {
		t.Fatalf("confirmed = %v, want the subscriber's token", body)
	}
	// The token opens the same record the manage page does.
	if code, sub := getJSONBody(t, mustReq("GET", srv.URL+"/subscription?token="+token, "")); code != 200 || sub["status"] != "pending" {
		t.Fatalf("subscription by that token = %d %v, want the pending subscriber", code, sub)
	}

	// An account whose confirmed address has no subscriber has nothing to manage yet.
	other, _ := signup(t, srv, "bob", "bob long password here")
	_, _ = getJSONBody(t, authed("POST", srv.URL+"/account/email", other, `{"email":"bob@x.test","current_password":"bob long password here"}`))
	if code, _ := acct(t, srv, "/account/email/verify", `{"token":"`+sent()[1].token+`"}`); code != http.StatusNoContent {
		t.Fatalf("verify bob = %d", code)
	}
	_, body = getJSONBody(t, authed("GET", srv.URL+"/account/newsletter", other, ""))
	if body["email_confirmed"] != true || body["subscribed"] != false || body["token"] != nil {
		t.Fatalf("no subscriber = %v, want a confirmed email and nothing subscribed", body)
	}
}

// The account's membership shows its payments, newest first, with the payment id from the receipt.
func TestAccountMembershipListsItsPayments(t *testing.T) {
	old := pbkdf2Iterations
	pbkdf2Iterations = 1000
	t.Cleanup(func() { pbkdf2Iterations = old })
	st := store.NewMemory()
	_, _, _ = st.ProExtend(t.Context(), "pay_one", "member@x.test", 31, "tok-member-0123456789")
	_, _, _ = st.ProExtend(t.Context(), "pay_two", "member@x.test", 31, "tok-member-0123456789")
	srv := httptest.NewServer(NewHandler(Deps{Frontier: frontier.NewMemory(nil), Store: st}))
	t.Cleanup(srv.Close)

	session, _ := signup(t, srv, "member", "a long password here")
	if code, _ := getJSONBody(t, authed("POST", srv.URL+"/account/pro", session, `{"payment_id":"pay_one"}`)); code != 200 {
		t.Fatalf("link pro = %d", code)
	}
	_, me := getJSONBody(t, authed("GET", srv.URL+"/account/me", session, ""))
	pro, _ := me["pro"].(map[string]any)
	list, _ := pro["payments"].([]any)
	if len(list) != 2 {
		t.Fatalf("payments = %v, want both on record", pro["payments"])
	}
	ids := map[string]bool{}
	for _, p := range list {
		m := p.(map[string]any)
		ids[m["id"].(string)] = true
		if len(m["date"].(string)) != 10 {
			t.Fatalf("payment date = %v, want YYYY-MM-DD", m["date"])
		}
	}
	if !ids["pay_one"] || !ids["pay_two"] {
		t.Fatalf("payment ids = %v", ids)
	}

	// An account with no membership has no payments.
	nobody, _ := signup(t, srv, "nobody", "another long password")
	_, me = getJSONBody(t, authed("GET", srv.URL+"/account/me", nobody, ""))
	if pro, _ := me["pro"].(map[string]any); pro["payments"] != nil {
		t.Fatalf("payments for a free account = %v", pro["payments"])
	}
}
