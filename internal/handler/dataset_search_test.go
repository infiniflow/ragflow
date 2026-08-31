package handler

import (
	"context"
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

func (f *fakeSearchDatasetService) SearchDataset(ctx context.Context, datasetID, userID string, req *service.SearchDatasetRequest) (*service.SearchDatasetsResponse, error) {
	f.datasetID = datasetID
	f.userID = userID
	f.req = req
	return f.resp, f.err
}

type fakeSearchDatasetsService struct {
	userID string
	req    *service.SearchDatasetsRequest
	resp   *service.SearchDatasetsResponse
	err    error
}

func (f *fakeSearchDatasetsService) SearchDatasets(ctx context.Context, req *service.SearchDatasetsRequest, userID string) (*service.SearchDatasetsResponse, error) {
	f.userID = userID
	f.req = req
	return f.resp, f.err
}

func TestDatasetsHandlerSearchDataset(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeSearchDatasetService{resp: &service.SearchDatasetsResponse{Total: 1}}
	h := &DatasetsHandler{searchDatasetService: fake}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/datasets/ds-1/search", strings.NewReader(`{"question":"hello","doc_ids":["doc-1"],"knn_top_k":7,"knn_num_candidates":14}`))
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
	if fake.req.KNNTopK == nil || *fake.req.KNNTopK != 7 || fake.req.KNNNumCandidates == nil || *fake.req.KNNNumCandidates != 14 {
		t.Fatalf("KNN parameters = (%v, %v), want (7, 14)", fake.req.KNNTopK, fake.req.KNNNumCandidates)
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
	req := httptest.NewRequest(http.MethodPost, "/api/v1/datasets/ds-1/search", strings.NewReader(`{"question":"   "}`))
	req.Header.Set("Content-Type", "application/json")
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	c.Set("user", &entity.User{ID: "user-1"})
	c.Params = gin.Params{{Key: "dataset_id", Value: "ds-1"}}

	h.SearchDataset(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := decodeSearchResponse(t, rec)
	if body["code"] != float64(common.CodeArgumentError) {
		t.Fatalf("code=%v want=%d body=%s", body["code"], common.CodeArgumentError, rec.Body.String())
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

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := decodeSearchResponse(t, rec)
	if body["code"] != float64(common.CodeDataError) || body["message"] != "boom" {
		t.Fatalf("response=%v want code=%d message=boom", body, common.CodeDataError)
	}
}

func TestDatasetsHandlerSearchDatasetParseErrorUsesArgumentEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &DatasetsHandler{}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/datasets/ds-1/search", strings.NewReader(`{"question"`))
	req.Header.Set("Content-Type", "application/json")
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	c.Set("user", &entity.User{ID: "user-1"})
	c.Params = gin.Params{{Key: "dataset_id", Value: "ds-1"}}

	h.SearchDataset(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := decodeSearchResponse(t, rec)
	if body["code"] != float64(common.CodeArgumentError) {
		t.Fatalf("code=%v want=%d body=%s", body["code"], common.CodeArgumentError, rec.Body.String())
	}
}

func TestDatasetsHandlerSearchDatasetsSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeSearchDatasetsService{resp: &service.SearchDatasetsResponse{Total: 2}}
	h := &DatasetsHandler{searchDatasetsService: fake}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/datasets/search", strings.NewReader(`{"question":"  hello  ","dataset_ids":["ds-1"],"top_k":7,"include_knowledge_compilation":false}`))
	req.Header.Set("Content-Type", "application/json")
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	c.Set("user", &entity.User{ID: "user-1"})

	h.SearchDatasets(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if fake.userID != "user-1" || fake.req == nil || fake.req.Question != "hello" || len(fake.req.DatasetIDs) != 1 || fake.req.DatasetIDs[0] != "ds-1" {
		t.Fatalf("call args userID=%q req=%#v", fake.userID, fake.req)
	}
	if fake.req.IncludeCompiledChunks == nil || *fake.req.IncludeCompiledChunks {
		t.Fatalf("include_knowledge_compilation=%v want false", fake.req.IncludeCompiledChunks)
	}
	if fake.req.TopK == nil || *fake.req.TopK != 7 {
		t.Fatalf("legacy top_k alias=%v want 7", fake.req.TopK)
	}
	body := decodeSearchResponse(t, rec)
	if body["code"] != float64(common.CodeSuccess) {
		t.Fatalf("code=%v want=%d body=%s", body["code"], common.CodeSuccess, rec.Body.String())
	}
}

func TestDatasetsHandlerSearchDatasetsValidationErrorsUseArgumentEnvelope(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "parse error", body: `{"question"`},
		{name: "missing question", body: `{"dataset_ids":["ds-1"]}`},
		{name: "blank question", body: `{"question":"   ","dataset_ids":["ds-1"]}`},
		{name: "missing dataset ids", body: `{"question":"hello"}`},
		{name: "empty dataset ids", body: `{"question":"hello","dataset_ids":[]}`},
		{name: "invalid top k", body: `{"question":"hello","dataset_ids":["ds-1"],"top_k":0}`},
		{name: "legacy top k too large", body: `{"question":"hello","dataset_ids":["ds-1"],"top_k":2049}`},
		{name: "knn top k too large", body: `{"question":"hello","dataset_ids":["ds-1"],"knn_top_k":2049}`},
		{name: "invalid knn candidates", body: `{"question":"hello","dataset_ids":["ds-1"],"knn_num_candidates":0}`},
		{name: "knn candidates below default top k", body: `{"question":"hello","dataset_ids":["ds-1"],"knn_num_candidates":10}`},
		{name: "knn candidates below top k", body: `{"question":"hello","dataset_ids":["ds-1"],"knn_top_k":8,"knn_num_candidates":7}`},
		{name: "invalid threshold", body: `{"question":"hello","dataset_ids":["ds-1"],"similarity_threshold":1.1}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			h := &DatasetsHandler{}

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/datasets/search", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			c, _ := gin.CreateTestContext(rec)
			c.Request = req
			c.Set("user", &entity.User{ID: "user-1"})

			h.SearchDatasets(c)

			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			body := decodeSearchResponse(t, rec)
			if body["code"] != float64(common.CodeArgumentError) {
				t.Fatalf("code=%v want=%d body=%s", body["code"], common.CodeArgumentError, rec.Body.String())
			}
		})
	}
}

func TestValidateSearchParamsKNNBounds(t *testing.T) {
	knnTopK := 2048
	if err := validateSearchParams(nil, nil, &knnTopK, nil, nil, nil, nil); err != nil {
		t.Fatalf("knn_top_k=2048 with default knn_num_candidates should be valid: %v", err)
	}

	legacyTopK := 2049
	err := validateSearchParams(nil, nil, nil, &legacyTopK, nil, nil, nil)
	if err == nil || err.Error() != "top_k (alias for knn_top_k) must be between 1 and 2048" {
		t.Fatalf("legacy top_k error = %v", err)
	}
}

func TestDatasetsHandlerSearchDatasetsPropagatesServiceError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeSearchDatasetsService{err: errors.New("boom")}
	h := &DatasetsHandler{searchDatasetsService: fake}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/datasets/search", strings.NewReader(`{"question":"hello","dataset_ids":["ds-1"]}`))
	req.Header.Set("Content-Type", "application/json")
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	c.Set("user", &entity.User{ID: "user-1"})

	h.SearchDatasets(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := decodeSearchResponse(t, rec)
	if body["code"] != float64(common.CodeDataError) || body["message"] != "boom" {
		t.Fatalf("response=%v want code=%d message=boom", body, common.CodeDataError)
	}
}

func decodeSearchResponse(t *testing.T, rec *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	if !json.Valid(rec.Body.Bytes()) {
		t.Fatalf("response is not valid json: %s", rec.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v body=%s", err, rec.Body.String())
	}
	return body
}
