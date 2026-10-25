# Features & Architecture Ledger

Maintained by `/ledger-update`. Seeded by `/onboard`. One entry per subsystem; entries are never deleted, only marked Deprecated.

### Frontier (resumable work queue)
- **Status**: Modified
- **Last Updated**: 2026-10-25 (57b59e8, uncommitted)
- **Entry Points / Key Files**:
  - `internal/frontier/frontier.go`: `Frontier` interface, `Crawl`, `CrawlSpec`, `IsArticlePath`
  - `internal/frontier/postgres.go`: `claimSQL`, `Claim`, `Finish`, `Reschedule`, `Release`, `Enqueue`
  - `internal/frontier/memory.go`: in-memory variant for tests
  - `internal/db/migrations/00001_init.sql`: `crawls` and `frontier` tables
- **Implementation Summary**: URLs are canonicalised by `internal/urlnorm` before they are queued, which drops tracking parameters (`utm_*`, `gclid`, `fbclid`, `tpcc` and others) so one article is stored once. One row per canonical URL per crawl is both the queue and the visited set. Workers claim due rows with `FOR UPDATE SKIP LOCKED` and take a lease in the same statement, shallowest depth first, so the depth limit means "within N hops". A row whose lease expired is claimed again and counts as a failed attempt, which is how a killed process recovers. After `MAX_ATTEMPTS` the row becomes `dead`.
- **Data & Schema Dependencies**: `crawls` (seed, depth, allowed hosts, path/article prefixes, split, image-from-source, category), `frontier` (status check constraint, partial claim index on pending/leased rows).
- **Production Trade-Offs**: Postgres is the only coordinator, so several crawler processes can share a database, but the politeness gate is per process (see Polite fetching). `frontier` rows cascade-delete with their crawl, which the crawl job prunes after 3 days.

### Crawl worker loop
- **Status**: Active
- **Last Updated**: 2026-10-20 (b54e7a7)
- **Entry Points / Key Files**:
  - `internal/crawler/crawler.go`: `Run`, `worker`, `handle`, `process`, `handlePage`, `savePage`
  - `cmd/crawlerx/main.go`: `run` wires every dependency and the shutdown order
  - `internal/config/config.go`: flags with env fallbacks
- **Implementation Summary**: Each worker claims a URL, checks robots.txt, asks the politeness gate for a slot, fetches, then saves the page and enqueues its links. Outcomes map to a frontier status: 2xx saves, 3xx queues the redirect target, 429/5xx reschedule with the server's `Retry-After`, other 4xx fail, a robots denial is `blocked`. An item that cannot be finished is released rather than lost. On shutdown workers drain for `SHUTDOWN_GRACE`, then pending page writes flush, then the HTTP servers stop.
- **Data & Schema Dependencies**: `frontier`, `pages`, `crawls`.
- **Production Trade-Offs**: A busy host defers the URL (reschedule) instead of sleeping, so a worker is never parked on one slow site. `kill -9` loses no work. `TestSIGTERMReleasesLeasesAndExitsCleanly` in `internal/e2e` fails on a clean checkout and is not understood yet.

### Polite fetching (fetch, robots, politeness)
- **Status**: Active
- **Last Updated**: 2026-10-20 (b54e7a7)
- **Entry Points / Key Files**:
  - `internal/fetch/fetch.go`: fetcher (2 MiB body cap, HTML only, no inline redirects)
  - `internal/fetch/retry.go`: `ShouldRetryStatus`, `ParseRetryAfter`, `Delay` (full-jitter backoff)
  - `internal/robots/robots.go`: RFC 9309 check with cache
  - `internal/politeness/politeness.go`: `Memory` gate (`Acquire`, `Penalize`)
  - `internal/urlnorm/urlnorm.go`: `Canonicalize`
- **Implementation Summary**: Every request goes through the robots check and a per-host gate that spaces requests by `HOST_DELAY` or the site's `Crawl-delay`, whichever is longer. A host that sends `Retry-After` is penalised as a whole. Redirects are not followed in the client; the target is queued like any link, so it is checked again. URLs are canonicalised before they enter the frontier.
- **Data & Schema Dependencies**: robots.txt cache in memory only.
- **Production Trade-Offs**: The gate and robots cache live in process memory, so run one crawler per database or the per-host delay is not enforced across processes. The crawler fetcher does not refuse private or loopback addresses (only the image proxy and source-image step do), so `POST /crawls` on an exposed instance is an SSRF risk; `READ_ONLY` removes it on public deployments.

### Page parsing and article detection
- **Status**: Modified
- **Last Updated**: 2026-10-25 (3738990, uncommitted)
- **Entry Points / Key Files**:
  - `internal/parse/parse.go`: `Parse`, `Page` (title, description, published date, images, text)
  - `internal/parse/sections.go`: `<article id>` blocks for `split_articles`
  - `internal/parse/notfound.go`: `LooksLikeNotFound` (error pages served with 200)
  - `internal/crawler/crawler.go`: `savePage` decides `is_article`
- **Implementation Summary**: One parse per page extracts metadata, links and the best picture. A page is an article if it has its own published date (meta tags, JSON-LD, or a date in the URL; a bare `<time>` does not count), unless the source sets `article_prefixes`, which then decides by path. Split sources turn each `<article id>` block into its own item linked as `page#id`, and the page itself is not an article. The page date is taken in this order: meta tag, JSON-LD `datePublished`, date in the URL, first `<time datetime>` (`walker.timeTag`, last resort because pages carry events or trending widgets before the article's own date). A date more than 24 hours in the future (`maxFutureSkew`) is discarded as unknown. A "not found" page that answers 200 is never an article.
- **Data & Schema Dependencies**: Writes `pages.published_at`, `is_article`, `content`, `image_url`, `body_image_url`, `source_url`.
- **Production Trade-Offs**: A site with no machine-readable date and no `article_prefixes` rule never appears. Undated pages (including ones whose date was discarded as future) sort by `first_seen_at`, so a first crawl lists existing posts as new.

### Page store and digest queries
- **Status**: Modified
- **Last Updated**: 2026-10-25 (3738990, uncommitted)
- **Entry Points / Key Files**:
  - `internal/store/store.go`: `Store` interface, `DigestQuery`, `Item`
  - `internal/store/postgres.go`: batched `Save`, `upsertSQL`, `Digest`, `DigestSources`, `hostSQL`, `categorySQL`
  - `internal/store/memory.go`: same filters in Go
  - `internal/db/migrations/`: 00001 to 00008 (`pages`, `published_at`, summaries, image columns, `category`)
- **Implementation Summary**: Saves are grouped into batches (group commit) and written with one upsert per row. The upsert skips rewriting a row when the content hash is unchanged, unless the category or `published_at` changed, so a parser date fix heals old rows on the next crawl. The digest orders by `COALESCE(published_at, first_seen_at)` and filters by window, full-text query, source host, article flag and category. The host is derived from the URL in SQL, and the effective image is the og:image unless five or more articles share it, then the body image.
- **Data & Schema Dependencies**: `pages` (tsvector `search` with GIN index, `pages_recent_idx`, partial indexes for pending summaries and source images), `job_runs`.
- **Production Trade-Offs**: Postgres and Memory stores must stay identical; each filter is written twice and tested in both. `pages` stores full article text in `content`, and undated pages are never pruned, so the 1 GB Neon free tier is the long-term limit. `hostSQL` is NULL-safe because a non-http URL once made `/digest` return 500.

### Sources and daily scheduler
- **Status**: Modified
- **Last Updated**: 2026-10-25 (3738990, uncommitted)
- **Entry Points / Key Files**:
  - `sources.json`: 47 source entries (46 sites), each with a `category`
  - `internal/scheduler/sources.go`: `LoadSources`, validation, `{date:...}` and `days_back` expansion, `Specs`
  - `internal/scheduler/scheduler.go`: `Run`, `runIfClaimed`, `runOnce`, `waitForCrawls`, `enrich`
  - `internal/scheduler/claims.go`: one run per day via `job_runs`
  - `internal/scheduler/shipped_sources_test.go`: shipped file rules
- **Implementation Summary**: At `DIGEST_AT` (or at once with `RUN_ON_START`, or as a catch-up) the scheduler claims today's `job_runs` row, starts a crawl per source, and polls until every crawl has no active work, bounded by `CrawlTimeout`. While waiting it runs summary and image passes whenever a full batch is ready, then a final pass that waits out rate limits. It logs `digest run complete`, which the crawl workflow waits for.
- **Data & Schema Dependencies**: `job_runs`, `crawls`, `frontier`, `pages`.
- **Production Trade-Offs**: `MAX_SITES` makes startup fail above a site cap. The daily claim means a second start the same day does nothing unless `RUN_ON_START` is set. A test pins Indian Express in `sources.json` and requires a category on every entry. Drishti IAS and InsightsIAS were removed from `sources.json`; the `split_articles`, `image_from_source` and `article_prefixes` features remain for other sources. Seven engineering blogs (Heroku, Cisco, Canva, Salesforce, Snyk, Amazon Science, eBay) were added after passing `checksource`. Do not reword the `digest run complete` message without changing `.github/workflows/crawl.yml`.

### Summaries
- **Status**: Modified
- **Last Updated**: 2026-10-25 (57b59e8, uncommitted)
- **Entry Points / Key Files**:
  - `internal/summarize/summarize.go`: `Pass`, `ExtractiveGist`, `Summarizer`
  - `internal/summarize/gemini.go`: batched Gemini client with `Retry-After` handling
  - `cmd/crawlerx/main.go`: `newScheduler` picks the provider
- **Implementation Summary**: Each new article gets a short gist as up to three bullets of at most 20 words each (`maxBulletWords`). The Gemini provider batches several articles per request and spaces calls to stay under the free quota. Articles under 400 characters skip the model, and an article the model omits gets the extractive gist. Before summarising, `dropBoilerplate` removes sentences that are page chrome (paywall and login prompts, newsletter sign-ups, navigation bar, video labels) from the description and text, for both providers; the Gemini prompt also says to ignore them. The extractive provider needs no key. `truncateWords` cuts a longer bullet at its last comma within the limit (or at the limit), never splits a word, never ends on a dangling word such as "and" or "to", and adds "…" unless the cut lands on a full stop. The Gemini prompt asks for one complete sentence of at most 20 words and the same cap is applied to its reply.
- **Data & Schema Dependencies**: `pages.summary`, `summarized_at`, `pages_unsummarized_idx`; Gemini API.
- **Production Trade-Offs**: A stored gist is never regenerated, so bullets cut under the old 220-character rule stay until `summarized_at` is cleared. On a rate limit the pass pauses, retries up to 3 times, then leaves the rest for the next run. Only article text is sent to Gemini, never images. The chrome pattern list is a heuristic (`boilerplate` in `summarize.go`); a site with other chrome text needs a pattern added. The crawl job defaults to `extractive`.

### Article images (source lookup and proxy)
- **Status**: Active
- **Last Updated**: 2026-10-20 (b54e7a7)
- **Entry Points / Key Files**:
  - `internal/images/images.go`: `Pass`, `Blocklist`, `publicOnlyTransport`
  - `internal/images/proxy.go`: `GET /img/{id}` proxy with in-memory cache
  - `internal/db/migrations/00006_image_from_source.sql`, `00007_body_image.sql`
- **Implementation Summary**: Images are linked, not stored. The proxy looks the image URL up by article id, never from the request, fetches it once, caches it in memory (`IMAGE_CACHE_MB`), and serves it with `nosniff` and a locked-down CSP. For sources with `image_from_source`, a daily pass fetches the cited story's `og:image`, honouring its robots.txt at one request a second.
- **Data & Schema Dependencies**: `pages.image_url`, `body_image_url`, `source_url`, `image_checked_at`.
- **Production Trade-Offs**: Private and loopback addresses are refused, only JPEG, PNG, GIF, WebP and AVIF up to 5 MiB are served, and a failed fetch is remembered for 10 minutes. A host that blocks us is asked twice per run, then skipped. The cache is per process, so each instance warms its own.

### Digest and crawl API (read-only mode)
- **Status**: Modified
- **Last Updated**: 2026-10-20 (b54e7a7)
- **Entry Points / Key Files**:
  - `internal/api/api.go`: `NewHandler`, `digest`, `digestSources`, `search`, `createCrawl`, `healthz`
  - `internal/config/config.go`: `READ_ONLY`, `LISTEN_ADDR`
- **Implementation Summary**: `GET /digest` takes `days` (1 to 30), `page`, `page_size` (max 200), `q`, `source`, `category` and `articles_only`, and returns items with image paths rewritten to `/img/{id}`. `GET /digest/sources` lists hosts with category and count. `POST /crawls` is only registered when `READ_ONLY` is off and rejects unknown JSON fields and bodies over the size cap. Digest responses carry `Cache-Control: public, max-age=300`.
- **Data & Schema Dependencies**: `pages`, `crawls`, `frontier` through the `Store` and `Frontier` interfaces.
- **Production Trade-Offs**: There is no authentication on any route. Public deployments rely on `READ_ONLY=true` plus a private network or basic auth for the rest. `/healthz` pings the database, so Render's health check uses `/digest.html` instead. `POST /crawls` does not accept `category`; only `sources.json` crawls set one.

### News front page ("The Daily Digest")
- **Status**: Active
- **Last Updated**: 2026-10-20 (b54e7a7)
- **Entry Points / Key Files**:
  - `internal/api/static/index.html`: the page, served at `/` and `/digest.html`
  - `internal/api/static/dashboard.html`: the crawler dashboard, served at `/dashboard.html`
  - `internal/api/api.go`: embedded static files and the `/digest.html` handler
- **Implementation Summary**: One static file with no libraries or external fonts. The home view shows a lead story, a Latest list and one block per category; a section view adds source chips and a pager; search shows a result list. State is in the URL (`days`, `category`, `source`, `q`, `page`) and a sequence counter drops stale replies. All DOM is built with `textContent` and links go through `safeURL`.
- **Data & Schema Dependencies**: `/digest` and `/digest/sources` only.
- **Production Trade-Offs**: Never use `innerHTML` with feed data. The default window is 2 days so thin categories are not empty. The home view makes about 8 parallel `/digest` calls, which is why the Worker caches the parsed snapshot.

### Edge snapshot (Cloudflare Worker)
- **Status**: Modified
- **Last Updated**: 2026-10-25 (3738990, uncommitted)
- **Entry Points / Key Files**:
  - `edge/worker.js`: `fromSnapshot`, `loadSnapshot`, `categoryOf`, proxy fallback
  - `edge/wrangler.toml`: `ORIGIN`, `html_handling = "none"`, `run_worker_first`
  - `scripts/build-snapshot.sh`: builds `snapshot.json`, images, page copies, `_headers`
- **Implementation Summary**: The crawl job starts the app read-only on loopback, pages through `/digest?days=7`, downloads each image (up to 1.5 MB, `SNAPSHOT_MAX_IMAGE_BYTES`) and writes everything into `edge/assets`. The Worker serves `/` and static files from assets, answers `/digest` and `/digest/sources` for the 1, 2 and 7 day views from the snapshot, and proxies everything else (30-day view, text search, dashboard) to Render. The parsed snapshot is cached per isolate.
- **Data & Schema Dependencies**: Reads the app's own API; writes static files only. Needs `CLOUDFLARE_API_TOKEN` and `CLOUDFLARE_ACCOUNT_ID`.
- **Production Trade-Offs**: The Worker re-implements the Go filtering, so a change to `/digest` filters must land in both. Data changes only when the crawl job deploys. The worker CPU cost of parsing a large snapshot on the free plan has not been measured in production.

### Crawl job and deployment
- **Status**: Modified
- **Last Updated**: 2026-10-25 (3738990, uncommitted)
- **Entry Points / Key Files**:
  - `.github/workflows/crawl.yml`: crawl every 6 hours, prune, snapshot deploy
  - `Dockerfile`: distroless static image for the Render web service
  - `.env.demo.example`: web-service settings
  - `docs/DEPLOYMENT.md`: step-by-step guide
- **Implementation Summary**: Render runs the same binary serve-only (`WORKERS=0`, `READ_ONLY=true`, `GOMEMLIMIT=180MiB`). A GitHub Actions job runs the crawl with `WORKERS=16`, `GOMEMLIMIT=1500MiB` and `IMAGE_CACHE_MB=32` (per-host delay stays 1 s, so more workers do not load any one site harder), waits up to 40 minutes for `digest run complete`, stops the process, deletes crawls older than 3 days and pages published over 60 days ago, then builds and deploys the snapshot. Migrations run on every start.
- **Data & Schema Dependencies**: Neon Postgres (direct connection, not the pooler), GitHub secrets, Cloudflare account.
- **Production Trade-Offs**: The 45-minute job timeout against a 47-source crawl at 16 workers has not been measured. `MAX_SITES` is 60 in the workflow. `.env.demo.example` still lists old crawl-job comments (`WORKERS=2`, `MAX_SITES=6`) that differ from the workflow (`16`, `60`). The Dockerfile exposes 8080 while the demo listens on 10000.

### checksource tool
- **Status**: Active
- **Last Updated**: 2026-10-20 (b54e7a7)
- **Entry Points / Key Files**:
  - `cmd/checksource/main.go`: vetting command
  - `catalog/`: pre-vetted candidate sources with robots AI-policy notes
- **Implementation Summary**: Runs the crawler's own robots check, fetcher and parser against a URL with a handful of one-per-second requests. It reports robots and AI-crawler policy, whether the site serves real HTML, which prefixes hold articles, and whether sampled pages count as articles, then prints a `sources.json` entry and a WORKS, PARTIAL or NO verdict.
- **Data & Schema Dependencies**: None (no database).
- **Production Trade-Offs**: It does not bypass bot challenges, so a blocked site is reported as NO. Some publishers' robots.txt signal that AI use is not wanted; the `note` field records it and the digest summarises with a model, so read each site's terms first.
