// Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package nlp

import (
	"math"
	"testing"

	"ragflow/internal/common"
)

// rankFeatureCase drives the same document shape through both rank-feature
// entry points. The document PageRank is fixed at 0.25 by chunk(); the query
// weight for common.PAGERANK_FLD is deliberately large (7.0) so using it as a
// multiplier on the document PageRank would produce a visibly wrong score.
type rankFeatureCase struct {
	name        string
	tags        interface{}
	rankFeature map[string]float64
	base        float64
	want        float64
}

func (tc rankFeatureCase) chunk() map[string]interface{} {
	return map[string]interface{}{
		common.TAG_FLD:      tc.tags,
		common.PAGERANK_FLD: 0.25,
	}
}

func rankFeatureCases() []rankFeatureCase {
	weightedTags := map[string]float64{"tag_a": 2.0, "tag_b": 1.0, common.PAGERANK_FLD: 7.0}
	// tag_score = (2*0.6 + 1*0.8) / |(0.6, 0.8)| / |(2, 1)|
	//           = 2.0 / 1.0 / sqrt(5)
	// final     = base + tag_score*10 + docPagerank
	tagged := 0.3 + (2.0/math.Sqrt(5.0))*10 + 0.25
	baseOnly := 0.3 + 0.25

	return []rankFeatureCase{
		{
			name:        "json_string_tag_feas",
			tags:        `{"tag_a": 0.6, "tag_b": 0.8}`,
			rankFeature: weightedTags,
			base:        0.3,
			want:        tagged,
		},
		{
			name:        "go_map_tag_feas",
			tags:        map[string]interface{}{"tag_a": 0.6, "tag_b": 0.8},
			rankFeature: weightedTags,
			base:        0.3,
			want:        tagged,
		},
		{
			name:        "invalid_json_tag_feas",
			tags:        `{"tag_a": 0.6,`,
			rankFeature: weightedTags,
			base:        0.3,
			want:        baseOnly,
		},
		{
			name:        "query_pagerank_only",
			tags:        map[string]interface{}{"tag_a": 0.6, "tag_b": 0.8},
			rankFeature: map[string]float64{common.PAGERANK_FLD: 7.0},
			base:        0.3,
			want:        baseOnly,
		},
		{
			name:        "document_tags_all_zero",
			tags:        map[string]interface{}{"tag_a": 0.0, "tag_b": 0.0},
			rankFeature: weightedTags,
			base:        0.3,
			want:        baseOnly,
		},
	}
}

// TestApplyRankFeatureScores_SameDocumentBothEntryPoints covers the chunk-based
// and ID-based scoring paths with identical inputs, each invocation receiving
// a fresh similarity slice because both functions mutate it in place.
func TestApplyRankFeatureScores_SameDocumentBothEntryPoints(t *testing.T) {
	for _, tc := range rankFeatureCases() {
		tc := tc
		t.Run(tc.name+"/chunks", func(t *testing.T) {
			sim := []float64{tc.base}
			got := applyRankFeatureScores([]map[string]interface{}{tc.chunk()}, sim, tc.rankFeature)
			assertRankFeatureScores(t, got, []float64{tc.want})
		})
		t.Run(tc.name+"/ids", func(t *testing.T) {
			const id = "c1"
			sim := []float64{tc.base}
			field := map[string]map[string]interface{}{id: tc.chunk()}
			got := applyRankFeatureScoresForIDs([]string{id}, field, sim, tc.rankFeature)
			assertRankFeatureScores(t, got, []float64{tc.want})
		})
	}
}

// TestApplyRankFeatureScoresForIDs_AlignsToRequestedIDs pins ID-to-score
// alignment: the field map is keyed independently of ids, so a positional read
// from field instead of a lookup by chunk ID would attach the wrong tag and
// PageRank to each result.
func TestApplyRankFeatureScoresForIDs_AlignsToRequestedIDs(t *testing.T) {
	ids := []string{"c3", "c1", "c2"}
	field := map[string]map[string]interface{}{
		"c1": {
			common.TAG_FLD:      map[string]interface{}{"tag_a": 0.6, "tag_b": 0.8},
			common.PAGERANK_FLD: 0.25,
		},
		"c2": {
			common.TAG_FLD:      map[string]interface{}{"tag_a": 0.3, "tag_b": 0.4},
			common.PAGERANK_FLD: 0.5,
		},
		"c3": {
			common.TAG_FLD:      map[string]interface{}{"tag_a": 0.9, "tag_b": 0.1},
			common.PAGERANK_FLD: 0.1,
		},
	}
	rankFeature := map[string]float64{"tag_a": 2.0, "tag_b": 1.0, common.PAGERANK_FLD: 7.0}
	sim := []float64{0.11, 0.22, 0.33}

	got := applyRankFeatureScoresForIDs(ids, field, sim, rankFeature)

	want := []float64{
		0.11 + ((2*0.9+1*0.1)/math.Hypot(0.9, 0.1)/math.Sqrt(5.0))*10 + 0.1,
		0.22 + ((2*0.6+1*0.8)/math.Hypot(0.6, 0.8)/math.Sqrt(5.0))*10 + 0.25,
		0.33 + ((2*0.3+1*0.4)/math.Hypot(0.3, 0.4)/math.Sqrt(5.0))*10 + 0.5,
	}
	assertRankFeatureScores(t, got, want)
}

func assertRankFeatureScores(t *testing.T, got, want []float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("length = %d, want %d (got %v)", len(got), len(want), got)
	}
	for i, v := range got {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Fatalf("score[%d] = %v, want finite value", i, v)
		}
		if math.Abs(v-want[i]) > 1e-9 {
			t.Errorf("score[%d] = %v, want %v (all got %v)", i, v, want[i], got)
		}
	}
}
