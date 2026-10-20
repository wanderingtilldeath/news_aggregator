package scheduler

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestShippedSourcesFilesAreValid keeps sources.json and the catalog/*.json files
// loadable and expandable, so a typo in a seed, layout or prefix is caught here
// and not on the first morning the job runs.
func TestShippedSourcesFilesAreValid(t *testing.T) {
	files, err := filepath.Glob("../../sources*.json")
	catalog, _ := filepath.Glob("../../catalog/*.json")
	files = append(files, catalog...)
	if err != nil || len(files) == 0 {
		t.Fatalf("no sources files found: %v", err)
	}

	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			sources, err := LoadSources(f)
			if err != nil {
				t.Fatal(err)
			}

			names := map[string]bool{}
			for _, s := range sources {
				if names[s.Name] {
					t.Errorf("duplicate source name %q", s.Name)
				}
				names[s.Name] = true
				if f == "../../sources.json" && s.Category == "" {
					t.Errorf("%s has no category", s.Name)
				}

				// Both sides of the year boundary must expand.
				for _, day := range []time.Time{time.Date(2026, 12, 31, 6, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 6, 0, 0, 0, time.UTC)} {
					specs, err := s.Specs(day)
					if err != nil || len(specs) == 0 {
						t.Errorf("%s: Specs(%s) = %v, %v", s.Name, day.Format("2006-01-02"), specs, err)
						continue
					}
					for _, sp := range specs {
						if sp.Seed == "" || sp.MaxDepth < 0 {
							t.Errorf("%s: bad spec %+v", s.Name, sp)
						}
					}
				}
			}
		})
	}
}

// TestDefaultSourcesIncludeTheRequiredSites pins the owner's rule for the
// default set: it always includes Drishti IAS, InsightsIAS and Indian Express.
func TestDefaultSourcesIncludeTheRequiredSites(t *testing.T) {
	sources, err := LoadSources("../../sources.json")
	if err != nil {
		t.Fatal(err)
	}

	sites := Sites(sources)

	have := map[string]bool{}
	for _, s := range sites {
		have[s] = true
	}
	for _, must := range []string{"drishtiias.com", "insightsonindia.com", "indianexpress.com"} {
		if !have[must] {
			t.Errorf("sources.json must always include %s; has %v", must, sites)
		}
	}

	// The catalog is a menu to pick from, so it must not repeat the default.
	defaultSeeds := map[string]bool{}
	for _, s := range sources {
		defaultSeeds[s.Seed] = true
	}
	catalog, _ := filepath.Glob("../../catalog/*.json")
	for _, f := range catalog {
		entries, err := LoadSources(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if defaultSeeds[e.Seed] {
				t.Errorf("%s repeats %q from sources.json", filepath.Base(f), e.Name)
			}
		}
	}
}

func TestSiteLimit(t *testing.T) {
	src := func(name, seed string) Source { return Source{Name: name, Seed: seed} }
	sources := []Source{
		src("Anthropic News", "https://www.anthropic.com/news"),
		src("Anthropic Research", "https://www.anthropic.com/research"), // same site
		src("Drishti 1", "https://www.drishtiias.com/a/{date:02-01-2006}"),
		src("Drishti 2", "https://www.drishtiias.com/b/{date:02-01-2006}"), // same site
		src("TNW", "https://thenextweb.com/"),
	}

	if got := Sites(sources); len(got) != 3 || got[0] != "anthropic.com" || got[1] != "drishtiias.com" || got[2] != "thenextweb.com" {
		t.Errorf("Sites = %v, want 3 distinct hosts without www", got)
	}
	if err := CheckSiteLimit(sources, 3); err != nil {
		t.Errorf("3 sites within a limit of 3: %v", err)
	}

	if err := CheckSiteLimit(sources, 0); err != nil {
		t.Errorf("a limit of 0 means no limit: %v", err)
	}

	err := CheckSiteLimit(sources, 2)
	if err == nil {
		t.Fatal("3 sites above a limit of 2 must fail")
	}
	for _, want := range []string{"3 sites", "at most 2", "anthropic.com", "thenextweb.com", "-max-sites"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should mention %q", err, want)
		}
	}
}
