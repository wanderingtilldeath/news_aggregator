package api

import (
	"testing"

	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

func TestCardsOfFallsBackToWhy(t *testing.T) {
	got := cardsOf(store.StoryCards{ID: "s1", Title: "Parliament passes a bill", Why: "It changes how elections are funded."})
	if len(got) != 1 || got[0].A != "It changes how elections are funded." || got[0].Story != "s1" {
		t.Fatalf("cardsOf = %+v; want one card answered by the why line", got)
	}
	if got := cardsOf(store.StoryCards{Title: "T", Why: "w", Facts: []string{"fact"}}); len(got) != 1 || got[0].A != "fact" {
		t.Fatalf("cardsOf with facts = %+v; facts must win over the why line", got)
	}
}
