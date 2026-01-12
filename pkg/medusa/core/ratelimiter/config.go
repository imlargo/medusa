package ratelimiter

import "time"

// Config holds the rate limiter configuration.
type Config struct {
	// TimeFrame is the duration of the time window for rate limiting.
	// For example, time.Minute for per-minute rate limiting.
	TimeFrame time.Duration

	// RequestsPerTimeFrame is the maximum number of requests allowed within the time frame.
	// For example, 100 means allow 100 requests per TimeFrame.
	RequestsPerTimeFrame int
}
