package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

// The Pro notebook: a member's saved sentences, their own notes and tags, kept on the server so they follow the
// member between devices. The member's Pro token, sent as a bearer token, is the credential, the same secret as
// in their printable-edition links. Reading and deleting work for any valid token, so a member whose period has
// ended can still get their notes out; adding and editing need an active period.
const (
	maxProNotes    = 10000
	maxNoteID      = 220
	maxNoteStory   = 200
	maxNoteTitle   = 300
	maxNoteText    = 2000
	maxNoteOwn     = 5000
	maxNoteTags    = 10
	maxTagLen      = 24
	maxNoteBody    = 16 << 10
	maxImportBody  = 2 << 20
	maxImportNotes = 500
)

type proNoteJSON struct {
	ID    string   `json:"id"`
	Story string   `json:"story"`
	Title string   `json:"title"`
	Text  string   `json:"text"`
	Note  string   `json:"note"`
	Tags  []string `json:"tags"`
	At    int64    `json:"at"`
}

func noteView(n store.ProNote) proNoteJSON {
	tags := n.Tags
	if tags == nil {
		tags = []string{}
	}

	return proNoteJSON{ID: n.ID, Story: n.Story, Title: n.Title, Text: n.Text, Note: n.Note, Tags: tags, At: n.At}
}

// cleanTag makes a tag a short lower-case word: letters and digits, with dashes between words.
func cleanTag(t string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(t)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == ' ' || r == '_':
			if s := b.String(); s != "" && !strings.HasSuffix(s, "-") {
				b.WriteByte('-')
			}
		}
		if b.Len() >= maxTagLen {
			break
		}
	}

	return strings.Trim(b.String(), "-")
}

func cleanTags(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, t := range in {
		if t = cleanTag(t); t != "" && !seen[t] && len(out) < maxNoteTags {
			seen[t] = true
			out = append(out, t)
		}
	}

	return out
}

func validNoteID(id string) bool {
	if id == "" || len(id) > maxNoteID {
		return false
	}
	for _, r := range id {
		if r < ' ' || r == 0x7f {
			return false
		}
	}

	return true
}

// cleanProNote checks a note from a client and returns the stored form, or a message saying what is wrong.
func cleanProNote(id string, in proNoteJSON) (store.ProNote, string) {
	n := store.ProNote{
		ID:    id,
		Story: strings.TrimSpace(in.Story),
		Title: strings.TrimSpace(in.Title),
		Text:  strings.TrimSpace(in.Text),
		Note:  strings.TrimSpace(in.Note),
		Tags:  cleanTags(in.Tags),
		At:    in.At,
	}
	switch {
	case !validNoteID(id):
		return n, "the note id is not valid"
	case n.Story == "" || len(n.Story) > maxNoteStory:
		return n, "the story id is missing or too long"
	case utf8.RuneCountInString(n.Title) > maxNoteTitle:
		return n, "the title is too long"
	case n.Text == "" || utf8.RuneCountInString(n.Text) > maxNoteText:
		return n, "the saved sentence is empty or too long"
	case utf8.RuneCountInString(n.Note) > maxNoteOwn:
		return n, "your note is too long"
	}
	if n.At <= 0 {
		n.At = time.Now().UnixMilli()
	}

	return n, ""
}

// proMember finds the member behind the bearer token; it answers 401 for none or an unknown one. With active it
// also answers 403 for a member whose period has ended.
func (s *server) proMember(w http.ResponseWriter, r *http.Request, active bool) (store.ProMember, bool) {
	token := bearer(r)
	if token == "" || len(token) > maxTokenLen {
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeError(w, http.StatusUnauthorized, "send your Pro link's token")
		return store.ProMember{}, false
	}
	m, ok, err := s.Store.ProByToken(r.Context(), token)
	if err != nil {
		s.Logger.Error("pro notes: member", "err", err)
		writeError(w, http.StatusInternalServerError, "could not check your membership")
		return store.ProMember{}, false
	}
	if !ok {
		writeError(w, http.StatusUnauthorized, "this Pro link is not valid")
		return store.ProMember{}, false
	}
	if active && !m.Until.After(time.Now()) {
		writeError(w, http.StatusForbidden, "your Pro period has ended; renew to add or change notes")
		return store.ProMember{}, false
	}

	return m, true
}

func (s *server) listProNotes(w http.ResponseWriter, r *http.Request) {
	m, ok := s.proMember(w, r, false)
	if !ok {
		return
	}
	notes, err := s.Store.ProNotes(r.Context(), m.Email)
	if err != nil {
		s.Logger.Error("pro notes: list", "err", err)
		writeError(w, http.StatusInternalServerError, "could not load your notes")
		return
	}
	out := make([]proNoteJSON, 0, len(notes))
	for _, n := range notes {
		out = append(out, noteView(n))
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"notes": out, "limit": maxProNotes})
}

func (s *server) saveProNote(w http.ResponseWriter, r *http.Request) {
	m, ok := s.proMember(w, r, true)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxNoteBody)
	var in proNoteJSON
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	n, msg := cleanProNote(r.PathValue("id"), in)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	saved, err := s.Store.SaveProNote(r.Context(), m.Email, n, maxProNotes)
	if err != nil {
		s.Logger.Error("pro notes: save", "err", err)
		writeError(w, http.StatusInternalServerError, "could not save the note")
		return
	}
	if !saved {
		writeError(w, http.StatusConflict, "your notebook is full; delete a note to add another")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, noteView(n))
}

type importRequest struct {
	Notes []proNoteJSON `json:"notes"`
}

// importProNotes adds notes a member kept in a browser before connecting Pro. It only adds: a note the server
// already has (maybe edited since) is left as it is.
func (s *server) importProNotes(w http.ResponseWriter, r *http.Request) {
	m, ok := s.proMember(w, r, true)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxImportBody)
	var in importRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(in.Notes) == 0 || len(in.Notes) > maxImportNotes {
		writeError(w, http.StatusBadRequest, "send 1 to 500 notes at a time")
		return
	}
	notes := make([]store.ProNote, 0, len(in.Notes))
	for _, raw := range in.Notes {
		n, msg := cleanProNote(raw.ID, raw)
		if msg != "" {
			writeError(w, http.StatusBadRequest, msg)
			return
		}
		notes = append(notes, n)
	}
	added, err := s.Store.AddProNotes(r.Context(), m.Email, notes, maxProNotes)
	if err != nil {
		s.Logger.Error("pro notes: import", "err", err)
		writeError(w, http.StatusInternalServerError, "could not import the notes")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]int{"added": added})
}

// deleteProNote removes one note, or all of them when no id is given.
func (s *server) deleteProNote(w http.ResponseWriter, r *http.Request) {
	m, ok := s.proMember(w, r, false)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if id != "" && !validNoteID(id) {
		writeError(w, http.StatusBadRequest, "the note id is not valid")
		return
	}
	n, err := s.Store.DeleteProNote(r.Context(), m.Email, id)
	if err != nil {
		s.Logger.Error("pro notes: delete", "err", err)
		writeError(w, http.StatusInternalServerError, "could not delete")
		return
	}
	if id != "" && n == 0 {
		writeError(w, http.StatusNotFound, "no such note")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
