package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
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
