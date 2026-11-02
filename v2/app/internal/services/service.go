// Package services holds the application's business logic.
package services

import (
	"github.com/imlargo/medusa/app/internal/config"
	"github.com/imlargo/medusa/app/internal/store"
)

// Service is what every service shares: the repositories and the
// configuration.
type Service struct {
	store  *store.Store
	config *config.Config
}

// NewService bundles the shared dependencies.
func NewService(store *store.Store, config *config.Config) *Service {
	return &Service{store: store, config: config}
}
