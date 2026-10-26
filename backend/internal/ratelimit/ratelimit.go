// Package ratelimit limits how often one client may do something.
//
// Each client (for now: each IP address; later: each logged-in user) gets its
// own "token bucket" from golang.org/x/time/rate:
//
//   - The bucket holds at most `burst` tokens and starts full.
//   - Every request takes one token. No token left -> the request is refused.
//   - Tokens flow back at a steady rate, e.g. 30 per hour = one every 2 minutes.
//
// So a user can ask a few questions quickly (the burst), but not hundreds.
package ratelimit

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Limiter keeps one token bucket per key (e.g. per IP address).
type Limiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	every    rate.Limit    // how fast tokens come back
	burst    int           // bucket size
	idle     time.Duration // forget clients that were quiet this long
	swept    time.Time     // last time we removed idle clients
	now      func() time.Time
}

type visitor struct {
	bucket   *rate.Limiter
	lastSeen time.Time
}

// New allows `count` requests per `period` for each key, with bursts of up
// to `burst` requests. Example: New(30, time.Hour, 10).
func New(count int, period time.Duration, burst int) *Limiter {
	return &Limiter{
		visitors: make(map[string]*visitor),
		every:    rate.Every(period / time.Duration(count)),
		burst:    burst,
		idle:     max(period, 10*time.Minute), // a full bucket can be dropped safely
		now:      time.Now,
	}
}

// Allow reports whether the client `key` may make a request now. If not, it
// also returns how long the client should wait (for the Retry-After header).
func (l *Limiter) Allow(key string) (ok bool, retryAfter time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.sweep(now)

	v, found := l.visitors[key]
	if !found {
		v = &visitor{bucket: rate.NewLimiter(l.every, l.burst)}
		l.visitors[key] = v
	}
	v.lastSeen = now

	// Reserve one token. If we would have to wait for it, give it back and
	// refuse - we never make a request wait, we answer "429" instead.
	r := v.bucket.ReserveN(now, 1)
	if delay := r.DelayFrom(now); delay > 0 {
		r.CancelAt(now)
		return false, delay
	}
	return true, 0
}

// Len returns the number of clients being tracked (used by tests).
func (l *Limiter) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.visitors)
}

// sweep removes clients that were idle for a long time, so the map does not
// grow forever. It runs at most once a minute. The caller must hold l.mu.
func (l *Limiter) sweep(now time.Time) {
	if now.Sub(l.swept) < time.Minute {
		return
	}
	l.swept = now
	for key, v := range l.visitors {
		if now.Sub(v.lastSeen) > l.idle {
			delete(l.visitors, key)
		}
	}
}
