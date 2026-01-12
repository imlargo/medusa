package handlers

import (
	"github.com/imlargo/go-api/internal/service"
	"github.com/imlargo/go-api/pkg/medusa/core/handler"
)

type AuthHandler struct {
	*handler.Handler
	authService service.AuthService
}

func NewAuthHandler(h *handler.Handler, authService service.AuthService) *AuthHandler {
	return &AuthHandler{
		Handler:     h,
		authService: authService,
	}
}

func (h *AuthHandler) RegisterWithPassword(id uint) {

}
