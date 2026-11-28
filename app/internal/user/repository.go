package user

import (
	"context"
	"fmt"

	"github.com/imlargo/medusa/postgres"

	"github.com/imlargo/medusa/app/internal/user/internal/db"
)

// Repository stores accounts. It speaks the package's errors: a missing row
// is ErrNotFound and a duplicate email ErrEmailTaken, so no driver error
// reaches the callers.
type Repository struct {
	pool postgres.Conn
}

// NewRepository stores accounts in the database behind pool.
func NewRepository(pool postgres.Conn) *Repository {
	return &Repository{pool: pool}
}

// queries runs on the transaction ctx carries, or on the pool.
func (r *Repository) queries(ctx context.Context) *db.Queries {
	return db.New(postgres.ConnFrom(ctx, r.pool))
}

// Create stores a new account with an already hashed password.
func (r *Repository) Create(ctx context.Context, email, passwordHash string) (*User, error) {
	row, err := r.queries(ctx).CreateUser(ctx, db.CreateUserParams{Email: NormalizeEmail(email), Password: passwordHash})
	if err != nil {
		return nil, classify("create user", err)
	}
	return toUser(row), nil
}

// Get returns the account with id.
func (r *Repository) Get(ctx context.Context, id int64) (*User, error) {
	row, err := r.queries(ctx).GetUser(ctx, id)
	if err != nil {
		return nil, classify("get user", err)
	}
	return toUser(row), nil
}

// GetByEmail returns the account with email, in any letter case.
func (r *Repository) GetByEmail(ctx context.Context, email string) (*User, error) {
	row, err := r.queries(ctx).GetUserByEmail(ctx, NormalizeEmail(email))
	if err != nil {
		return nil, classify("get user by email", err)
	}
	return toUser(row), nil
}

// Delete removes the account with id.
func (r *Repository) Delete(ctx context.Context, id int64) error {
	if err := r.queries(ctx).DeleteUser(ctx, id); err != nil {
		return classify("delete user", err)
	}
	return nil
}

func toUser(row db.User) *User {
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
