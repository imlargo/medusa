package repository

import (
	"context"
)

type WithCrud[T any] interface {
	Create(ctx context.Context, entity *T) error
	Get(ctx context.Context, id interface{}) (*T, error)
	FindAll(ctx context.Context) ([]T, error)
	Update(ctx context.Context, entity *T) error
	Delete(ctx context.Context, id interface{}) error
}

// CRUDRepository includes BaseRepository + CRUD operations
type CRUDRepository[T any] struct {
	*Repository
}

func NewCRUDRepository[T any](repository *Repository) *CRUDRepository[T] {
	return &CRUDRepository[T]{
		Repository: repository,
	}
}

// Create inserts a new entity
func (r *CRUDRepository[T]) Create(ctx context.Context, entity *T) error {
	return r.DB(ctx).Create(entity).Error
}

// Get finds an entity by its ID
func (r *CRUDRepository[T]) Get(ctx context.Context, id interface{}) (*T, error) {
	var entity T

	err := r.DB(ctx).First(&entity, id).Error
	if err != nil {
		return nil, err
	}

	return &entity, nil
}

// FindAll retrieves all entities
func (r *CRUDRepository[T]) FindAll(ctx context.Context) ([]T, error) {
	var entities []T

	if err := r.DB(ctx).Find(&entities).Error; err != nil {
		return nil, err
	}

	return entities, nil
}

// Update updates an existing entity
func (r *CRUDRepository[T]) Update(ctx context.Context, entity *T) error {
	if err := r.DB(ctx).Save(entity).Error; err != nil {
		return err
	}

	return nil
}

// Delete removes an entity by its ID
func (r *CRUDRepository[T]) Delete(ctx context.Context, id interface{}) error {
	var entity T

	result := r.DB(ctx).Delete(&entity, id)
	if result.Error != nil {
		return result.Error
	}

	return nil
}
