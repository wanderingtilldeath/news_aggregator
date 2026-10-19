package fetch

import (
	"net/http"
	"testing"
	"time"
)

func TestShouldRetryStatus(t *testing.T) {
	for _, status := range []int{429, 500, 502, 503, 504} {
		if !ShouldRetryStatus(status) {
			t.Errorf("ShouldRetryStatus(%d) = false", status)
		}
	}
	for _, status := range []int{200, 301, 400, 401, 403, 404, 410, 501} {
		if ShouldRetryStatus(status) {
			t.Errorf("ShouldRetryStatus(%d) = true", status)
		}
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 1, 2, 15, 0, 0, 0, time.UTC)

	tests := []struct {
		in   string
		want time.Duration
	}{
		{"", 0},
		{"0", 0},
		{"5", 5 * time.Second},
		{" 120 ", 2 * time.Minute},
		{"-3", 0},
		{"garbage", 0},
		{"99999999", maxRetryAfter},
		{now.Add(30 * time.Second).Format(http.TimeFormat), 30 * time.Second},
		{now.Add(-time.Minute).Format(http.TimeFormat), 0},
	}

	for _, tt := range tests {
		if got := ParseRetryAfter(tt.in, now); got != tt.want {
			t.Errorf("ParseRetryAfter(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestBackoff_FullJitterBounds(t *testing.T) {
	b := Backoff{Base: 100 * time.Millisecond, Max: time.Second}

	cases := []struct {
		attempt int
		ceiling time.Duration
	}{
		{0, 100 * time.Millisecond},
		{1, 200 * time.Millisecond},
		{3, 800 * time.Millisecond},
		{4, time.Second}, // capped
		{50, time.Second},
		{1 << 20, time.Second},
	}

	for _, c := range cases {
		if got := b.Delay(c.attempt, func() float64 { return 0 }); got != 0 {
			t.Errorf("attempt %d with rand=0: got %v, want 0", c.attempt, got)
		}

		got := b.Delay(c.attempt, func() float64 { return 0.999999 })
		if got >= c.ceiling || got < c.ceiling*99/100 {
			t.Errorf("attempt %d with rand~1: got %v, want just under %v", c.attempt, got, c.ceiling)
		}
	}
}

func TestBackoff_DefaultsAndRandom(t *testing.T) {
	var b Backoff // zero value must still work

	for attempt := 0; attempt < 10; attempt++ {
		d := b.Delay(attempt, nil)
		if d < 0 || d > DefaultBackoff.Max {
			t.Fatalf("attempt %d: delay %v out of range", attempt, d)
		}
	}
}
