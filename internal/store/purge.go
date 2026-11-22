package store

import (
	"net/url"
	"strings"
)

// blockedSet turns the hosts of a takedown into a set: lower case, without "www.", blanks dropped.
func blockedSet(hosts []string) map[string]bool {
	set := map[string]bool{}
	for _, h := range hosts {
		if h = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(h)), "www."); h != "" {
			set[h] = true
		}
	}

	return set
}

// onBlockedHost reports whether rawURL is on one of the hosts. An unparsable URL is not.
func onBlockedHost(rawURL string, blocked map[string]bool) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	return blocked[strings.TrimPrefix(strings.ToLower(u.Host), "www.")]
}

// dropBlockedStories removes the stories on blocked hosts from a compilation's areas, drops an area left empty,
// and returns the areas and how many stories went. The input is not changed.
func dropBlockedStories(areas []CompArea, blocked map[string]bool) ([]CompArea, int) {
	var out []CompArea
	removed := 0
	for _, a := range areas {
		kept := make([]CompItem, 0, len(a.Items))
		for _, it := range a.Items {
			if onBlockedHost(it.URL, blocked) {
				removed++
				continue
			}
			kept = append(kept, it)
		}
		if len(kept) > 0 {
			out = append(out, CompArea{Name: a.Name, Items: kept})
		}
	}

	return out, removed
}
