// Package router owns the API request-size policy.
package router

import (
	"mime"
	"net/http"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
)

const agentWebhookRoute = "/api/v1/agents/:canvas_id/webhook"

// requestBodyLimitMiddleware applies configurable limits to API request bodies.
// Multipart upload routes use a shared limit; handlers may enforce lower
// endpoint-specific limits. Public Agent webhooks retain their 10 MB ceiling.
func requestBodyLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body == nil || c.Request.Body == http.NoBody {
			c.Next()
			return
		}

		limit := common.GetMaxJSONRequestBodyBytes()
		fullPath := c.FullPath()
		switch {
		case fullPath == agentWebhookRoute:
			limit = common.MaxWebhookRequestBodyBytes
		case isMultipartUploadRequest(c):
			limit = common.GetMaxMultipartRequestBodyBytes()
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

func isMultipartUploadRequest(c *gin.Context) bool {
	if c.Request.Method != http.MethodPost || !isMultipartFormData(c.Request.Header.Get("Content-Type")) {
		return false
	}

	switch c.FullPath() {
	case "/api/v1/files",
		"/api/v1/agents/:canvas_id/upload",
		"/api/v1/chat/audio/transcription",
		"/api/v1/documents/upload",
		"/api/v1/datasets/:dataset_id/documents":
		return true
	default:
		return false
	}
}

func isMultipartFormData(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	return err == nil && mediaType == "multipart/form-data"
}
