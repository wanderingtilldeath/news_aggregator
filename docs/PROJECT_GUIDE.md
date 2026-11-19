# Postpaper: the complete project guide

This guide explains the whole project: what it does, how it is built, how the Go code and the HTTP API work, and how to run and change it. It teaches Go and HTTP API basics along the way, using real code from this repository, so each concept is shown where the project uses it.

It is written for someone who knows some programming but not necessarily Go. Read it top to bottom once, then use the table of contents as a reference.

Facts here were checked against the code in the repository (module `github.com/wanderingtilldeath/news-aggregator`, Go 1.26). Where something was not verified it says so. Three older documents are partly out of date (see chapter 36), so when they disagree with this guide, trust this guide and the code.

## Contents

- **[Part 1. What Postpaper is](#part-1-what-postpaper-is)**
  - [1. The product](#1-the-product)
  - [2. Glossary](#2-glossary)
- **[Part 2. Architecture](#part-2-architecture)**
  - [3. System overview](#3-system-overview)
  - [4. One binary, three run modes](#4-one-binary-three-run-modes)
  - [5. The request path of the public site](#5-the-request-path-of-the-public-site)
  - [6. The daily pipeline, in order](#6-the-daily-pipeline-in-order)
  - [7. Data flow and storage](#7-data-flow-and-storage)
  - [8. Deployment topology and free-tier limits](#8-deployment-topology-and-free-tier-limits)
  - [9. Repository layout](#9-repository-layout)
- **[Part 3. Go, taught through this codebase](#part-3-go-taught-through-this-codebase)**
  - [10. Modules, packages and the three programs](#10-modules-packages-and-the-three-programs)
  - [11. Types: structs, named types and optional values](#11-types-structs-named-types-and-optional-values)
  - [12. Interfaces and dependency injection](#12-interfaces-and-dependency-injection)
  - [13. Errors](#13-errors)
  - [14. Concurrency](#14-concurrency)
  - [15. `context.Context`](#15-contextcontext)
  - [16. Configuration](#16-configuration)
  - [17. Embedding files in the binary](#17-embedding-files-in-the-binary)
  - [18. Testing](#18-testing)
- **[Part 4. Package-by-package reference](#part-4-package-by-package-reference)**
  - [19. The crawl core](#19-the-crawl-core)
  - [20. The scheduler and sources](#20-the-scheduler-and-sources)
  - [21. The store and the database layer](#21-the-store-and-the-database-layer)
  - [22. Summaries and enrichment (`internal/summarize`)](#22-summaries-and-enrichment-internalsummarize)
  - [23. Content helpers](#23-content-helpers)
  - [24. Delivery: mail, channels, images](#24-delivery-mail-channels-images)
  - [25. Features and where they live](#25-features-and-where-they-live)
- **[Part 5. The HTTP API in Go](#part-5-the-http-api-in-go)**
  - [26. net/http basics, shown with this project](#26-nethttp-basics-shown-with-this-project)
  - [27. API reference](#27-api-reference)
  - [28. Security model](#28-security-model)
  - [29. Walkthroughs](#29-walkthroughs)
- **[Part 6. The database](#part-6-the-database)**
  - [30. Tables](#30-tables)
  - [31. SQL patterns explained](#31-sql-patterns-explained)
- **[Part 7. The frontend and the edge](#part-7-the-frontend-and-the-edge)**
  - [32. The Next.js site (`web/`)](#32-the-nextjs-site-web)
  - [33. The Cloudflare Worker (`edge/`)](#33-the-cloudflare-worker-edge)
  - [34. Scripts](#34-scripts)
- **[Part 8. Operating and changing the project](#part-8-operating-and-changing-the-project)**
  - [35. Running it](#35-running-it)
  - [36. Conventions that are easy to get wrong, risks and stale docs](#36-conventions-that-are-easy-to-get-wrong-risks-and-stale-docs)


---

# Part 1. What Postpaper is

## 1. The product

Postpaper is a **resumable, polite news crawler that also runs a daily news digest**. It crawls a list of websites (about 50 sources in `sources.json`), stores each article, writes a short summary of it, and publishes the result as a newspaper-style front page, a JSON API, a daily email, chat posts and a set of study tools.

It has two editions, called **tracks**:

- **UPSC**: current affairs for Indian civil-service exam candidates. It draws on the categories `india`, `world`, `upsc` and `editorial`.
- **Engineering**: technology and AI news for software engineers. It draws on `tech` and `ai`.

What a reader can get:

| Feature | What it is |
|---|---|
| Front page | A three-column newspaper layout, ranked by importance, with search, categories and a 1, 2, 7 or 30 day view. |
| Story page | A summary, a "why it matters" line, the exam relevance, concepts, other outlets covering the same story, and a link to the publisher. |
| "Today in 60 seconds" | A short written briefing per edition, made once a day. |
| Quiz | Five multiple-choice questions a day per edition, made from that day's stories, with a replay of missed ones. |
| Flashcards | Question and answer cards from the top stories, scheduled with spaced repetition in the browser. |
| Notebook | Sentences a reader saves from stories, kept only in the browser. |
| E-paper | Today's edition laid out as a printable newspaper page. |
| Concepts | A glossary of the ideas behind the news, each explained once and linked from the stories that use it. |
| Who is in the news | People, organisations and places named in stories, drawn as a graph, with a page per name. |
| Weekly revision and monthly compilations | The best stories grouped by syllabus area, printable. |
| Editorials | The editorials of The Hindu and The Indian Express side by side, with summary, why it matters and exam relevance. |
| Startup funding | Funding rounds of the week read from the headlines of five startup news sources, one row per round. |
| Underreported | Important stories that only one source carried. |
| Daily email | An edition sent each morning through Brevo, with one-click unsubscribe. |
| Telegram | A channel post each day and a bot that answers `/today`, `/topic`, `/quiz` and `/brief`. |
| Slack and Discord ("Postpaper Teams") | A team connects a channel webhook and gets the daily post. |
| Pro | A paid membership (two plans) that unlocks printable editions and priority email. Payments arrive by a Razorpay webhook. |
| Sources and takedown | A public page listing every site Postpaper reads, what each site's robots.txt says about AI use, and how to ask for removal. |

Everything is designed to run on **free tiers** with low load. That one constraint explains most of the architecture in Part 2.

A feature that existed and was removed: an "Upcoming" calendar that tried to find dates in article text. It kept picking up page furniture and repeating sentences, so it was deleted. Its database table still exists but nothing reads or writes it.

## 2. Glossary

These words appear throughout the code and this guide.

- **Source**: one entry in `sources.json`, describing a site section to crawl.
- **Crawl**: one job that starts from a seed URL and follows links to a depth. One crawl per source (or per source and day) is started each run.
- **Frontier**: the queue of URLs still to fetch plus the record of every URL already seen. In this project it is one Postgres table, also called `frontier`.
- **Lease**: a worker "borrows" a URL for a time. If the worker dies, the lease expires and another worker takes the URL. This is how crashes are survived without losing work.
- **Page**: one stored document in the `pages` table. An article is a page with `is_article` true. The API id of a story is the page's `url_hash`.
- **Gist** or **summary**: the short list of bullet sentences written for an article.
- **Enrichment**: extra fields written by the language model after the gist. **Stage 1** adds "why it matters", tags, importance and named entities. **Stage 2** (only for the top few stories) adds a long summary, concepts, and for UPSC an exam lens.
- **Exam lens**: how relevant a story is to Prelims, Mains, Essay and Ethics (0 to 3 each), plus both sides of the argument, facts and an answer skeleton.
- **Track**: an edition, `upsc` or `engineering`.
- **Edition day**: a calendar day in India time (`Asia/Kolkata`). Quizzes, intros and emails are per edition day.
- **Story group**: several articles from different sites about the same event, found by comparing headlines.
- **Snapshot**: a set of static JSON files (the last 7 days of the digest and more) built after each crawl and served by the edge Worker, so most reads never touch the database.
- **Edge**: the Cloudflare Worker that sits in front of the API.
- **Serve-only box**: the Render web service that runs the API with no crawling.

---

# Part 2. Architecture

## 3. System overview

```
                         ┌────────────────────────────────────────────────────┐
                         │                GitHub Actions (cron)               │
                         │   .github/workflows/crawl.yml, every 6 hours       │
                         │                                                    │
   ~50 news sites ◀──────┤  crawlerx  ──▶  Postgres (Neon)  ◀── Gemini API    │
   (robots.txt, polite)  │  crawl, parse,    pages, frontier,    summaries,   │
                         │  summarise,       subscribers, ...    enrichment,  │
                         │  group, enrich                        quiz, intro  │
                         │        │                                           │
                         │        ├─▶ prune old rows                          │
                         │        ├─▶ build-snapshot.sh ─▶ JSON + images      │
                         │        │       └─▶ wrangler deploy ──┐             │
                         │        └─▶ senddigest ─▶ Brevo (email), Telegram,  │
                         │                         Slack/Discord webhooks     │
                         └─────────────────────────────────────────┼──────────┘
                                                                   │ static assets
   Reader's browser                                                ▼
        │                                              ┌──────────────────────┐
        ▼                                              │ Cloudflare Worker    │
   Vercel: Next.js site (web/) ───────────────────────▶│ edge/worker.js       │
   server components, "use cache"        API_BASE_URL  │ answers from the     │
                                                       │ snapshot, else proxy │
                                                       └──────────┬───────────┘
                                                                  │ everything else
                                                                  ▼
                                                       ┌──────────────────────┐
                          Razorpay webhook ───────────▶│ Render: crawlerx     │──▶ Postgres
                                                       │ serve-only API       │
                                                       └──────────────────────┘
```

Read it from three directions:

1. **Write path (every six hours).** GitHub Actions runs the Go binary as a short-lived job. It crawls, stores, summarises and enriches, then builds a snapshot and publishes it to Cloudflare, then sends the daily posts.
2. **Read path (every page view).** The browser asks the Next.js site on Vercel. Its server components call the API through the Cloudflare Worker. The Worker answers from the static snapshot when it can. Only what the snapshot cannot answer reaches the Render API, which reads Postgres.
3. **Write path from readers.** Signups, confirmations, unsubscribes, Teams connections and Razorpay webhooks go through the Worker as plain proxied POST requests to the Render API, which writes Postgres.

Postgres is the only stateful service. There is no Redis, no queue service and no object store.

## 4. One binary, three run modes

The Go program `cmd/crawlerx` is a single binary. What it does depends on configuration. This is how the free deployment works:

| Mode | Where | Settings | What runs |
|---|---|---|---|
| **Full** | A laptop or a small server | `-sources sources.json`, `WORKERS` above 0 | The API, the crawl workers and the daily scheduler, all in one process. |
| **Serve-only** | Render (free web service) | `WORKERS=0 READ_ONLY=true`, no `SOURCES` | Only the HTTP API. `READ_ONLY` removes `POST /crawls`. Memory is tight (`GOMEMLIMIT=180MiB`), so it never crawls. |
| **Crawl job** | GitHub Actions | `-listen=` (API off) with `SOURCES=sources.json`, `RUN_ON_START=true` | Crawl workers and the scheduler for one run. The workflow waits for the log line `digest run complete`, stops the process, then continues. |

Why this shape: Render's free instance has 512 MB of memory and sleeps when idle, so it cannot crawl. GitHub Actions has plenty of memory and time, and it runs on a schedule, so it does the heavy work. Neon (free Postgres) suspends compute after five minutes idle, so the serve-only box uses `WORKERS=0` to avoid polling the database while it sleeps.

The log line `digest run complete` is a contract. The workflow greps for it. Do not reword it.

## 5. The request path of the public site

A reader's request travels:

```
browser ─▶ Vercel (Next.js) ─▶ Cloudflare Worker ─┬─▶ static snapshot file     (most reads)
                                                  └─▶ Render API ─▶ Postgres   (the rest)
```

The Worker (`edge/worker.js`) tries these in order:

1. `POST /telegram/webhook`: the Telegram bot.
2. `GET /`: the old static front page from the snapshot assets.
3. `GET /digest` and `GET /digest/sources`: answered from `snapshot.json` for the 1, 2 and 7 day views (and for a given edition `day=`). Anything else, such as the 30 day view or a text search, goes to the origin.
4. `GET /story/{id}`: a static `stories/{id}.json`, else the origin with a six hour cache.
5. A table of published JSON files (`/cards`, `/months`, `/concepts`, `/entities`), served only for the exact query strings the snapshot was built with.
6. `GET /month`, `GET /quiz`, `GET /intro`: static files for recent days, else the origin.
7. Everything else, including every `POST`, is proxied to the Render origin unchanged.

**The rule that is easy to break:** the Worker re-implements in JavaScript the filtering that Go does in `internal/api` and `internal/store` (category, source, search term, paging, and the `categoryOf` fallback to `other`). A change to how `/digest` filters must be made in both places, and their output compared for the same query.

## 6. The daily pipeline, in order

The scheduler (`internal/scheduler/scheduler.go`, function `runOnce`) does these steps. The GitHub Actions job wraps them with a few more.

1. **Start crawls.** For each source in `sources.json`, create a crawl (one per day back if the source uses `{date:...}` seeds).
2. **Crawl, and summarise while crawling.** Workers fetch pages in the background. Every few seconds the scheduler checks progress, and whenever a full batch of new articles is waiting, a pass writes their gists and looks up their images. The first cards appear minutes into the run, not after the last page.
3. **Final summary pass.** Whatever is left is summarised. If the model provider rate-limited the run, the scheduler waits out the limit (default 70 seconds) and finishes, up to three times.
4. **Group stories.** Compare headlines over the last 30 days (only stories within 72 hours of each other are compared), with no model, and mark stories several outlets cover.
5. **Enrich, stage 1** (all recent summarised articles, up to 60): why it matters, tags, importance, entities.
6. **Enrich, stage 2** (top 5 by importance): long summary, concepts and, for UPSC categories, the exam lens and flashcards.
7. **Explain concepts**: one model request writes up to 12 new glossary entries.
8. **Make quizzes**: one per track from the last 24 hours.
9. **Make intros**: the "Today in 60 seconds" text per track.
10. **Compile the month**: merge today's important stories into the monthly compilation, with no model.
11. **Log `digest run complete`.**

Then the workflow (`crawl.yml`):

12. Stop the binary.
13. **Prune**: delete crawls older than 3 days, pages older than 60 days, per-day records (deliveries, channel posts, job claims, quizzes, intros) older than 60 days, and pages of any takedown hosts.
14. **Build the snapshot** (`scripts/build-snapshot.sh`): start the app serve-only on loopback, read the API, write static files.
15. **Deploy** the snapshot to Cloudflare with `wrangler deploy`.
16. **Warm the caches** by visiting the main pages.
17. **Send the daily posts** (`cmd/senddigest`): email, Telegram, Slack and Discord.

Every step after the summaries is best effort. A failure is logged and the run continues, so one failing model call never blocks the deploy.

## 7. Data flow and storage

The central object is a row in the `pages` table. A page is created when the crawler stores a fetched document. Each later stage fills in more columns on the same row.

Every stage follows the same "pending, then set" pattern:

- A nullable timestamp column marks the stage as done (`summarized_at`, `enriched_at`, `detailed_at`, `image_checked_at`).
- A query selects rows where that column is still null (the "pending" rows), usually behind a partial index.
- A pass processes a batch and sets the column.

This makes every stage **idempotent and resumable**. Running a pass twice does nothing the second time. If a run is killed halfway, the next run picks up exactly the rows still pending.

Retention is handled by the workflow, not by Go code: crawls older than 3 days are deleted (their `frontier` rows cascade), and pages older than 60 days are deleted. Pages have no foreign key to crawls, so they outlive their crawl.

Derived tables outlive pages: `quizzes`, `intros` and `compilations` are stored separately, so the monthly compilation survives after the stories it links are pruned.

## 8. Deployment topology and free-tier limits

| Piece | Role | Free-tier facts that shaped the design |
|---|---|---|
| **Neon** | Postgres, the only datastore. | About 1 GB storage, compute suspends after 5 minutes idle. Use the direct connection string (no `-pooler`): the pgx statement cache and goose's advisory lock break behind transaction pooling. |
| **Render** | The serve-only API. | 512 MB memory and a small CPU share, sleeps after about 15 minutes idle and takes about a minute to wake. Blocks outbound SMTP, which is why mail goes over HTTPS. |
| **Cloudflare Worker and assets** | The edge and the snapshot. | Limited requests a day and CPU time per request, so the Worker does little work and serves static files. |
| **Vercel** | The Next.js site in `web/`. | Builds must not depend on the API being reachable. |
| **GitHub Actions** | The crawl job, every 6 hours. | Free on a public repository. Cron can start late. |
| **Brevo** | Email over HTTPS. | About 300 emails a day. |
| **Gemini** | Summaries, enrichment, quiz, intro, concepts. | Free key with quota. A quota error pauses the run instead of failing it. |
| **Telegram, Slack, Discord, Razorpay** | Channel posts and payments. | Webhook based, no servers to run. |

Design choices these forced: crawl in a scheduled job, never on the web box; publish reads as static files; keep Postgres connections few; batch model requests; make every stage resumable.

One known risk: a signup, confirmation or Teams request that arrives while Render is asleep may take about a minute to wake it, but the web app's server actions time out after 10 seconds, so the first request after a quiet period can fail and need a retry.

## 9. Repository layout

```
cmd/
  crawlerx/        the main binary: API, workers and scheduler (main.go wires everything)
  checksource/     a tool to vet a site before adding it to sources.json
  senddigest/      sends the daily email, Telegram and Teams posts
internal/          all application packages (not importable from outside this module)
  api/             HTTP handlers and the embedded pages (static/)
  chat/            Slack and Discord webhook posting, with a strict address check
  compile/         the monthly compilation merge
  config/          flags and environment variables
  crawler/         the worker loop
  db/              connection pool, embedded goose migrations
  digestmail/      builds and sends the daily email and channel posts
  e2e/             end-to-end tests that run the binary as a subprocess
  fakesite/        a synthetic website for tests
  fetch/           the polite HTTP fetcher, retry and backoff
  frontier/        the Postgres work queue (and an in-memory twin)
  graph/           the "who is in the news" graph
  group/           story grouping by headline
  images/          image lookup from source pages and the /img proxy
  kind/            opinion, analysis and explainer labels from URL paths
  mail/            email sending (log and Brevo) and confirmation mail
  parse/           HTML parsing, dates, article detection
  politeness/      the per-host request gate
  prose/           sentence splitting and lead extraction
  robots/          robots.txt checking
  scheduler/       sources.json and the daily run
  sourcetype/      publisher type per host
  store/           the Store interface, Postgres and in-memory implementations
  summarize/       gist, enrichment, concepts, quiz and intro (Gemini or extractive)
  telegram/        Telegram posting
  testutil/        a test Postgres schema per test
  track/           editions, reading modes and topic cleanup
  urlnorm/         URL canonicalisation
web/               the Next.js 16 site (deployed on Vercel)
edge/              the Cloudflare Worker, its config and the Telegram bot
scripts/           build-snapshot.sh and warm-caches.sh
docs/              deployment, reference, roadmap and this guide
catalog/           spare source entries not enabled by default
sources.json       the sites crawled each day
FEATURES.md        a ledger of every subsystem (key files, schema, trade-offs)
CLAUDE.md          short guidance for working in the repository
```

---

# Part 3. Go, taught through this codebase

This part explains the Go language features the project uses, one at a time, each with an excerpt from the real code. If you already know Go, skim it for where each idea lives in the repository.

## 10. Modules, packages and the three programs

**A module** is a tree of Go code with a `go.mod` file at its root. This project's module is named after its repository path:

```
module github.com/wanderingtilldeath/news-aggregator

go 1.26.0

require (
	github.com/jackc/pgx/v5 v5.11.0        // the Postgres driver and pool
	github.com/pressly/goose/v3 v3.29.0    // database migrations
	github.com/temoto/robotstxt v1.1.2     // robots.txt parsing
	golang.org/x/net v0.61.0               // the HTML parser
	golang.org/x/sync v0.24.0              // singleflight
)
```

There are only five direct dependencies. Everything else, including the HTTP server, JSON, templates, hashing, and testing, comes from Go's standard library. That is a deliberate choice: fewer dependencies means less to update and audit.

**A package** is a directory of `.go` files that share a `package name` line. Code in one package uses another by importing its path, for example `github.com/wanderingtilldeath/news-aggregator/internal/frontier`. Names that start with a capital letter (`Frontier`, `Claim`) are **exported**, usable from other packages. Lower-case names (`claimSQL`, `worker`) are private to their package.

**The `internal/` directory** has special meaning to the Go tool: packages under it can only be imported by code inside this module. So the whole application is private, and the only public surface is the programs and the HTTP API.

**A program** is a package named `main` with a `func main()`. There are three, under `cmd/`:

- `cmd/crawlerx`: the server, crawler and scheduler (chapter 4).
- `cmd/checksource`: a command-line tool that vets a site before it is added to `sources.json`.
- `cmd/senddigest`: sends the daily email, Telegram and Teams posts. It reuses the same config and store packages.

**How `main` is wired.** `main.go` does very little. It reads config, then calls `run`, which creates each piece and passes it to the next. This is called *constructor wiring* or *dependency injection by hand*:

```go
// cmd/crawlerx/main.go (shape only: errors, flags and optional pieces left out)
cfg, err := config.Load(os.Args[1:], os.Getenv, os.Stderr)
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()
pool, _ := db.Open(ctx, cfg.DatabaseURL, cfg.Workers+8) // pool size = workers + 8
_ = db.Migrate(ctx, pool)                                // goose, behind an advisory lock
front := frontier.NewPostgres(pool)
pages := store.NewPostgres(pool)
c := crawler.New(crawlerCfg, crawler.Deps{Frontier: front, Store: pages /* ... */})
go sched.Run(ctx)          // the daily scheduler, only if -sources was given
err = c.Run(ctx)           // blocks until shutdown
```

The shutdown order matters and is explicit in `main.go`: the workers drain, then the page writer is closed so its pending batch is flushed, then the HTTP server shuts down with a five second deadline.

## 11. Types: structs, named types and optional values

**A struct** groups fields. Configuration is a struct, a crawl is a struct, a page is a struct. Struct fields can carry **tags** that control JSON names:

```go
// internal/frontier/frontier.go
type DepthRow struct {
	Depth int              `json:"depth"`
	Stats map[string]int64 `json:"stats"`
}
```

**A named type with methods.** You can give any type a new name and attach methods to it. `Stats` is just a map from status to count, but it has behaviour:

```go
// internal/frontier/frontier.go
type Stats map[Status]int64

// Active is the number of rows that may still produce work.
func (s Stats) Active() int64 { return s[Pending] + s[Leased] }

// Total is the number of distinct URLs discovered.
func (s Stats) Total() int64 {
	var n int64
	for _, v := range s {
		n += v
	}
	return n
}
```

`Status` itself is a string type with constants, Go's version of an enum: `Pending`, `Leased`, `Done`, `Failed`, `Blocked`, `Skipped`, `Dead`. The database stores the same strings, and a `CHECK` constraint in the migration lists them.

**Pointers for "not set".** In `sources.json`, `max_depth` may be left out, and 0 is a real value, so a plain `int` cannot tell "missing" from "zero". The code uses a pointer, where `nil` means missing:

```go
// internal/scheduler/sources.go
MaxDepth *int `json:"max_depth"`

func (s Source) Depth() int {
	if s.MaxDepth == nil {
		return defaultSourceDepth
	}
	return *s.MaxDepth
}
```

**Useful zero values.** A good Go habit is to make the zero value of a type usable. The Slack and Discord poster is an empty struct that works with no setup, and tests can give it a custom HTTP client:

```go
// internal/chat/chat.go
// Webhook posts over HTTPS. The zero value works; tests give it a Client that routes elsewhere.
type Webhook struct {
	Client *http.Client
}
```

**Slices and maps.** A slice (`[]string`) is a growable list; a map (`map[string]bool`) is a hash table. A common idiom is a set as `map[string]struct{}` or `map[string]bool`; the parser uses one to de-duplicate links. Another is a composite key as an array: the in-memory store uses `map[[2]string]bool` keyed by `[edition day, channel]`.

## 12. Interfaces and dependency injection

**An interface** is a list of method signatures. Any type that has those methods satisfies the interface automatically; there is no `implements` keyword. This is how the code stays testable: components depend on an interface, and tests pass a fake.

The work queue is an interface (shortened):

```go
// internal/frontier/frontier.go
type Frontier interface {
	CreateCrawl(ctx context.Context, spec CrawlSpec) (Crawl, error)
	GetCrawl(ctx context.Context, id int64) (Crawl, error)
	Enqueue(ctx context.Context, crawlID int64, items []NewItem) (int, error)
	Claim(ctx context.Context, n int, lease time.Duration) ([]Item, error)
	Finish(ctx context.Context, it Item, status Status, note string) error
	Reschedule(ctx context.Context, it Item, at time.Time, countAttempt bool, note string) error
	// ...
}
```

There are two implementations in the same package: `Postgres` (production, safe across many processes) and `Memory` (a mutex and a map, for fast tests). The last line of each file is a **compile-time check** that the type really satisfies the interface:

```go
var _ Frontier = (*Postgres)(nil)   // internal/frontier/postgres.go
var _ Frontier = (*Memory)(nil)     // internal/frontier/memory.go
```

If someone adds a method to the interface and forgets one implementation, the build fails right there instead of at runtime.

The same pattern is used for the page store (`store.Store` with `Postgres` and `Memory`), the fetcher, the robots checker, the politeness gate, the mail sender, the Telegram and chat posters, the summariser and several more.

**The parity rule.** Because `Store` has two implementations, they must behave the same. Every filter in `DigestQuery` is written twice, as SQL in `postgres.go` and as Go in `memory.go`, and tested against both with one shared "contract" test function. Adding a store method means implementing it twice and adding it to the contract.

**Small interfaces, one big type.** The Gemini client is one concrete type, but the scheduler sees it through five tiny interfaces: `Summarizer`, `Enricher`, `Quizzer`, `Introducer` and `Explainer`. If a feature is not configured, that interface value is `nil`, and the scheduler skips the step. So "no API key" simply means fewer steps run.

**Optional interfaces by type assertion.** A value can be asked at runtime whether it also has an extra method. The scheduler logs the number of model calls only if the summariser can report it:

```go
// internal/scheduler/scheduler.go
if c, ok := s.deps.Summarizer.(interface{ Calls() int64 }); ok {
	attrs = append(attrs, "llm_calls", c.Calls())
}
```

**Dependency structs.** Components receive their collaborators in a `Deps` struct. Required ones are checked, optional ones may be nil, and `Now` is a function so tests can control time:

```go
// internal/crawler/crawler.go
// Deps are the crawler's collaborators. Frontier and Fetcher are required;
// every other field may be nil.
type Deps struct {
	Frontier frontier.Frontier
	Fetcher  fetch.Fetcher
	// Parser defaults to parse.HTMLParser.
	Parser parse.Parser
	// Store receives parsed pages; nil discards them.
	Store store.Store
	// Gate enforces per-host delay; nil disables politeness delays.
	Gate politeness.Gate
	// Robots enforces robots.txt; nil allows everything.
	Robots robots.Checker
	Logger *slog.Logger
	// Now and Rand are test seams.
	Now  func() time.Time
	Rand func() float64
}
```

## 13. Errors

Go has no exceptions. A function that can fail returns an `error` as its last value, and the caller checks it:

```go
rows, err := p.pool.Query(ctx, claimSQL, n, lease.Seconds())
if err != nil {
	return nil, fmt.Errorf("frontier: claim: %w", err)
}
defer rows.Close()
```

The idioms used consistently here:

- **Wrap with context** using `fmt.Errorf("package: what: %w", err)`. The `%w` verb keeps the original error inside, so callers can still inspect it. Messages read like a path: `store: digest: ...`.
- **Sentinel errors** are package-level values compared with `errors.Is`. Examples: `frontier.ErrNotFound`, `summarize.ErrRateLimited`, `chat.ErrGone`. The API turns `frontier.ErrNotFound` into a 404.
- **Error types** carry data. `images.BlockedError` records that a host refused, and `summarize.retryAfterError` carries the server's "try again in N seconds" hint. Callers find them with `errors.As`.
- **`errors.Join`** collects several errors into one. Config validation and `sources.json` parsing report every problem at once instead of stopping at the first.
- **Secrets stay out of errors.** The Telegram, Slack, Discord, mail and Gemini clients deliberately replace or omit underlying errors that could contain a token, a webhook address or a response body.
- **Best effort.** After the summaries, each scheduler step logs a failure and carries on. The log call checks `ctx.Err() == nil` first, so a normal shutdown is not reported as a failure.

**`defer`** schedules a call to run when the function returns, however it returns. It is used for closing rows, releasing locks and rolling back transactions:

```go
tx, err := s.pool.Begin(ctx)
// ...
defer tx.Rollback(ctx) //nolint:errcheck // no-op after Commit
```

## 14. Concurrency

Go's concurrency has three tools used here: goroutines, channels and the `sync` package.

**A goroutine** is a lightweight thread started with `go`. **A `sync.WaitGroup`** counts running goroutines so you can wait for all of them. The crawler's worker pool is the standard shape:

```go
// internal/crawler/crawler.go (Run)
var wg sync.WaitGroup
for i := 1; i <= c.cfg.Workers; i++ {
	wg.Add(1)
	go func(id int) {
		defer wg.Done()
		c.worker(ctx, procCtx, id)
	}(i)
}

drained := make(chan struct{})
go func() {
	wg.Wait()
	close(drained)
}()
```

The second goroutine waits for all workers and then closes a channel. Closing a channel is a broadcast: anything waiting on `<-drained` wakes up.

**`select`** waits on several channels at once and runs whichever is ready first. The shutdown logic nests two of them: wait for the workers to drain, or for the shutdown signal, and after the signal give the workers a grace period before forcing them to stop:

```go
select {
case <-drained:
case <-ctx.Done():
	grace := time.NewTimer(c.cfg.ShutdownGrace)
	defer grace.Stop()

	select {
	case <-drained:
	case <-grace.C:
		c.deps.Logger.Warn("shutdown grace expired, aborting in-flight work")
		procCancel()
		<-drained
	}
}
```

**Channels for a request and its answer.** The page store batches writes: many crawler workers call `Save`, but one goroutine groups them and sends one database round trip. Each call sends a request that carries its own reply channel:

```go
// internal/store/postgres.go
func (s *Postgres) Save(ctx context.Context, p Page) error {
	req := saveRequest{page: p, done: make(chan error, 1)}

	select {
	case s.reqs <- req:
	case <-ctx.Done():
		return ctx.Err()
	case <-s.quit:
		return errClosed
	}

	select {
	case err := <-req.done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
```

The reply channel has a buffer of 1, so the batching goroutine never blocks on a caller that has given up. The batching loop itself collects requests until it has 64 or 10 milliseconds have passed, using a labelled `break`:

```go
func (s *Postgres) run() {
	defer s.wg.Done()
	for {
		var batch []saveRequest
		select {
		case req := <-s.reqs:
			batch = append(batch, req)
		case <-s.quit:
			return
		}

		timer := time.NewTimer(s.linger)
	collect:
		for len(batch) < s.batchSize {
			select {
			case req := <-s.reqs:
				batch = append(batch, req)
			case <-timer.C:
				break collect
			case <-s.quit:
				break collect
			}
		}
		timer.Stop()
		s.flush(batch)
	}
}
```

This is called **group commit**: it trades a few milliseconds of latency for far fewer database round trips.

**A job channel and a feeder.** The summary pass starts a fixed number of workers that read batches from a channel, and a loop that feeds it, stopping early if the context is cancelled:

```go
// internal/summarize/summarize.go (Pass)
for i := 0; i < opt.Workers; i++ {
	wg.Add(1)
	go worker()
}

feed:
for _, b := range batches {
	select {
	case jobs <- b:
	case <-ctx.Done():
		break feed
	}
}
close(jobs)
wg.Wait()
```

**Mutexes.** A `sync.Mutex` protects shared data. The in-memory implementations lock at the top of every method with `defer` to unlock. The Gemini client uses one to enforce a minimum gap between requests.

**`sync.Once`** runs something exactly once; the store's `Close` uses it so closing twice is safe. **`sync/atomic`** gives lock-free counters; the Gemini client counts calls with an `atomic.Int64`.

**`singleflight`** (from `golang.org/x/sync`) makes concurrent callers share one in-flight call. The robots checker uses it so that sixteen workers hitting a new site fetch its robots.txt once, not sixteen times. The image proxy uses it the same way:

```go
// internal/robots/robots.go
// One fetch per origin at a time, however many workers ask at once.
v, err, _ := c.group.Do(origin, func() (any, error) {
	return c.load(ctx, origin)
})
```

**Where goroutines are started** (the complete list in non-test code):

| Where | Purpose |
|---|---|
| `crawler.Run` | The worker pool, and the goroutine that closes `drained`. |
| `crawler.cancelWhenDone` | A ticker that cancels a derived context when a crawl has no active URLs. |
| `store.NewPostgres` | The group-commit writer. |
| `summarize.Pass` | A pool of summary workers. |
| `scheduler.enrich` | Runs the image pass beside the summary pass; they share no state. |
| `mail` confirmation and Pro notifiers | A fire-and-forget send on a detached context. |
| `cmd/crawlerx/main.go` | The scheduler, `exitWhenDone`, the HTTP servers. |

Notably absent: the frontier has no goroutines at all. All coordination between workers, and between processes, happens in SQL (chapter 31).

## 15. `context.Context`

A `context.Context` carries a cancellation signal and a deadline through a call tree. Almost every function that does I/O takes `ctx` as its first parameter and passes it on. When the context is cancelled (the user pressed Ctrl-C, a deadline passed, an HTTP client disconnected), every well-behaved call returns early.

The forms used here:

- **`signal.NotifyContext`** in `main` turns Ctrl-C and SIGTERM into a cancelled context.
- **`context.WithTimeout`** bounds a call, for example the health check's two second database ping.
- **`context.WithCancel`** lets one goroutine stop its siblings; the summary pass cancels on a rate limit or store error.
- **`context.WithoutCancel`** (Go 1.21) keeps a context's values but drops its cancellation. This is used on purpose in four places: the crawler's in-flight work (so shutdown stops *claiming* but lets fetches finish), releasing leases on shutdown, the confirmation email (so a client disconnecting does not cancel the send) and the shared image fetch (so one viewer leaving does not cancel it for others).

```go
// internal/crawler/crawler.go
// In-flight work runs on its own context so cancelling ctx stops
// *claiming* immediately but lets current fetches finish.
procCtx, procCancel := context.WithCancel(context.WithoutCancel(ctx))
defer procCancel()
```

After each I/O call, the crawler checks `ctx.Err()`. That is how it tells "we are shutting down" (return quietly, release the URL) from "this fetch failed" (record a failure and retry later).

## 16. Configuration

All settings come from flags with environment fallbacks, in `internal/config`. The precedence is **flag, then environment variable, then built-in default**. The trick is to use the environment value as the flag's default:

```go
// internal/config/config.go
func Load(args []string, getenv func(string) string, out io.Writer) (Config, error) {
	env := envReader{getenv: getenv}
	// ...
	fs := flag.NewFlagSet("crawlerx", flag.ContinueOnError)
	fs.StringVar(&cfg.DatabaseURL, "database-url", env.str("DATABASE_URL", ""), "Postgres connection URL (env DATABASE_URL, required)")
	fs.StringVar(&cfg.ListenAddr, "listen", env.str("LISTEN_ADDR", ":8080"), "HTTP API listen address; empty disables the API (env LISTEN_ADDR)")
	fs.BoolVar(&cfg.ReadOnly, "read-only", env.bool("READ_ONLY", true), "do not serve POST /crawls; ...")
```

Two design points worth copying: `Load` takes `getenv` and the output writer as arguments instead of reading globals, so tests can pass a fake environment; and a malformed environment value (like `WORKERS=abc`) is collected and reported together with every other problem, rather than silently ignored. Secrets (`GEMINI_API_KEY`, `BREVO_API_KEY`, `TELEGRAM_BOT_TOKEN`, `RAZORPAY_WEBHOOK_SECRET`) are environment-only, with no flag, so they never appear in a process listing.

The full variable table is in chapter 35.

## 17. Embedding files in the binary

`//go:embed` copies files into the compiled program, so the binary needs nothing next to it. The project embeds two things:

```go
// internal/db/db.go
//go:embed migrations/*.sql
var migrations embed.FS

// internal/api/api.go
//go:embed static
var staticFiles embed.FS
```

The first holds the database migrations; the second holds the two HTML pages the API serves (`index.html`, the original front page, and `dashboard.html`, the crawler dashboard). `http.FileServerFS` serves an `embed.FS` directly.

## 18. Testing

Go's tests live next to the code in `*_test.go` files and run with `go test`. The project leans on a few patterns.

**Table-driven tests.** One test function loops over a slice of cases, each run as a named subtest:

```go
// internal/urlnorm/urlnorm_test.go
tests := []struct {
	name    string
	link    string
	baseURL string
	want    string
	wantOK  bool
}{
	{"relative path resolved against base", "/path", "https://example.com/base/", "https://example.com/path", true},
	{"fragment removed", "/path#top", "https://example.com/", "https://example.com/path", true},
	// ...
}
for _, tt := range tests {
	t.Run(tt.name, func(t *testing.T) {
		got, ok := Resolve(tt.link, tt.baseURL)
		// compare with tt.want and tt.wantOK
	})
}
```

**Fuzzing.** `FuzzCanonicalize` feeds random strings to the URL normaliser and checks one property: normalising twice gives the same result as once (idempotence). `make fuzz` runs it for 30 seconds.

**Injected clocks.** Code that needs the time takes a `now func() time.Time`. A test hands it a variable it can advance, so time-based logic is tested without sleeping:

```go
// internal/politeness/politeness_test.go
clock := time.Now()
gate := NewMemory(func() time.Time { return clock })
// first call: no wait
clock = clock.Add(250 * time.Millisecond)
// second call: must wait 750ms
```

**Fakes behind interfaces.** `frontier.Memory`, `store.Memory`, fake fetchers and fake model providers replace real ones, so most tests need no network and no database.

**Contract tests.** One test function takes the interface and is run against both implementations, so `Memory` and `Postgres` cannot drift.

**A real database per test.** `internal/testutil.Postgres(t)` creates a fresh schema named `t_<nanoseconds>`, runs the migrations in it, and drops it when the test ends. If `TEST_DATABASE_URL` is not set, the test is skipped. Tests that create and drop schemas refuse a URL that is not local (`IsLocalURL`: localhost, 127.0.0.1 or host `postgres`).

**HTTP tests.** `net/http/httptest` starts a real server on a random port (`httptest.NewServer`) or records a handler's reply in memory (`httptest.NewRecorder`).

**A synthetic website.** `internal/fakesite` is a deterministic fake news site with optional chaos (flaky, rate-limited, redirecting, huge and non-HTML responses). The crawler's end-to-end tests crawl it.

**End-to-end tests.** `internal/e2e` builds the real `crawlerx` binary with the race detector and runs it as a subprocess. One test kills it with `kill -9` and checks the restart does not re-crawl; another sends SIGTERM and checks leases are released. `TestSIGTERMReleasesLeasesAndExitsCleanly` is known to fail on a clean checkout (recorded in `CLAUDE.md`), and it can also be flaky in the full suite while passing alone.

**Commands:**

```sh
go test -short -race ./...                      # no database needed; database tests skip
TEST_DATABASE_URL=postgres://... make test      # full suite, with -race and -p 1
go test -race ./internal/store -run TestPostgresDigestFiltersByCategoryAndBackfillsIt
make lint                                       # go vet and staticcheck
make fuzz                                       # 30 seconds of fuzzing
```

`-race` turns on Go's data-race detector. `-p 1` runs one package at a time because the integration packages share a database.

---

# Part 4. Package-by-package reference

Each entry says what a package is for, its main types and functions, how it works, and what is easy to get wrong. File references are to the repository.

## 19. The crawl core

### `internal/config`
Flags and environment, covered in chapter 16. `Config` is one large struct. `Validate` returns every problem joined, including: `LEASE` must be longer than twice `TIMEOUT`; `CRAWL_TIMEOUT` at least 1 minute; `DIGEST_AT` must be `HH:MM`; the time zone must be a valid IANA name; `SUMMARY_BATCH` 1 to 20; `SUMMARY_WORKERS` 1 to 8; `SUMMARY_WINDOW_DAYS` 1 to 30; `MaxDepth` 0 to 10; Telegram chat ids must look like `@name` or a number, and a Telegram chat needs a bot token. `ParsePlans` turns `"9900:31,24900:92"` (paise and days) into Pro plans and rejects duplicates and non-positive values.

### `internal/urlnorm`
Turns any URL into one canonical form so the same page is never queued twice: lower-case scheme and host, default ports and credentials and fragments removed, dot segments resolved, tracking parameters (`utm_*`, `gclid`, `fbclid` and similar) dropped, and the query sorted. `Resolve(link, base)` resolves a relative link and rejects non-http schemes. A fuzz test proves canonicalising twice changes nothing.

### `internal/fetch`
The polite HTTP client. Key points:

- **It never follows redirects itself.** `CheckRedirect` returns `http.ErrUseLastResponse`. The crawler enqueues the redirect target one level deeper, so robots rules, the politeness delay, scope and depth apply to every hop.
- **Non-2xx is a result, not an error.** Only transport failures and non-HTML bodies are errors (`ErrUnsupportedContentType`).
- **The body is capped** with `io.ReadAll(io.LimitReader(resp.Body, max+1))`. Reading one byte past the cap makes truncation detectable.
- **Retry policy** (`retry.go`): `ShouldRetryStatus` (429 and 5xx), `ParseRetryAfter` (seconds or an HTTP date, capped at an hour), and `Backoff.Delay`, exponential with full jitter and an overflow guard. The random source is injectable so tests are deterministic.
- The user agent is configurable (`USER_AGENT`) and identifies the crawler honestly.

### `internal/robots`
Implements the robots.txt rules from RFC 9309: a 2xx response is parsed; any 4xx means "no restrictions"; an unparsable file means "disallow everything" for a short error period; up to five redirects are followed. Results are cached (24 hours; five minutes for errors). A mutex guards the cache and `singleflight` ensures one fetch per origin however many workers ask. `Decision` carries `Allowed` and the site's `CrawlDelay`, which the crawler honours when it is longer than the configured host delay. A 5xx is not a verdict: the check returns `robots.ErrUnavailable` (remembered for `ErrorTTL`, 5 minutes), and the crawler retries the URL after `RobotsRetry` (2 minutes) with its normal retry budget, so a site whose robots.txt is down for a few minutes loses nothing (the URL is `dead` only if it stays down). A `Crawl-delay` over 30 seconds (`maxCrawlDelay`) leaves the site's URLs `blocked` for the run instead of holding the run.

### `internal/politeness`
The per-host gate. `Acquire(ctx, host, delay)` never sleeps: it returns zero (go ahead) or how long to wait, and the crawler reschedules the URL rather than blocking a worker. `Penalize` pushes a host's next slot out when a server sends `Retry-After`. The only implementation is in memory and per process, so the rule is one crawler process per database.

### `internal/parse`
Turns HTML into a `Page` using `golang.org/x/net/html`, in one pass over the tree. It extracts title, description, first heading, a text snippet, the links, `noindex` and `nofollow` flags, the main image, the article text (capped at 8,000 characters), a publish date and whether the page is an article.

Rules that matter:

- **The published date** is tried from, in order: page metadata, JSON-LD, a date in the URL, and a bare `<time>` element. A date more than a day in the future is discarded.
- **`IsArticle` needs its own date** (from metadata, JSON-LD or the URL), or the path must fall under the source's `article_prefixes`. A bare `og:type=article` is not enough, because WordPress sets it on tag and listing pages. This is why listing and tag pages stay out of the digest.
- **Live blogs** (`LooksLikeLiveBlog`) and **soft 404s** (`LooksLikeNotFound`, a "not found" page served with status 200) are detected and are not articles.
- **`split_articles` sources** put several topics on one page; each `<article id>` becomes its own item addressed as `pageURL#ID`.

### `internal/frontier`
The queue and the visited set in one table (chapter 31 explains the SQL). `CrawlSpec` carries a crawl's rules: `Seed`, `MaxDepth`, `AllowedHosts`, `PathPrefixes`, `ArticlePrefixes`, `SplitArticles`, `ImageFromSource` and `Category`. Path prefixes may begin with `!` to exclude a section. `Memory` is the in-process twin used in tests, with an injectable clock.

A row's life: `pending` → `leased` (a worker holds it until `lease_until`) → a terminal status: `done`, `failed`, `blocked` (robots said no), `skipped` (not HTML) or `dead` (retry budget exhausted). A leased row whose lease expired can be claimed again, which counts as a failed attempt. That is the crash recovery.

### `internal/crawler`
The worker loop. Each worker repeats: claim a batch of due URLs, then for each one, `process`:

1. Load the crawl's rules (cached).
2. If the URL has failed too many times, mark it `dead`.
3. Check the per-host in-flight cap (`HOST_INFLIGHT`, default 2). If the host is busy, reschedule without counting an attempt.
4. Check robots.txt. Disallowed means `blocked`.
5. Ask the politeness gate. If it says wait, reschedule.
6. Fetch.
7. Act on the status:

```go
switch {
case res.Status == 304:
	return c.finish(ctx, it, frontier.Done, "not modified")
case res.Status >= 200 && res.Status < 300:
	return c.handlePage(ctx, crawl, it, res)
case res.Status >= 300 && res.Status < 400:
	return c.handleRedirect(ctx, crawl, it, res)
case fetch.ShouldRetryStatus(res.Status):
	// the server asked us to back off: pause the whole host, not just this URL
	return c.transient(ctx, it, fmt.Sprintf("HTTP %d", res.Status), res.RetryAfter)
default:
	return c.finish(ctx, it, frontier.Failed, fmt.Sprintf("HTTP %d", res.Status))
}
```

`handlePage` parses the page, saves it through the store, and enqueues the links that are in scope and under the depth limit. `transient` reschedules with jittered backoff and marks the URL `dead` after `MAX_ATTEMPTS`. A URL is released back to the queue if a worker fails for any reason other than finishing, so nothing stays leased unless the whole process was killed.

A bug worth knowing about: an older design used a bounded channel as the queue, and when every worker was blocked sending links into a full channel, the crawl deadlocked. Moving the queue into Postgres removed the problem, and `deadlock_test.go` keeps a regression test (each page links to 150 pages, with a 15 second guard).

## 20. The scheduler and sources

### `sources.json`
An array of entries. Fields (validated in `internal/scheduler/sources.go`):

| Field | Meaning |
|---|---|
| `name` | Label; defaults to the seed host. |
| `category` | Lowercase slug matching `^[a-z0-9-]{1,24}$`. Stored on crawls and pages. Empty shows as `other`. Every shipped entry has one (a test enforces it). |
| `note` | Free text, ignored by the crawler; holds the robots.txt AI-policy remark shown on `/sources`. |
| `seed` | The listing page to start from. May contain `{date:LAYOUT}` (a Go time layout) for date-based archives. |
| `max_depth` | 0 to 10, default 1. A pointer in Go, so "unset" differs from 0. |
| `allowed_hosts` | Defaults to the seed's host. |
| `path_prefixes` | Keep the crawl inside one section; a leading `!` excludes. |
| `article_prefixes` | Paths that count as articles even with no date. |
| `split_articles` | One item per `<article id>` topic. |
| `image_from_source` | Fetch the picture from the page the article cites. |
| `days_back` | 0 to 30; needs a `{date:}` placeholder in the seed. |

There are 57 entries on 55 hosts (tech 36, world 6, ai 4, india 2, upsc 2, editorial 2, funding 5). The `catalog/` folder holds spare entries that are not crawled. Tests require every entry to be valid, require Indian Express to stay, and require every host to have a publisher type.

### The daily run
`Scheduler.Run` waits until `DIGEST_AT` (default 06:00 in the configured time zone), or runs at once with `RUN_ON_START`, or runs a catch-up if today's time has already passed. To make sure only one process runs the job per day, it first **claims** the day:

```go
// internal/scheduler/claims.go (shape)
// INSERT INTO job_runs ... ON CONFLICT DO NOTHING; the caller won if one row was inserted
```

Only the process whose insert succeeds (`RowsAffected() == 1`) proceeds. The step order is chapter 6. Two parts deserve detail:

- **`waitForCrawls`** polls the frontier's stats on a ticker, under a `CrawlTimeout` (default two hours). On each tick it calls a callback that fires an enrichment pass when a full batch of articles is waiting.
- **`enrich`** runs the image lookup in one goroutine and the summary pass in another, joined with a `WaitGroup`. They share no state.

`sourcetype.For(host)` labels each host as official, think-tank, news, analysis, company or project, shown next to the byline.

## 21. The store and the database layer

### `internal/db`
`Open` creates a `pgxpool` with `Workers+8` connections and pings it. `Migrate` runs the embedded goose migrations through a `database/sql` bridge, using a Postgres session advisory lock so several processes can start at once and only one migrates. Use a direct Neon connection (not `-pooler`).

### `internal/store`
The `Store` interface has about 45 methods in groups: pages and digest (`Save`, `Digest`, `Item`, `DigestSources`, `Search`, `ImageURL`, `Validators`); the pipeline passes, each a pending and set pair (`PendingSummaries` and `SetSummary`, `PendingImages` and `SetImage`, `PendingEnrichment` and `SetEnrichment`, `PendingDetail` and `SetDetail`); grouping (`GroupCandidates`, `SetStories`); subscribers (`Subscribe`, `Confirm`, `Unsubscribe`, `SetPreferences`, `ConfirmedSubscribers`, `DeliveredOn`, `MarkDelivered`); channel posts; quizzes and intros; Pro (`ProExtend`, `ProByToken`, `ActiveProTokens`); flashcards (`Cards`); compilations; concepts; the entity graph (`EntityStories`); and Teams hooks.

Important behaviours in `postgres.go`:

- **Group-commit writer** (chapter 14): `Save` is cheap to call from many workers.
- **Skip-if-unchanged upsert.** Re-crawling an unchanged page rewrites nothing, so the full-text index is not touched. The row is rewritten only when `content_hash`, `category` or `published_at` changed. That is why editing a source's category in `sources.json` relabels old pages and a parser date fix heals old rows. `first_seen_at`, the summary and the enrichment columns are never overwritten by a re-crawl.
- **`categorySQL`** is `COALESCE(NULLIF(category, ''), 'other')` and **`hostSQL`** derives a page's host from its URL. `hostSQL` is NULL-safe on purpose: a URL that is not http once made `/digest` return 500.
- **The digest filter** (`digestWhere`) takes positional parameters: since, a search query, a host, "articles only", a category, and an optional upper bound. Pages dated more than a day in the future are dropped. Order is newest first by `COALESCE(published_at, first_seen_at)`.
- **Pagination** puts `count(*) OVER()` on every row so the total comes free; if a page is past the end and returns no rows, a separate `count(*)` runs.
- **Search** uses `websearch_to_tsquery('english', ...)` against a generated, weighted `tsvector` column with a GIN index (title and first heading weigh most).
- **Which picture is shown.** `genericCTE` finds image URLs shared by five or more distinct articles (site logos). `effectiveImageSQL` prefers the page's own image unless it is generic, then the body image unless generic, else none. The digest and the image proxy use the same SQL, so the proxy serves exactly what the digest advertises.
- **`ProExtend`** (chapter 29) is one transaction and idempotent on the payment id.

`memory.go` is the in-process twin. Search there is a naive substring match, not full text. Everything else must match Postgres; that is what the contract tests check.

## 22. Summaries and enrichment (`internal/summarize`)

**The gist.** The `Summarizer` interface turns an article into up to five bullets. `Extractive` needs no key: it uses the description and whole sentences from the article (sentence splitting is in `prose`). `Gemini` batches several articles per request.

**`Pass`** is the fan-out shown in chapter 14. Details: articles with too little text (under `MinLLMRunes`, 400) skip the model and get an extractive gist; a failed request falls back to extractive gists; a rate-limit error cancels the rest, leaves them pending and returns `ErrRateLimited`.

**The Gemini client** (`gemini.go`) enforces a minimum interval between requests (5 seconds, about 12 a minute), retries up to three times, honours the server's retry hint (capped at 90 seconds), and after three tries returns an error that wraps `ErrRateLimited`. It sends the key in a header, never echoes a response body into an error, and every prompt tells the model that article text is data and any instructions inside it must be ignored. Model output is parsed defensively: scores are clamped, tags validated against a pattern, entities and concepts de-duplicated, and a malformed answer is dropped instead of stored.

**Enrichment, two stages** (`enrich.go`):

- **Stage 1** (`EnrichPass`): for summarised articles in the last 48 hours (limit 60 per run), in batches of 5: a one-line "why it matters", tags, an importance score and named entities.
- **Stage 2** (`DetailPass`): for only the 5 most important stories per run, in batches of 3: a long summary (about 130 words), up to two concepts, and, for UPSC categories only, the exam lens (relevance scores 0 to 3 for Prelims, Mains, Essay and Ethics, plus both sides, facts and an answer skeleton) and flashcards. Concepts are queued for the explainer.

**Concepts, intro, quiz.**

- `ConceptPass` fills the glossary: one request writes up to 12 explainers for newly queued terms.
- `IntroPass` writes "Today in 60 seconds" per track from the top six headlines (it needs at least three stories and caps the text at 90 words).
- `QuizPass` writes five questions per track from up to eight articles (it needs at least four). Only well-formed questions are kept: exactly four options and an answer index of 0 to 3, and at least three usable questions.

All passes share the same shape: a sentinel `ErrRateLimited`, a per-track failure that is logged without stopping the others, and a count returned.

## 23. Content helpers

- **`group`** finds stories several outlets cover. It compares headlines with no model. Two stories join when they come from different hosts, were published within 72 hours of each other, and share at least three distinctive words with either at least half of the shorter headline's words or a Jaccard similarity of at least 0.45. Joins are transitive (union-find), so a group is a connected cluster. A group needs at least two hosts. Roundup headlines (briefing, roundup, newsletter, "as it happened", live blog or live updates) are never grouped, because such a headline borrows one topic of the day and another outlet's story on it would match by coincidence. The comparison is quadratic, so the scheduler caps it at 1,500 headlines from the last four days.
- **`kind`** labels a story opinion, analysis or explainer from words in its URL path (`/opinion/`, `/editorial/`, `/explained/`, ...), matched on whole path segments. Opinion beats explainer beats analysis.
- **`graph`** builds the "who is in the news" graph: entities are counted across stories (case-folded, first spelling kept), linked when named in the same story, and trimmed to the top entities, a few stories each and a few related names. Links are capped at 120.
- **`compile`** merges a day's important stories (importance at least 3, with a URL and a title) into the monthly compilation by syllabus area, at most 12 per area, using the first tag as the area. It uses no model and the scheduler persists the result in its own table, so it outlives the pages.
- **`prose`** splits text into sentences, handling abbreviations, initials and the Devanagari danda; drops boilerplate; and provides `Lead`, the first whole sentences of an article, which the story page shows when no good summary exists.
- **`track`** defines the two editions and their categories, the reading modes (`1min`, `7min`) and `NormalizeTopics` (at most five topics, 40 characters each, lower case, no duplicates).

## 24. Delivery: mail, channels, images

- **`mail`**: a `Sender` interface with a `Log` implementation (prints the text; the default) and a `Brevo` implementation over HTTPS (not SMTP, because Render blocks SMTP). Confirmation and Pro welcome emails are sent by a goroutine on a detached context with a 20 second timeout, so a slow provider never delays the web response.
- **`digestmail`**: `Run` mails every confirmed subscriber once per day, Pro members first. It uses the `deliveries` table so a rerun sends nothing twice, stops at the `-max-sends` limit of `senddigest` (to stay under Brevo's daily limit), stops after five consecutive failures, and pauses between sends. Each distinct combination of editions and reading mode is built once and reused. Every message carries `List-Unsubscribe` and `List-Unsubscribe-Post: List-Unsubscribe=One-Click` headers. `RunChannels` posts to Telegram and `RunTeams` to Slack and Discord, both idempotent through `channel_posts`. A webhook that answers 404 or 410 is removed.
- **`telegram`**: a `Poster` over the Bot API. The bot token is part of the URL, so errors are replaced by generic text.
- **`chat`**: Slack and Discord webhook posting with a strict address allow-list (chapter 28).
- **`images`**: two jobs. First, a pass that finds a picture for articles whose image comes from a cited source page, following up to three redirects, honouring robots.txt, and giving up on a host after it refuses twice in one run. Second, the `/img/{id}` proxy (chapter 28), with an SSRF-safe dialer and an LRU cache bounded in bytes.

## 25. Features and where they live

| Feature | Backend | Frontend |
|---|---|---|
| Front page, categories, search | `/digest`, `/digest/sources`, `/search`; `store.Digest` | `web/app/page.tsx`, `components/edition.tsx`, `views.tsx` |
| Story page | `/story/{id}`: item plus `also`, `exam`, `entities`, `lead` | `web/app/story/[id]/page.tsx` |
| Exam lens | stage 2 enrichment; `exam` JSON and `rel_*` columns | `components/exam-lens.tsx`, relevance chips on cards |
| "Today in 60 seconds" | `IntroPass`, `/intro` | `components/briefing.tsx` |
| Quiz and replay | `QuizPass`, `/quiz` | `web/app/quiz`, `quiz-player.tsx`, `revise.tsx` |
| Flashcards | `/cards` from the exam `cards` JSON | `web/app/flashcards`, `components/flashcards.tsx` (Leitner boxes in localStorage) |
| Notebook | none (browser only) | `web/app/notebook`, `save-note.tsx` |
| E-paper | `/digest`, `/intro` | `web/app/epaper` (print stylesheet) |
| Concepts | `ConceptPass`, `/concepts` | `web/app/concepts`; story page matches terms in the story text |
| Who is in the news | stage 1 entities, `graph.Build`, `/entities` | `web/app/graph` (server-drawn SVG), `web/app/entity/[name]` |
| Weekly revision and monthly compilations | `compile.Merge`, `/month`, `/months` | `web/app/week`, `web/app/month` (printable) |
| Startup funding | `funding` category sources; no model | `web/app/funding`, `web/lib/funding.ts` (rules over headlines) |
| Underreported | `story_id` and `coverage` columns | `web/app/underreported` |
| Archive | `/digest?day=` | `web/app/archive` |
| Daily email | `senddigest`, `digestmail`, Brevo | `web/app/confirm`, `unsubscribe`, `components/subscribe-form.tsx` |
| Telegram | `RunChannels`; the bot in `edge/telegram.js` | none |
| Slack and Discord | `/teams`, `chat`, `RunTeams` | `web/app/teams` |
| Pro | `/billing/razorpay`, `/pro`, `pro_members` | `web/app/pro`, `web/app/pro/edition` |
| RSS and sitemap | none | `web/app/rss.xml`, `sitemap.ts`, `robots.ts` |
| Sources and takedown | `BLOCKED_HOSTS`, `sources.json` notes | `web/app/sources` (reads the published `sources.json`) |

The removed **Upcoming calendar** (a page, an iCalendar feed, an extraction step, an API route and a snapshot file) is documented in `FEATURES.md` as deprecated. Its `events` table was left in place and is not used.

---

# Part 5. The HTTP API in Go

## 26. net/http basics, shown with this project

Go's standard library includes a production-grade HTTP server. You need no framework. These are the pieces `internal/api` uses.

**A handler** is a function with this signature:

```go
func (s *server) digest(w http.ResponseWriter, r *http.Request)
```

`r` is the request (method, URL, headers, body, context). `w` is how you write the response: set headers, call `WriteHeader(status)`, then write the body. Headers must be set *before* `WriteHeader`; after that they are ignored.

**A router (`http.ServeMux`)** maps request patterns to handlers. Since Go 1.22 a pattern can include the HTTP method and path wildcards:

```go
// internal/api/api.go (excerpt)
mux := http.NewServeMux()
if !deps.ReadOnly {
	mux.HandleFunc("POST /crawls", s.createCrawl)
}
mux.HandleFunc("GET /crawls", s.listCrawls)
mux.HandleFunc("GET /crawls/{id}/depths", s.crawlDepths)
mux.HandleFunc("GET /crawls/{id}", s.getCrawl)
mux.HandleFunc("GET /story/{id}", s.story)
mux.Handle("GET /img/{id}", deps.Images)   // a value that implements http.Handler
```

Read a wildcard with `r.PathValue("id")`:

```go
id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
```

Behaviours of `ServeMux` the project relies on:

- **The most specific pattern wins**, regardless of the order you registered them. `"GET /"` is a catch-all for everything not matched, and `"GET /digest.html"` still beats it.
- **A path that matches a pattern but with the wrong method gets an automatic `405 Method Not Allowed`** with an `Allow` header. That is how a read-only server hides `POST /crawls`: the route is simply not registered, and `TestReadOnlyHidesCreateCrawl` asserts the 405.
- Registering `GET` also registers `HEAD`.
- Routes can be registered conditionally. Here `POST /crawls` needs the server not to be read-only, `GET /img/{id}` needs an image proxy, `/teams` needs `TEAM_POSTS=true`, and `/billing/razorpay` needs a webhook secret.

**A middleware chain does not exist.** The handler returned by `NewHandler` is the `ServeMux` itself. There is no authentication, CORS layer or access log in Go; handlers log errors themselves. Security headers are set only by the image proxy. The edge Worker and the Next.js site handle the public-facing concerns. The package comment says plainly that the API has no authentication and should run on a private network.

**JSON in and out** are two tiny helpers:

```go
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
```

Every error is `{"error": "message"}`. One trap handled in several places: a nil slice encodes as `null`, not `[]`, so the code converts nil to an empty slice before encoding (`hits`, `months`, and others).

**Decoding a request body safely.** Cap its size, and refuse unknown fields so a typo in a request is an error:

```go
r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)   // 1 MiB for crawls
dec := json.NewDecoder(r.Body)
dec.DisallowUnknownFields()
if err := dec.Decode(&req); err != nil {
	writeError(w, 400, "invalid JSON body: "+err.Error())
	return
}
```

Body caps are 1 MiB for crawls, 4 KiB for subscribe, confirm and unsubscribe, 2 KiB for Teams and 64 KiB for the billing webhook.

**Context.** `r.Context()` is cancelled when the client disconnects, so it is passed to every store call. Two deliberate exceptions use `context.WithoutCancel(r.Context())`: sending the confirmation email and the shared image fetch, so a client leaving does not abort work others depend on.

**Dependency injection.** `NewHandler(deps Deps)` receives everything the handlers need (the frontier, the store, the image proxy, mail notifiers, secrets) in one struct, embedded in a private `server` type. Handlers are method values like `s.digest`. In tests the same `Deps` is filled with in-memory implementations.

**Caching headers.** Read-only endpoints that change once a day send `Cache-Control: public, max-age=300`. `GET /pro` sends `no-store`. `/crawls`, `/search` and the write endpoints send none.

**Embedded pages.** The server also serves two HTML pages from the binary (`/` and `/digest.html` for the original front page, `/dashboard.html` for the crawler dashboard). `/digest.html` is the Render health-check path.

**Server timeouts.** `cmd/crawlerx/main.go` builds the `http.Server` with `ReadHeaderTimeout` 10 seconds, `ReadTimeout` 30 seconds and `WriteTimeout` 60 seconds, so slow clients cannot hold connections open forever. Shutdown is graceful (5 seconds). Profiling (`net/http/pprof`) runs on a separate address (`PPROF_ADDR`) and is off by default.

**Testing a handler.** Two ways, both in the repo:

```go
// 1. A real server on a random port, as in the main API tests (shape only)
srv := httptest.NewServer(NewHandler(Deps{ /* in-memory frontier and store */ }))
t.Cleanup(srv.Close)

// 2. Call a handler directly and record the answer (shape; see internal/images/proxy_test.go)
req := httptest.NewRequest("GET", "/img/x?url=http://169.254.169.254/", nil)
req.SetPathValue("id", "x")          // wildcards must be set by hand when bypassing the mux
rec := httptest.NewRecorder()
proxy.ServeHTTP(rec, req)
```

## 27. API reference

Base URL: the Render origin directly, or (for the public site) the Cloudflare Worker, which answers some routes from static files. All responses are JSON unless noted. Errors are `{"error": "..."}`.

### Crawler and search

| Method and path | Purpose | Parameters and limits | Success | Errors |
|---|---|---|---|---|
| `POST /crawls` | Start a crawl. Absent unless `READ_ONLY=false`. | Body: `seed` (absolute http(s) URL, canonicalised), `max_depth` 0 to 10 (default 2), `allowed_hosts`, `path_prefixes`, `article_prefixes`, `split_articles`, `image_from_source`. Unknown fields rejected; body at most 1 MiB. | `201`, `Location: /crawls/{id}`, the crawl | 400, 500 |
| `GET /crawls` | The 20 newest crawls. | none | `200 {"crawls": [...]}` | 500 |
| `GET /crawls/{id}` | One crawl with its counts. | `id` a positive integer | `200` crawl, `stats`, `total`, `done` | 400, 404, 500 |
| `GET /crawls/{id}/depths` | Counts per depth and status. | same | `200 {"depths": [...]}` | 400, 404, 500 |
| `GET /search` | Full-text search. | `q` required; `page`, `page_size` (default 20, max 100) | `200 {query, hits, page, page_size, total, total_pages}` | 400, 500 |

`done` on a crawl is true when it has URLs and none are `pending` or `leased`.

### The digest

| Method and path | Purpose | Parameters and limits |
|---|---|---|
| `GET /digest` | Articles, newest first. | `days` 1 to 30 (default 1); `day=YYYY-MM-DD` (one edition day, India time); `source` (host); `category`; `q` (full text); `articles_only` (default true; only the literal `false` turns it off); `detail=1` (include the long summary); `page`, `page_size` (default 24, max 200). |
| `GET /digest/sources` | Hosts in the window with item counts, for filters. | same filters, no paging |
| `GET /story/{id}` | One story with `source_type`, `kind`, `also`, `exam`, `entities`, `lead`. | `id` 1 to 128 characters; 404 if unknown |
| `GET /quiz` | A track's quiz. | `track` (`upsc` or `engineering`); optional `day`. 404 if none yet. |
| `GET /intro` | "Today in 60 seconds". | same |
| `GET /cards` | Flashcards. | `days` 1 to 30 (default 14); `limit` 1 to 300 (default 120) |
| `GET /month` | A monthly compilation. | `month=YYYY-MM`, `track` |
| `GET /months` | Months that have compilations, newest first. | none |
| `GET /concepts` | The glossary (explained terms only, at most 500). | none |
| `GET /entities` | The knowledge graph. | `days` 1 to 60 (default 14); `limit` 1 to 300 (default 150) |

All of these send `Cache-Control: public, max-age=300`. `source_type` is filled from `sourcetype.For`, `kind` from the URL path, and when an image proxy is mounted every item's `image` is rewritten to `/img/<id>` (an empty string when it has none).

Example request and the shape of the answer:

```
GET /digest?days=1&category=tech&page=1&page_size=2
```

```json
{
  "since": "2026-11-14T10:00:00Z",
  "page": 1, "page_size": 2, "total": 37, "total_pages": 19,
  "items": [
    {
      "id": "3f9c...a1",
      "url": "https://example.com/blog/post",
      "title": "How the new scheduler works",
      "description": "…", "snippet": "…",
      "source": "example.com",
      "image": "/img/3f9c...a1",
      "summary": ["First whole sentence.", "Second whole sentence."],
      "is_article": true,
      "time": "2026-11-15T07:30:00Z",
      "category": "tech",
      "why": "One line on why it matters.",
      "tags": ["scheduling"],
      "importance": 4,
      "source_type": "company",
      "kind": "analysis",
      "coverage": 2,
      "entities": ["Example Corp"]
    }
  ]
}
```

`why`, `tags`, `importance` and the fields after it are omitted when empty. `long` appears only with `detail=1`. `rel` and `exam` appear only for UPSC stories that got stage 2 enrichment. `GET /story/{id}` returns the same item plus `also` (other outlets on the same story: `id`, `title`, `source`, `source_type`) and `lead`.

A quiz answer:

```json
{
  "day": "2026-11-15", "track": "upsc",
  "questions": [
    { "question": "…", "options": ["A", "B", "C", "D"], "answer": 2,
      "explanation": "…", "url": "https://…" }
  ]
}
```

A flashcard answer is `{"cards": [{"id": "<storyID>:<position>", "q": "…", "a": "…", "story": "<id>", "title": "…"}]}`. A monthly compilation is `{"month": "2026-11", "track": "upsc", "areas": [{"name": "…", "items": [{id, url, title, source, why, bullets, tags, importance, time}]}], "built": "…"}`. The graph is `{"entities": [{"name", "count", "stories": [...], "related": [{"name", "count"}]}], "links": [{"a", "b", "count"}]}`.

### Readers, teams and billing

| Method and path | Purpose | Notes |
|---|---|---|
| `POST /subscribe` | Start a subscription. | Body `{email, tracks[], mode, topics[]}`. Always `202 {"status":"check your inbox to confirm"}` for a valid request. 400 for bad email, unknown edition or bad mode; 429 when rate limited. |
| `POST /confirm` | Confirm by token. | Body `{token}`. 200, 400, 404. |
| `POST /unsubscribe` | Unsubscribe. | Token from `?token=` or the body (supports one-click unsubscribe). 200, 400, 404. |
| `GET /pro` | Is this Pro token active? | `?token=`; returns only `{active, until}`; `no-store`. |
| `POST /teams` | Connect a Slack or Discord channel. | Only if `TEAM_POSTS=true`. Body `{url, track}`. 202, 400, 429. |
| `POST /teams/stop` | Disconnect a channel. | Body `{token}`. 200, 404. |
| `POST /billing/razorpay` | Razorpay webhook. | Only if `RAZORPAY_WEBHOOK_SECRET` is set. Signed. |
| `GET /img/{id}` | The article picture, from our own origin. | Only if the image proxy is on. |
| `GET /healthz` | Liveness and database check. | `200 {"status":"ok"}` or `503` with the failing check. |

## 28. Security model

**No authentication, by design.** The API assumes a private network, or a public deployment with `READ_ONLY=true` in front of a CDN. `READ_ONLY` is on by default. Anyone who can reach a server started with `READ_ONLY=false` can call `POST /crawls`, and the server will fetch URLs they choose, so never expose that to the internet. Reads are public and cacheable.

**Anti-enumeration.** `POST /subscribe` returns the same 202 and message whether the address is new, pending or already confirmed, so the endpoint cannot be used to find out who subscribes. Preferences are written only while an address is still unconfirmed, so a signup form cannot change what a confirmed reader receives.

**Confirm and unsubscribe are button presses.** The link in the email goes to a *web page*; the page asks the reader to press a button, which then sends `POST /confirm`. Mail scanners that open every link in an email would otherwise confirm or unsubscribe people by accident. The email also carries `List-Unsubscribe` headers for one-click unsubscribe.

**Rate limiting.** One small limiter (a mutex-guarded counter over a one minute window, 30 requests) protects signups, and another instance protects Teams. It is global to the process, not per client, because the web app calls from one server and client addresses are not visible. A comment in the code marks this as a known shortcut. Over the limit returns 429 with `Retry-After: 60`. The limiter runs after validation, so malformed requests do not use up the quota. Nothing else is rate limited.

**Webhook signatures (Razorpay).** The handler computes an HMAC-SHA256 of the raw request body with the shared secret and compares it to the `X-Razorpay-Signature` header in constant time:

```go
mac := hmac.New(sha256.New, []byte(s.RazorpaySecret))
mac.Write(body)
want := hex.EncodeToString(mac.Sum(nil))
if !hmac.Equal([]byte(want), []byte(strings.ToLower(r.Header.Get("X-Razorpay-Signature")))) {
	// 400 "bad signature"
}
```

The signature is checked over the raw bytes *before* any JSON is parsed. There is no timestamp check; replays are harmless because each payment id counts once (chapter 29).

**Outgoing webhook allow-list (Slack and Discord).** A team pastes a webhook address, and the server will send requests to it, so the address is validated strictly by `chat.Kind`: https only; no credentials, query, fragment or port; at most 300 characters; and exactly `hooks.slack.com/services/...` or `discord.com/api/webhooks/...` (also `discordapp.com`) with the right path shape. The client never follows redirects, and errors never contain the address, because it is a secret. A test message is sent when a channel is connected, and the channel is saved only if it accepts that message.

**The image proxy (`internal/images/proxy.go`)** is not an open proxy:

- The image address is looked up **by article id in the database**, never taken from the request. A `?url=` parameter is ignored (a test checks it).
- The dialer rejects loopback, private, link-local, multicast and unspecified addresses **at connect time**, which also defeats DNS tricks that resolve to an internal address. This guards against server-side request forgery (SSRF).
- The upstream must answer 200. The body is capped at 5 MiB. The content type is **sniffed from the bytes**, not trusted from the header, and only JPEG, PNG, GIF, WebP and AVIF are allowed. SVG is excluded because it can carry script.
- Responses carry `X-Content-Type-Options: nosniff` and `Content-Security-Policy: default-src 'none'; sandbox`, an ETag and `Cache-Control: public, max-age=86400`.
- A 404 carries an `X-Image-Error` header with the reason (`upstream status 403`, `too large`, `not a supported image`, `timeout`, `fetch failed`), and the proxy logs one warning per fresh failure with the host and the reason. `build-snapshot.sh` prints the failures by site and reason.
- An in-memory LRU cache is bounded in bytes (`IMAGE_CACHE_MB`), `singleflight` collapses concurrent fetches of one URL, and a failed URL is remembered for 10 minutes.

**Secrets.** API keys and tokens are environment-only. Errors from clients that carry a secret in a URL or header (Telegram, Slack, Discord, mail, Gemini) are replaced with generic messages. The crawl job's `log` mail provider would print tokens into a public log, so the digest step runs only when a real provider or channel is configured.

**Crawl ethics.** The crawler identifies itself, obeys robots.txt (including `Crawl-delay`), limits requests per host, limits concurrent requests per host, and backs off on 429 and 5xx. Sites that block bots are not worked around. The public `/sources` page lists every site with its robots.txt AI-policy note, and `BLOCKED_HOSTS` removes a site at startup, from stored pages and from the public list.

## 29. Walkthroughs

### Subscribe, confirm, unsubscribe

1. The web form (a Next.js server action, `web/app/actions.ts`) posts `POST /subscribe` with the email, editions, reading mode and topics. A hidden "honeypot" field that real users leave empty makes bots fake-succeed.
2. `subscribe` validates the address (`net/mail.ParseAddress`, at most 254 bytes, a dot in the domain, lower-cased), the editions, the mode, and cleans the topics (never rejects them). It checks the rate limit.
3. `Store.Subscribe` runs one upsert: a new row gets a random 32-byte URL-safe token; an existing row keeps its token; a previously unsubscribed row gets a new token and restarts as unconfirmed.
4. If the address is unconfirmed, a goroutine sends a confirmation email containing a link to the web page `/confirm?token=...`. The handler has already replied `202`.
5. The reader presses the button on that page; the server action calls `POST /confirm`, which sets `confirmed_at`.
6. From then on `senddigest` includes the reader. Each email contains an unsubscribe link and the one-click headers; either calls `POST /unsubscribe`, which sets `unsubscribed_at`.

### A Razorpay payment grants Pro

1. Razorpay sends `POST /billing/razorpay` with a signature.
2. The handler checks the signature, then handles only `payment_link.paid` and `payment.captured`; other events get `200 ignored` so Razorpay does not retry.
3. It reads the payment id and the email (falling back to the payment link's customer email), and maps the amount in paise to a plan through `PRO_PLANS` (default `9900:31,24900:92`, meaning 99 rupees for 31 days and 249 rupees for 92 days). An unknown amount is logged and ignored.
4. `Store.ProExtend` runs one transaction: insert the payment with `ON CONFLICT DO NOTHING`; only if it was new, upsert the member and set `until = GREATEST(until, now()) + days`. Paying again while active stacks time. A repeat of the same webhook changes nothing, so retries are safe.
5. A store error returns 500, which makes Razorpay retry (the payment is not recorded yet). The welcome email is sent only for a new payment.

### Connect a Slack or Discord channel

The form posts `{url, track}` to `POST /teams`. The server validates the address (`chat.Kind`), the edition and the rate limit, generates a stop token, **sends a test message** (containing a stop link) and, only if the channel accepted it, saves the hook. The reply is `202` whether or not the address was already connected. The daily `senddigest` run posts to every hook through `RunTeams`, idempotent per day, and removes a hook that returns 404 or 410.

---

# Part 6. The database

## 30. Tables

The schema is built by 26 goose migrations in `internal/db/migrations`, embedded in the binary and applied at startup under an advisory lock. Each file has `-- +goose Up` and `-- +goose Down` sections.

```
crawls 1───* frontier            (ON DELETE CASCADE; crawls are pruned after 3 days)
crawls 1···* pages               (soft link by crawl_id, no foreign key: pages outlive their crawl)
pages  *···* pages               (grouped by story_id; entities in a text[] column)
subscribers 1───* deliveries     (idempotent daily mail)
pro_members 1···* pro_payments   (by email; payment_id is the key)
quizzes, intros, compilations, channel_posts, job_runs, concepts, team_hooks, events (standalone)
```

| Table | Key | Purpose and notable columns |
|---|---|---|
| `crawls` | `id` | One crawl job: `seed`, `max_depth`, `allowed_hosts[]`, `path_prefixes[]`, `article_prefixes[]`, `split_articles`, `image_from_source`, `category`, `created_at`. |
| `frontier` | `(crawl_id, url_hash)` | The queue and visited set: `url`, `host`, `depth`, `status` (checked), `attempts`, `next_fetch_at`, `lease_until`, `last_error`. Partial index on pending and leased rows by `(depth, next_fetch_at)` keeps claims fast; index on `host`. |
| `pages` | `url_hash` | The documents. Crawl fields (`url`, `status_code`, `title`, `description`, `h1`, `snippet`, `content`, `content_hash`, `etag`, `last_modified`, `fetched_at`...); dates (`published_at`, `first_seen_at`); `is_article`, `category`; images (`image_url`, `body_image_url`, `source_url`, `image_checked_at`); summary (`summary text[]`, `summarized_at`); enrichment (`why`, `tags`, `importance`, `long_summary`, `enriched_at`, `detailed_at`, `entities text[]`); exam (`rel_prelims`, `rel_mains`, `rel_essay`, `rel_ethics`, `exam jsonb`); grouping (`story_id`, `coverage`); and a generated weighted `search tsvector` with a GIN index. |
| `job_runs` | `(job, run_day)` | The once-a-day claim. |
| `subscribers` | `id`; unique `email`, unique `token` | Double opt-in: `confirmed_at`, `unsubscribed_at`; preferences `tracks[]`, `mode` (`1min` or `7min`), `topics[]`. Emails are stored lower-case (a `CHECK`). |
| `deliveries` | `(subscriber_id, edition_day)` | Which day's mail was sent to whom, so a rerun sends nothing twice. |
| `quizzes` | `(edition_day, track)` | `questions jsonb`. |
| `intros` | `(edition_day, track)` | The "Today in 60 seconds" text. |
| `channel_posts` | `(edition_day, channel)` | Telegram and Teams posted-once records. |
| `pro_members` | `email` | `token` (unique), `until`. |
| `pro_payments` | `payment_id` | Makes each payment count once. |
| `compilations` | `(month, track)` | `areas jsonb`, `built_at`. Outlives the pages. |
| `concepts` | `term_key` (lower-case) | `term`, `body`. An empty body means "queued for the explainer". |
| `team_hooks` | `id`; unique `url`, unique `token` | Slack or Discord channel: `kind`, `url` (a secret), `track`. |
| `events` | `(day, story_id, text_hash)` | **Unused.** Left behind by the removed Upcoming calendar. |

Migration notes: `00022` is a data migration (it re-queues recent summaries that ended in an ellipsis; its Down is a no-op). `00016` backfills existing rows as having had stage 2, so old stories are not re-processed. `00002` adds `pages_recent_idx` on `COALESCE(published_at, first_seen_at)`. `00007` adds the partial indexes used for logo detection.

## 31. SQL patterns explained

**Claiming work: `FOR UPDATE SKIP LOCKED` with a lease.** This one statement is the heart of the crawler. It selects due rows and leases them in the same step:

```sql
WITH picked AS (
    SELECT crawl_id, url_hash
    FROM frontier
    WHERE next_fetch_at <= now()
      AND (status = 'pending' OR (status = 'leased' AND lease_until < now()))
    ORDER BY depth, next_fetch_at
    LIMIT $1
    FOR UPDATE SKIP LOCKED
)
UPDATE frontier f
SET attempts    = f.attempts + CASE WHEN f.status = 'leased' THEN 1 ELSE 0 END,
    status      = 'leased',
    lease_until = now() + make_interval(secs => $2),
    updated_at  = now()
FROM picked
WHERE f.crawl_id = picked.crawl_id AND f.url_hash = picked.url_hash
RETURNING f.crawl_id, f.url_hash, f.url, f.host, f.depth, f.attempts
```

How to read it:

- `FOR UPDATE` locks the chosen rows so no one else changes them.
- `SKIP LOCKED` means "if another worker already locked a row, skip it instead of waiting". Many workers, even in different processes, can claim from the same table without blocking or taking the same URL.
- A row that is `leased` but whose `lease_until` has passed is claimable again. That is crash recovery: a worker that died simply never finishes, and the lease runs out (leases are not renewed, which is why `LEASE` must exceed twice `TIMEOUT`). Reclaiming counts as a failed attempt.
- `ORDER BY depth, next_fetch_at` makes the crawl breadth-first: shallow pages first.
- `Finish`, `Reschedule` and `Release` all add `AND status = 'leased'`, so a row that already finished or went back to pending is not changed by a late caller. They do not check who holds the lease: a slow worker whose lease expired and was reclaimed could still finish the new holder's row (a known gap).

A test (`TestPostgresTwoProcessesShareOneQueue`) runs two claimers against one database.

**Enqueue: insert many, dedupe, and lower the depth.** Links are inserted in one statement using `unnest` over arrays, with `ON CONFLICT (crawl_id, url_hash) DO UPDATE SET depth = EXCLUDED.depth WHERE status = 'pending' AND EXCLUDED.depth < frontier.depth`. So a URL rediscovered by a shorter path gets a lower depth, but only while it is still pending. `RETURNING (xmax = 0)` is a Postgres trick: `xmax` is zero for a freshly inserted row, so the count of true inserts is known. A Go helper (`dedupeShallowest`) removes duplicates from the batch first because one `INSERT ... ON CONFLICT DO UPDATE` may not touch a row twice.

**Skip-if-unchanged upsert.** Saving a page uses `INSERT ... ON CONFLICT (url_hash) DO UPDATE ... WHERE pages.content_hash IS DISTINCT FROM EXCLUDED.content_hash OR category differs OR published_at differs`. If nothing changed, Postgres does not rewrite the row, its text-search vector or its index entries.

**Once a day across processes.** `INSERT INTO job_runs (job, run_day) ... ON CONFLICT DO NOTHING`; the caller won if `RowsAffected() == 1`.

**Counting with the page.** `count(*) OVER()` is a window function that adds the total row count to every returned row, so one query gives both the page and the total.

**Idempotent payments.** `ProExtend` inserts the payment first and extends membership only if that insert was new (chapter 29).

**Why two implementations agree.** For every SQL filter in `digestWhere` there is Go code in `memory.go` doing the same thing, and the contract tests run the same cases against both. This is the one rule most likely to bite when changing `/digest` filtering, together with the matching JavaScript in the Worker.

---

# Part 7. The frontend and the edge

## 32. The Next.js site (`web/`)

**Stack.** Next.js 16 with React 19, the React Compiler and **Cache Components** turned on (`cacheComponents: true`, `partialPrefetching: true`). It is deployed on Vercel with `web` as the root directory. It has no test suite; checks are `npm run lint`, `npx tsc --noEmit` and `npm run build`. `web/AGENTS.md` warns that this Next.js version differs from older ones, so read the docs in `node_modules/next/dist/docs/` before changing framework-level code.

**All data goes through one helper.** `web/lib/api.ts` has `getJSON`, marked `"use cache"` with `cacheLife({ stale: 300, revalidate: 1800, expire: 86400 })`. The cache key is the path plus query. The base URL is `API_BASE_URL` (the Cloudflare Worker). Typed fetchers wrap it:

| Fetcher | API call |
|---|---|
| `getDigest` | `/digest?days&category&source&q&page&page_size&detail&day` |
| `getSources` | `/digest/sources` |
| `getStory` | `/story/{id}` |
| `getQuiz`, `getIntro` | `/quiz`, `/intro` with `track` and `day` |
| `getCards` | `/cards?days=14&limit=200` |
| `getConcepts` | `/concepts` |
| `getGraph` | `/entities?days=14&limit=150` |
| `getMonths`, `getMonth` | `/months`, `/month` |
| `getPublicSources` | the published `sources.json` |
| `getProStatus` | `/pro?token=` (no cache; a payment can activate a member at any moment) |

A fetch that fails throws, and `app/error.tsx` shows the error page; a missing resource (404) returns null where a page expects it. The 1-day and 2-day views are fetched as 7 days (`fetchDays`) so a quiet period still has stories.

**The request-time rule.** A page that depends on the clock or on request data must not be prerendered, or the build would bake in stale data and would need the API to be reachable. The pattern used everywhere is:

```tsx
async function Page() {
  await connection();          // opt this part out of prerendering
  const now = Date.now();
  const data = await getSomething();
  // ...
}
export default function Route() {
  return <Suspense fallback={null}><Page /></Suspense>;
}
```

A past deploy failed when a page prerendered a fetch that got a 429; that is why every page that calls the API reads at request time, and why builds were verified against an unreachable API.

**Routes.**

| Route | Shows |
|---|---|
| `/` | The front page. The URL carries `days`, `category`, `source`, `q`, `page`, `track`, `mode`; `components/edition.tsx` chooses the home view, a section view or search results. |
| `/story/[id]` | Headline, summary, why it matters, exam lens, concepts used, related stories, share, save-note, read-aloud, and the button to the publisher. Builds its own title and share tags. |
| `/quiz`, `/flashcards`, `/notebook` | Study tools. The notebook, flashcards and streak live in `localStorage`; nothing is sent to a server. |
| `/archive`, `/archive/[day]`, `/week`, `/month`, `/month/[month]`, `/epaper` | Archive of the last 14 days and printable revision pages. |
| `/funding` | Funding rounds of the last week as a table (company, round, amount, investors, outlets), parsed from headlines. |
| `/underreported`, `/concepts`, `/graph`, `/entity/[name]` | Discovery pages. The graph is drawn as SVG on the server. |
| `/sources` | Public source list with robots.txt policy and the takedown contact. |
| `/pro`, `/pro/edition` | Plans (links and prices from build-time environment variables) and the token-gated printable edition. |
| `/teams`, `/teams/stop`, `/confirm`, `/unsubscribe` | Forms that call server actions; token pages are `noindex`. |
| `/rss.xml`, `sitemap.ts`, `robots.ts` | Feeds and crawler files. |

**Server actions** (`web/app/actions.ts`) post to the API with a 10 second timeout and `no-store`. `subscribe` has a honeypot field. Confirm and unsubscribe are button presses in `components/confirm-form.tsx`.

**Components.** Server components render the pages (`edition.tsx`, `views.tsx`, `article.tsx`, `briefing.tsx`, `discover.tsx`, `exam-lens.tsx`, `graph-svg.tsx`, `site.tsx` for the masthead, section bar and footer). Client components handle interaction (`more-menu`, `theme-toggle`, `primary-nav`, `subscribe-form`, `quiz-player`, `flashcards`, `notebook`, `save-note`, `read-aloud`, `streak`, `revise`, `print-button`). The "More" menu sits outside the scrolling section list so its panel is not clipped, and closes on Escape, an outside click or following a link.

**Theming.** Colours, spacing, type sizes, radii and shadows are CSS custom properties in `app/globals.css`. The page opens in dark (`<html data-theme="dark">` in `layout.tsx`, whatever the system prefers); light, a cream newspaper palette, applies only for a reader who chose it with the switch. The dark tokens appear twice (once under the media query guarded by `:root:not([data-theme="light"])`, once under `:root[data-theme="dark"]`) and the two copies must be kept in step. An inline script in `layout.tsx` switches to light before first paint when `localStorage["pp-theme"]` is `light`, so a reader who chose light sees no flash. Fonts come from `next/font`. Print stylesheets force the light palette.

**Front-page balance.** `web/lib/balance.ts` estimates the pixel height of each card type (`heightLeft`, `heightRank`, `heightLatest`, `heightRow`, `heightMain`) and `fill` takes stories until a column's budget is used, so the three columns end level. These estimates must stay in step with the CSS of the cards.

**Browser storage keys:** `pp-theme` (theme), `pp-notebook` (saved sentences, at most 300), `pp-cards` (flashcard boxes; gaps of 0, 1, 3, 7, 14 and 30 days; a wrong answer is due again in 10 minutes), `pp-streak` (date and count), `pp-missed` (wrong quiz questions, at most 30). All reads and writes are wrapped so private browsing cannot break the page.

**Pictures** are plain `<img>` elements pointing at `/img/{id}`, not `next/image`. Cards show a tile with the site name underneath, so a missing or failing picture leaves a tile, not an empty space. Pictures keep their natural proportions.

**Safety rule for the old static page and the new site alike:** never put feed data into HTML as raw markup. The legacy `index.html` builds the DOM with `textContent` only and passes every link through `safeURL`; the Next.js site relies on React escaping and `safeURL` for links.

## 33. The Cloudflare Worker (`edge/`)

`edge/worker.js` is configured by `edge/wrangler.toml`: variables `ORIGIN` (the Render URL) and `WEB_URL`, the static assets directory `./assets` bound as `ASSETS`, and `run_worker_first` for `/`, `/digest` and `/digest/sources` so those paths reach the Worker instead of being served as files. Secrets for the Telegram bot are set with `wrangler secret put`.

The routing order is in chapter 5. Details worth knowing:

- **The snapshot** is `snapshot.json` with `{generated_at, days: 7, items}`. The Worker answers `/digest` from it for `days` of 1, 2 or 7 (and for a valid `day=` within the window, read in India time, mirroring the Go `editionZone`). It falls back to the origin for a text query, `articles_only=false`, a date older than the window, or `day` on `/digest/sources`. The parsed snapshot is cached per isolate.
- **`categoryOf`** is `item.category || 'other'`, the same fallback as `categorySQL` in Go.
- **Published files** are served only for the exact query strings the snapshot was built with: `/cards` (empty or `?days=14&limit=200`), `/months`, `/concepts`, and `/entities` (empty or `?days=14&limit=150`). If `web/lib/api.ts` changes a query without `build-snapshot.sh` and the Worker's table changing too, the Worker silently falls back to the origin.
- **Quiz and intro** are served from static files for today and the previous eight edition days; a missing file in that window is a 404 only if the snapshot itself exists.
- **Caching:** snapshot answers send `max-age=300` and an `x-edge: snapshot` header; origin-proxied stories are cached six hours and other origin reads one hour.
- **The Telegram bot** (`edge/telegram.js`) answers `/today [track]`, `/topic <words>`, `/quiz [track]` (sends up to five native quiz polls) and `/brief [track]`. The webhook checks a secret header. It replies 200 immediately and sends its answers in the background, because Telegram retries slow responses. Its track-to-category table is a copy of `internal/track`; keep them in step.

## 34. Scripts

**`scripts/build-snapshot.sh BINARY OUT_DIR`** builds everything the Worker serves:

1. Empties the output directory and starts the app serve-only on `127.0.0.1:18080` (`WORKERS=0 READ_ONLY=true`, with `SOURCES` unset), so the snapshot is exactly what the live API would return.
2. Pages through `/digest?days=7&detail=1&page_size=200`, downloads each picture through `/img/{id}` (6 at a time), and blanks any picture larger than `SNAPSHOT_MAX_IMAGE_BYTES` (default 3,000,000) so the card shows a tile instead.
3. Writes `stories/{id}.json` from `/story/{id}`, the quiz and intro for each track for today and the previous eight India-time days, `cards.json`, `months.json`, `concepts.json`, `entities.json`, `month/{track}/{month}.json` for the last twelve months, and `snapshot.json`.
4. Writes `sources.json` for the `/sources` page (host, names, categories, note, and a `policy` of `restricts` when the note says the site disallows AI use), leaving out `BLOCKED_HOSTS`.
5. Copies the legacy front page to `index.html` and `digest.html`, and writes a `_headers` file giving pictures a one-year immutable cache (their file names contain the article id, so a name never changes its content).

**`scripts/warm-caches.sh WEB_URL [API_URL]`** visits the main pages, categories, a few archive days and the newest stories twice, so the first real visitor does not pay for a cold cache. It never fails the job.

**Takedown** has no script. It is the repository variable `BLOCKED_HOSTS` (comma-separated). The scheduler drops those hosts at startup (`scheduler.DropBlocked`), the "Prune old rows" workflow step deletes their stored pages, and `build-snapshot.sh` omits them from the public list. `docs/PRO_BOT_TAKEDOWN.md` describes the full procedure, including setting the variable on Render too and running the `DELETE` by hand in Neon for an immediate removal.

---

# Part 8. Operating and changing the project

## 35. Running it

### Local, quickest path

```sh
export DATABASE_URL=postgres://...              # a disposable Postgres
go build ./...
go run ./cmd/crawlerx -sources sources.json -run-on-start     # one digest run now
go run ./cmd/checksource https://example.com/blog/            # vet a site first
```

The API is then at `http://localhost:8080` (for example `/digest`, `/healthz`). For the web app: `cd web`, copy `.env.example` to `.env.local`, set `API_BASE_URL=http://localhost:8080`, then `npm install` and `npm run dev`.

### Makefile targets

| Target | What it does |
|---|---|
| `make build` | `go build ./...` |
| `make build-linux` | Static Linux binary into `bin/crawlerx` |
| `make test-short` | `go test -short -race ./...` |
| `make test` | `go test -race -p 1 ./...` (needs `TEST_DATABASE_URL`) |
| `make lint` | `go vet` and `staticcheck` |
| `make fuzz` | 30 seconds of the URL fuzz test |

### The container

The `Dockerfile` is a multi-stage build: `golang:1.26` compiles with `CGO_ENABLED=0 -trimpath -tags timetzdata -ldflags="-s -w"`, and the result runs on `gcr.io/distroless/static-debian12:nonroot`, exposing 8080. It copies only `cmd/crawlerx`, `internal`, `go.mod`, `go.sum` and `sources.json`. `.dockerignore` keeps `.env` out of the image. It builds only `crawlerx`; `senddigest` and `checksource` are built in CI when needed.

### Environment variables

Flags override environment variables, which override defaults. Names below are environment variables; most have a matching lower-case flag with dashes.

**Core**

| Variable | Default | Meaning |
|---|---|---|
| `DATABASE_URL` | none | Required. Use a direct Neon URL (no `-pooler`). |
| `LISTEN_ADDR` | `:8080` | API address; empty turns the API off (crawl job). |
| `READ_ONLY` | true | Removes `POST /crawls`. Set `READ_ONLY=false` only on a private network. |
| `WORKERS` | 8 | Crawl workers; 0 means API only. |
| `HOST_DELAY` | 1s | Minimum gap between requests to one host. |
| `HOST_INFLIGHT` | 2 | Concurrent requests per host. |
| `MAX_BYTES` | 2 MiB | Largest page body read. |
| `TIMEOUT` | 15s | Per-request timeout. |
| `MAX_ATTEMPTS` | 5 | Tries before a URL is `dead`. |
| `LEASE` | 1m | Lease length; must exceed twice `TIMEOUT`. |
| `CRAWL_TIMEOUT` | 2h | Longest the daily run waits for its crawls before summarising what it has (the workflow sets 30m). |
| `SHUTDOWN_GRACE` | 15s | How long in-flight work may finish on shutdown. |
| `USER_AGENT` | built in | Crawler identity. |
| `CONDITIONAL` | false | Send `If-None-Match` and `If-Modified-Since`. |
| `LOG_LEVEL`, `LOG_FORMAT` | info, text | debug, info, warn, error; text or json. |
| `PPROF_ADDR` | empty | Profiling address; off by default. |
| `ALLOW_PRIVATE_SOURCES` | false | Allow private addresses (tests only). |
| `IMAGE_CACHE_MB` | 128 | Image proxy cache size. |
| `GOMEMLIMIT` | unset | Go runtime memory soft limit (for example `180MiB` on Render). |

**Scheduler and summaries**

| Variable | Default | Meaning |
|---|---|---|
| `SOURCES` | empty | Source file(s), comma-separated; enables the scheduler. |
| `DIGEST_AT` | `06:00` | Daily run time. |
| `TIMEZONE` | `Asia/Kolkata` | Zone for `DIGEST_AT`. |
| `RUN_ON_START` | false | Run immediately. |
| `MAX_SITES` | 0 | Cap on distinct sites (0 means none). |
| `SUMMARY_PROVIDER` | auto | `auto`, `gemini` or `extractive`. |
| `SUMMARY_MODEL` | built in | Gemini model name. |
| `SUMMARY_BATCH` | 6 | Articles per request (1 to 20). |
| `SUMMARY_WORKERS` | 3 | Parallel summary workers (1 to 8). |
| `SUMMARY_WINDOW_DAYS` | 7 | How far back to summarise (1 to 30). |
| `GEMINI_API_KEY` | none | Environment only. |
| `SEED`, `MAX_DEPTH`, `ALLOWED_HOSTS`, `PATH_PREFIXES`, `EXIT_WHEN_DONE` | none | Start one crawl at launch. |
| `BLOCKED_HOSTS` | none | Takedown list, comma-separated. |

**Mail, channels, billing**

| Variable | Default | Meaning |
|---|---|---|
| `MAIL_PROVIDER` | `log` | `log` or `brevo`. |
| `BREVO_API_KEY`, `MAIL_FROM`, `MAIL_FROM_NAME` | none, none, `Postpaper` | Required for Brevo; the sender domain must be verified there. |
| `WEB_BASE_URL` | none | Where email links point; required for Brevo. |
| `PUBLIC_API_URL` | none | Used in the `List-Unsubscribe` header. |
| `MAIL_FOOTER`, `MAIL_SPONSOR_TEXT`, `MAIL_SPONSOR_URL` | none | The sponsor line needs both text and URL. |
| `TELEGRAM_BOT_TOKEN`, `TELEGRAM_CHAT_UPSC`, `TELEGRAM_CHAT_ENGINEERING` | none | Channel posts; a chat needs a token. |
| `TEAM_POSTS` | false | Enables Slack and Discord (`true`, not `on`). |
| `RAZORPAY_WEBHOOK_SECRET` | none | Enables `/billing/razorpay`. |
| `PRO_PLANS` | `9900:31,24900:92` | Paise and days per plan. |

**Web app (`web/.env.example`, read at build time, so redeploy after changing):** `API_BASE_URL`, `SITE_URL`, `SPONSOR_TEXT`, `SPONSOR_URL`, `SPONSOR_LABEL`, `CF_ANALYTICS_TOKEN`, `CONTACT_EMAIL`, `PRO_MONTHLY_URL`, `PRO_MONTHLY_PRICE`, `PRO_QUARTERLY_URL`, `PRO_QUARTERLY_PRICE`.

**Scripts and CI:** `SNAPSHOT_PORT`, `SNAPSHOT_MAX_IMAGE_BYTES`, `WARM_PAUSE`; `CLOUDFLARE_API_TOKEN` and `CLOUDFLARE_ACCOUNT_ID` for `wrangler deploy`; the Worker secrets `TELEGRAM_BOT_TOKEN` and `TELEGRAM_WEBHOOK_SECRET`.

### The crawl workflow (`.github/workflows/crawl.yml`)

The only workflow. It runs at 00:30, 06:30, 12:30 and 18:30 UTC (06:00, 12:00, 18:00 and 00:00 IST) and on demand, one at a time, with a 45 minute limit.

1. Check out the repository, set up Go from `go.mod` and Node 22.
2. Build `crawlerx`.
3. **Crawl and summarise.** Run `./crawlerx -listen=` in the background. Poll its log every 5 seconds for up to 40 minutes for the exact text `digest run complete`, then stop it with SIGTERM. Fail if the text never appears.
4. **Prune old rows:** crawls older than 3 days; pages older than 60 days (pages with no date are kept deliberately); `deliveries`, `channel_posts`, `job_runs`, `quizzes` and `intros` older than 60 days (`pro_payments` is kept); pages of `BLOCKED_HOSTS`.
5. **Publish the snapshot** with `build-snapshot.sh` and `npx wrangler@4 deploy`, only if the Cloudflare secrets exist.
6. **Warm the caches** (continues on error).
7. **Send the daily digest** with `cmd/senddigest` only when a real mail provider, a Telegram chat or `TEAM_POSTS=true` is configured (it runs after the deploy, so a failure turns the run red without blocking it).

Job settings include `WORKERS=16`, `SOURCES=sources.json`, `RUN_ON_START=true`, `MAX_SITES=60` and `GOMEMLIMIT=1500MiB`. Secrets are `DATABASE_URL`, `GEMINI_API_KEY`, `CLOUDFLARE_API_TOKEN`, `CLOUDFLARE_ACCOUNT_ID`, `TELEGRAM_BOT_TOKEN` and `BREVO_API_KEY`; everything else is a repository variable.

The crawl job builds from the repository's default branch, so a change takes effect on the live site only after it is merged there.

### How to add things

**A news source.**
1. `go run ./cmd/checksource https://site.example/section/`. It runs the real robots check, fetcher and parser on the seed and three linked pages (one request a second) and reports whether robots.txt allows it, what its AI policy says, whether real HTML came back, whether dated articles were found, and a suggested prefix. It exits 1 on a "no" verdict. Flags: `-prefix`, `-article-prefix` (for sites without dates), `-n`, `-delay`, and `-ai-policy URL...` to print only the AI policy.
2. Copy the printed entry into `sources.json` with a `category` and the policy `note`. Never work around a site that blocks bots.
3. `go test ./internal/scheduler` validates every entry. Mind `MAX_SITES`.

**An API route.** Write a handler method on `server` in `internal/api`, validate parameters with limits, call the store, write JSON with `writeJSON`, set `Cache-Control` if it is cacheable, register it in `NewHandler`, and add a test with `httptest`. If the public site should read it from the snapshot, also add it to `build-snapshot.sh` and to the Worker's route table, and a typed fetcher in `web/lib/api.ts` with the *same* query string.

**A store method.** Add it to the `Store` interface, implement it in `postgres.go` and `memory.go`, and add it to a contract test that runs on both. Mind the parity rule.

**A migration.** Add the next numbered file in `internal/db/migrations` with `-- +goose Up` and `-- +goose Down`. Prefer additive changes (new nullable columns, new tables, partial indexes). Data migrations need a sensible Down.

**A pipeline step.** Follow the pending and set pattern: a nullable `*_at` column, a `Pending...` query behind a partial index, a pass function that takes a limit and returns a count, `ErrRateLimited` handling, and a call from `runOnce` that logs failures and never aborts the run.

## 36. Conventions that are easy to get wrong, risks and stale docs

**Conventions** (also in `CLAUDE.md`):

- **Category** is a lower-case slug stored on both `crawls` and `pages`. Never join through `crawls` to find a page's category, because crawls are pruned after 3 days. Empty shows as `other`.
- An item counts as an **article** only with its own published date or under a source's `article_prefixes`.
- Every `sources.json` entry needs a `category`; read the robots.txt AI-policy note before adding a site.
- **`/digest` filtering exists in three places**: SQL in `postgres.go`, Go in `memory.go` and JavaScript in `edge/worker.js`. Change all three.
- The **log line `digest run complete`** is read by the workflow.
- The **dark theme tokens** are written twice in the stylesheet.
- A page host is derived in SQL and must stay NULL-safe.
- Pages that call the API must render at request time.
- Update `FEATURES.md` (run `/ledger-update`) after a change to behaviour, schema, config, limits or deploy, before committing.

**Known risks and gaps.**

- No authentication on the API; rely on `READ_ONLY` and a CDN.
- Rate limits are global per process, so one noisy caller can lock out everyone's signups for a minute.
- The politeness gate is per process, so run one crawler per database.
- The first request after Render sleeps can exceed the 10 second server-action timeout.
- Story grouping is by headline similarity: it misses the same event worded differently and can join two similar events.
- The extractive summary fallback is plain; quality depends on the Gemini key and quota.
- `TestSIGTERMReleasesLeasesAndExitsCleanly` fails on a clean checkout.
- The `events` table is unused; drop it with a new migration if the space matters.
- Vendor free-tier figures in these docs come from vendor pages at the time of writing and may have changed.

**Documents that are partly out of date** (this guide and the code are the reference):

- `docs/CONCEPTS.md` describes the pre-Postgres design (`cmd/crawler`, `internal/fetcher`, a channel job queue, metrics). Its Go teaching is still sound, but its file references are not.
- `docs/DEPLOYMENT.md` gives a daily 06:00 IST schedule (it is every six hours), 43 or 47 sources (it is 50), `MAX_SITES=45` (it is 60), and says nothing references Vercel (the web app is deployed there). Its Worker route list predates the story, month, cards and quiz routes.
- `README.md` says "43 sources" and describes the old static front page.
- `.env.demo.example` lists crawl-job values that no longer match `crawl.yml`.

**Where to go next.** `FEATURES.md` is the ledger, with one entry per subsystem (key files, schema dependencies, trade-offs) and the removed Upcoming calendar marked Deprecated. `docs/ROADMAP.md` lists what is built and what is planned. `CLAUDE.md` is the shortest summary of the conventions. `docs/CONCEPTS.md` is a good introduction to Go concurrency despite its dated file references.
