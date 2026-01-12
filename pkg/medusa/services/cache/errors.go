package cache

import "errors"

var (
	// ErrKeyNotFound is returned when a cache key doesn't exist.
	// This is distinct from other errors like network failures or deserialization errors.
	//
	// Use this to differentiate between cache misses and actual errors:
	//
	//	if err := cache.Get(ctx, key, &value); err != nil {
	//	    if err == cache.ErrKeyNotFound {
	//	        // Cache miss - fetch from database
	//	    } else {
	//	        // Real error - log and handle
	//	    }
	//	}
	ErrKeyNotFound = errors.New("key not found in cache")

	// ErrNilValue is returned when a nil pointer is provided as the destination for Get operations.
	// This helps catch programming errors early.
	ErrNilValue = errors.New("nil value provided")
)
