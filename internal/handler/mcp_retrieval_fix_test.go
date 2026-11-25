package handler

import (
	"context"
	"fmt"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/mcp"
	"ragflow/internal/permission"
	"ragflow/internal/service"
)

// TestMCPRetrievalDeduplicatesRepeatedDatasetIDs is the RED test for #7: a
// repeated dataset ID must be enriched only once, while the response's
// dataset_count still reflects the original request length.
func TestMCPRetrievalDeduplicatesRepeatedDatasetIDs(t *testing.T) {
	probe := &recordingDatasetService{response: &service.SearchDatasetsResponse{
		Chunks: []map[string]interface{}{
			{"id": "c1", "dataset_id": "ds-1", "document_id": "doc-a", "document_keyword": "A"},
		},
		Total: 1,
	}}
	docs := batchTestDocuments()

	var result string
	var err error
	result, err = mcpRetrievalWithDocs(permission.WithIdentity(t.Context(), "user-1", ""), probe, docs, mcp.RetrievalRequest{
		DatasetIDs: []string{"ds-1", "ds-1"}, Question: "q", Page: 1, PageSize: 10,
	})
	if err != nil {
		t.Fatalf("mcpRetrievalWithDocs failed: %v", err)
	}
	if probe.getCalls != 1 {
		t.Fatalf("GetDataset calls = %d, want 1 for a repeated dataset ID", probe.getCalls)
	}
	if docs.listCalls != 1 || docs.metaCalls != 1 {
		t.Fatalf("list=%d metadata=%d, want 1/1 for a repeated dataset ID", docs.listCalls, docs.metaCalls)
	}
	payload := decodeBatchResult(t, result)
	queryInfo, ok := payload["query_info"].(map[string]interface{})
	if !ok {
		t.Fatalf("query_info missing: %#v", payload)
	}
	if queryInfo["dataset_count"] != float64(2) {
		t.Fatalf("dataset_count = %v, want 2 (original request length)", queryInfo["dataset_count"])
	}
}

// flakyListDocuments fails the first list call then succeeds, modelling a
// transient lookup error.
type flakyListDocuments struct {
	*countingMCPDocuments
	failRemaining int
}

func (f *flakyListDocuments) ListDocumentsByDatasetIDWithOptions(ctx context.Context, opts dao.DocumentListOptions, page, pageSize int) ([]*entity.DocumentListItem, int64, error) {
	if f.failRemaining > 0 {
		f.failRemaining--
		return nil, 0, fmt.Errorf("transient list failure")
	}
	return f.countingMCPDocuments.ListDocumentsByDatasetIDWithOptions(ctx, opts, page, pageSize)
}

// TestMCPRetrievalRetriesTransientListFailure is the RED test for #8: a single
// transient list error must not drop enrichment for every document in the
// dataset; a bounded retry should recover it.
func TestMCPRetrievalRetriesTransientListFailure(t *testing.T) {
	probe := &recordingDatasetService{response: &service.SearchDatasetsResponse{
		Chunks: []map[string]interface{}{
			{"id": "c1", "dataset_id": "ds-1", "document_id": "doc-a", "document_keyword": "A"},
		},
		Total: 1,
	}}
	base := batchTestDocuments()
	docs := &flakyListDocuments{countingMCPDocuments: base, failRemaining: 1}

	result, err := mcpRetrievalWithDocs(permission.WithIdentity(t.Context(), "user-1", ""), probe, docs, mcp.RetrievalRequest{
		DatasetIDs: []string{"ds-1"}, Question: "q", Page: 1, PageSize: 10,
	})
	if err != nil {
		t.Fatalf("mcpRetrievalWithDocs failed: %v", err)
	}
	chunks := batchResultChunks(t, decodeBatchResult(t, result))
	if len(chunks) != 1 {
		t.Fatalf("chunks = %d, want 1", len(chunks))
	}
	if _, ok := chunks[0]["document_metadata"]; !ok {
		t.Fatalf("document_metadata missing after a transient list failure (no retry): %#v", chunks[0])
	}
}
