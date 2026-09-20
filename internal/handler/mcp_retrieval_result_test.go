package handler

import (
	"encoding/json"
	"reflect"
	"testing"

	"ragflow/internal/mcp"
	"ragflow/internal/service"
)

func TestMCPRetrievalResult(t *testing.T) {
	req := mcp.RetrievalRequest{Question: "question", Page: 2, PageSize: 10, SimilarityThreshold: .2, VectorSimilarityWeight: .3, Keyword: true}
	raw := map[string]any{"id": "chunk", "dataset_id": "dataset", "document_id": "doc", "document_keyword": "file.txt", "custom": 42}
	metadata := map[string]map[string]any{"doc": {"document_id": "doc", "meta_fields": map[string]any{"author": "Ada"}}}
	result := mcpRetrievalResult(req, []string{"dataset"}, &service.SearchDatasetsResponse{Chunks: []map[string]any{raw}, Total: 21}, map[string]string{"dataset": "Dataset"}, metadata)
	b, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	chunk := got["chunks"].([]any)[0].(map[string]any)
	if chunk["dataset_name"] != "Dataset" || chunk["document_name"] != "file.txt" || chunk["custom"] != float64(42) || chunk["document_metadata"] == nil {
		t.Fatalf("chunk: %s", b)
	}
	if _, ok := raw["dataset_name"]; ok {
		t.Fatal("mutated service response")
	}
	if !reflect.DeepEqual(got["pagination"], map[string]any{"page": float64(2), "page_size": float64(10), "total_chunks": float64(21), "total_pages": float64(3)}) {
		t.Fatalf("pagination: %s", b)
	}
	if got["query_info"].(map[string]any)["vector_weight"] != .3 {
		t.Fatalf("query: %s", b)
	}
	empty := mcpRetrievalResult(req, []string{"dataset"}, &service.SearchDatasetsResponse{}, nil, nil)
	b, _ = json.Marshal(empty)
	if empty["chunks"] == nil || string(b) == "" {
		t.Fatalf("empty: %s", b)
	}
}
