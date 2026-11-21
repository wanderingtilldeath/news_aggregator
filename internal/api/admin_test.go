package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wanderingtilldeath/news-aggregator/internal/frontier"
	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

// adminMailbox records the sign-in links the API "mails".
type adminMailbox struct{ emails, tokens []string }

func adminFixture(t *testing.T, st store.Store) (*httptest.Server, *adminMailbox) {
	t.Helper()

	box := &adminMailbox{}
	srv := httptest.NewServer(NewHandler(Deps{
		Frontier:    frontier.NewMemory(nil),
		Store:       st,
		AdminEmails: []string{" Admin@X.test ", "second@x.test"},
		AdminNotify: func(_ context.Context, email, token string) {
			box.emails, box.tokens = append(box.emails, email), append(box.tokens, token)
		},
	}))
	t.Cleanup(srv.Close)

	return srv, box
}

// signIn asks for a link as the address, uses it, and returns the session.
func (b *adminMailbox) signIn(t *testing.T, srv *httptest.Server, email string) string {
	t.Helper()

	before := len(b.tokens)
	if got := post(t, srv, "/admin/login", `{"email":"`+email+`"}`); got != http.StatusAccepted {
		t.Fatalf("login = %d, want 202", got)
	}
	if len(b.tokens) != before+1 {
		t.Fatalf("no sign-in link was mailed to %s", email)
	}
	code, body := getJSONBody(t, mustReq("POST", srv.URL+"/admin/session", `{"token":"`+b.tokens[before]+`"}`))
	session, _ := body["session"].(string)
	if code != 200 || session == "" {
		t.Fatalf("session = %d %v", code, body)
	}

	return session
}

func TestOnlyAnAdminAddressIsMailedASignInLink(t *testing.T) {
	srv, box := adminFixture(t, store.NewMemory())

	// Anyone gets the same answer, but only an address on the list is mailed (matched without case or spaces).
	if got := post(t, srv, "/admin/login", `{"email":"stranger@x.test"}`); got != http.StatusAccepted {
		t.Fatalf("a stranger's login = %d, want the same 202", got)
	}
	if len(box.tokens) != 0 {
		t.Fatalf("a stranger was mailed a link: %v", box.emails)
	}
	if got := post(t, srv, "/admin/login", `{"email":"ADMIN@x.test"}`); got != http.StatusAccepted || len(box.tokens) != 1 || box.emails[0] != "admin@x.test" || len(box.tokens[0]) < 40 {
		t.Fatalf("admin login = %d, mailed %v; want one link to the lower-cased address", got, box.emails)
	}
	// Asking again within the minute mails nothing, so an admin's inbox cannot be flooded.
	if got := post(t, srv, "/admin/login", `{"email":"admin@x.test"}`); got != http.StatusAccepted || len(box.tokens) != 1 {
		t.Fatalf("second login within the gap mailed again: %v", box.emails)
	}
	if got := post(t, srv, "/admin/login", `{"email":"not an address"}`); got != http.StatusBadRequest {
		t.Fatalf("a malformed address = %d, want 400", got)
	}
}

func TestSignInLinkWorksOnceAndSessionReachesAnalytics(t *testing.T) {
	srv, box := adminFixture(t, store.NewMemory())

	// Before signing in, the analytics and /admin/me are closed.
	if code, _ := getJSONBody(t, mustReq("GET", srv.URL+"/analytics", "")); code != http.StatusUnauthorized {
		t.Fatalf("analytics without a session = %d, want 401", code)
	}
	bogus := mustReq("GET", srv.URL+"/analytics", "")
	bogus.Header.Set("Authorization", "Bearer made-up")
	if code, _ := getJSONBody(t, bogus); code != http.StatusUnauthorized {
		t.Fatalf("analytics with a made-up session = %d, want 401", code)
	}

	post(t, srv, "/admin/login", `{"email":"admin@x.test"}`)
	link := box.tokens[0]
	code, body := getJSONBody(t, mustReq("POST", srv.URL+"/admin/session", `{"token":"`+link+`"}`))
	session, _ := body["session"].(string)
	if code != 200 || session == "" || body["email"] != "admin@x.test" || body["role"] != "admin" {
		t.Fatalf("session = %d %v", code, body)
	}
	if code, _ := getJSONBody(t, mustReq("POST", srv.URL+"/admin/session", `{"token":"`+link+`"}`)); code != http.StatusUnauthorized {
		t.Fatalf("the sign-in link worked a second time (%d)", code)
	}
	if code, _ := getJSONBody(t, mustReq("POST", srv.URL+"/admin/session", `{"token":"`+session+`"}`)); code != http.StatusUnauthorized {
		t.Fatalf("a session worked as a sign-in link (%d)", code)
	}

	me := mustReq("GET", srv.URL+"/admin/me", "")
	me.Header.Set("Authorization", "Bearer "+session)
	code, body = getJSONBody(t, me)
	perms, _ := body["permissions"].([]any)
	if code != 200 || body["email"] != "admin@x.test" || len(perms) != 1 || perms[0] != "analytics:read" {
		t.Fatalf("/admin/me = %d %v", code, body)
	}
	an := mustReq("GET", srv.URL+"/analytics", "")
	an.Header.Set("Authorization", "Bearer "+session)
	if code, _ := getJSONBody(t, an); code != 200 {
		t.Fatalf("analytics with a session = %d, want 200", code)
	}

	// Signing out ends the session.
	out := mustReq("POST", srv.URL+"/admin/logout", "")
	out.Header.Set("Authorization", "Bearer "+session)
	if resp, err := http.DefaultClient.Do(out); err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("logout = %v, %v", resp, err)
	}
	if code, _ := getJSONBody(t, an); code != http.StatusUnauthorized {
		t.Fatalf("analytics after sign-out = %d, want 401", code)
	}
}

func TestRemovingAnAdminEndsTheirSessions(t *testing.T) {
	st := store.NewMemory()
	srv, box := adminFixture(t, st)
	session := box.signIn(t, srv, "second@x.test")

	// A second server over the same store, with the address taken off the list, stands for the next deploy.
	gone := httptest.NewServer(NewHandler(Deps{Frontier: frontier.NewMemory(nil), Store: st, AdminEmails: []string{"admin@x.test"}}))
	defer gone.Close()
	req := mustReq("GET", gone.URL+"/analytics", "")
	req.Header.Set("Authorization", "Bearer "+session)
	if code, _ := getJSONBody(t, req); code != http.StatusUnauthorized {
		t.Fatalf("a removed admin's session = %d, want 401", code)
	}
}

func TestRolesAndPermissions(t *testing.T) {
	if !can(roleAdmin, permAnalytics) {
		t.Fatal("an admin must be able to read analytics")
	}
	if can("", permAnalytics) || can("reader", permAnalytics) || can(roleAdmin, "nothing:else") {
		t.Fatal("an unknown role or permission must be refused")
	}
}
