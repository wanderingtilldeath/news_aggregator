package parse

import "testing"

func TestLooksLikeLiveBlog(t *testing.T) {
	for _, tc := range []struct {
		page Page
		want bool
	}{
		{Page{Title: "Airport chaos continues after US ground stop lifted – as it happened | World news"}, true},
		{Page{Title: "Budget 2027 live updates: Sitharaman presents"}, true},
		{Page{Title: "LIVE: Prime Minister addresses the nation"}, true},
		{Page{Title: "Minute-by-minute: how the vote unfolded"}, true},
		{Page{Title: "Election results", Snippet: "This blog is now closed. Thanks for reading."}, true},
		{Page{H1: "Live blog: floods in Assam"}, true},
		{Page{Title: "Why delivery drivers live in fear of the algorithm"}, false},
		{Page{Title: "Open Source DocumentDB 1.0 Reaches General Availability"}, false},
		{Page{Title: "How we deliver live video at scale"}, false},
	} {
		if got := tc.page.LooksLikeLiveBlog(); got != tc.want {
			t.Errorf("LooksLikeLiveBlog(%+v) = %v, want %v", tc.page, got, tc.want)
		}
	}
}
