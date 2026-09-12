package server

import (
	"sync"
	"time"
)

// ipLimiter is an in memory, per IP sliding window limiter. It is safe for
// concurrent use. State is process local and resets on restart, which is
// accepted for this service's scale.
type ipLimiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	hits   map[string][]time.Time
}

func newIPLimiter(max int, window time.Duration) *ipLimiter {
	return &ipLimiter{max: max, window: window, hits: make(map[string][]time.Time)}
}

// Allow records a hit for ip at time now and reports whether it is within
// the limit. When not allowed, retryAfter is how long the caller should
// wait before the oldest hit in the window falls out of it.
func (l *ipLimiter) Allow(ip string, now time.Time) (allowed bool, retryAfter time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	cutoff := now.Add(-l.window)
	prior := l.hits[ip]
	kept := prior[:0]
	for _, t := range prior {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}

	if len(kept) >= l.max {
		retryAfter = kept[0].Add(l.window).Sub(now)
		if retryAfter < 0 {
			retryAfter = 0
		}
		l.hits[ip] = kept
		return false, retryAfter
	}

	kept = append(kept, now)
	l.hits[ip] = kept
	return true, 0
}
