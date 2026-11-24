package api

import (
	"net/http"
	"strconv"
	"strings"
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

func TestMergeSyncMarks(t *testing.T) {
	mark := func(id, name string, at int64) syncMark {
		return syncMark{ID: id, Map: "tiger", Base: "india", Lon: 80, Lat: 20, Name: name, Item: "kanha", From: "star", At: at}
	}
	stored := decodeSync(syncState{Marks: []syncMark{mark("tiger:a", "A", 10), mark("tiger:b", "B", 20)}}.encode())

	// Merging unions by id, newest first, and a repeat keeps the newer copy.
	got := mergeSync(stored, syncRequest{Mode: "merge", Marks: &[]syncMark{mark("tiger:c", "C", 30), mark("tiger:a", "A newer", 40)}})
	if len(got.Marks) != 3 || got.Marks[0].ID != "tiger:a" || got.Marks[0].Name != "A newer" || got.Marks[2].ID != "tiger:b" {
		t.Fatalf("merged marks = %+v, want a (newer), c, b", got.Marks)
	}
	// Replacing makes the browser's copy the account's: a mark removed on a device stays removed.
	got = mergeSync(stored, syncRequest{Mode: "replace", Marks: &[]syncMark{mark("tiger:b", "B", 20)}})
	if len(got.Marks) != 1 || got.Marks[0].ID != "tiger:b" {
		t.Fatalf("replaced marks = %+v", got.Marks)
	}
	// A request without marks leaves them.
	if got = mergeSync(stored, syncRequest{Mode: "replace"}); len(got.Marks) != 2 {
		t.Fatalf("an empty request changed the marks: %+v", got.Marks)
	}

	// Junk is dropped and lengths are capped: no id, a repeated id, a position off the globe, an odd base or source.
	long := strings.Repeat("x", 800)
	cleaned := mergeSync(syncState{}, syncRequest{Mode: "replace", Marks: &[]syncMark{
		{ID: "", Name: "no id"},
		{ID: "ok", Name: " ", Note: long, Base: "mars", From: "weird", Lon: 80, Lat: 20},
		{ID: "ok", Name: "repeat", Lon: 80, Lat: 20},
		{ID: "off", Name: "off the globe", Lon: 80, Lat: 95},
	}})
	if len(cleaned.Marks) != 1 {
		t.Fatalf("cleaned = %+v, want one", cleaned.Marks)
	}
	if m := cleaned.Marks[0]; m.Name != "My place" || len([]rune(m.Note)) != 500 || m.Base != "india" || m.From != "note" {
		t.Fatalf("cleaned mark = %+v, want a default name, a 500-character note, india and note", m)
	}
	over := make([]syncMark, 0, maxSyncMarks+5)
	for i := 0; i < maxSyncMarks+5; i++ {
		over = append(over, syncMark{ID: strconv.Itoa(i), Name: "x", Lon: 80, Lat: 20})
	}
	if n := len(mergeSync(syncState{}, syncRequest{Mode: "replace", Marks: &over}).Marks); n != maxSyncMarks {
		t.Fatalf("kept %d marks, want %d", n, maxSyncMarks)
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

	body := `{"mode":"merge","bookmarks":[{"id":"s1","title":"One","source":"x.test","time":"","at":5}],"streak":"2026-11-22|3","cards":{"s1:0":{"box":2,"due":7}},"marks":[{"id":"tiger:kanha","map":"tiger","base":"india","lon":80.6,"lat":22.3,"name":"Kanha","note":"hook","item":"kanha","from":"star","at":9}]}`
	code, got := getJSONBody(t, authed("PUT", srv.URL+"/account/sync", alice, body))
	if code != 200 || got["streak"] != "2026-11-22|3" || len(got["bookmarks"].([]any)) != 1 || len(got["marks"].([]any)) != 1 {
		t.Fatalf("put = %d %v", code, got)
	}

	// A second device reads the same items; another account sees none of them.
	_, read := getJSONBody(t, authed("GET", srv.URL+"/account/sync", alice, ""))
	if len(read["bookmarks"].([]any)) != 1 || read["streak"] != "2026-11-22|3" || len(read["marks"].([]any)) != 1 {
		t.Fatalf("get = %v", read)
	}
	_, other := getJSONBody(t, authed("GET", srv.URL+"/account/sync", bob, ""))
	if len(other["bookmarks"].([]any)) != 0 || other["streak"] != "" || len(other["marks"].([]any)) != 0 {
		t.Fatalf("bob read alice's items: %v", other)
	}

	// Removing a bookmark on one device (replace) sticks.
	_, after := getJSONBody(t, authed("PUT", srv.URL+"/account/sync", alice, `{"mode":"replace","bookmarks":[]}`))
	if len(after["bookmarks"].([]any)) != 0 {
		t.Fatalf("replace kept the removed bookmark: %v", after)
	}
}
