package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// SystemHandler system handler
type SystemHandler struct{}

// NewSystemHandler create system handler
func NewSystemHandler() *SystemHandler {
	return &SystemHandler{}
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
