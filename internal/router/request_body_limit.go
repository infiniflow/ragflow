// Package router owns the API request-size policy.
package router

import (
	"mime"
	"net/http"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
)

const agentWebhookRoute = "/api/v1/agents/:canvas_id/webhook"

// requestBodyLimitMiddleware applies a small default cap to API request bodies.
// Multipart uploads use their handler-specific limits, while public Agent
// webhooks use their own 10 MB default and maximum.
func requestBodyLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body == nil || c.Request.Body == http.NoBody {
			c.Next()
			return
		}

		limit := common.MaxJSONRequestBodyBytes
		fullPath := c.FullPath()
		switch fullPath {
		case agentWebhookRoute:
			limit = common.MaxWebhookRequestBodyBytes
		case "/api/v1/files", "/api/v1/agents/:canvas_id/upload", "/api/v1/chat/audio/transcription":
			if c.Request.Method == http.MethodPost && isMultipartFormData(c.Request.Header.Get("Content-Type")) {
				// These handlers enforce their own upload-specific limits.
				c.Next()
				return
			}
		}

		if c.Request.ContentLength > limit {
			common.ResponseWithHttpCodeData(c, http.StatusRequestEntityTooLarge, common.CodeBadRequest, nil, "request body too large")
			c.Abort()
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		c.Next()
	}
}

func isMultipartFormData(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	return err == nil && mediaType == "multipart/form-data"
}
