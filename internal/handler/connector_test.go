package handler

import (
	"context"
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
	syncerconnector "ragflow/internal/syncer/connector"
)

type fakeConnectorService struct {
	connector *entity.Connector
	logs      []*entity.ConnectorSyncLog
	total     int64
	code      common.ErrorCode
	err       error
	html      string
	capture   *logListCapture
}

type logListCapture struct {
	page     int
	pageSize int
}

func (s fakeConnectorService) ListConnectors(context.Context, string) (*service.ListConnectorsResponse, error) {
	return &service.ListConnectorsResponse{}, nil
}

func (s fakeConnectorService) TestConnector(context.Context, string, string, entity.JSONMap) error {
	return s.err
}

func (s fakeConnectorService) CreateConnector(context.Context, string, *service.CreateConnectorRequest) (*entity.Connector, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.connector, nil
}

func (s fakeConnectorService) GetConnector(context.Context, string, string) (*entity.Connector, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.connector, nil
}

func (s fakeConnectorService) UpdateConnector(context.Context, string, string, *service.UpdateConnectorRequest) (*entity.Connector, common.ErrorCode, error) {
	if s.err != nil {
		return nil, s.code, s.err
	}
	return s.connector, common.CodeSuccess, nil
}

func (s fakeConnectorService) StartGoogleWebOAuth(context.Context, string, string, *service.StartGoogleWebOAuthRequest) (*service.StartGoogleWebOAuthResponse, common.ErrorCode, error) {
	if s.err != nil {
		return nil, s.code, s.err
	}
	return &service.StartGoogleWebOAuthResponse{}, common.CodeSuccess, nil
}

func (s fakeConnectorService) GoogleWebOAuthCallback(context.Context, string, string, string, string, string) string {
	return ""
}

func (s fakeConnectorService) PollGoogleWebOAuthResult(context.Context, string, string, *service.PollGoogleWebOAuthResultRequest) (*service.PollGoogleWebOAuthResultResponse, common.ErrorCode, error) {
	if s.err != nil {
		return nil, s.code, s.err
	}
	return &service.PollGoogleWebOAuthResultResponse{}, common.CodeSuccess, nil
}

func (s fakeConnectorService) StartBoxWebOAuth(context.Context, string, *service.StartBoxWebOAuthRequest) (*service.StartBoxWebOAuthResponse, common.ErrorCode, error) {
	if s.err != nil {
		return nil, s.code, s.err
	}
	return &service.StartBoxWebOAuthResponse{
		FlowID:           "flow-1",
		AuthorizationURL: "https://account.box.com/api/oauth2/authorize?state=flow-1",
		ExpiresIn:        900,
	}, common.CodeSuccess, nil
}

func (s fakeConnectorService) BoxWebOAuthCallback(context.Context, string, string, string, string) string {
	if s.html != "" {
		return s.html
	}
	return "<html>box</html>"
}

func (s fakeConnectorService) PollBoxWebOAuthResult(context.Context, string, *service.PollBoxWebOAuthResultRequest) (*service.PollBoxWebOAuthResultResponse, common.ErrorCode, error) {
	if s.err != nil {
		return nil, s.code, s.err
	}
	return &service.PollBoxWebOAuthResultResponse{}, common.CodeSuccess, nil
}

func (s fakeConnectorService) ListLog(context.Context, string, string, int, int) ([]*entity.ConnectorSyncLog, int64, common.ErrorCode, error) {
	if s.err != nil {
		return nil, 0, s.code, s.err
	}
	return s.logs, s.total, common.CodeSuccess, nil
}

func (s fakeConnectorService) ListLogs(_ context.Context, _, _ string, page, pageSize int) ([]*entity.ConnectorSyncLog, int64, common.ErrorCode, error) {
	if s.err != nil {
		return nil, 0, s.code, s.err
	}
	if s.capture != nil {
		s.capture.page = page
		s.capture.pageSize = pageSize
	}
	return s.logs, s.total, common.CodeSuccess, nil
}

func (s fakeConnectorService) DeleteConnector(context.Context, string, string) (bool, common.ErrorCode, error) {
	if s.err != nil {
		return false, s.code, s.err
	}
	return true, common.CodeSuccess, nil
}

func (s fakeConnectorService) RebuildConnector(context.Context, string, string, string) (bool, common.ErrorCode, error) {
	if s.err != nil {
		return false, s.code, s.err
	}
	return true, common.CodeSuccess, nil
}

func (s fakeConnectorService) ResumeFailedSync(context.Context, string, string, *service.ResumeFailedSyncRequest) (bool, common.ErrorCode, error) {
	if s.err != nil {
		return false, s.code, s.err
	}
	return true, common.CodeSuccess, nil
}

func TestConnectorHandlerGetConnector(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name        string
		connectorID string
		service     fakeConnectorService
		wantCode    common.ErrorCode
		wantData    interface{}
		wantMsg     string
	}{
		{
			name:        "success",
			connectorID: "connector-1",
			service: fakeConnectorService{
				connector: &entity.Connector{ID: "connector-1", TenantID: "tenant-1", Name: "REST source"},
			},
			wantCode: common.CodeSuccess,
			wantData: map[string]interface{}{
				"id": "connector-1",
			},
			wantMsg: "success",
		},
		{
			name:        "unauthorized",
			connectorID: "connector-1",
			service:     fakeConnectorService{err: service.ErrConnectorNoAuth},
			wantCode:    common.CodeAuthenticationError,
			wantData:    false,
			wantMsg:     "no authorization",
		},
		{
			name:        "not found",
			connectorID: "connector-missing",
			service:     fakeConnectorService{err: service.ErrConnectorNotFound},
			wantCode:    common.CodeDataError,
			wantData:    nil,
			wantMsg:     "Can't find this Connector!",
		},
		{
			name:        "missing id",
			connectorID: "",
			service:     fakeConnectorService{err: service.ErrConnectorIDRequired},
			wantCode:    common.CodeDataError,
			wantData:    nil,
			wantMsg:     "connector_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &ConnectorHandler{connectorService: tt.service}
			resp := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(resp)
			c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/connectors/"+tt.connectorID, nil)
			c.Params = gin.Params{{Key: "connector_id", Value: tt.connectorID}}
			c.Set("user", &entity.User{ID: "user-1"})

			h.GetConnector(c)

			var body map[string]interface{}
			if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
				t.Fatalf("unmarshal response: %v", err)
			}
			if body["code"] != float64(tt.wantCode) {
				t.Fatalf("code=%v want=%v body=%v", body["code"], tt.wantCode, body)
			}
			if tt.wantMsg != "" && body["message"] != tt.wantMsg {
				t.Fatalf("message=%v want=%v body=%v", body["message"], tt.wantMsg, body)
			}
			if wantData, ok := tt.wantData.(map[string]interface{}); ok {
				data, ok := body["data"].(map[string]interface{})
				if !ok {
					t.Fatalf("data=%v body=%v", body["data"], body)
				}
				if data["id"] != wantData["id"] {
					t.Fatalf("data id=%v want=%v body=%v", data["id"], wantData, body)
				}
			} else if body["data"] != tt.wantData {
				t.Fatalf("data=%v want=%v body=%v", body["data"], tt.wantData, body)
			}
		})
	}
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
		name        string
		err         error
		wantCode    common.ErrorCode
		wantMessage string
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
			wantCode: common.CodeNotImplemented,
		},
		{
			name:        "source not implemented",
			err:         fmt.Errorf("%w: seafile", service.ErrConnectorSourceNotImplemented),
			wantCode:    common.CodeNotImplemented,
			wantMessage: "connector source is not implemented: seafile",
		},
		{
			name:     "schema validation failure",
			err:      &syncerconnector.ConnectorValidationError{Message: "At least one content field must be configured (content_fields)."},
			wantCode: common.CodeDataError,
		},
		{
			name:     "missing credential failure",
			err:      &syncerconnector.ConnectorMissingCredentialError{Message: "REST API (bearer) requires 'token' in credentials"},
			wantCode: common.CodeDataError,
		},
		{
			name:     "rate limit failure",
			err:      &syncerconnector.RateLimitTriedTooManyTimesError{Message: "REST API rate limited"},
			wantCode: common.CodeDataError,
		},
		{
			name:        "unexpected failure",
			err:         fmt.Errorf("boom"),
			wantCode:    common.CodeServerError,
			wantMessage: "boom",
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
			if tt.wantMessage != "" && body["message"] != tt.wantMessage {
				t.Fatalf("message=%v want=%v body=%v", body["message"], tt.wantMessage, body)
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
			service:  fakeConnectorService{code: common.CodeAuthenticationError, err: fmt.Errorf("no authorization")},
			wantCode: common.CodeAuthenticationError,
			wantData: nil,
			wantMsg:  "no authorization",
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
			service:  fakeConnectorService{code: common.CodeAuthenticationError, err: fmt.Errorf("no authorization")},
			wantCode: common.CodeAuthenticationError,
			wantMsg:  "no authorization",
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

func TestConnectorHandlerListSyncLogs(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name         string
		query        string
		wantCode     common.ErrorCode
		wantMsg      string
		wantPage     int
		wantPageSize int
	}{
		{
			name:         "no pagination params uses defaults",
			query:        "",
			wantCode:     common.CodeSuccess,
			wantPage:     1,
			wantPageSize: 15,
		},
		{
			name:         "page only defaults page_size",
			query:        "?page=3",
			wantCode:     common.CodeSuccess,
			wantPage:     3,
			wantPageSize: 15,
		},
		{
			name:         "page_size only defaults page",
			query:        "?page_size=50",
			wantCode:     common.CodeSuccess,
			wantPage:     1,
			wantPageSize: 50,
		},
		{
			name:         "both params",
			query:        "?page=2&page_size=10",
			wantCode:     common.CodeSuccess,
			wantPage:     2,
			wantPageSize: 10,
		},
		{
			name:     "bad page",
			query:    "?page=abc",
			wantCode: common.CodeArgumentError,
			wantMsg:  "page must be an integer",
		},
		{
			name:     "bad page_size",
			query:    "?page_size=xyz",
			wantCode: common.CodeArgumentError,
			wantMsg:  "page_size must be an integer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capture := &logListCapture{}
			h := &ConnectorHandler{connectorService: fakeConnectorService{
				logs:    []*entity.ConnectorSyncLog{{ID: "log-1"}},
				total:   1,
				capture: capture,
			}}
			router := gin.New()
			router.GET("/api/v1/connectors/sync_logs", func(c *gin.Context) {
				c.Set("user", &entity.User{ID: "tenant-1"})
				h.ListSyncLogs(c)
			})

			resp := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/connectors/sync_logs"+tt.query, nil)
			router.ServeHTTP(resp, req)

			var body map[string]interface{}
			if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
				t.Fatalf("unmarshal response: %v", err)
			}
			if body["code"] != float64(tt.wantCode) {
				t.Fatalf("code=%v body=%v", body["code"], body)
			}
			if tt.wantMsg != "" && body["message"] != tt.wantMsg {
				t.Fatalf("message=%v body=%v", body["message"], body)
			}
			if tt.wantCode == common.CodeSuccess {
				if capture.page != tt.wantPage || capture.pageSize != tt.wantPageSize {
					t.Fatalf("service called with page=%d page_size=%d, want %d/%d", capture.page, capture.pageSize, tt.wantPage, tt.wantPageSize)
				}
			}
		})
	}
}
