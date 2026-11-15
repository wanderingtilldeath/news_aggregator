package api

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/wanderingtilldeath/news-aggregator/internal/track"
)

var monthPattern = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

// month returns the compilation of a month (month=YYYY-MM) and edition (track=).
func (s *server) month(w http.ResponseWriter, r *http.Request) {
	month, tr := r.URL.Query().Get("month"), r.URL.Query().Get("track")
	if !monthPattern.MatchString(month) {
		writeError(w, http.StatusBadRequest, "month must be YYYY-MM")
		return
	}
	if !track.Valid(tr) {
		writeError(w, http.StatusBadRequest, "track must be one of: "+strings.Join(track.All(), ", "))
		return
	}

	c, ok, err := s.Store.GetCompilation(r.Context(), month, tr)
	if err != nil {
		s.Logger.Error("month failed", "error", err)
		writeError(w, http.StatusInternalServerError, "month failed")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "no compilation for that month yet")
		return
	}

	w.Header().Set("Cache-Control", "public, max-age=300")
	writeJSON(w, http.StatusOK, c)
}

// months lists the months that have a compilation, newest first.
func (s *server) months(w http.ResponseWriter, r *http.Request) {
	months, err := s.Store.CompilationMonths(r.Context())
	if err != nil {
		s.Logger.Error("months failed", "error", err)
		writeError(w, http.StatusInternalServerError, "months failed")
		return
	}
	if months == nil {
		months = []string{}
	}

	w.Header().Set("Cache-Control", "public, max-age=300")
	writeJSON(w, http.StatusOK, map[string]any{"months": months})
}
