package politeness

import (
	"context"
	"testing"
	"time"
)

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

func TestMemoryGate_HostsAreIndependent(t *testing.T) {
	gate := NewMemory(nil)
	ctx := context.Background()

	if wait, _ := gate.Acquire(ctx, "a", time.Hour); wait != 0 {
		t.Fatalf("a: wait = %v", wait)
	}
	if wait, _ := gate.Acquire(ctx, "b", time.Hour); wait != 0 {
		t.Fatalf("b must not be held back by a: wait = %v", wait)
	}
}

func TestMemoryGate_PenalizeOnlyExtends(t *testing.T) {
	clock := time.Now()
	gate := NewMemory(func() time.Time { return clock })
	ctx := context.Background()

	gate.Acquire(ctx, "h", time.Second)
	_ = gate.Penalize(ctx, "h", 5*time.Second)
	if wait, _ := gate.Acquire(ctx, "h", time.Second); wait != 5*time.Second {
		t.Fatalf("after penalty wait = %v, want 5s", wait)
	}

	_ = gate.Penalize(ctx, "h", time.Second) // shorter: must not shorten
	if wait, _ := gate.Acquire(ctx, "h", time.Second); wait != 5*time.Second {
		t.Fatalf("shorter penalty shortened the wait: %v", wait)
	}
}

func TestMemoryGate_ZeroDelayNeverWaits(t *testing.T) {
	gate := NewMemory(nil)
	for range 3 {
		if wait, _ := gate.Acquire(context.Background(), "h", 0); wait != 0 {
			t.Fatalf("wait = %v", wait)
		}
	}
}
