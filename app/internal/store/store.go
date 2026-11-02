// Package store groups the repositories the services use.
package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/imlargo/medusa/app/internal/database/sqlc"
	"github.com/imlargo/medusa/app/internal/repositories"
)

// conn is what the repositories run on: the pool, or a transaction opened on
// it. Both can begin a transaction; inside one, Begin opens a savepoint.
type conn interface {
	sqlc.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Store holds one repository per aggregate.
type Store struct {
	conn  conn
	Users repositories.UserRepository
}

// New builds the repositories over conn, normally the *pgxpool.Pool.
func New(conn conn) *Store {
	queries := sqlc.New(conn)

	return &Store{
		conn:  conn,
		Users: repositories.NewUserRepository(queries),
	}
}

// WithTx runs fn with a Store whose repositories all share one transaction.
// The transaction commits when fn returns nil and rolls back when it returns
// an error or panics. Calling WithTx on the Store fn receives nests a
// savepoint.
//
//	err := s.store.WithTx(ctx, func(tx *store.Store) error {
//	    user, err := tx.Users.Create(ctx, email, hash)
//	    if err != nil {
//	        return err
//	    }
//	    return tx.Profiles.Create(ctx, user.ID)
//	})
func (s *Store) WithTx(ctx context.Context, fn func(tx *Store) error) error {
	err := pgx.BeginFunc(ctx, s.conn, func(tx pgx.Tx) error {
		return fn(New(tx))
	})
	if err != nil {
		return fmt.Errorf("transaction: %w", err)
	}
	return nil
}
