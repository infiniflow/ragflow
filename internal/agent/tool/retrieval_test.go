//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package tool

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"ragflow/internal/agent/runtime"
)

func TestRetrieval_StubsErrorWhenServiceMissing(t *testing.T) {
	t.Parallel()

	rt := NewRetrievalTool()
	out, err := rt.InvokableRun(context.Background(), `{"query":"hello"}`)
	if err == nil {
		t.Fatal("expected stub error, got nil")
	}
	if !errors.Is(err, ErrRetrievalServiceMissing) {
		t.Fatalf("err = %v, want ErrRetrievalServiceMissing", err)
	}

	// Output is a JSON envelope with the stub error message.
	var got retrievalResult
	if jerr := json.Unmarshal([]byte(out), &got); jerr != nil {
		t.Fatalf("output is not valid JSON: %v (raw=%s)", jerr, out)
	}
	if !got.Stub {
		t.Errorf("Stub = false, want true")
	}
	if !strings.Contains(got.Error, "service not yet implemented") {
		t.Errorf("Error = %q, want to mention 'service not yet implemented'", got.Error)
	}
}

func TestRetrieval_RejectsUseKG(t *testing.T) {
	t.Parallel()

	rt := NewRetrievalTool()
	out, err := rt.InvokableRun(context.Background(), `{"query":"x","use_kg":true}`)
	if !errors.Is(err, ErrGraphRAGNotSupported) {
		t.Fatalf("err = %v, want ErrGraphRAGNotSupported", err)
	}
	if !strings.Contains(out, "GraphRAG") {
		t.Errorf("output %q should mention GraphRAG", out)
	}
}

func TestRetrieval_InfoMatchesPythonMeta(t *testing.T) {
	t.Parallel()

	rt := NewRetrievalTool()
	info, err := rt.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Name != "search_my_dateset" {
		t.Errorf("Name = %q, want search_my_dateset (typo preserved)", info.Name)
	}
	if !strings.Contains(info.Desc, "datasets") {
		t.Errorf("Desc = %q, want to mention 'datasets'", info.Desc)
	}
	// The query param must be present and required. ToJSONSchema returns
	// a *jsonschema.Schema whose Properties is an *orderedmap.Map; we use
	// MarshalJSON to assert the parameter set without depending on the
	// map's concrete Get signature.
	params, err := info.ToJSONSchema()
	if err != nil {
		t.Fatalf("ToJSONSchema: %v", err)
	}
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	if !strings.Contains(string(raw), `"query"`) {
		t.Errorf("schema JSON does not contain 'query' key: %s", raw)
	}
	for _, nodeConfig := range []string{"dataset_ids", "kb_ids", "top_n", "top_k", "similarity_threshold", "keywords_similarity_weight", "use_kg"} {
		if strings.Contains(string(raw), `"`+nodeConfig+`"`) {
			t.Errorf("schema JSON exposes Canvas node config %q to the model: %s", nodeConfig, raw)
		}
	}
}

func TestRetrieval_EmptyArgsIsHandled(t *testing.T) {
	t.Parallel()

	rt := NewRetrievalTool()
	// Empty arguments should still return a stub error (not panic) — the
	// Python tool defaults to empty_response in this case. Without
	// wiring, the Go side surfaces the service-missing error.
	_, err := rt.InvokableRun(context.Background(), "")
	if !errors.Is(err, ErrRetrievalServiceMissing) {
		t.Fatalf("err = %v, want ErrRetrievalServiceMissing", err)
	}
}

func TestRetrieval_PassesTenantIDFromCanvasState(t *testing.T) {
	prev := GetRetrievalService()
	svc := &capturingRetrievalService{}
	SetRetrievalService(svc)
	t.Cleanup(func() { SetRetrievalService(prev) })

	state := runtime.NewCanvasState("run-1", "task-1")
	state.Sys["tenant_id"] = "tenant-1"
	ctx := runtime.WithState(context.Background(), state)

	rt := NewRetrievalTool()
	_, err := rt.InvokableRun(ctx, `{"query":"hello","dataset_ids":["kb-1"]}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if svc.req.TenantID != "tenant-1" {
		t.Fatalf("TenantID=%q want tenant-1", svc.req.TenantID)
	}
}

func TestRetrieval_PassesUserIDWhenTenantIDMissing(t *testing.T) {
	prev := GetRetrievalService()
	svc := &capturingRetrievalService{}
	SetRetrievalService(svc)
	t.Cleanup(func() { SetRetrievalService(prev) })

	state := runtime.NewCanvasState("run-1", "task-1")
	state.Sys["user_id"] = "user-1"
	ctx := runtime.WithState(context.Background(), state)

	rt := NewRetrievalTool()
	_, err := rt.InvokableRun(ctx, `{"query":"hello","dataset_ids":["kb-1"]}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if svc.req.TenantID != "user-1" {
		t.Fatalf("TenantID=%q want user-1", svc.req.TenantID)
	}
}

func TestRetrieval_UsesNodeParamsAsDefaults(t *testing.T) {
	prev := GetRetrievalService()
	svc := &capturingRetrievalService{}
	SetRetrievalService(svc)
	t.Cleanup(func() { SetRetrievalService(prev) })

	built, err := BuildByName("retrieval", map[string]any{
		"kb_ids":                     []any{"kb-1"},
		"top_n":                      float64(3),
		"top_k":                      float64(99),
		"keywords_similarity_weight": 0.7,
		"similarity_threshold":       0.42,
	})
	if err != nil {
		t.Fatalf("BuildByName(retrieval): %v", err)
	}
	rt, ok := built.(*RetrievalTool)
	if !ok {
		t.Fatalf("BuildByName(retrieval) returned %T, want *RetrievalTool", built)
	}

	_, err = rt.InvokableRun(context.Background(), `{"query":"hello"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if len(svc.req.DatasetIDs) != 1 || svc.req.DatasetIDs[0] != "kb-1" {
		t.Fatalf("DatasetIDs=%#v want [kb-1]", svc.req.DatasetIDs)
	}
	if svc.req.TopN != 3 {
		t.Fatalf("TopN=%d want 3", svc.req.TopN)
	}
	if svc.req.TopK != 99 {
		t.Fatalf("TopK=%d want 99", svc.req.TopK)
	}
	if svc.req.KeywordsSimilarityWeight == nil || *svc.req.KeywordsSimilarityWeight != 0.7 {
		t.Fatalf("KeywordsSimilarityWeight=%v want 0.7", svc.req.KeywordsSimilarityWeight)
	}
	if svc.req.SimilarityThreshold != 0.42 {
		t.Fatalf("SimilarityThreshold=%v want 0.42", svc.req.SimilarityThreshold)
	}
}

func TestRetrieval_IgnoresPythonOnlyNodeParams(t *testing.T) {
	t.Parallel()

	built, err := BuildByName("retrieval", map[string]any{
		"rerank_id":        "rerank-1",
		"toc_enhance":      true,
		"meta_data_filter": map[string]any{"method": "manual"},
		"empty_response":   "empty",
		"retrieval_from":   "database",
		"memory_ids":       []any{"memory-1"},
		"kb_vars":          map[string]any{"x": "y"},
		"cross_languages":  []any{"English"},
	})
	if err != nil {
		t.Fatalf("BuildByName(retrieval): %v", err)
	}
	rt, ok := built.(*RetrievalTool)
	if !ok {
		t.Fatalf("BuildByName(retrieval) returned %T, want *RetrievalTool", built)
	}
	if rt.defaults.TopN != 0 || rt.defaults.TopK != 0 || rt.defaults.KeywordsSimilarityWeight != nil {
		t.Fatalf("python-only params should not mutate retrieval defaults: %#v", rt.defaults)
	}
}

func TestRetrieval_IgnoresCanvasMetadataNodeParams(t *testing.T) {
	built, err := BuildByName("retrieval", map[string]any{
		"kb_ids":  []any{"kb-1"},
		"inputs":  map[string]any{"query": "upstream"},
		"outputs": map[string]any{"formalized_content": "downstream"},
	})
	if err != nil {
		t.Fatalf("BuildByName(retrieval): %v", err)
	}
	rt, ok := built.(*RetrievalTool)
	if !ok {
		t.Fatalf("BuildByName(retrieval) returned %T, want *RetrievalTool", built)
	}
	if len(rt.defaults.DatasetIDs) != 1 || rt.defaults.DatasetIDs[0] != "kb-1" {
		t.Fatalf("defaults.DatasetIDs=%#v want [kb-1]", rt.defaults.DatasetIDs)
	}
}

func TestRetrieval_ModelArgsOverrideNodeDatasetIDs(t *testing.T) {
	prev := GetRetrievalService()
	svc := &capturingRetrievalService{}
	SetRetrievalService(svc)
	t.Cleanup(func() { SetRetrievalService(prev) })

	rt := NewRetrievalToolWithDefaults(retrievalArgs{DatasetIDs: []string{"kb-default"}, TopN: 3})
	_, err := rt.InvokableRun(context.Background(), `{"query":"hello","dataset_ids":["kb-call"],"top_n":5}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if len(svc.req.DatasetIDs) != 1 || svc.req.DatasetIDs[0] != "kb-call" {
		t.Fatalf("DatasetIDs=%#v want [kb-call]", svc.req.DatasetIDs)
	}
	if svc.req.TopN != 5 {
		t.Fatalf("TopN=%d want 5", svc.req.TopN)
	}
}

func TestRetrieval_RecordsFrontendReferencePayload(t *testing.T) {
	prev := GetRetrievalService()
	SetRetrievalService(staticRetrievalService{chunks: []RetrievalChunk{
		{
			ID:               "ck-1",
			Content:          "answer",
			DocumentID:       "doc-1",
			DocumentName:     "paper.pdf",
			DatasetID:        "kb-1",
			ImageID:          "img-1",
			Positions:        [][]float64{{1, 2, 3, 4}},
			Score:            0.9,
			TermSimilarity:   0.7,
			VectorSimilarity: 0.8,
		},
	}})
	t.Cleanup(func() { SetRetrievalService(prev) })

	state := runtime.NewCanvasState("run-1", "task-1")
	ctx := runtime.WithState(context.Background(), state)

	rt := NewRetrievalTool()
	_, err := rt.InvokableRun(ctx, `{"query":"hello","dataset_ids":["kb-1"]}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}

	reference := state.GetRetrievalReference()
	chunks, _ := reference["chunks"].([]any)
	if len(chunks) != 1 {
		t.Fatalf("reference chunks length = %d, want 1", len(chunks))
	}
	chunk, _ := chunks[0].(map[string]any)
	if chunk["document_name"] != "paper.pdf" || chunk["image_id"] != "img-1" {
		t.Fatalf("reference chunk = %#v, want document_name/image_id", chunk)
	}
	docAggs, _ := reference["doc_aggs"].([]any)
	if len(docAggs) != 1 {
		t.Fatalf("reference doc_aggs length = %d, want 1", len(docAggs))
	}
	docAgg, _ := docAggs[0].(map[string]any)
	if docAgg["doc_id"] != "doc-1" || docAgg["doc_name"] != "paper.pdf" || docAgg["count"] != 1 {
		t.Fatalf("reference doc_agg = %#v, want doc metadata", docAgg)
	}
}

type capturingRetrievalService struct {
	req RetrievalRequest
}

func (s *capturingRetrievalService) Search(_ context.Context, req RetrievalRequest) ([]RetrievalChunk, error) {
	s.req = req
	return []RetrievalChunk{{ID: "ck-1", Content: "answer"}}, nil
}

type staticRetrievalService struct {
	chunks []RetrievalChunk
}

func (s staticRetrievalService) Search(_ context.Context, _ RetrievalRequest) ([]RetrievalChunk, error) {
	return s.chunks, nil
}
