package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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

func (s fakeConnectorService) GetConnector(string, string) (*entity.Connector, common.ErrorCode, error) {
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
