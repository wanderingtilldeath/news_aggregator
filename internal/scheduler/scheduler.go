package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/compile"
	"github.com/wanderingtilldeath/news-aggregator/internal/frontier"
	"github.com/wanderingtilldeath/news-aggregator/internal/group"
	"github.com/wanderingtilldeath/news-aggregator/internal/images"
	"github.com/wanderingtilldeath/news-aggregator/internal/store"
	"github.com/wanderingtilldeath/news-aggregator/internal/summarize"
	"github.com/wanderingtilldeath/news-aggregator/internal/track"
)

const jobName = "digest"

// Claims decides which process runs the job on a given day.
type Claims interface {
	// Claim returns true for exactly one caller per (job, day).
	Claim(ctx context.Context, job string, day time.Time) (bool, error)
}

// Config is the schedule.
type Config struct {
	Sources []Source
	// At is the local time of day to run, "HH:MM".
	At       string
	Location *time.Location
	// RunNow runs once at startup regardless of the day's claim.
	RunNow bool
	// CrawlTimeout bounds the wait for crawls; summaries still run after it.
	CrawlTimeout time.Duration
	// LLMBudget bounds the model work of one run (summaries, enrichment, concepts) from the start of the
	// run (default 12 minutes). When the model is slow or answering 503, the run stops asking and the rest
	// stays pending for the next run, instead of waiting out retry after retry for half an hour. The quiz
	// and the intro have their own short allowance (oneCall), so a slow day still gets them.
	LLMBudget time.Duration
	// SummaryWindow limits which articles get summarised and given images: those
	// dated within it, which should cover the longest period the digest page
	// offers so no card shows up without its gist.
	SummaryWindow time.Duration
	SummaryLimit  int
	// SummaryWorkers is how many LLM requests are in flight at once, and
	// SummaryBatch how many articles share one request.
	SummaryWorkers int
	SummaryBatch   int
	// RateLimitRetries is how many times the final pass waits RateLimitCooldown
	// and tries again when the LLM provider says it is out of quota, so a
	// per-minute limit does not leave articles without a gist.
	RateLimitRetries  int
	RateLimitCooldown time.Duration
	// MaxRequests caps the bulk model requests (summaries and the first enrichment stage) of one run, retries
	// included; 0 means 220, below 0 no cap. The long summaries, concepts, quiz and intro are not counted.
	MaxRequests int
	// MinLLMRunes: shorter articles get the extractive gist without an LLM call.
	MinLLMRunes int
	// ImageDelay spaces the requests that fetch images from cited sources.
	ImageDelay time.Duration
	// Poll is how often crawl progress is checked.
	Poll time.Duration
}

// Deps are the scheduler's collaborators.
type Deps struct {
	Frontier   frontier.Frontier
	Store      store.Store
	Summarizer summarize.Summarizer
	// Enricher adds the why line, tags, importance and long summary; nil skips that step.
	Enricher summarize.Enricher
	// Quizzer writes the daily quiz; nil skips that step.
	Quizzer summarize.Quizzer
	// Introducer writes the "Today in 60 seconds" paragraph; nil skips that step.
	Introducer summarize.Introducer
	// Explainer writes the glossary explainers of the concepts the stories name; nil skips that step.
	Explainer summarize.Explainer
	// Images fetches lead images from cited sources; nil skips that step.
	Images images.Resolver
	Claims Claims
	Logger *slog.Logger
	// Now is time.Now unless a test replaces it.
	Now func() time.Time
}

// Scheduler runs the daily job.
type Scheduler struct {
	cfg  Config
	deps Deps
	hour int
	min  int
}

// New validates cfg. It fills defaults for the zero values of the tuning
// fields.
func New(cfg Config, deps Deps) (*Scheduler, error) {
	var hour, min int
	if _, err := fmt.Sscanf(cfg.At, "%d:%d", &hour, &min); err != nil || hour < 0 || hour > 23 || min < 0 || min > 59 {
		return nil, fmt.Errorf("scheduler: time %q must be HH:MM", cfg.At)
	}
	if len(cfg.Sources) == 0 {
		return nil, errors.New("scheduler: no sources")
	}
	if cfg.Location == nil {
		cfg.Location = time.UTC
	}
	if cfg.CrawlTimeout <= 0 {
		cfg.CrawlTimeout = 2 * time.Hour
	}
	if cfg.LLMBudget <= 0 {
		cfg.LLMBudget = 12 * time.Minute
	}
	if cfg.SummaryWindow <= 0 {
		cfg.SummaryWindow = 7 * 24 * time.Hour
	}
	if cfg.SummaryBatch <= 0 {
		cfg.SummaryBatch = 6
	}
	if cfg.RateLimitRetries == 0 {
		cfg.RateLimitRetries = 3
	}
	if cfg.RateLimitCooldown <= 0 {
		cfg.RateLimitCooldown = 70 * time.Second
	}
	if cfg.MaxRequests == 0 {
		cfg.MaxRequests = 220
	}
	if cfg.MinLLMRunes <= 0 {
		cfg.MinLLMRunes = 400
	}
	if cfg.SummaryLimit <= 0 {
		cfg.SummaryLimit = 300
	}
	if cfg.ImageDelay <= 0 {
		cfg.ImageDelay = time.Second
	}
	if cfg.SummaryWorkers <= 0 {
		cfg.SummaryWorkers = 3
	}
	if cfg.Poll <= 0 {
		cfg.Poll = 5 * time.Second
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}

	return &Scheduler{cfg: cfg, deps: deps, hour: hour, min: min}, nil
}

// nextRun is the first scheduled time strictly after now.
func (s *Scheduler) nextRun(now time.Time) time.Time {
	now = now.In(s.cfg.Location)
	t := time.Date(now.Year(), now.Month(), now.Day(), s.hour, s.min, 0, 0, s.cfg.Location)
	if !t.After(now) {
		t = t.AddDate(0, 0, 1)
	}
	return t
}

// Run blocks until ctx ends. On start it runs immediately if RunNow is set,
// or catches up when today's time has already passed without a run; after
// that it runs once a day at the configured time.
func (s *Scheduler) Run(ctx context.Context) {
	log := s.deps.Logger

	if s.cfg.RunNow {
		log.Info("digest run forced at startup")
		s.runOnce(ctx)
	} else {
		now := s.deps.Now().In(s.cfg.Location)
		if today := time.Date(now.Year(), now.Month(), now.Day(), s.hour, s.min, 0, 0, s.cfg.Location); !now.Before(today) {
			s.runIfClaimed(ctx, "catch-up")
		}
	}

	for {
		next := s.nextRun(s.deps.Now())
		log.Info("next digest run", "at", next)

		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}

		s.runIfClaimed(ctx, "scheduled")
	}
}

func (s *Scheduler) runIfClaimed(ctx context.Context, why string) {
	day := s.deps.Now().In(s.cfg.Location)

	ok, err := s.deps.Claims.Claim(ctx, jobName, day)
	if err != nil {
		s.deps.Logger.Error("digest claim failed", "error", err)
		return
	}
	if !ok {
		s.deps.Logger.Info("digest already ran today", "why", why)
		return
	}

	s.runOnce(ctx)
}

// runOnce starts a crawl per source, waits for them, then summarises.
func (s *Scheduler) runOnce(ctx context.Context) {
	log := s.deps.Logger
	started := s.deps.Now()

	var ids []int64
	var runs []sourceCrawl
	for _, src := range s.cfg.Sources {
		specs, err := src.Specs(started.In(s.cfg.Location))
		if err != nil {
			log.Error("bad source", "source", src.Name, "error", err)
			continue
		}
		for _, spec := range specs {
			crawl, err := frontier.StartCrawlSpec(ctx, s.deps.Frontier, spec)
			if err != nil {
				log.Error("could not start crawl", "source", src.Name, "error", err)
				continue
			}
			log.Info("digest crawl started", "source", src.Name, "crawl", crawl.ID, "seed", spec.Seed)
			ids = append(ids, crawl.ID)
			runs = append(runs, sourceCrawl{name: src.Name, host: seedHost(spec.Seed), id: crawl.ID})
		}
	}

	// Enrich while crawling: whenever a full batch of new articles is waiting
	// the pass fires, so the first cards appear minutes into the run instead of
	// after the last page. The final pass (below) takes whatever is left.
	// One blocklist for the whole run: a host that refuses us is asked twice, once.
	blocked := images.NewBlocklist()

	// The model work of the whole run shares one budget (see Config.LLMBudget).
	llm, cancelLLM := context.WithTimeout(ctx, s.cfg.LLMBudget)
	defer cancelLLM()

	var calls func() int64
	if c, ok := s.deps.Summarizer.(interface{ Calls() int64 }); ok {
		calls = c.Calls
	}
	q := newQuota(s.deps.Now, s.cfg.RateLimitCooldown, int64(max(s.cfg.MaxRequests, 0)), calls)

	total := 0
	err := s.waitForCrawls(ctx, ids, func() {
		n, _ := s.enrich(ctx, llm, started, s.cfg.SummaryBatch, blocked, q)
		total += n
	})
	if err != nil {
		log.Warn("not every crawl finished; summarising what we have", "error", err)
	}
	s.recordSourceRuns(ctx, started, runs)

	// The final pass. If the provider rate-limited us, wait out the limit and
	// finish the rest, rather than leave those articles without a gist.
	for attempt := 0; ; attempt++ {
		// A limit hit during the crawl (or by the last try) paused the model: wait that out first.
		if w := q.wait(); w > 0 && !q.spent() {
			log.Info("model paused after a rate limit; waiting before finishing the rest", "wait", w.Round(time.Second), "attempt", attempt+1)
			select {
			case <-time.After(w):
			case <-llm.Done():
			}
		}
		n, limited := s.enrich(ctx, llm, started, 0, blocked, q)
		total += n
		if !limited || attempt >= s.cfg.RateLimitRetries || llm.Err() != nil || q.spent() {
			break
		}
	}
	if q.spent() {
		log.Warn("model quota reached; the rest of this run uses no model and what is left waits for the next run")
	}

	s.groupStories(ctx, started)
	enriched := s.enrichArticles(llm, started, q)
	s.explainConcepts(llm, q)
	// The quiz and the intro get a short allowance of their own, but not when the quota is gone: they would
	// only wait through their retries and fail.
	if !q.spent() {
		s.makeQuizzes(ctx, started)
		s.makeIntros(ctx, started)
	}
	s.compileMonth(ctx, started)
	if errors.Is(llm.Err(), context.DeadlineExceeded) {
		log.Warn("model time budget used; what is left waits for the next run", "budget", s.cfg.LLMBudget)
	}

	s.logImageCoverage(ctx, started)

	attrs := []any{"crawls", len(ids), "summarised", total, "enriched", enriched, "took", s.deps.Now().Sub(started).Round(time.Second)}
	if c, ok := s.deps.Summarizer.(interface{ Calls() int64 }); ok {
		attrs = append(attrs, "llm_calls", c.Calls())
	}
	if u, ok := s.deps.Summarizer.(interface{ Usage() summarize.Usage }); ok {
		usage := u.Usage()
		attrs = append(attrs, "llm_prompt_tokens", usage.Prompt, "llm_output_tokens", usage.Output, "llm_thought_tokens", usage.Thoughts)
	}
	log.Info("digest run complete", attrs...)
}

// logImageCoverage logs how many recent articles show a picture and why the rest do not, so a
// run says whether pictures are being lost (shared logos) or never found. It is only a log.
func (s *Scheduler) logImageCoverage(ctx context.Context, started time.Time) {
	c, err := s.deps.Store.ImageStats(ctx, started.Add(-s.cfg.SummaryWindow))
	if err != nil {
		if ctx.Err() == nil {
			s.deps.Logger.Warn("image coverage failed", "error", err)
		}
		return
	}
	s.deps.Logger.Info("image coverage", "shown", c.Shown, "logo_only", c.Logo, "none", c.None)
}

// Stories are grouped over the last month, newest articles first and capped, so one run compares a
// bounded number of headlines. Only stories within 72 hours of each other are compared, so a story
// that develops over days is linked end to end (the story page shows it as a timeline).
const (
	groupWindow = 30 * 24 * time.Hour
	groupLimit  = 5000
)

// groupStories finds the stories several sources cover and records the grouping. It uses no model
// and no network. A failure is logged and never stops the run.
func (s *Scheduler) groupStories(ctx context.Context, started time.Time) {
	since := started.Add(-groupWindow)
	cands, err := s.deps.Store.GroupCandidates(ctx, since, groupLimit)
	if err == nil {
		stories := make([]group.Story, len(cands))
		for i, c := range cands {
			stories[i] = group.Story{ID: c.ID, Host: c.Source, Time: c.Time, Title: c.Title}
		}
		groups := map[string]store.StoryGroup{}
		for id, g := range group.Cluster(stories) {
			groups[id] = store.StoryGroup{ID: g.Story, Sources: g.Sources}
		}
		if err = s.deps.Store.SetStories(ctx, since, groups); err == nil {
			s.deps.Logger.Info("stories grouped", "articles", len(cands), "grouped", len(groups))
		}
	}
	if err != nil && ctx.Err() == nil {
		s.deps.Logger.Error("story grouping failed", "error", err)
	}
}

// Enrichment is limited to recent articles and a fixed number per run, so a first
// run on a big backlog stays inside the provider's free quota; the next runs continue.
// The first stage (the why-it-matters line, tags, importance) reaches back as far as the pages the site shows by
// default (a week), newest first, so a story that missed its first run, or that was crawled before enrichment
// existed, still gets its line on a later one; it was 48 hours, which left every older story without one for good.
// The second stage (long summary, study cards) stays at 48 hours: it picks the most important stories, and a week's
// worth would crowd the day's own out.
const (
	enrichWindow = 7 * 24 * time.Hour
	enrichLimit  = 100
	detailWindow = 48 * time.Hour
	// detailLimit is how many of the most important stories get the long summary and study cards per run
	// (three per request). Exam categories go first, so the flashcard deck fills from the UPSC stories.
	detailLimit = 9
)

// enrichArticles runs the enrichment pass once, after the summaries are done. A
// failure is logged and never stops the run.
func (s *Scheduler) enrichArticles(ctx context.Context, started time.Time, q *quota) int {
	if s.deps.Enricher == nil {
		return 0
	}
	since := started.Add(-enrichWindow)
	n := 0
	var err error
	if s.modelAllowed(q, true) {
		n, err = summarize.EnrichPass(ctx, s.deps.Store, s.deps.Enricher, since, summarize.EnrichOptions{Limit: q.limit(enrichLimit, summarize.BasicBatch)}, s.deps.Logger)
		if errors.Is(err, summarize.ErrRateLimited) {
			q.strike()
		}
		if err != nil && ctx.Err() == nil && !errors.Is(err, summarize.ErrRateLimited) {
			s.deps.Logger.Error("enrichment pass failed", "error", err)
		}
	}
	// Stage 2 spends the longer replies on the few most important stories. They are exempt from the request
	// cap: about nine articles a run, the stories the flashcards are made from.
	if !errors.Is(err, summarize.ErrRateLimited) && s.modelAllowed(q, false) {
		d, derr := summarize.DetailPass(ctx, s.deps.Store, s.deps.Enricher, started.Add(-detailWindow), summarize.EnrichOptions{Limit: detailLimit}, s.deps.Logger)
		if errors.Is(derr, summarize.ErrRateLimited) {
			q.strike()
		}
		if derr != nil && ctx.Err() == nil && !errors.Is(derr, summarize.ErrRateLimited) {
			s.deps.Logger.Error("detail pass failed", "error", derr)
		}
		if d > 0 {
			s.deps.Logger.Info("long summaries and study cards written", "count", d)
		}
	}

	return n
}

// makeQuizzes writes today's quiz for each track that has none, from the last
// day's stories. A failure is logged and never stops the run.
func (s *Scheduler) makeQuizzes(ctx context.Context, started time.Time) {
	if s.deps.Quizzer == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, oneCall)
	defer cancel()
	day := started.In(s.cfg.Location).Format("2006-01-02")
	n, err := summarize.QuizPass(ctx, s.deps.Store, s.deps.Quizzer, started.Add(-24*time.Hour), day, s.deps.Logger)
	if err != nil && ctx.Err() == nil && !errors.Is(err, summarize.ErrRateLimited) {
		s.deps.Logger.Error("quiz pass failed", "error", err)
	}
	if n > 0 {
		s.deps.Logger.Info("quizzes written", "count", n, "day", day)
	}
}

// explainConcepts writes the explainers of the concepts the stories named, in one request. A
// failure is logged and never stops the run.
func (s *Scheduler) explainConcepts(ctx context.Context, q *quota) {
	if s.deps.Explainer == nil || !s.modelAllowed(q, false) {
		return
	}
	n, err := summarize.ConceptPass(ctx, s.deps.Store, s.deps.Explainer, conceptsPerRun, s.deps.Logger)
	if err != nil && ctx.Err() == nil && !errors.Is(err, summarize.ErrRateLimited) {
		s.deps.Logger.Error("concept pass failed", "error", err)
	}
	if n > 0 {
		s.deps.Logger.Info("concepts explained", "count", n)
	}
}

// conceptsPerRun bounds the glossary explainers one run asks for.
const conceptsPerRun = 12

// oneCall is the time the quiz and the intro each get, outside the run's model budget: they are one
// request each and the readers of the day's edition expect them.
const oneCall = 2 * time.Minute

// makeIntros writes today's intro for each track that has none, from the last day's headlines. A
// failure is logged and never stops the run.
func (s *Scheduler) makeIntros(ctx context.Context, started time.Time) {
	if s.deps.Introducer == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, oneCall)
	defer cancel()
	day := started.In(s.cfg.Location).Format("2006-01-02")
	n, err := summarize.IntroPass(ctx, s.deps.Store, s.deps.Introducer, started.Add(-24*time.Hour), day, s.deps.Logger)
	if err != nil && ctx.Err() == nil && !errors.Is(err, summarize.ErrRateLimited) {
		s.deps.Logger.Error("intro pass failed", "error", err)
	}
	if n > 0 {
		s.deps.Logger.Info("intros written", "count", n, "day", day)
	}
}

// compileWindow is how far back a day's run looks for stories to add to the month's compilation.
// It is longer than a day so that a story a run missed is still picked up by the next one.
const compileWindow = 36 * time.Hour

// compileMonth adds the recent important stories of each edition to this month's compilation (see
// package compile). It reads only stories already stored and uses no model. A failure is logged and
// never stops the run.
func (s *Scheduler) compileMonth(ctx context.Context, started time.Time) {
	month := started.In(s.cfg.Location).Format("2006-01")
	since := started.Add(-compileWindow)

	for _, tr := range track.All() {
		var items []store.Item
		for _, cat := range track.Categories(tr) {
			for offset := 0; offset < 2000; offset += 200 {
				got, total, err := s.deps.Store.Digest(ctx, store.DigestQuery{Since: since, ArticlesOnly: true, Category: cat, Limit: 200, Offset: offset})
				if err != nil {
					s.deps.Logger.Error("compilation: load stories failed", "track", tr, "error", err)
					return
				}
				items = append(items, got...)
				if offset+200 >= total {
					break
				}
			}
		}

		existing, _, err := s.deps.Store.GetCompilation(ctx, month, tr)
		if err != nil {
			s.deps.Logger.Error("compilation: load failed", "track", tr, "error", err)
			return
		}
		areas := compile.Merge(existing.Areas, items, tr, compile.PerArea)
		if len(areas) == 0 {
			continue
		}
		if err := s.deps.Store.SaveCompilation(ctx, store.Compilation{Month: month, Track: tr, Areas: areas}); err != nil {
			s.deps.Logger.Error("compilation: save failed", "track", tr, "error", err)
			return
		}
	}
	s.deps.Logger.Info("compilation updated", "month", month)
}

// enrich looks up source images (under ctx) and writes summaries (under llm, the run's model budget)
// for articles in the digest window, the two in parallel (they share no state). minPending > 0
// makes the summary pass wait until a full batch is ready. It returns how many
// summaries were stored and whether the provider rate-limited the pass.
func (s *Scheduler) enrich(ctx, llm context.Context, started time.Time, minPending int, blocked *images.Blocklist, q *quota) (int, bool) {
	log := s.deps.Logger
	window := started.Add(-s.cfg.SummaryWindow)

	var wg sync.WaitGroup
	if s.deps.Images != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := images.Pass(ctx, s.deps.Store, s.deps.Images, window, s.cfg.SummaryLimit, s.cfg.ImageDelay, blocked, log)
			if err != nil && ctx.Err() == nil {
				log.Warn("image pass stopped early", "error", err)
			}
			if got > 0 {
				log.Info("source images fetched", "found", got)
			}
		}()
	}

	// While the model is paused, out of quota or over the request cap, the images above still run and the
	// summaries wait.
	if !s.modelAllowed(q, true) {
		wg.Wait()
		return 0, false
	}
	n, err := summarize.Pass(llm, s.deps.Store, s.deps.Summarizer, window, summarize.Options{
		// What is left of the request cap bounds how many articles this pass may take (one request per batch).
		Limit:       q.limit(s.cfg.SummaryLimit, s.cfg.SummaryBatch),
		Workers:     s.cfg.SummaryWorkers,
		BatchSize:   s.cfg.SummaryBatch,
		MinLLMRunes: s.cfg.MinLLMRunes,
		MinPending:  minPending,
	}, log)
	limited := errors.Is(err, summarize.ErrRateLimited)
	if limited {
		q.strike()
	} else if err == nil && n > 0 {
		// Only a pass that stored something shows the limit has lifted; an idle one proves nothing.
		q.ok()
	}
	if err != nil && !limited && llm.Err() == nil && !errors.Is(err, summarize.ErrCredentials) {
		log.Error("summarise pass failed", "error", err)
	}
	if n > 0 {
		log.Info("summaries stored", "count", n)
	}

	wg.Wait()
	return n, limited
}

// modelAllowed reports whether a model pass may start now (see quota), logging once when the request cap is
// what stops it. bulk passes are the ones the cap applies to.
func (s *Scheduler) modelAllowed(q *quota, bulk bool) bool {
	if !q.allow(bulk) {
		if reached, first := q.overCap(); bulk && reached && first {
			s.deps.Logger.Warn("model request cap reached; the rest waits for the next run", "cap", s.cfg.MaxRequests)
		}
		return false
	}

	return true
}

// waitForCrawls polls until every crawl has no pending or leased work.
func (s *Scheduler) waitForCrawls(ctx context.Context, ids []int64, onTick func()) error {
	if len(ids) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, s.cfg.CrawlTimeout)
	defer cancel()

	ticker := time.NewTicker(s.cfg.Poll)
	defer ticker.Stop()

	remaining := ids
	for {
		var still []int64
		for _, id := range remaining {
			stats, err := s.deps.Frontier.Stats(ctx, id)
			if err != nil || stats.Active() > 0 {
				still = append(still, id)
			}
		}
		if remaining = still; len(remaining) == 0 {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("%d crawl(s) still running: %w", len(remaining), ctx.Err())
		case <-ticker.C:
		}

		if onTick != nil {
			onTick()
		}
	}
}

// sourceCrawl ties a crawl to the source (sources.json entry) that started it.
type sourceCrawl struct {
	name, host string
	id         int64
}

func seedHost(seed string) string {
	u, err := url.Parse(seed)
	if err != nil {
		return ""
	}

	return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
}

// recordSourceRuns stores how each source's crawl(s) went: pages fetched, articles found, failures, and an outcome. A
// source with several crawls in a run is one row. A failure is logged and never stops the run.
func (s *Scheduler) recordSourceRuns(ctx context.Context, at time.Time, crawls []sourceCrawl) {
	if len(crawls) == 0 {
		return
	}
	ids := make([]int64, len(crawls))
	for i, c := range crawls {
		ids[i] = c.id
	}
	articles, err := s.deps.Store.ArticlesByCrawl(ctx, ids)
	if err != nil {
		s.deps.Logger.Warn("source health: could not count articles", "error", err)
		return
	}

	type sum struct {
		store.SourceRun
		running bool
	}
	order := []string{}
	by := map[string]*sum{}
	for _, c := range crawls {
		stats, err := s.deps.Frontier.Stats(ctx, c.id)
		if err != nil {
			s.deps.Logger.Warn("source health: could not read crawl stats", "source", c.name, "error", err)
			continue
		}
		r := by[c.name]
		if r == nil {
			r = &sum{SourceRun: store.SourceRun{Source: c.name, Host: c.host, At: at}}
			by[c.name] = r
			order = append(order, c.name)
		}
		r.Pages += int(stats[frontier.Done])
		r.Failed += int(stats[frontier.Failed] + stats[frontier.Dead])
		r.Articles += articles[c.id]
		r.running = r.running || stats.Active() > 0
	}

	out := make([]store.SourceRun, 0, len(order))
	for _, name := range order {
		r := by[name]
		switch {
		case r.running:
			r.Outcome = "timeout"
		case r.Pages == 0:
			r.Outcome = "failed"
		case r.Articles == 0:
			r.Outcome = "empty"
		default:
			r.Outcome = "ok"
		}
		if r.Outcome != "ok" {
			s.deps.Logger.Warn("source health: no articles from this source", "source", name, "outcome", r.Outcome, "pages", r.Pages, "failed", r.Failed)
		}
		out = append(out, r.SourceRun)
	}
	if err := s.deps.Store.RecordSourceRuns(ctx, out); err != nil {
		s.deps.Logger.Warn("source health: could not record the runs", "error", err)
	}
}
