package router

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
)

func TestRequestBodyLimitMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("MAX_JSON_CONTENT_LENGTH", "")
	t.Setenv("MAX_MULTIPART_CONTENT_LENGTH", "")
	t.Setenv("MAX_CONTENT_LENGTH", "")

	engine := gin.New()
	engine.Use(requestBodyLimitMiddleware())
	readBody := func(c *gin.Context) {
		_, err := io.Copy(io.Discard, c.Request.Body)
		if err != nil {
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				c.Status(http.StatusRequestEntityTooLarge)
				return
			}
			c.Status(http.StatusBadRequest)
			return
		}
		c.Status(http.StatusNoContent)
	}
	engine.POST("/api/v1/test", readBody)
	engine.POST("/api/v1/files", readBody)
	engine.DELETE("/api/v1/files", readBody)
	engine.POST("/api/v1/agents/:canvas_id/upload", readBody)
	engine.POST("/api/v1/chat/audio/transcription", readBody)
	engine.POST("/api/v1/documents/upload", readBody)
	engine.POST("/api/v1/datasets/:dataset_id/documents", readBody)
	engine.POST(agentWebhookRoute, readBody)

	t.Run("rejects oversized default request", func(t *testing.T) {
		body := bytes.Repeat([]byte("x"), int(common.DefaultMaxJSONRequestBodyBytes+1))
		req := httptest.NewRequest(http.MethodPost, "/api/v1/test", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		engine.ServeHTTP(resp, req)
		if resp.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("status=%d, want %d", resp.Code, http.StatusRequestEntityTooLarge)
		}
	})

	t.Run("bounds chunked request body", func(t *testing.T) {
		body := bytes.Repeat([]byte("x"), int(common.DefaultMaxJSONRequestBodyBytes+1))
		req := httptest.NewRequest(http.MethodPost, "/api/v1/test", bytes.NewReader(body))
		req.ContentLength = -1
		req.TransferEncoding = []string{"chunked"}
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		engine.ServeHTTP(resp, req)
		if resp.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("status=%d, want %d", resp.Code, http.StatusRequestEntityTooLarge)
		}
	})

	t.Run("allows webhook body up to 10 MB", func(t *testing.T) {
		body := bytes.Repeat([]byte("x"), int(common.DefaultMaxJSONRequestBodyBytes+1))
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/a1/webhook", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		engine.ServeHTTP(resp, req)
		if resp.Code != http.StatusNoContent {
			t.Fatalf("status=%d, want %d", resp.Code, http.StatusNoContent)
		}
	})

	t.Run("rejects webhook body above 10 MB", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/a1/webhook", bytes.NewReader(nil))
		req.ContentLength = common.MaxWebhookRequestBodyBytes + 1
		resp := httptest.NewRecorder()
		engine.ServeHTTP(resp, req)
		if resp.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("status=%d, want %d", resp.Code, http.StatusRequestEntityTooLarge)
		}
	})

	t.Run("does not exempt non-upload methods on file routes", func(t *testing.T) {
		body := bytes.Repeat([]byte("x"), int(common.DefaultMaxJSONRequestBodyBytes+1))
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/files", bytes.NewReader(body))
		req.Header.Set("Content-Type", "multipart/form-data; boundary=test")
		resp := httptest.NewRecorder()
		engine.ServeHTTP(resp, req)
		if resp.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("status=%d, want %d", resp.Code, http.StatusRequestEntityTooLarge)
		}
	})

	t.Run("uses configured JSON request-body limit", func(t *testing.T) {
		t.Setenv("MAX_JSON_CONTENT_LENGTH", "16")
		body := bytes.Repeat([]byte("x"), 17)
		request := httptest.NewRequest(http.MethodPost, "/api/v1/test", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		if response.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("status=%d, want %d", response.Code, http.StatusRequestEntityTooLarge)
		}
	})

	t.Run("uses configured multipart limit for every upload route", func(t *testing.T) {
		t.Setenv("MAX_JSON_CONTENT_LENGTH", "8")
		t.Setenv("MAX_MULTIPART_CONTENT_LENGTH", "16")
		paths := []string{
			"/api/v1/files",
			"/api/v1/agents/a1/upload",
			"/api/v1/chat/audio/transcription",
			"/api/v1/documents/upload",
			"/api/v1/datasets/d1/documents",
		}
		for _, path := range paths {
			request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(bytes.Repeat([]byte("x"), 12)))
			request.Header.Set("Content-Type", "multipart/form-data; boundary=test")
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			if response.Code != http.StatusNoContent {
				t.Errorf("POST %s status=%d, want %d for body above JSON limit", path, response.Code, http.StatusNoContent)
			}

			request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(bytes.Repeat([]byte("x"), 17)))
			request.Header.Set("Content-Type", "multipart/form-data; boundary=test")
			response = httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			if response.Code != http.StatusRequestEntityTooLarge {
				t.Errorf("POST %s status=%d, want %d for body above multipart limit", path, response.Code, http.StatusRequestEntityTooLarge)
			}
		}
	})
}
