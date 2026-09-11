package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

func TestIsMemoryServiceNotFound(t *testing.T) {
	notFoundErr := &service.ResourceNotFoundError{Resource: "Memory", ID: "memory-1"}
	if !isMemoryServiceNotFound(fmt.Errorf("wrapped: %w", notFoundErr)) {
		t.Fatal("expected wrapped service not found error to map to not found")
	}
	messageNotFoundErr := &service.ResourceNotFoundError{Resource: "Message", ID: "message-1"}
	if isMemoryServiceNotFound(messageNotFoundErr) {
		t.Fatal("expected non-memory resource not found error to avoid memory 404 mapping")
	}
	if isMemoryServiceNotFound(fmt.Errorf("backend index does not exist")) {
		t.Fatal("backend text should not map to not found without service error type")
	}
}

func TestParseMemoryMessagePath(t *testing.T) {
	tests := []struct {
		name          string
		value         string
		wantMemoryID  string
		wantMessageID int64
		wantErr       bool
	}{
		{name: "valid", value: "memory-1:42", wantMemoryID: "memory-1", wantMessageID: 42},
		{name: "empty", value: "", wantErr: true},
		{name: "missing message id", value: "memory-1:", wantErr: true},
		{name: "missing memory id", value: ":42", wantErr: true},
		{name: "invalid message id", value: "memory-1:not-int", wantErr: true},
		{name: "negative message id", value: "memory-1:-1", wantErr: true},
		{name: "too many separators", value: "memory-1:2:3", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			memoryID, messageID, err := parseMemoryMessagePath(tt.value)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if memoryID != tt.wantMemoryID || messageID != tt.wantMessageID {
				t.Fatalf("got (%q, %d), want (%q, %d)", memoryID, messageID, tt.wantMemoryID, tt.wantMessageID)
			}
		})
	}
}

func TestForgetMessageRejectsMalformedPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h := NewMemoryHandler(service.NewMemoryService())
	router.DELETE("/api/v1/messages/:memory_message", func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "user-1"})
		h.ForgetMessage(c)
	})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/messages/memory-1:not-int", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var res map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if code := common.ErrorCode(res["code"].(float64)); code != common.CodeArgumentError {
		t.Fatalf("code = %v, want %v; body=%s", code, common.CodeArgumentError, w.Body.String())
	}
}
