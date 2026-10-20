// Package politeness enforces a minimum delay between requests to the same
// host. The gate is in-process: run one crawler per database, or two crawlers
// together will exceed the host delay.
package politeness

import (
	"context"
	"sync"
	"time"
)

// Gate decides whether a request to a host may go out now.
type Gate interface {
	// Acquire reserves the next request slot for host. It returns 0 if the
	// caller may fetch right now (the slot is consumed and the host's next
	// slot is pushed `delay` into the future), or the time to wait before
	// asking again. It never sleeps: callers reschedule instead of blocking.
	Acquire(ctx context.Context, host string, delay time.Duration) (wait time.Duration, err error)

	// Penalize pushes the host's next slot at least d into the future, used
	// when a server says 429/503 with Retry-After.
	Penalize(ctx context.Context, host string, d time.Duration) error
}

// Memory is the in-process Gate.
type Memory struct {
	mu   sync.Mutex
	now  func() time.Time
	next map[string]time.Time
}

// NewMemory returns an in-process Gate. now may be nil (time.Now).
func NewMemory(now func() time.Time) *Memory {
	if now == nil {
		now = time.Now
	}
	return &Memory{now: now, next: make(map[string]time.Time)}
}

func (m *Memory) Acquire(_ context.Context, host string, delay time.Duration) (time.Duration, error) {
	if delay <= 0 {
		return 0, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.now()
	if next := m.next[host]; now.Before(next) {
		return next.Sub(now).Round(time.Millisecond), nil
	}

	m.next[host] = now.Add(delay)
	return 0, nil
}

func (m *Memory) Penalize(_ context.Context, host string, d time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if target := m.now().Add(d); target.After(m.next[host]) {
		m.next[host] = target
	}
	return nil
}

var (
	_ Gate = (*Memory)(nil)
)
