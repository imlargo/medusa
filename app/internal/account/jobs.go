package account

import (
	"context"
	"log/slog"
)

// Kinds of job of this feature.
const (
	// KindWelcome is enqueued for every new account.
	KindWelcome = "account.welcome"

	// KindPurgeSessions runs daily and deletes expired refresh tokens.
	KindPurgeSessions = "account.purge_sessions"
)

// WelcomeJob is the payload of a KindWelcome job.
type WelcomeJob struct {
	UserID int64 `json:"user_id"`
}

// Welcome handles KindWelcome jobs. It is where a welcome email would be
// sent; until the app has mail, it records that the account was welcomed.
func Welcome(logger *slog.Logger) func(ctx context.Context, job WelcomeJob) error {
	return func(ctx context.Context, job WelcomeJob) error {
		logger.InfoContext(ctx, "account welcomed", slog.Int64("user_id", job.UserID))
		return nil
	}
}

// PurgeSessions handles KindPurgeSessions jobs.
func PurgeSessions(service *Service) func(ctx context.Context, _ struct{}) error {
	return func(ctx context.Context, _ struct{}) error {
		return service.PurgeExpiredSessions(ctx)
	}
}
