package jwt

import "github.com/golang-jwt/jwt/v5"

// CustomClaims extends the standard JWT claims with application-specific data.
// It includes the user ID for authentication purposes.
type CustomClaims struct {
	jwt.RegisteredClaims
	UserID uint `json:"user_id"`
}
