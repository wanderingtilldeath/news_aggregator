package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/wanderingtilldeath/news-aggregator/internal/frontier"
	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

func accountFixture(t *testing.T) (*httptest.Server, *store.Memory) {
	t.Helper()

	old := pbkdf2Iterations
	pbkdf2Iterations = 1000 // the real count is for production; the tests only need the same code path
	t.Cleanup(func() { pbkdf2Iterations = old })

	st := store.NewMemory()
	srv := httptest.NewServer(NewHandler(Deps{Frontier: frontier.NewMemory(nil), Store: st}))
	t.Cleanup(srv.Close)

	return srv, st
}

func acct(t *testing.T, srv *httptest.Server, path, body string) (int, map[string]any) {
	t.Helper()

	return getJSONBody(t, mustReq("POST", srv.URL+path, body))
}

func signup(t *testing.T, srv *httptest.Server, user, pass string) (session string, codes []string) {
	t.Helper()

	code, body := acct(t, srv, "/account/signup", fmt.Sprintf(`{"username":%q,"password":%q}`, user, pass))
	session, _ = body["session"].(string)
	raw, _ := body["recovery_codes"].([]any)
	for _, c := range raw {
		codes = append(codes, fmt.Sprint(c))
	}
	if code != 200 || session == "" || len(codes) != recoveryCodeCount || body["codes_left"] != float64(recoveryCodeCount) {
		t.Fatalf("signup %s = %d %v; want a session and %d recovery codes", user, code, body, recoveryCodeCount)
	}

	return session, codes
}

func TestPasswordHashingAndRecoveryCodes(t *testing.T) {
	if pbkdf2Iterations < 600_000 {
		t.Fatalf("the production iteration count is %d, want at least 600000", pbkdf2Iterations)
	}
	old := pbkdf2Iterations
	pbkdf2Iterations = 1000
	defer func() { pbkdf2Iterations = old }()

	h, err := hashPassword("correct horse battery")
	if err != nil || !strings.HasPrefix(h, "pbkdf2-sha256$1000$") {
		t.Fatalf("hash = %q, %v", h, err)
	}
	if !checkPassword("correct horse battery", h) || checkPassword("wrong horse battery", h) {
		t.Fatal("a password must match its own hash and nothing else")
	}
	h2, _ := hashPassword("correct horse battery")
	if h == h2 {
		t.Fatal("two hashes of one password must differ (a random salt each)")
	}
	for _, bad := range []string{"", "plain", "pbkdf2-sha256$x$a$b", "pbkdf2-sha256$1000$!!$!!", "other$1000$YQ$YQ"} {
		if checkPassword("anything", bad) {
			t.Fatalf("a malformed hash %q must never match", bad)
		}
	}

	code, err := newRecoveryCode()
	if err != nil || !regexp.MustCompile(`^[A-Z2-9]{5}(-[A-Z2-9]{5}){3}$`).MatchString(code) {
		t.Fatalf("recovery code = %q, %v", code, err)
	}
	if hashRecoveryCode(code) != hashRecoveryCode(strings.ToLower(strings.ReplaceAll(code, "-", " "))) {
		t.Fatal("a recovery code typed in lower case, with spaces, must match")
	}
	if other, _ := newRecoveryCode(); other == code {
		t.Fatal("two recovery codes must differ")
	}
}

func TestSignupRules(t *testing.T) {
	srv, _ := accountFixture(t)
	for name, c := range map[string]struct {
		body string
		want int
	}{
		"too short":      {`{"username":"ab","password":"long enough pw"}`, http.StatusBadRequest},
		"bad characters": {`{"username":"a b!","password":"long enough pw"}`, http.StatusBadRequest},
		"too long":       {`{"username":"` + strings.Repeat("a", 25) + `","password":"long enough pw"}`, http.StatusBadRequest},
		"reserved":       {`{"username":"Admin","password":"long enough pw"}`, http.StatusConflict},
		"short password": {`{"username":"carol","password":"short"}`, http.StatusBadRequest},
		"same as name":   {`{"username":"carol12345","password":"carol12345"}`, http.StatusBadRequest},
		"not json":       {`{`, http.StatusBadRequest},
	} {
		if code, _ := acct(t, srv, "/account/signup", c.body); code != c.want {
			t.Errorf("%s = %d, want %d", name, code, c.want)
		}
	}

	session, _ := signup(t, srv, "Alice", "a long password here")
	if code, _ := acct(t, srv, "/account/signup", `{"username":"alice","password":"another long one"}`); code != http.StatusConflict {
		t.Fatalf("a taken username (any case) = %d, want 409", code)
	}
	req := authed("GET", srv.URL+"/account/me", session, "")
	if code, body := getJSONBody(t, req); code != 200 || body["username"] != "alice" {
		t.Fatalf("me = %d %v; want the lower-cased username", code, body)
	}
}

func TestLoginLogoutAndLockout(t *testing.T) {
	srv, _ := accountFixture(t)
	signup(t, srv, "alice", "a long password here")
	signup(t, srv, "bob", "bobs long password")

	code, body := acct(t, srv, "/account/login", `{"username":"ALICE","password":"a long password here"}`)
	session, _ := body["session"].(string)
	if code != 200 || session == "" || body["recovery_codes"] != nil || body["codes_left"] != float64(recoveryCodeCount) {
		t.Fatalf("login = %d %v; want a session, no new recovery codes and the count left", code, body)
	}

	// A wrong password and an unknown username get one answer.
	var msgs []string
	for _, b := range []string{`{"username":"alice","password":"wrong password"}`, `{"username":"nobody","password":"a long password here"}`} {
		code, body := acct(t, srv, "/account/login", b)
		if code != http.StatusUnauthorized {
			t.Fatalf("login %s = %d, want 401", b, code)
		}
		msgs = append(msgs, fmt.Sprint(body["error"]))
	}
	if msgs[0] != msgs[1] {
		t.Fatalf("the answers differ and would say which part was wrong: %q", msgs)
	}

	// Logging out ends that session only.
	if resp, err := http.DefaultClient.Do(authed("POST", srv.URL+"/account/logout", session, "")); err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("logout = %v, %v", resp, err)
	}
	if code, _ := getJSONBody(t, authed("GET", srv.URL+"/account/me", session, "")); code != http.StatusUnauthorized {
		t.Fatalf("me after logout = %d, want 401", code)
	}

	// Repeated wrong passwords lock the username out; another username is not affected.
	for i := 0; i < maxAccountFailures; i++ {
		acct(t, srv, "/account/login", `{"username":"alice","password":"wrong password"}`)
	}
	if code, _ := acct(t, srv, "/account/login", `{"username":"alice","password":"a long password here"}`); code != http.StatusTooManyRequests {
		t.Fatalf("after %d wrong tries, the right password = %d, want 429", maxAccountFailures, code)
	}
	if code, _ := acct(t, srv, "/account/login", `{"username":"bob","password":"bobs long password"}`); code != 200 {
		t.Fatalf("another account = %d, want 200", code)
	}
}

func TestRecoveryCodesResetThePasswordOneAtATime(t *testing.T) {
	srv, _ := accountFixture(t)
	session, codes := signup(t, srv, "alice", "a long password here")

	if code, _ := acct(t, srv, "/account/recover", `{"username":"alice","recovery_code":"AAAAA-BBBBB-CCCCC-DDDDD","new_password":"a new long password"}`); code != http.StatusUnauthorized {
		t.Fatalf("a wrong code = %d, want 401", code)
	}
	if code, _ := acct(t, srv, "/account/recover", fmt.Sprintf(`{"username":"alice","recovery_code":%q,"new_password":"short"}`, codes[0])); code != http.StatusBadRequest {
		t.Fatalf("a weak new password = %d, want 400", code)
	}

	// A code typed in lower case works, resets the password, ends every old session and leaves nine codes.
	code, body := acct(t, srv, "/account/recover", fmt.Sprintf(`{"username":"Alice","recovery_code":%q,"new_password":"a new long password"}`, strings.ToLower(codes[0])))
	if code != 200 || body["session"] == "" || body["codes_left"] != float64(recoveryCodeCount-1) || body["recovery_codes"] != nil {
		t.Fatalf("recover = %d %v; want a session, nine codes left and no new codes shown", code, body)
	}
	if code, _ := getJSONBody(t, authed("GET", srv.URL+"/account/me", session, "")); code != http.StatusUnauthorized {
		t.Fatalf("an old session after a reset = %d, want 401", code)
	}
	if code, _ := acct(t, srv, "/account/login", `{"username":"alice","password":"a long password here"}`); code != http.StatusUnauthorized {
		t.Fatalf("the old password after a reset = %d, want 401", code)
	}
	if code, _ := acct(t, srv, "/account/login", `{"username":"alice","password":"a new long password"}`); code != 200 {
		t.Fatalf("the new password = %d, want 200", code)
	}

	// The spent code is spent; the others on the printed sheet still work.
	if code, _ := acct(t, srv, "/account/recover", fmt.Sprintf(`{"username":"alice","recovery_code":%q,"new_password":"yet another long one"}`, codes[0])); code != http.StatusUnauthorized {
		t.Fatalf("a spent recovery code = %d, want 401", code)
	}
	if code, body := acct(t, srv, "/account/recover", fmt.Sprintf(`{"username":"alice","recovery_code":%q,"new_password":"yet another long one"}`, codes[1])); code != 200 || body["codes_left"] != float64(recoveryCodeCount-2) {
		t.Fatalf("a second code = %d %v, want 200 with eight left", code, body)
	}
}

func TestAnAccountHoldsAProMembershipAndItsNotes(t *testing.T) {
	srv, st := accountFixture(t)
	ctx := context.Background()
	_, _, _ = st.ProExtend(ctx, "pay-1", "member@x.test", 30, "protoken-active-123456")
	_, _, _ = st.ProExtend(ctx, "pay-2", "lapsed@x.test", -3, "protoken-lapsed-1234567")
	alice, _ := signup(t, srv, "alice", "a long password here")
	bob, _ := signup(t, srv, "bobby", "bobs long password")

	// Without a membership the notes are closed to the account.
	if code, _ := getJSONBody(t, authed("GET", srv.URL+"/pro/notes", alice, "")); code != http.StatusForbidden {
		t.Fatalf("notes before linking = %d, want 403", code)
	}
	if code, _ := getJSONBody(t, authed("GET", srv.URL+"/pro/notes", "made-up", "")); code != http.StatusUnauthorized {
		t.Fatalf("notes with a made-up session = %d, want 401", code)
	}

	if code, _ := getJSONBody(t, authed("POST", srv.URL+"/account/pro", alice, `{"token":"not-a-real-token-0000"}`)); code != http.StatusNotFound {
		t.Fatalf("an unknown Pro link = %d, want 404", code)
	}
	if code, _ := getJSONBody(t, mustReq("POST", srv.URL+"/account/pro", `{"token":"protoken-active-123456"}`)); code != http.StatusUnauthorized {
		t.Fatalf("linking without a session = %d, want 401", code)
	}
	code, body := getJSONBody(t, authed("POST", srv.URL+"/account/pro", alice, `{"token":"protoken-active-123456"}`))
	pro, _ := body["pro"].(map[string]any)
	if code != 200 || pro["linked"] != true || pro["active"] != true {
		t.Fatalf("link = %d %v", code, body)
	}
	if code, _ := getJSONBody(t, authed("POST", srv.URL+"/account/pro", bob, `{"token":"protoken-active-123456"}`)); code != http.StatusConflict {
		t.Fatalf("a membership held by another account = %d, want 409", code)
	}

	// With it, the same notes endpoints work on the session.
	if code, _ := getJSONBody(t, authed("PUT", srv.URL+"/pro/notes/n1", alice, `{"story":"s","title":"T","text":"Saved by account","tags":["a"]}`)); code != 200 {
		t.Fatalf("save by session = %d, want 200", code)
	}
	_, list := getJSONBody(t, authed("GET", srv.URL+"/pro/notes", alice, ""))
	if notes, _ := list["notes"].([]any); len(notes) != 1 {
		t.Fatalf("notes = %v, want the one saved", list)
	}
	if code, body := getJSONBody(t, authed("GET", srv.URL+"/account/me", alice, "")); code != 200 || body["pro"].(map[string]any)["until"] == "" {
		t.Fatalf("me = %d %v", code, body)
	}

	// The account page gets the days left and the member's own printable-edition path; a lapsed one gets neither.
	_, me := getJSONBody(t, authed("GET", srv.URL+"/account/me", alice, ""))
	mp, _ := me["pro"].(map[string]any)
	if days, _ := mp["days_left"].(float64); days < 29 || days > 31 || mp["edition"] != "/pro/edition?token=protoken-active-123456" || mp["email"] != "member@x.test" {
		t.Fatalf("pro view = %v, want about 30 days left, the edition path and the paid address", mp)
	}

	// A lapsed membership can read but not add.
	carol, _ := signup(t, srv, "carol", "carols long password")
	getJSONBody(t, authed("POST", srv.URL+"/account/pro", carol, `{"token":"protoken-lapsed-1234567"}`))
	_, lapsed := getJSONBody(t, authed("GET", srv.URL+"/account/me", carol, ""))
	if lp, _ := lapsed["pro"].(map[string]any); lp["days_left"] != float64(0) || lp["edition"] != nil {
		t.Fatalf("lapsed pro view = %v, want no days left and no edition link", lp)
	}
	if code, _ := getJSONBody(t, authed("GET", srv.URL+"/pro/notes", carol, "")); code != 200 {
		t.Fatalf("a lapsed account's list = %d, want 200", code)
	}
	if code, _ := getJSONBody(t, authed("PUT", srv.URL+"/pro/notes/n1", carol, `{"story":"s","title":"T","text":"x"}`)); code != http.StatusForbidden {
		t.Fatalf("a lapsed account's save = %d, want 403", code)
	}
}

func TestLoginWithEmail(t *testing.T) {
	srv, sent := emailFixture(t)
	code, body := acct(t, srv, "/account/signup", `{"username":"alice","password":"a long password here","email":"Alice@Example.com"}`)
	if code != 200 {
		t.Fatalf("signup with email = %d %v", code, body)
	}
	if code, _ := acct(t, srv, "/account/signup", `{"username":"bob","password":"bobs long password","email":"not an email"}`); code != http.StatusBadRequest {
		t.Fatalf("signup with a bad email = %d, want 400", code)
	}

	// The username always works; the email works once the link mailed at signup has been opened.
	if code, _ := acct(t, srv, "/account/login", `{"username":"alice@example.com","password":"a long password here"}`); code != http.StatusUnauthorized {
		t.Fatalf("login with an unconfirmed email = %d, want 401", code)
	}
	links := sent()
	if len(links) != 1 || links[0].kind != "verify" {
		t.Fatalf("signup mails = %+v, want one verify link", links)
	}
	if code, _ := acct(t, srv, "/account/email/verify", `{"token":"`+links[0].token+`"}`); code != http.StatusNoContent {
		t.Fatalf("verify = %d, want 204", code)
	}
	if code, _ := acct(t, srv, "/account/signup", `{"username":"bob","password":"bobs long password","email":"alice@example.com"}`); code != http.StatusConflict {
		t.Fatalf("signup with a confirmed email = %d, want 409", code)
	}
	for _, who := range []string{"alice@example.com", "ALICE@example.com", "alice"} {
		if code, body := acct(t, srv, "/account/login", fmt.Sprintf(`{"username":%q,"password":"a long password here"}`, who)); code != 200 || body["session"] == "" {
			t.Fatalf("login as %s = %d %v, want a session", who, code, body)
		}
	}
	if code, _ := acct(t, srv, "/account/login", `{"username":"alice@example.com","password":"wrong password"}`); code != http.StatusUnauthorized {
		t.Fatalf("login with email and a wrong password = %d, want 401", code)
	}
}
