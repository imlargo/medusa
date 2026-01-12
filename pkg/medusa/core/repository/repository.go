package repository

import (
	"context"

	"github.com/imlargo/medusa/pkg/medusa/core/logger"
	"gorm.io/gorm"
)

type ctxKey string

const txKey ctxKey = "tx"

type Repository struct {
	logger *logger.Logger
	db     *gorm.DB
}

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

// DB returns the appropriate connection (tx if exists in context, or normal db)
func (r *Repository) DB(ctx context.Context) *gorm.DB {
	if tx, ok := ctx.Value(txKey).(*gorm.DB); ok {
		return tx
	}
	return r.db.WithContext(ctx)
}
