package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

type fakeConnectorService struct {
	err error
}

func (s fakeConnectorService) ListConnectors(string) (*service.ListConnectorsResponse, error) {
	return &service.ListConnectorsResponse{}, nil
}

func (s fakeConnectorService) TestConnector(string, string) error {
	return s.err
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
