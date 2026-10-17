package handler

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

func TestOceanBaseStatusRequiresAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("GET", "/api/v1/system/oceanbase/status", nil)

	NewSystemHandler(service.NewSystemService()).OceanBaseStatus(c)

	if recorder.Code != 200 {
		t.Fatalf("HTTP status=%d, want 200 for API error envelope", recorder.Code)
	}
	if recorder.Body.Len() == 0 {
		t.Fatal("expected authentication error response")
	}
	var response map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["code"] != float64(401) {
		t.Fatalf("code=%v, want 401", response["code"])
	}
}

func TestOceanBaseStatusReturnsNotInUseEnvelopeWithoutEngine(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("GET", "/api/v1/system/oceanbase/status", nil)
	c.Set("user", &entity.User{ID: "user-1"})
	c.Set("user_id", "user-1")

	NewSystemHandler(service.NewSystemService()).OceanBaseStatus(c)

	if recorder.Code != 200 {
		t.Fatalf("HTTP status=%d, want 200 for Python-compatible envelope", recorder.Code)
	}
	if recorder.Body.Len() == 0 {
		t.Fatal("expected not-in-use error response")
	}
	var response map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["code"] != float64(500) || response["message"] != "success" {
		t.Fatalf("response=%v, want Python error envelope", response)
	}
	data, ok := response["data"].(map[string]interface{})
	if !ok || data["status"] != "error" {
		t.Fatalf("data=%v, want status=error", response["data"])
	}
}
