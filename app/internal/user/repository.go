package user

import (
	"context"
	"fmt"

	"github.com/imlargo/medusa/postgres"

	"github.com/imlargo/medusa/app/internal/database/sqlc"
)

// Repository stores accounts. It speaks the package's errors: a missing row
// is ErrNotFound and a duplicate email ErrEmailTaken, so no driver error
// reaches the callers.
type Repository struct {
	queries *sqlc.Queries
}

// NewRepository stores accounts through queries.
func NewRepository(queries *sqlc.Queries) *Repository {
	return &Repository{queries: queries}
}

// Create stores a new account with an already hashed password.
func (r *Repository) Create(ctx context.Context, email, passwordHash string) (*User, error) {
	row, err := r.queries.CreateUser(ctx, sqlc.CreateUserParams{Email: NormalizeEmail(email), Password: passwordHash})
	if err != nil {
		return nil, classify("create user", err)
	}
	return toUser(row), nil
}

// Get returns the account with id.
func (r *Repository) Get(ctx context.Context, id int64) (*User, error) {
	row, err := r.queries.GetUser(ctx, id)
	if err != nil {
		return nil, classify("get user", err)
	}
	return toUser(row), nil
}

// GetByEmail returns the account with email, in any letter case.
func (r *Repository) GetByEmail(ctx context.Context, email string) (*User, error) {
	row, err := r.queries.GetUserByEmail(ctx, NormalizeEmail(email))
	if err != nil {
		return nil, classify("get user by email", err)
	}
	return toUser(row), nil
}

// Delete removes the account with id.
func (r *Repository) Delete(ctx context.Context, id int64) error {
	if err := r.queries.DeleteUser(ctx, id); err != nil {
		return classify("delete user", err)
	}
	return nil
}

func toUser(row sqlc.User) *User {
	return &User{
		ID:        row.ID,
		CreatedAt: row.CreatedAt.Time,
		UpdatedAt: row.UpdatedAt.Time,
		Email:     row.Email,
		Password:  row.Password,
	}
}

// classify maps the driver errors callers act on to this package's errors,
// and wraps everything else with the operation that failed.
func classify(op string, err error) error {
	if postgres.IsNoRows(err) {
		return ErrNotFound.WithCause(fmt.Errorf("%s: %w", op, err))
	}
	// The email is the only unique column besides the primary key.
	if postgres.IsUniqueViolation(err) {
		return ErrEmailTaken.WithCause(fmt.Errorf("%s: %w", op, err))
	}
	return fmt.Errorf("%s: %w", op, err)
}
