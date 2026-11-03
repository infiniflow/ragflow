package handler

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"slices"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/ingestion/knowledge_compile"
	"ragflow/internal/mcp"
	"ragflow/internal/service"
)

// recordingDatasetService records every call mcpRetrieval makes downstream. It
// implements each method explicitly (rather than embedding a nil service) so
// calls are counted even on paths that never reach them.
type recordingDatasetService struct {
	listCalls   int
	searchCalls int
	getCalls    int

	listPages  []int
	listSizes  []int
	request    *service.SearchDatasetsRequest
	searchUser string
	getIDs     []string

	listRows  []map[string]interface{}
	response  *service.SearchDatasetsResponse
	searchErr error
}

func (f *recordingDatasetService) ListDatasets(_ context.Context, _, _ string, page, pageSize int, _ []dao.OrderTerm, _ string, _ []string, _, _ string, _ []string, _ string) ([]map[string]interface{}, int64, common.ErrorCode, error) {
	f.listCalls++
	f.listPages = append(f.listPages, page)
	f.listSizes = append(f.listSizes, pageSize)
	return f.listRows, int64(len(f.listRows)), common.CodeSuccess, nil
}

func (f *recordingDatasetService) SearchDatasets(_ context.Context, req *service.SearchDatasetsRequest, userID string) (*service.SearchDatasetsResponse, error) {
	f.searchCalls++
	f.request = req
	f.searchUser = userID
	if f.searchErr != nil {
		return nil, f.searchErr
	}
	if f.response == nil {
		return &service.SearchDatasetsResponse{}, nil
	}
	return f.response, nil
}

func (f *recordingDatasetService) GetDataset(_ context.Context, datasetID, _ string) (map[string]interface{}, common.ErrorCode, error) {
	f.getCalls++
	f.getIDs = append(f.getIDs, datasetID)
	return map[string]interface{}{"name": "dataset-" + datasetID}, common.CodeSuccess, nil
}

// isolateKnowledgeCompilePublisher restores the process-wide publisher that the
// success path installs through document.NewDocumentService.
func isolateKnowledgeCompilePublisher(t *testing.T) {
	t.Helper()
	prev := knowledge_compile.DefaultPublisher()
	t.Cleanup(func() { knowledge_compile.SetScheduler(prev) })
}

func TestMCPRetrievalUsesFixedWindowAndForwardsPagination(t *testing.T) {
	cases := []struct {
		name       string
		page       int
		pageSize   int
		wantPage   *int
		wantSize   *int
		wantJSONPg int
		wantJSONPS int
	}{
		{name: "first page", page: 1, pageSize: 10, wantPage: new(1), wantSize: new(10), wantJSONPg: 1, wantJSONPS: 10},
		{name: "half-full page", page: 1, pageSize: 50, wantPage: new(1), wantSize: new(50), wantJSONPg: 1, wantJSONPS: 50},
		{name: "mid page", page: 2, pageSize: 50, wantPage: new(2), wantSize: new(50), wantJSONPg: 2, wantJSONPS: 50},
		{name: "last full page inside window", page: 7, pageSize: 10, wantPage: new(7), wantSize: new(10), wantJSONPg: 7, wantJSONPS: 10},
		{name: "schema max page size", page: 1, pageSize: 100, wantPage: new(1), wantSize: new(100), wantJSONPg: 1, wantJSONPS: 100},
		{name: "exact window boundary is allowed", page: 8, pageSize: 64, wantPage: new(8), wantSize: new(64), wantJSONPg: 8, wantJSONPS: 64},
		{name: "unset pagination is forwarded as nil", page: 0, pageSize: 0, wantJSONPg: 1, wantJSONPS: 30},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			isolateKnowledgeCompilePublisher(t)
			probe := &recordingDatasetService{}

			result, err := mcpRetrieval(t.Context(), probe, "user-1", mcp.RetrievalRequest{
				DatasetIDs:  []string{"ds-1"},
				DocumentIDs: []string{"doc-1"},
				Question:    "alpha",
				Page:        tc.page,
				PageSize:    tc.pageSize,
			})
			if err != nil {
				t.Fatalf("mcpRetrieval failed: %v", err)
			}
			if probe.searchCalls != 1 {
				t.Fatalf("SearchDatasets calls = %d, want 1", probe.searchCalls)
			}
			if probe.listCalls != 0 {
				t.Fatalf("ListDatasets calls = %d, want 0 when dataset IDs are given", probe.listCalls)
			}
			got := probe.request
			if got == nil {
				t.Fatal("SearchDatasets did not receive a request")
			}
			if got.RerankCandidatesCount == nil || *got.RerankCandidatesCount != 512 {
				t.Fatalf("unexpected rerank candidate count: %#v", got.RerankCandidatesCount)
			}
			if !sameIntPtr(got.Page, tc.wantPage) {
				t.Fatalf("page = %#v, want %#v", got.Page, tc.wantPage)
			}
			if !sameIntPtr(got.PageSize, tc.wantSize) {
				t.Fatalf("page size = %#v, want %#v", got.PageSize, tc.wantSize)
			}
			if !slices.Equal(got.DatasetIDs, []string{"ds-1"}) {
				t.Fatalf("dataset IDs = %v, want [ds-1]", got.DatasetIDs)
			}
			if !slices.Equal(got.DocumentIDs, []string{"doc-1"}) {
				t.Fatalf("document IDs = %v, want [doc-1]", got.DocumentIDs)
			}
			if got.Question != "alpha" {
				t.Fatalf("question = %q, want alpha", got.Question)
			}
			if probe.searchUser != "user-1" {
				t.Fatalf("search userID = %q, want user-1", probe.searchUser)
			}
			if probe.getCalls != 1 || !slices.Equal(probe.getIDs, []string{"ds-1"}) {
				t.Fatalf("GetDataset calls = %d (%v), want 1 ([ds-1])", probe.getCalls, probe.getIDs)
			}

			var payload map[string]interface{}
			if err := json.Unmarshal([]byte(result), &payload); err != nil {
				t.Fatalf("result is not valid JSON: %v (%q)", err, result)
			}
			pagination, ok := payload["pagination"].(map[string]interface{})
			if !ok {
				t.Fatalf("pagination missing from result: %#v", payload)
			}
			if pagination["page"] != float64(tc.wantJSONPg) || pagination["page_size"] != float64(tc.wantJSONPS) {
				t.Fatalf("pagination = %#v, want page=%d page_size=%d", pagination, tc.wantJSONPg, tc.wantJSONPS)
			}
			if pagination["total_chunks"] != float64(0) {
				t.Fatalf("total_chunks = %#v, want 0", pagination["total_chunks"])
			}
		})
	}
}

func TestMCPRetrievalRejectsOutOfWindowBeforeAnyDownstreamCall(t *testing.T) {
	cases := []struct {
		name       string
		page       int
		pageSize   int
		datasetIDs []string
	}{
		{name: "one past the boundary", page: 513, pageSize: 1, datasetIDs: []string{"ds-1"}},
		{name: "common overflow", page: 6, pageSize: 100, datasetIDs: []string{"ds-1"}},
		{name: "multiplication overflow", page: math.MaxInt, pageSize: math.MaxInt, datasetIDs: []string{"ds-1"}},
		{name: "no dataset ids", page: 513, pageSize: 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			probe := &recordingDatasetService{}

			result, err := mcpRetrieval(t.Context(), probe, "user-1", mcp.RetrievalRequest{
				DatasetIDs: tc.datasetIDs,
				Question:   "alpha",
				Page:       tc.page,
				PageSize:   tc.pageSize,
			})
			if err == nil || !strings.Contains(err.Error(), "exceeds the fixed rerank candidate window") {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != "" {
				t.Fatalf("unexpected result on rejection: %q", result)
			}
			if probe.listCalls != 0 || probe.searchCalls != 0 || probe.getCalls != 0 {
				t.Fatalf("out-of-window request invoked downstream services: list=%d search=%d get=%d",
					probe.listCalls, probe.searchCalls, probe.getCalls)
			}
		})
	}
}

func TestMCPRetrievalPropagatesSearchError(t *testing.T) {
	isolateKnowledgeCompilePublisher(t)
	sentinel := errors.New("search exploded")
	probe := &recordingDatasetService{searchErr: sentinel}

	result, err := mcpRetrieval(t.Context(), probe, "user-1", mcp.RetrievalRequest{
		DatasetIDs: []string{"ds-1"},
		Question:   "alpha",
		Page:       1,
		PageSize:   10,
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want the downstream sentinel", err)
	}
	if result != "" {
		t.Fatalf("unexpected result on failure: %q", result)
	}
	if probe.searchCalls != 1 {
		t.Fatalf("SearchDatasets calls = %d, want 1", probe.searchCalls)
	}
	if probe.getCalls != 0 {
		t.Fatalf("GetDataset calls = %d, want 0 after a search error", probe.getCalls)
	}
	if got := probe.request; got == nil ||
		got.RerankCandidatesCount == nil || *got.RerankCandidatesCount != 512 {
		t.Fatalf("search request not populated before the error: %#v", got)
	}
}

func sameIntPtr(got, want *int) bool {
	if got == nil || want == nil {
		return got == nil && want == nil
	}
	return *got == *want
}
