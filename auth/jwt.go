// Package auth is Medusa's authentication battery: HS256 access tokens and
// the Gin middleware that requires them.
//
// The token's subject (the standard "sub" claim) is typed, so an app keyed
// by int64 IDs never parses strings by hand:
//
//	tokens, err := auth.NewJWT[int64](cfg.Secret, auth.WithIssuer("my-api"))
//	token, err := tokens.Sign(user.ID, time.Now().Add(15*time.Minute))
//
//	protected := router.Group("/v1", auth.Middleware(tokens))
//	userID, ok := auth.SubjectFrom[int64](ctx)
//
// It is a separate module, so an app with its own authentication never
// downloads the JWT library.
package auth

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Subject is what a token identifies: an int64 or a string ID.
type Subject interface {
	int64 | string
}

// Claims are the validated contents of a token.
type Claims[S Subject] struct {
	Subject   S
	IssuedAt  time.Time
	ExpiresAt time.Time
}

// JWT signs and parses HS256 tokens with a shared secret.
type JWT[S Subject] struct {
	secret []byte
	issuer string
}

// Option configures a JWT.
type Option func(*options)

type options struct {
	issuer string
}

// WithIssuer stamps issuer on every token signed and rejects tokens from any
// other issuer, so a token minted by another service sharing the secret is
// not accepted here.
func WithIssuer(issuer string) Option {
	return func(o *options) { o.issuer = issuer }
}

// NewJWT returns a signer for secret, or an error when the secret is empty.
func NewJWT[S Subject](secret string, opts ...Option) (*JWT[S], error) {
	if secret == "" {
		return nil, errors.New("jwt: secret can't be empty")
	}

	var o options
	for _, opt := range opts {
		opt(&o)
	}

	return &JWT[S]{secret: []byte(secret), issuer: o.issuer}, nil
}

// MustNewJWT is NewJWT for tests and fixed configurations. It panics on an
// empty secret.
func MustNewJWT[S Subject](secret string, opts ...Option) *JWT[S] {
	j, err := NewJWT[S](secret, opts...)
	if err != nil {
		panic(err)
	}
	return j
}

// Sign returns a token for subject that expires at expiresAt.
func (j *JWT[S]) Sign(subject S, expiresAt time.Time) (string, error) {
	now := time.Now()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   formatSubject(subject),
		Issuer:    j.issuer,
		ExpiresAt: jwt.NewNumericDate(expiresAt),
		IssuedAt:  jwt.NewNumericDate(now),
		NotBefore: jwt.NewNumericDate(now),
	})

	signed, err := token.SignedString(j.secret)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return signed, nil
}

// Parse validates token (without the "Bearer " prefix): its HS256 signature,
// its expiry, its issuer when WithIssuer is set, and that its subject is an S.
func (j *JWT[S]) Parse(token string) (*Claims[S], error) {
	if token == "" {
		return nil, errors.New("token is empty")
	}

	parserOpts := []jwt.ParserOption{
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Name}),
	}
	if j.issuer != "" {
		parserOpts = append(parserOpts, jwt.WithIssuer(j.issuer))
	}

	var registered jwt.RegisteredClaims
	if _, err := jwt.ParseWithClaims(token, &registered, func(*jwt.Token) (any, error) {
		return j.secret, nil
	}, parserOpts...); err != nil {
		return nil, err
	}

	subject, err := parseSubject[S](registered.Subject)
	if err != nil {
		return nil, err
	}

	claims := &Claims[S]{
		Subject:   subject,
		ExpiresAt: registered.ExpiresAt.Time,
	}
	if registered.IssuedAt != nil {
		claims.IssuedAt = registered.IssuedAt.Time
	}
	return claims, nil
}

func formatSubject[S Subject](subject S) string {
	switch v := any(subject).(type) {
	case int64:
		return strconv.FormatInt(v, 10)
	default:
		return any(subject).(string)
	}
}

func parseSubject[S Subject](raw string) (S, error) {
	var zero S
	if raw == "" {
		return zero, errors.New("token has no subject")
	}

	switch any(zero).(type) {
	case int64:
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return zero, errors.New("token subject is not an integer ID")
		}
		return any(id).(S), nil
	default:
		return any(raw).(S), nil
	}
}
