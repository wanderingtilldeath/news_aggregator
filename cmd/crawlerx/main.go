// Command crawlerx is a resumable, polite web crawler. Any number of
// processes can share one Postgres (work queue) and one Redis (per-host
// politeness gate), and a killed process loses no work: its leased URLs are
// picked up by the others once the lease expires.
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

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"

	"github.com/wanderingtilldeath/news-aggregator/internal/api"
	"github.com/wanderingtilldeath/news-aggregator/internal/config"
	"github.com/wanderingtilldeath/news-aggregator/internal/crawler"
	"github.com/wanderingtilldeath/news-aggregator/internal/db"
	"github.com/wanderingtilldeath/news-aggregator/internal/fetch"
	"github.com/wanderingtilldeath/news-aggregator/internal/frontier"
	"github.com/wanderingtilldeath/news-aggregator/internal/metrics"
	"github.com/wanderingtilldeath/news-aggregator/internal/politeness"
	"github.com/wanderingtilldeath/news-aggregator/internal/robots"
	"github.com/wanderingtilldeath/news-aggregator/internal/store"
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

	var (
		gate       politeness.Gate
		robotCache robots.Cache
	)
	if cfg.RedisURL != "" {
		opts, err := redis.ParseURL(cfg.RedisURL)
		if err != nil {
			return fmt.Errorf("invalid redis URL: %w", err)
		}

		rdb := redis.NewClient(opts)
		defer rdb.Close()

		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err = rdb.Ping(pingCtx).Err()
		cancel()
		if err != nil {
			return fmt.Errorf("redis: %w", err)
		}

		gate, robotCache = politeness.NewRedis(rdb), robots.NewRedisCache(rdb)
		checks["redis"] = func(ctx context.Context) error { return rdb.Ping(ctx).Err() }
	} else {
		logger.Warn("no -redis-url: per-host delays are enforced within this process only, " +
			"so several crawler processes would exceed the host delay together")
		gate = politeness.NewMemory(nil)
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	m := metrics.New(reg)

	robotChecker := robots.New(robots.Options{
		UserAgent: cfg.UserAgent,
		Product:   userAgentProduct(cfg.UserAgent),
		HTTP:      &http.Client{Timeout: cfg.Timeout},
		Cache:     robotCache,
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
		Metrics:  m,
		Logger:   logger,
	})

	var servers []*http.Server

	if cfg.ListenAddr != "" {
		servers = append(servers, serve(logger, "api", cfg.ListenAddr, api.NewHandler(api.Deps{
			Frontier: front,
			Store:    pages,
			Metrics:  promhttp.HandlerFor(reg, promhttp.HandlerOpts{}),
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
