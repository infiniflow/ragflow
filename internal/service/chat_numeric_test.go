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
	"encoding/json"
	"testing"
)

func TestNormalizeRESTChatNumericUpdates(t *testing.T) {
	tests := []struct {
		name    string
		field   string
		value   interface{}
		want    interface{}
		wantErr string
	}{
		{name: "integer", field: "top_n", value: float64(6), want: int64(6)},
		{name: "integer JSON number", field: "top_k", value: json.Number("9007199254740993"), want: int64(9007199254740993)},
		{name: "float", field: "similarity_threshold", value: 0.25, want: 0.25},
		{name: "invalid integer string", field: "top_n", value: "abc", wantErr: "`top_n` must be an integer"},
		{name: "fractional integer", field: "top_n", value: 1.5, wantErr: "`top_n` must be an integer"},
		{name: "boolean integer", field: "top_k", value: true, wantErr: "`top_k` must be an integer"},
		{name: "null float", field: "similarity_threshold", wantErr: "`similarity_threshold` must be a number"},
		{name: "invalid float string", field: "vector_similarity_weight", value: "abc", wantErr: "`vector_similarity_weight` must be a number"},
		{name: "overflow integer", field: "top_n", value: json.Number("9223372036854775808"), wantErr: "`top_n` must be an integer"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updates := map[string]interface{}{tt.field: tt.value}
			err := normalizeRESTChatNumericUpdates(updates)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := updates[tt.field]; got != tt.want {
				t.Fatalf("%s = %v (%T), want %v (%T)", tt.field, got, got, tt.want, tt.want)
			}
		})
	}
}

func TestBuildCreateChatEntityNumericFields(t *testing.T) {
	fields := map[string]interface{}{
		"similarity_threshold":     0.25,
		"vector_similarity_weight": json.Number("0.5"),
		"top_n":                    float64(6),
		"rerank_candidates_count":  int64(64),
		"top_k":                    json.Number("1024"),
	}
	chat, err := buildCreateChatEntity(fields, "tenant")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chat.SimilarityThreshold != 0.25 || chat.VectorSimilarityWeight != 0.5 ||
		chat.TopN != 6 || chat.RerankCandidatesCount != 64 || chat.TopK != 1024 {
		t.Fatalf("unexpected numeric fields: %+v", chat)
	}

	delete(fields, "top_k")
	if _, err := buildCreateChatEntity(fields, "tenant"); err == nil || err.Error() != "`top_k` must be an integer" {
		t.Fatalf("missing top_k error = %v", err)
	}

	fields["top_k"] = "abc"
	if _, err := buildCreateChatEntity(fields, "tenant"); err == nil || err.Error() != "`top_k` must be an integer" {
		t.Fatalf("invalid top_k error = %v", err)
	}
}
