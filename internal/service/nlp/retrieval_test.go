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

package nlp

import (
	"context"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	modelModule "ragflow/internal/entity/models"
)

func TestBuildInfinityFusionExprUsesVectorSimilarityWeight(t *testing.T) {
	tests := []struct {
		name                   string
		vectorSimilarityWeight *float64
		expectedWeights        string
	}{
		{name: "default", vectorSimilarityWeight: nil, expectedWeights: "0.7,0.3"},
		{name: "text only", vectorSimilarityWeight: float64Ptr(0), expectedWeights: "1,0"},
		{name: "balanced", vectorSimilarityWeight: float64Ptr(0.5), expectedWeights: "0.5,0.5"},
		{name: "vector only", vectorSimilarityWeight: float64Ptr(1), expectedWeights: "0,1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr := buildInfinityFusionExpr(10, tt.vectorSimilarityWeight)
			if expr.Method != "weighted_sum" {
				t.Fatalf("expected weighted_sum, got %q", expr.Method)
			}
			if expr.TopN != 10 {
				t.Fatalf("expected TopN=10, got %d", expr.TopN)
			}
			weights, ok := expr.FusionParams["weights"].(string)
			if !ok || weights != tt.expectedWeights {
				t.Fatalf("expected weights=%q, got %v", tt.expectedWeights, expr.FusionParams["weights"])
			}
		})
	}
}

func float64Ptr(value float64) *float64 {
	return &value
}

func intPtr(value int) *int {
	return &value
}

type captureSearchDocEngine struct {
	engine.DocEngine
	searchRequest *types.SearchRequest
	result        *types.SearchResult
}

func (e *captureSearchDocEngine) Search(_ context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	e.searchRequest = req
	if e.result != nil {
		return e.result, nil
	}
	return &types.SearchResult{
		Chunks: []map[string]interface{}{{"id": "chunk-1"}},
		Total:  1,
	}, nil
}

func (e *captureSearchDocEngine) GetChunkIDs(_ []map[string]interface{}) []string {
	return []string{"chunk-1"}
}

func (e *captureSearchDocEngine) GetFields(_ []map[string]interface{}, _ []string) map[string]map[string]interface{} {
	return map[string]map[string]interface{}{}
}

func (e *captureSearchDocEngine) GetAggregation(_ []map[string]interface{}, _ string) []map[string]interface{} {
	return []map[string]interface{}{}
}

func (e *captureSearchDocEngine) GetHighlight(_ []map[string]interface{}, _ []string, _ string) map[string]string {
	return nil
}

type captureEmbeddingDriver struct {
	modelModule.ModelDriver
}

func (d *captureEmbeddingDriver) Embed(_ context.Context, _ *string, _ []string, _ *modelModule.APIConfig, _ *modelModule.EmbeddingConfig, _ *common.ModelUsage) ([]modelModule.EmbeddingData, error) {
	return []modelModule.EmbeddingData{{Embedding: []float64{0.1, 0.2}}}, nil
}

func TestSearchPassesVectorSimilarityWeightToFusionExpr(t *testing.T) {
	if GetQueryBuilder() == nil {
		globalQueryBuilder = NewQueryBuilder()
	}

	vectorWeight := 0.8
	docEngine := &captureSearchDocEngine{}
	service := NewRetrievalService(docEngine, nil)

	_, err := service.Search(context.Background(), &RetrievalSearchRequest{
		Question:               "test question",
		TenantIDs:              []string{"tenant-1"},
		KbIDs:                  []string{"kb-1"},
		Page:                   1,
		PageSize:               10,
		Top:                    10,
		RankFeature:            map[string]float64{},
		EmbeddingModel:         &modelModule.EmbeddingModel{ModelDriver: &captureEmbeddingDriver{}},
		VectorSimilarityWeight: &vectorWeight,
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if docEngine.searchRequest == nil || len(docEngine.searchRequest.MatchExprs) != 3 {
		t.Fatalf("expected three match expressions, got %#v", docEngine.searchRequest)
	}
	fusionExpr, ok := docEngine.searchRequest.MatchExprs[2].(*types.FusionExpr)
	if !ok {
		t.Fatalf("expected third match expression to be FusionExpr, got %T", docEngine.searchRequest.MatchExprs[2])
	}
	if got := fusionExpr.FusionParams["weights"]; got != "0.2,0.8" {
		t.Fatalf("expected weights=0.2,0.8, got %v", got)
	}
}

func TestRetrievalPassesVectorSimilarityWeightToSearch(t *testing.T) {
	if GetQueryBuilder() == nil {
		globalQueryBuilder = NewQueryBuilder()
	}

	vectorWeight := 0.8
	top := 10
	docEngine := &captureSearchDocEngine{
		result: &types.SearchResult{Chunks: []map[string]interface{}{}, Total: 0},
	}
	service := NewRetrievalService(docEngine, &dao.DocumentDAO{})

	_, err := service.Retrieval(context.Background(), &RetrievalRequest{
		Question:               "test question",
		TenantIDs:              []string{"tenant-1"},
		KbIDs:                  []string{"kb-1"},
		Page:                   1,
		PageSize:               10,
		Top:                    intPtr(top),
		RankFeature:            &map[string]float64{},
		EmbeddingModel:         &modelModule.EmbeddingModel{ModelDriver: &captureEmbeddingDriver{}},
		VectorSimilarityWeight: &vectorWeight,
	})
	if err != nil {
		t.Fatalf("Retrieval failed: %v", err)
	}

	if docEngine.searchRequest == nil || len(docEngine.searchRequest.MatchExprs) != 3 {
		t.Fatalf("expected three match expressions, got %#v", docEngine.searchRequest)
	}
	fusionExpr, ok := docEngine.searchRequest.MatchExprs[2].(*types.FusionExpr)
	if !ok {
		t.Fatalf("expected third match expression to be FusionExpr, got %T", docEngine.searchRequest.MatchExprs[2])
	}
	if got := fusionExpr.FusionParams["weights"]; got != "0.2,0.8" {
		t.Fatalf("expected weights=0.2,0.8, got %v", got)
	}
}
