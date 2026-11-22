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

package infinity

import (
	"testing"

	"ragflow/internal/engine/types"
)

// TestResolveDenseEF pins the ef that Search hands to Infinity's MatchDense
// (#22161): without an explicit ef Infinity defaults it to topN, which
// silently loses recall on large HNSW graphs, so the engine floors it.
func TestResolveDenseEF(t *testing.T) {
	cases := []struct {
		name string
		expr *types.MatchDenseExpr
		topN int
		want int
	}{
		{"no expr defaults to floor", nil, 10, 200},
		{"no options defaults to floor", &types.MatchDenseExpr{}, 10, 200},
		{"topN above floor wins", nil, 500, 500},
		{"topN equal to floor", nil, 200, 200},
		{"explicit int ef wins", &types.MatchDenseExpr{ExtraOptions: map[string]interface{}{"ef": 400}}, 10, 400},
		{"explicit float ef wins", &types.MatchDenseExpr{ExtraOptions: map[string]interface{}{"ef": 400.0}}, 10, 400},
		{"explicit string ef wins", &types.MatchDenseExpr{ExtraOptions: map[string]interface{}{"ef": "400"}}, 10, 400},
		{"explicit ef below topN is raised to topN", &types.MatchDenseExpr{ExtraOptions: map[string]interface{}{"ef": 50}}, 300, 300},
		{"zero ef falls back to floor", &types.MatchDenseExpr{ExtraOptions: map[string]interface{}{"ef": 0}}, 10, 200},
		{"negative ef falls back to floor", &types.MatchDenseExpr{ExtraOptions: map[string]interface{}{"ef": -5}}, 10, 200},
		{"non-numeric ef falls back to floor", &types.MatchDenseExpr{ExtraOptions: map[string]interface{}{"ef": "fast"}}, 10, 200},
		{"unrelated options keep floor", &types.MatchDenseExpr{ExtraOptions: map[string]interface{}{"similarity": 0.7}}, 10, 200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveDenseEF(tc.expr, tc.topN); got != tc.want {
				t.Fatalf("resolveDenseEF(%v, %d) = %d, want %d", tc.expr, tc.topN, got, tc.want)
			}
		})
	}
}
