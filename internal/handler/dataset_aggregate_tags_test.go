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

func newAggregateTagsHandlerRouter(authenticated bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := &DatasetsHandler{}
	r := gin.New()
	r.GET("/api/v1/datasets/tags/aggregation", func(c *gin.Context) {
		if authenticated {
			c.Set("user", &entity.User{ID: "user-1"})
		}
		h.AggregateTags(c)
	})
	return r
}

func TestDatasetsHandlerAggregateTagsRequiresDatasetIDs(t *testing.T) {
	r := newAggregateTagsHandlerRouter(true)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/datasets/tags/aggregation", nil)
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}

	var body struct {
		Code    int         `json:"code"`
		Data    interface{} `json:"data"`
		Message string      `json:"message"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v body=%s", err, resp.Body.String())
	}
	if body.Code != int(common.CodeDataError) {
		t.Fatalf("code=%d want=%d body=%s", body.Code, common.CodeDataError, resp.Body.String())
	}
	if body.Message != "Lack of dataset_ids in query parameters" {
		t.Fatalf("message=%q want=%q", body.Message, "Lack of dataset_ids in query parameters")
	}
	if body.Data != nil {
		t.Fatalf("data=%v want nil", body.Data)
	}
}

func TestDatasetsHandlerAggregateTagsRequiresAuth(t *testing.T) {
	r := newAggregateTagsHandlerRouter(false)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/datasets/tags/aggregation?dataset_ids=123e4567-e89b-12d3-a456-426614174000", nil)
	r.ServeHTTP(resp, req)

	var body struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v body=%s", err, resp.Body.String())
	}
	if body.Code != int(common.CodeUnauthorized) {
		t.Fatalf("code=%d want=%d body=%s", body.Code, common.CodeUnauthorized, resp.Body.String())
	}
	if body.Message != "User not found" {
		t.Fatalf("message=%q want=%q", body.Message, "User not found")
	}
}
