package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/service"

	"github.com/gin-gonic/gin"
)

func TestHandleNoRouteMatchesPythonMethodNotAllowed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/v1/chats//sessions", nil)

	HandleNoRoute(context)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d", recorder.Code, http.StatusOK)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["code"] != float64(common.CodeExceptionError) || body["message"] != "<MethodNotAllowed '405: Method Not Allowed'>" {
		t.Fatalf("body=%v", body)
	}
}

func TestHandleNoRouteMatchesPythonEmptySessionUpdate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPatch, "/api/v1/chats/chat-1/sessions/", nil)

	HandleNoRoute(context)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d", recorder.Code, http.StatusOK)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["code"] != float64(common.CodeExceptionError) || body["message"] != "<MethodNotAllowed '405: Method Not Allowed'>" {
		t.Fatalf("body=%v", body)
	}
}

func TestIngestionTaskErrorCode(t *testing.T) {
	testCases := []struct {
		name string
		err  error
		want common.ErrorCode
	}{
		{
			name: "invalid transition",
			err:  &service.InvalidTaskTransitionError{TaskID: "task-1", From: common.CREATED, To: common.COMPLETED},
			want: common.CodeConflict,
		},
		{
			name: "status conflict",
			err:  &service.TaskStatusConflictError{TaskID: "task-1", ExpectedFrom: common.CREATED, AttemptedTo: common.RUNNING, ActualCurrent: common.STOPPING},
			want: common.CodeConflict,
		},
		{
			name: "task not found",
			err:  common.ErrTaskNotFound,
			want: common.CodeNotFound,
		},
		{
			name: "fallback",
			err:  common.ErrInvalidToken,
			want: common.CodeExceptionError,
		},
	}

	for _, tc := range testCases {
		if got := IngestionTaskErrorCode(tc.err); got != tc.want {
			t.Fatalf("%s: got %d, want %d", tc.name, got, tc.want)
		}
	}
}
