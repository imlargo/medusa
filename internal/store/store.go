package store

import (
	"github.com/imlargo/go-api/internal/repositories"
	medusarepo "github.com/imlargo/go-api/pkg/medusa/core/repository"
)

type Store struct {
	*medusarepo.Store
	Users repositories.UserRepository
}

func NewStore(store *medusarepo.Store) *Store {
	return &Store{
		Store: store,
		Users: repositories.NewUserRepository(store.BaseRepo),
	}
}
