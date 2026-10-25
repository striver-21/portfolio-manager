// Package ratelimit provides a small in-memory token-bucket limiter keyed by an
// arbitrary string (typically a client IP). It is intentionally process-local;
// a multi-instance deployment would put this state in Redis or at the edge.
package ratelimit

import (
	"sync"
	"time"
)

// Limiter hands out tokens from one bucket per key.
type Limiter struct {
	rate  float64       // tokens added per second
	burst float64       // bucket capacity
	ttl   time.Duration // idle time after which a bucket is forgotten
	now   func() time.Time

	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	tokens float64
	last   time.Time
}

// New returns a limiter allowing burst requests at once and then
// perSecond requests per second on average.
func New(perSecond float64, burst int) *Limiter {
	if perSecond <= 0 {
		perSecond = 1
	}
	if burst < 1 {
		burst = 1
	}
	return &Limiter{
		rate:    perSecond,
		burst:   float64(burst),
		ttl:     time.Duration(float64(burst)/perSecond*float64(time.Second)) + time.Minute,
		now:     time.Now,
		buckets: make(map[string]*bucket),
	}
}

// Allow reports whether a request for key may proceed, consuming one token
// if so.
func (l *Limiter) Allow(key string) bool {
	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
		l.maybePrune(now)
	}
	b.tokens += now.Sub(b.last).Seconds() * l.rate
	if b.tokens > l.burst {
		b.tokens = l.burst
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// maybePrune drops buckets idle long enough to be full again. It only runs
// when the map has grown, so steady-state Allow calls stay O(1).
func (l *Limiter) maybePrune(now time.Time) {
	const pruneAt = 1024
	if len(l.buckets) < pruneAt {
		return
	}
	for k, b := range l.buckets {
		if now.Sub(b.last) > l.ttl {
			delete(l.buckets, k)
		}
	}
}
