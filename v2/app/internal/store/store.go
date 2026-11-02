// Package store groups the repositories the services use.
package store

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/imlargo/medusa/app/internal/database/sqlc"
	"github.com/imlargo/medusa/app/internal/repositories"
)

// Store holds one repository per aggregate.
type Store struct {
	Users repositories.UserRepository
}

// New builds the repositories over pool.
func New(pool *pgxpool.Pool) *Store {
	queries := sqlc.New(pool)

	return &Store{
		Users: repositories.NewUserRepository(queries),
	}
}
