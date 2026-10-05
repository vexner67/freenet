package inmem

import (
	"context"
	"sync"
	"time"
)

type window struct {
	count   int
	resetAt time.Time
}

// sweepInterval is how often expired windows are dropped, so the map
// does not grow with every address and email ever seen.
const sweepInterval = time.Minute

// RateLimiter keeps fixed-window counters in memory, for one replica only.
type RateLimiter struct {
	windows   map[string]window
	lastSweep time.Time
	now       func() time.Time
	mu        sync.Mutex
}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		windows: make(map[string]window),
		now:     time.Now,
	}
}

func (l *RateLimiter) Allow(_ context.Context, key string, limit int, period time.Duration) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.sweep(now)

	w := l.windows[key]
	if !now.Before(w.resetAt) {
		w = window{resetAt: now.Add(period)}
	}

	w.count++
	l.windows[key] = w

	return w.count <= limit, nil
}

// sweep drops expired windows; the caller holds the lock.
func (l *RateLimiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < sweepInterval {
		return
	}

	for key, w := range l.windows {
		if !now.Before(w.resetAt) {
			delete(l.windows, key)
		}
	}

	l.lastSweep = now
}
