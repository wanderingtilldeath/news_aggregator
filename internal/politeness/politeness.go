// Package politeness enforces a minimum delay between requests to the same
// host. The gate lives in Redis so every crawler process shares one clock per
// host: two processes together are exactly as polite as one.
package politeness

import (
	"context"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
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

// acquireScript is atomic: the check and the reservation happen in one Redis
// step, so concurrent workers in different processes cannot both pass.
// Time comes from the Redis server, so clock skew between crawlers is moot.
//
// KEYS[1] = host key, ARGV[1] = delay in ms.
var acquireScript = redis.NewScript(`
local t = redis.call('TIME')
local now = t[1] * 1000 + math.floor(t[2] / 1000)
local nxt = tonumber(redis.call('GET', KEYS[1]) or '0')
if now >= nxt then
    redis.call('SET', KEYS[1], now + ARGV[1], 'PX', math.max(tonumber(ARGV[1]), 1))
    return 0
end
return nxt - now
`)

// penalizeScript sets next-allowed to max(current, now+ARGV[1]).
var penalizeScript = redis.NewScript(`
local t = redis.call('TIME')
local now = t[1] * 1000 + math.floor(t[2] / 1000)
local nxt = tonumber(redis.call('GET', KEYS[1]) or '0')
local target = now + tonumber(ARGV[1])
if target > nxt then
    redis.call('SET', KEYS[1], target, 'PX', math.max(tonumber(ARGV[1]), 1))
end
return 0
`)

// Redis is the production Gate.
type Redis struct {
	client redis.Scripter
	prefix string
}

// NewRedis returns a Gate backed by client. Keys are namespaced under
// "crawlerx:host:".
func NewRedis(client redis.Scripter) *Redis {
	return &Redis{client: client, prefix: "crawlerx:host:"}
}

func (r *Redis) Acquire(ctx context.Context, host string, delay time.Duration) (time.Duration, error) {
	if delay <= 0 {
		return 0, nil
	}

	waitMs, err := acquireScript.Run(ctx, r.client, []string{r.prefix + host}, delay.Milliseconds()).Int64()
	if err != nil {
		return 0, err
	}

	return time.Duration(waitMs) * time.Millisecond, nil
}

func (r *Redis) Penalize(ctx context.Context, host string, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	return penalizeScript.Run(ctx, r.client, []string{r.prefix + host}, d.Milliseconds()).Err()
}

// Memory is an in-process Gate with the same semantics, for tests and for a
// single-process crawl without Redis.
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
	_ Gate = (*Redis)(nil)
	_ Gate = (*Memory)(nil)
)
