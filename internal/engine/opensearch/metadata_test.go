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

package opensearch

import (
	"testing"
)

func TestTotalHitsFromESResponse(t *testing.T) {
	tests := []struct {
		name    string
		resp    map[string]interface{}
		wantVal int64
		wantOk  bool
	}{
		{
			name:    "missing_hits",
			resp:    map[string]interface{}{},
			wantVal: 0,
			wantOk:  false,
		},
		{
			name:    "missing_total",
			resp:    map[string]interface{}{"hits": map[string]interface{}{}},
			wantVal: 0,
			wantOk:  false,
		},
		{
			name: "exact_total_under_cap",
			resp: map[string]interface{}{
				"hits": map[string]interface{}{
					"total": map[string]interface{}{"value": float64(42)},
				},
			},
			wantVal: 42,
			wantOk:  true,
		},
		{
			name: "exact_total_at_cap",
			resp: map[string]interface{}{
				"hits": map[string]interface{}{
					"total": map[string]interface{}{"value": float64(metaPushdownMaxSize)},
				},
			},
			wantVal: int64(metaPushdownMaxSize),
			wantOk:  true,
		},
		{
			name: "overflow_total_over_cap",
			resp: map[string]interface{}{
				"hits": map[string]interface{}{
					"total": map[string]interface{}{"value": float64(metaPushdownMaxSize + 1)},
				},
			},
			wantVal: int64(metaPushdownMaxSize + 1),
			wantOk:  true,
		},
		{
			name: "wrong_type",
			resp: map[string]interface{}{
				"hits": map[string]interface{}{
					"total": map[string]interface{}{"value": "10000"},
				},
			},
			wantVal: 0,
			wantOk:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotVal, gotOk := totalHitsFromESResponse(tt.resp)
			if gotOk != tt.wantOk {
				t.Errorf("ok mismatch: got %v want %v", gotOk, tt.wantOk)
			}
			if gotVal != tt.wantVal {
				t.Errorf("value mismatch: got %d want %d", gotVal, tt.wantVal)
			}
		})
	}
}

// TestPushdownCapSemantics documents the contract for the overflow path:
// when total > cap, the slice we'd build from ExtractDocIDs is necessarily
// a strict subset of the truth. FilterDocIdsByMetaPushdown must return
// nil in that case so the caller falls back to the in-memory meta_filter.
//
// This test exercises the response-shape contract directly rather than
// spinning up a real OpenSearch client; the integration is covered by
// the service-level tests in internal/service/metadata_filter_test.go.
func TestPushdownCapSemantics(t *testing.T) {
	// Build a response that LOOKS like a successful push-down but has
	// total > cap. totalHitsFromESResponse must report ok=true and the
	// value must exceed the cap, which is the trigger condition for
	// returning nil in FilterDocIdsByMetaPushdown.
	resp := map[string]interface{}{
		"hits": map[string]interface{}{
			"total": map[string]interface{}{"value": float64(metaPushdownMaxSize + 1)},
			"hits": []interface{}{
				map[string]interface{}{"_id": "doc-1"},
			},
		},
	}
	total, ok := totalHitsFromESResponse(resp)
	if !ok {
		t.Fatal("expected total to be parseable from a well-formed OpenSearch response")
	}
	if total <= int64(metaPushdownMaxSize) {
		t.Fatalf("expected total > cap, got total=%d cap=%d", total, metaPushdownMaxSize)
	}
}
