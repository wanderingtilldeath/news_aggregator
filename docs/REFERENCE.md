# CrawlerX reference

Full configuration, deploy and source-handling notes. The overview is in the [README](../README.md).

A resumable, polite web crawler in Go. Work lives in Postgres (a lease-based queue using `FOR UPDATE SKIP LOCKED`), and per-host politeness is enforced in-process. Kill it with `kill -9` and a restart finishes its work without re-crawling pages that were already done. Run one crawler per database.

**Only crawl your own site, or sites that allow it.** The API has no authentication and `POST /crawls` makes the server fetch URLs: keep it on a private network.

## Quick start

You need Go and a Postgres database. No Redis, Docker or other services.

```sh
export DATABASE_URL=postgres://user:pass@localhost:5432/crawlerx?sslmode=disable
go run ./cmd/crawlerx &                   # API on :8080; add -sources sources.json for the daily digest
curl -XPOST localhost:8080/crawls -d '{"seed":"https://example.com/","max_depth":2}'
curl localhost:8080/crawls/1              # counts by status, "done": true when finished
curl 'localhost:8080/search?q=nebula'
open http://localhost:8080                # the news page (The Daily Digest)
open http://localhost:8080/dashboard.html # crawl dashboard: status breakdown, throughput, depth chart, search
curl localhost:8080/healthz
```

To **resume** after a crash, restart `crawlerx` *without* `-seed`: it works whatever is pending. (`-seed` starts a new crawl.)

## How it works

| piece | what it does |
|---|---|
| `internal/frontier` | queue + visited set in one table; claim with `SKIP LOCKED`; expired leases are re-claimed (crash recovery); shallowest-first so the depth limit means "within N hops" |
| `internal/politeness` | in-process gate: one slot per host per delay; a denied worker reschedules the URL instead of sleeping |
| `internal/robots` | RFC 9309 via `temoto/robotstxt`: Allow/Disallow, wildcards, Crawl-delay; 4xx = allow, 5xx = disallow; cached in memory |
| `internal/fetch` | size cap (2 MiB), HTML only, honest User-Agent, no inline redirects (a redirect target is queued like any link), `Retry-After`, full-jitter backoff |
| `internal/parse`, `internal/urlnorm` | one parse per page; `<base href>`, meta robots; URL canonicalisation with a fuzz test |
| `internal/store` | batched page writes, `tsvector` + GIN full-text search, ETag/Last-Modified |

Config is flags with env fallbacks: see `go run ./cmd/crawlerx -h`.

## Tests

```sh
go test -short ./...                      # no services needed
TEST_DATABASE_URL=postgres://... make test  # all tests, -race, needs a disposable Postgres
```

The suite includes the original deadlock regression (`internal/crawler/deadlock_test.go`) and end-to-end `kill -9` / SIGTERM tests (`internal/e2e`). Concept notes: [docs/CONCEPTS.md](CONCEPTS.md). How the removed Redis, Prometheus, fakesite-as-a-service and Docker pieces worked: [docs/INFRA_FEATURES_INTERVIEW.md](INFRA_FEATURES_INTERVIEW.md).

## Deploy (free demo: Render + Neon + GitHub Actions + Cloudflare Worker)

A serve-only web service, a scheduled crawl job and an edge cache, all on free tiers:

- **Database:** a Neon project. Use the *direct* connection string (host without `-pooler`): the pgx statement cache and goose's advisory lock fail behind transaction pooling.
- **Web (Render, Docker):** build from `Dockerfile`; env in `.env.demo.example` (`WORKERS=0`, `READ_ONLY=true`, no `SOURCES`). `READ_ONLY` removes `POST /crawls`, so the public service cannot be told to fetch URLs. Health check path: `/digest.html` (not `/healthz`, which pings the database).
- **Crawl job (GitHub Actions):** `.github/workflows/crawl.yml` runs every 6 hours with `sources.json` (47 sources, 46 sites), waits for `digest run complete`, prunes old rows, then builds the edge snapshot and deploys it (below). Repo secrets: `DATABASE_URL`, `CLOUDFLARE_API_TOKEN` (token template "Edit Cloudflare Workers"), `CLOUDFLARE_ACCOUNT_ID`, optional `GEMINI_API_KEY`. Repo variables, all optional: `SUMMARY_PROVIDER` (`extractive` by default, or `auto`), `SUMMARY_WINDOW_DAYS` (30; use 7 with Gemini). Run it once by hand (`workflow_dispatch`) to publish the first snapshot; the default branch must be `main` for the schedule to run. Without the two Cloudflare secrets the crawl still runs and the snapshot step is skipped.
- **Edge snapshot (Cloudflare Worker):** `scripts/build-snapshot.sh` starts the app read-only on loopback and saves the last 7 days of `/digest`, their images (each up to 400 KB; larger pictures show the site tile) and `digest.html` into `edge/assets/`; `wrangler deploy` publishes them with `edge/worker.js`. The Worker answers `/digest` and `/digest/sources` for the 1, 2 and 7 day views from that snapshot, and everything else (30-day view, text search, the dashboard) from the Render URL in `edge/wrangler.toml`. A deploy replaces every file at once, so the data changes only when the scheduler has run. Visitors never wait for Render or Neon on the main views. Share the Worker URL.

Measured locally (Go 1.26, extractive summaries): the demo crawl takes about 75 s and stores about 440 pages; the web process idles at about 15 MB; the image is about 25 MB.

## Deploy (one small server: binary + systemd + nginx)

Memory is the constraint, so there is no Docker or Redis: one static binary and Postgres.

1. Build on your machine and copy: `make build-linux`, then `scp bin/crawlerx sources.json server:/opt/crawlerx/` (use `GOARCH=arm64` for ARM). Migrations and the web UI are embedded in the binary.
2. `/etc/crawlerx.env` (mode 600): `DATABASE_URL`, `LISTEN_ADDR=127.0.0.1:8080`, `SOURCES=/opt/crawlerx/sources.json`, `GEMINI_API_KEY`, `WORKERS=4`, `IMAGE_CACHE_MB=32`, `USER_AGENT=CrawlerX/1.0 (+your contact URL)`.
3. systemd unit: `EnvironmentFile=/etc/crawlerx.env`, `ExecStart=/opt/crawlerx/crawlerx`, `Restart=always`, `MemoryMax=300M`, run as a dedicated user.
4. nginx: `proxy_pass http://127.0.0.1:8080;` on its own host name, with `auth_basic` (the API has no authentication and `POST /crawls` makes the server fetch URLs) and TLS. The pages use absolute paths, so serve them from the root of a host, not a sub-path.
5. Add a 1 GB swapfile on a 512 MB server. Expect ~60 MB for crawlerx with `IMAGE_CACHE_MB=32`.
6. Update: `make build-linux`, `scp`, `systemctl restart crawlerx` (graceful shutdown; pending work resumes).

## News digest

Point crawls at news or notification listing pages (a shallow `max_depth`, 1 to 2, works well), then read what is new:

```
curl -XPOST localhost:8080/crawls -d '{"seed":"https://example.gov/press-releases","max_depth":1}'
curl 'localhost:8080/digest?days=1'              # last 24h, newest first (articles only; see "Daily digest")
curl 'localhost:8080/digest?days=7&q=exam+notice' # last week, filtered by full-text query
```

Each item has `url, title, description, snippet, source, image, summary, is_article, time`. `time` is the article's published date taken from page metadata (`article:published_time`, `datePublished`, or the first `<time datetime>`) or the URL; pages without one use the moment they were first crawled. By default only articles (pages with their own published date) are returned; pass `articles_only=false` to include every crawled page. Params: `days` 1-30, `page` (from 1) and `page_size` (1-200, default 24; `limit` is an alias), `q` and `source` (a host such as `indianexpress.com`) optional. The response carries `page`, `page_size`, `total` and `total_pages`. `GET /digest/sources?days=` lists the hosts in the window with their item counts. `GET /search?q=` is paginated the same way (`page`, `page_size` up to 100).

## Daily digest (scheduler, gist, review page)

`crawlerx` can run a daily job: crawl a list of news sites, write a short gist for each new article, and show the result as cards you can skim.

1. **Pick sources.** Edit [`sources.json`](../sources.json). Each entry is `{name, seed, max_depth, path_prefixes}`: the seed is a listing page, `max_depth` 1 reaches the articles it links to, and `path_prefixes` keeps the crawl to the links you want (the seed itself is always fetched). Adding a site is one more entry; restart to pick it up. Only add sites whose robots.txt and terms allow crawling. Four optional fields handle awkward sites:
   - `article_prefixes`: only pages under these paths count as articles, whatever dates they carry. Use it when article pages have no machine-readable date (Drishti IAS) and their listing pages must not count.
   - `split_articles`: a page holding several `<article id="...">` topics becomes one item per topic, linked as `page#id` (InsightsIAS daily posts). The page itself is then not an article.
   - `image_from_source`: for sites whose own pages carry only a generic logo, take the image from the original story the article cites (the first outside link in its text, e.g. a PIB or The Hindu page). After each crawl the daily job makes one small request per new article to that page and reads its `og:image`, honouring its robots.txt, one request a second. If the source has no image, the article keeps its own. The fetch refuses loopback and private addresses (`-allow-private-sources` lifts that, for tests against a local site only).
   - `days_back` with a `{date:LAYOUT}` seed, e.g. `.../news-analysis/{date:02-01-2006}` (Go time layout): each run crawls today and the previous `days_back` days, so a first run stays recent instead of crawling the whole archive.
2. **Optional: a free Gemini key** from [Google AI Studio](https://aistudio.google.com/apikey) gives better gists. `cp .env.example .env` and set `GEMINI_API_KEY`. With no key, the gist is the meta description plus the first sentences of the article.
3. **Run it.** `crawlerx -sources sources.json` runs the scheduler (see Deploy below). The job runs daily at `DIGEST_AT` (default 06:00, `TIMEZONE` default `Asia/Kolkata`). If you start after that time and today's run has not happened, it runs immediately. A per-day row in `job_runs` keeps it to one run per day even if started twice. Add `RUN_ON_START=true` to force a run now.
4. **Review.** Open http://localhost:8080: a newspaper-style front page with a lead story, the latest headlines and one block per category (India, World, UPSC, Tech, AI), each article with image, title, source, gist bullets and a link; section pages filter by source, and there is a period selector and text search. (`/digest.html` shows the same page.) The same data is `GET /digest` (`days`, `limit`, `q`, `source`, `articles_only`, default `true`), now with `image`, `summary`, `source` and `is_article`.

How it decides what is an article: by default a page counts when it has its own published date, from `article:published_time`, `datePublished`, or a date in the URL (`/2026/10/19/`, `/19-10-2026`). Listing, tag and filter pages never do, so they stay out of the digest. A source with `article_prefixes` uses that rule instead, and undated articles are dated by the day they were first crawled. Sites with neither a date nor an `article_prefixes` rule will not appear.

Images are linked, not downloaded. An article's image is, in order: the topic's own `<img>` (split topics), the image of the cited source (`image_from_source`), then the page's `og:image` or `twitter:image`. Sites with none give a placeholder card.

Locally: `go run ./cmd/crawlerx -sources sources.json -run-on-start` (flags: `-sources`, `-digest-at`, `-timezone`, `-run-on-start`, `-summary-provider auto|gemini|extractive`, `-summary-model`; the key is `GEMINI_API_KEY` in the environment only).

Free-tier notes: Gemini Flash-Lite's free limits (about 15 requests a minute and 1,000 a day at the time of writing; check AI Studio) are plenty for a few hundred articles a day, and the job spaces its calls to stay under them. Google may use free-tier prompts to improve its products, which is fine for public news text. If a quota runs out mid-run, the remaining articles stay unsummarised until the next run and the page shows their description.

Real sites are crawled with `HOST_DELAY` 1s by default.

### Speed and call budget

A run is built to make few LLM calls and to show results early:

- **Batching.** Several articles share one Gemini request (`-summary-batch`, default 6; each article is cut to 3,000 characters inside a batch). A day of 100 articles is about 17 requests instead of 100. A bigger batch means fewer calls but a longer wait per call; 6 is a balance. If the model leaves an article out of its reply, that article gets its extractive gist: nothing is retried.
- **Thin articles skip the LLM.** Under 400 characters of text there is nothing to condense, so the gist is the page description.
- **Pipelined.** Summaries and source-image lookups run while crawling is still going, whenever a full batch is waiting, so the first cards appear minutes into the run. `-summary-workers` (default 3) requests run at once, spaced to stay under Gemini's free per-minute limit.
- **Nothing twice.** A stored gist is never regenerated, only articles dated within `-summary-window-days` (default 7, match it to the longest period you browse) are considered, and a quota error pauses the run and leaves the rest for the next one.
- **Fast page loads.** `/digest` is paginated and cacheable for 30 s; images load lazily (the first row eagerly), and the digest page keeps its filters and page in the URL.

### Images

Every card image is served from this server (`GET /img/{article id}`), not hotlinked. The server looks the image URL up by article id (never from the request, so it is not an open proxy), fetches it once, keeps it in an in-memory cache (`-image-cache-mb`, default 128) and lets browsers cache it for a day. That avoids hotlink and referrer blocks, mixed-content blocking and slow third-party hosts. Only JPEG, PNG, GIF, WebP and AVIF up to 5 MiB are served (a mislabelled HTML error page or an SVG never is), and loopback and private addresses are refused. A failed fetch is remembered for 10 minutes. Images are never sent to Gemini, only article text is, so images add no LLM cost. On a 35-site run about 98% of the images the digest offered loaded (the rest were upstream 404s), at about 350 KB each.

**Which picture a card shows**, in order:
1. the page's own `og:image` or `twitter:image`, unless more than four articles share it (a site logo);
2. otherwise the first real picture inside the article itself, which costs no extra request. Logos, mascots, icons, avatars, author photos, ad banners, SVG, thumbnail-sized files and tiny images are skipped, and so is anything inside the page header, menu or sidebar;
3. otherwise a tile with the site's name on a colour derived from its host, so a site that has no usable image still looks intentional rather than broken.

For sites that summarise other stories (Drishti IAS), `image_from_source` fetches the cited story's `og:image` instead, one small request per new article. It is off for engineering blogs: the first outside link in a blog post is rarely a cited story (Go's is a meetup page in the footer). A cited site that blocks bots (PIB answers every request with HTTP 403) is asked twice in a run and then skipped, and its articles are marked looked up so it is not asked again every day; the crawler does not try to get around a block.

### The daily sites

[`sources.json`](../sources.json) is the set crawled every day: the required site (Indian Express), 39 developer and engineering blogs and 6 international-affairs sites, each checked live with `checksource` (below). Each source has a `category` (`tech`, `ai`, `upsc`, `india`, `world`; empty shows as `other`) that the digest page offers as a filter and `/digest?category=` accepts.

- **Company engineering blogs:** GitHub, Cloudflare, Stripe, Uber, Engineering at Meta, Google Developers, Google Cloud (developers), AWS (Architecture, DevOps), Shopify, Grab, Zalando, Spotify, Datadog, Honeycomb, Netlify, Supabase, Red Hat Developer
- **Languages, frameworks and platforms:** React, Rust, Inside Java, Go, Kubernetes, CNCF, CSS-Tricks, Smashing Magazine
- **Engineering reading:** InfoQ, ByteByteGo, Martin Fowler
- **AI engineering:** Hugging Face, Anthropic Engineering, Latent Space, NVIDIA Technical Blog
- **International affairs:** UN News, Al Jazeera, The Guardian (world), The Diplomat, Foreign Policy, Rest of World

There is no limit on the number of sites. Each one adds crawl time (about a second per page, sites in parallel) and Gemini calls (about one per six new articles), so if you want a ceiling set `-max-sites N` / `MAX_SITES=N`: the scheduler then refuses to start with more than N distinct sites and lists them (0, the default, means no limit). Several sections of one site count as one site. A test pins that Drishti, InsightsIAS and Indian Express always stay in `sources.json`.

**Not crawlable, so not included:** Netflix's tech blog (both `netflixtechblog.com` and its Medium address answer every request with a 403 bot challenge, which this crawler does not bypass), and for the same reason Airbnb, Lyft, Etsy, Instacart, Razorpay, Flipkart, DoorDash, Real Python and OpenAI's blog. Microsoft DevBlogs, LinkedIn Engineering, Hacker Noon and Mozilla Hacks disallow this crawler in robots.txt. Pinterest and Swiggy's engineering sites could not be reached.

[`catalog/`](../catalog) holds more entries that were checked the same way, as a menu to move from into `sources.json` (they are not crawled otherwise):

- [`catalog/dev.json`](../catalog/dev.json): Stack Overflow Blog, Figma, Docker, Elastic, Redis, Confluent, Twilio, Fly.io, AWS Open Source, AWS News, Google AI, DeepMind and more
- [`catalog/ai.json`](../catalog/ai.json): AI labs and AI news: Anthropic News and Research, Mistral, Cohere, Together AI, LangChain, Databricks, AWS Machine Learning, Stability AI, Google Research, Meta AI, Microsoft Research, Stanford HAI, MIT News (AI), BAIR and independent writers
- [`catalog/news.json`](../catalog/news.json): general tech news and Indian tech press (TechCrunch, The Verge, The Next Web, SiliconANGLE, Inc42, MediaNama, Hindustan Times, India Today and others), left out because they are news rather than developer blogs

**Check an entry's `note` before using it.** Some publishers list AI crawlers (ClaudeBot, GPTBot, anthropic-ai, Google-Extended and others) as disallowed in robots.txt, and the notes record that, or any Content-Signal line, per site. CrawlerX's own user agent is not named, so it is technically allowed to read them, but the digest summarises with an LLM, which those publishers' signals suggest they do not want for AI crawlers. **Indian Express, which is required, is one of them**; none of the 32 developer blogs in `sources.json` is. Read each site's terms before publishing digests beyond your own use.

Prefixes can use `{date:2006}` for the current year, so nothing needs editing in January, and a prefix starting with `!` excludes a path: `["/in/en/blog/", "!/in/en/blog/engineering/"]` follows Uber's articles but not its category pages. Sites with no dates (Uber, Go, Martin Fowler) list their existing posts as new on the first run, then only new ones.

If Gemini answers 429, the client waits as long as the server asks and retries, and the final pass waits a minute and goes again (up to 3 times) before leaving the rest for the next run.

### Adding your own sites: `checksource`

Before adding a URL to a sources file, ask the crawler what it would make of it:

```
go run ./cmd/checksource https://example.com/blog/
go run ./cmd/checksource -prefix /news/ -article-prefix /news/ https://example.com/news/
go run ./cmd/checksource -prefix /blog/,!/blog/tag/ https://example.com/blog/    # "!" excludes a path
go run ./cmd/checksource -ai-policy https://example.com/ https://other.example/   # just the robots.txt AI policy
```

It makes a handful of requests, one a second, and runs the crawler's own robots check, fetcher and parser. It reports whether robots.txt is open and what it says about AI crawlers; whether the site serves real HTML (not a bot challenge or a JavaScript shell); which path prefixes hold the articles; and, for a few sampled articles, whether they count as articles (date or `-article-prefix`), how much text they have and what image they carry. It ends with a verdict (WORKS, PARTIAL or NO, with reasons) and a `sources.json` entry to paste.

Sites may also be added with a date only in JSON-LD (`Article`, `BlogPosting`, `NewsArticle`) or in odd timestamp formats: both are read, so most blogs need no `article_prefixes`.
