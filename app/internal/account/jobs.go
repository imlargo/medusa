package account

import (
	"context"
	"log/slog"

	"github.com/imlargo/medusa/postgres/events"
)

// EventRegistered is published when an account is created.
const EventRegistered = "account.registered"

// Registered is the data of an EventRegistered event.
type Registered struct {
	UserID int64 `json:"user_id"`
}

// Names of this feature's jobs and subscribers.
const (
	// SubscriberWelcome reacts to EventRegistered.
	SubscriberWelcome = "account.welcome"

	// KindPurgeSessions runs daily and deletes expired refresh tokens.
	KindPurgeSessions = "account.purge_sessions"
)

// Welcome reacts to a new account. It is where a welcome email would be
// sent; until the app has mail, it records that the account was welcomed.
func Welcome(logger *slog.Logger) func(ctx context.Context, event events.Event[Registered]) error {
	return func(ctx context.Context, event events.Event[Registered]) error {
		logger.InfoContext(ctx, "account welcomed", slog.Int64("user_id", event.Data.UserID), slog.String("event_id", event.ID))
		return nil
	}
}

// PurgeSessions handles KindPurgeSessions jobs.
func PurgeSessions(service *Service) func(ctx context.Context, _ struct{}) error {
	return func(ctx context.Context, _ struct{}) error {
		return service.PurgeExpiredSessions(ctx)
	}
}
