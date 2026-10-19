# Benchmark

Command: `go run ./cmd/bench -database-url ... -redis-url ... -procs 1,2 -pages 500 -hosts 32` (`make bench`).

Machine: a 12-thread macOS (darwin/amd64) laptop, Go 1.26.0. **Postgres 17, Redis, the fake site and every crawler process all ran on that one machine**, so they competed for CPU. Workload: 32 fake hosts x 500 pages (16,000 pages), 20 ms server latency, crawler per-host delay 10 ms, 16 workers per process. Single run each; no repeats.

| crawler processes | pages | elapsed | pages/s | p50 fetch | p95 fetch | peak RSS per process | gate deferrals |
|---|---|---|---|---|---|---|---|
| 1 | 16000 | 70.7 s | 226 | 18 ms | 32 ms | 28 MiB | 22,809 |
| 2 | 16000 | 88.2 s | 181 | 18 ms | 35 ms | 28 MiB (55 MiB total) | 23,034 |

## What this shows

- **Two processes were not faster than one here (0.80x).** I did not get the scaling the plan hoped for, so no "X to Y pages/s across 2 workers" claim can be made from this run.
- Fetch latency is ~the server's 20 ms, so the crawler is not waiting on the network. Time goes elsewhere.
- The likely cause, **not verified**: database round trips. Each page costs a claim, an enqueue, a page write and a finish. On top of that, a worker denied by the per-host gate hands the URL back with an extra UPDATE. There were ~1.4 gate deferrals per page. With everything sharing one laptop's CPU, a second process mostly adds contention.
- An earlier 4-host run was capped by the gate itself (4 hosts x 1/10 ms = 400 pages/s), which is why this run uses 32 hosts.

## Ideas not done

Claim in batches, skip hosts known to be blocked when claiming, run Postgres on separate hardware, repeat runs and report variance.
