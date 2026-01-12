package jwt

import "errors"

// Config holds the JWT configuration settings.
type Config struct {
	// Secret is the key used to sign and validate JWT tokens.
	// It should be a strong, randomly generated string.
	Secret string
}

// Validate checks if the configuration is valid.
// Returns an error if the secret is empty.
func (cfg Config) Validate() error {
	if cfg.Secret == "" {
		return errors.New("secret can't be empty")
	}

	return nil
}
