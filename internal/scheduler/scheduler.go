package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/frontier"
	"github.com/wanderingtilldeath/news-aggregator/internal/images"
	"github.com/wanderingtilldeath/news-aggregator/internal/store"
	"github.com/wanderingtilldeath/news-aggregator/internal/summarize"
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
		}
	}

	// Enrich while crawling: whenever a full batch of new articles is waiting
	// the pass fires, so the first cards appear minutes into the run instead of
	// after the last page. The final pass (below) takes whatever is left.
	// One blocklist for the whole run: a host that refuses us is asked twice, once.
	blocked := images.NewBlocklist()

	total := 0
	err := s.waitForCrawls(ctx, ids, func() {
		n, _ := s.enrich(ctx, started, s.cfg.SummaryBatch, blocked)
		total += n
	})
	if err != nil {
		log.Warn("not every crawl finished; summarising what we have", "error", err)
	}

	// The final pass. If the provider rate-limited us, wait out the limit and
	// finish the rest, rather than leave those articles without a gist.
	for attempt := 0; ; attempt++ {
		n, limited := s.enrich(ctx, started, 0, blocked)
		total += n
		if !limited || attempt >= s.cfg.RateLimitRetries || ctx.Err() != nil {
			break
		}
		log.Info("summaries rate limited; waiting before finishing the rest",
			"wait", s.cfg.RateLimitCooldown, "attempt", attempt+1, "of", s.cfg.RateLimitRetries)
		select {
		case <-time.After(s.cfg.RateLimitCooldown):
		case <-ctx.Done():
		}
	}

	attrs := []any{"crawls", len(ids), "summarised", total, "took", s.deps.Now().Sub(started).Round(time.Second)}
	if c, ok := s.deps.Summarizer.(interface{ Calls() int64 }); ok {
		attrs = append(attrs, "llm_calls", c.Calls())
	}
	log.Info("digest run complete", attrs...)
}

// enrich looks up source images and writes summaries for articles in the
// digest window, the two in parallel (they share no state). minPending > 0
// makes the summary pass wait until a full batch is ready. It returns how many
// summaries were stored and whether the provider rate-limited the pass.
func (s *Scheduler) enrich(ctx context.Context, started time.Time, minPending int, blocked *images.Blocklist) (int, bool) {
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

	n, err := summarize.Pass(ctx, s.deps.Store, s.deps.Summarizer, window, summarize.Options{
		Limit:       s.cfg.SummaryLimit,
		Workers:     s.cfg.SummaryWorkers,
		BatchSize:   s.cfg.SummaryBatch,
		MinLLMRunes: s.cfg.MinLLMRunes,
		MinPending:  minPending,
	}, log)
	limited := errors.Is(err, summarize.ErrRateLimited)
	if err != nil && !limited && ctx.Err() == nil {
		log.Error("summarise pass failed", "error", err)
	}
	if n > 0 {
		log.Info("summaries stored", "count", n)
	}

	wg.Wait()
	return n, limited
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
