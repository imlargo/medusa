package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"

	"github.com/imlargo/medusa/apperr"
)

// ErrInvalidAPIKey: the request carries an API key that matches no client.
var ErrInvalidAPIKey = apperr.Unauthenticated("auth.invalid_api_key", "invalid API key")

// apiKeyHeader carries the key APIKeys reads.
const apiKeyHeader = "X-API-Key" //nolint:gosec // G101: a header name, not a credential

// APIKeyLookup finds the client an API key belongs to, given the key's hash
// (see [HashAPIKey]), and returns it as a [Principal], usually a
// [KindService] with the client's ID and scopes. It returns
// [ErrInvalidAPIKey] when no client has that key.
type APIKeyLookup func(ctx context.Context, keyHash string) (Principal, error)

// APIKeys authenticates a request by its X-API-Key header, one key per
// client. The application stores each key's hash, never the key, and looks
// the hash up: with an index on the hash that is one query, and comparing
// hashes leaks nothing about a key through timing.
func APIKeys(lookup APIKeyLookup) Authenticator {
	return AuthenticatorFunc(func(r *http.Request) (Principal, error) {
		key := r.Header.Get(apiKeyHeader)
		if key == "" {
			return Principal{}, ErrNoCredentials
		}

		principal, err := lookup(r.Context(), HashAPIKey(key))
		if err != nil {
			return Principal{}, err
		}
		if principal.Method == "" {
			principal.Method = MethodAPIKey
		}
		return principal, nil
	})
}

// HashAPIKey is the form an API key is stored and looked up in: its SHA-256
// in hex. A key is long and random, so a fast hash is enough; a password is
// not, and needs a slow one.
func HashAPIKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}
