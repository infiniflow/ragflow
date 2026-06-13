package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

func TestSearchMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &MemoryHandler{}

	t.Run("unauthorized_without_user", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/messages/search", nil)

		h.SearchMessage(c)

		var resp map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if int(resp["code"].(float64)) != int(common.CodeUnauthorized) {
			t.Fatalf("expected unauthorized code %d, got %v", common.CodeUnauthorized, resp["code"])
		}
	})

	t.Run("success_with_user", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/messages/search?memory_id=m1,m2&query=test&top_n=3", nil)
		c.Set("user", &entity.User{ID: "u1"})

		h.SearchMessage(c)

		var resp struct {
			Code    int                      `json:"code"`
			Message bool                     `json:"message"`
			Data    []map[string]interface{} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.Code != int(common.CodeSuccess) {
			t.Fatalf("expected success code %d, got %d", common.CodeSuccess, resp.Code)
		}
		if !resp.Message {
			t.Fatalf("expected message=true, got %v", resp.Message)
		}
		if len(resp.Data) != 0 {
			t.Fatalf("expected empty data, got %d items", len(resp.Data))
		}
	})
}
