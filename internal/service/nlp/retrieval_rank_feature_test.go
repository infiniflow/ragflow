//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package nlp

import (
	"context"
	"math"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine/types"
)

// projectingRetrievalEngine returns only the selected fields, like the real engines.
type projectingRetrievalEngine struct {
	retrievalCountEngine
}

func (e *projectingRetrievalEngine) Search(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	result, err := e.retrievalCountEngine.Search(ctx, req)
	if err != nil {
		return nil, err
	}
	fields := append([]string{"id"}, req.SelectFields...)
	chunks := make([]map[string]interface{}, len(result.Chunks))
	for i, row := range result.Chunks {
		chunks[i] = projectFields(row, fields)
	}
	return &types.SearchResult{Chunks: chunks, Total: result.Total}, nil
}

func (e *projectingRetrievalEngine) GetFields(chunks []map[string]interface{}, fields []string) map[string]map[string]interface{} {
	out := make(map[string]map[string]interface{}, len(chunks))
	for _, chunk := range chunks {
		if id, ok := chunk["id"].(string); ok {
			out[id] = projectFields(chunk, fields)
		}
	}
	return out
}

func projectFields(row map[string]interface{}, fields []string) map[string]interface{} {
	out := make(map[string]interface{}, len(fields))
	for _, field := range fields {
		if value, ok := row[field]; ok {
			out[field] = value
		}
	}
	return out
}

// All chunks share the vector score and all but "asked" the content, so any
// difference in similarity comes from pagerank_fea, tag_feas or question_tks.
func TestRetrievalRescoringReadsRankFeaturesAndQuestionTokens(t *testing.T) {
	oldQueryBuilder := globalQueryBuilder
	globalQueryBuilder = NewQueryBuilder()
	defer func() { globalQueryBuilder = oldQueryBuilder }()

	chunk := func(id string, extra map[string]interface{}) map[string]interface{} {
		row := map[string]interface{}{
			"id":                  id,
			"content_ltks":        "alpha",
			"content_with_weight": "alpha",
			"_score":              0.5,
		}
		for k, v := range extra {
			row[k] = v
		}
		return row
	}
	docEngine := &projectingRetrievalEngine{retrievalCountEngine{rows: []map[string]interface{}{
		chunk("plain", nil),
		chunk("ranked", map[string]interface{}{common.PAGERANK_FLD: float64(10)}),
		chunk("tagged", map[string]interface{}{common.TAG_FLD: map[string]interface{}{"finance": 1.0}}),
		chunk("asked", map[string]interface{}{"content_ltks": "omega", "question_tks": "alpha"}),
	}}}
	service := NewRetrievalService(docEngine, &dao.DocumentDAO{})

	candidates := 4
	threshold := 0.0
	vectorWeight := 0.3
	aggs := false
	rankFeature := map[string]float64{"finance": 5}
	result, err := service.Retrieval(t.Context(), &RetrievalRequest{
		Question:               "alpha",
		TenantIDs:              []string{"tenant-1"},
		Page:                   1,
		PageSize:               4,
		RerankCandidatesCount:  &candidates,
		SimilarityThreshold:    &threshold,
		VectorSimilarityWeight: &vectorWeight,
		RankFeature:            &rankFeature,
		Aggs:                   &aggs,
	})
	if err != nil {
		t.Fatalf("Retrieval failed: %v", err)
	}
	if len(result.Chunks) != 4 {
		t.Fatalf("chunk count = %d, want 4", len(result.Chunks))
	}

	sim := make(map[string]float64, len(result.Chunks))
	termSim := make(map[string]float64, len(result.Chunks))
	for _, c := range result.Chunks {
		id := c["chunk_id"].(string)
		sim[id] = c["similarity"].(float64)
		termSim[id] = c["term_similarity"].(float64)
	}

	if got := sim["ranked"] - sim["plain"]; math.Abs(got-10) > 1e-9 {
		t.Errorf("PageRank boost = %v, want 10 (similarities %v)", got, sim)
	}
	// Tag score: 10 * (5*1.0) / sqrt(1.0^2) / sqrt(5^2) = 10.
	if got := sim["tagged"] - sim["plain"]; math.Abs(got-10) > 1e-9 {
		t.Errorf("tag feature boost = %v, want 10 (similarities %v)", got, sim)
	}
	if termSim["asked"] < termSim["plain"] {
		t.Errorf("term similarity via question_tks = %v, want at least the content match %v", termSim["asked"], termSim["plain"])
	}
}
