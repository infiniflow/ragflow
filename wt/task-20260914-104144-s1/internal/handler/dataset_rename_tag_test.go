package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

func TestDatasetsHandlerRenameTagRequiresAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &DatasetsHandler{}
	r := gin.New()
	r.PUT("/api/v1/datasets/:dataset_id/tags", func(c *gin.Context) {
		h.RenameTag(c)
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/datasets/123e4567-e89b-12d3-a456-426614174000/tags", bytes.NewBufferString(`{"from_tag":"a","to_tag":"b"}`))
	req.Header.Set("Content-Type", "application/json")
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

func TestDatasetsHandlerRenameTagRejectsMissingFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &DatasetsHandler{}
	r := gin.New()
	r.PUT("/api/v1/datasets/:dataset_id/tags", func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "user-1"})
		h.RenameTag(c)
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/datasets/123e4567-e89b-12d3-a456-426614174000/tags", bytes.NewBufferString(`{"from_tag":"a"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(resp, req)

	var body struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v body=%s", err, resp.Body.String())
	}
	if body.Code != int(common.CodeDataError) {
		t.Fatalf("code=%d want=%d body=%s", body.Code, common.CodeDataError, resp.Body.String())
	}
}

func TestDatasetsHandlerRenameTagRejectsEmptyFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &DatasetsHandler{}
	r := gin.New()
	r.PUT("/api/v1/datasets/:dataset_id/tags", func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "user-1"})
		h.RenameTag(c)
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/datasets/123e4567-e89b-12d3-a456-426614174000/tags", bytes.NewBufferString(`{"from_tag":" ","to_tag":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(resp, req)

	var body struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v body=%s", err, resp.Body.String())
	}
	if body.Code != int(common.CodeArgumentError) {
		t.Fatalf("code=%d want=%d body=%s", body.Code, common.CodeArgumentError, resp.Body.String())
	}
	if body.Message != "from_tag and to_tag must not be empty" {
		t.Fatalf("message=%q want=%q", body.Message, "from_tag and to_tag must not be empty")
	}
}

func TestDatasetsHandlerRenameTagRejectsNonStringFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &DatasetsHandler{}
	r := gin.New()
	r.PUT("/api/v1/datasets/:dataset_id/tags", func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "user-1"})
		h.RenameTag(c)
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/datasets/123e4567-e89b-12d3-a456-426614174000/tags", bytes.NewBufferString(`{"from_tag":1,"to_tag":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(resp, req)

	var body struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v body=%s", err, resp.Body.String())
	}
	if body.Code != int(common.CodeArgumentError) {
		t.Fatalf("code=%d want=%d body=%s", body.Code, common.CodeArgumentError, resp.Body.String())
	}
	if body.Message != "from_tag and to_tag must be strings" {
		t.Fatalf("message=%q want=%q", body.Message, "from_tag and to_tag must be strings")
	}
}
