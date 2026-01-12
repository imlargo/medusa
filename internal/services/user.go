package services

import (
	"context"
	"errors"
	"strings"

	"github.com/imlargo/medusa/internal/dto"
	"github.com/imlargo/medusa/internal/models"
	"golang.org/x/crypto/bcrypt"
)

type UserService interface {
	CreateUser(user *dto.RegisterUser) (*models.User, error)
	DeleteUser(userID uint) error
	GetUserByID(userID uint) (*models.User, error)
	GetUserByEmail(email string) (*models.User, error)
}

type userServiceImpl struct {
	*Service
}

func NewUserService(container *Service) UserService {
	return &userServiceImpl{
		Service: container,
	}
}

func (s *userServiceImpl) CreateUser(registerUser *dto.RegisterUser) (*models.User, error) {

	registerUser.Email = strings.ToLower(registerUser.Email)

	// Validate user data
	user := &models.User{
		Email:    registerUser.Email,
		Password: registerUser.Password,
	}

	existingUser, _ := s.store.Users.GetByEmail(context.Background(), user.Email)
	if existingUser != nil {
		return nil, errors.New("user with this email already exists")
	}

	// Hash the password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, errors.New("failed to hash password")
	}
	user.Password = string(hashedPassword)

	if err := s.store.Users.Create(context.Background(), user); err != nil {
		return nil, err
	}

	return user, nil
}

func (s *userServiceImpl) DeleteUser(userID uint) error {
	return nil
}

func (s *userServiceImpl) GetUserByID(userID uint) (*models.User, error) {
	return s.store.Users.Get(context.Background(), userID)
}

func (s *userServiceImpl) GetUserByEmail(email string) (*models.User, error) {
	return s.store.Users.GetByEmail(context.Background(), email)
}
