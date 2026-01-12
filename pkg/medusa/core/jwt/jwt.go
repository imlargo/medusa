// Package jwt provides JSON Web Token (JWT) generation and validation functionality.
// It uses HS256 signing method and supports custom claims with user identification.
package jwt

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWT handles JWT token generation and parsing operations.
type JWT struct {
	config Config
}

// NewJwt creates a new JWT instance with the provided configuration.
// It panics if the configuration is invalid.
func NewJwt(cfg Config) *JWT {

	if err := cfg.Validate(); err != nil {
		panic(err)
	}

	return &JWT{config: cfg}
}

// GenerateToken creates a new JWT token for the specified user ID with an expiration time.
// The token is signed using HS256 algorithm with the configured secret.
// Returns the signed token string or an error if signing fails.
func (j *JWT) GenerateToken(userID uint, expiresAt time.Time) (string, error) {

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, CustomClaims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Audience:  []string{},
		},
	})

	// Sign and get the complete encoded token as a string using the key
	tokenString, err := token.SignedString([]byte(j.config.Secret))
	if err != nil {
		return "", err
	}
	return tokenString, nil
}

// ParseToken validates and parses a JWT token string.
// It automatically strips "Bearer " prefix if present and validates the token signature,
// expiration, and signing method. Returns the custom claims if valid, or an error otherwise.
func (j *JWT) ParseToken(tokenString string) (*CustomClaims, error) {
	tokenString = strings.TrimPrefix(tokenString, "Bearer ")
	tokenString = strings.TrimSpace(tokenString)
	if tokenString == "" {
		return nil, errors.New("token is empty")
	}

	token, err := jwt.ParseWithClaims(tokenString, &CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method %v", token.Header["alg"])
		}

		return []byte(j.config.Secret), nil
	},
		jwt.WithExpirationRequired(),
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Name}),
	)
	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*CustomClaims); ok && token.Valid {
		return claims, nil
	} else {
		return nil, err
	}
}
