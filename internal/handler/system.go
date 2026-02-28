package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"ragflow/internal/service"
)

// SystemHandler system handler
type SystemHandler struct {
	systemService *service.SystemService
}

// NewSystemHandler create system handler
func NewSystemHandler(systemService *service.SystemService) *SystemHandler {
	return &SystemHandler{
		systemService: systemService,
	}
}

// Ping health check endpoint
// @Summary Ping
// @Description Simple ping endpoint
// @Tags system
// @Produce plain
// @Success 200 {string} string "pong"
// @Router /v1/system/ping [get]
func (h *SystemHandler) Ping(c *gin.Context) {
	c.String(http.StatusOK, "pong")
}

// GetConfig get system configuration
// @Summary Get System Configuration
// @Description Get system configuration including register enabled status
// @Tags system
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /v1/system/config [get]
func (h *SystemHandler) GetConfig(c *gin.Context) {
	config, err := h.systemService.GetConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "Failed to get system configuration",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": config,
	})
}
