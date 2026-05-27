package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

type fakeConnectorService struct {
	connector *entity.Connector
	code      common.ErrorCode
	err       error
}

func (s fakeConnectorService) ListConnectors(string) (*service.ListConnectorsResponse, error) {
	return &service.ListConnectorsResponse{Connectors: []*dao.ConnectorListItem{}}, nil
}

func (s fakeConnectorService) CreateConnector(string, *service.CreateConnectorRequest) (*entity.Connector, error) {
	return s.connector, s.err
}

func (s fakeConnectorService) GetConnector(string, string) (*entity.Connector, common.ErrorCode, error) {
	if s.err != nil {
		return nil, s.code, s.err
	}
	return s.connector, common.CodeSuccess, nil
}

func (s fakeConnectorService) UpdateConnector(string, string, *service.UpdateConnectorRequest) (*entity.Connector, common.ErrorCode, error) {
	if s.err != nil {
		return nil, s.code, s.err
	}
	return s.connector, common.CodeSuccess, nil
}

func TestConnectorHandlerGetConnector(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name     string
		service  fakeConnectorService
		wantCode common.ErrorCode
		wantID   string
		wantMsg  string
	}{
		{
			name: "success",
			service: fakeConnectorService{connector: &entity.Connector{
				ID:       "connector-1",
				TenantID: "tenant-1",
				Name:     "Docs",
				Source:   "google_drive",
				Status:   "unstart",
				Config:   entity.JSONMap{"folder": "docs"},
			}},
			wantCode: common.CodeSuccess,
			wantID:   "connector-1",
		},
		{
			name:     "unauthorized",
			service:  fakeConnectorService{code: common.CodeAuthenticationError, err: fmt.Errorf("No authorization.")},
			wantCode: common.CodeAuthenticationError,
			wantMsg:  "No authorization.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &ConnectorHandler{connectorService: tt.service}
			router := gin.New()
			router.GET("/api/v1/connectors/:connector_id", func(c *gin.Context) {
				c.Set("user", &entity.User{ID: "tenant-1"})
				h.GetConnector(c)
			})

			resp := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/connectors/connector-1", nil)
			router.ServeHTTP(resp, req)
			if resp.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
			}

			var body map[string]interface{}
			if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
				t.Fatalf("unmarshal response: %v", err)
			}
			if body["code"] != float64(tt.wantCode) {
				t.Fatalf("code=%v body=%v", body["code"], body)
			}
			if tt.wantMsg != "" && body["message"] != tt.wantMsg {
				t.Fatalf("message=%v", body["message"])
			}
			if tt.wantID != "" && body["data"].(map[string]interface{})["id"] != tt.wantID {
				t.Fatalf("data=%v", body["data"])
			}
		})
	}
}

func TestGoogleGmailWebOAuthCallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &ConnectorHandler{}
	router := gin.New()
	router.GET("/api/v1/connectors/gmail/oauth/web/callback", h.GoogleGmailWebOAuthCallback)

	t.Run("missing_state", func(t *testing.T) {
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/connectors/gmail/oauth/web/callback", nil)
		router.ServeHTTP(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
		}
		if !strings.Contains(resp.Body.String(), "Missing OAuth state parameter.") {
			t.Fatalf("unexpected body: %s", resp.Body.String())
		}
		if !strings.Contains(resp.Body.String(), "ragflow-gmail-oauth") {
			t.Fatalf("missing popup payload type: %s", resp.Body.String())
		}
		if !strings.Contains(resp.Body.String(), "<title>Google Gmail Authorization</title>") {
			t.Fatalf("missing popup title: %s", resp.Body.String())
		}
	})

	t.Run("expired_state_without_redis", func(t *testing.T) {
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/connectors/gmail/oauth/web/callback?state=fake-state", nil)
		router.ServeHTTP(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
		}
		if !strings.Contains(resp.Body.String(), "Authorization session expired. Please restart from the main window.") {
			t.Fatalf("unexpected body: %s", resp.Body.String())
		}
	})
}

func TestGmailOAuthDefaults(t *testing.T) {
	if defaultGmailWebOAuthRedirectURI != "http://localhost:9380/api/v1/connectors/gmail/oauth/web/callback" {
		t.Fatalf("unexpected redirect uri default: %s", defaultGmailWebOAuthRedirectURI)
	}
	if gmailWebOAuthHTTPClient.Timeout != gmailWebOAuthHTTPTimeout {
		t.Fatalf("expected http client timeout %s, got %s", gmailWebOAuthHTTPTimeout, gmailWebOAuthHTTPClient.Timeout)
	}
}
