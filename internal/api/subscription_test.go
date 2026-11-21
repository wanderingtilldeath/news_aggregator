package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

func getJSONBody(t *testing.T, req *http.Request) (int, map[string]any) {
	t.Helper()

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)

	return resp.StatusCode, out
}

func TestManageSubscription(t *testing.T) {
	srv, n := subscribeFixture(t, true)
	if got := post(t, srv, "/subscribe", `{"email":"reader@example.com","topics":["rbi"],"source":" LinkedIn!<b> "}`); got != http.StatusAccepted {
		t.Fatalf("subscribe = %d", got)
	}
	token := n.tokens[0]

	req, _ := http.NewRequest("GET", srv.URL+"/subscription?token="+token, nil)
	code, body := getJSONBody(t, req)
	if code != 200 || body["email"] != "r***@example.com" || body["status"] != "pending" || body["mode"] != "1min" {
		t.Fatalf("GET /subscription = %d %v; want the masked address, pending, 1min", code, body)
	}
	if code, _ := getJSONBody(t, mustReq("GET", srv.URL+"/subscription?token=nope", "")); code != 404 {
		t.Fatalf("unknown token = %d, want 404", code)
	}
	if code, _ := getJSONBody(t, mustReq("GET", srv.URL+"/subscription", "")); code != 400 {
		t.Fatalf("no token = %d, want 400", code)
	}

	// New choices are saved and echoed; a bad length is refused.
	code, body = getJSONBody(t, mustReq("POST", srv.URL+"/subscription", `{"token":"`+token+`","mode":"7min","topics":["  Monsoon ","isro"]}`))
	topics, _ := body["topics"].([]any)
	if code != 200 || body["mode"] != "7min" || len(topics) != 2 || topics[0] != "monsoon" {
		t.Fatalf("POST /subscription = %d %v", code, body)
	}
	if code, _ := getJSONBody(t, mustReq("POST", srv.URL+"/subscription", `{"token":"`+token+`","mode":"forever"}`)); code != 400 {
		t.Fatalf("bad mode = %d, want 400", code)
	}
	if code, _ := getJSONBody(t, mustReq("POST", srv.URL+"/subscription", `{"token":"nope","mode":"1min"}`)); code != 404 {
		t.Fatalf("unknown token on save = %d, want 404", code)
	}

	// Once unsubscribed, a save is refused and the page says so.
	if got := post(t, srv, "/unsubscribe", `{"token":"`+token+`"}`); got != 200 {
		t.Fatalf("unsubscribe = %d", got)
	}
	if code, _ := getJSONBody(t, mustReq("POST", srv.URL+"/subscription", `{"token":"`+token+`","mode":"1min"}`)); code != http.StatusConflict {
		t.Fatalf("save after unsubscribe = %d, want 409", code)
	}
	if _, body := getJSONBody(t, mustReq("GET", srv.URL+"/subscription?token="+token, "")); body["status"] != "unsubscribed" {
		t.Fatalf("status = %v, want unsubscribed", body["status"])
	}
}

func mustReq(method, url, body string) *http.Request {
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, _ := http.NewRequest(method, url, r)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	return req
}

func TestAnalyticsCountsSourcesForASignedInAdmin(t *testing.T) {
	srv, a := adminFixture(t, store.NewMemory())
	session := a.signIn(t, srv, "admin@x.test")

	for _, email := range []string{"a@x.test", "b@x.test"} {
		if got := post(t, srv, "/subscribe", `{"email":"`+email+`","source":"LinkedIn"}`); got != http.StatusAccepted {
			t.Fatalf("subscribe = %d", got)
		}
	}
	if got := post(t, srv, "/subscribe", `{"email":"c@x.test"}`); got != http.StatusAccepted {
		t.Fatalf("subscribe = %d", got)
	}

	req := mustReq("GET", srv.URL+"/analytics?days=7", "")
	req.Header.Set("Authorization", "Bearer "+session)
	code, body := getJSONBody(t, req)
	if code != 200 {
		t.Fatalf("analytics = %d %v", code, body)
	}
	subs := body["subscribers"].(map[string]any)
	if subs["pending"] != float64(3) {
		t.Fatalf("subscribers = %v, want 3 pending", subs)
	}
	days := body["daily"].([]any)
	if len(days) != 7 || days[6].(map[string]any)["signups"] != float64(3) {
		t.Fatalf("daily = %v, want 7 days ending with 3 signups", days)
	}
	sources := body["sources"].([]any)
	if first := sources[0].(map[string]any); first["source"] != "linkedin" || first["signups"] != float64(2) {
		t.Fatalf("sources = %v, want linkedin first with 2, lower-cased", sources)
	}
	if sources[1].(map[string]any)["source"] != "direct" {
		t.Fatalf("sources = %v, want the untagged signup as direct", sources)
	}
	if strings.Contains(strings.ToLower(toJSON(body)), "@x.test") {
		t.Fatal("the analytics answer must never carry an address")
	}

	bad := mustReq("GET", srv.URL+"/analytics?days=0", "")
	bad.Header.Set("Authorization", "Bearer "+session)
	if code, _ := getJSONBody(t, bad); code != http.StatusBadRequest {
		t.Fatalf("days=0 = %d, want 400", code)
	}
}

func toJSON(v any) string {
	b, _ := json.Marshal(v)

	return string(b)
}

func TestCleanSourceKeepsAShortSafeLabel(t *testing.T) {
	for in, want := range map[string]string{
		"":                       "",
		" LinkedIn ":             "linkedin",
		"x.com_post-1":           "x.com_post-1",
		"<script>alert(1)":       "scriptalert1",
		strings.Repeat("a", 100): strings.Repeat("a", maxSourceLen),
	} {
		if got := cleanSource(in); got != want {
			t.Errorf("cleanSource(%q) = %q, want %q", in, got, want)
		}
	}
}
