// Package config reads crawlerx's settings from flags, with environment
// variables as the defaults (flag > env > built-in default), so the same
// binary runs from a shell, a container or a compose file.
package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/fetch"
)

// Config is the full runtime configuration.
type Config struct {
	DatabaseURL string

	// ListenAddr serves the API; empty disables it.
	ListenAddr string
	// ReadOnly drops POST /crawls from the API, for a public deployment.
	ReadOnly bool
	// PprofAddr serves net/http/pprof; empty disables it.
	PprofAddr string

	Workers       int
	HostDelay     time.Duration
	HostInflight  int
	MaxBytes      int64
	Timeout       time.Duration
	MaxAttempts   int
	Lease         time.Duration
	ShutdownGrace time.Duration
	UserAgent     string
	Conditional   bool

	LogLevel  string
	LogFormat string

	// Seed, when set, starts a crawl at launch.
	Seed         string
	MaxDepth     int
	AllowedHosts []string
	PathPrefixes []string
	// ExitWhenDone makes a process that started a crawl exit once it ends.
	ExitWhenDone bool

	// SourcesFile enables the daily digest job: a JSON list of sites to crawl.
	SourcesFile string
	// DigestAt is the local time of day ("HH:MM") the job runs, in Timezone.
	DigestAt   string
	Timezone   string
	RunOnStart bool
	// AllowPrivateSources lets the image step fetch cited source pages on
	// loopback and private addresses. Only for testing against a local site.
	AllowPrivateSources bool
	// MaxSites optionally caps how many distinct sites the daily job may crawl
	// (each costs crawl time and summaries); 0 means no limit.
	MaxSites int
	// ImageCacheMB bounds the in-memory cache of proxied article images.
	ImageCacheMB int
	// SummaryProvider is auto (Gemini when a key is set, else extractive),
	// gemini or extractive.
	SummaryProvider string
	SummaryModel    string
	// SummaryBatch articles share one LLM request, up to SummaryWorkers requests
	// run at once, and articles dated within SummaryWindowDays get a gist.
	SummaryBatch      int
	SummaryWorkers    int
	SummaryWindowDays int
	// GeminiAPIKey comes from the environment only, so it never shows up in
	// a process listing.
	GeminiAPIKey string
}

// Load parses args (without the program name). getenv supplies the
// environment (os.Getenv in production). Usage and parse errors go to out.
func Load(args []string, getenv func(string) string, out io.Writer) (Config, error) {
	env := envReader{getenv: getenv}

	var (
		cfg          Config
		allowedHosts string
		pathPrefixes string
	)

	fs := flag.NewFlagSet("crawlerx", flag.ContinueOnError)
	fs.SetOutput(out)

	fs.StringVar(&cfg.DatabaseURL, "database-url", env.str("DATABASE_URL", ""), "Postgres connection URL (env DATABASE_URL, required)")
	fs.StringVar(&cfg.ListenAddr, "listen", env.str("LISTEN_ADDR", ":8080"), "HTTP API listen address; empty disables the API (env LISTEN_ADDR)")
	fs.BoolVar(&cfg.ReadOnly, "read-only", env.bool("READ_ONLY", false), "do not serve POST /crawls; for a public deployment (env READ_ONLY)")
	fs.StringVar(&cfg.PprofAddr, "pprof-addr", env.str("PPROF_ADDR", ""), "serve net/http/pprof on this address; empty disables it (env PPROF_ADDR)")

	fs.IntVar(&cfg.Workers, "workers", env.int("WORKERS", 8), "concurrent crawl workers; 0 = API only (env WORKERS)")
	fs.DurationVar(&cfg.HostDelay, "host-delay", env.duration("HOST_DELAY", time.Second), "minimum delay between requests to one host (env HOST_DELAY)")
	fs.IntVar(&cfg.HostInflight, "host-inflight", env.int("HOST_INFLIGHT", 2), "max concurrent requests per host in this process (env HOST_INFLIGHT)")
	fs.Int64Var(&cfg.MaxBytes, "max-bytes", env.int64("MAX_BYTES", 2<<20), "maximum response body size in bytes (env MAX_BYTES)")
	fs.DurationVar(&cfg.Timeout, "timeout", env.duration("TIMEOUT", 15*time.Second), "HTTP request timeout (env TIMEOUT)")
	fs.IntVar(&cfg.MaxAttempts, "max-attempts", env.int("MAX_ATTEMPTS", 5), "attempts before a URL is dead-lettered (env MAX_ATTEMPTS)")
	fs.DurationVar(&cfg.Lease, "lease", env.duration("LEASE", time.Minute), "how long a claimed URL is reserved before another worker may take it (env LEASE)")
	fs.DurationVar(&cfg.ShutdownGrace, "shutdown-grace", env.duration("SHUTDOWN_GRACE", 15*time.Second), "time allowed for in-flight work on shutdown (env SHUTDOWN_GRACE)")
	fs.StringVar(&cfg.UserAgent, "user-agent", env.str("USER_AGENT", fetch.DefaultUserAgent), "User-Agent header; include a contact URL (env USER_AGENT)")
	fs.BoolVar(&cfg.Conditional, "conditional", env.bool("CONDITIONAL", false), "send If-None-Match/If-Modified-Since for re-fetches; links on 304 pages are not rediscovered (env CONDITIONAL)")

	fs.StringVar(&cfg.LogLevel, "log-level", env.str("LOG_LEVEL", "info"), "debug, info, warn or error (env LOG_LEVEL)")
	fs.StringVar(&cfg.LogFormat, "log-format", env.str("LOG_FORMAT", "text"), "text or json (env LOG_FORMAT)")

	fs.StringVar(&cfg.Seed, "seed", env.str("SEED", ""), "start a crawl at this URL at launch (env SEED)")
	fs.IntVar(&cfg.MaxDepth, "max-depth", env.int("MAX_DEPTH", 2), "link depth for the -seed crawl (env MAX_DEPTH)")
	fs.StringVar(&allowedHosts, "allowed-hosts", env.str("ALLOWED_HOSTS", ""), "comma-separated host[:port] list the -seed crawl may visit; default is the seed's host (env ALLOWED_HOSTS)")
	fs.BoolVar(&cfg.ExitWhenDone, "exit-when-done", env.bool("EXIT_WHEN_DONE", false), "exit once the -seed crawl completes (env EXIT_WHEN_DONE)")

	fs.StringVar(&pathPrefixes, "path-prefixes", env.str("PATH_PREFIXES", ""), "comma-separated URL path prefixes the -seed crawl may follow, e.g. /current-affairs/; default is any path (env PATH_PREFIXES)")

	fs.StringVar(&cfg.SourcesFile, "sources", env.str("SOURCES", ""), "JSON file listing the news sites to crawl daily; enables the digest scheduler (env SOURCES)")
	fs.StringVar(&cfg.DigestAt, "digest-at", env.str("DIGEST_AT", "06:00"), "local time of day (HH:MM) the daily digest runs (env DIGEST_AT)")
	fs.StringVar(&cfg.Timezone, "timezone", env.str("TIMEZONE", "Asia/Kolkata"), "IANA time zone for -digest-at (env TIMEZONE)")
	fs.BoolVar(&cfg.RunOnStart, "run-on-start", env.bool("RUN_ON_START", false), "run the digest job once at startup, even if it already ran today (env RUN_ON_START)")
	fs.BoolVar(&cfg.AllowPrivateSources, "allow-private-sources", env.bool("ALLOW_PRIVATE_SOURCES", false), "let the source-image step fetch loopback/private addresses; for testing only (env ALLOW_PRIVATE_SOURCES)")
	fs.StringVar(&cfg.SummaryProvider, "summary-provider", env.str("SUMMARY_PROVIDER", "auto"), "auto, gemini or extractive; auto uses Gemini when GEMINI_API_KEY is set (env SUMMARY_PROVIDER)")
	fs.StringVar(&cfg.SummaryModel, "summary-model", env.str("SUMMARY_MODEL", ""), "Gemini model for summaries; default gemini-3.5-flash-lite (env SUMMARY_MODEL)")
	fs.IntVar(&cfg.MaxSites, "max-sites", env.int("MAX_SITES", 0), "optional cap on distinct sites the daily digest may crawl; startup fails above it; 0 = no limit (env MAX_SITES)")
	fs.IntVar(&cfg.ImageCacheMB, "image-cache-mb", env.int("IMAGE_CACHE_MB", 128), "memory for the cache of proxied article images; about 70 MB covers a day of 200 articles (env IMAGE_CACHE_MB)")
	fs.IntVar(&cfg.SummaryBatch, "summary-batch", env.int("SUMMARY_BATCH", 6), "articles per LLM request: fewer requests with a bigger batch, lower latency with a smaller one (env SUMMARY_BATCH)")
	fs.IntVar(&cfg.SummaryWorkers, "summary-workers", env.int("SUMMARY_WORKERS", 3), "LLM requests in flight at once (env SUMMARY_WORKERS)")
	fs.IntVar(&cfg.SummaryWindowDays, "summary-window-days", env.int("SUMMARY_WINDOW_DAYS", 7), "summarise articles dated within this many days; keep it at least as long as the longest period the digest page offers you (env SUMMARY_WINDOW_DAYS)")
	cfg.GeminiAPIKey = env.str("GEMINI_API_KEY", "")

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	if err := env.err(); err != nil {
		return Config{}, err
	}

	for _, h := range strings.Split(allowedHosts, ",") {
		if h = strings.TrimSpace(h); h != "" {
			cfg.AllowedHosts = append(cfg.AllowedHosts, h)
		}
	}

	for _, p := range strings.Split(pathPrefixes, ",") {
		if p = strings.TrimSpace(p); p != "" {
			cfg.PathPrefixes = append(cfg.PathPrefixes, p)
		}
	}

	return cfg, cfg.Validate()
}

// Validate rejects settings that cannot work.
func (c Config) Validate() error {
	var errs []error

	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("database URL is required (-database-url or DATABASE_URL)"))
	}
	if c.Workers < 0 {
		errs = append(errs, errors.New("workers must be >= 0"))
	}
	if c.HostDelay < 0 {
		errs = append(errs, errors.New("host-delay must be >= 0"))
	}
	if c.MaxBytes <= 0 {
		errs = append(errs, errors.New("max-bytes must be > 0"))
	}
	if c.MaxAttempts < 1 {
		errs = append(errs, errors.New("max-attempts must be >= 1"))
	}
	if c.MaxDepth < 0 || c.MaxDepth > 10 {
		errs = append(errs, errors.New("max-depth must be between 0 and 10"))
	}
	if c.Lease <= c.Timeout {
		errs = append(errs, fmt.Errorf("lease (%v) must be longer than timeout (%v), or in-flight fetches lose their lease", c.Lease, c.Timeout))
	}
	if c.ExitWhenDone && c.Seed == "" {
		errs = append(errs, errors.New("exit-when-done needs -seed"))
	}
	if c.ExitWhenDone && c.Workers == 0 {
		errs = append(errs, errors.New("exit-when-done needs workers > 0"))
	}
	if c.SourcesFile != "" {
		if _, err := time.Parse("15:04", c.DigestAt); err != nil {
			errs = append(errs, fmt.Errorf("digest-at must be HH:MM (24-hour), got %q", c.DigestAt))
		}
		if _, err := time.LoadLocation(c.Timezone); err != nil {
			errs = append(errs, fmt.Errorf("timezone %q is not a known IANA zone", c.Timezone))
		}
	}
	if c.MaxSites < 0 {
		errs = append(errs, errors.New("max-sites must be 0 (no limit) or more"))
	}
	if c.ImageCacheMB < 1 {
		errs = append(errs, errors.New("image-cache-mb must be at least 1"))
	}
	if c.SummaryBatch < 1 || c.SummaryBatch > 20 {
		errs = append(errs, errors.New("summary-batch must be between 1 and 20"))
	}
	if c.SummaryWorkers < 1 || c.SummaryWorkers > 8 {
		errs = append(errs, errors.New("summary-workers must be between 1 and 8"))
	}
	if c.SummaryWindowDays < 1 || c.SummaryWindowDays > 30 {
		errs = append(errs, errors.New("summary-window-days must be between 1 and 30"))
	}
	switch c.SummaryProvider {
	case "auto", "extractive":
	case "gemini":
		if c.GeminiAPIKey == "" {
			errs = append(errs, errors.New("summary-provider gemini needs GEMINI_API_KEY"))
		}
	default:
		errs = append(errs, fmt.Errorf("summary-provider must be auto, gemini or extractive, got %q", c.SummaryProvider))
	}
	switch c.LogFormat {
	case "text", "json":
	default:
		errs = append(errs, fmt.Errorf("log-format must be text or json, got %q", c.LogFormat))
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		errs = append(errs, fmt.Errorf("log-level must be debug, info, warn or error, got %q", c.LogLevel))
	}

	return errors.Join(errs...)
}

// envReader turns environment variables into flag defaults, remembering
// malformed values so Load can report them instead of silently ignoring them.
type envReader struct {
	getenv func(string) string
	errs   []error
}

func (e *envReader) err() error { return errors.Join(e.errs...) }

func (e *envReader) str(key, def string) string {
	if v := e.getenv(key); v != "" {
		return v
	}
	return def
}

func (e *envReader) int(key string, def int) int {
	v := e.getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		e.errs = append(e.errs, fmt.Errorf("%s=%q is not an integer", key, v))
		return def
	}
	return n
}

func (e *envReader) int64(key string, def int64) int64 {
	v := e.getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		e.errs = append(e.errs, fmt.Errorf("%s=%q is not an integer", key, v))
		return def
	}
	return n
}

func (e *envReader) duration(key string, def time.Duration) time.Duration {
	v := e.getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		e.errs = append(e.errs, fmt.Errorf("%s=%q is not a duration (try 500ms, 2s)", key, v))
		return def
	}
	return d
}

func (e *envReader) bool(key string, def bool) bool {
	v := e.getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		e.errs = append(e.errs, fmt.Errorf("%s=%q is not a boolean", key, v))
		return def
	}
	return b
}
