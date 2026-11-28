// Package store groups the repositories of every feature, so writes across
// features can share one transaction.
package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/imlargo/medusa"
	"github.com/imlargo/medusa/auth"
	"github.com/imlargo/medusa/postgres"
	"github.com/imlargo/medusa/postgres/queue"

	"github.com/imlargo/medusa/app/internal/database/sqlc"
	"github.com/imlargo/medusa/app/internal/session"
	"github.com/imlargo/medusa/app/internal/user"
)

// Store holds one repository per aggregate.
type Store struct {
	conn     postgres.Conn
	jobs     *queue.Client
	Users    *user.Repository
	Sessions *session.Repository
}

// New builds the repositories over conn: the *pgxpool.Pool, or a transaction
// opened on it.
func New(conn postgres.Conn) *Store {
	queries := sqlc.New(conn)

	return &Store{
		conn:     conn,
		jobs:     queue.New(conn, queue.WithCorrelationID(medusa.RequestIDFrom), queue.WithMetadata(callerMetadata)),
		Users:    user.NewRepository(queries),
		Sessions: session.NewRepository(queries),
	}
}

// callerMetadata is what a job remembers about who caused it, so its
// handler runs as the same subject and tenant (see bootstrap/worker.go).
func callerMetadata(ctx context.Context) map[string]string {
	caller, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return nil
	}
	return caller.Metadata()
}

// Enqueue adds a job for the worker (see cmd/worker). On the Store that
// WithTx hands out, the job is part of the transaction: it exists exactly
// when the writes around it are committed.
func (s *Store) Enqueue(ctx context.Context, kind string, payload any, opts ...queue.EnqueueOption) error {
	_, _, err := s.jobs.EnqueueTx(ctx, s.conn, kind, payload, opts...)
	return err
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
