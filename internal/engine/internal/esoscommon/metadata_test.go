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

package esoscommon_test

import (
	"testing"

	"ragflow/internal/engine/internal/esoscommon"
)

// metaPushdownMaxSize mirrors the ES/OS metadata push-down cap. Its exact
// value is irrelevant to TotalHitsFromSearchResponse; only the boundary
// between under/at/over cap matters for these cases.
const metaPushdownMaxSize = 10000

func TestTotalHitsFromSearchResponse(t *testing.T) {
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
			gotVal, gotOk := esoscommon.TotalHitsFromSearchResponse(tt.resp)
			if gotOk != tt.wantOk {
				t.Errorf("ok mismatch: got %v want %v", gotOk, tt.wantOk)
			}
			if gotVal != tt.wantVal {
				t.Errorf("value mismatch: got %d want %d", gotVal, tt.wantVal)
			}
		})
	}
}
