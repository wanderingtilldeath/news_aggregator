# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

CrawlerX (Go module `github.com/wanderingtilldeath/news-aggregator`): a resumable, polite crawler that also runs a daily news digest. It crawls the sites in `sources.json`, writes a gist per article, and serves a newspaper-style front page ("The Daily Digest") plus a JSON API. Postgres is the only service. README.md has the full feature and deploy detail; `docs/CONCEPTS.md` explains the design.

## Commands

```sh
go build ./...
go test -short -race ./...                       # no database needed; DB tests skip
TEST_DATABASE_URL=postgres://... make test       # full suite, -race, -p 1
go test -race ./internal/store -run TestPostgresDigestFiltersByCategoryAndBackfillsIt   # one test (needs TEST_DATABASE_URL for Postgres tests)
make lint                                        # go vet + staticcheck
make fuzz                                        # urlnorm fuzz, 30 s

export DATABASE_URL=postgres://...
go run ./cmd/crawlerx -sources sources.json -run-on-start   # daily digest run now
go run ./cmd/checksource https://example.com/blog/          # vet a site before adding it to sources.json
```

- `TEST_DATABASE_URL` must point at a **disposable** Postgres. `internal/testutil` creates one schema per test and drops it; tests that create and drop schemas refuse a URL that is not local (`IsLocalURL`: localhost, 127.0.0.1 or host `postgres`).
- `-p 1` matters: integration packages share one database.
- `internal/e2e` spawns `crawlerx` as a subprocess (`kill -9` and SIGTERM tests). `TestSIGTERMReleasesLeasesAndExitsCleanly` already fails on a clean checkout; it is not caused by new changes.
- Migrations (`internal/db/migrations`, goose) and the web pages (`internal/api/static`) are embedded in the binary. Use a direct Neon connection string (no `-pooler`): the pgx statement cache and goose's advisory lock break behind transaction pooling.

## Architecture

Config is flags with env fallbacks (`internal/config`); `cmd/crawlerx/main.go` wires everything. The same binary runs in three modes depending on config, which is how the free deployment works:

- **Full** (local, small server): API + crawl workers + the daily scheduler (`-sources`).
- **Serve-only web box** (Render): `WORKERS=0 READ_ONLY=true`, no `SOURCES`. `READ_ONLY` removes `POST /crawls`. Memory is tight (`GOMEMLIMIT=180MiB`), so it never crawls.
- **Crawl job** (GitHub Actions, `.github/workflows/crawl.yml`): runs `crawlerx -listen=` with `SOURCES=sources.json`, waits for the log line `digest run complete`, stops it, prunes old rows, then runs `scripts/build-snapshot.sh` and `wrangler deploy`. That workflow greps the log, so do not reword the message.

Request path for the public site: Cloudflare Worker (`edge/worker.js`) answers `/` and `/digest`, `/digest/sources` for the 1, 2 and 7 day views from a static snapshot (`snapshot.json`; `/img/*` are static assets beside it) built by the crawl job, and proxies everything else (30-day view, text search, `/dashboard.html`) to Render. **The Worker re-implements the filtering in `internal/api`/`internal/store` in JS** (category, source, query, paging, `categoryOf`). A change to `/digest` filtering must be made in both places; compare their output for the same query.

Core pieces (all under `internal/`):

- `frontier`: queue and visited set in one Postgres table, claimed with `FOR UPDATE SKIP LOCKED` and leases (crash recovery). `Memory` variant for tests. `CrawlSpec` carries per-crawl rules (`path_prefixes`, `article_prefixes`, `split_articles`, `image_from_source`, `category`).
- `crawler`: worker loop: claim URL, robots check, politeness gate, fetch, parse, save page, enqueue links.
- `scheduler`: loads `sources.json` (`sources.go` validates every field), starts one crawl per source at `DIGEST_AT` (one run per day, guarded by a `job_runs` row), then pipelines summaries and source-image lookups while crawling continues.
- `store`: `Store` interface with `Postgres` and `Memory` implementations. **They must behave the same**: every filter in `DigestQuery` is written twice (SQL in `postgres.go`, Go in `memory.go`) and tested in both.
- `api`: HTTP handlers and the embedded pages. `/` is the news page (`static/index.html`, also served at `/digest.html`, the Render health check path); `/dashboard.html` is the crawler dashboard.
- `images`: `/img/{article id}` proxy. It looks the image URL up by article id, never from the request, so it is not an open proxy; refuses private addresses.
- `summarize`: Gemini (batched) or extractive fallback. `fetch`, `robots`, `politeness`, `parse`, `urlnorm` are the polite-fetch building blocks.

## Conventions that are easy to get wrong

- **Category** is a lowercase slug (`^[a-z0-9-]{1,24}$`), stored on both `crawls` and `pages` (crawls are pruned after 3 days, so never join through them). Empty shows as `other` via `categorySQL`. The page upsert also rewrites a row when only its category changed, so editing `sources.json` re-labels old pages.
- A page's host is derived in SQL from its URL (`hostSQL`). It is NULL-safe on purpose: a non-http URL once made `/digest` return 500.
- An item counts as an *article* only with its own published date, or under a source's `article_prefixes`. Listing and tag pages stay out of the digest.
- Every `sources.json` entry needs a `category`; a test enforces it, and that Indian Express, Drishti IAS and InsightsIAS stay present. Check a site with `cmd/checksource` and read the robots.txt AI-policy `note` before adding it; sites that block bots are not worked around.
- The front page is one static HTML file with no libraries or external fonts. All DOM is built with `node()` (textContent only) and links pass through `safeURL()`; never use `innerHTML` with feed data. State lives in the URL (`days`, `category`, `source`, `q`, `page`).
- Snapshot size is bounded by `SNAPSHOT_MAX_IMAGE_BYTES` (default 400000); larger pictures fall back to a site tile.

## Slash commands and the feature ledger

- `/onboard [focus area]` (`.claude/commands/onboard.md`): read-only tour of the codebase (stack, schema, traced request path, scaling and resilience, interview prep). Creates `FEATURES.md` if it is missing.
- `/ledger-update [feature | git range]` (`.claude/commands/ledger-update.md`): syncs `FEATURES.md` with the working-tree or recent-commit changes. Marks entries Modified or Deprecated, never deletes them, and does not commit.
- `FEATURES.md` is the per-subsystem ledger (key files, schema dependencies, trade-offs). After a change to behaviour, schema, config, limits or deploy, run `/ledger-update` before committing.
