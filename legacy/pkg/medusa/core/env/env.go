// Package env provides utilities for loading and accessing environment variables.
//
// This package simplifies working with environment configuration by providing
// type-safe accessors with fallback values and validation helpers.
//
// Features:
//   - Type-safe environment variable access (string, int, bool)
//   - Fallback values when variables are not set
//   - .env file loading support via godotenv
//   - Required variable validation
//
// Example usage:
//
//	// Load .env file
//	env.LoadEnv()
//
//	// Get environment variables with fallbacks
//	port := env.GetEnvInt("PORT", 8080)
//	debug := env.GetEnvBool("DEBUG", false)
//	host := env.GetEnvString("HOST", "localhost")
//
//	// Validate required environment variables
//	required := []string{"DATABASE_URL", "JWT_SECRET", "REDIS_URL"}
//	if err := env.CheckEnv(required); err != nil {
//	    log.Fatal(err)
//	}
package env

import (
	"os"
	"strconv"
)

// GetEnvInt retrieves an integer environment variable with a fallback value.
// If the variable is not set or cannot be parsed as an integer, returns the fallback.
//
// Example:
//
//	port := env.GetEnvInt("PORT", 8080) // Returns 8080 if PORT is not set
func GetEnvInt(key string, fallback int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return fallback
}

// GetEnvBool retrieves a boolean environment variable with a fallback value.
// Accepts: "1", "t", "T", "TRUE", "true", "True", "0", "f", "F", "FALSE", "false", "False".
// If the variable is not set or cannot be parsed as a boolean, returns the fallback.
//
// Example:
//
//	debug := env.GetEnvBool("DEBUG", false) // Returns false if DEBUG is not set
func GetEnvBool(key string, fallback bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return fallback
}

// GetEnvString retrieves a string environment variable with a fallback value.
// If the variable is not set or is empty, returns the fallback.
//
// Example:
//
//	host := env.GetEnvString("HOST", "localhost") // Returns "localhost" if HOST is not set
func GetEnvString(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
