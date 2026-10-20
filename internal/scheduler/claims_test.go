package scheduler_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/scheduler"
	"github.com/wanderingtilldeath/news-aggregator/internal/testutil"
)

func TestPostgresClaimsOneWinnerPerDay(t *testing.T) {
	c := scheduler.PostgresClaims{Pool: testutil.Postgres(t)}
	ctx := context.Background()
	day := time.Date(2026, 10, 19, 6, 0, 0, 0, time.UTC)

	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, err := c.Claim(ctx, "digest", day); err != nil {
				t.Error(err)
			} else if ok {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()

	if wins.Load() != 1 {
		t.Fatalf("%d concurrent claims won, want exactly 1", wins.Load())
	}
	if ok, _ := c.Claim(ctx, "digest", day.AddDate(0, 0, 1)); !ok {
		t.Error("the next day should be claimable")
	}
	if ok, _ := c.Claim(ctx, "other-job", day); !ok {
		t.Error("a different job on the same day should be claimable")
	}
}
