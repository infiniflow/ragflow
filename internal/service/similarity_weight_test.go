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

package service

import (
	"math"
	"strings"
	"testing"
)

func TestResolveVectorSimilarityWeight(t *testing.T) {
	keywordWeight := 0.7
	vectorWeight := 0.3

	tests := []struct {
		name     string
		keywords *float64
		vector   *float64
		want     *float64
		wantErr  string
	}{
		{name: "keyword weight is preferred", keywords: &keywordWeight, want: float64Pointer(0.3)},
		{name: "vector weight remains supported", vector: &vectorWeight, want: &vectorWeight},
		{name: "matching weights are accepted", keywords: &keywordWeight, vector: &vectorWeight, want: float64Pointer(0.3)},
		{name: "missing weights stay missing"},
		{name: "mismatched weights are rejected", keywords: &keywordWeight, vector: float64Pointer(0.4), wantErr: "must sum to 1"},
		{name: "invalid keyword weight is rejected", keywords: float64Pointer(1.1), wantErr: "keywords_similarity_weight must be between 0 and 1"},
		{name: "invalid vector weight is rejected", vector: float64Pointer(-0.1), wantErr: "vector_similarity_weight must be between 0 and 1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveVectorSimilarityWeight(tt.keywords, tt.vector)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveVectorSimilarityWeight failed: %v", err)
			}
			if tt.want == nil {
				if got != nil {
					t.Fatalf("weight = %v, want nil", *got)
				}
				return
			}
			if got == nil || math.Abs(*got-*tt.want) > similarityWeightTolerance {
				t.Fatalf("weight = %v, want %v", got, *tt.want)
			}
		})
	}
}

func TestNormalizeSimilarityWeights(t *testing.T) {
	tests := []struct {
		name         string
		values       map[string]interface{}
		wantVector   float64
		wantKeywords float64
	}{
		{
			// 1 - 0.7 is 0.30000000000000004 in float64; the persisted pair
			// must be the clean 0.3 / 0.7.
			name:         "keyword weight derives an exact vector weight",
			values:       map[string]interface{}{"keywords_similarity_weight": 0.7},
			wantVector:   0.3,
			wantKeywords: 0.7,
		},
		{
			name:         "vector weight derives an exact keyword weight",
			values:       map[string]interface{}{"vector_similarity_weight": 0.3},
			wantVector:   0.3,
			wantKeywords: 0.7,
		},
		{
			name: "matching weights are persisted unchanged",
			values: map[string]interface{}{
				"keywords_similarity_weight": 0.7,
				"vector_similarity_weight":   0.3,
			},
			wantVector:   0.3,
			wantKeywords: 0.7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := NormalizeSimilarityWeights(tt.values); err != nil {
				t.Fatalf("NormalizeSimilarityWeights failed: %v", err)
			}
			if got := tt.values["vector_similarity_weight"].(float64); got != tt.wantVector {
				t.Fatalf("vector_similarity_weight = %v, want exactly %v", got, tt.wantVector)
			}
			if got := tt.values["keywords_similarity_weight"].(float64); got != tt.wantKeywords {
				t.Fatalf("keywords_similarity_weight = %v, want exactly %v", got, tt.wantKeywords)
			}
			if sum := tt.values["vector_similarity_weight"].(float64) + tt.values["keywords_similarity_weight"].(float64); sum != 1 {
				t.Fatalf("weights sum to %v, want exactly 1", sum)
			}
		})
	}
}

func float64Pointer(value float64) *float64 {
	return &value
}
