package repository

// RepositoryOption is a functional option for configuring a Repository.
// This allows for flexible and extensible repository configuration without
// breaking existing code when new options are added.
//
// Example custom option:
//
//	func WithCustomLogger(log *logger.Logger) RepositoryOption {
//	    return func(r *Repository) {
//	        r.logger = log
//	    }
//	}
//
//	repo := repository.NewRepository(db, logger, WithCustomLogger(customLog))
type RepositoryOption func(*Repository)
