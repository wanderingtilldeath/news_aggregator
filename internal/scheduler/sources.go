// Package scheduler runs the daily digest job: start a crawl for every
// configured news source, wait for the crawls to finish, then summarise the
// new articles. Several crawlerx processes may run it; a per-day claim in
// Postgres makes sure it runs once.
package scheduler

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/frontier"
	"github.com/wanderingtilldeath/news-aggregator/internal/urlnorm"
)

const (
	defaultSourceDepth = 1
	maxSourceDepth     = 10
)

// Source is one site section to crawl every day.
type Source struct {
	Name string `json:"name"`
	// Note is free text for people reading the file (for example what the site's
	// robots.txt says about AI crawlers). The scheduler ignores it.
	Note string `json:"note,omitempty"`
	// Seed is the listing or category page the crawl starts from.
	Seed string `json:"seed"`
	// MaxDepth is how many links to follow from the seed; 1 reaches the
	// articles a listing page links to. Default 1.
	MaxDepth *int `json:"max_depth"`
	// AllowedHosts defaults to the seed's host.
	AllowedHosts []string `json:"allowed_hosts"`
	// PathPrefixes keeps the crawl inside one section of the site.
	PathPrefixes []string `json:"path_prefixes"`
	// ArticlePrefixes, when set, makes only pages under these paths count as
	// digest articles, whatever dates they carry. Use it for sites whose
	// article pages have no machine-readable date.
	ArticlePrefixes []string `json:"article_prefixes"`
	// SplitArticles turns a page of several <article id> topics into one
	// digest item per topic.
	SplitArticles bool `json:"split_articles"`
	// ImageFromSource fetches the original story each article cites (a link in
	// its text) and uses that page's lead image, for sites whose own pages
	// carry only a generic logo. One small request per new article.
	ImageFromSource bool `json:"image_from_source"`
	// DaysBack makes a date-templated seed run for today and this many days
	// before it, so a first run stays recent. Needs a {date:...} seed.
	DaysBack int `json:"days_back"`
	// Category groups the source in the digest, for example "tech", "ai",
	// "upsc" or "world": a lowercase slug. Empty shows as "other".
	Category string `json:"category"`
}

var categorySlug = regexp.MustCompile(`^[a-z0-9-]{1,24}$`)

// datePlaceholder matches {date:LAYOUT} where LAYOUT is a Go time layout,
// e.g. {date:02-01-2006} for 19-10-2026.
var datePlaceholder = regexp.MustCompile(`\{date:([^}]+)\}`)

// expandDate replaces every {date:LAYOUT} in s with day formatted by LAYOUT.
func expandDate(s string, day time.Time) (string, error) {
	var badLayout string
	out := datePlaceholder.ReplaceAllStringFunc(s, func(m string) string {
		layout := datePlaceholder.FindStringSubmatch(m)[1]
		formatted := day.Format(layout)
		if formatted == layout {
			badLayout = layout
		}
		return formatted
	})
	if badLayout != "" {
		return "", fmt.Errorf("%q is not a Go time layout (try 02-01-2006, or 2006 for the year)", badLayout)
	}
	return out, nil
}

func expandAll(list []string, day time.Time) ([]string, error) {
	var out []string
	for _, item := range list {
		v, err := expandDate(item, day)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// Specs returns the crawls to start for this source on the day of now: one per
// day from now back DaysBack days, with {date:LAYOUT} in the seed replaced by
// that day's date. A source with no placeholder gives exactly one crawl.
func (s Source) Specs(now time.Time) ([]frontier.CrawlSpec, error) {
	var specs []frontier.CrawlSpec
	seen := map[string]bool{}

	// Prefixes may carry {date:2006} so a year-based path need not be edited
	// every January; they use the run day.
	pathPrefixes, err := expandAll(s.PathPrefixes, now)
	if err != nil {
		return nil, fmt.Errorf("source %q: %w", s.Name, err)
	}
	articlePrefixes, err := expandAll(s.ArticlePrefixes, now)
	if err != nil {
		return nil, fmt.Errorf("source %q: %w", s.Name, err)
	}

	for d := 0; d <= s.DaysBack; d++ {
		expanded, err := expandDate(s.Seed, now.AddDate(0, 0, -d))
		if err != nil {
			return nil, fmt.Errorf("source %q: %w", s.Name, err)
		}

		seed, err := urlnorm.Canonicalize(expanded)
		if err != nil {
			return nil, fmt.Errorf("source %q: seed must be an absolute http(s) URL", s.Name)
		}
		if seen[seed] {
			continue
		}
		seen[seed] = true

		specs = append(specs, frontier.CrawlSpec{
			Seed:            seed,
			MaxDepth:        s.Depth(),
			AllowedHosts:    s.AllowedHosts,
			PathPrefixes:    pathPrefixes,
			ArticlePrefixes: articlePrefixes,
			SplitArticles:   s.SplitArticles,
			ImageFromSource: s.ImageFromSource,
			Category:        s.Category,
		})
	}

	return specs, nil
}

// Depth returns MaxDepth or its default.
func (s Source) Depth() int {
	if s.MaxDepth == nil {
		return defaultSourceDepth
	}
	return *s.MaxDepth
}

// LoadSources reads and validates sources files: each a JSON array of Source.
// path may name several files separated by commas (e.g.
// "sources.json,extra.json"); their sources are combined, in order.
func LoadSources(path string) ([]Source, error) {
	var all []Source
	for _, p := range strings.Split(path, ",") {
		if p = strings.TrimSpace(p); p == "" {
			continue
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("sources: %w", err)
		}
		sources, err := ParseSources(raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		all = append(all, sources...)
	}
	if len(all) == 0 {
		return nil, errors.New("sources: no sources file given")
	}

	return all, nil
}

// ParseSources is LoadSources on bytes.
func ParseSources(raw []byte) ([]Source, error) {
	var sources []Source
	if err := json.Unmarshal(raw, &sources); err != nil {
		return nil, fmt.Errorf("sources: invalid JSON (want an array): %w", err)
	}
	if len(sources) == 0 {
		return nil, errors.New("sources: the list is empty")
	}

	var errs []error
	for i := range sources {
		s := &sources[i]

		s.Seed = strings.TrimSpace(s.Seed)
		unnamed := strings.TrimSpace(s.Name) == ""
		if unnamed {
			s.Name = s.Seed
		}

		s.Category = strings.ToLower(strings.TrimSpace(s.Category))
		if s.Category != "" && !categorySlug.MatchString(s.Category) {
			errs = append(errs, fmt.Errorf("source %q: category must be a lowercase slug of up to 24 letters, digits or hyphens", s.Name))
			continue
		}

		if s.DaysBack < 0 || s.DaysBack > 30 {
			errs = append(errs, fmt.Errorf("source %q: days_back must be between 0 and 30", s.Name))
			continue
		}

		// The seed may be a date template, so validate by expanding it.
		specs, err := s.Specs(time.Now())
		if err != nil {
			errs = append(errs, fmt.Errorf("source %d: %w", i+1, err))
			continue
		}
		if unnamed {
			s.Name = urlnorm.Host(specs[0].Seed)
		}
		if s.DaysBack > 0 && !datePlaceholder.MatchString(s.Seed) {
			errs = append(errs, fmt.Errorf("source %q: days_back needs a {date:LAYOUT} placeholder in the seed", s.Name))
		}
		if d := s.Depth(); d < 0 || d > maxSourceDepth {
			errs = append(errs, fmt.Errorf("source %q: max_depth must be between 0 and %d", s.Name, maxSourceDepth))
		}
	}

	return sources, errors.Join(errs...)
}

// Sites returns the distinct sites (hosts, without a leading "www.") that
// sources crawl, sorted. Several sources on one host, such as a site's news and
// research sections, are one site.
func Sites(sources []Source) []string {
	seen := map[string]bool{}
	var out []string
	for _, src := range sources {
		specs, err := src.Specs(time.Now())
		if err != nil || len(specs) == 0 {
			continue
		}
		if h := strings.TrimPrefix(urlnorm.Host(specs[0].Seed), "www."); h != "" && !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	sort.Strings(out)
	return out
}

// CheckSiteLimit fails when sources cover more than max distinct sites; a max
// of 0 or less means no limit. Each site costs crawl time and summaries, so an
// owner who wants a bounded daily run (and LLM bill) can set one.
func CheckSiteLimit(sources []Source, max int) error {
	sites := Sites(sources)
	if max <= 0 || len(sites) <= max {
		return nil
	}
	return fmt.Errorf("sources: %d sites configured but at most %d are allowed (-max-sites): %s; remove some, or raise the limit if you accept the extra crawl and summary cost",
		len(sites), max, strings.Join(sites, ", "))
}
