package ratelimiter

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// tokenBucketLimiter implements rate limiting using the token bucket algorithm.
// It maintains a separate rate limiter for each key (typically IP address or user ID).
//
// The token bucket algorithm works by:
//   1. Starting with a full bucket of tokens (RequestsPerTimeFrame)
//   2. Each request consumes one token
//   3. Tokens are replenished at a steady rate (TimeFrame / RequestsPerTimeFrame)
//   4. If no tokens are available, the request is rejected
//
// This implementation is thread-safe and automatically cleans up inactive entries
// to prevent unbounded memory growth.
type tokenBucketLimiter struct {
	sync.RWMutex
	config  Config
	entries map[string]*tblEntry
}

// tblEntry represents a single rate limiter entry for a specific key.
type tblEntry struct {
	Limiter  *rate.Limiter // The actual rate limiter
	LastSeen time.Time     // Last time this key made a request (for cleanup)
}

// NewTokenBucketLimiter creates a new token bucket rate limiter with the given configuration.
// It starts a background goroutine to clean up inactive entries.
//
// Example:
//
//	config := ratelimiter.Config{
//	    TimeFrame:            time.Minute,
//	    RequestsPerTimeFrame: 60, // 60 requests per minute = 1 per second
//	}
//	limiter := ratelimiter.NewTokenBucketLimiter(config)
func NewTokenBucketLimiter(cfg Config) RateLimiter {
	rl := &tokenBucketLimiter{
		config:  cfg,
		entries: make(map[string]*tblEntry),
	}

	go rl.cleanUpEntries()

	return rl
}

// getEntry retrieves or creates a rate limiter entry for the given key.
// This method is safe for concurrent use.
func (rl *tokenBucketLimiter) getEntry(key string) *tblEntry {
	rl.Lock()
	_, exists := rl.entries[key]
	rl.Unlock()

	if !exists {
		// Create a new rate limiter with the configured rate
		limiter := rate.NewLimiter(rate.Every(rl.config.TimeFrame), rl.config.RequestsPerTimeFrame)
		rl.Lock()

		rl.entries[key] = &tblEntry{
			Limiter:  limiter,
			LastSeen: time.Now(),
		}

		rl.Unlock()
	}

	entry := rl.entries[key]
	entry.LastSeen = time.Now()

	return entry
}

// Allow checks if a request for the given key should be allowed based on the rate limit.
//
// Returns:
//   - allowed: true if the request should be allowed, false if rate limit is exceeded
//   - tokens: the number of tokens currently available (useful for rate limit headers)
//
// This method is safe for concurrent use.
func (rl *tokenBucketLimiter) Allow(key string) (bool, float64) {
	entry := rl.getEntry(key)

	allowed := entry.Limiter.Allow()
	tokens := entry.Limiter.Tokens()

	return allowed, tokens
}

// cleanUpEntries runs in a background goroutine and periodically removes
// rate limiter entries that haven't been used recently.
//
// This prevents memory leaks from accumulating entries for clients that
// are no longer active. Entries are removed after 3 minutes of inactivity.
func (rl *tokenBucketLimiter) cleanUpEntries() {
	maxAge := 3 * time.Minute
	timeInterval := time.Minute

	for {
		time.Sleep(timeInterval)

		rl.Lock()
		for key, entry := range rl.entries {
			if time.Since(entry.LastSeen) > maxAge {
				delete(rl.entries, key)
			}
		}
		rl.Unlock()
	}
}
