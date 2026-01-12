package store

import (
	"github.com/imlargo/go-api/internal/repository"
	medusarepo "github.com/imlargo/go-api/pkg/medusa/core/repository"
)

type Store struct {
	*medusarepo.Store
	Users repository.UserRepository
}

func NewStore(store *medusarepo.Store) *Store {
	return &Store{
		Store: store,
		Users: repository.NewUserRepository(store.BaseRepo),
	}
}
