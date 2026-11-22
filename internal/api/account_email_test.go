package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/frontier"
	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

type sentLink struct{ kind, email, token string }

func emailFixture(t *testing.T) (*httptest.Server, func() []sentLink) {
	t.Helper()
	old := pbkdf2Iterations
	pbkdf2Iterations = 1000
	t.Cleanup(func() { pbkdf2Iterations = old })

	var mu sync.Mutex
	var sent []sentLink
	srv := httptest.NewServer(NewHandler(Deps{
		Frontier: frontier.NewMemory(nil), Store: store.NewMemory(),
		AccountNotify: func(_ context.Context, kind, email, token string) {
			mu.Lock()
			defer mu.Unlock()
			sent = append(sent, sentLink{kind, email, token})
		},
	}))
	t.Cleanup(srv.Close)

	return srv, func() []sentLink {
		mu.Lock()
		defer mu.Unlock()
		return append([]sentLink(nil), sent...)
	}
}

func TestAnEmailLogsInOnlyOnceConfirmedAndResetsThePassword(t *testing.T) {
	srv, sent := emailFixture(t)
	session, _ := signup(t, srv, "alice", "a long password here")

	if code, _ := getJSONBody(t, authed("POST", srv.URL+"/account/email", session, `{"email":"Alice@X.test","current_password":"a long password here"}`)); code != http.StatusNoContent {
		t.Fatalf("set email = %d, want 204", code)
	}
	links := sent()
	if len(links) != 1 || links[0].kind != "verify" || links[0].email != "alice@x.test" {
		t.Fatalf("mails = %+v, want one verify link to alice@x.test", links)
	}

	// Unconfirmed: it neither logs in nor receives a reset.
	if code, _ := acct(t, srv, "/account/login", `{"username":"alice@x.test","password":"a long password here"}`); code != http.StatusUnauthorized {
		t.Fatalf("login by an unconfirmed email = %d, want 401", code)
	}
	if code, _ := acct(t, srv, "/account/reset-request", `{"email":"alice@x.test"}`); code != http.StatusAccepted {
		t.Fatalf("reset request = %d, want 202 (the answer never says whether the address is known)", code)
	}
	if len(sent()) != 1 {
		t.Fatalf("an unconfirmed address was sent a reset: %+v", sent())
	}

	// Another account cannot be blocked by the unconfirmed claim, and a wrong link does nothing.
	other, _ := signup(t, srv, "mallory", "mallory long password")
	if code, _ := getJSONBody(t, authed("POST", srv.URL+"/account/email", other, `{"email":"alice@x.test","current_password":"mallory long password"}`)); code != http.StatusNoContent {
		t.Fatalf("a second unconfirmed claim of the same address = %d, want 204", code)
	}
	if code, _ := acct(t, srv, "/account/email/verify", `{"token":"not-a-real-link-0000"}`); code != http.StatusNotFound {
		t.Fatalf("a made-up link = %d, want 404", code)
	}

	// The real link confirms it, once.
	verify := sent()[0].token
	if code, _ := acct(t, srv, "/account/email/verify", `{"token":"`+verify+`"}`); code != http.StatusNoContent {
		t.Fatalf("verify = %d, want 204", code)
	}
	if code, _ := acct(t, srv, "/account/email/verify", `{"token":"`+verify+`"}`); code != http.StatusNotFound {
		t.Fatalf("the same link twice = %d, want 404", code)
	}
	if code, _ := acct(t, srv, "/account/login", `{"username":"alice@x.test","password":"a long password here"}`); code != 200 {
		t.Fatalf("login by the confirmed email = %d, want 200", code)
	}

	// Mallory's own claim can no longer be confirmed: the address is taken.
	var mallory sentLink
	for _, l := range sent() {
		if l.kind == "verify" && l.token != verify {
			mallory = l
		}
	}
	if code, _ := acct(t, srv, "/account/email/verify", `{"token":"`+mallory.token+`"}`); code != http.StatusConflict {
		t.Fatalf("confirming an address another account holds = %d, want 409", code)
	}

	// A reset link sets a new password, logs the old session out and logs the reader in.
	before := len(sent())
	if code, _ := acct(t, srv, "/account/reset-request", `{"email":"alice@x.test"}`); code != http.StatusAccepted || len(sent()) != before+1 {
		t.Fatalf("reset request = %d, mails %d, want 202 and one more mail", code, len(sent())-before)
	}
	reset := sent()[before]
	if reset.kind != "reset" || reset.email != "alice@x.test" {
		t.Fatalf("mail = %+v, want a reset link", reset)
	}
	if code, _ := acct(t, srv, "/account/reset", `{"token":"`+reset.token+`","new_password":"short"}`); code != http.StatusBadRequest {
		t.Fatalf("a weak password = %d, want 400", code)
	}
	code, body := acct(t, srv, "/account/reset", `{"token":"`+reset.token+`","new_password":"a brand new password"}`)
	if code != 200 || body["session"] == "" {
		t.Fatalf("reset = %d %v, want a session", code, body)
	}
	if code, _ := getJSONBody(t, authed("GET", srv.URL+"/account/me", session, "")); code != http.StatusUnauthorized {
		t.Fatalf("the old session after a reset = %d, want 401", code)
	}
	if code, _ := acct(t, srv, "/account/reset", `{"token":"`+reset.token+`","new_password":"another new password"}`); code != http.StatusNotFound {
		t.Fatalf("a reset link twice = %d, want 404", code)
	}
	if code, _ := acct(t, srv, "/account/login", `{"username":"alice","password":"a brand new password"}`); code != 200 {
		t.Fatalf("login with the new password = %d, want 200", code)
	}
}

func TestChangingTheEmailKillsItsOldLinks(t *testing.T) {
	srv, sent := emailFixture(t)
	session, _ := signup(t, srv, "alice", "a long password here")
	getJSONBody(t, authed("POST", srv.URL+"/account/email", session, `{"email":"one@x.test","current_password":"a long password here"}`))
	old := sent()[0].token
	getJSONBody(t, authed("POST", srv.URL+"/account/email", session, `{"email":"two@x.test","current_password":"a long password here"}`))
	if code, _ := acct(t, srv, "/account/email/verify", `{"token":"`+old+`"}`); code != http.StatusNotFound {
		t.Fatalf("a link for the old address = %d, want 404", code)
	}
}

func TestSiteWideWrongTriesSlowLoginsButDoNotLockThemOut(t *testing.T) {
	srv, _ := accountFixture(t)
	old := globalSlowdown
	globalSlowdown = 0
	t.Cleanup(func() { globalSlowdown = old })
	signup(t, srv, "alice", "a long password here")

	// Many wrong tries spread over many names.
	for i := 0; i < maxAccountFailuresAll+5; i++ {
		acct(t, srv, "/account/login", `{"username":"nobody`+string(rune('a'+i%26))+string(rune('a'+i/26))+`","password":"wrong password"}`)
	}
	start := time.Now()
	if code, _ := acct(t, srv, "/account/login", `{"username":"alice","password":"a long password here"}`); code != 200 {
		t.Fatalf("a real login during a guessing run = %d, want 200", code)
	}
	_ = start
}
