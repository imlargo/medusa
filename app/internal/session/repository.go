// Package session stores refresh tokens: one row per token, grouped by the
// sign-in they descend from. The rules of issuing and rotating them are the
// auth battery's (auth.Refresher); this is its storage.
package session

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/imlargo/medusa/auth"
	"github.com/imlargo/medusa/postgres"

	"github.com/imlargo/medusa/app/internal/session/internal/db"
)

// Repository is the auth.RefreshStore of the app. A token's subject is the
// ID of its user.
type Repository struct {
	pool postgres.Conn
}

var _ auth.RefreshStore = (*Repository)(nil)

// NewRepository stores refresh tokens in the database behind pool.
func NewRepository(pool postgres.Conn) *Repository {
	return &Repository{pool: pool}
}

// queries runs on the transaction ctx carries, or on the pool.
func (r *Repository) queries(ctx context.Context) *db.Queries {
	return db.New(postgres.ConnFrom(ctx, r.pool))
}

// Create stores token.
func (r *Repository) Create(ctx context.Context, token auth.RefreshToken) error {
	userID, err := strconv.ParseInt(token.Subject, 10, 64)
	if err != nil {
		return fmt.Errorf("create refresh token: subject %q is not a user ID", token.Subject)
	}

	err = r.queries(ctx).CreateRefreshToken(ctx, db.CreateRefreshTokenParams{
		Hash:      token.Hash,
		Family:    token.Family,
		Parent:    pgtype.Text{String: token.Parent, Valid: token.Parent != ""},
		UserID:    userID,
		ExpiresAt: pgtype.Timestamptz{Time: token.ExpiresAt, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("create refresh token: %w", err)
	}
	return nil
}

// Consume marks the token with hash as used, retires its unused siblings,
// and reports whether it already was used and how long ago, by the
// database's clock. An unknown hash is auth.ErrRefreshTokenNotFound.
func (r *Repository) Consume(ctx context.Context, hash string) (auth.ConsumedToken, error) {
	row, err := r.queries(ctx).ConsumeRefreshToken(ctx, hash)
	if postgres.IsNoRows(err) {
		return auth.ConsumedToken{}, auth.ErrRefreshTokenNotFound
	}
	if err != nil {
		return auth.ConsumedToken{}, fmt.Errorf("consume refresh token: %w", err)
	}

	return auth.ConsumedToken{
		RefreshToken: auth.RefreshToken{
			Hash:      row.Hash,
			Family:    row.Family,
			Parent:    row.Parent.String,
			Subject:   strconv.FormatInt(row.UserID, 10),
			ExpiresAt: row.ExpiresAt.Time,
		},
		Reused:  row.Reused,
		UsedAgo: time.Duration(row.UsedAgoSeconds * float64(time.Second)),
	}, nil
}

// RevokeFamily removes every token of family.
func (r *Repository) RevokeFamily(ctx context.Context, family string) error {
	if err := r.queries(ctx).DeleteRefreshTokenFamily(ctx, family); err != nil {
		return fmt.Errorf("revoke refresh token family: %w", err)
	}
	return nil
}

// DeleteExpired removes the tokens whose time has passed and returns how
// many there were. Expired tokens are already refused; this keeps the table
// from growing forever.
func (r *Repository) DeleteExpired(ctx context.Context) (int64, error) {
	deleted, err := r.queries(ctx).DeleteExpiredRefreshTokens(ctx)
	if err != nil {
		return 0, fmt.Errorf("delete expired refresh tokens: %w", err)
	}
	return deleted, nil
}
