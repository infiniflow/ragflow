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
	html      string
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

func (s fakeConnectorService) UpdateConnector(string, string, *service.UpdateConnectorRequest) (*entity.Connector, common.ErrorCode, error) {
	if s.err != nil {
		return nil, s.code, s.err
	}
	return s.connector, common.CodeSuccess, nil
}

func (s fakeConnectorService) StartGoogleWebOAuth(string, string, *service.StartGoogleWebOAuthRequest) (*service.StartGoogleWebOAuthResponse, common.ErrorCode, error) {
	if s.err != nil {
		return nil, s.code, s.err
	}
	return &service.StartGoogleWebOAuthResponse{}, common.CodeSuccess, nil
}

func (s fakeConnectorService) GoogleWebOAuthCallback(string, string, string, string, string) string {
	return ""
}

func (s fakeConnectorService) PollGoogleWebOAuthResult(string, string, *service.PollGoogleWebOAuthResultRequest) (*service.PollGoogleWebOAuthResultResponse, common.ErrorCode, error) {
	if s.err != nil {
		return nil, s.code, s.err
	}
	return &service.PollGoogleWebOAuthResultResponse{}, common.CodeSuccess, nil
}

func (s fakeConnectorService) StartBoxWebOAuth(string, *service.StartBoxWebOAuthRequest) (*service.StartBoxWebOAuthResponse, common.ErrorCode, error) {
	if s.err != nil {
		return nil, s.code, s.err
	}
	return &service.StartBoxWebOAuthResponse{
		FlowID:           "flow-1",
		AuthorizationURL: "https://account.box.com/api/oauth2/authorize?state=flow-1",
		ExpiresIn:        900,
	}, common.CodeSuccess, nil
}

func (s fakeConnectorService) BoxWebOAuthCallback(string, string, string, string) string {
	if s.html != "" {
		return s.html
	}
	return "<html>box</html>"
}

func (s fakeConnectorService) PollBoxWebOAuthResult(string, *service.PollBoxWebOAuthResultRequest) (*service.PollBoxWebOAuthResultResponse, common.ErrorCode, error) {
	if s.err != nil {
		return nil, s.code, s.err
	}
	return &service.PollBoxWebOAuthResultResponse{}, common.CodeSuccess, nil
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

func TestConnectorHandlerStartBoxWebOAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := &ConnectorHandler{connectorService: fakeConnectorService{}}
	router := gin.New()
	router.POST("/api/v1/connectors/box/oauth/web/start", func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "tenant-1"})
		h.StartBoxWebOAuth(c)
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/connectors/box/oauth/web/start",
		strings.NewReader(`{"client_id":"client-1","client_secret":"secret-1"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}

	var body map[string]interface{}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if body["code"] != float64(common.CodeSuccess) {
		t.Fatalf("code=%v want=%v body=%v", body["code"], common.CodeSuccess, body)
	}
	data, ok := body["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data=%#v", body["data"])
	}
	if data["flow_id"] != "flow-1" {
		t.Fatalf("flow_id=%v", data["flow_id"])
	}
}

func TestConnectorHandlerBoxWebOAuthCallback(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := &ConnectorHandler{connectorService: fakeConnectorService{html: "<html>box callback</html>"}}
	router := gin.New()
	router.GET("/api/v1/connectors/box/oauth/web/callback", h.BoxWebOAuthCallback)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/connectors/box/oauth/web/callback?state=flow-1&code=code-1", nil)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	if got := resp.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("content-type=%q", got)
	}
	if resp.Body.String() != "<html>box callback</html>" {
		t.Fatalf("body=%q", resp.Body.String())
	}
}

func TestConnectorHandlerPollBoxWebOAuthResult(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := &ConnectorHandler{connectorService: fakeConnectorService{}}
	router := gin.New()
	router.POST("/api/v1/connectors/box/oauth/web/result", func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "tenant-1"})
		h.PollBoxWebOAuthResult(c)
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/connectors/box/oauth/web/result",
		strings.NewReader(`{"flow_id":"flow-1"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}

	var body map[string]interface{}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if body["code"] != float64(common.CodeSuccess) {
		t.Fatalf("code=%v want=%v body=%v", body["code"], common.CodeSuccess, body)
	}
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
		wantLogs  int
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
			wantLogs:  1,
		},
		{
			name: "empty logs",
			service: fakeConnectorService{
				logs:  nil,
				total: 0,
			},
			wantCode:  common.CodeSuccess,
			wantTotal: 0,
			wantLogs:  0,
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
				if len(logs) != tt.wantLogs {
					t.Fatalf("logs=%v body=%v", logs, body)
				}
				if logs[0].(map[string]interface{})["id"] != tt.wantLogID {
					t.Fatalf("logs=%v body=%v", logs, body)
				}
			}
			if tt.wantLogID == "" && tt.wantMsg == "" {
				data := body["data"].(map[string]interface{})
				if data["total"] != tt.wantTotal {
					t.Fatalf("total=%v body=%v", data["total"], body)
				}
				logs := data["logs"].([]interface{})
				if len(logs) != tt.wantLogs {
					t.Fatalf("logs=%v body=%v", logs, body)
				}
			}
		})
	}
}
