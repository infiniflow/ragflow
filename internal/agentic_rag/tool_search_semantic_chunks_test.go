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

package agentic_rag

import (
	"context"
	"strings"
	"testing"

	"gorm.io/gorm"

	"ragflow/internal/agent/runtime"
)

// capturingRetrievalService records every request a locate tool issues, so the
// dense-only contract is asserted at the boundary the engine actually sees —
// not at the tool's arguments.
type capturingRetrievalService struct {
	reqs   []runtime.RetrievalRequest
	chunks []runtime.RetrievalChunk
}

func (c *capturingRetrievalService) Search(_ context.Context, _ *gorm.DB, req runtime.RetrievalRequest) ([]runtime.RetrievalChunk, error) {
	c.reqs = append(c.reqs, req)
	return c.chunks, nil
}

func newCaptureService() *capturingRetrievalService {
	return &capturingRetrievalService{chunks: []runtime.RetrievalChunk{{
		ID:           "c1",
		Content:      "Dal Pescatore is a restaurant in Canneto sull'Oglio, Italy.",
		DocumentID:   "d1",
		DocumentName: "5426.md",
		Score:        0.83,
		DatasetID:    "kb1",
		PageNum:      1,
		ChunkIndex:   0,
	}}}
}

// TestSearchSemanticChunksToolAsksForTheDenseLegAlone pins the reason the tool
// exists: weight 0 on search_chunks is NOT a pure vector search (the engine
// still sends the BM25 clause and uses it as the kNN filter), so the request
// must carry VectorOnly — plus the two efficiency consequences it implies: no
// 4x candidate over-fetch and no fusion leg.
func TestSearchSemanticChunksToolAsksForTheDenseLegAlone(t *testing.T) {
	fake := newCaptureService()
	runtime.SetRetrievalService(fake)
	defer runtime.SetRetrievalService(nil)

	out, err := NewSearchSemanticChunksTool("t", []string{"kb1"}).InvokableRun(context.Background(),
		`{"queries":["an Italian family restaurant awarded three Michelin stars in 1996"]}`)
	if err != nil {
		t.Fatalf("invokable run: %v", err)
	}
	if len(fake.reqs) != 1 {
		t.Fatalf("requests = %d, want 1", len(fake.reqs))
	}
	req := fake.reqs[0]
	if !req.VectorOnly {
		t.Error("VectorOnly must be set: without it the engine filters kNN candidates by the query's own words")
	}
	if req.KeywordsSimilarityWeight == nil || *req.KeywordsSimilarityWeight != 0 {
		t.Errorf("keyword weight = %v, want 0", req.KeywordsSimilarityWeight)
	}
	if req.TopK != req.TopN {
		t.Errorf("TopK = %d, want TopN = %d: vector-only has no fusion pool to fill", req.TopK, req.TopN)
	}
	if !strings.Contains(out, `<search_results count="1"`) {
		t.Errorf("output is not the shared locate payload: %s", out)
	}
}

// TestSearchSemanticChunksToolSchemaHasNoWeightKnob keeps the fusion weight out
// of the model's reach: offering it would let the caller rebuild search_chunks
// by hand, which is the one thing this tool must not degrade into.
func TestSearchSemanticChunksToolSchemaHasNoWeightKnob(t *testing.T) {
	info, err := NewSearchSemanticChunksTool("t", nil).Info(context.Background())
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	if info.Name != searchSemanticChunksToolName {
		t.Fatalf("tool name = %q, want %q", info.Name, searchSemanticChunksToolName)
	}
	schema, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("schema: %v", err)
	}
	if schema.Properties != nil {
		if _, ok := schema.Properties.Get("keywords_similarity_weight"); ok {
			t.Error("the pure-vector tool must not expose keywords_similarity_weight")
		}
	}
	if !strings.Contains(info.Desc, "no keyword leg") {
		t.Error("the description must state that no keyword leg runs")
	}
}

// TestSearchChunksToolStillHybridAfterRefactor guards the shared-body
// extraction: the hybrid leg keeps its default 0.7 weight and its 4x candidate
// pool, and must never ask for the dense-only mode.
func TestSearchChunksToolStillHybridAfterRefactor(t *testing.T) {
	fake := newCaptureService()
	runtime.SetRetrievalService(fake)
	defer runtime.SetRetrievalService(nil)

	if _, err := NewSearchChunksTool("t", []string{"kb1"}).InvokableRun(context.Background(),
		`{"queries":["family business hallmark award"]}`); err != nil {
		t.Fatalf("invokable run: %v", err)
	}
	if len(fake.reqs) != 1 {
		t.Fatalf("requests = %d, want 1", len(fake.reqs))
	}
	req := fake.reqs[0]
	if req.VectorOnly {
		t.Error("search_chunks must stay hybrid: VectorOnly belongs to the semantic tool")
	}
	if req.KeywordsSimilarityWeight == nil || *req.KeywordsSimilarityWeight != searchChunksDefaultKeywordsSimilarityWeight {
		t.Errorf("keyword weight = %v, want the 0.7 default", req.KeywordsSimilarityWeight)
	}
	if req.TopK != req.TopN*4 {
		t.Errorf("TopK = %d, want 4x TopN = %d", req.TopK, req.TopN*4)
	}
}

// TestSearchSemanticChunksToolRejectsTooManyQueries keeps the shared body's
// declared bounds enforceable for the new leg too.
func TestSearchSemanticChunksToolRejectsTooManyQueries(t *testing.T) {
	runtime.SetRetrievalService(newCaptureService())
	defer runtime.SetRetrievalService(nil)

	out, err := NewSearchSemanticChunksTool("t", []string{"kb1"}).InvokableRun(context.Background(),
		`{"queries":["a","b","c","d","e","f"]}`)
	if err != nil {
		t.Fatalf("bounds violation must become a result, got error: %v", err)
	}
	if !strings.Contains(out, `<tool_error tool="search_semantic_chunks"`) || !strings.Contains(out, "at most 5") {
		t.Errorf("want a canonical tool_error naming the tool, got: %s", out)
	}
}

// TestZeroHitNextStepRoutesByLeg pins the behavioral trigger: a lexical 0-hit is
// the dead-end signal that should hand over to the meaning-based leg, while a
// semantic 0-hit must NOT invite another semantic retry of the same description
// (that is the loop the trigger exists to break).
func TestZeroHitNextStepRoutesByLeg(t *testing.T) {
	for _, tool := range []string{"grep_chunks", "search_bm25_chunks"} {
		hint := zeroHitNextStep(tool)
		if !strings.Contains(hint, "search_semantic_chunks") {
			t.Errorf("%s 0-hit hint must name the meaning-based leg, got: %s", tool, hint)
		}
		if !strings.Contains(hint, "CATEGORY") {
			t.Errorf("%s 0-hit hint must offer the category shift", tool)
		}
	}
	semantic := zeroHitNextStep(searchSemanticChunksToolName)
	if strings.Contains(semantic, "run ONE search_semantic_chunks") {
		t.Error("a semantic 0-hit must not ask for the same semantic retry")
	}
	if !strings.Contains(semantic, "CATEGORY") {
		t.Error("a semantic 0-hit must point at the category/description, not the vocabulary")
	}

	out := formatLocateResultsXML(context.Background(), "search_bm25_chunks", "some query", nil)
	if !strings.Contains(out, "LEXICAL DEAD-END") || !strings.Contains(out, "search_semantic_chunks") {
		t.Errorf("a 0-hit lexical result must carry the trigger, got: %s", out)
	}
}
