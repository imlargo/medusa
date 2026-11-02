package auth_test

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/imlargo/medusa/auth"
)

var secret = strings.Repeat("s", 32)

func TestSignAndParseRoundTrip(t *testing.T) {
	t.Run("int64 subject", func(t *testing.T) {
		tokens := auth.MustNewJWT[int64](secret)
		expiresAt := time.Now().Add(time.Hour).Truncate(time.Second)

		token, err := tokens.Sign(42, expiresAt)
		if err != nil {
			t.Fatalf("Sign() = %v", err)
		}
		claims, err := tokens.Parse(token)
		if err != nil {
			t.Fatalf("Parse() = %v", err)
		}
		if claims.Subject != 42 {
			t.Errorf("Subject = %d, want 42", claims.Subject)
		}
		if !claims.ExpiresAt.Equal(expiresAt) {
			t.Errorf("ExpiresAt = %s, want %s", claims.ExpiresAt, expiresAt)
		}
		if claims.IssuedAt.IsZero() {
			t.Error("IssuedAt is zero, want the signing time")
		}
	})

	t.Run("string subject", func(t *testing.T) {
		tokens := auth.MustNewJWT[string](secret)

		token, err := tokens.Sign("user_01J", time.Now().Add(time.Hour))
		if err != nil {
			t.Fatalf("Sign() = %v", err)
		}
		claims, err := tokens.Parse(token)
		if err != nil {
			t.Fatalf("Parse() = %v", err)
		}
		if claims.Subject != "user_01J" {
			t.Errorf("Subject = %q, want user_01J", claims.Subject)
		}
	})
}

func TestParseRejects(t *testing.T) {
	tokens := auth.MustNewJWT[int64](secret, auth.WithIssuer("api"))
	inAnHour := time.Now().Add(time.Hour)

	signWith := func(t *testing.T, method jwt.SigningMethod, key any, claims jwt.RegisteredClaims) string {
		t.Helper()
		token, err := jwt.NewWithClaims(method, claims).SignedString(key)
		if err != nil {
			t.Fatalf("sign test token: %v", err)
		}
		return token
	}
	valid := func() jwt.RegisteredClaims {
		return jwt.RegisteredClaims{Subject: "42", Issuer: "api", ExpiresAt: jwt.NewNumericDate(inAnHour)}
	}

	tests := []struct {
		name  string
		token func(t *testing.T) string
	}{
		{name: "empty", token: func(*testing.T) string { return "" }},
		{name: "garbage", token: func(*testing.T) string { return "not.a.token" }},
		{name: "expired", token: func(t *testing.T) string {
			c := valid()
			c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Minute))
			return signWith(t, jwt.SigningMethodHS256, []byte(secret), c)
		}},
		{name: "no expiry", token: func(t *testing.T) string {
			c := valid()
			c.ExpiresAt = nil
			return signWith(t, jwt.SigningMethodHS256, []byte(secret), c)
		}},
		{name: "another secret", token: func(t *testing.T) string {
			return signWith(t, jwt.SigningMethodHS256, []byte(strings.Repeat("x", 32)), valid())
		}},
		{name: "another HMAC algorithm", token: func(t *testing.T) string {
			return signWith(t, jwt.SigningMethodHS512, []byte(secret), valid())
		}},
		{name: "unsigned", token: func(t *testing.T) string {
			return signWith(t, jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType, valid())
		}},
		{name: "another issuer", token: func(t *testing.T) string {
			c := valid()
			c.Issuer = "other"
			return signWith(t, jwt.SigningMethodHS256, []byte(secret), c)
		}},
		{name: "no subject", token: func(t *testing.T) string {
			c := valid()
			c.Subject = ""
			return signWith(t, jwt.SigningMethodHS256, []byte(secret), c)
		}},
		{name: "non-integer subject", token: func(t *testing.T) string {
			c := valid()
			c.Subject = "abc"
			return signWith(t, jwt.SigningMethodHS256, []byte(secret), c)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if claims, err := tokens.Parse(tt.token(t)); err == nil {
				t.Errorf("Parse() = %+v, want an error", claims)
			}
		})
	}
}

func TestWithIssuerStampsTheIssuer(t *testing.T) {
	token, err := auth.MustNewJWT[int64](secret, auth.WithIssuer("api")).Sign(1, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("Sign() = %v", err)
	}

	if _, err := auth.MustNewJWT[int64](secret, auth.WithIssuer("api")).Parse(token); err != nil {
		t.Errorf("Parse() with the same issuer = %v, want nil", err)
	}
	if _, err := auth.MustNewJWT[int64](secret, auth.WithIssuer("other")).Parse(token); err == nil {
		t.Error("Parse() with another issuer = nil, want an error")
	}
}

func TestNewJWTRejectsAnEmptySecret(t *testing.T) {
	if _, err := auth.NewJWT[int64](""); err == nil {
		t.Error("NewJWT(\"\") = nil error, want one")
	}
}
