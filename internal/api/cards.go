package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

const (
	defaultCardDays  = 14
	defaultCardLimit = 120
	maxCardLimit     = 300
)

// deckCard is one flashcard of the deck, with the story it came from. ID is stable for a card
// (story id and position), so a reader's progress in the browser survives a new deck.
type deckCard struct {
	ID    string `json:"id"`
	Q     string `json:"q"`
	A     string `json:"a"`
	Story string `json:"story"`
	Title string `json:"title"`
}

// cards returns the flashcard deck: the Prelims cards of the most important recent stories. A story
// written before cards existed contributes its Prelims facts, each as "recall a key fact" with the
// fact as the answer.
func (s *server) cards(w http.ResponseWriter, r *http.Request) {
	days, limit := defaultCardDays, defaultCardLimit
	if v := r.URL.Query().Get("days"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 30 {
			writeError(w, http.StatusBadRequest, "days must be between 1 and 30")
			return
		}
		days = n
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxCardLimit {
			writeError(w, http.StatusBadRequest, "limit must be between 1 and "+strconv.Itoa(maxCardLimit))
			return
		}
		limit = n
	}

	stories, err := s.Store.Cards(r.Context(), time.Now().UTC().AddDate(0, 0, -days), limit)
	if err != nil {
		s.Logger.Error("cards failed", "error", err)
		writeError(w, http.StatusInternalServerError, "cards failed")
		return
	}

	deck := make([]deckCard, 0, limit)
	for _, st := range stories {
		for _, c := range cardsOf(st) {
			if len(deck) == limit {
				break
			}
			deck = append(deck, c)
		}
	}
	// Positions are per story, so number them after the deck is built.
	next := map[string]int{}
	for i := range deck {
		deck[i].ID = deck[i].Story + ":" + strconv.Itoa(next[deck[i].Story])
		next[deck[i].Story]++
	}

	w.Header().Set("Cache-Control", "public, max-age=300")
	writeJSON(w, http.StatusOK, map[string]any{"cards": deck})
}

// cardsOf turns a story into its flashcards.
func cardsOf(st store.StoryCards) []deckCard {
	title := st.Title
	if head := strings.TrimSpace(strings.Split(title, " | ")[0]); len([]rune(head)) >= 20 {
		title = head
	}

	var out []deckCard
	for _, c := range st.Cards {
		out = append(out, deckCard{Q: c.Q, A: c.A, Story: st.ID, Title: title})
	}
	if len(out) == 0 {
		for _, f := range st.Facts {
			out = append(out, deckCard{Q: "Recall a key fact from: " + title, A: f, Story: st.ID, Title: title})
		}
	}

	return out
}
