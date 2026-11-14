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

package chunk

import (
	"math"
	"testing"

	"ragflow/internal/service"
)

func floatPtr(v float64) *float64 { return &v }

func intPtr(v int) *int { return &v }

// TestResolveSearchbotRetrievalParams_EndpointDefaults: no request value and
// no saved config leaves the endpoint defaults.
func TestResolveSearchbotRetrievalParams_EndpointDefaults(t *testing.T) {
	threshold, weight, topK := resolveSearchbotRetrievalParams(&service.RetrievalTestRequest{}, nil)
	if threshold != 0.0 {
		t.Errorf("similarityThreshold = %v, want 0.0", threshold)
	}
	if weight != 0.3 {
		t.Errorf("vectorSimilarityWeight = %v, want 0.3", weight)
	}
	if topK != 1024 {
		t.Errorf("topK = %d, want 1024", topK)
	}
}

// TestResolveSearchbotRetrievalParams_RequestWins: a request value beats the
// saved config and the endpoint default.
func TestResolveSearchbotRetrievalParams_RequestWins(t *testing.T) {
	req := &service.RetrievalTestRequest{
		SimilarityThreshold:    floatPtr(0.5),
		VectorSimilarityWeight: floatPtr(0.8),
		TopK:                   intPtr(100),
	}
	searchConfig := map[string]interface{}{
		"similarity_threshold":     0.1,
		"vector_similarity_weight": 0.9,
		"top_k":                    50,
	}
	threshold, weight, topK := resolveSearchbotRetrievalParams(req, searchConfig)
	if threshold != 0.5 {
		t.Errorf("similarityThreshold = %v, want 0.5", threshold)
	}
	if weight != 0.8 {
		t.Errorf("vectorSimilarityWeight = %v, want 0.8", weight)
	}
	if topK != 100 {
		t.Errorf("topK = %d, want 100", topK)
	}
}

// TestResolveSearchbotRetrievalParams_SavedConfigFallback: an omitted request
// value falls back to the saved config.
func TestResolveSearchbotRetrievalParams_SavedConfigFallback(t *testing.T) {
	searchConfig := map[string]interface{}{
		"similarity_threshold":     0.2,
		"vector_similarity_weight": 0.7,
		"top_k":                    200,
	}
	threshold, weight, topK := resolveSearchbotRetrievalParams(&service.RetrievalTestRequest{}, searchConfig)
	if threshold != 0.2 {
		t.Errorf("similarityThreshold = %v, want 0.2 from search_config", threshold)
	}
	if weight != 0.7 {
		t.Errorf("vectorSimilarityWeight = %v, want 0.7 from search_config", weight)
	}
	if topK != 200 {
		t.Errorf("topK = %d, want 200 from search_config", topK)
	}
}

// TestResolveSearchbotRetrievalParams_KeywordsWeightConfig: a config carrying
// only keywords_similarity_weight drives vector as 1 - keywords.
func TestResolveSearchbotRetrievalParams_KeywordsWeightConfig(t *testing.T) {
	searchConfig := map[string]interface{}{"keywords_similarity_weight": 0.7}
	_, weight, _ := resolveSearchbotRetrievalParams(&service.RetrievalTestRequest{}, searchConfig)
	if math.Abs(weight-0.3) > 1e-9 {
		t.Errorf("vectorSimilarityWeight = %v, want 0.3 (1 - 0.7)", weight)
	}
}

// TestResolveSearchbotRetrievalParams_ExplicitZeroWins: an explicit zero
// overrides the saved config — it is how a caller disables the threshold floor
// or selects a zero vector weight. Only a nil pointer means "omitted".
func TestResolveSearchbotRetrievalParams_ExplicitZeroWins(t *testing.T) {
	req := &service.RetrievalTestRequest{
		SimilarityThreshold:    floatPtr(0),
		VectorSimilarityWeight: floatPtr(0),
		TopK:                   intPtr(0),
	}
	searchConfig := map[string]interface{}{
		"similarity_threshold":     0.15,
		"vector_similarity_weight": 0.6,
		"top_k":                    300,
	}
	threshold, weight, topK := resolveSearchbotRetrievalParams(req, searchConfig)
	if threshold != 0 {
		t.Errorf("similarityThreshold = %v, want the explicit 0", threshold)
	}
	if weight != 0 {
		t.Errorf("vectorSimilarityWeight = %v, want the explicit 0", weight)
	}
	if topK != 0 {
		t.Errorf("topK = %d, want the explicit 0", topK)
	}
}

// TestResolveSearchbotRetrievalParams_PartialConfig: fields the saved config
// omits keep their endpoint defaults.
func TestResolveSearchbotRetrievalParams_PartialConfig(t *testing.T) {
	searchConfig := map[string]interface{}{"similarity_threshold": 0.4}
	threshold, weight, topK := resolveSearchbotRetrievalParams(&service.RetrievalTestRequest{}, searchConfig)
	if threshold != 0.4 {
		t.Errorf("similarityThreshold = %v, want 0.4 from search_config", threshold)
	}
	if weight != 0.3 {
		t.Errorf("vectorSimilarityWeight = %v, want 0.3 default", weight)
	}
	if topK != 1024 {
		t.Errorf("topK = %d, want 1024 default", topK)
	}
}
