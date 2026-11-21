package api

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

func TestChangingThePasswordNeedsTheOldOneAndEndsOtherSessions(t *testing.T) {
	srv, _ := accountFixture(t)
	first, _ := signup(t, srv, "alice", "a long password here")
	_, body := acct(t, srv, "/account/login", `{"username":"alice","password":"a long password here"}`)
	second, _ := body["session"].(string)

	for name, c := range map[string]struct {
		body string
		want int
	}{
		"wrong current":  {`{"current_password":"not the password","new_password":"a brand new password"}`, http.StatusUnauthorized},
		"weak new":       {`{"current_password":"a long password here","new_password":"short"}`, http.StatusBadRequest},
		"same as a name": {`{"current_password":"a long password here","new_password":"alice"}`, http.StatusBadRequest},
	} {
		if code, _ := getJSONBody(t, authed("POST", srv.URL+"/account/password", first, c.body)); code != c.want {
			t.Errorf("%s = %d, want %d", name, code, c.want)
		}
	}
	if code, _ := getJSONBody(t, mustReq("POST", srv.URL+"/account/password", `{"current_password":"a long password here","new_password":"a brand new password"}`)); code != http.StatusUnauthorized {
		t.Fatalf("without a session = %d, want 401", code)
	}

	code, body := getJSONBody(t, authed("POST", srv.URL+"/account/password", first, `{"current_password":"a long password here","new_password":"a brand new password"}`))
	fresh, _ := body["session"].(string)
	if code != 200 || fresh == "" {
		t.Fatalf("change = %d %v; want a fresh session", code, body)
	}
	for name, old := range map[string]string{"the session it was changed from": first, "another device's session": second} {
		if code, _ := getJSONBody(t, authed("GET", srv.URL+"/account/me", old, "")); code != http.StatusUnauthorized {
			t.Errorf("%s after a change = %d, want 401", name, code)
		}
	}
	if code, _ := getJSONBody(t, authed("GET", srv.URL+"/account/me", fresh, "")); code != 200 {
		t.Fatalf("the fresh session = %d, want 200", code)
	}
	if code, _ := acct(t, srv, "/account/login", `{"username":"alice","password":"a brand new password"}`); code != 200 {
		t.Fatalf("the new password = %d, want 200", code)
	}
}

func TestNewRecoveryCodesReplaceTheOldOnes(t *testing.T) {
	srv, _ := accountFixture(t)
	session, old := signup(t, srv, "alice", "a long password here")

	if code, _ := getJSONBody(t, authed("POST", srv.URL+"/account/codes", session, `{"current_password":"wrong password"}`)); code != http.StatusUnauthorized {
		t.Fatalf("new codes with a wrong password = %d, want 401", code)
	}
	code, body := getJSONBody(t, authed("POST", srv.URL+"/account/codes", session, `{"current_password":"a long password here"}`))
	fresh, _ := body["recovery_codes"].([]any)
	if code != 200 || len(fresh) != recoveryCodeCount || body["codes_left"] != float64(recoveryCodeCount) {
		t.Fatalf("new codes = %d %v; want %d fresh codes", code, body, recoveryCodeCount)
	}
	if code, _ := acct(t, srv, "/account/recover", fmt.Sprintf(`{"username":"alice","recovery_code":%q,"new_password":"yet another long one"}`, old[0])); code != http.StatusUnauthorized {
		t.Fatalf("an old code after new codes = %d, want 401", code)
	}
	if code, _ := acct(t, srv, "/account/recover", fmt.Sprintf(`{"username":"alice","recovery_code":%q,"new_password":"yet another long one"}`, fresh[0])); code != 200 {
		t.Fatalf("a new code = %d, want 200", code)
	}
}

func TestLogOutEverywhereAndDeletingTheAccount(t *testing.T) {
	srv, st := accountFixture(t)
	a, _ := signup(t, srv, "alice", "a long password here")
	_, body := acct(t, srv, "/account/login", `{"username":"alice","password":"a long password here"}`)
	b, _ := body["session"].(string)
	other, _ := signup(t, srv, "bobby", "bobs long password")

	if resp, err := http.DefaultClient.Do(authed("POST", srv.URL+"/account/logout-all", a, "")); err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("logout-all = %v, %v", resp, err)
	}
	for _, s := range []string{a, b} {
		if code, _ := getJSONBody(t, authed("GET", srv.URL+"/account/me", s, "")); code != http.StatusUnauthorized {
			t.Fatalf("a session after logging out everywhere = %d, want 401", code)
		}
	}
	if code, _ := getJSONBody(t, authed("GET", srv.URL+"/account/me", other, "")); code != 200 {
		t.Fatalf("another account's session = %d, want 200", code)
	}

	_, body = acct(t, srv, "/account/login", `{"username":"alice","password":"a long password here"}`)
	s, _ := body["session"].(string)
	if code, _ := getJSONBody(t, authed("POST", srv.URL+"/account/delete", s, `{"current_password":"wrong password"}`)); code != http.StatusUnauthorized {
		t.Fatalf("delete with a wrong password = %d, want 401", code)
	}
	if resp, err := http.DefaultClient.Do(authed("POST", srv.URL+"/account/delete", s, `{"current_password":"a long password here"}`)); err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete = %v, %v", resp, err)
	}
	if _, found, _ := st.AccountByUsername(context.Background(), "alice"); found {
		t.Fatal("the account must be gone")
	}
	if code, _ := acct(t, srv, "/account/login", `{"username":"alice","password":"a long password here"}`); code != http.StatusUnauthorized {
		t.Fatalf("logging in to a deleted account = %d, want 401", code)
	}
}

func TestAnAccountWithoutAPasswordCannotLogInWithOne(t *testing.T) {
	srv, st := accountFixture(t)
	if _, ok, err := st.CreateAccount(context.Background(), "googler", "", nil); err != nil || !ok {
		t.Fatalf("create = %v, %v", ok, err)
	}
	if code, _ := acct(t, srv, "/account/login", `{"username":"googler","password":""}`); code != http.StatusUnauthorized {
		t.Fatalf("an empty password on a password-less account = %d, want 401", code)
	}
	if code, _ := acct(t, srv, "/account/login", `{"username":"googler","password":"a long password here"}`); code != http.StatusUnauthorized {
		t.Fatalf("a password on a password-less account = %d, want 401", code)
	}
}

func TestProIsLinkedByPaymentIdAsWellAsByLink(t *testing.T) {
	srv, st := accountFixture(t)
	ctx := context.Background()
	_, _, _ = st.ProExtend(ctx, "pay_ABCDEF123456", "member@x.test", 30, "protoken-active-123456")
	alice, _ := signup(t, srv, "alice", "a long password here")
	bob, _ := signup(t, srv, "bobby", "bobs long password")

	for name, body := range map[string]string{
		"neither":         `{}`,
		"both":            `{"token":"protoken-active-123456","payment_id":"pay_ABCDEF123456"}`,
		"unknown payment": `{"payment_id":"pay_nope"}`,
	} {
		want := http.StatusBadRequest
		if name == "unknown payment" {
			want = http.StatusNotFound
		}
		if code, _ := getJSONBody(t, authed("POST", srv.URL+"/account/pro", alice, body)); code != want {
			t.Errorf("%s = %d, want %d", name, code, want)
		}
	}

	code, body := getJSONBody(t, authed("POST", srv.URL+"/account/pro", alice, `{"payment_id":" pay_ABCDEF123456 "}`))
	pro, _ := body["pro"].(map[string]any)
	roles, _ := body["roles"].([]any)
	if code != 200 || pro["linked"] != true || pro["active"] != true || len(roles) != 2 || roles[1] != "pro" {
		t.Fatalf("link by payment id = %d %v; want a linked, active membership and the pro role", code, body)
	}
	if code, _ := getJSONBody(t, authed("POST", srv.URL+"/account/pro", bob, `{"payment_id":"pay_ABCDEF123456"}`)); code != http.StatusConflict {
		t.Fatalf("a payment id whose membership another account holds = %d, want 409", code)
	}
}

func TestGuessingPaymentIdsIsLimited(t *testing.T) {
	srv, _ := accountFixture(t)
	s, _ := signup(t, srv, "alice", "a long password here")
	for i := 0; i < maxAccountFailures; i++ {
		getJSONBody(t, authed("POST", srv.URL+"/account/pro", s, fmt.Sprintf(`{"payment_id":"pay_guess%d"}`, i)))
	}
	if code, _ := getJSONBody(t, authed("POST", srv.URL+"/account/pro", s, `{"payment_id":"pay_guess_again"}`)); code != http.StatusTooManyRequests {
		t.Fatalf("after %d wrong ids = %d, want 429", maxAccountFailures, code)
	}
}
