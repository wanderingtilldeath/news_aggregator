package fetch

import (
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// maxRetryAfter bounds how long a server can park a URL.
const maxRetryAfter = time.Hour

// ShouldRetryStatus reports whether an HTTP status is a transient failure
// worth retrying later. Other 4xx statuses are permanent.
func ShouldRetryStatus(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests ||
		statusCode == http.StatusInternalServerError ||
		statusCode == http.StatusBadGateway ||
		statusCode == http.StatusServiceUnavailable ||
		statusCode == http.StatusGatewayTimeout
}

// ParseRetryAfter parses a Retry-After header, either delta-seconds or an
// HTTP date, into a delay from now. Unparsable or negative values give 0.
func ParseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}

	var d time.Duration
	if secs, err := strconv.ParseInt(value, 10, 64); err == nil {
		if secs < 0 {
			return 0
		}
		if secs > int64(maxRetryAfter/time.Second) {
			return maxRetryAfter
		}
		d = time.Duration(secs) * time.Second
	} else if t, err := http.ParseTime(value); err == nil {
		d = t.Sub(now)
	} else {
		return 0
	}

	if d < 0 {
		return 0
	}
	return min(d, maxRetryAfter)
}

// Backoff computes exponential backoff with full jitter: the delay is drawn
// uniformly from [0, min(Max, Base*2^attempt)). Full jitter spreads retries
// from many workers instead of synchronising them into waves.
type Backoff struct {
	Base time.Duration
	Max  time.Duration
}

// DefaultBackoff is used when a Backoff is zero.
var DefaultBackoff = Backoff{Base: 500 * time.Millisecond, Max: 5 * time.Minute}

// Delay returns the wait before retry number attempt (0-based) and takes the
// random fraction in [0,1) so tests can make it deterministic.
func (b Backoff) Delay(attempt int, randFloat func() float64) time.Duration {
	if b.Base <= 0 {
		b = DefaultBackoff
	}
	if randFloat == nil {
		randFloat = rand.Float64
	}
	if attempt < 0 {
		attempt = 0
	}

	ceiling := b.Max
	if attempt < 30 { // avoid shifting into overflow
		if c := b.Base << attempt; c > 0 && c < ceiling {
			ceiling = c
		}
	}

	return time.Duration(float64(ceiling) * randFloat())
}
