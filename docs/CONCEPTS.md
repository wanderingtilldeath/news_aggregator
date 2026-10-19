# Concepts & System Design Guide

This document explains **how this crawler works, why it's structured the way
it is, and which Go/system-design ideas each piece demonstrates**. It's
written for someone new to Go who wants to use this repository as a study
aid for SDE interviews. Every concept below is tied to real code in this
repository — read the explanation, then open the referenced file.

---

## 1. What this program does

Starting from one seed URL, the crawler:

1. Fetches the page over HTTP.
2. Parses out every hyperlink on it.
3. Enqueues each newly-discovered link as a new unit of work, one hop
   deeper than the page it came from.
4. Repeats, using a fixed pool of goroutines working off a shared queue,
   until there's no more work within the configured depth limit, or the
   process is asked to shut down.

It's a small program, but it exercises almost every concurrency primitive
Go has: goroutines, channels, `select`, `sync.WaitGroup`, `sync.Mutex`,
atomics, and `context.Context`. That combination — a **bounded worker pool
consuming a self-feeding queue, with graceful cancellation** — appears
constantly in real backend systems (task queues, message consumers,
batch pipelines), which is exactly why it's a good interview topic.

---

## 2. Project layout, and why it's shaped this way

```
cmd/crawler/              entrypoint: flags, logger, signal handling, wiring
internal/crawler/         worker pool, job queue, dedup, metrics, orchestration
internal/fetcher/         HTTP fetch, behind an interface
internal/linkextractor/   HTML parsing -> links, behind an interface
```

This is the standard shape of a non-trivial Go program:

- **`cmd/<binary-name>/main.go`** is the *only* place that knows about the
  outside world for a given binary: command-line flags, environment,
  process signals, `os.Exit`. If this project grew a second binary (say, a
  `cmd/crawler-server` that exposes crawl results over HTTP), it would get
  its own `main.go` under `cmd/`, and both binaries would import the same
  `internal/` packages. This is why logic never lives in `main.go` beyond
  wiring — it keeps the actual logic reusable and, crucially, testable
  without spinning up a process.

- **`internal/`** is not just a convention — the Go compiler enforces it.
  A package under `internal/` can only be imported by code rooted at the
  parent of the `internal` directory (here, anything under the module
  `github.com/wanderingtilldeath/news-aggregator`). Try to import
  `.../internal/crawler` from a different module and it's a compile error.
  This is Go's built-in answer to "how do I expose an API publicly but keep
  implementation details private?" — no `public`/`private` keywords needed,
  just where a package lives on disk.

- **One package per concern** (`crawler`, `fetcher`, `linkextractor`).
  Each package has a single, narrow responsibility:
  - `fetcher` only knows how to turn a URL into an `*http.Response`.
  - `linkextractor` only knows how to turn an HTML body into a list of
    absolute URLs — it does zero I/O, which is exactly why it's the
    simplest thing in the codebase to unit test (see §7).
  - `crawler` only knows how to *orchestrate* — schedule work, dedupe,
    track metrics — and treats fetching/parsing as black boxes it was
    handed. It doesn't know or care that fetching means HTTP, or that
    parsing means HTML.

  This is **separation of concerns**: each piece can be understood, changed,
  and tested in isolation. It's also what makes dependency injection
  (§4.6) possible.

---

## 3. Concurrency patterns in this codebase

### 3.1 Bounded worker pool

`internal/crawler/crawler.go:87-89` starts a **fixed** number of goroutines
(`Config.Workers`) up front, all reading from the same channel:

```go
for i := 1; i <= c.cfg.Workers; i++ {
    go c.worker(ctx, i)
}
```

Contrast this with spawning one goroutine *per job* (unbounded
concurrency). Unbounded concurrency for a crawler is dangerous: a popular
page can have hundreds of links, so depth-2 alone could try to open
thousands of simultaneous HTTP connections, exhausting file descriptors and
hammering the target host. A **bounded pool** caps how much work happens in
parallel regardless of how much work is *discovered* — the classic fix for
"fan-out that could otherwise be unbounded."

### 3.2 Job queue = channel

`c.jobs` (`internal/crawler/crawler.go:45`) is a buffered `chan Job`. This
is Go's built-in **producer/consumer queue**: workers *are* consumers, and
they're also producers (a page's links become new jobs). A channel gives
you, for free, what you'd otherwise hand-roll with a mutex + condition
variable + slice: safe concurrent enqueue/dequeue, and blocking semantics
when the queue is empty (a receive blocks) or full (a send blocks).

### 3.3 Backpressure via a buffered channel

The channel is created with capacity `Config.QueueSize`
(`internal/crawler/crawler.go:65`, wired to `100` in
`cmd/crawler/main.go:35`). A **buffered** channel of size N lets up to N
sends complete without a receiver ready; the (N+1)th send blocks until
something is received. This is **backpressure**: if pages are discovering
links faster than workers can process them, producers (the workers
themselves, since they enqueue the links they find) simply block on send
instead of the queue growing without bound. It's the same idea as a bounded
queue in front of a thread pool in Java, or a bounded channel in front of a
goroutine pool anywhere else — pick a buffer size that absorbs bursts
without letting memory grow unboundedly.

### 3.4 `sync.WaitGroup` — knowing when there's no more work

The tricky part of a *self-feeding* queue (workers enqueue more work than
they dequeue) is knowing when to stop: you can't just close the channel
after the first job, because more jobs get added as you go, and you can't
close it when the channel is momentarily empty either (a worker might be
about to add more). `internal/crawler/crawler.go` uses a `sync.WaitGroup`
as the source of truth for "is there any pending or in-flight work?":

- `c.wg.Add(1)` happens *before* a job is handed to the channel — once for
  the seed URL (`crawler.go:91`), once per discovered link
  (`crawler.go:173`).
- `c.wg.Done()` happens after a worker finishes processing a job
  (`crawler.go:115`).
- `c.wg.Wait()` (`crawler.go:98`) blocks the orchestrating goroutine until
  every `Add` has a matching `Done` — i.e., the counter returns to zero,
  meaning no job is queued *and* no job is mid-flight.

Only once `Wait()` returns is it safe to `close(c.jobs)` (`crawler.go:99`):
closing a channel while someone might still send on it panics, so ordering
here matters. **A `WaitGroup`'s counter must always be incremented before
the goroutine it's tracking starts, never from inside that goroutine** —
notice `Add(1)` always happens at the call site that hands off the job,
never inside `worker`/`crawl` itself. This is a very common source of bugs
in real code — an `Add` that races with the matching `Wait()` because it
happened too late.

### 3.5 Rate limiting with `time.Ticker`

`c.rateLimiter` (`internal/crawler/crawler.go:48`) is a single
`*time.Ticker` shared by every worker. A ticker's channel (`.C`) receives a
value every `RateLimit` interval. Because it's one channel shared by all
workers, and a channel receive delivers each value to exactly *one*
receiver, the ticker naturally rate-limits the whole pool combined, not
each worker independently — five workers waiting on the same ticker
collectively make one request per tick, not five. This is a minimal
stand-in for a **token bucket**: each tick is a token; a worker can proceed
only once it "spends" one (`crawler.go:129-133`).

### 3.6 Graceful shutdown via `context.Context`

`context.Context` is Go's standard way to propagate a single "please stop"
signal to arbitrarily many goroutines at once. `cmd/crawler/main.go:39`
creates a cancellable context; a background goroutine
(`listenForShutdown`, `main.go:55-63`) waits on OS signals (`SIGINT`,
`SIGTERM` — i.e. `Ctrl+C` or `docker stop`) and calls `cancel()` when one
arrives. Every goroutine that matters — each worker (`crawler.go:105`), and
each blocking point inside `crawl` (waiting for a rate-limit tick,
`crawler.go:130`; waiting to enqueue a link, `crawler.go:175`) — has a
`select` with a `case <-ctx.Done():` branch, so cancellation is observed
*wherever* a goroutine happens to be blocked, not just at the top of a
loop. This is what makes shutdown prompt instead of "eventually, whenever
the current job finishes" — see `TestCrawler_ContextCancellationStopsPromptly`
in `internal/crawler/crawler_test.go` for a test that asserts exactly this.

---

## 4. Core language mechanics, explained

### 4.1 Goroutines

A goroutine (`go someFunc()`) is a function running concurrently with its
caller, scheduled by the Go runtime rather than the OS. The runtime
multiplexes potentially thousands of goroutines onto a small number of OS
threads (an "M:N" scheduler — M goroutines over N OS threads), and a
goroutine's stack starts at a few KB and grows as needed. That's why
spawning thousands of goroutines is normal in Go, while doing the same with
OS threads elsewhere is not. This project still deliberately *bounds* how
many run concurrently (§3.1) — cheap goroutines don't mean unlimited
concurrency is free; the limiting resource here is the target server and
your own file descriptors/sockets, not goroutine memory.

### 4.2 Channels

A channel is a typed, concurrency-safe pipe: `ch <- v` sends, `v := <-ch`
receives, both block until the other side is ready (for an **unbuffered**
channel), or until the buffer has room/data (for a **buffered** one,
`make(chan T, n)`). `close(ch)` signals "no more values will ever be
sent" — receivers see `v, ok := <-ch` with `ok == false` once the channel is
both closed and drained (used in `worker`, `crawler.go:114`, to notice the
job queue is done). Sending on a closed channel panics; receiving from one
never blocks. A nil channel blocks forever on send and receive, which is
occasionally used deliberately to "disable" a `select` case — not used
here, but worth knowing.

### 4.3 `select`

`select` waits on multiple channel operations at once and proceeds with
whichever is ready first (randomly, if several are ready simultaneously).
It's how this program achieves "block on the next job, *unless* we've been
cancelled" (`crawler.go:104-116`) — a single-channel receive can't express
"or stop waiting if X happens"; `select` with two cases can. A `select`
with a `default` case never blocks — useful for a non-blocking check,
though this codebase mostly uses two-case blocking selects (job-or-cancel,
tick-or-cancel).

### 4.4 `sync.Mutex` vs. atomics

Both protect shared, mutable state from data races (two goroutines
accessing the same memory concurrently, at least one a write). The
difference is what they protect:

- **`sync.Mutex`** guards a *critical section* — arbitrary code, often
  touching multiple related fields (or a whole data structure like a map)
  that must change together. `visitedSet` (`internal/crawler/visited.go`)
  uses a mutex because "check the map, then conditionally write to it" is
  one logical operation (`markVisited`) that must happen atomically as a
  whole — a mutex is the natural fit for a map, which has no atomic
  equivalent in the standard library.
- **`sync/atomic`** (`atomic.Int64`, etc.) protects a *single word* of
  memory with lock-free CPU instructions — faster than a mutex, but it can
  only do simple things (add, load, compare-and-swap) to one value at a
  time. `Metrics` (`internal/crawler/metrics.go`) uses three independent
  atomics rather than a mutex + struct: incrementing "pages crawled" never
  needs to block on, or be blocked by, incrementing "pages failed" — they're
  unrelated counters, so giving each one its own lock-free counter avoids
  contention that a single mutex around the whole struct would introduce.

  One tradeoff worth knowing for interviews: `Metrics.Snapshot()`
  (`metrics.go:37-50`) reads two independent atomics (`pagesCrawled`,
  `totalLatencyNs`) one after another. Between those two reads, another
  goroutine could record a success, so the pair is not perfectly
  consistent with each other at the exact same instant. That's an
  acceptable tradeoff for a summary stat printed once at the end, but it's
  exactly the kind of thing an interviewer will probe: "when would this
  matter, and how would you fix it?" (Answer: wrap both fields in one
  mutex, or pack them into a single value updated with one CAS, if you
  needed a truly atomic combined snapshot.)

- **`visitedSet` and `Metrics` intentionally have separate locks.** Putting
  everything behind one mutex is simpler to write but serializes unrelated
  work — a worker recording a metric would block a worker that's just
  trying to check whether a *different* URL was visited. Splitting locks by
  what they actually protect is a basic but important scalability lever:
  the smaller and more specific a critical section, the less contention.

### 4.5 `context.Context`

`context.Context` carries a cancellation signal (and optionally deadlines
and request-scoped values) through a call chain, without every function in
between needing to know why it might stop. Convention: it's always the
first parameter, named `ctx`, never stored in a struct field long-term. In
this codebase, one root `context.Context` (built once, in `main`) is passed
into `Crawler.Start`, and from there into every worker and every
`fetcher.Fetch` call — cancelling that one context stops HTTP requests
in-flight (because `http.NewRequestWithContext`, `fetcher.go:32`, ties the
request's lifetime to it) *and* every goroutine blocked in a `select` on
`ctx.Done()`, with one function call.

### 4.6 Interfaces and dependency injection

Go interfaces are satisfied **implicitly** — a type doesn't declare "I
implement `Fetcher`"; it just needs the right method signature, and the
compiler figures out the rest. That's the whole trick behind this
project's tests: `crawler.Crawler` depends on the `fetcher.Fetcher` and
`linkextractor.Extractor` **interfaces**
(`internal/crawler/crawler.go:38-39`), not on concrete `HTTPFetcher`/
`HTMLExtractor` types. `crawler.New(cfg, f, e, logger)` takes those
interfaces as parameters — this is **dependency injection**: the caller
decides what implementation to hand in, rather than the callee reaching
out and constructing (or hardcoding) its own dependencies.

In production (`cmd/crawler/main.go:37`), real implementations are passed
in. In tests (`internal/crawler/crawler_test.go`), the *real* fetcher and
extractor are still used, but pointed at an in-process `httptest.Server`
instead of the real internet — no network flakiness, no rate limits, fully
deterministic, and fast (the whole suite runs in under two seconds). If you
needed to unit-test, say, a single malformed-HTML edge case without
standing up a server at all, you could hand `Crawler` a five-line
hand-written fake `Extractor` — no mocking framework required, because
Go's structural typing makes "a struct with an `Extract` method" a valid
`Extractor` automatically.

### 4.7 Error handling and `defer`

Go has no exceptions for ordinary error paths: functions that can fail
return an `error` as their last value, and callers check it immediately
(`if err != nil { ... }`) rather than relying on unwinding. `defer`
schedules a call to run when the surrounding function returns, regardless
of which `return` triggered it — used throughout for cleanup that must
happen on every exit path: closing a response body
(`crawler.go:144: defer resp.Body.Close()`), stopping the rate-limit ticker
(`crawler.go:85: defer c.rateLimiter.Stop()`), or firing a deferred
`cancel()` in `main` (`main.go:40`) so the context is always cancelled
before the process exits, even on unexpected early returns.

---

## 5. System design principles this project demonstrates

These are the ideas an interviewer is really listening for — the code
above is just where to point at a concrete example of each.

- **Separation of concerns / layered architecture** (§2): orchestration,
  network I/O, and parsing are three independently replaceable layers.
  You could swap `HTTPFetcher` for one that fetches via a proxy, or
  `HTMLExtractor` for one that also extracts `<img>` sources, and nothing
  in `crawler.go` would need to change.

- **Dependency inversion via interfaces** (§4.6): the high-level policy
  (`Crawler`, which decides *what* to crawl and *when* to stop) depends on
  abstractions (`Fetcher`, `Extractor`), not concrete implementations —
  the concrete types depend on (implement) those abstractions, not the
  other way around. This is what makes the system both testable and
  extensible without modification.

- **Idempotency / deduplication** (`visitedSet`, §4.4): the same URL is
  never processed twice, which is what makes it safe for the crawl graph
  to contain cycles (page A links to B links back to A) without infinite
  work. Real systems apply the same idea to, e.g., deduplicating retried
  messages in a queue by an idempotency key.

- **Backpressure and bounded resource usage** (§3.1, §3.3): a fixed worker
  count and a bounded queue mean memory and outbound-connection usage are
  capped no matter how large the link graph turns out to be — a program
  that behaves predictably under load, rather than degrading (or OOMing)
  as load grows.

- **Rate limiting** (§3.5): protects a downstream dependency (the site
  being crawled) from being overwhelmed by a client's own concurrency —
  the same reasoning behind rate limiting *your own* API's clients, just
  from the other side.

- **Graceful shutdown** (§3.6): a production service should stop accepting
  new work and let in-flight work either finish or cancel promptly when
  asked to stop — not be killed mid-request. This is what separates "polite"
  shutdown (drain, then exit) from a hard `os.Exit`/`kill -9`.

- **Observability via metrics**: `Metrics`/`Snapshot` is a minimal example
  of a very common pattern — separating *counters that get incremented in
  the hot path* (cheap, lock-free) from *reading a consistent view of
  them* (done rarely, e.g. once at the end, or periodically for a
  dashboard). Real systems do the same thing at a larger scale with tools
  like Prometheus.

- **Graceful degradation on partial failure**: one page returning a 404 or
  timing out only marks that one page failed (`crawl.go` calls
  `RecordFailure` and returns) — it doesn't abort the whole crawl. A
  system that lets one failed unit of work take down the entire job is
  much more fragile than one where failures are isolated and recorded.

### How would you scale this beyond one process?

A natural interview follow-up. Some directions, deliberately **not**
implemented here to keep the codebase minimal, but worth being able to
discuss:

- **Distributed frontier queue**: replace the in-memory `chan Job` with a
  durable, shared queue (e.g. Kafka, SQS, Redis Streams) so multiple
  crawler processes on different machines can pull from the same backlog
  instead of each only knowing about its own in-memory jobs.
- **Distributed deduplication**: `visitedSet`'s in-memory map only works
  within one process. At scale you'd use a shared store (e.g. Redis
  `SETNX` for an exact set, or a **bloom filter** if the visited set is too
  large to store exactly and a small false-positive rate is acceptable).
- **Persistence**: this repo used to ship an unused `docker-compose.yml`
  with a Postgres container. A natural real use for a database here would
  be persisting the frontier (pending/visited URLs) and crawl results so a
  crash doesn't lose all progress — durable state instead of purely
  in-memory. It was removed since nothing in the code used it yet, but
  it's exactly the kind of component you'd add first when moving from "one
  process, in-memory" to "durable, multi-process."
- **Politeness at scale**: real crawlers respect `robots.txt` and rate-limit
  *per host* (not globally), since crawling many different sites
  concurrently shouldn't rate-limit them against each other the way this
  single shared ticker does.
- **Retries with backoff**: a transient failure (timeout, 503) is currently
  just recorded as failed. A production crawler would typically retry with
  exponential backoff before giving up.

---

## 6. Interview-question quick map

| Question | Where the answer lives |
|---|---|
| How do you cap concurrency in Go? | Fixed-size worker pool, §3.1, `crawler.go:87-89` |
| Buffered vs. unbuffered channel — when would you use each? | §4.2, §3.3 (buffered channel as a bounded queue) |
| How does cancellation propagate to many goroutines at once? | §3.6, §4.5, `context.Context` + `select`/`ctx.Done()` everywhere |
| How do you know when a self-feeding worker pool has no more work? | §3.4, `sync.WaitGroup` counting in-flight + queued jobs |
| `sync.Mutex` vs `atomic` — how do you choose? | §4.4 |
| How do you unit-test concurrent code deterministically? | §4.6, `internal/crawler/crawler_test.go` (real logic, fake network via `httptest`) |
| How do you avoid a goroutine leak? | Every blocking point has a `ctx.Done()` escape hatch (§3.6); see `TestCrawler_ContextCancellationStopsPromptly` |
| What's the difference between a data race and a logical race? | A data race is unsynchronized memory access (`go test -race` catches it); `visitedSet.markVisited` also prevents the *logical* race of two workers both deciding to crawl the same URL |
| How would this scale past one machine? | §5, "How would you scale this beyond one process?" |

---

## 7. Suggested exercises

Once you're comfortable with the code above, try extending it yourself —
each of these touches a different part of the layout in §2:

1. Add a `-max-pages` flag that stops the crawl after N pages regardless of
   depth (touches `Config` and `Crawler.crawl`).
2. Add per-host rate limiting instead of one global ticker (touches
   `Crawler`'s rate limiter, keyed by host).
3. Write a fake `linkextractor.Extractor` by hand (no server needed) and
   use it to unit-test a `Crawler` edge case purely in memory.
4. Swap `visitedSet`'s `map` + `sync.Mutex` for a `sync.Map` and reason
   about whether it would actually help here (hint: `sync.Map` is optimized
   for keys written once and read many times, or disjoint key sets per
   goroutine — think about whether that matches this access pattern).
