package graph

import (
	"testing"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

var now = time.Now()

func story(id string, hoursAgo int, entities ...string) store.EntityStory {
	return store.EntityStory{ID: id, Title: "Story " + id, Source: "site.test", Time: now.Add(-time.Duration(hoursAgo) * time.Hour), Entities: entities}
}

func TestBuildRanksEntitiesAndFindsCompanions(t *testing.T) {
	g := Build([]store.EntityStory{ // newest first, as the store returns them
		story("s1", 1, "India", "Sheikh Hasina", "Bangladesh"),
		story("s2", 2, "India", "Bangladesh"),
		story("s3", 3, "india", "RBI"), // the same entity in another case
		story("s4", 4, "Sheikh Hasina"),
	}, 10, 2, 5)

	if len(g.Entities) != 4 || g.Entities[0].Name != "India" || g.Entities[0].Count != 3 {
		t.Fatalf("entities = %+v, want India first with 3 stories (case folded, shown as first written)", g.Entities)
	}
	india := g.Entities[0]
	if len(india.Stories) != 2 || india.Stories[0].ID != "s1" || india.Stories[1].ID != "s2" {
		t.Fatalf("India stories = %+v, want the newest two", india.Stories)
	}
	if len(india.Related) == 0 || india.Related[0].Name != "Bangladesh" || india.Related[0].Count != 2 {
		t.Fatalf("India related = %+v, want Bangladesh named alongside it twice", india.Related)
	}
	if len(g.Links) == 0 || g.Links[0].Count != 2 {
		t.Fatalf("links = %+v, want the strongest link first", g.Links)
	}
}

func TestBuildHonoursTheLimitsAndHandlesNoStories(t *testing.T) {
	var many []store.EntityStory
	for i := 0; i < 6; i++ {
		many = append(many, story("s"+string(rune('a'+i)), i, "Alpha", "Beta", "Gamma", "Delta"))
	}
	g := Build(many, 2, 3, 1)
	if len(g.Entities) != 2 {
		t.Fatalf("topN gave %d entities, want 2", len(g.Entities))
	}
	if len(g.Entities[0].Stories) != 3 || len(g.Entities[0].Related) != 1 {
		t.Fatalf("limits ignored: %d stories, %d related", len(g.Entities[0].Stories), len(g.Entities[0].Related))
	}
	for _, l := range g.Links {
		if (l.A != "Alpha" && l.A != "Beta") || (l.B != "Alpha" && l.B != "Beta") {
			t.Fatalf("link %+v joins an entity that is not in the graph", l)
		}
	}

	empty := Build(nil, 10, 5, 5)
	if empty.Entities == nil || empty.Links == nil || len(empty.Entities) != 0 {
		t.Fatalf("no stories must give empty lists, not nil: %+v", empty)
	}
}
