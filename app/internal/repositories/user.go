// Package repositories reads and writes the domain models through the
// queries sqlc generates from internal/database/queries.
package repositories

import (
	"context"
	"errors"
	"fmt"

	"github.com/imlargo/medusa/postgres"

	"github.com/imlargo/medusa/app/internal/database/sqlc"
	"github.com/imlargo/medusa/app/internal/models"
)

var (
	// ErrNotFound means no row matched.
	ErrNotFound = errors.New("not found")

	// ErrDuplicate means the write collided with a unique constraint.
	ErrDuplicate = errors.New("duplicate")
)

// UserRepository stores accounts.
type UserRepository interface {
	Create(ctx context.Context, email, passwordHash string) (*models.User, error)
	Get(ctx context.Context, id int64) (*models.User, error)
	GetByEmail(ctx context.Context, email string) (*models.User, error)
	Delete(ctx context.Context, id int64) error
}

type userRepository struct {
	queries *sqlc.Queries
}

// NewUserRepository stores accounts through queries.
func NewUserRepository(queries *sqlc.Queries) UserRepository {
	return &userRepository{queries: queries}
}

func (r *userRepository) Create(ctx context.Context, email, passwordHash string) (*models.User, error) {
	row, err := r.queries.CreateUser(ctx, sqlc.CreateUserParams{Email: email, Password: passwordHash})
	if err != nil {
		return nil, classify("create user", err)
	}
	return toUser(row), nil
}

func (r *userRepository) Get(ctx context.Context, id int64) (*models.User, error) {
	row, err := r.queries.GetUser(ctx, id)
	if err != nil {
		return nil, classify("get user", err)
	}
	return toUser(row), nil
}

func (r *userRepository) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	row, err := r.queries.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, classify("get user by email", err)
	}
	return toUser(row), nil
}

func (r *userRepository) Delete(ctx context.Context, id int64) error {
	if err := r.queries.DeleteUser(ctx, id); err != nil {
		return classify("delete user", err)
	}
	return nil
}

func toUser(row sqlc.User) *models.User {
	return &models.User{
		ID:        row.ID,
		CreatedAt: row.CreatedAt.Time,
		UpdatedAt: row.UpdatedAt.Time,
		Email:     row.Email,
		Password:  row.Password,
	}
}

// classify maps the driver errors callers branch on to this package's
// sentinels, and wraps everything else with the operation that failed.
func classify(op string, err error) error {
	if postgres.IsNoRows(err) {
		return fmt.Errorf("%s: %w", op, ErrNotFound)
	}
	if postgres.IsUniqueViolation(err) {
		return fmt.Errorf("%s: %w", op, ErrDuplicate)
	}
	return fmt.Errorf("%s: %w", op, err)
}
