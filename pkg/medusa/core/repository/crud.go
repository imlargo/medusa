package repository

import (
	"context"

	"gorm.io/gorm"
)

type CrudInteface[T any] interface {
	Create(ctx context.Context, entity *T) error
	Get(ctx context.Context, id interface{}) (*T, error)
	FindAll(ctx context.Context) ([]T, error)
	FindWhere(ctx context.Context, conditions map[string]interface{}) ([]T, error)
	FindOne(ctx context.Context, conditions map[string]interface{}) (*T, error)
	Update(ctx context.Context, entity *T) error
	UpdateFields(ctx context.Context, id interface{}, fields map[string]interface{}) error
	Delete(ctx context.Context, id interface{}) error
	SoftDelete(ctx context.Context, id interface{}) error
	Count(ctx context.Context) (int64, error)
	Exists(ctx context.Context, conditions map[string]interface{}) (bool, error)
	Paginate(ctx context.Context, page, pageSize int) ([]T, int64, error)
}

// CRUDRepository incluye BaseRepository + operaciones CRUD
type CRUDRepository[T any] struct {
	*Repository
}

func NewCRUDRepository[T any](repository *Repository) *CRUDRepository[T] {
	return &CRUDRepository[T]{
		Repository: repository,
	}
}

// Create inserta una nueva entidad
func (r *CRUDRepository[T]) Create(ctx context.Context, entity *T) error {
	return r.DB(ctx).Create(entity).Error
}

// Get busca una entidad por su ID
func (r *CRUDRepository[T]) Get(ctx context.Context, id interface{}) (*T, error) {
	var entity T

	err := r.DB(ctx).First(&entity, id).Error
	if err != nil {
		return nil, err
	}

	return &entity, nil
}

// FindAll obtiene todas las entidades
func (r *CRUDRepository[T]) FindAll(ctx context.Context) ([]T, error) {
	var entities []T

	if err := r.DB(ctx).Find(&entities).Error; err != nil {
		return nil, err
	}

	return entities, nil
}

// FindWhere busca entidades con condiciones personalizadas
func (r *CRUDRepository[T]) FindWhere(ctx context.Context, conditions map[string]interface{}) ([]T, error) {
	var entities []T

	query := r.DB(ctx)
	for key, value := range conditions {
		query = query.Where(key+" = ?", value)
	}

	if err := query.Find(&entities).Error; err != nil {
		return nil, err
	}

	return entities, nil
}

// FindOne busca una sola entidad con condiciones personalizadas
func (r *CRUDRepository[T]) FindOne(ctx context.Context, conditions map[string]interface{}) (*T, error) {
	var entity T

	query := r.DB(ctx)
	for key, value := range conditions {
		query = query.Where(key+" = ?", value)
	}

	err := query.First(&entity).Error
	if err != nil {
		return nil, err
	}

	return &entity, nil
}

// Update actualiza una entidad existente
func (r *CRUDRepository[T]) Update(ctx context.Context, entity *T) error {
	if err := r.DB(ctx).Save(entity).Error; err != nil {
		return err
	}

	return nil
}

// UpdateFields actualiza campos específicos de una entidad
func (r *CRUDRepository[T]) UpdateFields(ctx context.Context, id interface{}, fields map[string]interface{}) error {
	var entity T

	result := r.DB(ctx).Model(&entity).Where("id = ?", id).Updates(fields)
	if result.Error != nil {
		return result.Error
	}

	return nil
}

// Delete elimina una entidad por su ID
func (r *CRUDRepository[T]) Delete(ctx context.Context, id interface{}) error {
	var entity T

	result := r.DB(ctx).Delete(&entity, id)
	if result.Error != nil {
		return result.Error
	}

	return nil
}

// SoftDelete realiza un borrado lógico (si la entidad tiene DeletedAt)
func (r *CRUDRepository[T]) SoftDelete(ctx context.Context, id interface{}) error {
	var entity T

	result := r.DB(ctx).Model(&entity).Where("id = ?", id).Update("deleted_at", gorm.Expr("NOW()"))
	if result.Error != nil {
		return result.Error
	}

	return nil
}

// Count cuenta el número total de entidades
func (r *CRUDRepository[T]) Count(ctx context.Context) (int64, error) {
	var count int64
	var entity T

	if err := r.DB(ctx).Model(&entity).Count(&count).Error; err != nil {
		return 0, err
	}

	return count, nil
}

// Exists verifica si existe una entidad con las condiciones dadas
func (r *CRUDRepository[T]) Exists(ctx context.Context, conditions map[string]interface{}) (bool, error) {
	var count int64
	var entity T

	query := r.DB(ctx).Model(&entity)
	for key, value := range conditions {
		query = query.Where(key+" = ?", value)
	}

	if err := query.Count(&count).Error; err != nil {
		return false, err
	}

	return count > 0, nil
}

// Paginate devuelve resultados paginados
func (r *CRUDRepository[T]) Paginate(ctx context.Context, page, pageSize int) ([]T, int64, error) {
	var entities []T
	var total int64
	var entity T

	if err := r.DB(ctx).Model(&entity).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := r.DB(ctx).Limit(pageSize).Offset(offset).Find(&entities).Error; err != nil {
		return nil, 0, err
	}

	return entities, total, nil
}
