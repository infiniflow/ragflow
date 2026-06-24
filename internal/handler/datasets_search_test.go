package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

type fakeSearchDatasetService struct {
	datasetID string
	userID    string
	req       *service.SearchDatasetRequest
	resp      *service.SearchDatasetsResponse
	err       error
}

func (f *fakeSearchDatasetService) SearchDataset(datasetID, userID string, req *service.SearchDatasetRequest) (*service.SearchDatasetsResponse, error) {
	f.datasetID = datasetID
	f.userID = userID
	f.req = req
	return f.resp, f.err
}

func TestDatasetsHandlerSearchDataset(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeSearchDatasetService{resp: &service.SearchDatasetsResponse{Total: 1}}
	h := &DatasetsHandler{searchDatasetService: fake}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/datasets/ds-1/search", strings.NewReader(`{"question":"hello","doc_ids":["doc-1"],"top_k":7}`))
	req.Header.Set("Content-Type", "application/json")
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	c.Set("user", &entity.User{ID: "user-1"})
	c.Params = gin.Params{{Key: "dataset_id", Value: "ds-1"}}

	h.SearchDataset(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if fake.datasetID != "ds-1" || fake.userID != "user-1" {
		t.Fatalf("call args = (%q,%q), want (ds-1,user-1)", fake.datasetID, fake.userID)
	}
	if fake.req == nil || fake.req.Question != "hello" || len(fake.req.DocIDs) != 1 || fake.req.DocIDs[0] != "doc-1" {
		t.Fatalf("request = %#v", fake.req)
	}
	if len(fake.req.ToSearchDatasetsRequest("ds-1").DatasetIDs) != 1 {
		t.Fatal("request conversion failed")
	}

	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v body=%s", err, rec.Body.String())
	}
	if body["code"] != float64(common.CodeSuccess) {
		t.Fatalf("code=%v want=%d", body["code"], common.CodeSuccess)
	}
}

func TestDatasetsHandlerSearchDatasetValidatesQuestion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &DatasetsHandler{}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/datasets/ds-1/search", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	c.Set("user", &entity.User{ID: "user-1"})
	c.Params = gin.Params{{Key: "dataset_id", Value: "ds-1"}}

	h.SearchDataset(c)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !json.Valid(rec.Body.Bytes()) {
		t.Fatalf("response is not valid json: %s", rec.Body.String())
	}
}

func TestDatasetsHandlerSearchDatasetPropagatesServiceError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeSearchDatasetService{err: errors.New("boom")}
	h := &DatasetsHandler{searchDatasetService: fake}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/datasets/ds-1/search", strings.NewReader(`{"question":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	c.Set("user", &entity.User{ID: "user-1"})
	c.Params = gin.Params{{Key: "dataset_id", Value: "ds-1"}}

	h.SearchDataset(c)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
