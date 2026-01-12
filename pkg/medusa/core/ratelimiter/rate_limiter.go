// Package ratelimiter provides rate limiting functionality using the token bucket algorithm.
//
// The rate limiter helps protect your API from abuse by limiting the number of requests
// a client can make within a specific time window. It uses per-key tracking (typically
// by IP address) and automatically cleans up inactive entries to prevent memory leaks.
//
// Example usage:
//
//	config := ratelimiter.Config{
//	    TimeFrame:            time.Minute,
//	    RequestsPerTimeFrame: 100, // 100 requests per minute
//	}
//	limiter := ratelimiter.NewTokenBucketLimiter(config)
//
//	// In middleware
//	allowed, tokensLeft := limiter.Allow(clientIP)
//	if !allowed {
//	    return fmt.Errorf("rate limit exceeded, tokens left: %.2f", tokensLeft)
//	}
//
// The limiter is safe for concurrent use and automatically removes inactive entries
// after 3 minutes of inactivity.
package ratelimiter

// RateLimiter defines the interface for rate limiting implementations.
//
// Implementations must be safe for concurrent use by multiple goroutines.
type RateLimiter interface {
	// Allow checks if a request for the given key should be allowed.
	// Returns true if the request is allowed, false if rate limit is exceeded.
	// The second return value is the number of tokens remaining (for debugging/headers).
	//
	// The key is typically a client identifier like an IP address or user ID.
	Allow(key string) (bool, float64)
}
