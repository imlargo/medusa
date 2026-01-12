package repository

import (
	"context"

	"github.com/imlargo/go-api/internal/models"
	medusarepo "github.com/imlargo/go-api/pkg/medusa/core/repository"
)

type UserRepository interface {
	GetByID(ctx context.Context, id uint) (*models.User, error)
	GetByEmail(ctx context.Context, email string) (*models.User, error)
	Create(ctx context.Context, user *models.User) error
	Delete(ctx context.Context, id uint) error
}

type userRepository struct {
	*medusarepo.Repository
}

func NewUserRepository(repo *medusarepo.Repository) UserRepository {
	return &userRepository{Repository: repo}
}

func (r *userRepository) GetByID(ctx context.Context, id uint) (*models.User, error) {
	var user models.User
	if err := r.DB(ctx).First(&user, id).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *userRepository) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	var user models.User
	if err := r.DB(ctx).Where("email = ?", email).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *userRepository) Create(ctx context.Context, user *models.User) error {
	return r.DB(ctx).Create(user).Error
}

func (r *userRepository) Delete(ctx context.Context, id uint) error {
	return r.DB(ctx).Delete(&models.User{}, id).Error
}
