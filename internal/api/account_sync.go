package api

import (
	"encoding/json"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

// What a free account carries between devices (see store.SyncData). The browser keeps the working copy; these
// endpoints keep one copy per account so a second device gets the same bookmarks, flashcard progress, streak and
// missed quiz questions, and the places starred and notes written on the revision maps. The notebook is a Pro feature and has
// its own endpoints.
const (
	maxSyncBody      = 1 << 20
	maxSyncBookmarks = 300
	maxSyncMarks     = 300
	maxSyncMissed    = 30
	maxSyncCards     = 5000
	maxSyncBox       = 5
)

var streakRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}\|\d{1,5}$`)

type syncBookmark struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Source string `json:"source"`
	Time   string `json:"time"`
	At     int64  `json:"at"`
}

// syncMark is a place a reader starred on a revision map; Item is the place's id on that map.
type syncMark struct {
	ID   string  `json:"id"`
	Map  string  `json:"map"`
	Base string  `json:"base"`
	Lon  float64 `json:"lon"`
	Lat  float64 `json:"lat"`
	Name string  `json:"name"`
	Item string  `json:"item"`
	From string  `json:"from"`
	At   int64   `json:"at"`
}

type syncCard struct {
	Box int   `json:"box"`
	Due int64 `json:"due"`
}

type syncQuestion struct {
	Question    string   `json:"question"`
	Options     []string `json:"options"`
	Answer      int      `json:"answer"`
	Explanation string   `json:"explanation"`
	URL         string   `json:"url,omitempty"`
}

// syncState is the decoded form of store.SyncData.
type syncState struct {
	Bookmarks []syncBookmark      `json:"bookmarks"`
	Cards     map[string]syncCard `json:"cards"`
	Streak    string              `json:"streak"`
	Missed    []syncQuestion      `json:"missed"`
	Marks     []syncMark          `json:"marks"`
}

// syncRequest is a PUT: the kinds the browser sends (a missing kind is left as stored) and how to apply them. Mode
// "merge" unions them with what is stored (the first time a device meets the account); "replace" makes the browser's
// copy the account's (a change made on this device). The streak is always merged: it only moves forward.
type syncRequest struct {
	Mode      string               `json:"mode"`
	Bookmarks *[]syncBookmark      `json:"bookmarks"`
	Cards     *map[string]syncCard `json:"cards"`
	Streak    *string              `json:"streak"`
	Missed    *[]syncQuestion      `json:"missed"`
	Marks     *[]syncMark          `json:"marks"`
}

func cut(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}

	return s
}

func decodeSync(d store.SyncData) syncState {
	st := syncState{Cards: map[string]syncCard{}}
	_ = json.Unmarshal([]byte(d.Bookmarks), &st.Bookmarks)
	_ = json.Unmarshal([]byte(d.Cards), &st.Cards)
	_ = json.Unmarshal([]byte(d.Missed), &st.Missed)
	_ = json.Unmarshal([]byte(d.Marks), &st.Marks)
	st.Streak = d.Streak
	if st.Cards == nil {
		st.Cards = map[string]syncCard{}
	}

	return st
}

func (st syncState) encode() store.SyncData {
	b, _ := json.Marshal(nilSafe(st.Bookmarks))
	c, _ := json.Marshal(st.Cards)
	m, _ := json.Marshal(nilSafe(st.Missed))
	k, _ := json.Marshal(nilSafe(st.Marks))

	return store.SyncData{Bookmarks: string(b), Cards: string(c), Streak: st.Streak, Missed: string(m), Marks: string(k)}
}

func nilSafe[T any](l []T) []T {
	if l == nil {
		return []T{}
	}

	return l
}

// cleanBookmarks drops malformed entries and caps lengths, keeping the first of a repeated id.
func cleanBookmarks(in []syncBookmark) []syncBookmark {
	seen := map[string]bool{}
	out := make([]syncBookmark, 0, len(in))
	for _, b := range in {
		if b.ID == "" || len(b.ID) > 200 || b.Title == "" || seen[b.ID] {
			continue
		}
		seen[b.ID] = true
		b.Title, b.Source, b.Time = cut(b.Title, 300), cut(b.Source, 300), cut(b.Time, 40)
		out = append(out, b)
		if len(out) == maxSyncBookmarks {
			break
		}
	}

	return out
}

// cleanMarks drops malformed marks (and the free notes an earlier version kept, which have no place of a map), caps lengths, and
// keeps the first of a repeated id.
func cleanMarks(in []syncMark) []syncMark {
	seen := map[string]bool{}
	out := make([]syncMark, 0, len(in))
	for _, m := range in {
		if m.ID == "" || len(m.ID) > 200 || m.Item == "" || seen[m.ID] || math.IsNaN(m.Lon) || math.IsNaN(m.Lat) || math.Abs(m.Lon) > 180 || math.Abs(m.Lat) > 90 {
			continue
		}
		seen[m.ID] = true
		if m.Base != "world" {
			m.Base = "india"
		}
		if m.From != "test" {
			m.From = "star"
		}
		m.Map, m.Item, m.Name = cut(m.Map, 100), cut(m.Item, 120), cut(strings.TrimSpace(m.Name), 60)
		if m.Name == "" {
			m.Name = "My place"
		}
		out = append(out, m)
		if len(out) == maxSyncMarks {
			break
		}
	}

	return out
}

func cleanMissed(in []syncQuestion) []syncQuestion {
	seen := map[string]bool{}
	out := make([]syncQuestion, 0, len(in))
	for _, q := range in {
		if q.Question == "" || len(q.Options) < 2 || len(q.Options) > 8 || q.Answer < 0 || q.Answer >= len(q.Options) || seen[q.Question] {
			continue
		}
		seen[q.Question] = true
		q.Question, q.Explanation, q.URL = cut(q.Question, 500), cut(q.Explanation, 1000), cut(q.URL, 500)
		for i := range q.Options {
			q.Options[i] = cut(q.Options[i], 300)
		}
		out = append(out, q)
		if len(out) == maxSyncMissed {
			break
		}
	}

	return out
}

func cleanCards(in map[string]syncCard) map[string]syncCard {
	out := make(map[string]syncCard, len(in))
	for id, c := range in {
		if id == "" || len(id) > 100 || len(out) >= maxSyncCards {
			continue
		}
		c.Box = min(max(c.Box, 0), maxSyncBox)
		out[id] = c
	}

	return out
}

func parseStreak(s string) (day time.Time, n int, ok bool) {
	if !streakRe.MatchString(s) {
		return time.Time{}, 0, false
	}
	day, err := time.Parse("2006-01-02", s[:10])
	if err != nil {
		return time.Time{}, 0, false
	}
	n, _ = strconv.Atoi(s[11:])

	return day, n, true
}

// mergeStreak keeps the later day; on the same day the longer count. A fresh one-day streak that starts the day after
// the other one ended continues it instead of replacing it, since a new device would otherwise reset a long streak.
func mergeStreak(a, b string) string {
	da, na, oka := parseStreak(a)
	db, nb, okb := parseStreak(b)
	switch {
	case !oka:
		if okb {
			return b
		}
		return ""
	case !okb:
		return a
	}
	if da.Equal(db) {
		return a[:10] + "|" + strconv.Itoa(max(na, nb))
	}
	late, early, nl, ne, ls := da, db, na, nb, a
	if db.After(da) {
		late, early, nl, ne, ls = db, da, nb, na, b
	}
	if late.Sub(early) == 24*time.Hour && nl == 1 {
		return ls[:10] + "|" + strconv.Itoa(ne+1)
	}

	return ls
}

// mergeSync applies a request to the stored state and returns the new state.
func mergeSync(stored syncState, req syncRequest) syncState {
	out := stored
	union := req.Mode != "replace"

	if req.Bookmarks != nil {
		in := cleanBookmarks(*req.Bookmarks)
		if union {
			all := append(append([]syncBookmark{}, in...), out.Bookmarks...)
			sort.SliceStable(all, func(i, j int) bool { return all[i].At > all[j].At })
			in = cleanBookmarks(all)
		}
		out.Bookmarks = in
	}
	if req.Marks != nil {
		in := cleanMarks(*req.Marks)
		if union {
			all := append(append([]syncMark{}, in...), out.Marks...)
			sort.SliceStable(all, func(i, j int) bool { return all[i].At > all[j].At })
			in = cleanMarks(all)
		}
		out.Marks = in
	}
	if req.Missed != nil {
		in := cleanMissed(*req.Missed)
		if union {
			in = cleanMissed(append(append([]syncQuestion{}, in...), out.Missed...))
		}
		out.Missed = in
	}
	if req.Cards != nil {
		in := cleanCards(*req.Cards)
		if union {
			for id, c := range out.Cards {
				if have, ok := in[id]; !ok || c.Box > have.Box || (c.Box == have.Box && c.Due > have.Due) {
					in[id] = c
				}
			}
			in = cleanCards(in)
		}
		out.Cards = in
	}
	if req.Streak != nil {
		out.Streak = mergeStreak(out.Streak, strings.TrimSpace(*req.Streak))
	}

	return out
}

func (s *server) accountSyncGet(w http.ResponseWriter, r *http.Request) {
	a, ok := s.accountFrom(r)
	if !ok {
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeError(w, http.StatusUnauthorized, "log in first")
		return
	}
	d, err := s.Store.AccountSync(r.Context(), a.ID)
	if err != nil {
		s.Logger.Error("account sync get", "err", err)
		writeError(w, http.StatusInternalServerError, "could not load your saved items")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, decodeSync(d))
}

func (s *server) accountSyncPut(w http.ResponseWriter, r *http.Request) {
	a, ok := s.accountFrom(r)
	if !ok {
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeError(w, http.StatusUnauthorized, "log in first")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxSyncBody)
	var req syncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Mode != "merge" && req.Mode != "replace" {
		writeError(w, http.StatusBadRequest, `mode must be "merge" or "replace"`)
		return
	}

	d, err := s.Store.AccountSync(r.Context(), a.ID)
	if err != nil {
		s.Logger.Error("account sync read", "err", err)
		writeError(w, http.StatusInternalServerError, "could not save your items")
		return
	}
	next := mergeSync(decodeSync(d), req)
	if err := s.Store.SetAccountSync(r.Context(), a.ID, next.encode()); err != nil {
		s.Logger.Error("account sync save", "err", err)
		writeError(w, http.StatusInternalServerError, "could not save your items")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, next)
}
