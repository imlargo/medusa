package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/imlargo/go-api/pkg/medusa/core/responses"
)

type HealthHandler struct {
	*Handler
}

func NewHealthHandler(handler *Handler) *HealthHandler {
	return &HealthHandler{
		Handler: handler,
	}
}

func (a *HealthHandler) Health(c *gin.Context) {
	responses.SuccessOK(c, "ok")
}
