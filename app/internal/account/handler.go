package account

import (
	"context"

	"github.com/imlargo/medusa"
	"github.com/imlargo/medusa/api"
	"github.com/imlargo/medusa/auth"

	"github.com/imlargo/medusa/app/internal/user"
)

// Credentials are an email and a password, as login and registration take
// them.
type Credentials struct {
	Email    string `json:"email" format:"email" doc:"The account's email, in any letter case"`
	Password string `json:"password" minLength:"1" doc:"The account's password"`
}

// LoginInput is the input of a password login.
type LoginInput struct {
	Body Credentials
}

// RegisterInput is the input of a registration.
type RegisterInput struct {
	Body Credentials
}

// RefreshInput is the input of a token refresh and of a logout.
type RefreshInput struct {
	Body struct {
		RefreshToken string `json:"refresh_token" minLength:"1" doc:"The refresh token of the session"`
	}
}

// Handler serves the account routes. Each method takes its typed input and
// returns the resource or an error; the api battery binds, validates,
// documents and renders them.
type Handler struct {
	service *Service
}

// NewHandler serves the account routes through service.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Login exchanges credentials for a session.
func (h *Handler) Login(ctx context.Context, in *LoginInput) (*Session, error) {
	return h.service.Login(ctx, in.Body.Email, in.Body.Password)
}

// Register creates an account and signs it in.
func (h *Handler) Register(ctx context.Context, in *RegisterInput) (*Session, error) {
	return h.service.Register(ctx, in.Body.Email, in.Body.Password)
}

// Refresh trades a refresh token for new tokens.
func (h *Handler) Refresh(ctx context.Context, in *RefreshInput) (*Tokens, error) {
	return h.service.Refresh(ctx, in.Body.RefreshToken)
}

// Logout ends the session a refresh token belongs to.
func (h *Handler) Logout(ctx context.Context, in *RefreshInput) (*api.Empty, error) {
	return nil, h.service.Logout(ctx, in.Body.RefreshToken)
}

// Me returns the signed-in user.
func (h *Handler) Me(ctx context.Context, _ *struct{}) (*user.User, error) {
	userID, ok := auth.SubjectFrom[int64](ctx)
	if !ok {
		// Only reachable if the route is registered without auth.Middleware.
		return nil, medusa.ErrMissingCredentials
	}

	return h.service.Me(ctx, userID)
}
