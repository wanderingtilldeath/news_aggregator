package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresClaims claims a run by inserting into job_runs; the primary key
// lets exactly one process win.
type PostgresClaims struct{ Pool *pgxpool.Pool }

func (c PostgresClaims) Claim(ctx context.Context, job string, day time.Time) (bool, error) {
	tag, err := c.Pool.Exec(ctx,
		`INSERT INTO job_runs (job, run_day) VALUES ($1, $2::date) ON CONFLICT DO NOTHING`,
		job, day.Format("2006-01-02"))
	if err != nil {
		return false, fmt.Errorf("scheduler: claim %s: %w", job, err)
	}

	return tag.RowsAffected() == 1, nil
}

// MemoryClaims is the in-process equivalent, for tests.
type MemoryClaims struct {
	mu   sync.Mutex
	seen map[string]bool
}

func (c *MemoryClaims) Claim(_ context.Context, job string, day time.Time) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.seen == nil {
		c.seen = make(map[string]bool)
	}
	key := job + day.Format("2006-01-02")
	if c.seen[key] {
		return false, nil
	}
	c.seen[key] = true

	return true, nil
}
