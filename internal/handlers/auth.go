package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/imlargo/go-api/internal/dto"
	"github.com/imlargo/go-api/internal/services"
	"github.com/imlargo/go-api/pkg/medusa/core/handler"
	"github.com/imlargo/go-api/pkg/medusa/core/responses"
)

type AuthHandler struct {
	*handler.Handler
	authService services.AuthService
}

func NewAuthHandler(handler *handler.Handler, authService services.AuthService) *AuthHandler {
	return &AuthHandler{
		Handler:     handler,
		authService: authService,
	}
}

func (a *AuthHandler) LoginWithPassword(c *gin.Context) {
	var payload dto.LoginWithPassword
	if err := c.ShouldBindJSON(&payload); err != nil {
		responses.ErrorBadRequest(c, "Invalid request payload")
		return
	}

	authResponse, err := a.authService.LoginWithPassword(payload.Email, payload.Password)
	if err != nil {
		responses.ErrorInternalServerWithMessage(c, err.Error(), nil)
		return
	}

	responses.SuccessOK(c, authResponse)
}

func (a *AuthHandler) Register(c *gin.Context) {
	var payload dto.RegisterUser
	if err := c.ShouldBindJSON(&payload); err != nil {
		responses.ErrorBadRequest(c, "Invalid request payload")
		return
	}

	authData, err := a.authService.RegisterWithPassword(&payload)
	if err != nil {
		responses.ErrorInternalServerWithMessage(c, err.Error(), nil)
		return
	}

	responses.SuccessOK(c, authData)
}

func (a *AuthHandler) GetUser(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		responses.ErrorUnauthorized(c, "User not authenticated")
		return
	}

	user, err := a.authService.GetUser(userID.(uint))
	if err != nil {
		responses.ErrorInternalServerWithMessage(c, err.Error(), nil)
		return
	}

	responses.SuccessOK(c, user)
}
