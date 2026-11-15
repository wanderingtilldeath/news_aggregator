package compile

import (
	"testing"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

var day = time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC)

func story(id string, imp int, hoursAgo int, tags ...string) store.Item {
	return store.Item{
		ID: id, URL: "https://site.test/" + id, Title: "Story " + id, Source: "www.site.test", Importance: imp,
		Time: day.Add(-time.Duration(hoursAgo) * time.Hour), Tags: tags, Why: "why " + id, Summary: []string{"a", "b", "c", "d", "e"},
	}
}

func names(areas []store.CompArea) []string {
	var out []string
	for _, a := range areas {
		out = append(out, a.Name)
	}

	return out
}

func ids(a store.CompArea) string {
	s := ""
	for _, c := range a.Items {
		s += c.ID + " "
	}

	return s
}

func TestMergeGroupsByAreaInSyllabusOrderAndKeepsTheBest(t *testing.T) {
	items := []store.Item{
		story("eco1", 4, 2, "gs3-economy"),
		story("pol1", 5, 3, "gs2-polity-governance"),
		story("pol2", 3, 1, "gs2-polity-governance"),
		story("low", 2, 1, "gs2-polity-governance"), // below the importance bar
		story("none", 4, 1),                         // no tags: Other
		story("unk", 4, 1, "kubernetes"),            // not a syllabus tag: Other
	}
	got := Merge(nil, items, "upsc", 2)

	if want := []string{"Polity and governance (GS2)", "Economy (GS3)", "Other"}; len(got) != 3 || got[0].Name != want[0] || got[1].Name != want[1] || got[2].Name != want[2] {
		t.Fatalf("areas = %v, want %v (syllabus order, Other last)", names(got), want)
	}
	if ids(got[0]) != "pol1 pol2 " {
		t.Fatalf("polity = %q, want the most important first and the low story left out", ids(got[0]))
	}
	if len(got[2].Items) != 2 {
		t.Fatalf("other = %q, want the untagged and the unknown-tag story", ids(got[2]))
	}
	if n := len(got[0].Items[0].Bullets); n != maxBullets {
		t.Fatalf("a story kept %d bullets, want %d", n, maxBullets)
	}
}

func TestMergeAddsDayByDayWithoutDuplicatesAndCapsAnArea(t *testing.T) {
	first := Merge(nil, []store.Item{story("a", 4, 5, "gs3-economy"), story("b", 3, 5, "gs3-economy")}, "upsc", 2)

	// The next day: a is seen again (and re-scored), c is better than b, and the cap is two.
	again := story("a", 5, 5, "gs3-economy")
	second := Merge(first, []store.Item{again, story("c", 4, 1, "gs3-economy")}, "upsc", 2)
	if len(second) != 1 || ids(second[0]) != "a c " {
		t.Fatalf("economy = %q, want a (re-scored to 5) then c, with b pushed out by the cap", ids(second[0]))
	}
	if second[0].Items[0].Importance != 5 {
		t.Fatalf("the new copy of a story must win: %+v", second[0].Items[0])
	}

	// A story whose tags changed moves area instead of appearing twice.
	moved := Merge(second, []store.Item{story("a", 5, 5, "gs4-ethics")}, "upsc", 5)
	total := 0
	for _, a := range moved {
		total += len(a.Items)
	}
	if total != 2 || len(moved) != 2 {
		t.Fatalf("after the move: %v with %d stories, want 2 areas and 2 stories", names(moved), total)
	}
}

func TestMergeNamesEngineeringAreasFromTagsAndSkipsUnsafeLinks(t *testing.T) {
	bad := story("js", 5, 1, "javascript:alert(1)")
	bad.URL = "javascript:alert(1)"
	got := Merge(nil, []store.Item{story("k", 4, 1, "kubernetes"), story("m", 4, 1, "ai-models"), bad}, "engineering", 5)
	if want := []string{"Ai models", "Kubernetes"}; len(got) != 2 || got[0].Name != want[0] || got[1].Name != want[1] {
		t.Fatalf("areas = %v, want %v (topic names, alphabetical, the unsafe link left out)", names(got), want)
	}
}
