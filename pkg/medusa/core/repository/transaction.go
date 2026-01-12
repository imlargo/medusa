package repository

import (
	"context"
	"database/sql"

	"gorm.io/gorm"
)

// TransactionManager provides centralized transaction management with context propagation.
//
// It handles database transactions in a safe, idiomatic way by storing the transaction
// in the context. This allows nested function calls to participate in the same transaction
// transparently.
//
// Key Features:
//   - Automatic transaction reuse for nested operations
//   - Context-based transaction propagation
//   - Automatic rollback on error
//   - Automatic commit on success
//   - Support for custom transaction options
//
// Example:
//
//	tm := repository.NewTransactionManager(db)
//
//	err := tm.WithTransaction(ctx, func(txCtx context.Context) error {
//	    // All operations here use the same transaction
//	    if err := userRepo.Create(txCtx, user); err != nil {
//	        return err // Triggers rollback
//	    }
//	    if err := profileRepo.Create(txCtx, profile); err != nil {
//	        return err // Triggers rollback
//	    }
//	    return nil // Triggers commit
//	})
type TransactionManager interface {
	// WithTransaction executes the provided function within a database transaction.
	// If the function returns an error, the transaction is rolled back.
	// If the function returns nil, the transaction is committed.
	//
	// If a transaction already exists in the context, it is reused (nested transactions).
	WithTransaction(ctx context.Context, fn func(ctx context.Context) error) error

	// WithTransactionOpts is like WithTransaction but allows custom transaction options.
	// The opts parameter can specify isolation level, read-only mode, etc.
	WithTransactionOpts(ctx context.Context, opts *sql.TxOptions, fn func(ctx context.Context) error) error
}

type transactionManager struct {
	db *gorm.DB
}

// NewTransactionManager creates a new transaction manager with the provided database connection.
//
// Example:
//
//	tm := repository.NewTransactionManager(db)
func NewTransactionManager(db *gorm.DB) TransactionManager {
	return &transactionManager{db: db}
}

// WithTransaction executes fn within a database transaction using default options.
// See TransactionManager.WithTransaction for details.
func (tm *transactionManager) WithTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	return tm.WithTransactionOpts(ctx, nil, fn)
}

// WithTransactionOpts executes fn within a database transaction with custom options.
//
// If a transaction already exists in the context (from a parent WithTransaction call),
// this reuses the existing transaction. This allows nested transaction calls to participate
// in the same database transaction, preventing deadlocks and ensuring atomicity.
//
// Transaction handling:
//   - If fn returns nil, the transaction is committed
//   - If fn returns an error, the transaction is rolled back
//   - If fn panics, the transaction is rolled back
//
// Example with custom isolation level:
//
//	opts := &sql.TxOptions{
//	    Isolation: sql.LevelReadCommitted,
//	    ReadOnly:  false,
//	}
//	err := tm.WithTransactionOpts(ctx, opts, func(txCtx context.Context) error {
//	    // Operations here use the specified isolation level
//	    return userRepo.Create(txCtx, user)
//	})
func (tm *transactionManager) WithTransactionOpts(ctx context.Context, opts *sql.TxOptions, fn func(ctx context.Context) error) error {
	// If there's already a transaction in context, reuse it (nested transactions)
	if _, ok := ctx.Value(txKey).(*gorm.DB); ok {
		return fn(ctx)
	}

	return tm.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, txKey, tx)
		return fn(txCtx)
	}, opts)
}
