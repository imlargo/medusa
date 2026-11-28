// Package session stores refresh tokens: one row per token, grouped by the
// sign-in they descend from. The rules of issuing and rotating them are the
// auth battery's (auth.Refresher); this is its storage.
package session

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/imlargo/medusa/auth"
	"github.com/imlargo/medusa/postgres"

	"github.com/imlargo/medusa/app/internal/database/sqlc"
)

// Repository is the auth.RefreshStore of the app. A token's subject is the
// ID of its user.
type Repository struct {
	queries *sqlc.Queries
}

var _ auth.RefreshStore = (*Repository)(nil)

// NewRepository stores refresh tokens through queries.
func NewRepository(queries *sqlc.Queries) *Repository {
	return &Repository{queries: queries}
}

// Create stores token.
func (r *Repository) Create(ctx context.Context, token auth.RefreshToken) error {
	userID, err := strconv.ParseInt(token.Subject, 10, 64)
	if err != nil {
		return fmt.Errorf("create refresh token: subject %q is not a user ID", token.Subject)
	}

	err = r.queries.CreateRefreshToken(ctx, sqlc.CreateRefreshTokenParams{
		Hash:      token.Hash,
		Family:    token.Family,
		UserID:    userID,
		ExpiresAt: pgtype.Timestamptz{Time: token.ExpiresAt, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("create refresh token: %w", err)
	}
	return nil
}

// Consume marks the token with hash as used and reports whether it already
// was. An unknown hash is auth.ErrRefreshTokenNotFound.
func (r *Repository) Consume(ctx context.Context, hash string) (auth.RefreshToken, bool, error) {
	row, err := r.queries.ConsumeRefreshToken(ctx, hash)
	if postgres.IsNoRows(err) {
		return auth.RefreshToken{}, false, auth.ErrRefreshTokenNotFound
	}
	if err != nil {
		return auth.RefreshToken{}, false, fmt.Errorf("consume refresh token: %w", err)
	}

	return auth.RefreshToken{
		Hash:      row.Hash,
		Family:    row.Family,
		Subject:   strconv.FormatInt(row.UserID, 10),
		ExpiresAt: row.ExpiresAt.Time,
	}, row.Reused, nil
}

// RevokeFamily removes every token of family.
func (r *Repository) RevokeFamily(ctx context.Context, family string) error {
	if err := r.queries.DeleteRefreshTokenFamily(ctx, family); err != nil {
		return fmt.Errorf("revoke refresh token family: %w", err)
	}
	return nil
}

// DeleteExpired removes the tokens whose time has passed and returns how
// many there were. Expired tokens are already refused; this keeps the table
// from growing forever.
func (r *Repository) DeleteExpired(ctx context.Context) (int64, error) {
	deleted, err := r.queries.DeleteExpiredRefreshTokens(ctx)
	if err != nil {
		return 0, fmt.Errorf("delete expired refresh tokens: %w", err)
	}
	return deleted, nil
}
