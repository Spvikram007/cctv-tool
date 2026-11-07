package security

import (
	"sync"
	"time"
)

// IPRateLimiter provides token-bucket rate limiting per client IP
type IPRateLimiter struct {
	mu       sync.Mutex
	limits   map[string]*clientBucket
	rate     float64 // tokens per second
	capacity float64 // max token burst
}

type clientBucket struct {
	tokens     float64
	lastRefill time.Time
}

func NewIPRateLimiter(rate float64, capacity float64) *IPRateLimiter {
	limiter := &IPRateLimiter{
		limits:   make(map[string]*clientBucket),
		rate:     rate,
		capacity: capacity,
	}

	// Periodically evict idle clients (every 5 minutes)
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		for range ticker.C {
			limiter.mu.Lock()
			cutoff := time.Now().Add(-10 * time.Minute)
			for ip, b := range limiter.limits {
				if b.lastRefill.Before(cutoff) {
					delete(limiter.limits, ip)
				}
			}
			limiter.mu.Unlock()
		}
	}()

	return limiter
}

// Allow checks if a request from the given IP is allowed
func (lim *IPRateLimiter) Allow(ip string) bool {
	lim.mu.Lock()
	defer lim.mu.Unlock()

	now := time.Now()
	b, exists := lim.limits[ip]
	if !exists {
		lim.limits[ip] = &clientBucket{
			tokens:     lim.capacity - 1.0,
			lastRefill: now,
		}
		return true
	}

	// Refill tokens based on elapsed time
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens += elapsed * lim.rate
	if b.tokens > lim.capacity {
		b.tokens = lim.capacity
	}
	b.lastRefill = now

	if b.tokens >= 1.0 {
		b.tokens -= 1.0
		return true
	}

	return false
}
