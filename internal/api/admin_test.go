package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/frontier"
	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

const testAdminPassword = "correct horse battery"

// admin signs in as the accounts of a fixture: the admin accounts "boss" and "second" are on the list, "reader" is not.
type admin struct{}

func adminFixture(t *testing.T, st store.Store) (*httptest.Server, admin) {
	t.Helper()

	old := pbkdf2Iterations
	pbkdf2Iterations = 1000 // the real count is for production; the tests only need the same code path
	t.Cleanup(func() { pbkdf2Iterations = old })

	srv := httptest.NewServer(NewHandler(Deps{
		Frontier:       frontier.NewMemory(nil),
		Store:          st,
		AdminUsernames: []string{" Boss ", "second"},
		AdminPassword:  testAdminPassword,
	}))
	t.Cleanup(srv.Close)

	return srv, admin{}
}

// account makes (or logs in to) the account and returns its session.
func (admin) account(t *testing.T, srv *httptest.Server, username string) string {
	t.Helper()

	code, body := getJSONBody(t, mustReq("POST", srv.URL+"/account/signup", fmt.Sprintf(`{"username":%q,"password":"an account password"}`, username)))
	if code == http.StatusConflict {
		code, body = getJSONBody(t, mustReq("POST", srv.URL+"/account/login", fmt.Sprintf(`{"username":%q,"password":"an account password"}`, username)))
	}
	session, _ := body["session"].(string)
	if code != 200 || session == "" {
		t.Fatalf("account %s = %d %v", username, code, body)
	}

	return session
}

// login is the admin step-up: the account's session and the admin password.
func login(t *testing.T, srv *httptest.Server, accountSession, password string) (int, map[string]any) {
	t.Helper()

	return getJSONBody(t, authed("POST", srv.URL+"/admin/login", accountSession, fmt.Sprintf(`{"password":%q}`, password)))
}

// signIn logs in to the account, gives the right password and returns the admin session.
func (a admin) signIn(t *testing.T, srv *httptest.Server, username string) string {
	t.Helper()

	code, body := login(t, srv, a.account(t, srv, username), testAdminPassword)
	session, _ := body["session"].(string)
	if code != 200 || session == "" {
		t.Fatalf("admin sign-in as %s = %d %v", username, code, body)
	}

	return session
}

func authed(method, url, session, body string) *http.Request {
	req := mustReq(method, url, body)
	req.Header.Set("Authorization", "Bearer "+session)

	return req
}

func TestAdminNeedsAnAdminAccountAndThePassword(t *testing.T) {
	srv, a := adminFixture(t, store.NewMemory())

	// Before signing in everything is closed.
	for _, path := range []string{"/analytics", "/admin/me", "/admin/content", "/admin/subscribers"} {
		if code, _ := getJSONBody(t, mustReq("GET", srv.URL+path, "")); code != http.StatusUnauthorized {
			t.Fatalf("%s without a session = %d, want 401", path, code)
		}
	}
	if code, _ := getJSONBody(t, authed("GET", srv.URL+"/analytics", "made-up", "")); code != http.StatusUnauthorized {
		t.Fatalf("a made-up session = %d, want 401", code)
	}

	// The step-up needs a logged-in account first.
	if code, _ := getJSONBody(t, mustReq("POST", srv.URL+"/admin/login", fmt.Sprintf(`{"password":%q}`, testAdminPassword))); code != http.StatusUnauthorized {
		t.Fatalf("the admin password with no account session = %d, want 401", code)
	}

	// The account page can tell an admin account from the others (it only offers the step-up to one).
	boss := a.account(t, srv, "boss")
	if _, me := getJSONBody(t, authed("GET", srv.URL+"/account/me", boss, "")); fmt.Sprint(me["roles"]) != "[account admin]" {
		t.Fatalf("an admin account's roles = %v, want [account admin]", me["roles"])
	}
	if _, me := getJSONBody(t, authed("GET", srv.URL+"/account/me", a.account(t, srv, "reader"), "")); fmt.Sprint(me["roles"]) != "[account]" {
		t.Fatalf("a reader's roles = %v, want [account]", me["roles"])
	}

	// An account on the list, with the right password, gets an admin session (the username matched without case or spaces).
	code, body := login(t, srv, boss, testAdminPassword)
	if code != 200 || body["username"] != "boss" || body["role"] != "admin" || len(body["session"].(string)) < 40 {
		t.Fatalf("sign-in = %d %v", code, body)
	}

	// A wrong password for an admin account, and the right password for an account that is not an admin, get one answer.
	reader := a.account(t, srv, "reader")
	var messages []string
	for name, c := range map[string]struct{ session, password string }{
		"wrong password":   {boss, "wrong password!!"},
		"not an admin":     {reader, testAdminPassword},
		"not admin, wrong": {reader, "wrong password!!"},
	} {
		code, body := login(t, srv, c.session, c.password)
		if code != http.StatusUnauthorized {
			t.Fatalf("%s = %d, want 401", name, code)
		}
		messages = append(messages, fmt.Sprint(body["error"]))
	}
	if messages[0] != messages[1] || messages[1] != messages[2] {
		t.Fatalf("the answers differ and would say which accounts are admins: %q", messages)
	}

	session := a.signIn(t, srv, "boss")
	code, body = getJSONBody(t, authed("GET", srv.URL+"/admin/me", session, ""))
	perms, _ := body["permissions"].([]any)
	if code != 200 || body["username"] != "boss" || len(perms) != 4 {
		t.Fatalf("/admin/me = %d %v", code, body)
	}
	// The account session alone does not open the admin routes: the step-up is a separate session.
	if code, _ := getJSONBody(t, authed("GET", srv.URL+"/admin/me", boss, "")); code != http.StatusUnauthorized {
		t.Fatalf("an account session on an admin route = %d, want 401", code)
	}

	// Signing out ends the admin session.
	if resp, err := http.DefaultClient.Do(authed("POST", srv.URL+"/admin/logout", session, "")); err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("logout = %v, %v", resp, err)
	}
	if code, _ := getJSONBody(t, authed("GET", srv.URL+"/admin/me", session, "")); code != http.StatusUnauthorized {
		t.Fatalf("after sign-out = %d, want 401", code)
	}
}

func TestRepeatedWrongPasswordsLockTheAccountOut(t *testing.T) {
	srv, a := adminFixture(t, store.NewMemory())
	boss, second := a.account(t, srv, "boss"), a.account(t, srv, "second")

	for i := 0; i < maxFailuresPerEmail; i++ {
		if code, _ := login(t, srv, boss, "wrong password!!"); code != http.StatusUnauthorized {
			t.Fatalf("attempt %d = %d, want 401", i+1, code)
		}
	}
	// Past the limit even the right password waits; another admin account is not affected.
	if code, _ := login(t, srv, boss, testAdminPassword); code != http.StatusTooManyRequests {
		t.Fatalf("after %d wrong tries, the right password = %d, want 429", maxFailuresPerEmail, code)
	}
	if code, _ := login(t, srv, second, testAdminPassword); code != 200 {
		t.Fatalf("another admin = %d, want 200", code)
	}
}

func TestFailureCountsExpireAndClear(t *testing.T) {
	var a attempts
	t0 := time.Now()
	a.fail("x", t0)
	a.fail("x", t0.Add(time.Minute))
	if n := a.recent("x", t0.Add(2*time.Minute)); n != 2 {
		t.Fatalf("recent = %d, want 2", n)
	}
	if n := a.recent("x", t0.Add(loginWindow+30*time.Second)); n != 1 {
		t.Fatalf("after the window the first failure must be forgotten, recent = %d, want 1", n)
	}
	a.clear("x")
	if n := a.recent("x", t0); n != 0 {
		t.Fatalf("after clear recent = %d, want 0", n)
	}
}

func TestAdminRoutesNeedAUsernameAndALongPassword(t *testing.T) {
	for name, d := range map[string]Deps{
		"nothing set":        {},
		"password too short": {AdminUsernames: []string{"boss"}, AdminPassword: "short"},
		"no username":        {AdminPassword: testAdminPassword},
	} {
		d.Frontier, d.Store = frontier.NewMemory(nil), store.NewMemory()
		srv := httptest.NewServer(NewHandler(d))
		for _, path := range []string{"/analytics", "/admin/me", "/admin/content", "/admin/subscribers"} {
			if code, _ := getJSONBody(t, mustReq("GET", srv.URL+path, "")); code != http.StatusNotFound {
				t.Errorf("%s: %s = %d, want 404 (the route must not exist)", name, path, code)
			}
		}
		srv.Close()
	}
}

func TestRemovingAnAdminEndsTheirSessions(t *testing.T) {
	st := store.NewMemory()
	srv, a := adminFixture(t, st)
	session := a.signIn(t, srv, "second")

	// A second server over the same store, with the username taken off the list, stands for the next deploy.
	gone := httptest.NewServer(NewHandler(Deps{Frontier: frontier.NewMemory(nil), Store: st, AdminUsernames: []string{"boss"}, AdminPassword: testAdminPassword}))
	defer gone.Close()
	if code, _ := getJSONBody(t, authed("GET", gone.URL+"/analytics", session, "")); code != http.StatusUnauthorized {
		t.Fatalf("a removed admin's session = %d, want 401", code)
	}
}

func TestRolesAndPermissions(t *testing.T) {
	for _, p := range []string{permAnalytics, permContent, permSubscribersRead, permSubscribersWrite} {
		if !can(roleAdmin, p) {
			t.Fatalf("an admin must have %s", p)
		}
	}
	if can("", permAnalytics) || can("reader", permAnalytics) || can(roleAdmin, "nothing:else") {
		t.Fatal("an unknown role or permission must be refused")
	}
}

func TestDashboardContentAndSubscribers(t *testing.T) {
	st := store.NewMemory()
	ctx := t.Context()
	_ = st.Save(ctx, store.Page{URLHash: "p1", URL: "https://news.test/a", Title: "A", IsArticle: true, Category: "india", Published: time.Now().Add(-time.Minute)})
	_ = st.Save(ctx, store.Page{URLHash: "p2", URL: "https://news.test/b", Title: "B", IsArticle: true, Category: "tech", Published: time.Now().Add(-time.Minute)})
	srv, a := adminFixture(t, st)
	session := a.signIn(t, srv, "boss")

	code, body := getJSONBody(t, authed("GET", srv.URL+"/admin/content?days=3", session, ""))
	days, _ := body["daily"].([]any)
	waiting, _ := body["waiting"].(map[string]any)
	cats, _ := body["categories"].([]any)
	if code != 200 || len(days) != 3 || days[2].(map[string]any)["articles"] != float64(2) || waiting["summary"] != float64(2) || len(cats) != 2 || body["latest_article"] == "" {
		t.Fatalf("content = %d %v", code, body)
	}
	if code, _ := getJSONBody(t, authed("GET", srv.URL+"/admin/content?days=0", session, "")); code != http.StatusBadRequest {
		t.Fatalf("days=0 = %d, want 400", code)
	}

	for _, e := range []string{"reader@x.test", "other@x.test"} {
		if got := post(t, srv, "/subscribe", `{"email":"`+e+`"}`); got != http.StatusAccepted {
			t.Fatalf("subscribe = %d", got)
		}
	}
	code, body = getJSONBody(t, authed("GET", srv.URL+"/admin/subscribers?q=READER", session, ""))
	subs, _ := body["subscribers"].([]any)
	if code != 200 || body["total"] != float64(1) || len(subs) != 1 {
		t.Fatalf("subscribers = %d %v", code, body)
	}
	row := subs[0].(map[string]any)
	if row["email"] != "reader@x.test" || row["status"] != "pending" {
		t.Fatalf("row = %v", row)
	}
	if strings.Contains(toJSON(body), "token") {
		t.Fatal("the subscriber list must never carry a token")
	}

	id := fmt.Sprint(int64(row["id"].(float64)))
	if resp, err := http.DefaultClient.Do(authed("DELETE", srv.URL+"/admin/subscribers/"+id, session, "")); err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("remove = %v, %v", resp, err)
	}
	if code, _ := getJSONBody(t, authed("DELETE", srv.URL+"/admin/subscribers/"+id, session, "")); code != http.StatusNotFound {
		t.Fatalf("removing twice = %d, want 404", code)
	}
	if code, _ := getJSONBody(t, authed("DELETE", srv.URL+"/admin/subscribers/zero", session, "")); code != http.StatusBadRequest {
		t.Fatalf("a bad id = %d, want 400", code)
	}
	if code, _ := getJSONBody(t, mustReq("DELETE", srv.URL+"/admin/subscribers/"+id, "")); code != http.StatusUnauthorized {
		t.Fatalf("removing without a session = %d, want 401", code)
	}
}
