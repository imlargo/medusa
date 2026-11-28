package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type txKey struct{}

// Transact runs fn in a transaction on conn. The transaction travels in the
// context fn receives: every repository that takes its connection from
// [ConnFrom] with that context joins it, whichever package it lives in. It
// commits when fn returns nil and rolls back when fn returns an error or
// panics.
//
//	err := postgres.Transact(ctx, pool, func(ctx context.Context) error {
//	    order, err := orders.Create(ctx, in)   // one package
//	    if err != nil {
//	        return err
//	    }
//	    return stock.Reserve(ctx, order.Items) // another, same transaction
//	})
//
// Called with a context that already carries a transaction, it opens a
// savepoint inside it, so a function that needs atomicity can ask for it
// without knowing whether its caller already did.
//
// Pass the context on. A call made with another context, such as
// context.Background(), runs outside the transaction and is not rolled back
// with it.
func Transact(ctx context.Context, conn Conn, fn func(ctx context.Context) error) error {
	err := pgx.BeginFunc(ctx, ConnFrom(ctx, conn), func(tx pgx.Tx) error {
		return fn(context.WithValue(ctx, txKey{}, tx))
	})
	if err != nil {
		return fmt.Errorf("transaction: %w", err)
	}
	return nil
}

// ConnFrom returns the transaction ctx carries, or fallback, usually the
// pool, when it carries none. A repository takes its connection from it on
// every call, so the same method works inside and outside [Transact].
func ConnFrom(ctx context.Context, fallback Conn) Conn {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}
	return fallback
}
