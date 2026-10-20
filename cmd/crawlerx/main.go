// Command crawlerx is a resumable, polite web crawler. Postgres is the work
// queue, and a killed process loses no work: its leased URLs are picked up on
// restart once the lease expires.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wanderingtilldeath/news-aggregator/internal/api"
	"github.com/wanderingtilldeath/news-aggregator/internal/config"
	"github.com/wanderingtilldeath/news-aggregator/internal/crawler"
	"github.com/wanderingtilldeath/news-aggregator/internal/db"
	"github.com/wanderingtilldeath/news-aggregator/internal/fetch"
	"github.com/wanderingtilldeath/news-aggregator/internal/frontier"
	"github.com/wanderingtilldeath/news-aggregator/internal/images"
	"github.com/wanderingtilldeath/news-aggregator/internal/politeness"
	"github.com/wanderingtilldeath/news-aggregator/internal/robots"
	"github.com/wanderingtilldeath/news-aggregator/internal/scheduler"
	"github.com/wanderingtilldeath/news-aggregator/internal/store"
	"github.com/wanderingtilldeath/news-aggregator/internal/summarize"
	"github.com/wanderingtilldeath/news-aggregator/internal/urlnorm"
)

func main() {
	cfg, err := config.Load(os.Args[1:], os.Getenv, os.Stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "crawlerx:", err)
		os.Exit(2)
	}

	logger := newLogger(cfg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, stop, cfg, logger); err != nil {
		logger.Error("crawlerx failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, stop context.CancelFunc, cfg config.Config, logger *slog.Logger) error {
	pool, err := db.Open(ctx, cfg.DatabaseURL, cfg.Workers+8)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		return err
	}

	front := frontier.NewPostgres(pool)
	pages := store.NewPostgres(pool)

	checks := map[string]func(context.Context) error{"postgres": pool.Ping}

	// One process, so an in-process gate keeps every host to HostDelay.
	gate := politeness.NewMemory(nil)

	robotChecker := robots.New(robots.Options{
		UserAgent: cfg.UserAgent,
		Product:   userAgentProduct(cfg.UserAgent),
		HTTP:      &http.Client{Timeout: cfg.Timeout},
	})

	c := crawler.New(crawler.Config{
		Workers:       cfg.Workers,
		Lease:         cfg.Lease,
		HostDelay:     cfg.HostDelay,
		HostInflight:  cfg.HostInflight,
		MaxAttempts:   cfg.MaxAttempts,
		ShutdownGrace: cfg.ShutdownGrace,
		Conditional:   cfg.Conditional,
	}, crawler.Deps{
		Frontier: front,
		Fetcher:  fetch.New(fetch.Options{UserAgent: cfg.UserAgent, Timeout: cfg.Timeout, MaxBytes: cfg.MaxBytes}),
		Store:    pages,
		Gate:     gate,
		Robots:   robotChecker,
		Logger:   logger,
	})

	var servers []*http.Server

	if cfg.ListenAddr != "" {
		servers = append(servers, serve(logger, "api", cfg.ListenAddr, api.NewHandler(api.Deps{
			Frontier: front,
			Store:    pages,
			Images:   newImageProxy(pages, cfg),
			Checks:   checks,
			Logger:   logger,
		})))
	}
	if cfg.PprofAddr != "" {
		servers = append(servers, serve(logger, "pprof", cfg.PprofAddr, pprofHandler()))
	}

	if cfg.Seed != "" {
		crawl, err := startSeedCrawl(ctx, front, cfg)
		if err != nil {
			return err
		}
		logger.Info("crawl started", "id", crawl.ID, "seed", crawl.Seed, "max_depth", crawl.MaxDepth, "hosts", crawl.AllowedHosts)

		if cfg.ExitWhenDone {
			go exitWhenDone(ctx, stop, front, crawl.ID, logger)
		}
	}

	if cfg.SourcesFile != "" {
		sched, err := newScheduler(cfg, front, pages, pool, robotChecker, logger)
		if err != nil {
			return err
		}
		go sched.Run(ctx)
	}

	logger.Info("crawlerx running", "workers", cfg.Workers, "host_delay", cfg.HostDelay, "api", cfg.ListenAddr)

	if cfg.Workers > 0 {
		_ = c.Run(ctx)
	} else {
		<-ctx.Done()
	}

	// Shutdown order: workers are drained (their leases finished or
	// released), then pending page writes are flushed, then the servers stop.
	logger.Info("shutting down")

	if err := pages.Close(); err != nil {
		logger.Warn("closing store", "error", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, srv := range servers {
		_ = srv.Shutdown(shutdownCtx)
	}

	return nil
}

// newImageProxy builds the cached image proxy served at /img/{id}.
func newImageProxy(pages store.Store, cfg config.Config) *images.Proxy {
	p := images.NewProxy(pages, cfg.UserAgent, cfg.AllowPrivateSources)
	p.MaxCacheBytes = int64(cfg.ImageCacheMB) << 20
	return p
}

// newScheduler wires the daily digest job from the -sources file.
func newScheduler(cfg config.Config, front frontier.Frontier, pages store.Store, pool *pgxpool.Pool, rb robots.Checker, logger *slog.Logger) (*scheduler.Scheduler, error) {
	sources, err := scheduler.LoadSources(cfg.SourcesFile)
	if err != nil {
		return nil, err
	}
	if err := scheduler.CheckSiteLimit(sources, cfg.MaxSites); err != nil {
		return nil, err
	}
	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		return nil, fmt.Errorf("timezone: %w", err)
	}

	var sum summarize.Summarizer = summarize.Extractive{}
	if cfg.SummaryProvider == "gemini" || (cfg.SummaryProvider == "auto" && cfg.GeminiAPIKey != "") {
		sum = summarize.NewGemini(cfg.GeminiAPIKey, cfg.SummaryModel)
	}
	logger.Info("digest scheduler enabled", "sources", len(sources), "sites", len(scheduler.Sites(sources)), "at", cfg.DigestAt, "timezone", cfg.Timezone,
		"summaries", fmt.Sprintf("%T", sum))

	return scheduler.New(scheduler.Config{
		Sources:  sources,
		At:       cfg.DigestAt,
		Location: loc,
		RunNow:   cfg.RunOnStart,

		SummaryBatch:   cfg.SummaryBatch,
		SummaryWorkers: cfg.SummaryWorkers,
		SummaryWindow:  time.Duration(cfg.SummaryWindowDays) * 24 * time.Hour,
	}, scheduler.Deps{
		Frontier:   front,
		Store:      pages,
		Summarizer: sum,
		Images:     images.NewFetching(cfg.UserAgent, cfg.Timeout, 1<<20, rb, cfg.AllowPrivateSources),
		Claims:     scheduler.PostgresClaims{Pool: pool},
		Logger:     logger,
	})
}

func startSeedCrawl(ctx context.Context, f frontier.Frontier, cfg config.Config) (frontier.Crawl, error) {
	seed, err := urlnorm.Canonicalize(cfg.Seed)
	if err != nil {
		return frontier.Crawl{}, fmt.Errorf("invalid -seed %q: %w", cfg.Seed, err)
	}
	return frontier.StartCrawl(ctx, f, seed, cfg.MaxDepth, cfg.AllowedHosts, cfg.PathPrefixes)
}

func exitWhenDone(ctx context.Context, stop context.CancelFunc, f frontier.Frontier, crawlID int64, logger *slog.Logger) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			stats, err := f.Stats(ctx, crawlID)
			if err == nil && stats.Active() == 0 {
				logger.Info("crawl complete", "id", crawlID, "done", stats[frontier.Done], "failed", stats[frontier.Failed],
					"blocked", stats[frontier.Blocked], "skipped", stats[frontier.Skipped], "dead", stats[frontier.Dead])
				stop()
				return
			}
		}
	}
}

func serve(logger *slog.Logger, name, addr string, h http.Handler) *http.Server {
	srv := &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
	}

	go func() {
		logger.Info("listening", "server", name, "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "server", name, "error", err)
		}
	}()

	return srv
}

func pprofHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	return mux
}

func newLogger(cfg config.Config) *slog.Logger {
	var level slog.Level
	_ = level.UnmarshalText([]byte(cfg.LogLevel))

	opts := &slog.HandlerOptions{Level: level}
	if cfg.LogFormat == "json" {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}

// userAgentProduct returns the leading product token of a User-Agent
// ("CrawlerX/1.0 (+url)" -> "CrawlerX"), which is what robots.txt groups
// are matched against.
func userAgentProduct(ua string) string {
	if i := strings.IndexAny(ua, "/ "); i > 0 {
		return ua[:i]
	}
	return ua
}
