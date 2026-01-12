package services

import (
	"github.com/imlargo/medusa/internal/config"
	"github.com/imlargo/medusa/internal/store"
	"github.com/imlargo/medusa/pkg/medusa/core/service"
)

type Service struct {
	*service.Service
	store  *store.Store
	config *config.Config
}

func NewService(
	medusa *service.Service,
	store *store.Store,
	config *config.Config,
) *Service {
	return &Service{
		medusa,
		store,
		config,
	}
}
