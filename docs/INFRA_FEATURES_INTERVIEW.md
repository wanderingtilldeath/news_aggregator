# Infrastructure features: implementation notes for interviews

CrawlerX started life as a multi-process, horizontally scalable crawler. Five pieces of infrastructure supported that goal. For the **personal single-box deployment** they were removed (see "Why removed" in each section). This document records how each one was built, why, what breaks, and the questions an interviewer is likely to ask, so the knowledge survives the deletion.

Code excerpts below are quoted from the tree **before** removal. Files that were unmodified at the time are also recoverable with `git show 8256e6d:<path>` (`internal/politeness/politeness.go`, `internal/robots/robots.go`, `internal/metrics/metrics.go`, `Dockerfile`). `cmd/crawlerx/main.go`, `internal/crawler/crawler.go` and `docker-compose.yml` had uncommitted edits at that point, so for those the excerpts here are the record.

## Contents
1. [The shared picture: what a crawl worker does](#1-the-shared-picture)
2. [Redis: shared politeness gate and robots cache](#2-redis-shared-politeness-gate-and-robots-cache)
3. [Prometheus metrics](#3-prometheus-metrics)
4. [fakesite: a deterministic hostile website](#4-fakesite-a-deterministic-hostile-website)
5. [Docker, compose and the second crawler](#5-docker-compose-and-the-second-crawler)
6. [Cross-cutting: scale-out vs scale-up](#6-cross-cutting-scale-out-vs-scale-up)
7. [The 60-second pitch](#7-the-60-second-pitch)

---

## 1. The shared picture

All the features below hang off one loop in `internal/crawler/crawler.go`. For each claimed URL (`process`):

```
claim from Postgres (lease)      <- frontier: SKIP LOCKED, status 'leased'
  -> robots.txt check            <- robots.Client (cache: local map + optional Redis)
  -> politeness gate Acquire     <- politeness.Gate (Redis Lua or in-memory)
       wait > 0 ? Reschedule(now+wait) and move on   (never sleep)
  -> fetch (size cap, HTML only)
  -> 304 / 2xx / 3xx / retryable / other
  -> finish(done|failed|blocked|skipped) or transient(retry w/ backoff)
  -> Metrics counters/histograms updated along the way
```

Key architectural facts worth saying out loud:

- **Postgres is the queue.** `frontier` is both the work queue and the visited set. Claim, lease, retry, dead-letter are all SQL.
- **Workers are stateless.** All durable state is in Postgres; the only cross-process state outside it was the Redis gate/robots cache.
- **The gate is an interface** (`politeness.Gate`) with two implementations. That seam is why Redis could be removed with a few dozen lines changed.

---

## 2. Redis: shared politeness gate and robots cache

### Problem
A polite crawler waits at least `HostDelay` (say 1s) between requests to one host, and honours `Crawl-delay` from robots.txt. With one process this is a map of `host -> next allowed time`. With **N processes** a per-process map means N times the intended rate, so the rule needs shared state with an atomic "check and reserve".

### Design
- Shared store: Redis, one key per host, value = "earliest next request time" in ms.
- `Acquire(host, delay)` returns **how long to wait**, `0` meaning "go now, and I have reserved the slot".
- It **never sleeps**. A denied worker reschedules the URL in Postgres for later and picks other work. One slow host cannot park the worker pool.
- `Penalize(host, d)` pushes the host's next slot out when the server says 429/503 with `Retry-After`.
- Same interface, second implementation `Memory` for tests and single-process runs.

### Implementation walk-through

**The interface** (`internal/politeness/politeness.go`):

```go
type Gate interface {
    // Acquire reserves the next request slot for host. It returns 0 if the
    // caller may fetch right now (the slot is consumed and the host's next
    // slot is pushed `delay` into the future), or the time to wait before
    // asking again. It never sleeps: callers reschedule instead of blocking.
    Acquire(ctx context.Context, host string, delay time.Duration) (wait time.Duration, err error)

    // Penalize pushes the host's next slot at least d into the future...
    Penalize(ctx context.Context, host string, d time.Duration) error
}
```

**The atomic step is a Lua script**, so the check and the reservation are one Redis operation:

```lua
local t = redis.call('TIME')
local now = t[1] * 1000 + math.floor(t[2] / 1000)
local nxt = tonumber(redis.call('GET', KEYS[1]) or '0')
if now >= nxt then
    redis.call('SET', KEYS[1], now + ARGV[1], 'PX', math.max(tonumber(ARGV[1]), 1))
    return 0
end
return nxt - now
```

Three deliberate choices in six lines:
1. **Atomicity.** Two processes calling `GET` then `SET` could both see "free" and both fetch. Redis runs a script single-threaded, so exactly one caller gets `0`.
2. **Redis `TIME`, not the client's clock.** Crawlers on different machines have skewed clocks; using the server's clock gives every process one timeline.
3. **`SET ... PX`.** Keys expire after the delay, so idle hosts leave no garbage and there is no cleanup job.

`penalizeScript` sets `next = max(current, now + d)`, so a penalty can only extend, never shorten, a wait.

**The in-memory twin**, same semantics under a mutex (also used by every unit test with an injectable clock):

```go
func (m *Memory) Acquire(_ context.Context, host string, delay time.Duration) (time.Duration, error) {
    m.mu.Lock(); defer m.mu.Unlock()
    now := m.now()
    if next := m.next[host]; now.Before(next) {
        return next.Sub(now).Round(time.Millisecond), nil
    }
    m.next[host] = now.Add(delay)
    return 0, nil
}
```

**How the crawler uses it** (`crawler.go`, in `process`):

```go
delay := max(c.cfg.HostDelay, crawlDelay)      // robots Crawl-delay can only slow us down
if c.deps.Gate != nil {
    wait, err := c.deps.Gate.Acquire(ctx, it.Host, delay)
    if err != nil {
        // gate unavailable: back off 2s instead of hammering the site
        return c.deps.Frontier.Reschedule(ctx, it, c.deps.Now().Add(2*time.Second), false, "gate unavailable")
    }
    if wait > 0 {
        // Not our turn on this host. Hand the URL back instead of sleeping
        // so this worker can serve other hosts meanwhile.
        *retryIn = wait + time.Millisecond
        return c.deps.Frontier.Reschedule(ctx, it, c.deps.Now().Add(*retryIn), false, "")
    }
}
...
case fetch.ShouldRetryStatus(res.Status):
    if res.RetryAfter > 0 && c.deps.Gate != nil {
        _ = c.deps.Gate.Penalize(ctx, it.Host, res.RetryAfter)   // pause the whole host
    }
```

Note the **fail-safe direction**: if Redis is down the worker does *not* fetch. A broken politeness gate must slow the crawler, not turn it into a DoS.

**Robots.txt** (`internal/robots/robots.go`) is the second Redis user:
- Parsed with `temoto/robotstxt`, following RFC 9309 semantics: 2xx parse; **4xx = allow all**; **5xx or unparsable = disallow all** (conservative); a redirect chain longer than `maxRedirects` counts as "unavailable".
- TTLs: good result 24h (`TTL`), bad result 5 min (`ErrorTTL`), so a temporarily broken robots.txt is retried soon but a good one is not refetched all day.
- `singleflight.Group` per origin: 8 workers hitting a new host at once produce **one** robots.txt fetch.
- Two cache levels: a process-local map, then an optional shared `Cache` interface. The Redis implementation stores the **raw response**, not the parsed object:

```go
type cachedResponse struct { Status int `json:"status"`; Body string `json:"body"` }
```
  Storing status+body means the cache format does not depend on the parser library's internals; each process re-parses with `interpret`. Cache writes are best-effort (`_ = Cache.Set(...)`): a cache failure never blocks a crawl.

**Wiring** (`cmd/crawlerx/main.go`): `-redis-url` / `REDIS_URL` was optional. If set: parse URL, ping with a 5 s timeout (fail fast at startup), build `politeness.NewRedis` + `robots.NewRedisCache`, register a `redis` check on `/healthz`. If empty: log a warning ("several crawler processes would exceed the host delay together") and use `politeness.NewMemory`.

### Failure modes and trade-offs
| Situation | Behaviour |
|---|---|
| Redis down | Gate errors -> URLs rescheduled 2 s ahead; no fetches. Fail-closed. Robots cache errors are ignored (fetch directly). |
| Redis restart (data loss) | Keys vanish; every host looks free for one request each. Brief burst of at most one extra request per host. Acceptable. |
| Fixed-slot vs token bucket | This is a fixed-interval slot (no burst). Simple and strictly polite; a token bucket would allow bursts, not wanted by a crawler. |
| Hot key | One key per host; a single huge host is one key written at ~1/delay Hz. Not a Redis scaling concern. |
| Network latency | One RTT per fetch, small next to a ~100 ms+ HTTP fetch. |

### How it was tested
- `internal/politeness/politeness_test.go`: allow-then-delay, hosts independent, **only one concurrent caller wins** (goroutine stampede), `Penalize`, zero delay never waits, run against `miniredis` (an in-process Redis), plus an opt-in test (`TEST_REDIS_URL`) with two separate clients on a real server to check the Lua script is not just passing on the fake.
- `internal/robots/robots_test.go`: allow/disallow/wildcards/crawl-delay, 4xx=allow, 5xx=disallow and retried after `ErrorTTL`, unparsable, redirect loop, per-origin caching, shared cache across two clients.
- `internal/e2e` `TestTwoProcessesShareOneHostGate`: two real OS processes crawl one host; the test sorts the fake site's request timestamps and asserts every consecutive gap is at least `delay - 60ms`.

### Interview Q&A
- **Why Lua rather than `GET` then `SET`?** Without atomicity two workers both read "free" and both fetch. Alternatives: `SET NX PX` per slot (works for a fixed interval too), or `WATCH/MULTI`. Lua keeps the logic in one round trip.
- **Why not use Postgres advisory locks or a table for the gate?** Possible (the queue is already there) but it puts a write on the hot path for every fetch attempt, including denied ones, and rows would need cleanup. Redis gives TTL expiry and sub-ms operations.
- **What if two crawlers' clocks differ?** Irrelevant here: time comes from Redis `TIME`. That is the point of doing it in the script.
- **Why return a wait instead of blocking?** Blocking couples worker count to the slowest host (head-of-line blocking). Returning a duration lets the worker defer that URL and serve another host.
- **Fail-open or fail-closed?** Closed for the gate (protect third-party sites), open for the robots cache (it is only an optimisation).
- **Why cache robots.txt raw?** Format stability and the parser can change without invalidating the cache.

### What I would do differently
Add jitter to the rescheduled time to avoid a thundering herd of retries landing together; use `PEXPIRE` with a floor on the key so a `Penalize` with a tiny value cannot create a zero-TTL key; make gate unavailability bounded (circuit breaker) rather than retrying every 2 s per URL.

### Why removed
One process cannot disagree with itself, so the in-memory gate gives the same politeness guarantee with no server, no network hop and ~100 MB less to run. `politeness.Memory` was already the default when `REDIS_URL` was empty.

---

## 3. Prometheus metrics

### Problem
Answer "is the crawler healthy and how fast is it?" without log grepping: throughput, errors, latency, backlog.

### Design
Seven collectors on a **private registry** (not Prometheus's global default), exposed at `GET /metrics`, scraped every 5 s. A `Metrics` struct is injected into the crawler (`Deps.Metrics`); if a test passes nil, `crawler.New` creates one on a throwaway registry so tests never share global state or panic on duplicate registration.

### Implementation walk-through (`internal/metrics/metrics.go`)

| Metric | Type | Updated where |
|---|---|---|
| `crawlerx_pages_total{status}` | CounterVec | `finish`: every URL reaching a terminal state (done/failed/blocked/skipped/dead) |
| `crawlerx_fetch_duration_seconds` | Histogram, buckets 5 ms..10 s | after each fetch, `res.Latency` |
| `crawlerx_queue_depth` | Gauge | sampled every 2 s by `sampleQueueDepth` (a goroutine with a ticker), not on every change |
| `crawlerx_retries_total` | Counter | `transient` (rescheduled after transient failure) |
| `crawlerx_robots_blocked_total` | Counter | robots disallow |
| `crawlerx_host_inflight{host}` | GaugeVec | `Inc` before fetch, `Dec` after |
| `crawlerx_gate_deferred_total` | Counter | gate said wait |

Wiring in `main.go`:

```go
reg := prometheus.NewRegistry()
reg.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
m := metrics.New(reg)
...
api.Deps{ Metrics: promhttp.HandlerFor(reg, promhttp.HandlerOpts{}) }   // GET /metrics
```
`api.Deps.Metrics` is an `http.Handler`, optional (`if deps.Metrics != nil { mux.Handle("GET /metrics", ...) }`), so the API package does not import Prometheus at all. Go runtime and process collectors give goroutines, GC, RSS and FDs for free. Compose's `deploy/prometheus.yml` (scrape every 5 s, targets `crawler1:8080`, `crawler2:8080`) lived behind a `--profile metrics` switch so the demo stack did not need it.

### Design decisions worth defending
- **Gauge sampled on a ticker** instead of updated per event: a `COUNT(*)` on every state change would add load to the very table that is the hot path; every 2 s is plenty for a dashboard.
- **Histogram for latency** (not an average): can compute p50/p95/p99 across processes with `histogram_quantile`, and averages hide tails.
- **Counters only go up**: rates are computed at query time (`rate(crawlerx_pages_total[1m])`), so a process restart does not corrupt the data.

### Failure modes and trade-offs
- **Label cardinality**: `host` as a label on `host_inflight` is unbounded in theory (every host ever crawled becomes a series). Fine for dozens of sites; dangerous for a broad crawl. It `Dec`s back to 0 but the series persists.
- **Pull model**: a crashed process simply disappears from the scrape; absence is detected with `up == 0`.
- **Multi-process**: each process exposes its own counters; Prometheus sums them. There is no shared registry.

### Interview Q&A
- **RED vs USE?** Rate = pages_total rate, Errors = failed/dead share, Duration = fetch histogram (RED); Utilisation/Saturation = host_inflight, queue_depth (USE).
- **Why a private registry?** Testability and no duplicate-registration panics in tests; the default registry is global mutable state.
- **Counter vs gauge vs histogram?** Monotonic event totals vs a value that goes up and down vs a distribution.
- **What would you alert on?** `queue_depth` growing while `rate(pages_total)` is flat (stuck), failure ratio above a threshold, `gate_deferred_total` rate spiking (a host throttling us), `up == 0`.
- **Push or pull?** Pull: the server owns discovery, targets stay simple, and health is implicit.

### What I would do differently
Drop the `host` label or cap it (top-N hosts); add exemplars or a per-crawl-id queue depth; export frontier counts by status from the DB (a custom collector) rather than only the last-sampled depth.

### Why removed
Nobody is scraping it. For one personal box `journalctl`, `/healthz` and the dashboard page (`index.html`, which reads the API) are enough. The seven `Metrics.*` call sites in `crawler.go` disappear with the package. `PPROF_ADDR` (off by default) remains for the rare memory question.

---

## 4. fakesite: a deterministic hostile website

### Problem
A crawler is a client for servers you do not control. Testing it against the real internet is flaky, slow, impolite, and cannot reproduce failure modes (a server that fails twice then recovers; an 8 MB page; a redirect loop). You need a **controllable adversary**.

### Design (`internal/fakesite/fakesite.go`)
`fakesite.Site` is an `http.Handler`, so it runs in-process under `httptest.NewServer` (fast, no ports to manage) **or** standalone via `cmd/fakesite` (for the Docker demo and benchmark).

- **Deterministic graph.** Pages `/` and `/p/{i}` form a `Fanout`-ary tree (every page reachable) plus pseudo-random cross links derived from a seeded integer hash (`mix`, `word`). Same `Seed` gives byte-identical pages, so test failures reproduce.
- **Options**: `Pages`, `Fanout`, `Seed`, `Latency` (added to every page, makes crashes land mid-crawl), `CrawlDelay` (advertised in robots.txt), `Chaos`, `Log` (one JSON line per request).
- **Chaos endpoints** under `/x/`, each aimed at one crawler behaviour:

| Path | Behaviour | What it proves |
|---|---|---|
| `flaky/*` | 503 twice then 200 (per-path counter under a mutex) | retry with backoff |
| `ratelimit` | 429 + `Retry-After: 1` once | host penalty via `Gate.Penalize` |
| `redirect` | 301 to `/p/1` | redirects are queued like links, not followed inline |
| `loop/a`, `loop/b` | redirect cycle | no infinite loop |
| `huge` | ~8 MB streamed | response size cap |
| `pdf` | `application/pdf` | non-HTML is skipped |
| `gone`, `error` | 410, 500 | terminal vs retried failures |
| `noindex`, `nofollow` | meta robots | meta robots respected |

- **A request recorder**: every request is stored as `Request{Time, Host, Path, Status, UA}`. `Requests()` / `PageRequests()` return them so tests assert on **what the server saw**: spacing between requests (politeness), the count per path (no re-crawl after kill -9), the User-Agent sent.
- **News section** (`/news/`, `digestPages`): articles with `og:type`, `og:image`, published times, plus an archive page whose bare `<time>` tags must **not** count as articles; used by the digest/scheduler tests and `internal/e2e/digest_test.go`.
- **Multi-host mode** in `cmd/fakesite`: `-hosts N` listens on N consecutive ports, each a separate "host" to the crawler, so per-host politeness does not cap benchmark throughput (the benchmark measured the crawler, not the delay).

### Why this beats mocks
A mock `Fetcher` returns what the test author believes servers do. A real HTTP server exercises the real client stack: TLS-less HTTP/1.1 behaviour, redirects, body limits, timeouts, header parsing. The fake site only fakes the *content*.

### How it was tested
`internal/fakesite/fakesite_test.go` tests the fixture itself (determinism, link graph reachability, each chaos endpoint). It is then the fixture for `internal/crawler/integration_test.go` (the chaos crawl against real Postgres), `internal/e2e/*` (kill -9, SIGTERM, two processes, digest), and `cmd/bench`.

### Interview Q&A
- **How do you test code that talks to third-party servers?** Hermetic fake server in-process; make time and failure injectable; assert on server-side observations.
- **Why deterministic?** Reproducible failures; seeded hash instead of `math/rand` global state; the tree guarantees reachability so "all N pages crawled" is a checkable invariant.
- **Test double vs mock vs stub vs fake?** This is a *fake*: a working lightweight implementation, not canned answers or call-expectation checks.
- **Why not record/replay (VCR)?** Fixtures rot, cannot express stateful failures (flaky N times), and cannot generate load shapes.
- **How do you test robustness?** Chaos endpoints, plus kill -9 at a random point and a final invariant check.

### What I would do differently
Make chaos behaviours configurable per request via query parameters (`/x/flaky?fail=3`), add slow-loris (trickling body) and TCP reset cases, and add a property-style test that varies `Seed`, `Fanout` and `Pages`.

### Why removed (partly)
`cmd/fakesite` (the standalone server) and `cmd/bench` were demo/benchmark tooling that required Docker and Redis. `internal/fakesite` itself is a **test fixture** used by the integration and end-to-end tests, so it stays as test-only code; deleting it would delete the best regression coverage the project has.

---

## 5. Docker, compose and the second crawler

### Problem
"Does it work with more than one process?" is a claim that needs a repeatable environment: Postgres, Redis, a fake site, two crawlers, and optionally Prometheus, all started with one command.

### Design
`make up` -> `docker compose up --build -d`.

**One Dockerfile for several binaries** (multi-stage, distroless):

```dockerfile
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download            # separate layer: re-runs only when deps change
COPY . .
ARG TARGET=crawlerx
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/${TARGET}

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
ENTRYPOINT ["/app"]
```
- `ARG TARGET` builds `crawlerx` or `fakesite` from the same file.
- `CGO_ENABLED=0` gives a fully static binary, which is what lets it run on `distroless/static` (no libc, no shell, no package manager): small image, small attack surface, runs as a non-root user.
- `-trimpath -ldflags="-s -w"` removes build paths and debug symbols (smaller, more reproducible).
- Layer ordering (`go.mod` first) maximises build cache.

**Compose** (`docker-compose.yml`):
- A YAML anchor `x-crawler: &crawler` defines the crawler service once (build args, mounted `sources.json` and `catalog/` read-only, env), and `crawler1` / `crawler2` inherit it with `<<: *crawler`, differing only in the published port (8081, 8082). A second anchor `&crawler-env` shares the environment.
- `postgres:17` and `redis:7` have healthchecks (`pg_isready`, `redis-cli ping`); crawlers use `depends_on: condition: service_healthy`, so they never start against a database that is still booting (the migration step would fail).
- `fakesite` runs with `-chaos`, 2000 pages, on port 9000.
- Prometheus is under `profiles: [metrics]`: opt-in (`--profile metrics`).
- Environment knobs: `DATABASE_URL`, `REDIS_URL`, `HOST_DELAY`, `SOURCES`, `MAX_SITES`, `GEMINI_API_KEY`, `DIGEST_AT`, `TIMEZONE`, `USER_AGENT`, with `${VAR:-default}` interpolation so `.env` overrides cleanly.

### Why two crawlers?
Not for capacity. They are the **proof** that the design is correct under concurrency: two processes share one queue and one gate, and either can be killed. Three mechanisms make that safe:

**1. Claim with `FOR UPDATE SKIP LOCKED` plus a lease**, in one statement (`internal/frontier/postgres.go`):

```sql
WITH picked AS (
    SELECT crawl_id, url_hash FROM frontier
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
FROM picked WHERE f.crawl_id = picked.crawl_id AND f.url_hash = picked.url_hash
RETURNING ...
```
- `SKIP LOCKED`: a second worker does not wait on rows another holds; it takes different rows. No double-claim, no blocking.
- **Lease** instead of a lock held for the duration: a process that dies leaves `status='leased'` with `lease_until` in the past, and the same query reclaims it later (`attempts + 1`, because a dead holder counts as a failed attempt). That is crash recovery with no coordinator.
- `ORDER BY depth, next_fetch_at`: shallowest first, so "depth 2" means "within 2 hops".
- A `Release` method hands leased rows back immediately on graceful shutdown (SIGTERM), so a clean restart does not wait for lease expiry.
- Terminal writes check `WHERE status = 'leased'` so a slow zombie worker cannot overwrite a row that was reclaimed and finished by someone else.

**2. The shared gate** (section 2) so two processes together obey one host delay.

**3. A once-per-day claim for the digest scheduler** (`internal/scheduler/claims.go`):

```go
INSERT INTO job_runs (job, run_day) VALUES ($1, $2::date) ON CONFLICT DO NOTHING
// RowsAffected() == 1  => this process won the day's run
```
Both crawlers run the scheduler; the primary key lets exactly one win. `MemoryClaims` is the test twin.

### How it was tested (`internal/e2e/e2e_test.go`, real subprocess binaries built with `-race`)
- `TestKillNineAndResumeDoesNotRecrawl`: process A crawls a 300-page site until ~60 pages are done, then `kill -9`. The test records which rows were leased at death. Process B restarts with no seed. Assertions: all 300 pages end `done`, none left pending/leased, and **no URL completed before the crash was fetched again** (only URLs in flight at death may appear twice; none more than twice). "No re-crawl" is checked on the server side via the fake site's request log.
- `TestSIGTERMReleasesLeasesAndExitsCleanly`: SIGTERM -> process exits cleanly and **zero rows remain leased**.
- `TestTwoProcessesShareOneHostGate`: described in section 2.

### Interview Q&A
- **Why a Postgres queue instead of Kafka/SQS/RabbitMQ?** Dedup, state, retries, scheduling by time (`next_fetch_at`) and querying all come free with SQL; the volume (hundreds of sites/day) is nowhere near needing a broker. Trade-off: polling, and the table is a hot spot at very high scale.
- **What does `SKIP LOCKED` guarantee, and what does it not?** Workers do not block each other and never claim the same row concurrently. It does not give you fairness or ordering across workers, and the lease handles the crash case.
- **At-least-once or exactly-once?** At-least-once: a worker can finish the fetch and die before the terminal write, so the URL may be fetched again. Page writes are idempotent (`ON CONFLICT (url_hash) DO UPDATE` in `internal/store/postgres.go`) so the effect is exactly-once.
- **How do you pick the lease length?** Longer than the worst-case fetch + processing time (here 1 min vs a 15 s timeout), short enough that a crash recovers quickly. Too short causes duplicate work and zombie writers (guarded by `status='leased'`).
- **Why distroless/static?** Tiny image, no shell, non-root; requires `CGO_ENABLED=0`. Downside: nothing to `exec` into for debugging.
- **Why separate `go mod download` layer?** Docker layer caching: dependencies change rarely, source changes constantly.
- **Why healthchecks and `service_healthy`?** `depends_on` alone only waits for the container to start, not for the database to accept connections.

### What I would do differently
Pin images by digest, add `restart: unless-stopped`, run the migration as a separate one-shot service. (Today each crawler calls `db.Migrate` at startup; that is safe because goose is configured with a Postgres session locker, `lock.NewPostgresSessionLocker()`, so two crawlers starting together serialise on an advisory lock.)

### Why removed
For one personal box a single binary under systemd is lighter and simpler: Docker's daemon alone would take ~80-120 MB of a tiny server's RAM, building the image needs more memory than the server has, and a second crawler does nothing useful for a once-a-day job. The crash-recovery design (leases, `SKIP LOCKED`, `Release`) stays in the code: it still protects a single process from `kill -9` and OOM kills, and the kill-9 and SIGTERM tests still run. Only the two-process test and the compose file go.

---

## 6. Cross-cutting: scale-out vs scale-up

| Concern | Multi-process version (before) | Single-process version (after) |
|---|---|---|
| Per-host politeness | Redis Lua gate shared by all | In-memory `politeness.Memory` |
| robots.txt cache | Local map + Redis | Local map only (24 h TTL) |
| Observability | Prometheus scrape of N processes | Logs, `/healthz`, pprof if needed |
| Packaging | Docker image + compose | Static binary + systemd + nginx |
| Test environment | Postgres + Redis + fakesite | Postgres only; fakesite is in-process |
| Crash recovery | Leases + `SKIP LOCKED` | Same: still needed for kill -9 / OOM |
| Digest job exactly-once | `job_runs` primary-key claim | Same: harmless with one process, protects accidental double start |

Lessons you can quote:
1. **Put the seam in early.** `Gate` and `Cache` were interfaces, so removing Redis was deleting an implementation, not redesigning.
2. **Scale to the real requirement.** The distributed design proved correctness under concurrency; the product (a daily personal digest) needs none of it. Choosing the simple deployment is an engineering decision, not a regression.
3. **Keep what protects correctness, drop what protects scale.** Leases and idempotent writes stay; shared gates and scrapers go.
4. **Tests are the real asset.** The kill -9 test and the chaos site are what let you remove infrastructure without fear.

## 7. The 60-second pitch

"I built a resumable, polite crawler in Go with Postgres as a lease-based work queue, `FOR UPDATE SKIP LOCKED` claims, and exponential backoff with jitter. For politeness across processes I used a Redis Lua script that reserves a per-host time slot atomically using the server clock, and returns a wait instead of sleeping so workers never block on a slow host. It exposed Prometheus metrics, and was tested against a deterministic fake website with chaos endpoints and end-to-end `kill -9` tests asserting that nothing finished before the crash is re-crawled. I packaged it with a static distroless image and ran two crawlers in compose to prove horizontal scaling. For the personal deployment I deliberately removed Redis, Prometheus and Docker and ran one binary under systemd behind nginx, because the interface seams made that a deletion rather than a rewrite."
