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
  - `internal/parse/liveblog.go`: `LooksLikeLiveBlog` (live blogs are never articles)
  - `internal/crawler/crawler.go`: `savePage` decides `is_article`
- **Implementation Summary**: One parse per page extracts metadata, links and the best picture. A page is an article if it has its own published date (meta tags, JSON-LD, or a date in the URL; a bare `<time>` does not count), unless the source sets `article_prefixes`, which then decides by path. Split sources turn each `<article id>` block into its own item linked as `page#id`, and the page itself is not an article. The page date is taken in this order: meta tag, JSON-LD `datePublished`, date in the URL, first `<time datetime>` (`walker.timeTag`, last resort because pages carry events or trending widgets before the article's own date). A date more than 24 hours in the future (`maxFutureSkew`) is discarded as unknown. A "not found" page that answers 200 is never an article.
- **Data & Schema Dependencies**: Writes `pages.published_at`, `is_article`, `content`, `image_url`, `body_image_url`, `source_url`.
- **Production Trade-Offs**: A site with no machine-readable date and no `article_prefixes` rule never appears. Undated pages (including ones whose date was discarded as future) sort by `first_seen_at`, so a first crawl lists existing posts as new.

### Page store and digest queries
- **Status**: Modified
- **Last Updated**: 2026-11-14 (d2638dd, uncommitted)
- **Entry Points / Key Files**:
  - `internal/store/store.go`: `Store` interface, `DigestQuery`, `Item`
  - `internal/store/postgres.go`: batched `Save`, `upsertSQL`, `Digest`, `DigestSources`, `hostSQL`, `categorySQL`
  - `internal/store/memory.go`: same filters in Go
  - `internal/db/migrations/`: 00001 to 00014 (`pages` and its columns, `subscribers`, `deliveries`, enrichment columns, subscriber preferences, `quizzes`, `channel_posts`)
- **Implementation Summary**: Saves are grouped into batches (group commit) and written with one upsert per row. The upsert skips rewriting a row when the content hash is unchanged, unless the category or `published_at` changed, so a parser date fix heals old rows on the next crawl. The digest orders by `COALESCE(published_at, first_seen_at)` and filters by window (pages dated more than a day ahead are left out by `digestWhere` and `digestLocked`), full-text query, source host, article flag and category. The host is derived from the URL in SQL, and the effective image is the og:image unless five or more articles share it, then the body image.
- **Data & Schema Dependencies**: `pages` (tsvector `search` with GIN index, `pages_recent_idx`, partial indexes for pending summaries and source images), `job_runs`.
- **Production Trade-Offs**: `DigestQuery` also has `Until` (one edition day) and `Detail` (long summaries). The `Store` interface now also holds `Item` (one article by id), subscribers, deliveries, quizzes, enrichment and channel posts. Postgres and Memory stores must stay identical; each filter is written twice and tested in both. `pages` stores full article text in `content`, and undated pages are never pruned, so the 1 GB Neon free tier is the long-term limit. `hostSQL` is NULL-safe because a non-http URL once made `/digest` return 500.

### Sources and daily scheduler
- **Status**: Modified
- **Last Updated**: 2026-11-14 (d2638dd, uncommitted)
- **Entry Points / Key Files**:
  - `sources.json`: 50 source entries, each with a `category` (adds ORF, Down To Earth and Scroll)
  - `internal/scheduler/sources.go`: `LoadSources`, validation, `{date:...}` and `days_back` expansion, `Specs`
  - `internal/scheduler/scheduler.go`: `Run`, `runIfClaimed`, `runOnce`, `waitForCrawls`, `enrich`
  - `internal/scheduler/claims.go`: one run per day via `job_runs`
  - `internal/scheduler/shipped_sources_test.go`: shipped file rules
- **Implementation Summary**: At `DIGEST_AT` (or at once with `RUN_ON_START`, or as a catch-up) the scheduler claims today's `job_runs` row, starts a crawl per source, and polls until every crawl has no active work, bounded by `CrawlTimeout`. While waiting it runs summary and image passes whenever a full batch is ready, then a final pass that waits out rate limits. After the summary passes it runs the enrichment pass and writes the day's quizzes, then logs `digest run complete`, which the crawl workflow waits for.
- **Data & Schema Dependencies**: `job_runs`, `crawls`, `frontier`, `pages`.
- **Production Trade-Offs**: `MAX_SITES` makes startup fail above a site cap. The daily claim means a second start the same day does nothing unless `RUN_ON_START` is set. A test pins Indian Express in `sources.json` and requires a category on every entry. Drishti IAS and InsightsIAS were removed from `sources.json`; the `split_articles`, `image_from_source` and `article_prefixes` features remain for other sources. Seven engineering blogs (Heroku, Cisco, Canva, Salesforce, Snyk, Amazon Science, eBay) were added after passing `checksource`. Do not reword the `digest run complete` message without changing `.github/workflows/crawl.yml`.

### Summaries
- **Status**: Modified
- **Last Updated**: 2026-11-14 (d2638dd, uncommitted)
- **Entry Points / Key Files**:
  - `internal/summarize/summarize.go`: `Pass`, `ExtractiveGist`, `Summarizer`
  - `internal/summarize/gemini.go`: batched Gemini client with `Retry-After` handling
  - `cmd/crawlerx/main.go`: `newScheduler` picks the provider
- **Implementation Summary**: Each new article gets a short gist as up to three bullets of at most 20 words each (`maxBulletWords`). The Gemini provider batches several articles per request and spaces calls to stay under the free quota. Articles under 400 characters skip the model, and an article the model omits gets the extractive gist. Before summarising, `dropBoilerplate` removes sentences that are page chrome (paywall and login prompts, newsletter sign-ups, navigation bar, video labels) from the description and text, for both providers; the Gemini prompt also says to ignore them. The extractive provider needs no key. `truncateWords` cuts a longer bullet at its last comma within the limit (or at the limit), never splits a word, never ends on a dangling word such as "and" or "to", and adds "…" unless the cut lands on a full stop. The Gemini prompt asks for one complete sentence of at most 20 words and the same cap is applied to its reply.
- **Data & Schema Dependencies**: `pages.summary`, `summarized_at`, `pages_unsummarized_idx`; Gemini API.
- **Production Trade-Offs**: A stored gist is never regenerated, so bullets cut under the old 220-character rule stay until `summarized_at` is cleared. On a rate limit the pass pauses, retries up to 3 times, then leaves the rest for the next run. Only article text is sent to Gemini, never images. The chrome pattern list is a heuristic (`boilerplate` in `summarize.go`); a site with other chrome text needs a pattern added. The crawl job now defaults to `auto`, which uses Gemini when the `GEMINI_API_KEY` secret is set. Live blogs (`parse.LooksLikeLiveBlog`) are kept out of the digest and "this blog is now closed" is dropped as chrome. `Gemini.generate` is the shared retrying call for gists, enrichment and quizzes.

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
- **Last Updated**: 2026-11-15 (a70bcad, uncommitted)
- **Entry Points / Key Files**:
  - `internal/api/api.go`: `NewHandler`, `digest`, `digestSources`, `search`, `createCrawl`, `healthz`
  - `internal/config/config.go`: `READ_ONLY`, `LISTEN_ADDR`
- **Implementation Summary**: `GET /digest` takes `days` (1 to 30), `day` (one edition day, India time), `detail`, `page`, `page_size` (max 200), `q`, `source`, `category` and `articles_only`, and returns items (with why, tags and importance when enriched) whose image paths are rewritten to `/img/{id}`. `GET /story/{id}` (one article by id, no long summary), `GET /quiz`, `POST /subscribe`, `/confirm` and `/unsubscribe` are also served (the subscriber routes stay on a `READ_ONLY` box). `GET /digest/sources` lists hosts with category and count. `POST /crawls` is only registered when `READ_ONLY` is off and rejects unknown JSON fields and bodies over the size cap. Digest responses carry `Cache-Control: public, max-age=300`.
- **Data & Schema Dependencies**: `pages`, `crawls`, `frontier` through the `Store` and `Frontier` interfaces.
- **Production Trade-Offs**: There is no authentication on any route. Public deployments rely on `READ_ONLY=true` plus a private network or basic auth for the rest. `/healthz` pings the database, so Render's health check uses `/digest.html` instead. `POST /crawls` does not accept `category`; only `sources.json` crawls set one.

### News front page ("Postpaper")
- **Status**: Deprecated
- **Last Updated**: 2026-11-14 (d2638dd, uncommitted)
- **Entry Points / Key Files**:
  - Replaced by the Next.js frontend (see "Next.js web frontend (Vercel)"); still served until the Vercel site is the public address
  - `internal/api/static/index.html`: the page, served at `/` and `/digest.html`
  - `internal/api/static/dashboard.html`: the crawler dashboard, served at `/dashboard.html`
  - `internal/api/api.go`: embedded static files and the `/digest.html` handler
- **Implementation Summary**: One static file with no libraries or external fonts. The home view shows a lead story, a Latest list and one block per category. Every list is newest first. The 1-day and 2-day periods are fetched as 7 days (`fetchDays`), so older articles fill quiet sections. a section view adds source chips and a pager; search shows a result list. State is in the URL (`days`, `category`, `source`, `q`, `page`) and a sequence counter drops stale replies. All DOM is built with `textContent` and links go through `safeURL`.
- **Data & Schema Dependencies**: `/digest` and `/digest/sources` only.
- **Production Trade-Offs**: Never use `innerHTML` with feed data. The 1-day and 2-day periods show 7 days of articles, newest first, so a quiet period never leaves a section empty (the period filter is effectively a minimum). The home view makes about 8 parallel `/digest` calls, which is why the Worker caches the parsed snapshot.

### Edge snapshot (Cloudflare Worker)
- **Status**: Modified
- **Last Updated**: 2026-11-15 (a70bcad, uncommitted)
- **Entry Points / Key Files**:
  - `edge/worker.js`: `fromSnapshot`, `loadSnapshot`, `categoryOf`, proxy fallback
  - `edge/wrangler.toml`: `ORIGIN`, `html_handling = "none"`, `run_worker_first`
  - `scripts/build-snapshot.sh`: builds `snapshot.json`, images, page copies, `_headers`
- **Implementation Summary**: The crawl job starts the app read-only on loopback, pages through `/digest?days=7`, downloads each image (up to 1.5 MB, `SNAPSHOT_MAX_IMAGE_BYTES`) and writes everything into `edge/assets`. The Worker serves `/` and static files from assets, answers `/digest` and `/digest/sources` for the 1, 2 and 7 day views from the snapshot, and proxies everything else (30-day view, text search, dashboard) to Render. The parsed snapshot is cached per isolate.
- **Data & Schema Dependencies**: Reads the app's own API; writes static files only. Needs `CLOUDFLARE_API_TOKEN` and `CLOUDFLARE_ACCOUNT_ID`.
- **Production Trade-Offs**: The Worker re-implements the Go filtering, so a change to `/digest` filters must land in both. Requests with `day=` or `detail=` are sent to the origin because the snapshot has neither. `GET /story/{id}` is answered from the snapshot when the story is in it, else by the origin. Data changes only when the crawl job deploys. The worker CPU cost of parsing a large snapshot on the free plan has not been measured in production.

### Crawl job and deployment
- **Status**: Modified
- **Last Updated**: 2026-11-14 (d2638dd, uncommitted)
- **Entry Points / Key Files**:
  - `.github/workflows/crawl.yml`: crawl every 6 hours, prune, snapshot deploy
  - `Dockerfile`: distroless static image for the Render web service
  - `.env.demo.example`: web-service settings
  - `docs/DEPLOYMENT.md`: step-by-step guide
- **Implementation Summary**: Render runs the same binary serve-only (`WORKERS=0`, `READ_ONLY=true`, `GOMEMLIMIT=180MiB`). A GitHub Actions job runs the crawl with `WORKERS=16`, `GOMEMLIMIT=1500MiB` and `IMAGE_CACHE_MB=32` (per-host delay stays 1 s, so more workers do not load any one site harder), waits up to 40 minutes for `digest run complete`, stops the process, deletes crawls older than 3 days and pages published over 60 days ago, then builds and deploys the snapshot, then runs `senddigest` (email and Telegram) when `MAIL_PROVIDER` is `brevo` or a Telegram channel is set. Migrations run on every start.
- **Data & Schema Dependencies**: Neon Postgres (direct connection, not the pooler), GitHub secrets, Cloudflare account.
- **Production Trade-Offs**: The 45-minute job timeout against a 47-source crawl at 16 workers has not been measured. `MAX_SITES` is 60 in the workflow. `.env.demo.example` still lists old crawl-job comments (`WORKERS=2`, `MAX_SITES=6`) that differ from the workflow (`16`, `60`). The Dockerfile exposes 8080 while the demo listens on 10000.

### Subscribers and signup
- **Status**: Active
- **Last Updated**: 2026-11-14 (d2638dd, uncommitted)
- **Entry Points / Key Files**:
  - `internal/api/subscribe.go`: `POST /subscribe`, `POST /confirm`, `POST /unsubscribe`, `limiter`
  - `internal/store/store.go`: `Subscribe`, `Confirm`, `Unsubscribe`, `SetPreferences`, `ConfirmedSubscribers`
  - `internal/db/migrations/00009_subscribers.sql`, `00012_subscriber_prefs.sql`
- **Implementation Summary**: Signup stores an unconfirmed subscriber with a random token and sends a confirmation mail; the reader presses a button on `/confirm` (a POST, so mail scanners cannot confirm). The answer is the same for new, pending and confirmed addresses. Choices (tracks and 1min/7min) are stored only while the address is unconfirmed. `POST /unsubscribe` takes the token in the query string or JSON, so it also serves the one-click header. Signups are capped at 30 a minute for the whole process.
- **Data & Schema Dependencies**: `subscribers` (email, token, confirmed_at, unsubscribed_at, tracks, mode).
- **Production Trade-Offs**: The token is stored in plain text and never expires, because the unsubscribe link needs it. The limit is global and per process because the API only sees the web server's address. A subscriber who unsubscribed and signs up again gets a new token and must confirm again.

### Email (Brevo) and the daily send
- **Status**: Active
- **Last Updated**: 2026-11-14 (d2638dd, uncommitted)
- **Entry Points / Key Files**:
  - `internal/mail/`: `Sender`, `Brevo`, `Log`, `ConfirmationMessage`, `ConfirmationNotifier`
  - `cmd/senddigest/main.go`, `internal/digestmail/`: edition building, rendering, `Run`
  - `internal/db/migrations/00010_deliveries.sql`
  - `.github/workflows/crawl.yml`: the "Email the daily digest" step
- **Implementation Summary**: Brevo is called over HTTPS because free Render services block SMTP. `senddigest` builds one edition per combination of tracks and reading length (cached), ranks stories by importance, caps three per site, and mails each confirmed subscriber once per edition day (a `deliveries` row after each send). A send cap (default 250), a rate-limit stop, and five consecutive failures stop a run. `-dry-run` prints one sample, `-to` sends one sample and records nothing. Every mail carries an unsubscribe link and List-Unsubscribe headers.
- **Data & Schema Dependencies**: `subscribers`, `deliveries`, `pages`, `quizzes`; Brevo API.
- **Production Trade-Offs**: Brevo's free plan allows 300 mails a day, shared by confirmations and the digest. A crash between send and record can duplicate one mail. Whether Brevo passes `List-Unsubscribe-Post` through is unverified. The step runs only when `MAIL_PROVIDER` is `brevo` or a Telegram channel is set, because the log provider prints unsubscribe tokens.

### Enrichment: why it matters, tags, importance, long summary
- **Status**: Active
- **Last Updated**: 2026-11-14 (d2638dd, uncommitted)
- **Entry Points / Key Files**:
  - `internal/summarize/enrich.go`: `EnrichBatch`, `EnrichPass`, `parseEnrichment`
  - `internal/scheduler/scheduler.go`: `enrichArticles` (48 hours, 60 articles a run)
  - `internal/store/`: `PendingEnrichment`, `SetEnrichment`, `Item.Why/Tags/Importance/Long`
  - `internal/db/migrations/00011_enrichment.sql`
- **Implementation Summary**: After the gist pass, summarised articles get one Gemini call per five: a one-sentence why-it-matters line, up to three tags (the first is a UPSC syllabus area for india, world and upsc stories), an importance score 1 to 5 and a 90 to 130 word summary. The reply is cleaned (tags must match a pattern, text is truncated). `/digest` returns why, tags and importance; the long summary only with `detail=1`, which the Worker leaves to the origin so the snapshot stays small.
- **Data & Schema Dependencies**: `pages.why`, `tags`, `importance`, `long_summary`, `enriched_at`; Gemini API.
- **Production Trade-Offs**: Needs `GEMINI_API_KEY`; without it nothing is enriched and ranking falls back to recency. A skipped article stays pending for the next run. Importance is the model's opinion, not verified. Enrichment is limited to the last 48 hours and 60 articles per run to stay in the free quota.

### Tracks and reading modes
- **Status**: Active
- **Last Updated**: 2026-11-14 (d2638dd, uncommitted)
- **Entry Points / Key Files**:
  - `internal/track/track.go`: `UPSC`, `Engineering`, `Normalize`, `Categories`
  - `web/lib/view.ts`: `TRACKS` (mirrors the Go map)
  - `internal/digestmail/render.go`: `builder.edition`, `pick`
- **Implementation Summary**: A track is a set of source categories: upsc is india, world and upsc; engineering is tech and ai. A subscriber picks tracks and a length: 1min (headline and why line, at most 5 stories a track) or 7min (long summary or gist bullets, tags, quiz questions, up to 8 a track). The web app filters by `track=` and `mode=`.
- **Data & Schema Dependencies**: `subscribers.tracks`, `subscribers.mode`.
- **Production Trade-Offs**: The category-to-track map exists in Go and in TypeScript and must be changed in both. A track with fewer stories than the minimum skips that reader's mail only.

### Daily quiz
- **Status**: Active
- **Last Updated**: 2026-11-14 (d2638dd, uncommitted)
- **Entry Points / Key Files**:
  - `internal/summarize/quiz.go`: `GenerateQuiz`, `parseQuiz`, `QuizPass`
  - `internal/api/api.go`: `GET /quiz`
  - `web/app/quiz/page.tsx`, `web/components/quiz-player.tsx`
  - `internal/db/migrations/00013_quizzes.sql`
- **Implementation Summary**: Once per track per edition day the daily run writes five questions from the eight most important stories. Only well-formed questions (four options, a valid answer index) are kept, and at least three are required. `GET /quiz?track=&day=` returns the quiz (latest when day is omitted); the web page reveals answers client-side. The email links the quiz and, in the 7-minute read, lists the questions without answers.
- **Data & Schema Dependencies**: `quizzes(edition_day, track, questions jsonb)`; Gemini API.
- **Production Trade-Offs**: Answers are in the JSON, so a quiz is not a secret. Questions are model-written and unreviewed; check some before presenting them as exam practice. Needs `GEMINI_API_KEY`.

### Telegram channel posts
- **Status**: Active
- **Last Updated**: 2026-11-14 (d2638dd, uncommitted)
- **Entry Points / Key Files**:
  - `internal/telegram/telegram.go`: `Bot`, `Log`
  - `internal/digestmail/channels.go`: `RunChannels`, `telegramText`
  - `internal/db/migrations/00014_channel_posts.sql`
- **Implementation Summary**: After the email run, `senddigest` posts each track's one-minute edition to its channel (`TELEGRAM_CHAT_UPSC`, `TELEGRAM_CHAT_ENGINEERING`) once per channel per day, with the quiz link. Text is HTML-escaped and kept under Telegram's 4096-byte limit; errors never contain the bot token.
- **Data & Schema Dependencies**: `channel_posts`; Telegram Bot API.
- **Production Trade-Offs**: One-way broadcast only: there is no per-reader Telegram delivery. WhatsApp is not supported (its business API is paid).

### Next.js web frontend (Vercel)
- **Status**: Modified
- **Last Updated**: 2026-11-15 (5037a93, uncommitted)
- **Entry Points / Key Files**:
  - `web/app/layout.tsx`, `web/components/site.tsx`: the nameplate (`Masthead`: date numeral, wordmark between rules), the sticky `SectionBar` (sections, search, Subscribe) and the footer
  - `web/components/today.tsx`: client component for today's date, read in the browser so the static header bakes in no date
  - `web/app/globals.css`: the design system as CSS tokens (4px spacing scale, 1.2 type scale, three radii, two shadows, one accent; light and dark, all text pairs pass WCAG AA). Minimalist: flat cards with no boxes, hairline dividers, generous space
  - `web/components/primary-nav.tsx`: client component marking the current section or tool in the sections bar
  - `web/app/page.tsx`, `web/components/edition.tsx`, `views.tsx`, `article.tsx`, `hero.tsx`, `subscribe-form.tsx`
  - `web/app/story/[id]/page.tsx`: one story on its own page (summary, why it matters, tags, a button to the publisher, more from the section, per-story title and share tags)
  - `web/app/quiz`, `archive`, `archive/[day]`, `week`, `confirm`, `unsubscribe`, `not-found.tsx`
  - `web/lib/api.ts`: cached server-side fetches; `web/lib/safe.ts`: `safeURL`
- **Implementation Summary**: The public site. Pages fetch on the server from the Worker (`API_BASE_URL`), cached for 5 minutes with `use cache`; the browser never calls the API. The look is a newspaper: a nameplate with a large date numeral and the wordmark between double rules, a sticky text-only sections bar (Top stories, India, World, UPSC, Tech, AI, then Quiz, Archive, Weekly), and a strap line with the day's counts. The home page is a three-column front page divided by rules: text stories plus a newsletter box on the left, the MAIN STORY (labelled, with a large picture) and thumbnail rows in the middle, and numbered Top stories and Latest lists on the right; then a dark band of four, More top stories (with a Brief / Detailed switch), the signup band and a row per section with a See all button. Stories are ranked by importance. Cards and rows show the text in full. The story page and the cards share one fallback: the site's gradient tile shows under the picture and stays when there is no image or it fails to load. Type is Inter for UI and Newsreader for headlines, self-hosted by `next/font` at build time. The loading state is a skeleton of the page's layout. the nav offers editions, reading length, quiz, archive (one page per day, India time) and a printable weekly revision. Every card, briefing item and archive row opens the Postpaper story page first (`storyHref`); only the story page's button and the quiz's "Read the story" links go to the publisher. Feed text is always rendered as escaped React text and links pass `safeURL`.
- **Data & Schema Dependencies**: The Worker/API: `/digest`, `/digest/sources`, `/quiz`, `/story/{id}`, `/subscribe`, `/confirm`, `/unsubscribe`.
- **Production Trade-Offs**: A story page exists only while the page is in the database (about 60 days); an older link shows the not-found page. The page shows Postpaper's short summary only, never the publisher's text or the long summary. A story older than the Worker's 7-day snapshot is served by Render and can hit its cold start. The 7-minute mode and the archive use `detail=` and `day=`, which the Worker sends to Render, so they pay Render's cold start. `/archive/<bad date>` shows the not-found page with status 200 because the page streams. `next/font` fetches the fonts from Google while building, so a build with no network fails. The old static page in `internal/api/static` is still served by the Go API and the Worker.

### checksource tool
- **Status**: Active
- **Last Updated**: 2026-10-20 (b54e7a7)
- **Entry Points / Key Files**:
  - `cmd/checksource/main.go`: vetting command
  - `catalog/`: pre-vetted candidate sources with robots AI-policy notes
- **Implementation Summary**: Runs the crawler's own robots check, fetcher and parser against a URL with a handful of one-per-second requests. It reports robots and AI-crawler policy, whether the site serves real HTML, which prefixes hold articles, and whether sampled pages count as articles, then prints a `sources.json` entry and a WORKS, PARTIAL or NO verdict.
- **Data & Schema Dependencies**: None (no database).
- **Production Trade-Offs**: It does not bypass bot challenges, so a blocked site is reported as NO. Some publishers' robots.txt signal that AI use is not wanted; the `note` field records it and the digest summarises with a model, so read each site's terms first.
