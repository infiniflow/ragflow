package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"ragflow/internal/handler"
)

func TestRouterSetupRegistersDatasetSearchRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	r := &Router{
		authHandler:     handler.NewAuthHandler(),
		datasetsHandler: handler.NewDatasetsHandler(nil, nil),
	}
	r.Setup(engine)

	for _, path := range []string{"/api/v1/datasets/dataset-1/search", "/api/v1/datasets/search", "/api/v1/retrieval"} {
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, nil)
		engine.ServeHTTP(resp, req)
		if resp.Code == http.StatusNotFound {
			t.Fatalf("POST %s returned 404", path)
		}
		if resp.Code != http.StatusUnauthorized {
			t.Fatalf("POST %s status=%d body=%s; want auth middleware to handle registered route", path, resp.Code, resp.Body.String())
		}
	}
}
