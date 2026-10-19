package frontier_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/frontier"
	"github.com/wanderingtilldeath/news-aggregator/internal/testutil"
)

// backend is one Frontier implementation plus a way to advance its clock.
type backend struct {
	name string
	make func(t *testing.T) (f frontier.Frontier, advance func(time.Duration))
}

func backends() []backend {
	return []backend{
		{"memory", func(t *testing.T) (frontier.Frontier, func(time.Duration)) {
			now := time.Now()
			m := frontier.NewMemory(func() time.Time { return now })
			return m, func(d time.Duration) { now = now.Add(d) }
		}},
		{"postgres", func(t *testing.T) (frontier.Frontier, func(time.Duration)) {
			// Postgres uses the database clock, so tests use real, short sleeps.
			return frontier.NewPostgres(testutil.Postgres(t)), func(d time.Duration) { time.Sleep(d) }
		}},
	}
}

// forEachBackend runs the same behavioural test against every implementation,
// so the in-memory fake cannot drift from the real thing.
func forEachBackend(t *testing.T, fn func(t *testing.T, f frontier.Frontier, advance func(time.Duration))) {
	for _, b := range backends() {
		t.Run(b.name, func(t *testing.T) {
			f, advance := b.make(t)
			fn(t, f, advance)
		})
	}
}

func newCrawl(t *testing.T, f frontier.Frontier) frontier.Crawl {
	t.Helper()

	crawl, err := f.CreateCrawl(context.Background(), "http://a.test/", 3, nil)
	if err != nil {
		t.Fatal(err)
	}
	return crawl
}

func TestCreateCrawlDefaultsToSeedHost(t *testing.T) {
	forEachBackend(t, func(t *testing.T, f frontier.Frontier, _ func(time.Duration)) {
		ctx := context.Background()

		crawl, err := f.CreateCrawl(ctx, "http://A.test:8080/x", 2, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(crawl.AllowedHosts) != 1 || crawl.AllowedHosts[0] != "a.test:8080" {
			t.Fatalf("AllowedHosts = %v, want [a.test:8080]", crawl.AllowedHosts)
		}

		got, err := f.GetCrawl(ctx, crawl.ID)
		if err != nil || got.Seed != "http://A.test:8080/x" || got.MaxDepth != 2 || !got.AllowsHost("A.test:8080") {
			t.Fatalf("GetCrawl = %+v, %v", got, err)
		}

		if _, err := f.GetCrawl(ctx, 999999); err != frontier.ErrNotFound {
			t.Fatalf("GetCrawl(missing) err = %v, want ErrNotFound", err)
		}
	})
}

func TestEnqueueDeduplicates(t *testing.T) {
	forEachBackend(t, func(t *testing.T, f frontier.Frontier, _ func(time.Duration)) {
		ctx := context.Background()
		crawl := newCrawl(t, f)

		n, err := f.Enqueue(ctx, crawl.ID, []frontier.NewItem{
			{URL: "http://a.test/1", Depth: 1}, {URL: "http://a.test/2", Depth: 1}, {URL: "http://a.test/1", Depth: 2},
		})
		if err != nil || n != 2 {
			t.Fatalf("Enqueue = %d, %v; want 2 new", n, err)
		}

		n, err = f.Enqueue(ctx, crawl.ID, []frontier.NewItem{{URL: "http://a.test/2"}, {URL: "http://a.test/3"}})
		if err != nil || n != 1 {
			t.Fatalf("second Enqueue = %d, %v; want 1 new", n, err)
		}

		// The same URL in a different crawl is independent.
		other := newCrawl(t, f)
		if n, _ := f.Enqueue(ctx, other.ID, []frontier.NewItem{{URL: "http://a.test/1"}}); n != 1 {
			t.Fatalf("same URL in another crawl: %d new, want 1", n)
		}

		stats, _ := f.Stats(ctx, crawl.ID)
		if stats[frontier.Pending] != 3 || stats.Total() != 3 {
			t.Fatalf("stats = %v", stats)
		}
	})
}

func TestClaimLeasesAndFinishes(t *testing.T) {
	forEachBackend(t, func(t *testing.T, f frontier.Frontier, _ func(time.Duration)) {
		ctx := context.Background()
		crawl := newCrawl(t, f)
		f.Enqueue(ctx, crawl.ID, []frontier.NewItem{{URL: "http://a.test/1", Depth: 1}, {URL: "http://a.test/2", Depth: 2}})

		items, err := f.Claim(ctx, 5, time.Minute)
		if err != nil || len(items) != 2 {
			t.Fatalf("Claim = %v, %v; want 2 items", items, err)
		}
		for _, it := range items {
			if it.Host != "a.test" || it.URLHash != frontier.Hash(it.URL) || it.CrawlID != crawl.ID || it.Attempts != 0 {
				t.Fatalf("unexpected item %+v", it)
			}
		}

		// Leased rows are not claimable by anyone else.
		if again, _ := f.Claim(ctx, 5, time.Minute); len(again) != 0 {
			t.Fatalf("second Claim returned %v while leased", again)
		}

		if err := f.Finish(ctx, items[0], frontier.Done, ""); err != nil {
			t.Fatal(err)
		}
		if err := f.Finish(ctx, items[1], frontier.Failed, "HTTP 404"); err != nil {
			t.Fatal(err)
		}

		stats, _ := f.Stats(ctx, crawl.ID)
		if stats[frontier.Done] != 1 || stats[frontier.Failed] != 1 || stats.Active() != 0 {
			t.Fatalf("stats = %v", stats)
		}

		if after, _ := f.Claim(ctx, 5, time.Minute); len(after) != 0 {
			t.Fatalf("finished rows were claimed again: %v", after)
		}
	})
}

func TestClaimRespectsLimit(t *testing.T) {
	forEachBackend(t, func(t *testing.T, f frontier.Frontier, _ func(time.Duration)) {
		ctx := context.Background()
		crawl := newCrawl(t, f)

		var items []frontier.NewItem
		for i := 0; i < 10; i++ {
			items = append(items, frontier.NewItem{URL: fmt.Sprintf("http://a.test/%d", i)})
		}
		f.Enqueue(ctx, crawl.ID, items)

		got, _ := f.Claim(ctx, 3, time.Minute)
		if len(got) != 3 {
			t.Fatalf("claimed %d, want 3", len(got))
		}
		if depth, _ := f.QueueDepth(ctx); depth != 7 {
			t.Fatalf("QueueDepth = %d, want 7", depth)
		}
	})
}

func TestExpiredLeaseIsReclaimedAndCountsAnAttempt(t *testing.T) {
	forEachBackend(t, func(t *testing.T, f frontier.Frontier, advance func(time.Duration)) {
		ctx := context.Background()
		crawl := newCrawl(t, f)
		f.Enqueue(ctx, crawl.ID, []frontier.NewItem{{URL: "http://a.test/1"}})

		first, _ := f.Claim(ctx, 1, 100*time.Millisecond)
		if len(first) != 1 || first[0].Attempts != 0 {
			t.Fatalf("first claim = %+v", first)
		}

		// The holder "crashes": it never finishes. Before expiry nobody else gets it.
		if early, _ := f.Claim(ctx, 1, time.Minute); len(early) != 0 {
			t.Fatal("row claimed before its lease expired")
		}

		advance(250 * time.Millisecond)

		second, err := f.Claim(ctx, 1, time.Minute)
		if err != nil || len(second) != 1 {
			t.Fatalf("reclaim = %v, %v", second, err)
		}
		if second[0].Attempts != 1 {
			t.Fatalf("Attempts = %d after reclaiming an expired lease, want 1", second[0].Attempts)
		}
	})
}

func TestFinishByAStaleHolderIsIgnored(t *testing.T) {
	forEachBackend(t, func(t *testing.T, f frontier.Frontier, advance func(time.Duration)) {
		ctx := context.Background()
		crawl := newCrawl(t, f)
		f.Enqueue(ctx, crawl.ID, []frontier.NewItem{{URL: "http://a.test/1"}})

		stale, _ := f.Claim(ctx, 1, 50*time.Millisecond)
		advance(150 * time.Millisecond)
		fresh, _ := f.Claim(ctx, 1, time.Minute)
		if len(fresh) != 1 {
			t.Fatal("reclaim failed")
		}

		// The new holder finishes; the stale one's late Finish must not
		// overwrite it or resurrect the row.
		f.Finish(ctx, fresh[0], frontier.Done, "")
		f.Finish(ctx, stale[0], frontier.Failed, "late")

		if stats, _ := f.Stats(ctx, crawl.ID); stats[frontier.Done] != 1 || stats[frontier.Failed] != 0 {
			t.Fatalf("stats = %v", stats)
		}
	})
}

func TestRescheduleDefersAndOptionallyCountsAttempt(t *testing.T) {
	forEachBackend(t, func(t *testing.T, f frontier.Frontier, advance func(time.Duration)) {
		ctx := context.Background()
		crawl := newCrawl(t, f)
		f.Enqueue(ctx, crawl.ID, []frontier.NewItem{{URL: "http://a.test/1"}})

		it, _ := f.Claim(ctx, 1, time.Minute)

		// Deferred without counting (host busy): not due for a while.
		if err := f.Reschedule(ctx, it[0], time.Now().Add(300*time.Millisecond), false, ""); err != nil {
			t.Fatal(err)
		}
		if early, _ := f.Claim(ctx, 1, time.Minute); len(early) != 0 {
			t.Fatal("row claimed before next_fetch_at")
		}

		advance(500 * time.Millisecond)

		it, _ = f.Claim(ctx, 1, time.Minute)
		if len(it) != 1 || it[0].Attempts != 0 {
			t.Fatalf("after non-counting reschedule: %+v", it)
		}

		// Counted (transient failure): attempts goes up.
		f.Reschedule(ctx, it[0], time.Now().Add(-time.Second), true, "HTTP 503")
		it, _ = f.Claim(ctx, 1, time.Minute)
		if len(it) != 1 || it[0].Attempts != 1 {
			t.Fatalf("after counting reschedule: %+v", it)
		}
	})
}

func TestReleaseReturnsRowsToPendingImmediately(t *testing.T) {
	forEachBackend(t, func(t *testing.T, f frontier.Frontier, _ func(time.Duration)) {
		ctx := context.Background()
		crawl := newCrawl(t, f)
		f.Enqueue(ctx, crawl.ID, []frontier.NewItem{{URL: "http://a.test/1"}, {URL: "http://a.test/2"}})

		items, _ := f.Claim(ctx, 2, time.Hour)
		if err := f.Release(ctx, items); err != nil {
			t.Fatal(err)
		}

		again, _ := f.Claim(ctx, 2, time.Hour)
		if len(again) != 2 {
			t.Fatalf("claimed %d after Release, want 2", len(again))
		}
		for _, it := range again {
			if it.Attempts != 0 {
				t.Fatalf("a graceful Release must not count as an attempt: %+v", it)
			}
		}
	})
}

func TestConcurrentClaimsNeverOverlap(t *testing.T) {
	forEachBackend(t, func(t *testing.T, f frontier.Frontier, _ func(time.Duration)) {
		ctx := context.Background()
		crawl := newCrawl(t, f)

		const total = 300
		var items []frontier.NewItem
		for i := 0; i < total; i++ {
			items = append(items, frontier.NewItem{URL: fmt.Sprintf("http://a.test/%d", i)})
		}
		f.Enqueue(ctx, crawl.ID, items)

		var (
			mu   sync.Mutex
			seen = map[string]int{}
			wg   sync.WaitGroup
		)
		for w := 0; w < 12; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					got, err := f.Claim(ctx, 5, time.Minute)
					if err != nil {
						t.Error(err)
						return
					}
					if len(got) == 0 {
						return
					}
					mu.Lock()
					for _, it := range got {
						seen[it.URLHash]++
					}
					mu.Unlock()
				}
			}()
		}
		wg.Wait()

		if len(seen) != total {
			t.Fatalf("claimed %d distinct rows, want %d", len(seen), total)
		}
		for hash, n := range seen {
			if n != 1 {
				t.Fatalf("row %s claimed %d times", hash, n)
			}
		}
	})
}

func TestClaimIsShallowestFirst(t *testing.T) {
	forEachBackend(t, func(t *testing.T, f frontier.Frontier, _ func(time.Duration)) {
		ctx := context.Background()
		crawl := newCrawl(t, f)

		// Enqueued deepest-first: a FIFO queue would hand them out in this order.
		for _, depth := range []int{5, 3, 1, 4, 2} {
			f.Enqueue(ctx, crawl.ID, []frontier.NewItem{{URL: fmt.Sprintf("http://a.test/d%d", depth), Depth: depth}})
			time.Sleep(2 * time.Millisecond)
		}

		var got []int
		for {
			items, _ := f.Claim(ctx, 1, time.Minute)
			if len(items) == 0 {
				break
			}
			got = append(got, items[0].Depth)
		}

		want := []int{1, 2, 3, 4, 5}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("claim order by depth = %v, want %v", got, want)
		}
	})
}

// A page can first be discovered through a long chain of links and only later
// through a short one. If the queue kept the first (deep) depth, the page
// could sit at the depth limit and never have its own links followed, even
// though it is within the limit by the short path.
func TestRediscoveryAtShallowerDepthLowersPendingDepth(t *testing.T) {
	forEachBackend(t, func(t *testing.T, f frontier.Frontier, _ func(time.Duration)) {
		ctx := context.Background()
		crawl := newCrawl(t, f)

		if n, _ := f.Enqueue(ctx, crawl.ID, []frontier.NewItem{{URL: "http://a.test/x", Depth: 9}}); n != 1 {
			t.Fatalf("first enqueue = %d, want 1", n)
		}

		// Same URL, shorter path: not a new row, but its depth drops to 2.
		if n, _ := f.Enqueue(ctx, crawl.ID, []frontier.NewItem{{URL: "http://a.test/x", Depth: 2}}); n != 0 {
			t.Fatalf("rediscovery counted as new: %d", n)
		}

		// A longer path never raises it again.
		f.Enqueue(ctx, crawl.ID, []frontier.NewItem{{URL: "http://a.test/x", Depth: 7}})

		items, _ := f.Claim(ctx, 1, time.Minute)
		if len(items) != 1 || items[0].Depth != 2 {
			t.Fatalf("claimed %+v, want depth 2", items)
		}
	})
}

func TestEnqueueDuplicatesInOneBatchKeepShallowest(t *testing.T) {
	forEachBackend(t, func(t *testing.T, f frontier.Frontier, _ func(time.Duration)) {
		ctx := context.Background()
		crawl := newCrawl(t, f)

		n, err := f.Enqueue(ctx, crawl.ID, []frontier.NewItem{
			{URL: "http://a.test/x", Depth: 6}, {URL: "http://a.test/x", Depth: 3}, {URL: "http://a.test/x", Depth: 8},
		})
		if err != nil || n != 1 {
			t.Fatalf("Enqueue = %d, %v; want 1 new row", n, err)
		}

		items, _ := f.Claim(ctx, 1, time.Minute)
		if len(items) != 1 || items[0].Depth != 3 {
			t.Fatalf("claimed %+v, want depth 3", items)
		}
	})
}

func TestListCrawlsNewestFirst(t *testing.T) {
	forEachBackend(t, func(t *testing.T, f frontier.Frontier, _ func(time.Duration)) {
		ctx := context.Background()

		var ids []int64
		for i := 0; i < 3; i++ {
			ids = append(ids, newCrawl(t, f).ID)
		}

		got, err := f.ListCrawls(ctx, 2)
		if err != nil || len(got) != 2 || got[0].ID != ids[2] || got[1].ID != ids[1] {
			t.Fatalf("ListCrawls(2) = %+v, %v; want the two newest, newest first", got, err)
		}
	})
}

func TestDepthStatsGroupsByDepthAndStatus(t *testing.T) {
	forEachBackend(t, func(t *testing.T, f frontier.Frontier, _ func(time.Duration)) {
		ctx := context.Background()
		crawl := newCrawl(t, f)

		f.Enqueue(ctx, crawl.ID, []frontier.NewItem{
			{URL: "http://a.test/", Depth: 0},
			{URL: "http://a.test/1", Depth: 1}, {URL: "http://a.test/2", Depth: 1}, {URL: "http://a.test/3", Depth: 1},
		})

		items, _ := f.Claim(ctx, 2, time.Minute) // depth 0 and one depth-1 row
		f.Finish(ctx, items[0], frontier.Done, "")
		f.Finish(ctx, items[1], frontier.Failed, "HTTP 404")

		got, err := f.DepthStats(ctx, crawl.ID)
		if err != nil || len(got) != 2 {
			t.Fatalf("DepthStats = %+v, %v", got, err)
		}
		if got[0].Depth != 0 || got[0].Stats["done"] != 1 {
			t.Errorf("depth 0 = %+v", got[0])
		}
		if got[1].Depth != 1 || got[1].Stats["failed"] != 1 || got[1].Stats["pending"] != 2 {
			t.Errorf("depth 1 = %+v", got[1])
		}
	})
}
