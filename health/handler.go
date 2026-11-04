package health

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/imlargo/medusa"
)

// retryAfter is what an unready instance tells the prober to wait.
const retryAfter = 10 * time.Second

// Handler serves the probe endpoints.
type Handler struct {
	service *Service
}

// NewHandler serves probes backed by service.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Live reports that the process is up. It never consults a dependency: a
// failing liveness probe restarts the container, and restarting does not fix
// a database outage.
func (h *Handler) Live(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":     StatusHealthy,
		"request_id": medusa.NewContext(c).RequestID(),
	})
}

// Ready runs every check and replies 503 with Retry-After when any fails.
// The body names the failing checks; why they failed goes to Gin's error list,
// which the access log records.
func (h *Handler) Ready(c *gin.Context) {
	report := h.service.Check(c.Request.Context())
	for _, result := range report.Checks {
		if result.Err != nil {
			_ = c.Error(fmt.Errorf("health check %s: %w", result.Name, result.Err))
		}
	}

	body := gin.H{
		"status":     report.Status,
		"checks":     report.Checks,
		"request_id": medusa.NewContext(c).RequestID(),
	}

	if report.Status == StatusUnhealthy {
		c.Header("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
		c.JSON(http.StatusServiceUnavailable, body)
		return
	}

	c.JSON(http.StatusOK, body)
}
