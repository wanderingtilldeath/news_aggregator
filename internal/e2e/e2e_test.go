// Package e2e runs the real crawlerx binary against the fake site, Postgres
// and Redis: kill -9 and resume, shutdown, and two processes sharing one
// politeness gate. It needs TEST_DATABASE_URL and TEST_REDIS_URL, and is
// skipped under -short.
package e2e

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wanderingtilldeath/news-aggregator/internal/fakesite"
	"github.com/wanderingtilldeath/news-aggregator/internal/testutil"
)

var binary string

func TestMain(m *testing.M) {
	flag.Parse() // testing.Short needs flags parsed

	code := func() int {
		if testing.Short() || os.Getenv("TEST_DATABASE_URL") == "" || os.Getenv("TEST_REDIS_URL") == "" {
			return m.Run() // every test skips itself
		}

		dir, err := os.MkdirTemp("", "crawlerx-e2e")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		defer os.RemoveAll(dir)

		binary = filepath.Join(dir, "crawlerx")
		build := exec.Command("go", "build", "-race", "-o", binary, "../../cmd/crawlerx")
		if out, err := build.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "build crawlerx: %v\n%s", err, out)
			return 1
		}

		return m.Run()
	}()

	os.Exit(code)
}

func requireEnv(t *testing.T) {
	t.Helper()

	if testing.Short() {
		t.Skip("e2e skipped under -short")
	}
	if os.Getenv("TEST_DATABASE_URL") == "" || os.Getenv("TEST_REDIS_URL") == "" {
		t.Skip("TEST_DATABASE_URL / TEST_REDIS_URL not set")
	}
}

// proc is a running crawlerx process.
type proc struct {
	cmd    *exec.Cmd
	out    *bytes.Buffer
	exited chan struct{} // closed when the process has exited
	err    error         // valid once exited is closed
}

func startCrawlerx(t *testing.T, dbURL string, args ...string) *proc {
	t.Helper()

	base := []string{
		"-database-url", dbURL,
		"-redis-url", os.Getenv("TEST_REDIS_URL"),
		"-listen", "",
		"-workers", "8",
		"-lease", "3s", "-timeout", "2s", "-shutdown-grace", "5s",
		"-log-level", "info",
	}

	p := &proc{out: &bytes.Buffer{}, exited: make(chan struct{})}
	p.cmd = exec.Command(binary, append(base, args...)...)
	p.cmd.Stdout, p.cmd.Stderr = p.out, p.out

	if err := p.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() {
		p.err = p.cmd.Wait()
		close(p.exited)
	}()

	t.Cleanup(func() {
		select {
		case <-p.exited:
		default:
			_ = p.cmd.Process.Kill()
			<-p.exited
		}
	})

	return p
}

// wait blocks until the process exits, returning its exit error.
func (p *proc) wait(t *testing.T, timeout time.Duration) error {
	t.Helper()

	select {
	case <-p.exited:
		return p.err
	case <-time.After(timeout):
		t.Fatalf("process did not exit within %v\n%s", timeout, p.out.String())
		return nil
	}
}

func (p *proc) kill9() {
	_ = p.cmd.Process.Signal(syscall.SIGKILL)
	<-p.exited
}

// statusCounts reads the frontier row counts per status.
func statusCounts(t *testing.T, pool *pgxpool.Pool) map[string]int {
	t.Helper()

	rows, err := pool.Query(context.Background(), `SELECT status, count(*) FROM frontier GROUP BY status`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	out := map[string]int{}
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			t.Fatal(err)
		}
		out[status] = n
	}
	return out
}

func waitFor(t *testing.T, timeout time.Duration, what string, cond func() bool, diagnose ...func() string) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}

	extra := ""
	for _, d := range diagnose {
		extra += "\n" + d()
	}
	t.Fatalf("timed out waiting for %s%s", what, extra)
}

func tail(s string, n int) string {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}

func TestKillNineAndResumeDoesNotRecrawl(t *testing.T) {
	requireEnv(t)
	pool, dbURL := testutil.PostgresWithURL(t)
	testutil.Redis(t)

	const pages = 300

	site := fakesite.New(fakesite.Options{Pages: pages, Fanout: 4, Latency: 30 * time.Millisecond})
	ts := httptest.NewServer(site)
	defer ts.Close()

	// Process A starts the crawl.
	a := startCrawlerx(t, dbURL, "-seed", ts.URL+"/", "-max-depth", "10", "-host-delay", "5ms", "-max-attempts", "5")

	waitFor(t, 30*time.Second, "~60 pages done", func() bool { return statusCounts(t, pool)["done"] >= 60 })

	a.kill9() // no signal handlers run: leases are left behind

	// Whatever was in flight at the moment of death is still leased.
	leasedAtDeath := map[string]bool{}
	rows, err := pool.Query(context.Background(), `SELECT url FROM frontier WHERE status = 'leased'`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var u string
		rows.Scan(&u)
		leasedAtDeath[u] = true
	}
	rows.Close()

	doneBefore := statusCounts(t, pool)["done"]
	t.Logf("killed with %d done, %d leased", doneBefore, len(leasedAtDeath))
	if doneBefore >= pages {
		t.Fatal("crawl finished before the kill; make the site slower")
	}

	// Process B restarts WITHOUT a seed: it just works the shared queue.
	b := startCrawlerx(t, dbURL, "-host-delay", "5ms", "-max-attempts", "5")

	waitFor(t, 60*time.Second, "crawl to complete", func() bool {
		c := statusCounts(t, pool)
		return c["done"] == pages && c["pending"] == 0 && c["leased"] == 0
	}, func() string {
		return fmt.Sprintf("counts=%v\nB output:\n%s", statusCounts(t, pool), tail(b.out.String(), 3000))
	})

	_ = b.cmd.Process.Signal(syscall.SIGTERM)
	if err := b.wait(t, 15*time.Second); err != nil {
		t.Fatalf("B exit: %v\n%s", err, b.out.String())
	}

	// Every page crawled, and the page rows agree.
	var stored int
	pool.QueryRow(context.Background(), `SELECT count(*) FROM pages`).Scan(&stored)
	if stored != pages {
		t.Errorf("%d pages stored, want %d", stored, pages)
	}

	// The headline claim: nothing finished before the crash was fetched again.
	// Only URLs that were in flight at the moment of death may appear twice.
	var refetched []string
	for key, n := range site.PageRequests() {
		path := key[strings.Index(key, "/"):]
		url := ts.URL + path
		if n > 1 {
			if !leasedAtDeath[url] {
				refetched = append(refetched, fmt.Sprintf("%s x%d", path, n))
			}
		}
		if n > 2 {
			t.Errorf("%s fetched %d times", path, n)
		}
	}
	sort.Strings(refetched)
	if len(refetched) > 0 {
		t.Errorf("URLs completed before the crash were crawled again: %v", refetched)
	}

	if len(site.PageRequests()) != pages {
		t.Errorf("fakesite served %d distinct pages, want %d", len(site.PageRequests()), pages)
	}
}

func TestSIGTERMReleasesLeasesAndExitsCleanly(t *testing.T) {
	requireEnv(t)
	pool, dbURL := testutil.PostgresWithURL(t)
	testutil.Redis(t)

	site := fakesite.New(fakesite.Options{Pages: 300, Fanout: 4, Latency: 150 * time.Millisecond})
	ts := httptest.NewServer(site)
	defer ts.Close()

	p := startCrawlerx(t, dbURL, "-seed", ts.URL+"/", "-max-depth", "10", "-host-delay", "5ms")

	waitFor(t, 30*time.Second, "crawl under way", func() bool {
		c := statusCounts(t, pool)
		return c["done"] >= 10 && c["leased"] > 0
	})

	start := time.Now()
	_ = p.cmd.Process.Signal(syscall.SIGTERM)
	if err := p.wait(t, 20*time.Second); err != nil {
		t.Fatalf("exit error after SIGTERM: %v\n%s", err, p.out.String())
	}
	t.Logf("shut down in %v", time.Since(start))

	if c := statusCounts(t, pool); c["leased"] != 0 {
		t.Fatalf("%d rows still leased after a graceful shutdown: %v", c["leased"], c)
	}
}

func TestTwoProcessesShareOneHostGate(t *testing.T) {
	requireEnv(t)
	pool, dbURL := testutil.PostgresWithURL(t)
	testutil.Redis(t)

	site := fakesite.New(fakesite.Options{Pages: 25, Fanout: 3})
	ts := httptest.NewServer(site)
	defer ts.Close()

	const delay = 250 * time.Millisecond

	args := []string{"-host-delay", delay.String(), "-host-inflight", "4"}
	a := startCrawlerx(t, dbURL, append([]string{"-seed", ts.URL + "/", "-max-depth", "10"}, args...)...)
	b := startCrawlerx(t, dbURL, args...)

	waitFor(t, 60*time.Second, "crawl to complete", func() bool {
		c := statusCounts(t, pool)
		return c["done"] == 25 && c["pending"] == 0 && c["leased"] == 0 // "/" is page 0, so 25 URLs
	}, func() string {
		return fmt.Sprintf("counts=%v\nA output:\n%s", statusCounts(t, pool), tail(a.out.String(), 2000))
	})
	_ = a.cmd.Process.Signal(syscall.SIGTERM)
	_ = b.cmd.Process.Signal(syscall.SIGTERM)
	a.wait(t, 15*time.Second)
	b.wait(t, 15*time.Second)

	// Both processes fetched from this one host; its requests must be spaced
	// by the delay no matter which process sent them.
	var times []time.Time
	for _, r := range site.Requests() {
		if r.Path == "/" || strings.HasPrefix(r.Path, "/p/") {
			times = append(times, r.Time)
		}
	}
	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })

	if len(times) < 25 {
		t.Fatalf("only %d page requests", len(times))
	}

	// Allow some slack for arrival reordering at the server.
	minGap := delay - 60*time.Millisecond
	for i := 1; i < len(times); i++ {
		if gap := times[i].Sub(times[i-1]); gap < minGap {
			t.Errorf("requests %d and %d arrived %v apart; two processes together must respect the %v host delay", i-1, i, gap, delay)
		}
	}
}
