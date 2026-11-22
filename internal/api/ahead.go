package api

import (
	"net/http"
	"time"
)

const (
	aheadDays  = 14
	aheadLimit = 50
)

// ahead lists the stories with a dated event to watch in the next two weeks, soonest first, for the Week ahead
// page. The dates come from the enrichment pass (store.Extra) and were checked there; the window is counted in
// the edition's time zone, so "today" is the reader's India day.
func (s *server) ahead(w http.ResponseWriter, r *http.Request) {
	today := time.Now().In(editionZone)
	from, to := today.Format("2006-01-02"), today.AddDate(0, 0, aheadDays).Format("2006-01-02")

	items, err := s.Store.Ahead(r.Context(), from, to, aheadLimit)
	if err != nil {
		s.Logger.Error("ahead failed", "error", err)
		writeError(w, http.StatusInternalServerError, "ahead failed")
		return
	}

	w.Header().Set("Cache-Control", "public, max-age=600")
	writeJSON(w, http.StatusOK, map[string]any{"from": from, "to": to, "items": items})
}
