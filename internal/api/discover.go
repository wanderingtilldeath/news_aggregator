package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/graph"
)

const (
	maxConceptsShown = 500
	defaultEntityTop = 150
	entityStories    = 12
	entityRelated    = 8
)

// concepts returns the glossary: every term that has an explainer.
func (s *server) concepts(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.Concepts(r.Context(), maxConceptsShown)
	if err != nil {
		s.Logger.Error("concepts failed", "error", err)
		writeError(w, http.StatusInternalServerError, "concepts failed")
		return
	}

	w.Header().Set("Cache-Control", "public, max-age=300")
	writeJSON(w, http.StatusOK, map[string]any{"concepts": list})
}

// entities returns the knowledge graph of the last days days (14 by default): the limit most
// mentioned entities, each with its newest stories and who is named alongside it.
func (s *server) entities(w http.ResponseWriter, r *http.Request) {
	days, limit := 14, defaultEntityTop
	if v := r.URL.Query().Get("days"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 60 {
			writeError(w, http.StatusBadRequest, "days must be between 1 and 60")
			return
		}
		days = n
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 300 {
			writeError(w, http.StatusBadRequest, "limit must be between 1 and 300")
			return
		}
		limit = n
	}

	stories, err := s.Store.EntityStories(r.Context(), time.Now().UTC().AddDate(0, 0, -days))
	if err != nil {
		s.Logger.Error("entities failed", "error", err)
		writeError(w, http.StatusInternalServerError, "entities failed")
		return
	}

	w.Header().Set("Cache-Control", "public, max-age=300")
	writeJSON(w, http.StatusOK, graph.Build(stories, limit, entityStories, entityRelated))
}
