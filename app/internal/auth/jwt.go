// Package auth issues and validates the API's access tokens and carries the
// authenticated user through a request.
package auth

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims are the token claims: the registered set plus the user's ID.
type Claims struct {
	jwt.RegisteredClaims
	UserID int64 `json:"user_id"`
}

// JWT signs and parses HS256 tokens with a shared secret.
type JWT struct {
	secret []byte
}

// NewJWT returns a signer for secret, or an error when the secret is empty.
func NewJWT(secret string) (*JWT, error) {
	if secret == "" {
		return nil, errors.New("jwt: secret can't be empty")
	}
	return &JWT{secret: []byte(secret)}, nil
}

// MustNewJWT is NewJWT for tests and fixed configurations. It panics on an
// empty secret.
func MustNewJWT(secret string) *JWT {
	j, err := NewJWT(secret)
	if err != nil {
		panic(err)
	}
	return j
}

// GenerateToken signs a token for userID that expires at expiresAt.
func (j *JWT) GenerateToken(userID int64, expiresAt time.Time) (string, error) {
	now := time.Now()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
	})

	signed, err := token.SignedString(j.secret)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return signed, nil
}

// ParseToken validates the signature, the signing method and the expiry of
// token and returns its claims. A leading "Bearer " is ignored.
func (j *JWT) ParseToken(token string) (*Claims, error) {
	token = strings.TrimSpace(strings.TrimPrefix(token, "Bearer "))
	if token == "" {
		return nil, errors.New("token is empty")
	}

	parsed, err := jwt.ParseWithClaims(token, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method %v", t.Header["alg"])
		}
		return j.secret, nil
	},
		jwt.WithExpirationRequired(),
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Name}),
	)
	if err != nil {
		return nil, err
	}

	claims, ok := parsed.Claims.(*Claims)
	if !ok || !parsed.Valid {
		return nil, errors.New("token claims are not valid")
	}

	return claims, nil
}
