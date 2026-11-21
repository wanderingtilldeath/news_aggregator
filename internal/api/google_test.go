package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/frontier"
	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

// fakeGoogle stands in for Google's token endpoint: it hands back an ID token with whatever claims the test set, and
// remembers the form the API sent.
type fakeGoogle struct {
	claims map[string]any
	status int
	form   url.Values
}

func (f *fakeGoogle) idToken() string {
	enc := base64.RawURLEncoding
	header, _ := json.Marshal(map[string]string{"alg": "RS256"})
	payload, _ := json.Marshal(f.claims)

	return enc.EncodeToString(header) + "." + enc.EncodeToString(payload) + ".signature"
}

func (f *fakeGoogle) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	f.form = r.PostForm
	if f.status != 0 && f.status != http.StatusOK {
		w.WriteHeader(f.status)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"id_token": f.idToken()})
}

func (f *fakeGoogle) as(sub string) {
	f.claims = map[string]any{"iss": "https://accounts.google.com", "aud": "client-id", "sub": sub, "exp": time.Now().Add(time.Hour).Unix()}
}

func googleFixture(t *testing.T) (*httptest.Server, *fakeGoogle, *store.Memory) {
	t.Helper()

	old := pbkdf2Iterations
	pbkdf2Iterations = 1000
	t.Cleanup(func() { pbkdf2Iterations = old })

	g := &fakeGoogle{}
	gs := httptest.NewServer(g)
	t.Cleanup(gs.Close)

	st := store.NewMemory()
	srv := httptest.NewServer(NewHandler(Deps{
		Frontier: frontier.NewMemory(nil), Store: st,
		GoogleClientID: "client-id", GoogleClientSecret: "client-secret", GoogleTokenURL: gs.URL,
	}))
	t.Cleanup(srv.Close)

	return srv, g, st
}

const googleBody = `{"code":"auth-code","redirect_uri":"https://site.test/auth/google/callback","code_verifier":"verifier-123"}`

func TestGoogleMakesAnAccountThenLogsBackIn(t *testing.T) {
	srv, g, st := googleFixture(t)
	g.as("sub-100")

	code, body := acct(t, srv, "/account/google", googleBody)
	name, _ := body["username"].(string)
	codes, _ := body["recovery_codes"].([]any)
	if code != 200 || !strings.HasPrefix(name, "reader_") || body["session"] == "" || len(codes) != recoveryCodeCount {
		t.Fatalf("first sign-in = %d %v; want a new reader_ account, a session and %d recovery codes", code, body, recoveryCodeCount)
	}
	for k, want := range map[string]string{"code": "auth-code", "client_id": "client-id", "client_secret": "client-secret", "redirect_uri": "https://site.test/auth/google/callback", "grant_type": "authorization_code", "code_verifier": "verifier-123"} {
		if g.form.Get(k) != want {
			t.Errorf("the token request had %s = %q, want %q", k, g.form.Get(k), want)
		}
	}
	if a, ok, _ := st.AccountByUsername(t.Context(), name); !ok || a.PasswordHash != "" {
		t.Fatalf("the account = %+v, %v; want one with no password", a, ok)
	}
	if code, _ := acct(t, srv, "/account/login", fmt.Sprintf(`{"username":%q,"password":"a long password here"}`, name)); code != http.StatusUnauthorized {
		t.Fatalf("a password login to a Google-only account = %d, want 401", code)
	}

	// The same Google identity again is the same account, with no new codes and no second account.
	code, again := acct(t, srv, "/account/google", googleBody)
	if code != 200 || again["username"] != name || again["recovery_codes"] != nil || again["codes_left"] != float64(recoveryCodeCount) {
		t.Fatalf("second sign-in = %d %v; want the same account, no new codes", code, again)
	}
}

func TestGoogleLinksToTheSignedInAccountAndIsARecoveryPath(t *testing.T) {
	srv, g, _ := googleFixture(t)
	alice, _ := signup(t, srv, "alice", "a long password here")
	bob, _ := signup(t, srv, "bobby", "bobs long password")
	g.as("sub-200")

	req := authed("POST", srv.URL+"/account/google", alice, googleBody)
	code, body := getJSONBody(t, req)
	ids, _ := body["identities"].([]any)
	if code != 200 || body["username"] != "alice" || len(ids) != 1 || ids[0] != "google" {
		t.Fatalf("link = %d %v; want alice with the google identity", code, body)
	}
	// Another account cannot take an identity that is linked.
	if code, _ := getJSONBody(t, authed("POST", srv.URL+"/account/google", bob, googleBody)); code != http.StatusConflict {
		t.Fatalf("linking an identity another account holds = %d, want 409", code)
	}
	// Signing in with that Google identity now opens alice's account, so Google is a way back in.
	code, in := acct(t, srv, "/account/google", googleBody)
	if code != 200 || in["username"] != "alice" {
		t.Fatalf("sign-in with the linked identity = %d %v; want alice", code, in)
	}
	// A made-up session cannot link anything.
	if code, _ := getJSONBody(t, authed("POST", srv.URL+"/account/google", "made-up", googleBody)); code != http.StatusUnauthorized {
		t.Fatalf("linking with a made-up session = %d, want 401", code)
	}
}

func TestGoogleRefusesATokenThatIsNotForUs(t *testing.T) {
	srv, g, _ := googleFixture(t)
	now := time.Now()
	for name, claims := range map[string]map[string]any{
		"wrong audience": {"iss": "https://accounts.google.com", "aud": "someone-else", "sub": "s", "exp": now.Add(time.Hour).Unix()},
		"wrong issuer":   {"iss": "https://evil.test", "aud": "client-id", "sub": "s", "exp": now.Add(time.Hour).Unix()},
		"expired":        {"iss": "https://accounts.google.com", "aud": "client-id", "sub": "s", "exp": now.Add(-time.Minute).Unix()},
		"no subject":     {"iss": "https://accounts.google.com", "aud": "client-id", "sub": "", "exp": now.Add(time.Hour).Unix()},
	} {
		g.claims = claims
		if code, body := acct(t, srv, "/account/google", googleBody); code != http.StatusUnauthorized || body["session"] != nil {
			t.Errorf("%s = %d %v, want 401 and no session", name, code, body)
		}
	}

	g.as("sub-1")
	g.status = http.StatusBadRequest
	if code, _ := acct(t, srv, "/account/google", googleBody); code != http.StatusUnauthorized {
		t.Fatalf("Google refusing the code = %d, want 401", code)
	}
	if code, _ := acct(t, srv, "/account/google", `{"code":""}`); code != http.StatusBadRequest {
		t.Fatalf("an empty code = %d, want 400", code)
	}
}

func TestGoogleIsOffWithoutAClientIdAndSecret(t *testing.T) {
	srv, _ := accountFixture(t)
	if code, _ := acct(t, srv, "/account/google", googleBody); code != http.StatusNotFound && code != http.StatusMethodNotAllowed {
		t.Fatalf("/account/google with no Google settings = %d, want 404 or 405 (the route must not exist)", code)
	}
}
