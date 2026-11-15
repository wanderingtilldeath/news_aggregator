package group

import (
	"testing"
	"time"
)

var now = time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)

func s(id, host, title string, hoursAgo int) Story {
	return Story{ID: id, Host: host, Title: title, Time: now.Add(-time.Duration(hoursAgo) * time.Hour)}
}

func TestClusterGroupsTheSameEventAcrossSources(t *testing.T) {
	got := Cluster([]Story{
		s("c", "www.aljazeera.com", "US suggested Ukraine cede parts of Donbas to Russia to become part of NATO | Al Jazeera", 3),
		s("a", "www.theguardian.com", "Zelenskyy says US floated Ukraine ceding Donbas to Russia in exchange for NATO membership", 2),
		s("b", "scroll.in", "Why India's trade deal with the US matters for textile exporters", 2),
		s("d", "news.un.org", "Airport chaos continues as flight stoppage stems from routine maintenance", 1),
		s("e", "www.theguardian.com", "US officials say flight stoppage stemmed from routine maintenance as travel chaos continues", 1),
	})

	if got["a"].Story != "a" || got["c"].Story != "a" || got["a"].Sources != 2 {
		t.Fatalf("the Donbas stories must share group %q, got %v", "a", got)
	}
	if got["d"].Story != "d" || got["e"].Story != "d" {
		t.Fatalf("the flight stories must share group %q, got %v", "d", got)
	}
	if _, ok := got["b"]; ok {
		t.Fatalf("an unrelated story must stay out of every group: %v", got)
	}
}

func TestClusterNeedsDifferentSourcesAndNearbyDates(t *testing.T) {
	// Two stories from one source are not "covered by several sources".
	got := Cluster([]Story{
		s("x1", "a.test", "Parliament passes the new data protection bill after long debate", 1),
		s("x2", "a.test", "Parliament passes the new data protection bill after a long debate", 2),
	})
	if len(got) != 0 {
		t.Fatalf("same-source stories grouped: %v", got)
	}

	// The same words a week apart are two different events.
	got = Cluster([]Story{
		s("y1", "a.test", "Parliament passes the new data protection bill after long debate", 1),
		s("y2", "b.test", "Parliament passes the new data protection bill after long debate", 24*8),
	})
	if len(got) != 0 {
		t.Fatalf("stories a week apart grouped: %v", got)
	}
}

func TestClusterIgnoresThinOverlap(t *testing.T) {
	got := Cluster([]Story{
		s("p", "a.test", "India wins the first test against Australia in Perth", 1),
		s("q", "b.test", "India signs a new trade agreement with Australia", 1),
	})
	if len(got) != 0 {
		t.Fatalf("two different India-Australia stories were grouped: %v", got)
	}
}

func TestClusterLeavesRoundupsOut(t *testing.T) {
	got := Cluster([]Story{
		s("a", "www.theguardian.com", "Ukraine war briefing: Zelenskyy says Starshield restrictions continue on 'entire front line'", 2),
		s("b", "www.reuters.com", "Zelenskyy says Starshield restrictions continue on entire front line", 2),
	})
	if len(got) != 0 {
		t.Fatalf("a briefing was grouped with a single story: %v", got)
	}
}
