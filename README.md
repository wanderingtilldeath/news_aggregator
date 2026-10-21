# CrawlerX: The Daily Digest

A polite, resumable news crawler in Go that turns 40+ news and engineering sites into one newspaper-style front page with a short gist for every article. Postgres is the only service it needs.

## The problem

Keeping up with news, engineering blogs and exam-prep sites (UPSC current affairs) means opening dozens of sites every day, skimming long articles and losing track of what you have already seen. CrawlerX does that once a day, automatically:

- crawls the sites listed in [`sources.json`](sources.json), respecting `robots.txt` and per-host rate limits,
- keeps only real articles (pages with their own published date), not listing or tag pages,
- writes a 2-3 line gist for each new article (Gemini, or an extractive fallback with no API key),
- serves the result as a front page with categories, source filters, search and a JSON API.

## Features

- **Resumable crawling.** The queue lives in Postgres with leases. `kill -9` the process and a restart carries on without re-crawling finished pages.
- **Polite by design.** RFC 9309 robots handling, per-host delay, honest User-Agent, size cap, `Retry-After` and jittered backoff. Sites that block bots are skipped, not worked around.
- **Daily digest scheduler.** One run per day (guarded by a `job_runs` row), with summaries and image lookups pipelined while crawling continues.
- **Cheap summaries.** Articles are batched into few Gemini calls; stored gists are never regenerated; quota errors pause the run instead of failing it.
- **Newspaper front page.** Lead story, category sections (India, World, UPSC, Tech, AI), 1/2/7/30-day views, text search. One static HTML file, no libraries.
- **Image proxy.** Card images are served from `/img/{article id}`, looked up by id (never from the request), with private-address refusal and a size/type check.
- **Free-tier deployment.** Render + Neon + GitHub Actions + a Cloudflare Worker edge cache, all on free plans.
- **Tooling for sources.** `checksource` vets a site (robots, AI policy, HTML vs JS shell, article detection) before you add it.

## Architecture

```mermaid
flowchart LR
    GH["GitHub Actions<br/>cron every 6h"] -->|runs| CX["crawlerx<br/>scheduler + workers"]
    CX -->|polite fetch| SITES[("43 news sources")]
    CX -->|batched gists| GEM["Gemini API<br/>(optional)"]
    CX <-->|queue, pages, gists| PG[("Postgres / Neon")]
    GH -->|build-snapshot.sh| SNAP["snapshot.json + images"]
    SNAP -->|wrangler deploy| CF["Cloudflare Worker<br/>edge/worker.js"]
    USER(("Reader")) --> CF
    CF -->|"1, 2, 7 day views"| SNAP
    CF -->|"30-day view, search, dashboard"| RENDER["Render web service<br/>read-only API"]
    RENDER --> PG
```

The same binary runs in three modes, which is how the free deployment works:

| Mode | Where | Config |
|---|---|---|
| **Full** | local or a small server | API + crawl workers + daily scheduler (`-sources`) |
| **Serve-only** | Render | `WORKERS=0 READ_ONLY=true`, no `SOURCES`; never crawls, so memory stays near 15 MB |
| **Crawl job** | GitHub Actions | `crawlerx -listen=` with `SOURCES=sources.json`; exits after `digest run complete` |

After each crawl the job prunes old rows, builds a snapshot of the last 7 days and deploys it with the Worker. Readers get the main views from the edge without waiting on Render or the database.

### Code layout

| Package | Role |
|---|---|
| `cmd/crawlerx` | wiring: config, DB, scheduler, API |
| `cmd/checksource` | vet a site before adding it to `sources.json` |
| `internal/frontier` | queue and visited set in one table, claimed with `FOR UPDATE SKIP LOCKED` and leases |
| `internal/crawler` | worker loop: claim, robots check, politeness gate, fetch, parse, save, enqueue links |
| `internal/scheduler` | loads sources, starts one crawl per source at `DIGEST_AT`, pipelines summaries |
| `internal/store` | `Store` interface with Postgres and in-memory implementations; full-text search (`tsvector` + GIN) |
| `internal/summarize` | batched Gemini client with an extractive fallback |
| `internal/fetch`, `robots`, `politeness`, `parse`, `urlnorm` | polite-fetch building blocks |
| `internal/images` | `/img/{id}` proxy and cache |
| `internal/api` | HTTP handlers and the embedded pages |
| `internal/db` | pgx connection and embedded goose migrations |
| `edge/` | Cloudflare Worker serving the snapshot |

## Tech stack

| Area | Choice |
|---|---|
| Language | Go 1.26, standard library HTTP |
| Database | PostgreSQL (Neon in production), `pgx/v5`, `goose` migrations embedded in the binary |
| Queue | Postgres table with `SKIP LOCKED` and leases, no Redis |
| Search | Postgres `tsvector` + GIN |
| Summaries | Google Gemini (Flash-Lite, batched) or extractive fallback |
| Robots | `temoto/robotstxt` |
| Frontend | one static HTML file, vanilla JS, no framework or external fonts |
| Edge | Cloudflare Workers + static assets |
| CI / cron | GitHub Actions |
| Hosting | Render (Docker) for the read-only API |

## Quick start

You need Go and a Postgres database.

```sh
export DATABASE_URL=postgres://user:pass@localhost:5432/crawlerx?sslmode=disable
go run ./cmd/crawlerx -sources sources.json -run-on-start   # crawl the sources now
open http://localhost:8080                                  # The Daily Digest
```

Without `GEMINI_API_KEY` gists are extractive. For better ones, get a free key from [Google AI Studio](https://aistudio.google.com/apikey) and export it.

Crawl a single site on demand:

```sh
curl -XPOST localhost:8080/crawls -d '{"seed":"https://example.com/","max_depth":2}'
curl 'localhost:8080/digest?days=1&category=tech'
curl 'localhost:8080/search?q=kubernetes'
```

### API

| Endpoint | Purpose |
|---|---|
| `GET /digest` | articles, newest first (`days`, `category`, `source`, `q`, `page`, `page_size`) |
| `GET /digest/sources` | hosts in the window with item counts |
| `GET /search?q=` | full-text search over crawled pages |
| `GET /img/{id}` | proxied card image |
| `POST /crawls`, `GET /crawls/{id}` | start and inspect a crawl (removed when `READ_ONLY=true`) |
| `GET /healthz` | liveness and database ping |

## Adding a news source

```sh
go run ./cmd/checksource https://example.com/blog/
```

It reports robots and AI-crawler policy, whether the site serves real HTML, which paths hold articles and whether sampled pages count as articles, then prints a `sources.json` entry. Every entry needs a `category`. Read the robots.txt AI-policy note first: some publishers (Indian Express included) disallow AI crawlers, so check their terms before publishing digests beyond your own use.

## Testing

```sh
go test -short -race ./...                   # no database needed
TEST_DATABASE_URL=postgres://... make test   # full suite, needs a disposable Postgres
make lint                                    # go vet + staticcheck
make fuzz                                    # URL canonicalisation fuzz test
```

The suite includes a deadlock regression test and end-to-end `kill -9` / SIGTERM tests in `internal/e2e`.

## Deployment

Free demo: Neon (Postgres), Render (read-only web), GitHub Actions (crawl every 6 hours), Cloudflare Worker (edge snapshot). Required repo secrets: `DATABASE_URL`, `CLOUDFLARE_API_TOKEN`, `CLOUDFLARE_ACCOUNT_ID`; optional `GEMINI_API_KEY`. Run the `crawl` workflow once by hand to publish the first snapshot. Use Neon's direct connection string (no `-pooler`).

A single small server (static binary + systemd + nginx) also works. Full steps are in [docs/REFERENCE.md](docs/REFERENCE.md) and [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md).

## Docs

- [docs/REFERENCE.md](docs/REFERENCE.md): configuration, source options, image rules, deploy details
- [docs/CONCEPTS.md](docs/CONCEPTS.md): design concepts behind the crawler
- [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md): deployment guide
- [FEATURES.md](FEATURES.md): per-subsystem ledger

## Responsible use

Only crawl sites that allow it. The API has no authentication and `POST /crawls` makes the server fetch URLs, so keep a full-mode instance on a private network and run the public one with `READ_ONLY=true`.
