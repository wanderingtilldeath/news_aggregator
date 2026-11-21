// Package config reads crawlerx's settings from flags, with environment
// variables as the defaults (flag > env > built-in default), so the same
// binary runs from a shell, a container or a compose file.
package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/mail"
	"net/url"
	"regexp"
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
	// SummaryFallbackModel is tried when SummaryModel stays overloaded (HTTP 503) or rate limited; empty means none.
	SummaryFallbackModel string
	// SummaryBatch articles share one LLM request, up to SummaryWorkers requests
	// run at once, and articles dated within SummaryWindowDays get a gist.
	SummaryBatch      int
	SummaryWorkers    int
	SummaryWindowDays int
	// CrawlTimeout is the longest the daily run waits for its crawls before it summarises what it has.
	CrawlTimeout time.Duration
	// LLMBudget is the time one run spends on the model (summaries, enrichment, concepts) before it stops asking.
	LLMBudget time.Duration
	// GeminiAPIKey comes from the environment only, so it never shows up in
	// a process listing.
	GeminiAPIKey string

	// MailProvider is "log" (print the mail, the default) or "brevo". BrevoAPIKey
	// comes from the environment only. WebBaseURL is the public address of the web
	// frontend, which the links in emails point to.
	MailProvider string
	BrevoAPIKey  string
	MailFrom     string
	MailFromName string
	WebBaseURL   string
	// PublicAPIURL is the public address of the API (the Cloudflare Worker): the
	// one-click unsubscribe header of the daily email POSTs there.
	PublicAPIURL string
	// TelegramBotToken comes from the environment only. TelegramChats maps a track
	// to the channel its edition is posted to (empty: not posted).
	TelegramBotToken string
	TelegramChats    map[string]string
	// MailFooter is a line added to every daily email, for the sender's name and
	// postal address where the law asks for one.
	MailFooter string
	// BlockedHosts are sites that must not be crawled or shown (a publisher's takedown request),
	// comma separated in BLOCKED_HOSTS. The crawl job also deletes their stored pages.
	BlockedHosts []string
	// TeamPosts turns on Postpaper Teams (TEAM_POSTS=true): the API takes Slack and Discord webhooks at
	// POST /teams and the daily send posts the edition to them.
	TeamPosts bool
	// RazorpayWebhookSecret comes from the environment only; without it POST /billing/razorpay is
	// not served. ProPlans maps an amount paid, in paise, to the days of Pro it buys, from
	// PRO_PLANS ("9900:31,24900:92" by default: 99 rupees for a month, 249 for three months).
	RazorpayWebhookSecret string
	ProPlans              map[int]int
	proPlansRaw           string
	// AdminEmails are the addresses that may sign in as admin (ADMIN_EMAILS, comma separated, lower-cased) and
	// AdminPassword is the password they sign in with (ADMIN_PASSWORD, environment only). The admin routes are
	// served only when there is at least one address and a password of 12 characters or more.
	AdminEmails   []string
	AdminPassword string
	// MailSponsorText and MailSponsorURL are one labelled sponsor line in every daily email; it is
	// left out unless both are set.
	MailSponsorText, MailSponsorURL string
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
	fs.BoolVar(&cfg.ReadOnly, "read-only", env.bool("READ_ONLY", true), "do not serve POST /crawls (default true: the crawler fetches any URL it is given, so enable it with READ_ONLY=false only on a private network) (env READ_ONLY)")
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
	fs.StringVar(&cfg.SummaryFallbackModel, "summary-fallback-model", env.str("SUMMARY_FALLBACK_MODEL", ""), "Gemini model tried when the main one stays overloaded or rate limited; empty means none (env SUMMARY_FALLBACK_MODEL)")
	fs.IntVar(&cfg.SummaryBatch, "summary-batch", env.int("SUMMARY_BATCH", 6), "articles per LLM request: fewer requests with a bigger batch, lower latency with a smaller one (env SUMMARY_BATCH)")
	fs.IntVar(&cfg.SummaryWorkers, "summary-workers", env.int("SUMMARY_WORKERS", 3), "LLM requests in flight at once (env SUMMARY_WORKERS)")
	fs.DurationVar(&cfg.CrawlTimeout, "crawl-timeout", env.duration("CRAWL_TIMEOUT", 2*time.Hour), "longest the daily run waits for its crawls before summarising what it has; keep it under the job's own time limit (env CRAWL_TIMEOUT)")
	fs.DurationVar(&cfg.LLMBudget, "llm-budget", env.duration("LLM_BUDGET", 12*time.Minute), "time one run spends on the model before the rest waits for the next run, so a slow or overloaded model cannot stretch the run (env LLM_BUDGET)")
	fs.IntVar(&cfg.SummaryWindowDays, "summary-window-days", env.int("SUMMARY_WINDOW_DAYS", 7), "summarise articles dated within this many days; keep it at least as long as the longest period the digest page offers you (env SUMMARY_WINDOW_DAYS)")
	cfg.GeminiAPIKey = env.str("GEMINI_API_KEY", "")
	fs.StringVar(&cfg.MailProvider, "mail-provider", env.str("MAIL_PROVIDER", "log"), "log (print mail, send nothing) or brevo (env MAIL_PROVIDER)")
	fs.StringVar(&cfg.MailFrom, "mail-from", env.str("MAIL_FROM", ""), "sender address of emails; for brevo it must be on a domain you verified there (env MAIL_FROM)")
	fs.StringVar(&cfg.MailFromName, "mail-from-name", env.str("MAIL_FROM_NAME", "Postpaper"), "sender name of emails (env MAIL_FROM_NAME)")
	fs.StringVar(&cfg.WebBaseURL, "web-base-url", env.str("WEB_BASE_URL", ""), "public address of the web frontend, for links in emails, e.g. https://digest.example.com; empty means http://localhost:3000 with the log provider (env WEB_BASE_URL)")
	fs.StringVar(&cfg.PublicAPIURL, "public-api-url", env.str("PUBLIC_API_URL", ""), "public address of the API, used by the one-click unsubscribe header of the daily email (env PUBLIC_API_URL)")
	fs.StringVar(&cfg.MailSponsorText, "mail-sponsor-text", env.str("MAIL_SPONSOR_TEXT", ""), "text of the sponsor line in every daily email (env MAIL_SPONSOR_TEXT)")
	fs.StringVar(&cfg.MailSponsorURL, "mail-sponsor-url", env.str("MAIL_SPONSOR_URL", ""), "link of the sponsor line, http(s) only (env MAIL_SPONSOR_URL)")
	fs.StringVar(&cfg.MailFooter, "mail-footer", env.str("MAIL_FOOTER", ""), "line added to every daily email, e.g. your name and postal address (env MAIL_FOOTER)")
	for _, h := range strings.Split(env.str("BLOCKED_HOSTS", ""), ",") {
		if h = strings.TrimSpace(h); h != "" {
			cfg.BlockedHosts = append(cfg.BlockedHosts, h)
		}
	}
	cfg.TeamPosts = env.bool("TEAM_POSTS", false)
	cfg.RazorpayWebhookSecret = env.str("RAZORPAY_WEBHOOK_SECRET", "")
	cfg.AdminPassword = env.str("ADMIN_PASSWORD", "")
	for _, e := range strings.Split(env.str("ADMIN_EMAILS", ""), ",") {
		if e = strings.ToLower(strings.TrimSpace(e)); e != "" {
			cfg.AdminEmails = append(cfg.AdminEmails, e)
		}
	}
	fs.StringVar(&cfg.proPlansRaw, "pro-plans", env.str("PRO_PLANS", "9900:31,24900:92"), "Pro plans as paise:days pairs, e.g. 9900:31,24900:92 (env PRO_PLANS)")
	cfg.BrevoAPIKey = env.str("BREVO_API_KEY", "")
	cfg.TelegramBotToken = env.str("TELEGRAM_BOT_TOKEN", "")
	cfg.TelegramChats = map[string]string{}
	for tr, key := range map[string]string{"upsc": "TELEGRAM_CHAT_UPSC", "engineering": "TELEGRAM_CHAT_ENGINEERING"} {
		if v := env.str(key, ""); v != "" {
			cfg.TelegramChats[tr] = v
		}
	}

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

	cfg.WebBaseURL = strings.TrimRight(cfg.WebBaseURL, "/")
	cfg.PublicAPIURL = strings.TrimRight(cfg.PublicAPIURL, "/")
	if cfg.WebBaseURL == "" && cfg.MailProvider == "log" {
		cfg.WebBaseURL = "http://localhost:3000"
	}

	plans, err := ParsePlans(cfg.proPlansRaw)
	if err != nil {
		return cfg, err
	}
	cfg.ProPlans = plans

	return cfg, cfg.Validate()
}

// telegramChat matches a channel username or a numeric chat id.
var telegramChat = regexp.MustCompile(`^(@[A-Za-z][A-Za-z0-9_]{4,31}|-?[0-9]{5,20})$`)

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
	// One URL can wait for robots.txt (up to timeout) and then fetch (up to timeout), and a lease is not renewed.
	if c.Lease <= 2*c.Timeout {
		errs = append(errs, fmt.Errorf("lease (%v) must be longer than twice the timeout (%v), or in-flight fetches lose their lease", c.Lease, 2*c.Timeout))
	}
	if c.CrawlTimeout < time.Minute {
		errs = append(errs, errors.New("crawl-timeout must be at least 1m"))
	}
	if c.LLMBudget < time.Minute {
		errs = append(errs, errors.New("llm-budget must be at least 1m"))
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
	for tr, chat := range c.TelegramChats {
		if !telegramChat.MatchString(chat) {
			errs = append(errs, fmt.Errorf("the Telegram chat for the %s track must be @channelname or a numeric chat id, got %q", tr, chat))
		}
	}
	if len(c.TelegramChats) > 0 && c.TelegramBotToken == "" {
		errs = append(errs, errors.New("posting to Telegram needs TELEGRAM_BOT_TOKEN"))
	}
	switch c.MailProvider {
	case "log":
	case "brevo":
		if c.BrevoAPIKey == "" {
			errs = append(errs, errors.New("mail-provider brevo needs BREVO_API_KEY"))
		}
		if _, err := mail.ParseAddress(c.MailFrom); err != nil {
			errs = append(errs, fmt.Errorf("mail-provider brevo needs a valid -mail-from or MAIL_FROM address: %w", err))
		}
		if u, err := url.Parse(c.WebBaseURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			errs = append(errs, errors.New("mail-provider brevo needs -web-base-url or WEB_BASE_URL as an http(s) address"))
		}
	default:
		errs = append(errs, fmt.Errorf("mail-provider must be log or brevo, got %q", c.MailProvider))
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

// ParsePlans reads "paise:days" pairs separated by commas into a map, for example
// "9900:31,24900:92". An amount or a number of days that is not a whole number above zero, or an
// amount listed twice, is an error, so a typo is found at startup and not on a customer's payment.
func ParsePlans(s string) (map[int]int, error) {
	plans := map[int]int{}
	for _, pair := range strings.Split(s, ",") {
		if pair = strings.TrimSpace(pair); pair == "" {
			continue
		}
		amount, days, ok := strings.Cut(pair, ":")
		a, errA := strconv.Atoi(strings.TrimSpace(amount))
		d, errD := strconv.Atoi(strings.TrimSpace(days))
		if !ok || errA != nil || errD != nil || a <= 0 || d <= 0 {
			return nil, fmt.Errorf("pro-plans: %q must be paise:days with both above zero, like 9900:31", pair)
		}
		if _, dup := plans[a]; dup {
			return nil, fmt.Errorf("pro-plans: the amount %d is listed twice", a)
		}
		plans[a] = d
	}

	return plans, nil
}
