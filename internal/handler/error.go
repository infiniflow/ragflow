package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"ragflow/internal/logger"
)

// HandleNoRoute handles requests to undefined routes
func HandleNoRoute(c *gin.Context) {
	// Log the request details on server side
	logger.Logger.Warn("The requested URL was not found",
		zap.String("method", c.Request.Method),
		zap.String("path", c.Request.URL.Path),
		zap.String("query", c.Request.URL.RawQuery),
		zap.String("remote_addr", c.ClientIP()),
		zap.String("user_agent", c.Request.UserAgent()),
	)

	// Return JSON error response
	c.JSON(http.StatusNotFound, gin.H{
		"code":    404,
		"message": "Not Found: " + c.Request.URL.Path,
		"data":    nil,
		"error":   "Not Found",
	})
}
