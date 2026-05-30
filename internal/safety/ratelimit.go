package safety

import (
	"sync"
	"time"
)

type RateLimiter struct {
	mu       sync.Mutex
	limits   map[string]*rateLimit
	maxCalls int
	window   time.Duration
}

type rateLimit struct {
	count  int
	window time.Time
}

func NewRateLimiter(maxCalls int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		limits:   make(map[string]*rateLimit),
		maxCalls: maxCalls,
		window:   window,
	}
}

func (r *RateLimiter) Allow(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	lim, ok := r.limits[key]
	if !ok || (r.window > 0 && now.After(lim.window)) {
		r.limits[key] = &rateLimit{count: 1, window: now.Add(r.window)}
		return true
	}

	if lim.count >= r.maxCalls {
		return false
	}

	lim.count++
	return true
}
