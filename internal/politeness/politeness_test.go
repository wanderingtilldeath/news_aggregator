package politeness

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newMiniredisGate(t *testing.T) (*Redis, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { client.Close() })

	return NewRedis(client), mr
}

func TestRedisGate_AllowsThenDelays(t *testing.T) {
	gate, mr := newMiniredisGate(t)
	ctx := context.Background()
	now := time.Now()
	mr.SetTime(now)

	wait, err := gate.Acquire(ctx, "a.test", time.Second)
	if err != nil || wait != 0 {
		t.Fatalf("first Acquire = %v, %v; want 0, nil", wait, err)
	}

	mr.SetTime(now.Add(400 * time.Millisecond))
	wait, err = gate.Acquire(ctx, "a.test", time.Second)
	if err != nil || wait != 600*time.Millisecond {
		t.Fatalf("second Acquire = %v, %v; want 600ms, nil", wait, err)
	}

	mr.SetTime(now.Add(1001 * time.Millisecond))
	mr.FastForward(1001 * time.Millisecond) // expire the key like real time would
	wait, err = gate.Acquire(ctx, "a.test", time.Second)
	if err != nil || wait != 0 {
		t.Fatalf("Acquire after delay = %v, %v; want 0, nil", wait, err)
	}
}

func TestRedisGate_HostsAreIndependent(t *testing.T) {
	gate, mr := newMiniredisGate(t)
	ctx := context.Background()
	mr.SetTime(time.Now())

	for _, host := range []string{"a.test", "b.test", "c.test:8080"} {
		if wait, err := gate.Acquire(ctx, host, time.Minute); err != nil || wait != 0 {
			t.Fatalf("Acquire(%s) = %v, %v", host, wait, err)
		}
	}
}

func TestRedisGate_OnlyOneConcurrentCallerWins(t *testing.T) {
	gate, mr := newMiniredisGate(t)
	mr.SetTime(time.Now())

	var winners atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if wait, err := gate.Acquire(context.Background(), "a.test", time.Minute); err == nil && wait == 0 {
				winners.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := winners.Load(); got != 1 {
		t.Fatalf("%d callers passed the gate, want exactly 1", got)
	}
}

func TestRedisGate_Penalize(t *testing.T) {
	gate, mr := newMiniredisGate(t)
	ctx := context.Background()
	now := time.Now()
	mr.SetTime(now)

	if err := gate.Penalize(ctx, "a.test", 30*time.Second); err != nil {
		t.Fatal(err)
	}

	wait, err := gate.Acquire(ctx, "a.test", time.Second)
	if err != nil || wait != 30*time.Second {
		t.Fatalf("Acquire after Penalize = %v, %v; want 30s", wait, err)
	}

	// A shorter penalty must not shorten an existing, longer one.
	if err := gate.Penalize(ctx, "a.test", time.Second); err != nil {
		t.Fatal(err)
	}
	if wait, _ := gate.Acquire(ctx, "a.test", time.Second); wait != 30*time.Second {
		t.Fatalf("wait = %v after shorter penalty, want 30s", wait)
	}
}

func TestRedisGate_ZeroDelayNeverWaits(t *testing.T) {
	gate, _ := newMiniredisGate(t)

	for i := 0; i < 3; i++ {
		if wait, err := gate.Acquire(context.Background(), "a.test", 0); err != nil || wait != 0 {
			t.Fatalf("Acquire = %v, %v", wait, err)
		}
	}
}

// Real-Redis check, enabled with TEST_REDIS_URL, so the Lua script is also
// exercised against the real server and not only miniredis.
func TestRedisGate_RealServerSharedAcrossClients(t *testing.T) {
	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		t.Skip("TEST_REDIS_URL not set")
	}

	opts, err := redis.ParseURL(url)
	if err != nil {
		t.Fatal(err)
	}

	a, b := redis.NewClient(opts), redis.NewClient(opts) // two "processes"
	defer a.Close()
	defer b.Close()

	host := "shared-" + time.Now().Format("150405.000000") + ".test"
	ctx := context.Background()

	if wait, err := NewRedis(a).Acquire(ctx, host, 500*time.Millisecond); err != nil || wait != 0 {
		t.Fatalf("client A Acquire = %v, %v", wait, err)
	}

	wait, err := NewRedis(b).Acquire(ctx, host, 500*time.Millisecond)
	if err != nil || wait <= 0 || wait > 500*time.Millisecond {
		t.Fatalf("client B Acquire = %v, %v; want a wait in (0, 500ms]", wait, err)
	}
}

func TestMemoryGate(t *testing.T) {
	clock := time.Now()
	gate := NewMemory(func() time.Time { return clock })
	ctx := context.Background()

	if wait, _ := gate.Acquire(ctx, "h", time.Second); wait != 0 {
		t.Fatalf("first wait = %v", wait)
	}

	clock = clock.Add(250 * time.Millisecond)
	if wait, _ := gate.Acquire(ctx, "h", time.Second); wait != 750*time.Millisecond {
		t.Fatalf("second wait = %v, want 750ms", wait)
	}

	clock = clock.Add(750 * time.Millisecond)
	if wait, _ := gate.Acquire(ctx, "h", time.Second); wait != 0 {
		t.Fatalf("third wait = %v, want 0", wait)
	}
}
