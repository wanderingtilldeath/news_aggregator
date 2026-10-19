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

// TestPostgresTwoProcessesShareOneQueue models two crawler processes (two
// separate connection pools) draining one frontier: SKIP LOCKED must give
// each row to exactly one of them.
func TestPostgresTwoProcessesShareOneQueue(t *testing.T) {
	pool := testutil.Postgres(t)
	ctx := context.Background()

	a, b := frontier.NewPostgres(pool), frontier.NewPostgres(pool)

	crawl, err := a.CreateCrawl(ctx, "http://a.test/", 3, nil)
	if err != nil {
		t.Fatal(err)
	}

	const total = 500
	var items []frontier.NewItem
	for i := 0; i < total; i++ {
		items = append(items, frontier.NewItem{URL: fmt.Sprintf("http://a.test/%d", i)})
	}
	if n, err := a.Enqueue(ctx, crawl.ID, items); err != nil || n != total {
		t.Fatalf("Enqueue = %d, %v", n, err)
	}

	var (
		mu     sync.Mutex
		owners = map[string]string{}
		wg     sync.WaitGroup
	)
	for name, f := range map[string]frontier.Frontier{"A": a, "B": b} {
		for w := 0; w < 4; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					got, err := f.Claim(ctx, 7, time.Minute)
					if err != nil || len(got) == 0 {
						return
					}
					mu.Lock()
					for _, it := range got {
						if prev, dup := owners[it.URLHash]; dup {
							t.Errorf("row %s claimed by %s and %s", it.URL, prev, name)
						}
						owners[it.URLHash] = name
					}
					mu.Unlock()
					for _, it := range got {
						f.Finish(ctx, it, frontier.Done, "")
					}
				}
			}()
		}
	}
	wg.Wait()

	if len(owners) != total {
		t.Fatalf("%d rows claimed, want %d", len(owners), total)
	}
	if stats, _ := a.Stats(ctx, crawl.ID); stats[frontier.Done] != total {
		t.Fatalf("stats = %v", stats)
	}
}

func TestMigrateIsIdempotentAndConcurrencySafe(t *testing.T) {
	pool := testutil.Postgres(t) // already migrated once

	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- migrateAgain(pool)
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("re-running migrations failed: %v", err)
		}
	}
}
