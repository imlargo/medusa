package account

import (
	"github.com/imlargo/medusa"
	"github.com/imlargo/medusa/auth"

	"github.com/imlargo/medusa/app/internal/user"
)

// LoginRequest is the body of a password login.
type LoginRequest struct {
	Email    string `json:"email"    binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// RegisterRequest is the body of a registration.
type RegisterRequest struct {
	Email    string `json:"email"    binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// Handler serves the account endpoints. Each method returns its result and
// its error; the medusa.Handle adapters bind, validate and render them.
type Handler struct {
	service *Service
}

// NewHandler serves the account endpoints through service.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Login exchanges credentials for a session.
//
//	@Summary		Login user
//	@Router			/v1/auth/login [post]
//	@Description	Login with email and password
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			payload	body		account.LoginRequest							true	"Credentials"
//	@Success		200		{object}	medusa.SuccessResponse{data=account.Session}	"Logged in"
//	@Failure		400		{object}	httperr.Problem									"request.validation_failed"
//	@Failure		401		{object}	httperr.Problem									"account.invalid_credentials"
//	@Failure		500		{object}	httperr.Problem									"Internal Server Error"
func (h *Handler) Login(ctx *medusa.Context, in *LoginRequest) (*Session, error) {
	return h.service.Login(ctx.Ctx(), in.Email, in.Password)
}

// Register creates an account and returns a session for it.
//
//	@Summary		Register user
//	@Router			/v1/auth/register [post]
//	@Description	Register a new user with email and password
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			payload	body		account.RegisterRequest							true	"Registration payload"
//	@Success		201		{object}	medusa.SuccessResponse{data=account.Session}	"Registered"
//	@Failure		400		{object}	httperr.Problem									"request.validation_failed"
//	@Failure		409		{object}	httperr.Problem									"user.email_taken"
//	@Failure		500		{object}	httperr.Problem									"Internal Server Error"
func (h *Handler) Register(ctx *medusa.Context, in *RegisterRequest) (*Session, error) {
	return h.service.Register(ctx.Ctx(), in.Email, in.Password)
}

// Me returns the signed-in user.
//
//	@Summary		Get user info
//	@Router			/v1/auth/user [get]
//	@Description	Get the authenticated user's information
//	@Tags			auth
//	@Produce		json
//	@Success		200	{object}	medusa.SuccessResponse{data=user.User}	"The authenticated user"
//	@Failure		401	{object}	httperr.Problem							"auth.invalid_token"
//	@Failure		404	{object}	httperr.Problem							"user.not_found"
//	@Failure		500	{object}	httperr.Problem							"Internal Server Error"
//	@Security		BearerAuth
func (h *Handler) Me(ctx *medusa.Context) (*user.User, error) {
	userID, ok := auth.SubjectFrom[int64](ctx)
	if !ok {
		// Only reachable if the route is registered without auth.Middleware.
		return nil, medusa.ErrMissingCredentials
	}

	return h.service.Me(ctx.Ctx(), userID)
}
