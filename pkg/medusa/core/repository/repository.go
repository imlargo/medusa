// Package repository provides a clean repository pattern implementation with GORM.
//
// This package implements the Repository pattern to abstract database operations,
// providing a clean separation between the data access layer and business logic.
// It supports CRUD operations, transactions, and context-aware database connections.
//
// Key Features:
//   - Generic CRUD operations with type safety
//   - Transaction management with context propagation
//   - Automatic transaction reuse for nested operations
//   - Context-aware database connections
//   - Thread-safe operations
//
// Example usage:
//
//	// Create a store with base repository and transaction manager
//	store := repository.NewStore(db, logger)
//
//	// Use CRUD operations
//	userRepo := repository.NewCRUDRepository[User](store.BaseRepo)
//	user := &User{Name: "John", Email: "john@example.com"}
//	if err := userRepo.Create(ctx, user); err != nil {
//	    return err
//	}
//
//	// Use transactions
//	err := store.Transaction.WithTransaction(ctx, func(txCtx context.Context) error {
//	    // All operations in this function use the same transaction
//	    if err := userRepo.Create(txCtx, user1); err != nil {
//	        return err // Transaction will be rolled back
//	    }
//	    if err := userRepo.Create(txCtx, user2); err != nil {
//	        return err // Transaction will be rolled back
//	    }
//	    return nil // Transaction will be committed
//	})
package repository

import (
	"context"

	"github.com/imlargo/medusa/pkg/medusa/core/logger"
	"gorm.io/gorm"
)

type ctxKey string

const txKey ctxKey = "tx"

// Repository is the base repository that provides database access with transaction support.
// It automatically uses a transaction if one exists in the context, otherwise uses the base connection.
//
// This type should be embedded in your custom repositories to inherit the DB() method.
type Repository struct {
	logger *logger.Logger
	db     *gorm.DB
}

// NewRepository creates a new base repository with the provided database connection and logger.
// Options can be provided to customize the repository behavior.
//
// Example:
//
//	repo := repository.NewRepository(db, logger)
func NewRepository(db *gorm.DB, logger *logger.Logger, opts ...RepositoryOption) *Repository {
	r := &Repository{
		db:     db,
		logger: logger,
	}

	for _, opt := range opts {
		opt(r)
	}

	return r
}

// DB returns the appropriate database connection for the current context.
// If a transaction exists in the context (created via TransactionManager.WithTransaction),
// it returns the transaction connection. Otherwise, it returns the base connection.
//
// This method is used internally by repository operations to ensure they participate
// in active transactions when present.
//
// Example:
//
//	func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*User, error) {
//	    var user User
//	    err := r.DB(ctx).Where("email = ?", email).First(&user).Error
//	    return &user, err
//	}
func (r *Repository) DB(ctx context.Context) *gorm.DB {
	if tx, ok := ctx.Value(txKey).(*gorm.DB); ok {
		return tx
	}
	return r.db.WithContext(ctx)
}
