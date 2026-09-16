package httpapi

import (
	"sync"
	"time"
)

// tokenBucket is a single in-memory token-bucket limiter (plan 10.3: "per-
// token in-memory token bucket на запись"). It refills continuously
// (fractional tokens tracked internally) rather than in discrete ticks, so
// burst capacity and sustained rate are both exact regardless of how often
// Allow is called.
type tokenBucket struct {
	mu         sync.Mutex
	tokens     float64
	capacity   float64
	refillRate float64 // tokens per second
	updatedAt  time.Time
	now        func() time.Time
}

func newTokenBucket(capacity int, refillInterval time.Duration, now func() time.Time) *tokenBucket {
	if capacity <= 0 {
		capacity = 1
	}
	if refillInterval <= 0 {
		refillInterval = time.Second
	}
	return &tokenBucket{
		tokens:     float64(capacity),
		capacity:   float64(capacity),
		refillRate: 1 / refillInterval.Seconds(),
		updatedAt:  now(),
		now:        now,
	}
}

// Allow reports whether one request may proceed right now, consuming one
// token if so.
func (b *tokenBucket) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := b.now()
	if elapsed := now.Sub(b.updatedAt).Seconds(); elapsed > 0 {
		b.tokens += elapsed * b.refillRate
		if b.tokens > b.capacity {
			b.tokens = b.capacity
		}
		b.updatedAt = now
	}
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

// rateLimiter holds one tokenBucket per key (plan 10.3's "per-token"; this
// MVP's single shared user secret means every write currently shares one
// key/bucket in practice, but the mechanism is generic). Buckets are
// created lazily and never removed: this is an explicitly accepted MVP
// limitation (plan 10.3: "После рестарта in-memory лимиты сбрасываются;
// это приемлемое ограничение локального MVP") — a long-running server with
// many distinct keys would leak memory slowly, which does not apply here
// since the key space is effectively one value (the one configured user
// token) for the lifetime of a running process.
type rateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*tokenBucket
	capacity int
	interval time.Duration
	now      func() time.Time
}

func newRateLimiter(capacity int, interval time.Duration, now func() time.Time) *rateLimiter {
	return &rateLimiter{buckets: make(map[string]*tokenBucket), capacity: capacity, interval: interval, now: now}
}

// Allow reports whether a request keyed by key may proceed right now.
func (l *rateLimiter) Allow(key string) bool {
	l.mu.Lock()
	b, ok := l.buckets[key]
	if !ok {
		b = newTokenBucket(l.capacity, l.interval, l.now)
		l.buckets[key] = b
	}
	l.mu.Unlock()
	return b.Allow()
}
