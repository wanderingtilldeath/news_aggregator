package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wanderingtilldeath/news-aggregator/internal/frontier"
	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

func notesFixture(t *testing.T) *httptest.Server {
	t.Helper()

	st := store.NewMemory()
	if _, _, err := st.ProExtend(t.Context(), "pay-1", "member@x.test", 30, "tok-active"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.ProExtend(t.Context(), "pay-2", "lapsed@x.test", -3, "tok-lapsed"); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(NewHandler(Deps{Frontier: frontier.NewMemory(nil), Store: st}))
	t.Cleanup(srv.Close)

	return srv
}

func notesReq(method, url, token, body string) *http.Request {
	req := mustReq(method, url, body)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	return req
}

const sampleNote = `{"story":"s1","title":"A title","text":"A saved sentence.","note":"  my thought ","tags":[" Economy ","UPSC prep","economy","","x!y"],"at":1700000000000}`

func TestProNotesNeedAValidProToken(t *testing.T) {
	srv := notesFixture(t)
	for name, token := range map[string]string{"none": "", "unknown": "tok-nope"} {
		for _, c := range []struct{ method, path string }{{"GET", "/pro/notes"}, {"PUT", "/pro/notes/n1"}, {"DELETE", "/pro/notes/n1"}, {"DELETE", "/pro/notes"}, {"POST", "/pro/notes/import"}} {
			if code, _ := getJSONBody(t, notesReq(c.method, srv.URL+c.path, token, sampleNote)); code != http.StatusUnauthorized {
				t.Errorf("%s token: %s %s = %d, want 401", name, c.method, c.path, code)
			}
		}
	}
}

func TestProNotesSaveEditListAndDelete(t *testing.T) {
	srv := notesFixture(t)

	code, body := getJSONBody(t, notesReq("PUT", srv.URL+"/pro/notes/s1:abc", "tok-active", sampleNote))
	tags, _ := body["tags"].([]any)
	if code != 200 || body["id"] != "s1:abc" || body["note"] != "my thought" || len(tags) != 3 || tags[0] != "economy" || tags[1] != "upsc-prep" || tags[2] != "xy" {
		t.Fatalf("save = %d %v; want the id from the path, a trimmed note and cleaned, de-duplicated tags", code, body)
	}

	// A second note, then the list is newest first and carries the limit.
	_, _ = getJSONBody(t, notesReq("PUT", srv.URL+"/pro/notes/s2:def", "tok-active", `{"story":"s2","title":"T","text":"Newer","at":1800000000000}`))
	code, body = getJSONBody(t, notesReq("GET", srv.URL+"/pro/notes", "tok-active", ""))
	list, _ := body["notes"].([]any)
	if code != 200 || len(list) != 2 || list[0].(map[string]any)["id"] != "s2:def" || body["limit"] != float64(maxProNotes) {
		t.Fatalf("list = %d %v", code, body)
	}

	// Saving again with the same id replaces it.
	_, _ = getJSONBody(t, notesReq("PUT", srv.URL+"/pro/notes/s2:def", "tok-active", `{"story":"s2","title":"T","text":"Newer","note":"edited","at":1800000000000}`))
	_, body = getJSONBody(t, notesReq("GET", srv.URL+"/pro/notes", "tok-active", ""))
	if list, _ = body["notes"].([]any); len(list) != 2 || list[0].(map[string]any)["note"] != "edited" {
		t.Fatalf("an edit must replace the note, not add one: %v", body)
	}

	if resp, err := http.DefaultClient.Do(notesReq("DELETE", srv.URL+"/pro/notes/s2:def", "tok-active", "")); err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete = %v, %v", resp, err)
	}
	if code, _ := getJSONBody(t, notesReq("DELETE", srv.URL+"/pro/notes/s2:def", "tok-active", "")); code != http.StatusNotFound {
		t.Fatalf("deleting twice = %d, want 404", code)
	}
	if resp, err := http.DefaultClient.Do(notesReq("DELETE", srv.URL+"/pro/notes", "tok-active", "")); err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete all = %v, %v", resp, err)
	}
	if _, body = getJSONBody(t, notesReq("GET", srv.URL+"/pro/notes", "tok-active", "")); len(body["notes"].([]any)) != 0 {
		t.Fatalf("notes left after delete all: %v", body)
	}
}

func TestProNotesRefuseBadInput(t *testing.T) {
	srv := notesFixture(t)
	for name, c := range map[string]struct{ path, body string }{
		"empty sentence":  {"/pro/notes/n1", `{"story":"s","title":"t","text":"  "}`},
		"no story":        {"/pro/notes/n1", `{"title":"t","text":"x"}`},
		"long sentence":   {"/pro/notes/n1", `{"story":"s","title":"t","text":"` + strings.Repeat("a", maxNoteText+1) + `"}`},
		"long own note":   {"/pro/notes/n1", `{"story":"s","title":"t","text":"x","note":"` + strings.Repeat("a", maxNoteOwn+1) + `"}`},
		"long id":         {"/pro/notes/" + strings.Repeat("i", maxNoteID+1), `{"story":"s","title":"t","text":"x"}`},
		"not json":        {"/pro/notes/n1", `{`},
		"unknown id char": {"/pro/notes/a%00b", `{"story":"s","title":"t","text":"x"}`},
	} {
		if code, _ := getJSONBody(t, notesReq("PUT", srv.URL+c.path, "tok-active", c.body)); code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", name, code)
		}
	}
	// A note with more than 10 tags is kept with the first 10.
	var many []string
	for i := 0; i < 15; i++ {
		many = append(many, fmt.Sprintf(`"tag%d"`, i))
	}
	_, body := getJSONBody(t, notesReq("PUT", srv.URL+"/pro/notes/n1", "tok-active", `{"story":"s","title":"t","text":"x","tags":[`+strings.Join(many, ",")+`]}`))
	if tags, _ := body["tags"].([]any); len(tags) != maxNoteTags {
		t.Fatalf("tags = %v, want the first %d", body["tags"], maxNoteTags)
	}
}

func TestProNotesImportOnlyAddsMissingNotes(t *testing.T) {
	srv := notesFixture(t)
	_, _ = getJSONBody(t, notesReq("PUT", srv.URL+"/pro/notes/s1:a", "tok-active", `{"story":"s1","title":"T","text":"Server copy","note":"edited on the server"}`))

	code, body := getJSONBody(t, notesReq("POST", srv.URL+"/pro/notes/import", "tok-active",
		`{"notes":[{"id":"s1:a","story":"s1","title":"T","text":"Browser copy"},{"id":"s1:b","story":"s1","title":"T","text":"Only in the browser"}]}`))
	if code != 200 || body["added"] != float64(1) {
		t.Fatalf("import = %d %v; want 1 added", code, body)
	}
	_, body = getJSONBody(t, notesReq("GET", srv.URL+"/pro/notes", "tok-active", ""))
	for _, n := range body["notes"].([]any) {
		if m := n.(map[string]any); m["id"] == "s1:a" && (m["text"] != "Server copy" || m["note"] != "edited on the server") {
			t.Fatalf("an import overwrote the server's copy: %v", m)
		}
	}
	for name, payload := range map[string]string{"empty": `{"notes":[]}`, "bad note": `{"notes":[{"id":"x","story":"s","title":"t","text":""}]}`} {
		if code, _ := getJSONBody(t, notesReq("POST", srv.URL+"/pro/notes/import", "tok-active", payload)); code != http.StatusBadRequest {
			t.Errorf("%s import = %d, want 400", name, code)
		}
	}
}

func TestALapsedMemberCanReadAndDeleteButNotAdd(t *testing.T) {
	srv := notesFixture(t)
	if code, _ := getJSONBody(t, notesReq("GET", srv.URL+"/pro/notes", "tok-lapsed", "")); code != 200 {
		t.Fatalf("a lapsed member's list = %d, want 200 (they can still get their notes out)", code)
	}
	if code, _ := getJSONBody(t, notesReq("PUT", srv.URL+"/pro/notes/n1", "tok-lapsed", sampleNote)); code != http.StatusForbidden {
		t.Fatalf("a lapsed member's save = %d, want 403", code)
	}
	if code, _ := getJSONBody(t, notesReq("POST", srv.URL+"/pro/notes/import", "tok-lapsed", `{"notes":[{"id":"x","story":"s","title":"t","text":"x"}]}`)); code != http.StatusForbidden {
		t.Fatalf("a lapsed member's import = %d, want 403", code)
	}
	if resp, err := http.DefaultClient.Do(notesReq("DELETE", srv.URL+"/pro/notes", "tok-lapsed", "")); err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("a lapsed member's delete all = %v, %v; want allowed", resp, err)
	}
}

func TestCleanTag(t *testing.T) {
	for in, want := range map[string]string{
		" Economy ": "economy", "UPSC prep": "upsc-prep", "a__b": "a-b", "--x--": "x", "x!y": "xy", "": "", "!!!": "",
		strings.Repeat("a", 40): strings.Repeat("a", maxTagLen),
	} {
		if got := cleanTag(in); got != want {
			t.Errorf("cleanTag(%q) = %q, want %q", in, got, want)
		}
	}
}

// A note the member typed (story "own") and a reworded sentence save and sync like any other note: the same id with new
// text replaces the old one.
func TestProNotesTakeTypedNotesAndRewordedSentences(t *testing.T) {
	srv := notesFixture(t)

	typed := `{"story":"own","title":"My own notes","text":"Revise the repo rate\nand inflation","at":1700000000000}`
	code, body := getJSONBody(t, notesReq("PUT", srv.URL+"/pro/notes/own:lq9z-ab12", "tok-active", typed))
	if code != 200 || body["story"] != "own" || body["text"] != "Revise the repo rate\nand inflation" {
		t.Fatalf("typed note = %d %v, want it stored as typed (line break kept)", code, body)
	}

	_, _ = getJSONBody(t, notesReq("PUT", srv.URL+"/pro/notes/s1:abc", "tok-active", sampleNote))
	reworded := `{"story":"s1","title":"A title","text":"A reworded sentence.","note":"my thought","at":1700000000000}`
	_, _ = getJSONBody(t, notesReq("PUT", srv.URL+"/pro/notes/s1:abc", "tok-active", reworded))
	_, body = getJSONBody(t, notesReq("GET", srv.URL+"/pro/notes", "tok-active", ""))
	list, _ := body["notes"].([]any)
	if len(list) != 2 {
		t.Fatalf("notes = %v, want the typed note and one sentence (the reword replaces, it does not add)", list)
	}
	texts := map[string]string{}
	for _, n := range list {
		m := n.(map[string]any)
		texts[m["id"].(string)] = m["text"].(string)
	}
	if texts["s1:abc"] != "A reworded sentence." {
		t.Fatalf("texts = %v, want the reworded sentence under its old id", texts)
	}

	// An emptied sentence is still refused: a note is removed, not blanked.
	if code, _ := getJSONBody(t, notesReq("PUT", srv.URL+"/pro/notes/s1:abc", "tok-active", `{"story":"s1","title":"T","text":"   ","at":1}`)); code != http.StatusBadRequest {
		t.Fatalf("blank text = %d, want 400", code)
	}
}
