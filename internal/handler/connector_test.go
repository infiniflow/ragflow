package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

type fakeConnectorService struct {
	connector *entity.Connector
	logs      []*entity.ConnectorSyncLog
	total     int64
	code      common.ErrorCode
	err       error
}

func (s fakeConnectorService) ListConnectors(string) (*service.ListConnectorsResponse, error) {
	return &service.ListConnectorsResponse{}, nil
}

func (s fakeConnectorService) TestConnector(string, string) error {
	return s.err
}

func (s fakeConnectorService) CreateConnector(string, *service.CreateConnectorRequest) (*entity.Connector, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.connector, nil
}

func (s fakeConnectorService) GetConnector(string, string) (*entity.Connector, common.ErrorCode, error) {
	if s.err != nil {
		return nil, s.code, s.err
	}
	return s.connector, common.CodeSuccess, nil
}

func (s fakeConnectorService) ListLog(string, string, int, int) ([]*entity.ConnectorSyncLog, int64, common.ErrorCode, error) {
	if s.err != nil {
		return nil, 0, s.code, s.err
	}
	return s.logs, s.total, common.CodeSuccess, nil
}

func (s fakeConnectorService) DeleteConnector(string, string) (bool, common.ErrorCode, error) {
	if s.err != nil {
		return false, s.code, s.err
	}
	return true, common.CodeSuccess, nil
}

func (s fakeConnectorService) RebuildConnector(string, string, string) (bool, common.ErrorCode, error) {
	if s.err != nil {
		return false, s.code, s.err
	}
	return true, common.CodeSuccess, nil
}

func TestConnectorHandlerTestConnector(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name     string
		err      error
		wantCode common.ErrorCode
	}{
		{
			name:     "success",
			err:      nil,
			wantCode: common.CodeSuccess,
		},
		{
			name:     "not found",
			err:      service.ErrConnectorNotFound,
			wantCode: common.CodeDataError,
		},
		{
			name:     "unauthorized",
			err:      service.ErrConnectorNoAuth,
			wantCode: common.CodeAuthenticationError,
		},
		{
			name:     "unsupported source",
			err:      service.ErrConnectorTestUnsupported,
			wantCode: common.CodeArgumentError,
		},
		{
			name:     "validation failure",
			err:      fmt.Errorf("connector credentials are missing"),
			wantCode: common.CodeDataError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &ConnectorHandler{connectorService: fakeConnectorService{err: tt.err}}
			router := gin.New()
			router.POST("/api/v1/connectors/:connector_id/test", func(c *gin.Context) {
				c.Set("user", &entity.User{ID: "tenant-1"})
				h.TestConnector(c)
			})

			resp := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/connectors/connector-1/test", nil)
			router.ServeHTTP(resp, req)
			if resp.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
			}

			var body map[string]interface{}
			if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
				t.Fatalf("unmarshal response: %v", err)
			}
			if body["code"] != float64(tt.wantCode) {
				t.Fatalf("code=%v want=%v body=%v", body["code"], tt.wantCode, body)
			}
		})
	}
}

func TestConnectorHandlerDeleteConnector(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name     string
		service  fakeConnectorService
		wantCode common.ErrorCode
		wantData interface{}
		wantMsg  string
	}{
		{
			name:     "success",
			service:  fakeConnectorService{},
			wantCode: common.CodeSuccess,
			wantData: true,
		},
		{
			name:     "unauthorized",
			service:  fakeConnectorService{code: common.CodeAuthenticationError, err: fmt.Errorf("No authorization.")},
			wantCode: common.CodeAuthenticationError,
			wantData: nil,
			wantMsg:  "No authorization.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &ConnectorHandler{connectorService: tt.service}
			router := gin.New()
			router.DELETE("/api/v1/connectors/:connector_id", func(c *gin.Context) {
				c.Set("user", &entity.User{ID: "tenant-1"})
				h.DeleteConnector(c)
			})

			resp := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodDelete, "/api/v1/connectors/connector-1", nil)
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
			if body["data"] != tt.wantData {
				t.Fatalf("data=%v body=%v", body["data"], body)
			}
			if tt.wantMsg != "" && body["message"] != tt.wantMsg {
				t.Fatalf("message=%v", body["message"])
			}
		})
	}
}

func TestConnectorHandlerListLogs(t *testing.T) {
	gin.SetMode(gin.TestMode)

	startedAt := time.Date(2026, 5, 28, 8, 30, 0, 0, time.Local)
	updatedAt := time.Date(2026, 5, 28, 9, 0, 0, 0, time.Local)

	tests := []struct {
		name      string
		service   fakeConnectorService
		wantCode  common.ErrorCode
		wantMsg   string
		wantTotal float64
		wantLogID string
	}{
		{
			name: "success",
			service: fakeConnectorService{
				logs: []*entity.ConnectorSyncLog{{
					ID:                   "log-1",
					ConnectorID:          "connector-1",
					TaskType:             "sync",
					KbID:                 "kb-1",
					UpdateDate:           &updatedAt,
					NewDocsIndexed:       2,
					TotalDocsIndexed:     10,
					DocsRemovedFromIndex: 1,
					ErrorMsg:             "",
					ErrorCount:           0,
					TimeStarted:          &startedAt,
					RefreshFreq:          5,
					PruneFreq:            5,
					KbName:               "Docs",
					Status:               "3",
				}},
				total: 1,
			},
			wantCode:  common.CodeSuccess,
			wantTotal: 1,
			wantLogID: "log-1",
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
			router.GET("/api/v1/connectors/:connector_id/logs", func(c *gin.Context) {
				c.Set("user", &entity.User{ID: "tenant-1"})
				h.ListLogs(c)
			})

			resp := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/connectors/connector-1/logs?page=2&page_size=5", nil)
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
			if tt.wantLogID != "" {
				data := body["data"].(map[string]interface{})
				if data["total"] != tt.wantTotal {
					t.Fatalf("total=%v body=%v", data["total"], body)
				}
				logs := data["logs"].([]interface{})
				if logs[0].(map[string]interface{})["id"] != tt.wantLogID {
					t.Fatalf("logs=%v body=%v", logs, body)
				}
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
