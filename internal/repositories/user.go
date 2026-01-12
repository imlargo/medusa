package repositories

import (
	"context"

	"github.com/imlargo/medusa/internal/models"
	"github.com/imlargo/medusa/pkg/medusa/core/repository"
)

type UserRepository interface {
	repository.WithCrud[models.User]
	GetByEmail(ctx context.Context, email string) (*models.User, error)
}

type userRepository struct {
	*repository.CRUDRepository[models.User]
}

func NewUserRepository(repo *repository.Repository) UserRepository {
	return &userRepository{CRUDRepository: repository.NewCRUDRepository[models.User](repo)}
}

func (r *userRepository) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	var user models.User
	if err := r.DB(ctx).Where("email = ?", email).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}
