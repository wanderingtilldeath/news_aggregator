// Command bench measures crawlerx throughput against the synthetic site with
// one and with several crawler processes sharing one Postgres and one Redis.
//
// It starts the fake site in-process (one port per "host", so per-host
// politeness does not cap the result), creates one crawl per host, runs N
// crawlerx subprocesses over the shared queue, and reports pages/s, fetch
// latency percentiles (from each process's Prometheus histogram) and peak
// memory (RSS, sampled with ps).
//
//	go run ./cmd/bench -database-url postgres://... -redis-url redis://... -procs 1,2
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/wanderingtilldeath/news-aggregator/internal/db"
	"github.com/wanderingtilldeath/news-aggregator/internal/fakesite"
	"github.com/wanderingtilldeath/news-aggregator/internal/frontier"
)

type options struct {
	databaseURL string
	redisURL    string
	procs       []int
	pages       int
	hosts       int
	fanout      int
	latency     time.Duration
	hostDelay   time.Duration
	workers     int
	basePort    int
	metricsPort int
	binary      string
	out         string
}

type result struct {
	procs      int
	pages      int
	elapsed    time.Duration
	pagesPerS  float64
	p50, p95   time.Duration
	rssPeakMiB float64 // largest single process
	rssSumMiB  float64 // sum over processes at their peaks
	retries    int64
	deferred   int64
}

func main() {
	var (
		o        options
		procsArg string
	)
	flag.StringVar(&o.databaseURL, "database-url", os.Getenv("DATABASE_URL"), "Postgres URL (a disposable database: it is truncated)")
	flag.StringVar(&o.redisURL, "redis-url", os.Getenv("REDIS_URL"), "Redis URL")
	flag.StringVar(&procsArg, "procs", "1,2", "comma-separated crawler process counts to compare")
	flag.IntVar(&o.pages, "pages", 2000, "pages per fake host")
	flag.IntVar(&o.hosts, "hosts", 8, "fake hosts (ports)")
	flag.IntVar(&o.fanout, "fanout", 6, "links per page")
	flag.DurationVar(&o.latency, "latency", 20*time.Millisecond, "artificial server latency per page")
	flag.DurationVar(&o.hostDelay, "host-delay", 10*time.Millisecond, "crawler per-host delay (keep low so hosts*1/delay does not cap throughput)")
	flag.IntVar(&o.workers, "workers", 16, "workers per crawler process")
	flag.IntVar(&o.basePort, "base-port", 9300, "first fake-site port")
	flag.IntVar(&o.metricsPort, "metrics-port", 9390, "first crawler metrics port")
	flag.StringVar(&o.binary, "crawlerx", "", "path to a crawlerx binary (default: build one)")
	flag.StringVar(&o.out, "out", "", "also write the markdown report to this file")
	flag.Parse()

	if o.databaseURL == "" || o.redisURL == "" {
		log.Fatal("need -database-url and -redis-url (or DATABASE_URL / REDIS_URL)")
	}

	for _, s := range strings.Split(procsArg, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil || n < 1 {
			log.Fatalf("bad -procs value %q", s)
		}
		o.procs = append(o.procs, n)
	}

	if o.binary == "" {
		dir, err := os.MkdirTemp("", "crawlerx-bench")
		if err != nil {
			log.Fatal(err)
		}
		defer os.RemoveAll(dir)

		o.binary = filepath.Join(dir, "crawlerx")
		log.Printf("building crawlerx...")
		if out, err := exec.Command("go", "build", "-o", o.binary, "./cmd/crawlerx").CombinedOutput(); err != nil {
			log.Fatalf("build: %v\n%s", err, out)
		}
	}

	var results []result
	for _, n := range o.procs {
		log.Printf("--- %d process(es) ---", n)
		r, err := runOnce(o, n)
		if err != nil {
			log.Fatalf("run with %d processes: %v", n, err)
		}
		log.Printf("%d process(es): %d pages in %v = %.0f pages/s, p95 fetch %v", n, r.pages, r.elapsed.Round(time.Millisecond), r.pagesPerS, r.p95)
		results = append(results, r)
	}

	report := render(o, results)
	fmt.Println(report)
	if o.out != "" {
		if err := os.WriteFile(o.out, []byte(report), 0o644); err != nil {
			log.Fatal(err)
		}
	}
}

func runOnce(o options, procs int) (result, error) {
	ctx := context.Background()

	pool, err := db.Open(ctx, o.databaseURL, 4)
	if err != nil {
		return result{}, err
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		return result{}, err
	}
	if _, err := pool.Exec(ctx, `TRUNCATE crawls, pages CASCADE`); err != nil {
		return result{}, err
	}

	ropts, err := redis.ParseURL(o.redisURL)
	if err != nil {
		return result{}, err
	}
	rdb := redis.NewClient(ropts)
	defer rdb.Close()
	if keys, _ := rdb.Keys(ctx, "crawlerx:*").Result(); len(keys) > 0 {
		rdb.Del(ctx, keys...)
	}

	// Fake site: one listener per host.
	site := fakesite.New(fakesite.Options{Pages: o.pages, Fanout: o.fanout, Latency: o.latency})
	var servers []*http.Server
	for h := 0; h < o.hosts; h++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", o.basePort+h))
		if err != nil {
			return result{}, err
		}
		srv := &http.Server{Handler: site, ReadHeaderTimeout: 10 * time.Second}
		servers = append(servers, srv)
		go srv.Serve(ln)
	}
	defer func() {
		for _, srv := range servers {
			srv.Close()
		}
	}()

	front := frontier.NewPostgres(pool)

	// Start the crawler processes first, idle, so process start-up is not
	// part of the measured interval.
	var running []*crawler
	for i := 0; i < procs; i++ {
		c, err := startCrawler(o, i)
		if err != nil {
			return result{}, err
		}
		defer c.stop()
		running = append(running, c)
	}
	time.Sleep(time.Second)

	// One crawl per host; the work spreads over every process.
	start := time.Now()
	for h := 0; h < o.hosts; h++ {
		seed := fmt.Sprintf("http://127.0.0.1:%d/", o.basePort+h)
		if _, err := frontier.StartCrawl(ctx, front, seed, 12, nil); err != nil {
			return result{}, err
		}
	}

	// expected: every page of every host, plus the /private/secret link per host (robots-blocked).
	if err := waitDone(ctx, pool, 20*time.Minute); err != nil {
		return result{}, err
	}
	elapsed := time.Since(start)

	var done int
	pool.QueryRow(ctx, `SELECT count(*) FROM frontier WHERE status = 'done'`).Scan(&done)

	var allBuckets []bucket
	var res result
	for _, c := range running {
		b, retries, deferred := c.scrape()
		allBuckets = mergeBuckets(allBuckets, b)
		res.retries += retries
		res.deferred += deferred

		peak := c.stopSampling()
		res.rssSumMiB += peak
		res.rssPeakMiB = max(res.rssPeakMiB, peak)
	}

	res.procs = procs
	res.pages = done
	res.elapsed = elapsed
	res.pagesPerS = float64(done) / elapsed.Seconds()
	res.p50 = quantile(allBuckets, 0.50)
	res.p95 = quantile(allBuckets, 0.95)

	if want := o.hosts * o.pages; done != want {
		return res, fmt.Errorf("crawled %d pages, expected %d", done, want)
	}

	return res, nil
}

func waitDone(ctx context.Context, pool *pgxpool.Pool, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	lastLog := time.Now()

	for time.Now().Before(deadline) {
		var active, done int
		err := pool.QueryRow(ctx, `
			SELECT count(*) FILTER (WHERE status IN ('pending', 'leased')),
			       count(*) FILTER (WHERE status = 'done')
			FROM frontier`).Scan(&active, &done)
		if err != nil {
			return err
		}

		if time.Since(lastLog) > 5*time.Second {
			log.Printf("  %d done, %d active", done, active)
			lastLog = time.Now()
		}

		// done > 0 guards against checking before the seeds are visible.
		if active == 0 && done > 0 {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("crawl did not finish within %v", timeout)
}

// crawler is one crawlerx subprocess.
type crawler struct {
	cmd         *exec.Cmd
	metricsAddr string
	exited      chan struct{}
	stopRSS     chan struct{}
	peakKiB     chan int
}

func startCrawler(o options, index int) (*crawler, error) {
	addr := fmt.Sprintf("127.0.0.1:%d", o.metricsPort+index)

	cmd := exec.Command(o.binary,
		"-database-url", o.databaseURL,
		"-redis-url", o.redisURL,
		"-listen", addr,
		"-workers", strconv.Itoa(o.workers),
		"-host-delay", o.hostDelay.String(),
		"-host-inflight", "4",
		"-timeout", "10s", "-lease", "60s",
		"-log-level", "warn",
	)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	c := &crawler{
		cmd: cmd, metricsAddr: addr,
		exited: make(chan struct{}), stopRSS: make(chan struct{}), peakKiB: make(chan int, 1),
	}
	go func() {
		cmd.Wait()
		close(c.exited)
	}()
	go c.sampleRSS()

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if resp, err := http.Get("http://" + addr + "/healthz"); err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return c, nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	c.stop()
	return nil, fmt.Errorf("crawlerx %d did not become healthy", index)
}

func (c *crawler) sampleRSS() {
	peak := 0
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopRSS:
			c.peakKiB <- peak
			return
		case <-ticker.C:
			out, err := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(c.cmd.Process.Pid)).Output()
			if err != nil {
				continue
			}
			if kib, err := strconv.Atoi(strings.TrimSpace(string(out))); err == nil && kib > peak {
				peak = kib
			}
		}
	}
}

// stopSampling ends RSS sampling and returns the peak in MiB.
func (c *crawler) stopSampling() float64 {
	select {
	case <-c.stopRSS:
	default:
		close(c.stopRSS)
	}
	return float64(<-c.peakKiB) / 1024
}

func (c *crawler) stop() {
	select {
	case <-c.exited:
		return
	default:
	}

	_ = c.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-c.exited:
	case <-time.After(20 * time.Second):
		_ = c.cmd.Process.Kill()
		<-c.exited
	}
}

type bucket struct {
	le    float64
	count float64
}

// scrape reads the fetch-latency histogram and two counters from /metrics.
func (c *crawler) scrape() (buckets []bucket, retries, deferred int64) {
	resp, err := http.Get("http://" + c.metricsAddr + "/metrics")
	if err != nil {
		log.Printf("scrape %s: %v", c.metricsAddr, err)
		return nil, 0, 0
	}
	defer resp.Body.Close()

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()

		switch {
		case strings.HasPrefix(line, "crawlerx_fetch_duration_seconds_bucket{le=\""):
			rest := strings.TrimPrefix(line, "crawlerx_fetch_duration_seconds_bucket{le=\"")
			end := strings.Index(rest, "\"")
			if end < 0 {
				continue
			}
			le := rest[:end]
			value := strings.Fields(rest[end:])
			if len(value) < 2 {
				continue
			}
			count, _ := strconv.ParseFloat(value[len(value)-1], 64)
			bound := inf
			if le != "+Inf" {
				bound, _ = strconv.ParseFloat(le, 64)
			}
			buckets = append(buckets, bucket{le: bound, count: count})

		case strings.HasPrefix(line, "crawlerx_retries_total "):
			f, _ := strconv.ParseFloat(strings.Fields(line)[1], 64)
			retries = int64(f)
		case strings.HasPrefix(line, "crawlerx_gate_deferred_total "):
			f, _ := strconv.ParseFloat(strings.Fields(line)[1], 64)
			deferred = int64(f)
		}
	}

	return buckets, retries, deferred
}

var inf = 1e300

func mergeBuckets(a, b []bucket) []bucket {
	sums := map[float64]float64{}
	for _, list := range [][]bucket{a, b} {
		for _, bk := range list {
			sums[bk.le] += bk.count
		}
	}

	out := make([]bucket, 0, len(sums))
	for le, count := range sums {
		out = append(out, bucket{le, count})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].le < out[j].le })

	return out
}

// quantile estimates a quantile from cumulative histogram buckets the way
// Prometheus' histogram_quantile does (linear interpolation in the bucket).
func quantile(buckets []bucket, q float64) time.Duration {
	if len(buckets) == 0 {
		return 0
	}

	total := buckets[len(buckets)-1].count
	if total == 0 {
		return 0
	}

	rank := q * total
	prevLE, prevCount := 0.0, 0.0
	for _, b := range buckets {
		if b.count >= rank {
			if b.le >= inf { // beyond the last finite bucket
				return time.Duration(prevLE * float64(time.Second))
			}
			frac := 0.0
			if b.count > prevCount {
				frac = (rank - prevCount) / (b.count - prevCount)
			}
			return time.Duration((prevLE + (b.le-prevLE)*frac) * float64(time.Second))
		}
		prevLE, prevCount = b.le, b.count
	}

	return 0
}

func render(o options, results []result) string {
	var b strings.Builder

	fmt.Fprintf(&b, "| crawler processes | pages | elapsed | pages/s | speed-up | p50 fetch | p95 fetch | peak RSS (per process) | total RSS | retries | gate deferrals |\n")
	fmt.Fprintf(&b, "|---|---|---|---|---|---|---|---|---|---|---|\n")

	base := 0.0
	for i, r := range results {
		if i == 0 {
			base = r.pagesPerS
		}
		fmt.Fprintf(&b, "| %d | %d | %v | %.0f | %.2fx | %v | %v | %.0f MiB | %.0f MiB | %d | %d |\n",
			r.procs, r.pages, r.elapsed.Round(10*time.Millisecond), r.pagesPerS, r.pagesPerS/base,
			r.p50.Round(time.Millisecond), r.p95.Round(time.Millisecond), r.rssPeakMiB, r.rssSumMiB, r.retries, r.deferred)
	}

	fmt.Fprintf(&b, "\nSettings: %d hosts x %d pages (fanout %d), server latency %v, crawler host-delay %v, %d workers per process, GOMAXPROCS=%d, %s/%s, %s.\n",
		o.hosts, o.pages, o.fanout, o.latency, o.hostDelay, o.workers, runtime.GOMAXPROCS(0), runtime.GOOS, runtime.GOARCH, runtime.Version())

	return b.String()
}
