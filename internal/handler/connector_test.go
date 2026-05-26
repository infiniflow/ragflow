package handler

import (
	"encoding/json"
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
	gotConnectorID string
	gotUserID      string
}

func (f *fakeConnectorService) GetConnector(connectorID, userID string) (*entity.Connector, common.ErrorCode, error) {
	f.gotConnectorID = connectorID
	f.gotUserID = userID
	return &entity.Connector{
		ID:          connectorID,
		TenantID:    userID,
		Name:        "REST source",
		Source:      "rest_api",
		InputType:   "poll",
		Config:      entity.JSONMap{"url": "https://example.com/feed"},
		RefreshFreq: 5,
		PruneFreq:   5,
		TimeoutSecs: 1740,
		Status:      "0",
	}, common.CodeSuccess, nil
}

func (f *fakeConnectorService) ListConnectors(userID string) (*service.ListConnectorsResponse, error) {
	return &service.ListConnectorsResponse{Connectors: []*dao.ConnectorListItem{}}, nil
}

func TestGetConnector(t *testing.T) {
	gin.SetMode(gin.TestMode)

	connectorService := &fakeConnectorService{}
	h := &ConnectorHandler{connectorService: connectorService}
	r := gin.New()
	r.GET("/api/v1/connectors/:connector_id", func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "tenant123"})
		h.GetConnector(c)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/connectors/conn123", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if connectorService.gotConnectorID != "conn123" {
		t.Fatalf("connectorID = %q, want conn123", connectorService.gotConnectorID)
	}
	if connectorService.gotUserID != "tenant123" {
		t.Fatalf("userID = %q, want tenant123", connectorService.gotUserID)
	}

	var resp struct {
		Code common.ErrorCode `json:"code"`
		Data entity.Connector `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Code != common.CodeSuccess {
		t.Fatalf("code = %d, want %d, body: %s", resp.Code, common.CodeSuccess, w.Body.String())
	}
	if resp.Data.ID != "conn123" || resp.Data.TenantID != "tenant123" {
		t.Fatalf("response connector = %+v", resp.Data)
	}
	if resp.Data.Config["url"] != "https://example.com/feed" {
		t.Fatalf("config url = %v", resp.Data.Config["url"])
	}
}
