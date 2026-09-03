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

	"gorm.io/gorm"
)

func TestRetrieval_StubsErrorWhenServiceMissing(t *testing.T) {
	ctx := t.Context()
	t.Parallel()

	rt := NewRetrievalTool()
	out, err := rt.InvokableRun(ctx, `{"query":"hello","dataset_ids":["kb-1"]}`)
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
	ctx := t.Context()
	t.Parallel()
	rt := NewRetrievalTool()
	out, err := rt.InvokableRun(ctx, `{"query":"x","use_kg":true}`)
	if !errors.Is(err, ErrGraphRAGNotSupported) {
		t.Fatalf("err = %v, want ErrGraphRAGNotSupported", err)
	}
	if !strings.Contains(out, "GraphRAG") {
		t.Errorf("output %q should mention GraphRAG", out)
	}
}

func TestRetrieval_InfoMatchesPythonMeta(t *testing.T) {
	ctx := t.Context()
	t.Parallel()

	rt := NewRetrievalTool()
	info, err := rt.Info(ctx)
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
	ctx := t.Context()
	t.Parallel()

	rt := NewRetrievalTool()
	out, err := rt.InvokableRun(ctx, "")
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	var result retrievalResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.FormalizedContent != "" {
		t.Fatalf("FormalizedContent = %q, want empty default", result.FormalizedContent)
	}
}

func TestRetrieval_PassesTenantIDFromCanvasState(t *testing.T) {
	prev := GetRetrievalService()
	svc := &capturingRetrievalService{}
	SetRetrievalService(svc)
	t.Cleanup(func() { SetRetrievalService(prev) })

	state := runtime.NewCanvasState("run-1", "task-1")
	state.Sys["tenant_id"] = "tenant-1"
	ctx := runtime.WithState(t.Context(), state)

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
	ctx := runtime.WithState(t.Context(), state)

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
		"rerank_id":                  "rerank@provider",
		"cross_languages":            []any{"English", "Chinese"},
		"toc_enhance":                true,
		"meta_data_filter":           map[string]any{"method": "manual"},
	})
	if err != nil {
		t.Fatalf("BuildByName(retrieval): %v", err)
	}
	rt, ok := built.(*RetrievalTool)
	if !ok {
		t.Fatalf("BuildByName(retrieval) returned %T, want *RetrievalTool", built)
	}

	ctx := t.Context()

	_, err = rt.InvokableRun(ctx, `{"query":"hello"}`)
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
	if svc.req.SimilarityThreshold == nil || *svc.req.SimilarityThreshold != 0.42 {
		t.Fatalf("SimilarityThreshold=%v want 0.42", svc.req.SimilarityThreshold)
	}
	if svc.req.RerankID != "rerank@provider" {
		t.Fatalf("RerankID=%q", svc.req.RerankID)
	}
	if len(svc.req.CrossLanguages) != 2 || svc.req.CrossLanguages[1] != "Chinese" {
		t.Fatalf("CrossLanguages=%#v", svc.req.CrossLanguages)
	}
	if !svc.req.TOCEnhance || svc.req.MetaDataFilter["method"] != "manual" {
		t.Fatalf("enhancement request=%#v", svc.req)
	}
}

func TestRetrieval_ExplicitZeroSimilarityArgsOverrideDefaults(t *testing.T) {
	previous := GetRetrievalService()
	service := &capturingRetrievalService{}
	SetRetrievalService(service)
	t.Cleanup(func() { SetRetrievalService(previous) })

	similarityThreshold := 0.42
	keywordsSimilarityWeight := 0.7
	retrievalTool := NewRetrievalToolWithDefaults(retrievalArgs{
		DatasetIDs:               []string{"kb-1"},
		SimilarityThreshold:      &similarityThreshold,
		KeywordsSimilarityWeight: &keywordsSimilarityWeight,
	})
	ctx := t.Context()
	_, err := retrievalTool.InvokableRun(ctx, `{"query":"hello","similarity_threshold":0,"keywords_similarity_weight":0}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if service.req.SimilarityThreshold == nil || *service.req.SimilarityThreshold != 0 {
		t.Fatalf("SimilarityThreshold = %v; want explicit zero", service.req.SimilarityThreshold)
	}
	if service.req.KeywordsSimilarityWeight == nil || *service.req.KeywordsSimilarityWeight != 0 {
		t.Fatalf("KeywordsSimilarityWeight = %v; want explicit zero", service.req.KeywordsSimilarityWeight)
	}
}

func TestRetrieval_ParsesEnhancementNodeParams(t *testing.T) {
	t.Parallel()

	built, err := BuildByName("retrieval", map[string]any{
		"rerank_id":        "rerank-1",
		"toc_enhance":      true,
		"meta_data_filter": map[string]any{"method": "manual"},
		"empty_response":   "empty",
		"retrieval_from":   "dataset",
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
	if rt.defaults.RerankID != "rerank-1" || !rt.defaults.TOCEnhance {
		t.Fatalf("enhancement defaults were not parsed: %#v", rt.defaults)
	}
	if len(rt.defaults.CrossLanguages) != 1 || rt.defaults.CrossLanguages[0] != "English" {
		t.Fatalf("CrossLanguages = %#v", rt.defaults.CrossLanguages)
	}
	if rt.defaults.MetaDataFilter["method"] != "manual" {
		t.Fatalf("MetaDataFilter = %#v", rt.defaults.MetaDataFilter)
	}
	if rt.defaults.EmptyResponse != "empty" {
		t.Fatalf("EmptyResponse = %q, want empty", rt.defaults.EmptyResponse)
	}
}

func TestRetrieval_UsesEmptyResponseForEmptyQuery(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	rt := NewRetrievalToolWithDefaults(retrievalArgs{EmptyResponse: "No query or result."})
	out, err := rt.InvokableRun(ctx, `{"query":""}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	var result retrievalResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.FormalizedContent != "No query or result." {
		t.Fatalf("FormalizedContent = %q", result.FormalizedContent)
	}
}

func TestRetrieval_RoutesMemoryRequests(t *testing.T) {
	previous := GetMemoryRetrievalService()
	memoryService := &capturingRetrievalService{}
	SetMemoryRetrievalService(memoryService)
	t.Cleanup(func() { SetMemoryRetrievalService(previous) })

	rt := NewRetrievalToolWithDefaults(retrievalArgs{
		MemoryIDs: []string{"memory-1"},
	})
	if _, err := rt.InvokableRun(t.Context(), `{"query":"remember this"}`); err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if memoryService.req.RetrievalFrom != "memory" {
		t.Fatalf("RetrievalFrom = %q, want memory", memoryService.req.RetrievalFrom)
	}
	if len(memoryService.req.MemoryIDs) != 1 || memoryService.req.MemoryIDs[0] != "memory-1" {
		t.Fatalf("MemoryIDs = %#v", memoryService.req.MemoryIDs)
	}
}

func TestRetrieval_ParsesUserIDNodeParam(t *testing.T) {
	t.Parallel()

	// A memory-mode Retrieval node declared as an Agent tool carries user_id
	// in its node-level params; building the tool must accept it.
	built, err := BuildByName("retrieval", map[string]any{
		"retrieval_from": "memory",
		"memory_ids":     []any{"memory-1"},
		"user_id":        "sys.user_id",
	})
	if err != nil {
		t.Fatalf("BuildByName(retrieval) with user_id: %v", err)
	}
	rt, ok := built.(*RetrievalTool)
	if !ok {
		t.Fatalf("BuildByName(retrieval) returned %T, want *RetrievalTool", built)
	}
	if rt.defaults.UserID != "sys.user_id" {
		t.Fatalf("UserID = %q, want sys.user_id", rt.defaults.UserID)
	}
}

func TestRetrieval_RoutesMemoryRequestsWithUserFilter(t *testing.T) {
	previous := GetMemoryRetrievalService()
	memoryService := &capturingRetrievalService{}
	SetMemoryRetrievalService(memoryService)
	t.Cleanup(func() { SetMemoryRetrievalService(previous) })

	state := runtime.NewCanvasState("run-1", "session-1")
	state.Sys["user_id"] = "user-42"
	ctx := runtime.WithState(t.Context(), state)

	rt := NewRetrievalToolWithDefaults(retrievalArgs{
		MemoryIDs: []string{"memory-1"},
		UserID:    "{sys.user_id}",
	})
	if _, err := rt.InvokableRun(ctx, `{"query":"remember this"}`); err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if memoryService.req.UserID != "user-42" {
		t.Fatalf("UserID = %q, want user-42 resolved from {sys.user_id}", memoryService.req.UserID)
	}

	// The bare `sys.user_id` form (no braces) resolves against the canvas
	// state as well.
	bare := NewRetrievalToolWithDefaults(retrievalArgs{
		MemoryIDs: []string{"memory-1"},
		UserID:    "sys.user_id",
	})
	if _, err := bare.InvokableRun(ctx, `{"query":"remember this"}`); err != nil {
		t.Fatalf("InvokableRun(bare ref): %v", err)
	}
	if memoryService.req.UserID != "user-42" {
		t.Fatalf("UserID = %q, want user-42 resolved from bare sys.user_id", memoryService.req.UserID)
	}
}

func TestRetrieval_UnsetBareUserRefYieldsNoFilter(t *testing.T) {
	previous := GetMemoryRetrievalService()
	memoryService := &capturingRetrievalService{}
	SetMemoryRetrievalService(memoryService)
	t.Cleanup(func() { SetMemoryRetrievalService(previous) })

	// sys.user_id is deliberately left unset: an unresolvable bare ref must
	// mean "no user filter" (UserID == ""), not a literal "sys.user_id"
	// filter that matches no user.
	state := runtime.NewCanvasState("run-1", "session-1")
	ctx := runtime.WithState(t.Context(), state)

	rt := NewRetrievalToolWithDefaults(retrievalArgs{
		MemoryIDs: []string{"memory-1"},
		UserID:    "sys.user_id",
	})
	if _, err := rt.InvokableRun(ctx, `{"query":"remember this"}`); err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if memoryService.req.UserID != "" {
		t.Fatalf("UserID = %q, want empty (no user filter) for unset sys.user_id", memoryService.req.UserID)
	}

	// The braced form of an unset ref keeps the loud failure — Python's
	// canvas.get_variable_value raises KeyError on missing globals.
	braced := NewRetrievalToolWithDefaults(retrievalArgs{
		MemoryIDs: []string{"memory-1"},
		UserID:    "{sys.user_id}",
	})
	if _, err := braced.InvokableRun(ctx, `{"query":"remember this"}`); err == nil {
		t.Fatal("InvokableRun(braced unset ref): expected error, got nil")
	}
}

func TestRetrieval_LiteralUserIDPassesThrough(t *testing.T) {
	previous := GetMemoryRetrievalService()
	memoryService := &capturingRetrievalService{}
	SetMemoryRetrievalService(memoryService)
	t.Cleanup(func() { SetMemoryRetrievalService(previous) })

	// sys.user_id is set to prove a non-ref literal is never substituted
	// from state.
	state := runtime.NewCanvasState("run-1", "session-1")
	state.Sys["user_id"] = "user-42"
	ctx := runtime.WithState(t.Context(), state)

	rt := NewRetrievalToolWithDefaults(retrievalArgs{
		MemoryIDs: []string{"memory-1"},
		UserID:    "user-77",
	})
	if _, err := rt.InvokableRun(ctx, `{"query":"remember this"}`); err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if memoryService.req.UserID != "user-77" {
		t.Fatalf("UserID = %q, want literal user-77", memoryService.req.UserID)
	}
}

func TestRetrieval_ResolvesCanvasVariables(t *testing.T) {
	state := runtime.NewCanvasState("run-1", "session-1")
	state.SetVar("source", "ids", []any{"kb-1", "kb-2"})
	state.SetVar("source", "query", "semantic question")
	state.SetVar("source", "value", "2026")
	ctx := runtime.WithState(t.Context(), state)

	ids, err := resolveRetrievalDatasetIDs(ctx, []string{"source@ids"})
	if err != nil {
		t.Fatalf("resolveRetrievalDatasetIDs: %v", err)
	}
	if len(ids) != 2 || ids[0] != "kb-1" || ids[1] != "kb-2" {
		t.Fatalf("resolved dataset IDs = %#v", ids)
	}
	query, err := resolveRetrievalQuery(ctx, "{{source@query}}")
	if err != nil || query != "semantic question" {
		t.Fatalf("resolved query = %q, err = %v", query, err)
	}
	filter, err := resolveRetrievalFilter(ctx, map[string]any{
		"method": "manual",
		"value":  "{{source@value}}",
	})
	if err != nil || filter["value"] != "2026" {
		t.Fatalf("resolved filter = %#v, err = %v", filter, err)
	}
}

func TestRetrieval_OmitsUnsetEmptyResponseFromArguments(t *testing.T) {
	arguments, err := json.Marshal(retrievalArgs{Query: "love"})
	if err != nil {
		t.Fatalf("marshal arguments: %v", err)
	}
	if strings.Contains(string(arguments), `"empty_response"`) {
		t.Fatalf("arguments unexpectedly include empty_response: %s", arguments)
	}
}

func TestRetrieval_UsesEmptyResponseWhenSearchHasNoChunks(t *testing.T) {
	ctx := t.Context()
	prev := GetRetrievalService()
	SetRetrievalService(staticRetrievalService{})
	t.Cleanup(func() { SetRetrievalService(prev) })

	rt := NewRetrievalToolWithDefaults(retrievalArgs{DatasetIDs: []string{"kb-1"}, EmptyResponse: "No matching chunk."})
	out, err := rt.InvokableRun(ctx, `{"query":"love"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	var result retrievalResult
	if err = json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.FormalizedContent != "No matching chunk." {
		t.Fatalf("FormalizedContent = %q", result.FormalizedContent)
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
	ctx := t.Context()
	prev := GetRetrievalService()
	svc := &capturingRetrievalService{}
	SetRetrievalService(svc)
	t.Cleanup(func() { SetRetrievalService(prev) })

	rt := NewRetrievalToolWithDefaults(retrievalArgs{DatasetIDs: []string{"kb-default"}, TopN: 3})
	_, err := rt.InvokableRun(ctx, `{"query":"hello","dataset_ids":["kb-call"],"top_n":5}`)
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
	ctx := runtime.WithState(t.Context(), state)

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

func (s *capturingRetrievalService) Search(_ context.Context, _ *gorm.DB, req RetrievalRequest) ([]RetrievalChunk, error) {
	s.req = req
	return []RetrievalChunk{{ID: "ck-1", Content: "answer"}}, nil
}

type staticRetrievalService struct {
	chunks []RetrievalChunk
}

func (s staticRetrievalService) Search(_ context.Context, _ *gorm.DB, _ RetrievalRequest) ([]RetrievalChunk, error) {
	return s.chunks, nil
}
