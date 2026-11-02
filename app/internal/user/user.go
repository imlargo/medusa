// Package models holds the domain types the services work with and the API
// returns.
package models

import "time"

// User is an account. Password holds the bcrypt hash and is never serialized.
type User struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Email    string `json:"email"`
	Password string `json:"-"`
}
