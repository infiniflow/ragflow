package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/handler"
	"ragflow/internal/service"
)

func TestConnectorRoutesDoNotConflictWithOAuthCallbacks(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	engine.GET("/api/v1/connectors/gmail/oauth/web/callback", func(c *gin.Context) {
		c.String(http.StatusOK, "gmail")
	})
	engine.GET("/api/v1/connectors/google-drive/oauth/web/callback", func(c *gin.Context) {
		c.String(http.StatusOK, "google-drive")
	})
	engine.GET("/api/v1/connectors/box/oauth/web/callback", func(c *gin.Context) {
		c.String(http.StatusOK, "box")
	})

	connectors := engine.Group("/api/v1/connectors")
	connectors.GET("/:connector_id", func(c *gin.Context) {
		c.String(http.StatusOK, c.Param("connector_id"))
	})
	connectors.GET("/:connector_id/logs", func(c *gin.Context) {
		c.String(http.StatusOK, c.Param("connector_id"))
	})

	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "gmail callback",
			path: "/api/v1/connectors/gmail/oauth/web/callback",
			want: "gmail",
		},
		{
			name: "google drive callback",
			path: "/api/v1/connectors/google-drive/oauth/web/callback",
			want: "google-drive",
		},
		{
			name: "box callback",
			path: "/api/v1/connectors/box/oauth/web/callback",
			want: "box",
		},
		{
			name: "connector detail",
			path: "/api/v1/connectors/connector-1",
			want: "connector-1",
		},
		{
			name: "connector logs",
			path: "/api/v1/connectors/connector-1/logs",
			want: "connector-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			engine.ServeHTTP(resp, req)

			if resp.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
			}
			if resp.Body.String() != tt.want {
				t.Fatalf("body=%q want=%q", resp.Body.String(), tt.want)
			}
		})
	}
}

func TestRouterSetupRegistersUpdateDatasetRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	r := &Router{
		authHandler:     handler.NewAuthHandler(),
		datasetsHandler: handler.NewDatasetsHandler(nil, nil),
	}
	r.Setup(engine)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/datasets/dataset-1", nil)
	engine.ServeHTTP(resp, req)

	if resp.Code == http.StatusNotFound {
		t.Fatalf("PUT /api/v1/datasets/:dataset_id returned 404; UpdateDataset route is not registered")
	}
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s; want auth middleware to handle registered UpdateDataset route", resp.Code, resp.Body.String())
	}
}

func TestRouterSetupRegistersSearchbotMindMapRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	r := &Router{
		authHandler:      handler.NewAuthHandler(),
		searchBotHandler: handler.NewSearchBotHandler(nil, nil, nil, nil),
	}
	r.Setup(engine)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/searchbots/mindmap", nil)
	engine.ServeHTTP(resp, req)

	if resp.Code == http.StatusNotFound {
		t.Fatalf("POST /api/v1/searchbots/mindmap returned 404; MindMap route is not registered")
	}
	var body struct {
		Code common.ErrorCode `json:"code"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if body.Code != common.CodeUnauthorized {
		t.Fatalf("status=%d body=%s; want beta auth middleware to handle registered MindMap route", resp.Code, resp.Body.String())
	}
}

func TestRouterSetupRegistersChatbotInfoOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	r := &Router{
		authHandler: handler.NewAuthHandler(),
		botHandler:  handler.NewBotHandler(nil),
	}
	r.Setup(engine)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/chatbots/dialog-1/info", nil)
	engine.ServeHTTP(resp, req)

	if resp.Code == http.StatusNotFound {
		t.Fatalf("GET /api/v1/chatbots/:dialog_id/info returned 404; ChatbotInfo route is not registered")
	}
	var body struct {
		Code common.ErrorCode `json:"code"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if body.Code != common.CodeUnauthorized {
		t.Fatalf("status=%d body=%s; want beta auth middleware to handle registered ChatbotInfo route", resp.Code, resp.Body.String())
	}
}

func TestRouterSetupRegistersChatMindMapRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	r := &Router{
		authHandler: handler.NewAuthHandler(),
		chatHandler: handler.NewChatHandler(
			service.NewChatService(),
			service.NewUserService(),
		),
	}
	r.Setup(engine)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chat/mindmap", nil)
	engine.ServeHTTP(resp, req)

	if resp.Code == http.StatusNotFound {
		t.Fatalf("POST /api/v1/chat/mindmap returned 404; Chat MindMap route is not registered")
	}
	var body struct {
		Code common.ErrorCode `json:"code"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if body.Code != common.CodeUnauthorized {
		t.Fatalf("status=%d body=%s; want auth middleware to handle registered Chat MindMap route", resp.Code, resp.Body.String())
	}
}
