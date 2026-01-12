package jwt

import "errors"

type Config struct {
	Secret string
}

func (cfg Config) Validate() error {
	if cfg.Secret == "" {
		return errors.New("secret cant be empty")
	}

	return nil
}
