//go:build !ee

package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ragflow/internal/common"

	"github.com/gin-gonic/gin"
)

// TestHandleNoRouteMatchesPythonEmptyOAuthChannel pins the community edition:
// with no OAuth routes, GET /api/v1/auth/login/ reaches NoRoute and has to
// reproduce the Python MethodNotAllowed body. The enterprise build supplies
// its own isOAuthLoginNoRoute, so this assertion is community-only.
func TestHandleNoRouteMatchesPythonEmptyOAuthChannel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/login/", nil)

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
