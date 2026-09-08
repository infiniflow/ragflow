package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"ragflow/internal/handler"
)

func TestRouterSetupRegistersSearchDatasetRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	r := &Router{
		authHandler:     handler.NewAuthHandler(),
		datasetsHandler: handler.NewDatasetsHandler(nil, nil),
	}
	r.Setup(engine)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/datasets/dataset-1/search", nil)
	engine.ServeHTTP(resp, req)

	if resp.Code == http.StatusNotFound {
		t.Fatalf("POST /api/v1/datasets/:dataset_id/search returned 404; SearchDataset route is not registered")
	}
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s; want auth middleware to handle registered SearchDataset route", resp.Code, resp.Body.String())
	}
}
