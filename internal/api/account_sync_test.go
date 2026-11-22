package api

import (
	"net/http"
	"testing"
)

func TestMergeStreak(t *testing.T) {
	for _, c := range []struct{ a, b, want string }{
		{"", "", ""},
		{"2026-11-20|4", "", "2026-11-20|4"},
		{"", "2026-11-20|4", "2026-11-20|4"},
		{"2026-11-21|5", "2026-11-21|2", "2026-11-21|5"}, // same day: the longer
		{"2026-11-20|5", "2026-11-22|1", "2026-11-22|1"}, // a gap: the later one stands alone
		{"2026-11-21|5", "2026-11-22|1", "2026-11-22|6"}, // starts the day after: it continues
		{"2026-11-22|1", "2026-11-21|5", "2026-11-22|6"}, // same, other order
		{"garbage", "2026-11-22|2", "2026-11-22|2"},      // a bad value never wins
		{"2026-11-22|2", "2026-13-99|2", "2026-11-22|2"},
	} {
		if got := mergeStreak(c.a, c.b); got != c.want {
			t.Errorf("mergeStreak(%q, %q) = %q, want %q", c.a, c.b, got, c.want)
		}
	}
}

func TestMergeSyncUnionAndReplace(t *testing.T) {
	stored := decodeSync(syncState{
		Bookmarks: []syncBookmark{{ID: "a", Title: "A", At: 10}, {ID: "b", Title: "B", At: 20}},
		Cards:     map[string]syncCard{"c1": {Box: 3, Due: 5}, "c2": {Box: 1, Due: 9}},
	}.encode())

	in := syncRequest{
		Mode:      "merge",
		Bookmarks: &[]syncBookmark{{ID: "c", Title: "C", At: 30}, {ID: "a", Title: "A", At: 10}},
		Cards:     &map[string]syncCard{"c1": {Box: 1, Due: 1}, "c3": {Box: 2, Due: 2}},
	}
	got := mergeSync(stored, in)
	if len(got.Bookmarks) != 3 || got.Bookmarks[0].ID != "c" || got.Bookmarks[2].ID != "a" {
		t.Fatalf("merged bookmarks = %+v, want c, b, a newest first", got.Bookmarks)
	}
	if got.Cards["c1"].Box != 3 || got.Cards["c2"].Box != 1 || got.Cards["c3"].Box != 2 {
		t.Fatalf("merged cards = %+v, want the higher box of each", got.Cards)
	}

	in.Mode = "replace"
	got = mergeSync(stored, in)
	if len(got.Bookmarks) != 2 || got.Cards["c1"].Box != 1 || len(got.Cards) != 2 {
		t.Fatalf("replaced = %+v %+v, want the browser's copy", got.Bookmarks, got.Cards)
	}

	// A kind left out stays as stored.
	got = mergeSync(stored, syncRequest{Mode: "replace"})
	if len(got.Bookmarks) != 2 || len(got.Cards) != 2 {
		t.Fatalf("an empty request changed the state: %+v", got)
	}
}

func TestMergeSyncCleansWhatItStores(t *testing.T) {
	long := make([]byte, 600)
	for i := range long {
		long[i] = 'x'
	}
	got := mergeSync(syncState{Cards: map[string]syncCard{}}, syncRequest{
		Mode:      "replace",
		Bookmarks: &[]syncBookmark{{ID: "", Title: "no id"}, {ID: "ok", Title: string(long)}, {ID: "ok", Title: "repeat"}},
		Missed:    &[]syncQuestion{{Question: "bad answer index", Options: []string{"a", "b"}, Answer: 5}, {Question: "fine", Options: []string{"a", "b"}, Answer: 1}},
		Cards:     &map[string]syncCard{"x": {Box: 99, Due: 1}},
	})
	if len(got.Bookmarks) != 1 || len([]rune(got.Bookmarks[0].Title)) != 300 {
		t.Fatalf("bookmarks = %+v, want one with a 300-character title", got.Bookmarks)
	}
	if len(got.Missed) != 1 || got.Missed[0].Question != "fine" || got.Cards["x"].Box != maxSyncBox {
		t.Fatalf("missed %+v cards %+v", got.Missed, got.Cards)
	}
}

func TestAccountSyncEndpoints(t *testing.T) {
	srv, _ := accountFixture(t)
	alice, _ := signup(t, srv, "alice", "a long password here")
	bob, _ := signup(t, srv, "bobby", "bobs long password")

	if code, _ := getJSONBody(t, mustReq("GET", srv.URL+"/account/sync", "")); code != http.StatusUnauthorized {
		t.Fatalf("anonymous get = %d, want 401", code)
	}
	if code, _ := getJSONBody(t, authed("PUT", srv.URL+"/account/sync", alice, `{"bookmarks":[]}`)); code != http.StatusBadRequest {
		t.Fatalf("put without a mode = %d, want 400", code)
	}

	body := `{"mode":"merge","bookmarks":[{"id":"s1","title":"One","source":"x.test","time":"","at":5}],"streak":"2026-11-22|3","cards":{"s1:0":{"box":2,"due":7}}}`
	code, got := getJSONBody(t, authed("PUT", srv.URL+"/account/sync", alice, body))
	if code != 200 || got["streak"] != "2026-11-22|3" || len(got["bookmarks"].([]any)) != 1 {
		t.Fatalf("put = %d %v", code, got)
	}

	// A second device reads the same items; another account sees none of them.
	_, read := getJSONBody(t, authed("GET", srv.URL+"/account/sync", alice, ""))
	if len(read["bookmarks"].([]any)) != 1 || read["streak"] != "2026-11-22|3" {
		t.Fatalf("get = %v", read)
	}
	_, other := getJSONBody(t, authed("GET", srv.URL+"/account/sync", bob, ""))
	if len(other["bookmarks"].([]any)) != 0 || other["streak"] != "" {
		t.Fatalf("bob read alice's items: %v", other)
	}

	// Removing a bookmark on one device (replace) sticks.
	_, after := getJSONBody(t, authed("PUT", srv.URL+"/account/sync", alice, `{"mode":"replace","bookmarks":[]}`))
	if len(after["bookmarks"].([]any)) != 0 {
		t.Fatalf("replace kept the removed bookmark: %v", after)
	}
}
