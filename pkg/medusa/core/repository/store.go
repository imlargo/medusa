package repository

import (
	"github.com/imlargo/medusa/pkg/medusa/core/logger"
	"gorm.io/gorm"
)

// Store is a convenient container that bundles a base repository and transaction manager.
// It provides a single point of access for database operations, making it easy to
// pass both capabilities to your service layer.
//
// The Store pattern is useful when you want to:
//   - Share a single database connection across multiple repositories
//   - Provide transaction management alongside repository operations
//   - Simplify dependency injection in your services
//
// Example:
//
//	store := repository.NewStore(db, logger)
//
//	// Use in a service
//	type UserService struct {
//	    store *repository.Store
//	    repo  *repository.CRUDRepository[User]
//	}
//
//	func NewUserService(store *repository.Store) *UserService {
//	    return &UserService{
//	        store: store,
//	        repo:  repository.NewCRUDRepository[User](store.BaseRepo),
//	    }
//	}
//
//	func (s *UserService) CreateUserWithProfile(ctx context.Context, user *User, profile *Profile) error {
//	    return s.store.Transaction.WithTransaction(ctx, func(txCtx context.Context) error {
//	        if err := s.repo.Create(txCtx, user); err != nil {
//	            return err
//	        }
//	        profile.UserID = user.ID
//	        return s.profileRepo.Create(txCtx, profile)
//	    })
//	}
type Store struct {
	// BaseRepo is the base repository that can be used to create type-specific repositories.
	BaseRepo *Repository

	// Transaction provides transaction management capabilities.
	Transaction TransactionManager
}

// NewStore creates a new Store with a base repository and transaction manager.
// The store shares the same database connection and logger across all operations.
//
// Example:
//
//	store := repository.NewStore(db, logger)
//	userRepo := repository.NewCRUDRepository[User](store.BaseRepo)
//	productRepo := repository.NewCRUDRepository[Product](store.BaseRepo)
func NewStore(db *gorm.DB, logger *logger.Logger) *Store {
	return &Store{
		BaseRepo:    NewRepository(db, logger),
		Transaction: NewTransactionManager(db),
	}
}
