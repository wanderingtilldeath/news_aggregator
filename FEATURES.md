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
- **Last Updated**: 2026-11-15 (a902c5b, uncommitted)
- **Entry Points / Key Files**:
  - `internal/store/store.go`: `Store` interface, `DigestQuery`, `Item`
  - `internal/store/postgres.go`: batched `Save`, `upsertSQL`, `Digest`, `DigestSources`, `hostSQL`, `categorySQL`
  - `internal/store/memory.go`: same filters in Go
  - `internal/db/migrations/`: 00001 to 00022 (`pages` and its columns, `subscribers`, `deliveries`, enrichment columns, subscriber preferences, `quizzes`, `channel_posts`, exam lens, `detailed_at`, story groups, subscriber topics, `intros`, `pro_members`, `compilations`)
- **Implementation Summary**: Saves are grouped into batches (group commit) and written with one upsert per row. The upsert skips rewriting a row when the content hash is unchanged, unless the category or `published_at` changed, so a parser date fix heals old rows on the next crawl. The digest orders by `COALESCE(published_at, first_seen_at)` and filters by window (pages dated more than a day ahead are left out by `digestWhere` and `digestLocked`), full-text query, source host, article flag and category. The host is derived from the URL in SQL, and the effective image is the og:image unless five or more articles share it, then the body image.
- **Data & Schema Dependencies**: `pages` (`story_id` and `coverage` for story grouping, `detailed_at` for the second enrichment stage, tsvector `search` with GIN index, `pages_recent_idx`, partial indexes for pending summaries and source images), `job_runs`.
- **Production Trade-Offs**: `DigestQuery` also has `Until` (one edition day) and `Detail` (long summaries). `Digest` returns each item's `coverage`; `Item` also returns the `also` list. `GroupCandidates` and `SetStories` serve story grouping, `GetIntro` and `SaveIntro` the daily intro. The `Store` interface now also holds `Item` (one article by id), subscribers, deliveries, quizzes, enrichment and channel posts. Postgres and Memory stores must stay identical; each filter is written twice and tested in both. `pages` stores full article text in `content`, and undated pages are never pruned, so the 1 GB Neon free tier is the long-term limit. `hostSQL` is NULL-safe because a non-http URL once made `/digest` return 500.

### Sources and daily scheduler
- **Status**: Modified
- **Last Updated**: 2026-11-15 (5f836f6, uncommitted)
- **Entry Points / Key Files**:
  - `sources.json`: 50 source entries, each with a `category` (adds ORF, Down To Earth and Scroll)
  - `internal/scheduler/sources.go`: `LoadSources`, validation, `{date:...}` and `days_back` expansion, `Specs`
  - `internal/scheduler/scheduler.go`: `Run`, `runIfClaimed`, `runOnce`, `waitForCrawls`, `enrich`
  - `internal/scheduler/claims.go`: one run per day via `job_runs`
  - `internal/scheduler/shipped_sources_test.go`: shipped file rules
- **Implementation Summary**: At `DIGEST_AT` (or at once with `RUN_ON_START`, or as a catch-up) the scheduler claims today's `job_runs` row, starts a crawl per source, and polls until every crawl has no active work, bounded by `CrawlTimeout`. While waiting it runs summary and image passes whenever a full batch is ready, then a final pass that waits out rate limits. After the summary passes it groups stories covered by several sources (`groupStories`, no model), runs the two enrichment stages, writes the day's quizzes and the "Today in 60 seconds" intros (`makeIntros`), adds the day's important stories to the month's compilation (`compileMonth`), then logs `digest run complete`, which the crawl workflow waits for.
- **Data & Schema Dependencies**: `job_runs`, `crawls`, `frontier`, `pages`.
- **Production Trade-Offs**: `MAX_SITES` makes startup fail above a site cap. The daily claim means a second start the same day does nothing unless `RUN_ON_START` is set. A test pins Indian Express in `sources.json` and requires a category on every entry. Drishti IAS and InsightsIAS were removed from `sources.json`; the `split_articles`, `image_from_source` and `article_prefixes` features remain for other sources. Seven engineering blogs (Heroku, Cisco, Canva, Salesforce, Snyk, Amazon Science, eBay) were added after passing `checksource`. Do not reword the `digest run complete` message without changing `.github/workflows/crawl.yml`.

### Summaries
- **Status**: Modified
- **Last Updated**: 2026-11-15 2026-11-15 (d65f638, uncommitted)
- **Entry Points / Key Files**:
  - `internal/summarize/summarize.go`: `Pass`, `ExtractiveGist`, `Summarizer`
  - `internal/prose/prose.go`: `Sentences`, `DropBoilerplate`, `Fragment`, `Lead` (shared by the summariser and the store)
  - `internal/summarize/gemini.go`: batched Gemini client with `Retry-After` handling
  - `cmd/crawlerx/main.go`: `newScheduler` picks the provider
- **Implementation Summary**: Each new article gets a short gist as up to five bullets of at most 20 words each (`maxBullets`, `maxBulletWords`); the Gemini prompt asks for the five most relevant facts, fewer only when the article gives fewer. Cards show the first bullet, the story page lists all of them with `web/components/bullets.tsx`: a bullet longer than 26 words is shown short first (`shorten` in `web/lib/view.ts`: the first 22 words, cut after a comma when half remain, never on a dangling word, then "…") with a "more" button that opens the whole sentence and "less" to close it, so nothing is thrown away and a cut never reads as the end of the thought (bullets cut by an older summariser, which end in "…", are not shown on the story page: it opens with `Item.lead` instead, the first three whole sentences (about 70 words) of the stored article text, set only by `Store.Item` through `prose.Lead`; a card tidies a publisher's cut meta description with `tidy` in `web/lib/view.ts`, which drops a trailing half word and ends with "…" when a 140-character or longer text does not end like a sentence), and the email keeps the first three (`maxEmailBullets`). The Gemini provider batches several articles per request and spaces calls to stay under the free quota. Articles under 400 characters skip the model, and an article the model omits gets the extractive gist. Before summarising, `dropBoilerplate` removes sentences that are page chrome (paywall and login prompts, newsletter sign-ups, navigation bar, video labels) from the description and text, for both providers; the Gemini prompt also says to ignore them. The extractive provider needs no key: it copies whole sentences (whole, up to `maxExtractWords`, 80 words, so a run-on sentence cannot fill the page), splits them with `endsSentence` (no split after "Dr.", initials, "U.S." or "Nov." when the next word is not a new sentence, and none before a dash or a lower-case word) and skips fragments that start with a dash, a comma or a lower-case letter. It cuts a longer sentence at a comma only when at least `minClauseWords` (12) words remain. `truncateWords` cuts a longer bullet at its last comma within the limit (or at the limit), never splits a word, never ends on a dangling word such as "and" or "to", and adds "…" unless the cut lands on a full stop. The Gemini prompt asks for one complete sentence of at most 20 words and the same cap is applied to its reply.
- **Data & Schema Dependencies**: `pages.summary`, `summarized_at`, `pages_unsummarized_idx`; Gemini API.
- **Production Trade-Offs**: Migration `00022_resummarise_truncated.sql` marks the articles of the last three days that have a bullet ending in "…" or "..." (and still have their text) as not summarised, so the next crawl run rewrites them with the whole-sentence logic; older articles keep their cut bullets, and a Gemini key makes the rewrite cost model calls once. A sentence longer than 80 words is still cut in storage. A stored gist is never regenerated, so stories summarised before the five-bullet change keep three bullets, and bullets cut under the old 220-character rule stay, until `summarized_at` is cleared. Five bullets raise the reply budget from 220 to 330 tokens per article (`tokensPerArticle`). A short or paywalled page can still give one or two bullets. On a rate limit the pass pauses, retries up to 3 times, then leaves the rest for the next run. Only article text is sent to Gemini, never images. The chrome pattern list is a heuristic (`boilerplate` in `summarize.go`); a site with other chrome text needs a pattern added. The crawl job now defaults to `auto`, which uses Gemini when the `GEMINI_API_KEY` secret is set. Live blogs (`parse.LooksLikeLiveBlog`) are kept out of the digest and "this blog is now closed" is dropped as chrome. `Gemini.generate` is the shared retrying call for gists, enrichment and quizzes.

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
- **Last Updated**: 2026-11-15 (0b4f392, uncommitted)
- **Entry Points / Key Files**:
  - `internal/api/api.go`: `NewHandler`, `digest`, `digestSources`, `search`, `createCrawl`, `healthz`
  - `internal/config/config.go`: `READ_ONLY`, `LISTEN_ADDR`
- **Implementation Summary**: `GET /digest` takes `days` (1 to 30), `day` (one edition day, India time), `detail`, `page`, `page_size` (max 200), `q`, `source`, `category` and `articles_only`, and returns items (with why, tags and importance when enriched) whose image paths are rewritten to `/img/{id}`. `GET /story/{id}` (one article by id, no long summary, with the full exam lens); items carry `source_type` from `internal/sourcetype` and, for UPSC stories, `rel`, `GET /quiz`, `POST /subscribe`, `/confirm` and `/unsubscribe` are also served (the subscriber routes stay on a `READ_ONLY` box). `GET /digest/sources` lists hosts with category and count. `POST /crawls` is only registered when `READ_ONLY` is off and rejects unknown JSON fields and bodies over the size cap. Digest responses carry `Cache-Control: public, max-age=300`.
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
- **Last Updated**: 2026-11-15 (84d95ed, uncommitted)
- **Entry Points / Key Files**:
  - `edge/worker.js`: `fromSnapshot`, `loadSnapshot`, `categoryOf`, proxy fallback
  - `edge/wrangler.toml`: `ORIGIN`, `html_handling = "none"`, `run_worker_first`
  - `scripts/build-snapshot.sh`: builds `snapshot.json`, images, `stories/`, `quiz/`, `intro/`, `cards.json`, `months.json`, `month/`, `sources.json`, page copies, `_headers`
  - `edge/telegram.js`: the Telegram bot (see its own entry)
- **Implementation Summary**: The crawl job starts the app read-only on loopback, pages through `/digest?days=7&detail=1` (so the few long summaries are in the snapshot), downloads each image (up to 1.5 MB, `SNAPSHOT_MAX_IMAGE_BYTES`) and writes everything into `edge/assets`, plus one `stories/{id}.json` per story (the same answer `/story/{id}` gives, image rewritten to the local copy). The Worker serves `/` and static files from assets, answers `/digest` and `/digest/sources` for the 1, 2 and 7 day views from the snapshot (and `/digest` with `detail=1`, or with `day=` when the whole India-time day is inside the snapshot window; `long` is stripped unless `detail` is asked for), and proxies everything else (30-day view, text search, dashboard) to Render. The parsed snapshot is cached per isolate.
- **Data & Schema Dependencies**: Reads the app's own API; writes static files only. Needs `CLOUDFLARE_API_TOKEN` and `CLOUDFLARE_ACCOUNT_ID`.
- **Production Trade-Offs**: The Worker re-implements the Go filtering, so a change to `/digest` filters must land in both. The snapshot also writes `quiz/{track}.json`, `quiz/{track}/{day}.json` and the same for `intro/` (the latest and the last nine days, India time): `/quiz` and `/intro` for the two known tracks are answered from them, and a missing file inside that window is an edge 404, with no origin call. A `day=` older than the snapshot window, a `day` that is not a real date, text search, `articles_only=false` and the 30-day view still go to the origin. The track list in `edge/worker.js` is a copy of `internal/track`. `/cards` (only the standard `days=14&limit=200` query), `/months` and `/month?month=&track=` are answered from `cards.json`, `months.json` and `month/{track}/{month}.json` (the last twelve months), else from the origin through the cache. `GET /story/{id}` is answered from `stories/{id}.json` in the assets (ids must match `[A-Za-z0-9_-]{1,128}`); a story not in the snapshot goes to the origin through Cloudflare's cache (six hours). The story files add one asset per story to the daily deploy (Cloudflare's free plan allows 20,000 files). Data changes only when the crawl job deploys. The worker CPU cost of parsing a large snapshot on the free plan has not been measured in production.

### Crawl job and deployment
- **Status**: Modified
- **Last Updated**: 2026-11-15 (84d95ed, uncommitted)
- **Entry Points / Key Files**:
  - `.github/workflows/crawl.yml`: crawl every 6 hours, prune, snapshot deploy, cache warm-up
  - `scripts/warm-caches.sh`: visits the sections, quiz, archive days, week, feed, sitemap and the 15 newest stories on the web site twice (the first visit makes Vercel refresh a stale page), so the crawl job pays any cold start
  - `Dockerfile`: distroless static image for the Render web service
  - `.env.demo.example`: web-service settings
  - `docs/DEPLOYMENT.md`: step-by-step guide
- **Implementation Summary**: Render runs the same binary serve-only (`WORKERS=0`, `READ_ONLY=true`, `GOMEMLIMIT=180MiB`). A GitHub Actions job runs the crawl with `WORKERS=16`, `GOMEMLIMIT=1500MiB` and `IMAGE_CACHE_MB=32` (per-host delay stays 1 s, so more workers do not load any one site harder), waits up to 40 minutes for `digest run complete`, stops the process, deletes crawls older than 3 days and pages published over 60 days ago, then builds and deploys the snapshot, then warms the caches (`scripts/warm-caches.sh`, when the `WEB_BASE_URL` variable is set), then runs `senddigest` (email and Telegram) when `MAIL_PROVIDER` is `brevo` or a Telegram channel is set. Migrations run on every start.
- **Data & Schema Dependencies**: Neon Postgres (direct connection, not the pooler), GitHub secrets, Cloudflare account.
- **Production Trade-Offs**: The warm-up step is `continue-on-error` and counts only HTTP 200; it adds about a minute to the job and does not warm pages a reader reaches by search or the 30-day view. The 45-minute job timeout against a 47-source crawl at 16 workers has not been measured. `MAX_SITES` is 60 in the workflow. `.env.demo.example` still lists old crawl-job comments (`WORKERS=2`, `MAX_SITES=6`) that differ from the workflow (`16`, `60`). The Dockerfile exposes 8080 while the demo listens on 10000.

### Subscribers and signup
- **Status**: Modified
- **Last Updated**: 2026-11-15 (5f836f6, uncommitted)
- **Entry Points / Key Files**:
  - `internal/api/subscribe.go`: `POST /subscribe`, `POST /confirm`, `POST /unsubscribe`, `limiter`
  - `internal/store/store.go`: `Subscribe`, `Confirm`, `Unsubscribe`, `SetPreferences`, `ConfirmedSubscribers`
  - `internal/db/migrations/00009_subscribers.sql`, `00012_subscriber_prefs.sql`, `00018_subscriber_topics.sql`
- **Implementation Summary**: Signup stores an unconfirmed subscriber with a random token and sends a confirmation mail; the reader presses a button on `/confirm` (a POST, so mail scanners cannot confirm). The answer is the same for new, pending and confirmed addresses. Choices (tracks, 1min/7min and up to five followed topics) are stored only while the address is unconfirmed. Topics are cleaned by `track.NormalizeTopics` (lower case, 40 characters, no duplicates) and never rejected. `POST /unsubscribe` takes the token in the query string or JSON, so it also serves the one-click header. Signups are capped at 30 a minute for the whole process.
- **Data & Schema Dependencies**: `subscribers` (email, token, confirmed_at, unsubscribed_at, tracks, mode, topics).
- **Production Trade-Offs**: The token is stored in plain text and never expires, because the unsubscribe link needs it. The limit is global and per process because the API only sees the web server's address. A subscriber who unsubscribed and signs up again gets a new token and must confirm again.

### Email (Brevo) and the daily send
- **Status**: Modified
- **Last Updated**: 2026-11-15 (5f836f6, uncommitted)
- **Entry Points / Key Files**:
  - `internal/mail/`: `Sender`, `Brevo`, `Log`, `ConfirmationMessage`, `ConfirmationNotifier`
  - `cmd/senddigest/main.go`, `internal/digestmail/`: edition building, rendering, `Run`
  - `internal/db/migrations/00010_deliveries.sql`
  - `.github/workflows/crawl.yml`: the "Email the daily digest" step
- **Implementation Summary**: Brevo is called over HTTPS because free Render services block SMTP. `senddigest` builds one edition per combination of tracks and reading length (cached), ranks stories by importance, caps three per site, and mails each confirmed subscriber once per edition day (a `deliveries` row after each send). A send cap (default 250), a rate-limit stop, and five consecutive failures stop a run. `-dry-run` prints one sample, `-to` sends one sample and records nothing. A subscriber who follows topics gets a "Your topics" section: for each word the newest stories matching the site's full-text search that the edition does not already hold (two a word, six in all, `topicSection`). Each track section opens with that track's "Today in 60 seconds" paragraph when one exists. One labelled sponsor line (`MAIL_SPONSOR_TEXT`, `MAIL_SPONSOR_URL`, http(s) only, `rel="sponsored"`) is added when both are set. Every mail carries an unsubscribe link and List-Unsubscribe headers.
- **Data & Schema Dependencies**: `subscribers` (topics), `deliveries`, `pages`, `quizzes`, `intros`; Brevo API.
- **Production Trade-Offs**: Brevo's free plan allows 300 mails a day, shared by confirmations and the digest. A crash between send and record can duplicate one mail. Whether Brevo passes `List-Unsubscribe-Post` through is unverified. The step runs only when `MAIL_PROVIDER` is `brevo` or a Telegram channel is set, because the log provider prints unsubscribe tokens.

### Enrichment: why it matters, tags, importance, long summary
- **Status**: Modified
- **Last Updated**: 2026-11-15 (5f836f6, uncommitted)
- **Entry Points / Key Files**:
  - `internal/summarize/enrich.go`: `EnrichBatch`/`EnrichPass` (stage 1), `DetailBatch`/`DetailPass` (stage 2), `parseEnrichment`, `parseDetail`
  - `internal/scheduler/scheduler.go`: `enrichArticles` (48 hours, 60 articles a run, `detailLimit` 5)
  - `internal/store/`: `PendingEnrichment`, `SetEnrichment`, `PendingDetail`, `SetDetail`, `Item.Why/Tags/Importance/Long`
  - `internal/db/migrations/00011_enrichment.sql`, `00015_exam_lens.sql`, `00016_detail_stage.sql`
- **Implementation Summary**: Two stages keep the Gemini bill small. Stage 1 sends five summarised articles per call (260 output tokens each) and returns a why-it-matters line, up to three tags (the first a UPSC syllabus area for india, world and upsc stories) and an importance score 1 to 5, for every recent story. Stage 2 runs once stage 1 is done and spends longer replies (three articles per call, 800 tokens each) on only the five most important enriched stories per run (`PendingDetail`: importance, then newest): a 90 to 130 word summary and, for the india, world and upsc categories, the exam lens (relevance to Prelims, Mains, Essay and Ethics 0 to 3, dimensions, arguments for and against, Prelims facts, a Mains answer skeleton, and 2 or 3 Prelims flashcards `{q, a}`: `maxCards`, the question at most 30 words and the answer 20). Replies are cleaned (tags match a pattern, scores clamped, lists capped, the lens dropped for other categories). The four scores sit in `pages.rel_*` and the rest in `pages.exam` (JSONB); lists carry only the scores (`rel`), `Store.Item` the whole lens. `detailed_at` marks stage 2 done; migration 00016 marks stories that already had a long summary or lens. A rate limit in stage 1 skips stage 2. `/digest` returns why, tags and importance; the long summary only with `detail=1`, which the Worker leaves to the origin.
- **Data & Schema Dependencies**: `pages.why`, `tags`, `importance`, `long_summary`, `exam`, `rel_*`, `enriched_at`, `detailed_at`; Gemini API.
- **Production Trade-Offs**: Only the top stories carry a long summary and an exam lens, so a lower-ranked UPSC story shows no lens and the 7-minute read falls back to the gist bullets for it. The lens is model-written and unreviewed, so the story page labels it AI-generated. Needs `GEMINI_API_KEY`; without it nothing is enriched and ranking falls back to recency. A skipped article stays pending for the next run. Importance is the model's opinion, not verified. Enrichment is limited to the last 48 hours and 60 articles per run to stay in the free quota.

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
- **Last Updated**: 2026-11-15 (0b4f392, uncommitted)
- **Entry Points / Key Files**:
  - `internal/summarize/quiz.go`: `GenerateQuiz`, `parseQuiz`, `QuizPass`
  - `internal/api/api.go`: `GET /quiz`
  - `web/app/quiz/page.tsx`, `web/components/quiz-player.tsx`
  - `internal/db/migrations/00013_quizzes.sql`
- **Implementation Summary**: Once per track per edition day the daily run writes five questions from the eight most important stories, with each story's extracted Prelims facts placed first so questions come from them. Only well-formed questions (four options, a valid answer index) are kept, and at least three are required. `GET /quiz?track=&day=` returns the quiz (latest when day is omitted); the web page reveals answers client-side and remembers wrong answers in the reader's browser (`pp-missed` in localStorage, 30 at most) for a "Revise the questions you missed" box on `/quiz`. The email links the quiz and, in the 7-minute read, lists the questions without answers.
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
- **Last Updated**: 2026-11-15 (5f836f6, uncommitted)
- **Entry Points / Key Files**:
  - `web/app/layout.tsx`, `web/components/site.tsx`: the nameplate (`Masthead`: date numeral, wordmark between rules), the sticky `SectionBar` (sections, search, Subscribe) and the footer
  - `web/components/exam-lens.tsx`: the story page's exam lens panel (relevance bars, dimensions, both sides, answer skeleton, Prelims fact box)
  - `web/components/logo.tsx`: the brand logo (the red and gold origami bird redrawn as twelve SVG facets from the supplied image, plus the "PostPaper" wordmark in Montserrat, slate `#444c5c` on light); the two darkest facets use `--logo-dark`, lightened in dark mode. `web/app/icon.svg` is the favicon and `web/app/opengraph-image.png` (1200x630, made from the supplied image) the default share image
  - `web/components/today.tsx`: client component for today's date, read in the browser so the static header bakes in no date
  - `web/app/globals.css`: the design system as CSS tokens (4px spacing scale, 1.2 type scale, three radii, two shadows, one accent, the logo's crimson `#c8213a` (coral-pink `#f0475d` in dark); light and dark, all text pairs pass WCAG AA). Minimalist: flat cards with no boxes, hairline dividers, generous space
  - `web/components/primary-nav.tsx`: client component marking the current section or tool in the sections bar
  - `web/app/page.tsx`, `web/components/edition.tsx`, `views.tsx`, `article.tsx`, `hero.tsx`, `subscribe-form.tsx`
  - `web/app/story/[id]/page.tsx`: one story on its own page (summary, why it matters, tags, a button to the publisher, more from the section, per-story title and share tags)
  - `web/app/flashcards`, `underreported`, `month`, `sources`, `pro` (see their own entries)
  - `web/app/quiz`, `archive`, `archive/[day]`, `week`, `confirm`, `unsubscribe`, `not-found.tsx`
  - `web/components/briefing.tsx`: the "Today in 60 seconds" box on the home page; `read-aloud.tsx` (browser speech, no server), `streak.tsx` and `revise.tsx` (localStorage via `web/lib/local.ts`), `share.tsx` (WhatsApp, Telegram, X anchors), `sponsor.tsx`
  - `web/app/sitemap.ts`, `robots.ts`, `rss.xml/route.ts` (`?category=`); `web/lib/site.ts`: `SITE_URL`
  - `web/lib/api.ts`: cached server-side fetches; `web/lib/safe.ts`: `safeURL`
- **Implementation Summary**: Every byline and meta row shows the publisher type (Official, Think tank, News, Analysis, Company blog, Project blog) and UPSC cards and the main story show the two papers a story is most relevant to as chips ("Prelims ●●●"). The story page ends with the exam lens for UPSC stories, labelled AI-generated. The public site. Pages fetch on the server from the Worker (`API_BASE_URL`), cached with `use cache` (fresh 30 minutes, stale up to a day; the data changes only when the crawl runs); the browser never calls the API. The look is a newspaper: a nameplate with a large date numeral and the logo between double rules, a sticky text-only sections bar (Top stories, India, World, UPSC, Tech, AI, then Quiz, Archive, Weekly), and a strap line with the day's counts. The home page is a three-column front page divided by rules: text stories plus a newsletter box on the left, the MAIN STORY (labelled, with a large picture) and thumbnail rows in the middle, and numbered Top stories and Latest lists on the right; then a dark band of four, More top stories (with a Brief / Detailed switch), the signup band and a row per section with a See all button. Stories are ranked by importance. Cards and rows show the text in full. The story page and the cards share one fallback: the site's gradient tile shows under the picture and stays when there is no image or it fails to load. Type is Inter for UI and Newsreader for headlines, self-hosted by `next/font` at build time. The loading state is a skeleton of the page's layout. the nav offers editions, reading length, quiz, archive (one page per day, India time) and a printable weekly revision. Every card, briefing item and archive row opens the Postpaper story page first (`storyHref`); only the story page's button and the quiz's "Read the story" links go to the publisher. Cards show the story's kind (Opinion, Analysis, Explainer, from the API), "Covered by N sources" and "Single source" (an importance 4 or 5 story only one of our sources carried); the story page lists "Also covered by" the other sources' stories. The signup form takes followed topics. Reading streak (days in a row the reader reached the end of the edition) and missed quiz questions live in the reader's browser only. Environment: `SITE_URL` (share links, sitemap, feed), `SPONSOR_TEXT`/`SPONSOR_URL`/`SPONSOR_LABEL` (one labelled sponsor line above the footer) and `CF_ANALYTICS_TOKEN` (Cloudflare Web Analytics beacon); all three are read when the site is built, so changing them needs a redeploy. Feed text is always rendered as escaped React text and links pass `safeURL`.
- **Data & Schema Dependencies**: The Worker/API: `/digest`, `/digest/sources`, `/quiz`, `/intro`, `/story/{id}`, `/subscribe`, `/confirm`, `/unsubscribe`.
- **Production Trade-Offs**: The sitemap lists the stories of the last week from one cached call; if the API is down while the page is generated it lists only the fixed pages until the next refresh. A story page exists only while the page is in the database (about 60 days); an older link shows the not-found page. The page shows Postpaper's short summary only, never the publisher's text or the long summary. A story older than the Worker's 7-day snapshot is served by Render and can hit its cold start. The 7-minute mode and the archive use `detail=` and `day=`, which the Worker sends to Render, so they pay Render's cold start. `/archive/<bad date>` shows the not-found page with status 200 because the page streams. `next/font` fetches the fonts from Google while building, so a build with no network fails. The old static page in `internal/api/static` is still served by the Go API and the Worker.

### Story grouping (covered by several sources)
- **Status**: Active
- **Last Updated**: 2026-11-15 (5f836f6, uncommitted)
- **Entry Points / Key Files**:
  - `internal/group/group.go`: `Cluster`, `Group`
  - `internal/scheduler/scheduler.go`: `groupStories`
  - `internal/store/`: `GroupCandidates`, `SetStories`, `Item.Coverage`, `Item.Also`
  - `internal/db/migrations/00017_story_groups.sql`
  - `web/app/underreported/page.tsx`, `web/lib/view.ts` (`coverageByKind`)
- **Implementation Summary**: After the crawl the scheduler compares the headlines of the last four days (at most 1,500, newest first) with no model and no network. Two stories are the same event when they come from different hosts, are under 72 hours apart and share at least three distinctive words that make up half of the shorter headline (or 45 percent of all words); matches are joined transitively. A group needs two or more hosts. `SetStories` rewrites `pages.story_id` and `coverage` for that window in one transaction, clearing stories that no longer match. The digest shows `coverage`; `/story/{id}` also lists up to eight other stories of the event, each with its publisher type (`source_type`, filled in by the API), and the story page says who reported it by kind ("Reported by 1 official body, 1 news outlet", `coverageByKind` in `web/lib/view.ts`). `/underreported` (`web/app/underreported/page.tsx`) lists stories of the last two days with importance 4 or 5 and no coverage, read from the cached digest, so it adds no load.
- **Data & Schema Dependencies**: `pages.story_id` (partial index), `pages.coverage`.
- **Production Trade-Offs**: Headline similarity misses the same event worded differently and can join two similar events; it is a hint, not an editorial judgement. The comparison is quadratic, hence the cap. "Single source" means only one of the sources Postpaper reads carried the story, not that nobody else did. A story older than the window keeps its last grouping. The Underreported page repeats that caveat: a story can be listed because another outlet covered it in different words, or because we do not read that outlet.

### Kind labels (opinion, analysis, explainer)
- **Status**: Active
- **Last Updated**: 2026-11-15 (5f836f6, uncommitted)
- **Entry Points / Key Files**:
  - `internal/kind/kind.go`: `For`
  - `internal/api/api.go`: `kindOf`, set on `/digest` and `/story/{id}` items
  - `web/lib/view.ts`: `KINDS`; `web/components/article.tsx`: `Signals`
- **Implementation Summary**: A story is labelled from words in its URL path (`opinion`, `editorial`, `column`, `explained`, `analysis`, ...), matched on whole path segments or a segment prefix before a hyphen. Opinion wins over explainer over analysis. Nothing is stored and no model is used; an ordinary report has no label.
- **Data & Schema Dependencies**: None.
- **Production Trade-Offs**: Only publishers that put the kind in the path are labelled; others are shown as plain news. The label is computed per response, so it also appears on old rows.

### Daily intro ("Today in 60 seconds")
- **Status**: Active
- **Last Updated**: 2026-11-15 (5f836f6, uncommitted)
- **Entry Points / Key Files**:
  - `internal/summarize/intro.go`: `Gemini.Intro`, `cleanIntro`, `IntroPass`
  - `internal/scheduler/scheduler.go`: `makeIntros`; `cmd/crawlerx/main.go` wires `Deps.Introducer`
  - `internal/store/`: `GetIntro`, `SaveIntro`; `internal/api/api.go`: `GET /intro`
  - `internal/db/migrations/00019_intros.sql`
  - `web/components/briefing.tsx`; the email section in `internal/digestmail/render.go`
- **Implementation Summary**: Once per track per edition day, after the quizzes, one Gemini call turns the six most important headlines and their why-it-matters lines (no article text, about 160 output tokens) into a paragraph of at most 90 words. A track with fewer than three stories is skipped and a failure is logged without stopping the run. `GET /intro?track=&day=` returns it (latest when `day` is omitted). The home page shows it above the front page with a Listen button; the email opens each track with it.
- **Data & Schema Dependencies**: `intros(edition_day, track, body)`; Gemini API.
- **Production Trade-Offs**: Two small calls a day. The paragraph is model-written and unreviewed. It is written once a day, from the day's first run, so later crawls do not update it. Without `GEMINI_API_KEY` there is no intro and the box is hidden.

### Sources transparency and takedown
- **Status**: Active
- **Last Updated**: 2026-11-15 (5dbaf34, uncommitted)
- **Entry Points / Key Files**:
  - `web/app/sources/page.tsx`: the public page (what we do with a story, a table of the sites with each robots.txt AI policy, how to ask for removal)
  - `scripts/build-snapshot.sh`: writes `sources.json` (host, sections, categories, note, `policy`) into the assets; blocked hosts are left out
  - `internal/scheduler/sources.go`: `DropBlocked`; `internal/config/config.go`: `BlockedHosts` (`BLOCKED_HOSTS`)
  - `.github/workflows/crawl.yml`: the "Prune old rows" step deletes a blocked host's pages
  - `web/lib/site.ts`: `CONTACT_EMAIL`
  - `docs/PRO_BOT_TAKEDOWN.md`
- **Implementation Summary**: A takedown is one setting. `BLOCKED_HOSTS` (comma separated, `www.` and case ignored) makes the scheduler drop every source on that host and logs a warning; the crawl job's prune step deletes the host's stored pages with a SQL match on the URL's host; the snapshot script leaves it out of `sources.json`, so the next deploy has none of its stories and no row on `/sources`. The page reads `sources.json` from the Worker's assets, derives "Asks AI users to keep out" from the `note` text in `sources.json` (a robots.txt disallow or an explicit `=no`), and gives `CONTACT_EMAIL` for takedowns and corrections.
- **Data & Schema Dependencies**: `sources.json` (the `note` field), `pages.url`; no new table.
- **Production Trade-Offs**: The AI-policy column is a copy of what robots.txt said when the source was added, not a live check, and it depends on every restricted site having a `note`. A takedown takes effect at the next crawl run (a few hours), not instantly; the Render service keeps serving old rows until then, and `sub.example.com` is a different host from `example.com`. The delete matches the host exactly. `CONTACT_EMAIL` defaults to `digest@postpaper.in`; point it at a mailbox you read. The page makes no legal promise beyond "usually within a day".

### Telegram bot commands
- **Status**: Active
- **Last Updated**: 2026-11-15 (5dbaf34, uncommitted)
- **Entry Points / Key Files**:
  - `edge/telegram.js`: `replies` (commands `/today`, `/topic`, `/quiz`, `/brief`, `/help`)
  - `edge/worker.js`: `telegramWebhook` (route `POST /telegram/webhook`)
  - `edge/wrangler.toml`: `WEB_URL`; secrets `TELEGRAM_BOT_TOKEN`, `TELEGRAM_WEBHOOK_SECRET`
  - `docs/PRO_BOT_TAKEDOWN.md`: setup and `setWebhook`
- **Implementation Summary**: Telegram posts each message to the Worker, which checks the `X-Telegram-Bot-Api-Secret-Token` header against `TELEGRAM_WEBHOOK_SECRET` (404 on a mismatch or when the secrets are missing) and answers 200 at once. The replies are sent afterwards with `ctx.waitUntil` through the Bot API. `/today [upsc|engineering]` lists the five most important stories of the last two days with a link to each story page; `/topic <words>` matches every word against title, description, why and tags over the 7-day snapshot; `/quiz` sends the day's questions as Telegram quiz polls (the right answer shows after the pick); `/brief` sends the "Today in 60 seconds" text. All data comes from the snapshot and the quiz and intro files, so no database, model or Render call is made.
- **Data & Schema Dependencies**: The published assets (`snapshot.json`, `quiz/`, `intro/`); Telegram Bot API.
- **Production Trade-Offs**: Content is as fresh as the last crawl deploy. The `TRACK_CATEGORIES` list is a copy of `internal/track`. Search is a substring match, not full text. Telegram keeps retrying a webhook that does not answer 200, so the handler never fails after the secret check. The Worker free plan allows 100,000 requests a day, shared with the site. A group chat gets answers only to commands. This is separate from the channel posts, which `senddigest` makes.

### Pro membership (Razorpay)
- **Status**: Active
- **Last Updated**: 2026-11-15 (35321dd, uncommitted)
- **Entry Points / Key Files**:
  - `internal/api/billing.go`: `POST /billing/razorpay` (`billing`), `GET /pro` (`proStatus`)
  - `internal/store/`: `ProExtend`, `ProByToken`, `ActiveProTokens`, `ProMember`
  - `internal/db/migrations/00020_pro_members.sql`
  - `internal/mail/pro.go`: `ProWelcomeMessage`, `ProNotifier`
  - `internal/digestmail/run.go`, `render.go`: Pro first, no sponsor, printable links
  - `web/app/pro/page.tsx`, `web/app/pro/edition/page.tsx`, `web/lib/api.ts` (`getProStatus`), `web/lib/site.ts` (`PRO_PLANS`, the two payment links and prices)
  - `docs/PRO_BOT_TAKEDOWN.md`
- **Implementation Summary**: A reader pays through a Razorpay Payment Link (no API key or checkout code here). Razorpay's webhook is accepted only when its HMAC-SHA256 over the raw body, made with `RAZORPAY_WEBHOOK_SECRET`, matches `X-Razorpay-Signature` (400 otherwise; the route is not served without the secret). A `payment_link.paid` or `payment.captured` event records the payment id in `pro_payments` and extends `pro_members.until` from the later of now and the current end by the days the amount paid buys (`PRO_PLANS`, paise to days, default `9900:31,24900:92`: 99 rupees a month, 249 rupees three months), in one transaction; an amount in no plan is logged and ignored; the payment id makes the two events and any retry count once. The payer's email comes from the payment, else from the payment link's customer. A new payment sends a welcome mail with the member's private link in the background. `senddigest` loads the active members, sends their mail first (so a send cap or rate limit never leaves one for the next run), leaves out the sponsor line and adds links to `/pro/edition?token=&day=` and `&scope=week`. That web page is a printable edition (the briefing, the ranked stories with the five bullets and long summary, and the quiz with answers; or the week) that the reader saves as a PDF; it asks `GET /pro` first and shows a message for an unknown, expired or unreachable link.
- **Data & Schema Dependencies**: `pro_members(email, token, until)`, `pro_payments(payment_id, email)`; Razorpay webhooks; Brevo for the welcome mail.
- **Production Trade-Offs**: The plans are matched by amount in paise, so a price change needs `PRO_PLANS` on Render and the new links on Vercel together; 249 rupees for 92 days is about 16% below three monthly payments (297). The token is stored in plain text and is the only credential: whoever has the link can open the printable pages, which hold no secret data, so the gate is soft. A member must subscribe to the email with the address they paid with to get the Pro mail; the match is by lowercased email. There is no auto-renewal, refund handling, cancel flow or invoice: a refund does not shorten `until`, and `refund` events are ignored. Razorpay needs a verified business account. The webhook goes through the Worker to Render, which may be asleep: Razorpay retries for a day. Flashcards and synced streaks are not built, so the Pro page lists only ad-free email, printable editions and priority delivery. Check Vercel Hobby's terms for commercial use before charging.

### Flashcards (spaced repetition)
- **Status**: Active
- **Last Updated**: 2026-11-15 (a902c5b, uncommitted)
- **Entry Points / Key Files**:
  - `internal/summarize/enrich.go`: the `cards` field of the detail prompt, `parseDetail`
  - `internal/store/`: `Cards`, `StoryCards`, `Card`, `ExamLens.Cards`
  - `internal/api/cards.go`: `GET /cards` (`days` 1 to 30, default 14; `limit` up to 300, default 120)
  - `scripts/build-snapshot.sh` writes `cards.json`; `edge/worker.js` serves it
  - `web/app/flashcards/page.tsx`, `web/components/flashcards.tsx`
- **Implementation Summary**: Stage 2 enrichment (the top five stories of a run, UPSC categories only) also asks for 2 or 3 flashcards per story, each a short question and answer. `Store.Cards` returns the stories in the window with cards or facts, most important first. The deck is each card as `storyId:position`; a story written before cards existed contributes its Prelims facts as "Recall a key fact from: title" cards. The page uses the Leitner system in the reader's browser (`pp-cards` in localStorage): a right answer moves a card up a box and it comes back after 1, 3, 7, 14 and 30 days, a miss sends it to box 0 for ten minutes and asks it once more at the end of the sitting. A sitting is at most 12 cards, due ones first, then new ones. Nothing is sent to the server.
- **Data & Schema Dependencies**: `pages.exam` JSON (`cards`, `facts`); no new table.
- **Production Trade-Offs**: Cards exist only for the few stories that get stage 2, so the deck grows by about 10 to 20 cards a day and is empty until the first run after deploy. They are model-written and unreviewed, and the page says so. Progress lives in one browser and is lost when storage is cleared; there is no sync (an account feature). The reply budget for stage 2 rose from 800 to 900 tokens per article. The fact-only cards are weak questions; they fade out as new cards replace them.

### Monthly compilation
- **Status**: Active
- **Last Updated**: 2026-11-15 (a902c5b, uncommitted)
- **Entry Points / Key Files**:
  - `internal/compile/compile.go`: `Merge`, syllabus areas and their order
  - `internal/scheduler/scheduler.go`: `compileMonth` (36 hour window)
  - `internal/store/`: `GetCompilation`, `SaveCompilation`, `CompilationMonths`, `Compilation`, `CompArea`, `CompItem`
  - `internal/api/compilation.go`: `GET /month?month=YYYY-MM&track=`, `GET /months`
  - `internal/db/migrations/00021_compilations.sql`
  - `scripts/build-snapshot.sh` writes `months.json` and `month/{track}/{month}.json`; `edge/worker.js` serves them
  - `web/app/month/page.tsx`, `web/app/month/[month]/page.tsx`
- **Implementation Summary**: Each daily run adds the last 36 hours' stories with importance 3 or more to the current India-time month's compilation of each edition, with no model call. A story goes to an area by its first tag: the nine UPSC syllabus areas in syllabus order for the UPSC edition (anything else is "Other"), the tag as a topic name for the engineering edition. `Merge` keeps a story once (the newer copy wins, and a story whose tags changed moves area), keeps the 12 most important per area and lists areas in reading order. The result is stored as JSON per month and edition, with the title, link, source, why-it-matters line, three gist bullets, tags and score, so it outlives the pages (deleted after 60 days). The web page prints the month by edition and area, each title linking to the publisher.
- **Data & Schema Dependencies**: `compilations(month, track, areas jsonb)`.
- **Production Trade-Offs**: It holds only what the daily runs saw, so a month starts at the first run after deploy and a missed run leaves a gap. A story below importance 3, or one the model did not score, is never listed. The cap of 12 per area drops older, lower-scored stories as the month fills. The stored text is a copy of model summaries; check a fact against the source. The current month changes every day until it ends, and the page is cached for up to 30 minutes.

### Source types
- **Status**: Active
- **Last Updated**: 2026-11-15 (0b4f392, uncommitted)
- **Entry Points / Key Files**:
  - `internal/sourcetype/sourcetype.go`: `For(host)`, the host-to-type map
  - `internal/scheduler/shipped_sources_test.go`: `TestEveryShippedSourceHasAType`
  - `internal/api/api.go`: sets `source_type` on digest items and stories
- **Implementation Summary**: Every source host in `sources.json` has a publisher type: official, think-tank, news, analysis, company or project. The API adds it to each item, and the site shows it next to the source name so a reader can weigh a claim. It is objective (a fact about the publisher), not a judgement of a story's lean.
- **Data & Schema Dependencies**: None stored; derived from the host at read time.
- **Production Trade-Offs**: Adding a source means adding its host to the map or the shipped-sources test fails. A source whose host is missing shows no badge rather than an error.

### checksource tool
- **Status**: Active
- **Last Updated**: 2026-10-20 (b54e7a7)
- **Entry Points / Key Files**:
  - `cmd/checksource/main.go`: vetting command
  - `catalog/`: pre-vetted candidate sources with robots AI-policy notes
- **Implementation Summary**: Runs the crawler's own robots check, fetcher and parser against a URL with a handful of one-per-second requests. It reports robots and AI-crawler policy, whether the site serves real HTML, which prefixes hold articles, and whether sampled pages count as articles, then prints a `sources.json` entry and a WORKS, PARTIAL or NO verdict.
- **Data & Schema Dependencies**: None (no database).
- **Production Trade-Offs**: It does not bypass bot challenges, so a blocked site is reported as NO. Some publishers' robots.txt signal that AI use is not wanted; the `note` field records it and the digest summarises with a model, so read each site's terms first.
