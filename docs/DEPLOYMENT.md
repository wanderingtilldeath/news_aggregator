# news_aggregator: deployment guide and history

This documents how the news digest app (`crawlerx`) was deployed for a free live demo: what runs where, every setting, every code change made for the deployment, how caching works, what was measured, what went wrong along the way, and how to operate it.

- Repository: `wanderingtilldeath/news_aggregator` (GitHub). Deployed branch: `main`. `slim-deploy` points at the same commit.
- Public URL to share: `https://news-digest-edge.crawlerx.workers.dev/digest.html`
- Origin (Render): `https://news-aggregator-4jjy.onrender.com`
- Cost: $0 (all free tiers). Pricing and limits quoted below come from vendor docs where noted and otherwise from secondary sources, so re-check them on the vendors' pages before relying on them.

No secret values appear in this file. Where a secret is needed, the name and where to put it are given.

---

## 1. What the app is

`crawlerx` is one static Go binary (Go 1.26, module `github.com/wanderingtilldeath/news-aggregator`).

- **Crawls** a list of sources (`sources.json`, 37 entries; the demo uses `sources.demo.json`, 6 entries) once a day, depth 1, honouring robots.txt, 1 s per-host delay, 2 in flight per host, 2 MiB body cap.
- **Stores** pages in Postgres (queue, visited set, pages, a full-text index). Migrations are embedded and run at every start under a Postgres advisory lock (goose).
- **Summarises** new articles into a 3-bullet gist: Gemini if a key is set, otherwise an extractive fallback (page description plus first sentences). Gists are stored, so restarts never re-spend quota.
- **Serves** a JSON API and two static pages: the digest at `/digest.html` (image cards, source filter, search, paging) and a crawl dashboard at `/`.
- Its scheduler is in-process (06:00 `Asia/Kolkata` by default). There is no one-shot mode, no authentication, and `POST /crawls` makes the server fetch an arbitrary URL.

API surface used by the digest page: `GET /digest`, `GET /digest/sources`, `GET /img/{id}` (an image proxy that looks the image up by article id and caches it in memory), and the static page itself. Others: `GET /search`, `POST|GET /crawls`, `GET /crawls/{id}`, `GET /healthz` (pings the database and echoes errors).

---

## 2. Final architecture

```
                         ┌─────────────────────────────────────────────┐
 GitHub Actions cron     │ 06:00 IST daily + manual "Run workflow"     │
 (.github/workflows/     │  1. build crawlerx                          │
  crawl.yml)             │  2. crawl sources.demo.json  ─────────────┐ │
                         │  3. prune old rows            ──────────┐ │ │
                         │  4. build snapshot (7 days)  ◄────────┐ │ │ │
                         │  5. wrangler deploy ───────────┐      │ │ │ │
                         └────────────────────────────────┼──────┼─┼─┼─┘
                                                          │      │ ▼ ▼
   Visitors ──► Cloudflare Worker "news-digest-edge" ◄────┘   Neon Postgres (free)
                 static assets: /digest.html, /img/*,            ▲
                 /snapshot.json                                  │
                 Worker code: /digest, /digest/sources           │
                 (from snapshot, days 1/2/7, no q)               │
                      │ everything else (30-day view, search,    │
                      ▼  dashboard, /healthz)                    │
                 Render web service (serve-only crawlerx) ───────┘
                 WORKERS=0, READ_ONLY=true, sleeps when idle
```

Roles:

| Component | Role | Always on? |
|---|---|---|
| **Neon** | The only datastore | Suspends after 5 min idle, resumes on first query |
| **GitHub Actions** | The only thing that crawls; also builds and publishes the snapshot | Runs daily, ~5–7 min |
| **Render** | Serve-only API for the paths the snapshot can't answer (30-day view, text search, dashboard) | Sleeps after 15 min idle, ~1 min to wake |
| **Cloudflare Worker + static assets** | What visitors hit. Serves the last 7 days with no dependence on Render or Neon | Global, no cold start |

Why the web service and the crawler are separate processes: any `crawlerx` with `WORKERS>0` polls the database about 10 times a second forever (`PollInterval` is 100 ms, not configurable), which would keep Neon awake and burn its free compute hours. So the Render service runs with `WORKERS=0` and the crawl runs as a short-lived job.

---

## 3. How we got here (decision history)

1. **Scope.** Four projects were considered (gatekeeper, verity, docs-rag-ingest, news_aggregator). The user chose to deploy one at a time, **news_aggregator first**; gatekeeper was dropped; verity and docs-rag-ingest are not yet deployed. (`docs-rag-digest` does not exist; the closest is `docs-rag-ingest`.)
2. **Hosting survey.** Free options compared: Render, Railway, Fly.io, Koyeb, Vercel, Hugging Face Spaces, Cloudflare, Oracle Always Free (cut to 2 OCPU / 12 GB on 2026-06-15), GCP e2-micro, AWS/Azure credits, Hetzner (paid), plus Neon/Supabase for Postgres. Railway, Fly.io and Koyeb are not free for new accounts; Vercel is serverless/non-commercial only and cannot run a long-lived Go server; an Oracle VM would be always-on but needs a card.
3. **Chosen stack:** Render (Docker) for the web service, Neon for Postgres, GitHub Actions for the crawl, Cloudflare Worker for the public face.
4. **Vercel was tried and removed.** A leftover Vercel project (from the old React frontend on the `master` branch) failed on every push to `main`/`slim-deploy`. No Vercel file exists in any branch; the failure came from the project's own settings. The user deleted the Vercel project. Nothing in the repo references Vercel.
5. **Caching redesign.** The first Worker version cached with the Cache API, which is **per Cloudflare data center**. A warm-up from a US runner could not warm other regions, so the first visit from a new location still hit cold Render + cold Neon + ~24 uncached images. It was replaced by a snapshot of the last 7 days deployed as static assets (section 7).

---

## 4. Code and configuration changes made for the deployment

### 4.1 Application code

| Change | Files | Why |
|---|---|---|
| `READ_ONLY` setting (env `READ_ONLY`, flag `-read-only`) removes `POST /crawls` | `internal/api/api.go` (`Deps.ReadOnly`), `internal/config/config.go`, `cmd/crawlerx/main.go`, tests in `internal/api/api_test.go`, `internal/config/config_test.go` | The public service must not be told to fetch URLs: the crawler has no private-address refusal (only the image proxy does), so an open `POST /crawls` is an SSRF with read-back, and queued crawls would be executed by the next job run. With it set, `POST /crawls` returns 405. |
| `Cache-Control` on `/digest` and `/digest/sources`: `max-age=30` → `300` | `internal/api/api.go` | The data changes once a day. |
| **Soft-404 filter** | `internal/parse/notfound.go` (`Page.LooksLikeNotFound`), `internal/parse/notfound_test.go`, hook in `internal/crawler/crawler.go` `savePage`, test in `internal/crawler/crawler_test.go` | Drishti serves error pages with HTTP 200 and title "Not Found" plus an `error.png` ("OOPS") picture. Under `article_prefixes` they were stored as articles. |
| Removed an unused parser field and stopped tracking `.DS_Store` / `.idea/.gitignore` | `internal/parse/parse.go`, git index | `staticcheck` U1000; the files were already in `.gitignore`. |

**Soft-404 rules.** A page counts as not-found when its `<title>` or first `<h1>` (lower-cased, trimmed) equals, or starts with one of the following and then a separator (space, `|`, `-`, `–`, `—`, `:`, `!`, `.`): `not found`, `page not found`, `404`, `404 not found`, `oops`, `we couldn't find`, `we couldn’t find`, `page unavailable`; or when the article picture's file name is `error.png` or `404.png`. Content length alone is deliberately **not** a signal: "President of India" (147 chars) and "Central Armed Police Forces (CAPF)" (50 chars) are real infographic pages, and several short github.blog articles are real too. The page is still stored with `is_article=false` so the frontier marks it done and it is not refetched. One existing row was fixed on Neon with an idempotent `UPDATE pages SET is_article = false WHERE title ~* '^(404|not found|page not found|oops|we couldn.t find|page unavailable)'` (1 of 458 rows matched: `.../daily-news-analysis/new-it-rules-202`).

### 4.2 New deployment files

| File | Purpose |
|---|---|
| `sources.demo.json` | 6 sources instead of 37 (see 4.3) |
| `Dockerfile` | Two-stage build (`golang:1.26` → `gcr.io/distroless/static-debian12:nonroot`), `CGO_ENABLED=0 -trimpath -buildvcs=false -tags timetzdata -ldflags="-s -w"`. Copies only `cmd/crawlerx`, `internal`, `go.mod`, `go.sum`, and `sources.demo.json`. Image ≈ 24.8 MB. `timetzdata` embeds the time-zone database so `TIMEZONE` works on any base image. The `# syntax=` line was removed so the build needs no extra registry pull. |
| `.dockerignore` | `.git .github .idea .DS_Store .env .env.* catalog docs edge bin`, which keeps the local `.env` (with a real Gemini key) out of the image. Verified by listing the image: only `crawlerx` and `sources.demo.json`. |
| `.env.demo.example` | Documents web-service and crawl-job variables (no real values) |
| `.github/workflows/crawl.yml` | Daily crawl + prune + snapshot publish (section 6) |
| `scripts/build-snapshot.sh` | Builds the edge snapshot (section 7) |
| `edge/worker.js`, `edge/wrangler.toml` | The Cloudflare Worker (section 7) |
| `.gitignore` additions | `edge/assets/`, `edge/.wrangler/`, `edge/node_modules/` |
| `README.md` | New "Deploy (free demo ...)" section |

### 4.3 The 6 demo sources

Chosen from the 37 in `sources.json` (which is left untouched: a test, `TestDefaultSourcesIncludeTheRequiredSites`, pins Drishti IAS, InsightsIAS and Indian Express to it). All six passed `go run ./cmd/checksource` (robots allowed, articles detected, text and images found).

| Source | Notes |
|---|---|
| Drishti IAS: news analysis | `days_back: 2` (Drishti publishes nothing on Sundays), `image_from_source` |
| InsightsIAS (one item per topic) | `split_articles`; many cards per day |
| GitHub Blog | frequent, has images |
| InfoQ News | robots.txt asks for a 3 s crawl delay, which the crawler honours |
| Hugging Face Blog | AI, frequent |
| Anthropic Engineering | publishes rarely, so it shows 0–1 items in short windows |

Effect: roughly 1,400 pages per run (37 sources) becomes roughly 200; the crawl takes 1–5 minutes. Indian Express was left out: its robots.txt disallows AI crawlers and its seed page is the heaviest (993 KB). `MAX_SITES=6` in the job makes startup fail if someone later adds more sources than intended.

---

## 5. Services, one by one

### 5.1 Neon (Postgres)

- Free tier (figures from secondary sources, not verified on Neon's own pages): 100 CU-hours per project per month, 1 GB storage per project, compute suspends after 5 minutes idle (cannot be disabled on free).
- Database name `neondb`, role `neondb_owner`, Postgres 18.6, no extensions needed (the app needs Postgres 12+ for stored generated columns).
- **Use the DIRECT connection string** (host without `-pooler`): `postgresql://neondb_owner:<password>@ep-<id>.<region>.aws.neon.tech/neondb?sslmode=require`. Do not add `&channel_binding=require`. The pooled endpoint is not recommended: pgx's default prepared-statement cache and goose's session-level advisory lock (taken on every start) do not work behind PgBouncer transaction pooling. (This was reasoned from the code; the pooled endpoint was not tried.)
- The pooled URL the user first pasted was converted to the direct one for all tests.
- Size after the first runs: about 12 MB for ~470 pages; growth estimated at 0.3–1 MB/day. The job prunes `crawls` older than 3 days and dated `pages` older than 60 days (undated pages are kept: re-crawling would make them look new again and trigger re-summarising).
- **Measured** from India (the database is in `us-east-1`): a fresh `psql` connect costs about 1.6–2.0 s (TLS and auth round trips); app start including migrations on a suspended database took 3.2 s; first `/digest` after resume 0.86 s; warm `/digest` 0.2–1.0 s. A US host sees far less.
- **The database password was pasted into a chat during setup.** If that transcript could be shared, rotate it in the Neon console and update the two places that hold it (GitHub secret `DATABASE_URL`, Render env var `DATABASE_URL`).

### 5.2 Render (web service)

| Setting | Value |
|---|---|
| Type | Web service, Docker (builds `Dockerfile`) |
| Branch | `main` (it was briefly pointed at `slim-deploy` while the work was unmerged; keep it on `main`) |
| Health check path | `/digest.html` (not `/healthz`, which pings the database and wakes Neon) |
| Port | 10000 (Render's default) |
| Region | choose the US East region, next to Neon |

Environment variables:

| Variable | Value | Why |
|---|---|---|
| `DATABASE_URL` | the direct Neon string | |
| `LISTEN_ADDR` | `:10000` | the app reads only `LISTEN_ADDR`, not `$PORT` |
| `WORKERS` | `0` | serve-only; no polling of the database |
| `READ_ONLY` | `true` | removes `POST /crawls` |
| `IMAGE_CACHE_MB` | `24` | in-memory image cache cap |
| `GOMEMLIMIT` | `180MiB` | stays well inside 512 MB |
| `LOG_FORMAT` | `json` | |
| `SOURCES` | **leave unset** | if set, the web box would start the daily crawl itself |

Free-tier behaviour (vendor figures as reported by secondary sources): 512 MB RAM and 0.1 CPU, 750 instance-hours per month, sleeps after 15 minutes without traffic and takes about a minute to wake.

Measured: serve-only process idles at about 12–15 MB RSS; as a container with a 256 MB limit it used about 8 MiB.

Render only serves what the Worker proxies: the 30-day view, text search, the dashboard and `/healthz`. After pushing to `main`, trigger a deploy of the latest commit if auto-deploy is off. A correct deploy shows `COPY cmd/crawlerx ./cmd/crawlerx` and `-tags timetzdata` in the build log; `COPY . .` and `-o /out/app` mean the old Dockerfile (an old branch) was built.

Checks on a correct deploy: `/digest.html` 200, `/digest/sources` 200, `Cache-Control: public, max-age=300` on `/digest`, and `POST /crawls` → **405**.

### 5.3 GitHub Actions

Workflow `.github/workflows/crawl.yml`, job `crawl` on `ubuntu-latest`, `timeout-minutes: 45`, `concurrency: crawl` (no cancel). Triggers: `schedule: cron "30 0 * * *"` (00:30 UTC = 06:00 IST) and `workflow_dispatch`.

Job environment: `DATABASE_URL`, `GEMINI_API_KEY`, `CLOUDFLARE_API_TOKEN`, `CLOUDFLARE_ACCOUNT_ID` (all from secrets), and `WORKERS=2`, `SOURCES=sources.demo.json`, `RUN_ON_START=true`, `TIMEZONE=UTC`, `MAX_SITES=6`, `IMAGE_CACHE_MB=1`, `GOMEMLIMIT=350MiB`, `SUMMARY_PROVIDER` (variable, default `extractive`), `SUMMARY_WINDOW_DAYS` (variable, default `30`), `SUMMARY_BATCH=12`, `USER_AGENT` (variable, default `CrawlerX/1.0 (+https://github.com/<repo>)`).

Steps:

1. `actions/checkout@v4`, `actions/setup-go@v5` (from `go.mod`), `actions/setup-node@v4` (Node 22).
2. **Build:** `CGO_ENABLED=0 go build -trimpath -tags timetzdata -ldflags="-s -w" -o crawlerx ./cmd/crawlerx`.
3. **Crawl and summarise:** runs `./crawlerx -listen=` in the background (the flag `-listen=` disables the API; an empty `LISTEN_ADDR` env var does not, because empty means "default"), polls `run.log` every 5 s for up to 40 minutes for the line `digest run complete`, then sends SIGTERM, prints the last 40 log lines, and fails if the line never appeared.
4. **Prune old rows:** `DELETE FROM crawls WHERE created_at < now() - interval '3 days'` and `DELETE FROM pages WHERE published_at < now() - interval '60 days'` via `psql`.
5. **Publish snapshot to Cloudflare** (runs only if both Cloudflare secrets are set): `scripts/build-snapshot.sh ./crawlerx edge/assets`, then `cd edge && npx --yes wrangler@4 deploy`.
6. **Snapshot skipped** (runs only if either secret is missing): prints a notice. The crawl itself still succeeds, which is why a run can be green while nothing was published (this happened once; see the troubleshooting table).

Repository **secrets** (Settings → Secrets and variables → Actions → Secrets → Repository secrets):

| Name | Value |
|---|---|
| `DATABASE_URL` | the direct Neon string |
| `CLOUDFLARE_API_TOKEN` | a token from the "Edit Cloudflare Workers" template |
| `CLOUDFLARE_ACCOUNT_ID` | the account ID shown in the Cloudflare dashboard (Workers & Pages overview), or by `wrangler whoami` |
| `GEMINI_API_KEY` | optional; without it gists are extractive |

Repository **variables** (all optional): `SUMMARY_PROVIDER` (`extractive` or `auto`), `SUMMARY_WINDOW_DAYS` (30; use 7 with Gemini), `USER_AGENT`.

Important GitHub facts:

- Scheduled runs and the "Run workflow" button only exist for workflows **on the default branch**. The default branch was `master` (which has no workflow file), so the workflow was invisible until the default branch was switched to `main` (Settings → **General** → Default branch, not the Branches page).
- A crawl takes about 5 minutes on the runner (a 5m24s run before the snapshot step existed; the snapshot step adds roughly a minute). From a laptop in India over Neon it took 4m07s for the crawl alone.
- GitHub cron can start late; the previous snapshot stays live until the next successful run.
- Public repository: unlimited Actions minutes; private repository: 2,000 free minutes per month (the job uses about 150–250).

### 5.4 Cloudflare Worker `news-digest-edge`

Deployed to `https://news-digest-edge.crawlerx.workers.dev` (account subdomain `crawlerx`). It was first deployed by hand with `wrangler login` + `wrangler deploy` (Node from `~/.nvm/versions/node/v24.22.0/bin`; it is not on the default `PATH`). From the first successful snapshot run onward the workflow deploys it, so a manual deploy is no longer needed. Details in the next two sections.

---

## 6. The daily flow, step by step

1. 00:30 UTC: GitHub starts the job.
2. The job builds `crawlerx`, starts it with `SOURCES=sources.demo.json` and `RUN_ON_START=true`. It crawls the six sources (2 workers, 1 s host delay), extracts titles, dates, images and text, stores pages in Neon, then summarises new articles (batches of 12 with Gemini, or extractively), and looks up images from cited source pages. It logs `digest run complete` and is stopped.
3. Old rows are pruned.
4. `scripts/build-snapshot.sh` starts `crawlerx` again, serve-only on `127.0.0.1:18080` (`env -u SOURCES -u RUN_ON_START WORKERS=0 READ_ONLY=true`), so the snapshot is exactly what the live API returns.
5. `wrangler deploy` uploads `edge/assets/` and the Worker code. A deploy replaces the whole asset set at once. That is the cache invalidation.
6. Visitors now get the new data from the edge.

---

## 7. Caching and the snapshot

### 7.1 Why there is a snapshot

On a cold visit from a new location the old design had to wait for: an empty Cloudflare data-center cache, Render waking (about 1 minute), Neon resuming (2–3 s from India), then about 24 image requests each doing a database lookup and a download from the original site on a 0.1-CPU instance. The Cloudflare Cache API is per data center, so warming it from one place does not help others.

### 7.2 What the snapshot contains (`edge/assets/`, generated, not committed)

| File | Content |
|---|---|
| `snapshot.json` | `{"generated_at", "days": 7, "items": [...]}`: every item from `GET /digest?days=7`, paged at 200 per page, with `image` rewritten |
| `img/<id>.<ext>` | each item's picture, fetched from `/img/<id>` (extension from the content type: jpg, png, webp, gif, avif) |
| `digest.html` | copied from `internal/api/static/digest.html` |
| `_headers` | `/img/*` → `Cache-Control: public, max-age=31536000, immutable` (a file name carries the article id, so its content never changes) |

Image rules in `scripts/build-snapshot.sh`: six downloads in parallel, 30 s timeout each. If an image fails, or is larger than **700,000 bytes** (override with `SNAPSHOT_MAX_IMAGE_BYTES`), the item's `image` becomes `""` and the page shows its coloured site tile instead (the page already does this on a missing image). The first snapshot was 51.7 MB because 28 images over 500 KB made up 30.8 MB; with the cap it was 25 MB and 123 images (20 skipped). The latest live snapshot has 170 items, 124 with images.

### 7.3 What the Worker does (`edge/worker.js`)

- Static assets (`/digest.html`, `/img/*`, `/snapshot.json`) are served directly by Cloudflare.
- `GET /digest` and `GET /digest/sources` run the Worker first (`run_worker_first = ["/digest", "/digest/sources"]`) and are answered from `snapshot.json` when `days` is `1`, `2` or `7` (default 1), there is no `q`, and `articles_only` is not `false`. Otherwise the request goes to Render: the 30-day view, text search, listing pages.
- Anything that doesn't match a static asset (for example `/`, `/search`, `/healthz`, `/crawls`) is proxied to `ORIGIN`.
- If `snapshot.json` is missing or malformed, `/digest*` falls back to Render.
- The logic mirrors the Go handlers:
  - `since = now − days × 24 h`; keep items whose `time` is at or after it (`time` is `COALESCE(published_at, first_seen_at)`), so items age out of the 1/2-day views on their own between deployments.
  - `source` filter: trimmed, lower-cased equality on the item's `source` (the host).
  - Order is the snapshot order (`time DESC, url`).
  - `page` ≥ 1 and `page_size`/`limit` between 1 and 200, default 24, with the same error messages (`page must be 1 or more`, `page_size must be between 1 and 200`) and HTTP 400.
  - Response keys: `since`, `items`, `page`, `page_size`, `total`, `total_pages`. Sources: `{"sources":[{"source","count"}]}` sorted by count descending, then host.
  - API responses carry `Cache-Control: public, max-age=300` and `x-edge: snapshot`.

### 7.4 `edge/wrangler.toml`

```toml
name = "news-digest-edge"
main = "worker.js"
compatibility_date = "2026-10-01"

[vars]
ORIGIN = "https://news-aggregator-4jjy.onrender.com"

[assets]
directory = "./assets"
binding = "ASSETS"
html_handling = "none"
run_worker_first = ["/digest", "/digest/sources"]
```

`html_handling = "none"` is essential: by default Cloudflare serves `digest.html` at `/digest` (and redirects `/digest.html` there), which would collide with the `/digest` API path. With `none`, files are served at their exact paths. To point at a different Render URL, edit `ORIGIN` and redeploy.

### 7.5 Layers, summarised

| Layer | What it does | When it changes |
|---|---|---|
| Static assets at the edge | serve page, images, snapshot | on each deploy (daily) |
| Worker | filters/pages the snapshot for `days` 1/2/7 | same |
| Browser | `max-age=300` on API responses; images immutable for a year | per visitor |
| Render | origin for the rest; in-memory image cache (24 MB) | on process restart |
| Neon | source of truth | on each crawl |

### 7.6 Free-tier limits (from Cloudflare's docs)

Static assets on the free plan: up to 20,000 files per version, 25 MiB per file; static asset requests are not billed. Requests that match `run_worker_first` patterns invoke the Worker and count against the free request allowance (100,000 per day on the free plan; if exceeded those paths return 429 instead of falling back to assets). Worker CPU time limit on free is small (about 10–30 ms per request in the docs/reports); the filtering over ~170 items is well inside that.

---

## 8. Measurements

| What | Result |
|---|---|
| Docker image | 24.8 MB; contains only `crawlerx` and `sources.demo.json` |
| Container RSS (256 MB limit, serve-only) | ~8 MiB |
| Native serve-only RSS | 12–15 MB |
| Demo crawl, local Postgres | 75 s, 441 pages, 319 gists (extractive), 8 crawls |
| Demo crawl against Neon from a laptop in India | 4m07s (crawl only) |
| Neon cold: app start with migrations | 3.2 s; first `/digest` 0.86 s |
| `psql` connect to Neon from India | 1.6–2.0 s each |
| Snapshot size | 51.7 MB uncapped → 25 MB with the 700 KB image cap (123 images) |
| Latest live snapshot | 170 items, 124 with images, 89 in the 1-day view |
| Worker parity test (local `wrangler dev` vs the Go API) | 41 query variants, 0 differences (days 1/2/7, every source filter, pages 1–3, bad input, `/digest/sources`) |
| Simulated first visit through the live Worker (curl) | page 0.37 s, digest 0.34 s, sources 0.32 s, 12 images in parallel 1.5 s → about 2.4 s wall |
| Fallback to Render (awake) | 0.55 s |
| Go checks | `gofmt`, `go vet`, `staticcheck`, `go test -short -race ./...` all clean |

---

## 9. Setting it up from scratch

1. **Neon:** create a project; copy the direct connection string (no `-pooler`, `sslmode=require`).
2. **GitHub:** repo → Settings → General → set the default branch to `main`. Settings → Secrets and variables → Actions → add secrets `DATABASE_URL`, `CLOUDFLARE_API_TOKEN`, `CLOUDFLARE_ACCOUNT_ID` (and optionally `GEMINI_API_KEY`).
3. **Cloudflare:** dash.cloudflare.com → My Profile → API Tokens → Create Token → template "Edit Cloudflare Workers" → account resources: your account → create and copy. The account ID is on the Workers & Pages overview page. A `workers.dev` subdomain must exist (the first deploy prompts for one).
4. **Render:** New → Web Service → connect the repo → branch `main` → language Docker. Add the environment variables in 5.2 and the health check path `/digest.html`. Put the service in the US East region.
5. Put the Render URL in `edge/wrangler.toml` (`ORIGIN`) and push.
6. GitHub → Actions → crawl → **Run workflow**. Expect every step green, with "Publish snapshot to Cloudflare" as success (not skipped). The Worker URL is printed by `wrangler deploy`.
7. Open `https://<worker>.workers.dev/digest.html`.

---

## 10. Operations

- **Run now:** Actions → crawl → Run workflow.
- **Check what was published:** `curl -s https://news-digest-edge.crawlerx.workers.dev/snapshot.json | head -c 200` shows `generated_at`; `curl -sI .../digest?days=1` should show `x-edge: snapshot`.
- **Check a run without opening GitHub:** `curl -s https://api.github.com/repos/wanderingtilldeath/news_aggregator/actions/runs?per_page=3` (public repo), then `.../runs/<id>/jobs` for step results.
- **Change sources:** edit `sources.demo.json` (schema in `internal/scheduler/sources.go`), validate each seed with `go run ./cmd/checksource -n 3 -prefix <path> [-article-prefix <path>] <seed-url>` (run it from a network like the runner's, since datacenter IPs are sometimes blocked), keep `MAX_SITES` in the workflow in step with the count.
- **Use Gemini summaries:** add the `GEMINI_API_KEY` secret, set the variable `SUMMARY_PROVIDER=auto` and `SUMMARY_WINDOW_DAYS=7`. Run one canary first: a 400/401/403/404 from Gemini is stored permanently as the extractive gist for that article (no retry). The default model string `gemini-3.5-flash-lite` is unconfirmed; override with `SUMMARY_MODEL`.
- **Change the origin or redeploy the Worker by hand:** `cd edge && export PATH="$HOME/.nvm/versions/node/v24.22.0/bin:$PATH" && npx wrangler@4 deploy` (needs `wrangler login` or the two Cloudflare env vars, and a built `edge/assets/` from `scripts/build-snapshot.sh`).
- **Rebuild the snapshot locally:** `DATABASE_URL=... scripts/build-snapshot.sh ./crawlerx edge/assets` (about 25 s over Neon from a laptop).
- **Test the Worker locally:** `cd edge && npx wrangler@4 dev --port 8788 --var ORIGIN:http://127.0.0.1:18081` with the Go app running on that port and `edge/assets/` built. Docker note: inside a plain `node:22-slim` container wrangler's runtime needs `ca-certificates` installed.
- **Rotate the database password:** change it in Neon, then update the GitHub secret and the Render variable.
- **Pre-demo habit:** open the Worker URL a few minutes before presenting. It is warm anyway; this also wakes Render for the 30-day view and search.

---

## 11. Troubleshooting log (everything that went wrong, and the fix)

| Symptom | Cause | Fix |
|---|---|---|
| Actions run: `crawlerx: database URL is required`, env shows `DATABASE_URL:` blank | The `DATABASE_URL` repository secret did not exist (or was a variable, or in another secret scope) | Add it under Secrets → Actions → Repository secrets, exact name |
| `crawl` workflow not in the Actions tab | Default branch was `master`, which has no workflow file; only the default branch's workflows are listed and scheduled | Settings → General → Default branch → `main` |
| "Invalid workflow file … line 79" | `run: echo "Skipping the snapshot: set ..."`: the `: ` inside an unquoted scalar is read as a YAML key | Use a block scalar (`run: \|`); validate workflow YAML with a parser before pushing |
| Run green but nothing published; Worker still old (`x-edge: HIT/MISS`, `/snapshot.json` 404) | `CLOUDFLARE_API_TOKEN` / `CLOUDFLARE_ACCOUNT_ID` secrets missing, so "Publish snapshot" was skipped and "Snapshot skipped" ran | Add both secrets, run again |
| Render live but `/digest.html` 404, `/digest/sources` 404, `POST /crawls` returns 201, old startup log line `no -redis-url` | Render was building `main` before `slim-deploy` was merged (and later, an old Dockerfile with `COPY . .` / `-o /out/app`) | Set the branch to the one with the new code (now `main`) and redeploy |
| Vercel deployment failed on every push | A leftover Vercel project built the old React frontend that no longer exists on these branches | Project deleted; nothing in the repo references Vercel |
| First page load very slow | Everything missed: empty per-data-center cache, Render asleep, Neon asleep, ~24 uncached images | Replaced by the static snapshot (section 7) |
| `/digest.html` would serve the JSON API | Cloudflare's default HTML handling maps `digest.html` to `/digest` | `html_handling = "none"` |
| Soft-404 card "Not Found" with the OOPS picture | Error page served with HTTP 200 matched `article_prefixes` | `LooksLikeNotFound` filter + one-time `UPDATE` |
| Snapshot 51 MB | A few large PNGs | 700 KB cap per image |
| `docker build`: `docker-credential-desktop` not found | Docker Desktop's helper not on `PATH` | `export PATH="$PATH:/Applications/Docker.app/Contents/Resources/bin"`; also dropped the `# syntax=` line |
| `wrangler dev` in a node-slim container: "No CA certificates were loaded" | The image lacks `ca-certificates` | `apt-get install -y ca-certificates` first (or run wrangler natively) |
| `npx vercel …` / `wrangler` say not authenticated | No login on this machine | `wrangler login` (browser); Vercel is no longer used |
| `git commit`: `index.lock: File exists` | A transient lock from another git process | Retry once the lock is gone (do not chain a push after a failed commit) |
| Git push to delete remote `master` rejected: `refusing to delete the current branch` | `master` is the default branch | Change the default branch to `main` first, then delete |

---

## 12. Git history and branches

- `main` and `slim-deploy` point at the same commit and have identical trees. GitHub's default branch is `main` (switched during setup). The old remote `master` branch was deleted after setup (its tip `bb70042` is reachable from `main`, so no history was lost). `main` and the old `master` line had no common history; `main` was brought up to date with an `--allow-unrelated-histories` merge whose tree was then made identical to `slim-deploy`'s, so nothing was force-pushed on `main`.
- Files that existed only on the old `main` and were removed by that merge (still in history): `docker-compose.yml`, `deploy/prometheus.yml`, `cmd/bench`, `cmd/fakesite`, `internal/metrics`, `docs/BENCHMARK.md`, `.DS_Store`, `.idea/.gitignore`. They belong to the earlier Redis/Prometheus setup that `slim-deploy` had already dropped.
- Commits of this work, newest first (all on `main`): `46c63f0` Fix invalid YAML in the crawl workflow; `a296e71` Serve the last 7 days from an edge snapshot rebuilt by the daily job; `c9ebb62` Hide soft-404 pages from the digest; `45ba6bd` Merge slim-deploy into main (it also contains the demo sources, read-only API, Dockerfile, workflow and the first edge Worker, plus the cleanup commit that stopped tracking `.DS_Store`).
- Commit authorship: Umesh Godhwani. At the user's request, the `Co-Authored-By: Claude` trailer was removed from the three commits that carried it (their messages were rewritten and `slim-deploy` was force-pushed with a lease, since it was a branch created that day), and no commit since has one.
- Local-only leftovers deleted: the local `master` branch and two backup branches.

---

## 13. Security notes

- The API has no authentication. The public path is therefore read-only: Render runs with `READ_ONLY=true` (no `POST /crawls`), and the crawl job does not listen at all (`-listen=`).
- The crawler and robots clients have no private-address refusal (only the image proxy does). That is why `POST /crawls` must stay unreachable; a dial-time private-IP guard would be a worthwhile later hardening.
- `/healthz` echoes database error text; don't point public monitors at it. The dashboard at `/` and `/crawls` (read-only list) are reachable through the Worker's fallback to Render.
- The image proxy cannot be used as an open proxy: it looks images up by article id, never by request URL, refuses private addresses at dial time, caps images at 5 MiB, whitelists content types (no SVG) and sends a sandbox CSP.
- Secrets live only in GitHub secrets and Render environment variables. `.env` (with a real Gemini key) is git-ignored and excluded from the Docker image; do not copy it into any host. The database password was shared in a chat during setup, so rotate it if that transcript is not private.
- Gemini's free tier may use inputs for training; the demo defaults to extractive summaries.
- Respect robots.txt and each site's terms; Indian Express (disallows AI crawlers) is excluded from the demo set, and Stripe's blog signals `ai-train=no` (not in the demo set).

---

## 14. Known limitations and open items

- **Old `master` branch:** deleted from the remote once the default branch was `main`; its history stays reachable from `main` (tip `bb70042`).
- **Render redeploy** from `main` is needed once to carry the soft-404 fix into the Render process (the daily job already builds from source, so crawls and the snapshot have it). Confirm the Render log shows the latest commit.
- **30-day view, text search and the dashboard** still depend on Render and Neon and can take about a minute when both are asleep. The 1/2/7-day views and images do not.
- **Large images** (over 700 KB) show the site tile on the snapshot path.
- **Snapshot freshness:** the data changes only when the workflow runs. If a run fails, the previous snapshot stays live.
- **Unverified figures:** vendor free-tier numbers other than those cited from Cloudflare's docs came from secondary sources; the Gemini default model name is unconfirmed; the pooled Neon endpoint was not tried.
- **Small cleanups not done:** `testutil.IsLocalURL` is unused (its comment says schema-dropping tests refuse non-local databases, but nothing enforces it); `docs/INFRA_FEATURES_INTERVIEW.md` describes the removed Docker/Redis/Prometheus setup and may be stale.
- **Not deployed yet:** verity and docs-rag-ingest (planned one at a time); gatekeeper was dropped.

---

## 15. File index

| Path | What it is |
|---|---|
| `cmd/crawlerx/main.go` | binary entry point (wires config, stores, crawler, scheduler, API) |
| `internal/api/` | HTTP API and the embedded static pages (`static/digest.html`, `static/index.html`) |
| `internal/config/config.go` | all environment variables and flags |
| `internal/crawler/` | crawl workers, `savePage` (article decision, soft-404 hook) |
| `internal/parse/` | HTML parsing, `notfound.go` (soft-404 detector) |
| `internal/scheduler/` | daily digest job, `sources.go` (source schema) |
| `internal/store/`, `internal/frontier/`, `internal/db/` | Postgres access and migrations |
| `sources.json` / `sources.demo.json` | full list / 6-source demo list |
| `Dockerfile`, `.dockerignore` | Render build |
| `.github/workflows/crawl.yml` | daily crawl, prune, snapshot publish |
| `scripts/build-snapshot.sh` | snapshot builder |
| `edge/worker.js`, `edge/wrangler.toml` | Cloudflare Worker and its configuration |
| `.env.demo.example` | variable reference for the demo |
| `docs/DEPLOYMENT.md` | this document |

### Environment variable reference (application)

Defaults from `internal/config/config.go`.

| Variable | Default | Used for |
|---|---|---|
| `DATABASE_URL` | none (required) | Postgres |
| `LISTEN_ADDR` | `:8080` | API address; use flag `-listen=` (not the env var) to disable the API |
| `READ_ONLY` | `false` | removes `POST /crawls` |
| `WORKERS` | `8` | crawl workers; `0` = serve-only |
| `HOST_DELAY` / `HOST_INFLIGHT` | `1s` / `2` | politeness |
| `TIMEOUT` / `MAX_ATTEMPTS` / `LEASE` / `SHUTDOWN_GRACE` | `15s` / `5` / `1m` / `15s` | fetching and recovery |
| `MAX_BYTES` | 2 MiB | body cap (keep it: 1 MiB would truncate some seed pages) |
| `SOURCES` | empty | path(s) of sources JSON; setting it enables the scheduler |
| `DIGEST_AT` / `TIMEZONE` | `06:00` / `Asia/Kolkata` | scheduler time (the job uses `UTC`) |
| `RUN_ON_START` | `false` | run the digest job at startup |
| `MAX_SITES` | `0` (no limit) | startup fails above this many distinct sites |
| `SUMMARY_PROVIDER` / `SUMMARY_MODEL` | `auto` / empty | `auto`, `gemini`, `extractive` |
| `SUMMARY_BATCH` / `SUMMARY_WORKERS` / `SUMMARY_WINDOW_DAYS` | `6` / `3` / `7` | summary batching and window |
| `GEMINI_API_KEY` | empty | optional |
| `IMAGE_CACHE_MB` | `128` | in-memory image cache |
| `USER_AGENT` | repo default | identify the crawler (put a contact URL) |
| `LOG_LEVEL` / `LOG_FORMAT` | `info` / `text` | logging |
| `PPROF_ADDR` | empty | pprof off |
| `GOMEMLIMIT` | unset | Go runtime soft memory limit (not an app setting) |
