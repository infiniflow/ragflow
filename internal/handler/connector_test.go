package handler

import (
	"bytes"
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
	gotReq         *service.UpdateConnectorRequest
}

func (f *fakeConnectorService) ListConnectors(userID string) (*service.ListConnectorsResponse, error) {
	return &service.ListConnectorsResponse{Connectors: []*dao.ConnectorListItem{}}, nil
}

func (f *fakeConnectorService) UpdateConnector(connectorID, userID string, req *service.UpdateConnectorRequest) (*entity.Connector, common.ErrorCode, error) {
	f.gotConnectorID = connectorID
	f.gotUserID = userID
	f.gotReq = req
	config := entity.JSONMap{}
	if req != nil && req.Config != nil {
		config = entity.JSONMap(*req.Config)
	}
	return &entity.Connector{
		ID:          connectorID,
		TenantID:    userID,
		Name:        "REST source",
		Source:      "rest_api",
		InputType:   "poll",
		Config:      config,
		RefreshFreq: valueOrZero(req.RefreshFreq),
		PruneFreq:   valueOrZero(req.PruneFreq),
		TimeoutSecs: valueOrZero(req.TimeoutSecs),
		Status:      "5",
	}, common.CodeSuccess, nil
}

func valueOrZero(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func TestUpdateConnector(t *testing.T) {
	gin.SetMode(gin.TestMode)

	connectorService := &fakeConnectorService{}
	h := &ConnectorHandler{connectorService: connectorService}
	r := gin.New()
	r.PATCH("/api/v1/connectors/:connector_id", func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "tenant123"})
		h.UpdateConnector(c)
	})

	body := []byte(`{"data":{"refresh_freq":10,"prune_freq":20,"timeout_secs":180,"config":{"sync_deleted_files":true},"status":"SCHEDULE"}}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/connectors/conn123", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
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
	if connectorService.gotReq == nil {
		t.Fatal("service request was not captured")
	}
	if connectorService.gotReq.RefreshFreq == nil || *connectorService.gotReq.RefreshFreq != 10 {
		t.Fatalf("refresh_freq = %v, want 10", connectorService.gotReq.RefreshFreq)
	}
	if connectorService.gotReq.PruneFreq == nil || *connectorService.gotReq.PruneFreq != 20 {
		t.Fatalf("prune_freq = %v, want 20", connectorService.gotReq.PruneFreq)
	}
	if connectorService.gotReq.TimeoutSecs == nil || *connectorService.gotReq.TimeoutSecs != 180 {
		t.Fatalf("timeout_secs = %v, want 180", connectorService.gotReq.TimeoutSecs)
	}
	if connectorService.gotReq.Config == nil || (*connectorService.gotReq.Config)["sync_deleted_files"] != true {
		t.Fatalf("config = %#v", connectorService.gotReq.Config)
	}
	if connectorService.gotReq.Status != "SCHEDULE" {
		t.Fatalf("status = %q, want SCHEDULE", connectorService.gotReq.Status)
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
}
