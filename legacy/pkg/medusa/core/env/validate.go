package env

import (
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
)

// LoadEnv loads environment variables from a .env file in the current directory.
// If the .env file doesn't exist or cannot be loaded, it logs a warning and
// continues with system environment variables.
//
// This function should be called early in your application's initialization:
//
//	func main() {
//	    env.LoadEnv()
//	    // ... rest of initialization
//	}
//
// The .env file format is:
//
//	KEY=value
//	DATABASE_URL=postgres://localhost/mydb
//	DEBUG=true
func LoadEnv() {
	// Load .env file if it exists
	if err := godotenv.Load(); err != nil {
		log.Println("Error loading .env file, proceeding with system environment variables")
	}
}

// CheckEnv validates that all required environment variables are set.
// It first calls LoadEnv() to load variables from .env file, then checks
// that each required variable has a non-empty value.
//
// Returns an error listing all missing variables if any are not set.
// Returns nil if all required variables are present.
//
// Example:
//
//	required := []string{
//	    "DATABASE_URL",
//	    "JWT_SECRET",
//	    "REDIS_URL",
//	}
//	if err := env.CheckEnv(required); err != nil {
//	    log.Fatal(err) // Prints: missing required environment variables: [JWT_SECRET REDIS_URL]
//	}
//
// This is useful for failing fast at startup if critical configuration is missing.
func CheckEnv(required []string) error {
	var missingEnvVars []string

	LoadEnv()

	// Check for missing environment variables
	for _, envVar := range required {
		if os.Getenv(envVar) == "" {
			missingEnvVars = append(missingEnvVars, envVar)
		}
	}

	// If there are missing variables, return an error listing them
	if len(missingEnvVars) > 0 {
		return fmt.Errorf("missing required environment variables: %v", missingEnvVars)
	}

	return nil
}
