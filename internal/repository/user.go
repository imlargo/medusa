package repository

import (
	"context"

	"github.com/imlargo/go-api/internal/models"
	medusarepo "github.com/imlargo/go-api/pkg/medusa/core/repository"
)

type UserRepository interface {
	medusarepo.WithCrud[models.User]
	GetByEmail(ctx context.Context, email string) (*models.User, error)
}

type userRepository struct {
	*medusarepo.CRUDRepository[models.User]
}

func NewUserRepository(repo *medusarepo.Repository) UserRepository {
	return &userRepository{CRUDRepository: medusarepo.NewCRUDRepository[models.User](repo)}
}

func (r *userRepository) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	var user models.User
	if err := r.DB(ctx).Where("email = ?", email).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}
