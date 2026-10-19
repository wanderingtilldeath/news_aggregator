# CrawlerX

A resumable, polite web crawler in Go. Work lives in Postgres (a lease-based queue using `FOR UPDATE SKIP LOCKED`), per-host politeness is shared through Redis, and any number of processes can crawl together. Kill one with `kill -9` and the rest (or a restart) finish its work without re-crawling pages that were already done.

**Only crawl your own site, or sites that allow it.** The API has no authentication and `POST /crawls` makes the server fetch URLs: keep it on a private network.

## Quick start

```sh
make up                                   # postgres, redis, fakesite, 2 crawlers (needs Docker)
curl -XPOST localhost:8081/crawls -d '{"seed":"http://fakesite:9000/","max_depth":6}'
curl localhost:8081/crawls/1              # counts by status, "done": true when finished
curl 'localhost:8081/search?q=nebula'
open http://localhost:8081                # dashboard: status breakdown, throughput, depth chart, search
curl localhost:8081/metrics
```

Without Docker, run Postgres and Redis yourself:

```sh
export DATABASE_URL=postgres://... REDIS_URL=redis://...
go run ./cmd/fakesite -pages 500 &
go run ./cmd/crawlerx -seed http://127.0.0.1:9000/ -max-depth 6 -exit-when-done
```

To **resume** after a crash, restart `crawlerx` *without* `-seed`: it works whatever is pending. (`-seed` starts a new crawl.)

## How it works

| piece | what it does |
|---|---|
| `internal/frontier` | queue + visited set in one table; claim with `SKIP LOCKED`; expired leases are re-claimed (crash recovery); shallowest-first so the depth limit means "within N hops" |
| `internal/politeness` | Redis Lua gate: one slot per host per delay, shared by all processes; a denied worker reschedules the URL instead of sleeping |
| `internal/robots` | RFC 9309 via `temoto/robotstxt`: Allow/Disallow, wildcards, Crawl-delay; 4xx = allow, 5xx = disallow; cached in Redis |
| `internal/fetch` | size cap (2 MiB), HTML only, honest User-Agent, no inline redirects (a redirect target is queued like any link), `Retry-After`, full-jitter backoff |
| `internal/parse`, `internal/urlnorm` | one parse per page; `<base href>`, meta robots; URL canonicalisation with a fuzz test |
| `internal/store` | batched page writes, `tsvector` + GIN full-text search, ETag/Last-Modified |

Config is flags with env fallbacks: see `go run ./cmd/crawlerx -h`.

## Tests

```sh
go test -short ./...                      # no services needed
make test                                 # all tests, -race, needs TEST_DATABASE_URL and TEST_REDIS_URL
```

The suite includes the original deadlock regression (`internal/crawler/deadlock_test.go`) and end-to-end `kill -9` / SIGTERM / two-process tests (`internal/e2e`). Benchmark: `make bench`, results and caveats in [docs/BENCHMARK.md](docs/BENCHMARK.md). Concept notes: [docs/CONCEPTS.md](docs/CONCEPTS.md).

## News digest

Point crawls at news or notification listing pages (a shallow `max_depth`, 1 to 2, works well), then read what is new:

```
curl -XPOST localhost:8081/crawls -d '{"seed":"https://example.gov/press-releases","max_depth":1}'
curl 'localhost:8081/digest?days=1'              # last 24h, newest first
curl 'localhost:8081/digest?days=7&q=exam+notice' # last week, filtered by full-text query
```

Each item has `url, title, description, snippet, time`. `time` is the article's published date taken from page metadata (`article:published_time`, `datePublished`, or the first `<time datetime>`); pages without one use the moment they were first crawled. Params: `days` 1-30, `limit` 1-200 (default 50), `q` optional.
