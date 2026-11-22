package scheduler

import (
	"sync"
	"time"
)

// strikesToSpend is how many rate limits in a row, each after a pause, mean the model's quota is gone for
// the day: a per-minute limit clears within one pause, so a second limit after it is the daily cap.
const strikesToSpend = 2

// quota follows the model's limits through one run. The first rate limit pauses every model pass for a
// cooldown; a second one after the pause ends all model work for the run, so the poll loop does not keep
// asking (and waiting through retries) until the time budget runs out. It also holds the request cap: bulk
// passes stop at maxCalls, while the few calls the edition needs (long summaries, concepts) do not count
// against it.
type quota struct {
	mu       sync.Mutex
	now      func() time.Time
	cooldown time.Duration
	maxCalls int64 // 0 means no cap
	calls    func() int64

	strikes int
	until   time.Time
	capped  bool // logged once
}

func newQuota(now func() time.Time, cooldown time.Duration, maxCalls int64, calls func() int64) *quota {
	return &quota{now: now, cooldown: cooldown, maxCalls: maxCalls, calls: calls}
}

// allow reports whether a model pass may start. bulk passes also stop at the request cap.
func (q *quota) allow(bulk bool) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.strikes >= strikesToSpend || q.now().Before(q.until) {
		return false
	}

	return !(bulk && q.overCapLocked())
}

// overCap reports (once, true) when the request cap was just reached, so the caller can log it.
func (q *quota) overCap() (reached, first bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if !q.overCapLocked() {
		return false, false
	}
	first, q.capped = !q.capped, true

	return true, first
}

func (q *quota) overCapLocked() bool {
	return q.maxCalls > 0 && q.calls != nil && q.calls() >= q.maxCalls
}

// limit shrinks the number of articles a bulk pass may take so that its requests (about one per `perRequest`
// articles) fit what is left of the cap; without a cap it returns n unchanged.
func (q *quota) limit(n, perRequest int) int {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.maxCalls <= 0 || q.calls == nil {
		return n
	}

	return min(n, int(max(q.maxCalls-q.calls(), 0))*perRequest)
}

// strike records a rate limit and starts a pause.
func (q *quota) strike() {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.strikes++
	q.until = q.now().Add(q.cooldown)
}

// ok records a pass that finished without a limit, so the next limit is a first strike again.
func (q *quota) ok() {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.strikes < strikesToSpend {
		q.strikes = 0
	}
}

// spent reports whether the quota is gone for this run.
func (q *quota) spent() bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	return q.strikes >= strikesToSpend
}

// wait is how long until the pause ends (0 when not paused).
func (q *quota) wait() time.Duration {
	q.mu.Lock()
	defer q.mu.Unlock()

	return max(q.until.Sub(q.now()), 0)
}
