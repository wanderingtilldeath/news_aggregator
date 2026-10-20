// Command checksource tells you whether a site will work as a daily-digest
// source before you add it to sources.json. It runs the crawler's own robots
// check, fetcher and parser on the seed page and a few of the articles it links
// to, and reports what the crawler would see: is robots.txt open, did the site
// serve real HTML (not a bot challenge), do the linked pages count as articles
// (date, text, image), and which path prefix to use. It sends a handful of
// requests, one a second.
//
//	go run ./cmd/checksource https://example.com/blog/
//	go run ./cmd/checksource -prefix /news/ -article-prefix /news/ https://example.com/news/
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/wanderingtilldeath/news-aggregator/internal/fetch"
	"github.com/wanderingtilldeath/news-aggregator/internal/frontier"
	"github.com/wanderingtilldeath/news-aggregator/internal/parse"
	"github.com/wanderingtilldeath/news-aggregator/internal/robots"
	"github.com/wanderingtilldeath/news-aggregator/internal/urlnorm"
)

func main() {
	var (
		prefixes        = flag.String("prefix", "", "comma-separated path prefixes of the article links to follow (default: suggest one)")
		articlePrefixes = flag.String("article-prefix", "", "comma-separated path prefixes that count as articles even without a date")
		samples         = flag.Int("n", 3, "how many linked pages to fetch and check")
		delay           = flag.Duration("delay", time.Second, "pause between requests")
		aiOnly          = flag.Bool("ai-policy", false, "only print each URL's robots.txt AI-crawler policy (takes one or more URLs)")
	)
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: checksource [flags] SEED_URL\n\nChecks whether SEED_URL and the articles it links to will work in the daily digest.")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *aiOnly {
		client := &http.Client{Timeout: 15 * time.Second}
		for _, seed := range flag.Args() {
			note := aiPolicy(context.Background(), client, seed)
			if note == "" {
				note = "none"
			}
			fmt.Printf("%s\t%s\n", seed, note)
			time.Sleep(*delay)
		}
		return
	}

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	r := check(ctx, flag.Arg(0), options{
		Prefixes:        splitList(*prefixes),
		ArticlePrefixes: splitList(*articlePrefixes),
		Samples:         *samples,
		Delay:           *delay,
	}, os.Stdout)

	if r.Verdict == verdictNo {
		os.Exit(1)
	}
}

type options struct {
	Prefixes, ArticlePrefixes []string
	Samples                   int
	Delay                     time.Duration
	// Fetcher, Robots and HTTP are replaced in tests.
	Fetcher fetch.Fetcher
	Robots  robots.Checker
	HTTP    *http.Client
}

type verdict string

const (
	verdictWorks   verdict = "WORKS"
	verdictPartial verdict = "PARTIAL"
	verdictNo      verdict = "NO"
)

// sample is what the crawler sees on one page.
type sample struct {
	URL       string
	Status    int
	Article   bool
	Date      string
	Runes     int
	Image     string
	SourceURL string
	Note      string
}

type report struct {
	Seed string
	// AINote summarises what robots.txt says about AI crawlers ("" = nothing).
	AINote      string
	Verdict     verdict
	Reasons     []string
	Prefixes    []string
	ArticleOnly bool
	Samples     []sample
}

var challengeRE = regexp.MustCompile(`(?i)just a moment|attention required|human verification|captcha|checking your browser|access denied|enable javascript and cookies|security checkpoint`)

func check(ctx context.Context, rawSeed string, o options, out interface{ Write([]byte) (int, error) }) report {
	say := func(format string, a ...any) { fmt.Fprintf(out, format+"\n", a...) }

	rep := report{Verdict: verdictNo}
	fail := func(reason string) report {
		rep.Reasons = append(rep.Reasons, reason)
		say("\nVERDICT: NO: %s", reason)
		return rep
	}

	seed, err := urlnorm.Canonicalize(rawSeed)
	if err != nil {
		return fail("the seed is not an absolute http(s) URL")
	}
	rep.Seed = seed
	say("Seed: %s", seed)

	if o.Fetcher == nil {
		o.Fetcher = fetch.New(fetch.Options{Timeout: 20 * time.Second, MaxBytes: 2 << 20})
	}
	if o.Robots == nil {
		o.Robots = robots.New(robots.Options{
			UserAgent: fetch.DefaultUserAgent, Product: "CrawlerX", HTTP: &http.Client{Timeout: 15 * time.Second},
		})
	}
	if o.Samples < 1 {
		o.Samples = 3
	}

	// robots.txt
	d, err := o.Robots.Check(ctx, seed)
	switch {
	case err != nil:
		return fail("robots.txt could not be read: " + err.Error())
	case !d.Allowed:
		return fail("robots.txt disallows the seed for this crawler")
	}
	if d.CrawlDelay > 0 {
		say("robots.txt: allowed, Crawl-delay %s (the crawler honours it)", d.CrawlDelay)
	} else {
		say("robots.txt: allowed")
	}

	// Not a blocker (our user agent is not named), but the digest summarises
	// with an LLM, so say what the publisher has signalled.
	if o.HTTP == nil {
		o.HTTP = &http.Client{Timeout: 15 * time.Second}
	}
	if note := aiPolicy(ctx, o.HTTP, seed); note != "" {
		rep.AINote = note
		say("AI policy in robots.txt: %s", note)
	}

	// seed page
	page, final, why := fetchPage(ctx, o, seed, say)
	if why != "" {
		return fail(why)
	}
	if final != seed {
		rep.Reasons = append(rep.Reasons, "the seed redirects to "+final+": use that as the seed (the crawler does not follow a redirect from the seed itself)")
		say("note: the seed redirects to %s: use that as the seed", final)
	}

	// links
	host := hostOf(final)
	var links []string
	for _, l := range page.Links {
		if u, err := url.Parse(l); err == nil && sameSite(hostOf(l), host) && u.Fragment == "" {
			links = append(links, l)
		}
	}
	say("%d same-site links on the seed page", len(links))
	if len(links) == 0 {
		return fail("the seed page has no links in its HTML (it may be built with JavaScript)")
	}

	say("most common first path segments: %s", histogram(links, 6))

	prefixes := o.Prefixes
	if len(prefixes) == 0 {
		prefixes = suggestPrefixes(links)
		if len(prefixes) == 0 {
			return fail("no group of article-looking links found; pass -prefix")
		}
		say("suggested path prefixes: %s", strings.Join(prefixes, ", "))
	}
	rep.Prefixes = prefixes
	rep.ArticleOnly = len(o.ArticlePrefixes) > 0

	var candidates []string
	for _, l := range links {
		if hasPrefix(l, prefixes) && !samePage(l, final) {
			candidates = append(candidates, l)
		}
	}
	candidates = dedupe(candidates)
	say("%d links match the prefix", len(candidates))
	if len(candidates) == 0 {
		return fail("no links match the path prefix " + strings.Join(prefixes, ", "))
	}

	// Sample real articles where we can tell them from index pages: a section
	// page like /news/research/ says nothing about the articles under it.
	pool := candidates
	if like := filter(candidates, func(l string) bool { u, _ := url.Parse(l); return u != nil && articleLike(u.Path) }); len(like) >= o.Samples {
		pool = like
	}
	say("sampling %d of %d article-like links", min(o.Samples, len(pool)), len(pool))

	for _, l := range pick(pool, o.Samples) {
		time.Sleep(o.Delay)
		s := sample{URL: l}

		if d, err := o.Robots.Check(ctx, l); err == nil && !d.Allowed {
			s.Note = "robots.txt disallows this page"
			rep.Samples = append(rep.Samples, s)
			continue
		}

		p, _, why := fetchPage(ctx, o, l, func(string, ...any) {})
		if why != "" {
			s.Note = why
			rep.Samples = append(rep.Samples, s)
			continue
		}

		s.Status = 200
		s.Article = p.IsArticle
		if len(o.ArticlePrefixes) > 0 {
			s.Article = hasPrefix(l, o.ArticlePrefixes) && !samePage(l, final)
		}
		if !p.Published.IsZero() {
			s.Date = p.Published.Format("2006-01-02")
		}
		s.Runes = utf8.RuneCountInString(p.Content)
		s.Image, s.SourceURL = p.Image, p.SourceURL
		rep.Samples = append(rep.Samples, s)
	}

	say("")
	for _, s := range rep.Samples {
		if s.Note != "" {
			say("  x %s\n      %s", short(s.URL, 90), s.Note)
			continue
		}
		say("  %s %s\n      article=%v date=%s text=%d chars image=%s", mark(s), short(s.URL, 90), s.Article, orDash(s.Date), s.Runes, orDash(short(s.Image, 70)))
	}

	rep.Verdict, rep.Reasons = judge(rep.Samples, len(o.ArticlePrefixes) > 0)
	if rep.AINote != "" {
		rep.Reasons = append(rep.Reasons, "robots.txt AI policy: "+rep.AINote+": the digest summarises with an LLM, so decide whether you are comfortable")
	}
	say("\nVERDICT: %s", rep.Verdict)
	for _, r := range rep.Reasons {
		say("  - %s", r)
	}

	if rep.Verdict != verdictNo {
		say("\nsources.json entry:\n%s", suggestEntry(final, prefixes, o.ArticlePrefixes, rep.Samples))
	}

	return rep
}

// fetchPage fetches rawURL (following up to 3 redirects by hand, since the
// fetcher reports them) and parses it. why is non-empty when the page is
// unusable.
func fetchPage(ctx context.Context, o options, rawURL string, say func(string, ...any)) (page parse.Page, final, why string) {
	target := rawURL
	for hop := 0; hop <= 3; hop++ {
		res, err := o.Fetcher.Fetch(ctx, fetch.Request{URL: target})
		if err != nil {
			return page, target, "fetch failed: " + err.Error()
		}

		switch {
		case res.Status >= 300 && res.Status < 400 && res.Location != "":
			target = res.Location
			continue
		case res.Status == 403 || res.Status == 405 || res.Status == 429 || res.Status == 503:
			return page, target, fmt.Sprintf("HTTP %d: blocked or rate limited (a bot challenge, not bypassed)", res.Status)
		case res.Status != 200:
			return page, target, fmt.Sprintf("HTTP %d", res.Status)
		}

		if head := string(res.Body[:min(len(res.Body), 6000)]); challengeRE.MatchString(head) && len(res.Body) < 60000 {
			return page, target, "the site served a bot-challenge page, not content"
		}

		page, err = parse.HTMLParser{}.Parse(strings.NewReader(string(res.Body)), target)
		if err != nil {
			return page, target, "parse failed: " + err.Error()
		}
		say("fetched %s (%d KB)", short(target, 80), len(res.Body)/1024)
		return page, target, ""
	}

	return page, target, "too many redirects"
}

// judge turns the samples into a verdict with plain-language reasons.
func judge(samples []sample, strict bool) (verdict, []string) {
	var ok, articles, text, images int
	imgCount := map[string]int{}
	for _, s := range samples {
		if s.Note != "" {
			continue
		}
		ok++
		if s.Article {
			articles++
		}
		if s.Runes >= 300 {
			text++
		}
		if s.Image != "" {
			images++
			imgCount[s.Image]++
		}
	}

	if ok == 0 {
		return verdictNo, []string{"none of the sampled pages could be fetched"}
	}

	need := (ok*2 + 2) / 3 // two thirds, rounded up
	var reasons []string
	verdictSoFar := verdictWorks

	if articles < need {
		verdictSoFar = verdictPartial
		if strict {
			reasons = append(reasons, "the -article-prefix did not match most sampled pages: check the prefix")
		} else {
			reasons = append(reasons, "most pages carry no machine-readable date, so they would not count as articles: add article_prefixes")
		}
	}
	if text < need {
		verdictSoFar = verdictPartial
		reasons = append(reasons, "little article text in the HTML (under 300 characters): the summary would be thin, the page may need JavaScript")
	}

	switch {
	case images == 0:
		reasons = append(reasons, "no page has an image: cards will show a placeholder")
	case images < need:
		reasons = append(reasons, fmt.Sprintf("only %d of %d pages have an image", images, ok))
	default:
		for img, n := range imgCount {
			if n == ok && ok > 1 {
				reasons = append(reasons, "every page has the same image ("+short(img, 60)+"): a site logo, so it will be hidden")
			}
		}
	}
	if verdictSoFar == verdictWorks && len(reasons) == 0 {
		reasons = append(reasons, fmt.Sprintf("%d of %d pages are articles with text", articles, ok))
	}

	return verdictSoFar, reasons
}

func suggestEntry(seed string, prefixes, articlePrefixes []string, samples []sample) string {
	// A struct, not a map, so the keys come out in the order people read them.
	entry := struct {
		Name            string   `json:"name"`
		Seed            string   `json:"seed"`
		MaxDepth        int      `json:"max_depth"`
		PathPrefixes    []string `json:"path_prefixes"`
		ArticlePrefixes []string `json:"article_prefixes,omitempty"`
	}{"TODO name: " + hostOf(seed), seed, 1, prefixes, articlePrefixes}

	b, _ := json.MarshalIndent(entry, "", "  ")
	return string(b)
}

// ---- robots.txt AI policy ----

// aiBots are the crawler names publishers use to opt out of AI use.
var aiBots = []string{"gptbot", "claudebot", "anthropic-ai", "claude-web", "ccbot", "google-extended", "perplexitybot", "cohere-ai", "bytespider", "amazonbot", "applebot-extended", "meta-externalagent"}

// aiPolicy reads the host's robots.txt and reports AI-related signals: named AI
// crawlers that are disallowed from the whole site, and Content-Signal lines.
func aiPolicy(ctx context.Context, client *http.Client, seed string) string {
	u, err := url.Parse(seed)
	if err != nil {
		return ""
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.Scheme+"://"+u.Host+"/robots.txt", nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", fetch.DefaultUserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}

	buf := make([]byte, 512<<10)
	n, _ := resp.Body.Read(buf)
	// Read may return less than everything; keep reading to the cap.
	for n < len(buf) {
		m, err := resp.Body.Read(buf[n:])
		n += m
		if err != nil {
			break
		}
	}

	return aiPolicyOf(string(buf[:n]))
}

// aiPolicyOf parses robots.txt text. It is deliberately simple: groups of
// User-agent lines followed by rules; a group "blocks" a bot if it lists it and
// disallows "/".
func aiPolicyOf(txt string) string {
	var (
		agents  []string
		inRules bool
		blocked = map[string]bool{}
		signals []string
	)

	for _, line := range strings.Split(txt, "\n") {
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key, val = strings.ToLower(strings.TrimSpace(key)), strings.TrimSpace(val)

		switch key {
		case "user-agent":
			if inRules {
				agents, inRules = nil, false
			}
			agents = append(agents, strings.ToLower(val))
		case "disallow":
			inRules = true
			if val == "/" {
				for _, a := range agents {
					for _, bot := range aiBots {
						if a == bot {
							blocked[bot] = true
						}
					}
				}
			}
		case "allow", "crawl-delay", "sitemap":
			inRules = key != "sitemap"
		case "content-signal":
			signals = append(signals, val)
		}
	}

	var parts []string
	if len(blocked) > 0 {
		var names []string
		for b := range blocked {
			names = append(names, b)
		}
		sort.Strings(names)
		parts = append(parts, "disallows "+strings.Join(names, ", ")+" from the whole site")
	}
	if len(signals) > 0 {
		parts = append(parts, "Content-Signal: "+signals[0])
	}

	return strings.Join(parts, "; ")
}

// ---- link helpers ----

var dated = regexp.MustCompile(`/(20\d{2})/(\d{2})/|/\d{2}-\d{2}-20\d{2}`)

// suggestPrefixes finds the groups of article-looking links (a long hyphenated
// slug, or a date in the path) and returns the path prefixes that hold them:
// "/blog/", "/2026/", or "/" when slugs sit at the site root. Besides the
// biggest group it keeps others at least a third as large, up to four, since
// many sites spread articles over a few sections.
func suggestPrefixes(links []string) []string {
	groups := map[string]int{}
	for _, l := range links {
		u, err := url.Parse(l)
		if err != nil || !articleLike(u.Path) {
			continue
		}
		segs := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(segs) == 1 {
			groups["/"]++
			continue
		}
		groups["/"+segs[0]+"/"]++
	}

	type kv struct {
		k string
		v int
	}
	var all []kv
	for k, v := range groups {
		all = append(all, kv{k, v})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].v != all[j].v {
			return all[i].v > all[j].v
		}
		return all[i].k < all[j].k
	})

	if len(all) == 0 || all[0].v < 3 {
		return nil
	}
	// Root-level slugs mean the site cannot be narrowed by section.
	if all[0].k == "/" {
		return []string{"/"}
	}

	var out []string
	for _, e := range all {
		if len(out) == 4 || e.k == "/" || e.v < 3 || e.v*3 < all[0].v {
			continue
		}
		out = append(out, e.k)
	}
	return out
}

func filter(in []string, keep func(string) bool) []string {
	var out []string
	for _, s := range in {
		if keep(s) {
			out = append(out, s)
		}
	}
	return out
}

func articleLike(path string) bool {
	if dated.MatchString(path) {
		return true
	}
	last := path[strings.LastIndex(strings.TrimRight(path, "/"), "/")+1:]
	last = strings.Trim(last, "/")
	return strings.Count(last, "-") >= 3 || len(last) >= 30
}

func histogram(links []string, top int) string {
	counts := map[string]int{}
	for _, l := range links {
		if u, err := url.Parse(l); err == nil {
			seg := strings.SplitN(strings.Trim(u.Path, "/"), "/", 2)[0]
			counts["/"+seg]++
		}
	}
	type kv struct {
		k string
		v int
	}
	var all []kv
	for k, v := range counts {
		all = append(all, kv{k, v})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].v != all[j].v {
			return all[i].v > all[j].v
		}
		return all[i].k < all[j].k
	})
	var parts []string
	for i, e := range all {
		if i >= top {
			break
		}
		parts = append(parts, fmt.Sprintf("%s (%d)", e.k, e.v))
	}
	return strings.Join(parts, ", ")
}

// pick returns up to n links spread across the list (not just the first few,
// which are often pinned or sponsored).
func pick(links []string, n int) []string {
	if len(links) <= n {
		return links
	}
	var out []string
	step := float64(len(links)) / float64(n)
	for i := 0; i < n; i++ {
		out = append(out, links[int(float64(i)*step)])
	}
	return out
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// hasPrefix applies the same rules as the crawler, including "!" exclusions.
func hasPrefix(rawURL string, prefixes []string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return frontier.PathMatches(u.Path, prefixes)
}

func samePage(a, b string) bool {
	return strings.TrimRight(a, "/") == strings.TrimRight(b, "/")
}

func hostOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
}

func sameSite(a, b string) bool {
	return a == b || strings.HasSuffix(a, "."+b) || strings.HasSuffix(b, "."+a)
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func short(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func mark(s sample) string {
	if s.Article && s.Runes >= 300 {
		return "✓"
	}
	return "~"
}
