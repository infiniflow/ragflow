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

// wantRankFeature computes the expected final score for one chunk independently
// of applyRankFeatureScores. It mirrors the documented formula:
//
//	qDenor = sqrt(sum(query_weight^2, excluding pagerank_fea))
//	tag_score = sum(qw*sc for known tags) / sqrt(sum(sc^2)) / qDenor
//	final = sim + tag_score*10 + pagerank
func wantRankFeature(
	sim float64,
	tagFeas map[string]float64,
	pagerank float64,
	rankFeature map[string]float64,
) float64 {
	qDenor := 0.0
	for k, w := range rankFeature {
		if k == common.PAGERANK_FLD {
			continue
		}
		qDenor += w * w
	}
	qDenor = math.Sqrt(qDenor)

	tagScore := 0.0
	if qDenor != 0 {
		var nor, denor float64
		for k, sc := range tagFeas {
			if w, ok := rankFeature[k]; ok && k != common.PAGERANK_FLD {
				nor += w * sc
			}
			denor += sc * sc
		}
		if denor != 0 {
			tagScore = nor / math.Sqrt(denor) / qDenor
		}
	}
	return sim + tagScore*10 + pagerank
}

func TestApplyRankFeatureScores_EmptyInputs(t *testing.T) {
	if got := applyRankFeatureScores(nil, nil, map[string]float64{"a": 1}); len(got) != 0 {
		t.Fatalf("nil chunks -> %v, want empty", got)
	}
	sim := []float64{1, 2}
	if got := applyRankFeatureScores(nil, sim, nil); !floatsClose(got, sim, 0) {
		t.Fatalf("nil chunks with scores -> %v, want unchanged %v", got, sim)
	}
	if got := applyRankFeatureScores([]map[string]interface{}{{}}, nil, nil); len(got) != 0 {
		t.Fatalf("nil sim -> %v, want empty", got)
	}
}

func TestApplyRankFeatureScores_PagerankOnly(t *testing.T) {
	chunks := []map[string]interface{}{
		{common.PAGERANK_FLD: 2.5},
		{common.PAGERANK_FLD: 1},
		{}, // missing pagerank counts as 0
	}
	sim := []float64{1, 1, 1}
	got := applyRankFeatureScores(chunks, sim, nil)
	want := []float64{3.5, 2, 1}
	if !floatsClose(got, want, 1e-9) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// TestApplyRankFeatureScores_StringifiedPagerank pins the engine field
// projection: GetFields stringifies non-string values, so a chunk's pagerank
// reaches the reranker as "7" and must still count (Python coerces it through
// np.array(..., dtype=float)).
func TestApplyRankFeatureScores_StringifiedPagerank(t *testing.T) {
	chunks := []map[string]interface{}{
		{common.PAGERANK_FLD: "7"},
		{common.PAGERANK_FLD: 3},
	}
	got := applyRankFeatureScores(chunks, []float64{1, 1}, nil)
	want := []float64{8, 4}
	if !floatsClose(got, want, 1e-9) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// TestApplyRankFeatureScoresForIDs_ProjectionShapes mirrors the ES field map as
// the reranker actually receives it: numbers stringified and tag_feas rendered
// as JSON.
func TestApplyRankFeatureScoresForIDs_ProjectionShapes(t *testing.T) {
	field := map[string]map[string]interface{}{
		"a": {common.TAG_FLD: `{"tag_a":0.5}`, common.PAGERANK_FLD: "5"},
		"b": {common.PAGERANK_FLD: "0"},
	}
	rankFeature := map[string]float64{"tag_a": 1}
	got := applyRankFeatureScoresForIDs([]string{"a", "b"}, field, []float64{0.2, 0.2}, rankFeature)
	want := []float64{
		wantRankFeature(0.2, map[string]float64{"tag_a": 0.5}, 5, rankFeature),
		0.2,
	}
	if !floatsClose(got, want, 1e-9) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestApplyRankFeatureScores_PagerankOnlyQueryNoDivZero(t *testing.T) {
	chunks := []map[string]interface{}{{common.PAGERANK_FLD: 10}}
	sim := []float64{0.5}
	got := applyRankFeatureScores(chunks, sim, map[string]float64{common.PAGERANK_FLD: 10})
	want := []float64{10.5}
	if !floatsClose(got, want, 1e-9) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for _, v := range got {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Fatalf("non-finite score leaked: %v", got)
		}
	}
}

func TestApplyRankFeatureScores_Formula(t *testing.T) {
	rankFeature := map[string]float64{"tag_a": 2, "tag_b": 1}
	chunks := []map[string]interface{}{
		{
			common.TAG_FLD:      map[string]interface{}{"tag_a": 0.5, "tag_b": 2.0, "unknown": 1.0},
			common.PAGERANK_FLD: 0.5,
		},
		{common.TAG_FLD: map[string]float64{"tag_a": 1}},
	}
	sim := []float64{0.1, 0.2}
	want := []float64{
		wantRankFeature(0.1, map[string]float64{"tag_a": 0.5, "tag_b": 2.0, "unknown": 1.0}, 0.5, rankFeature),
		wantRankFeature(0.2, map[string]float64{"tag_a": 1}, 0, rankFeature),
	}
	got := applyRankFeatureScores(chunks, sim, rankFeature)
	if !floatsClose(got, want, 1e-9) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestApplyRankFeatureScores_UnknownTagOnlyDenor(t *testing.T) {
	rankFeature := map[string]float64{"tag_a": 1}
	// "unknown" must contribute to denor but not to nor.
	withUnknown := applyRankFeatureScores(
		[]map[string]interface{}{{common.TAG_FLD: map[string]float64{"tag_a": 1, "unknown": 1}}},
		[]float64{0},
		rankFeature,
	)
	knownOnly := applyRankFeatureScores(
		[]map[string]interface{}{{common.TAG_FLD: map[string]float64{"tag_a": 1}}},
		[]float64{0},
		rankFeature,
	)
	if withUnknown[0] >= knownOnly[0] {
		t.Fatalf("unknown tag raised the score: %v vs %v", withUnknown, knownOnly)
	}
}

func TestApplyRankFeatureScores_PagerankExcludedFromQDenor(t *testing.T) {
	chunks := []map[string]interface{}{{common.TAG_FLD: map[string]float64{"tag_a": 1}}}
	sim := []float64{0}
	for name, rf := range map[string]map[string]float64{
		"tag only":      {"tag_a": 1},
		"tag plus page": {"tag_a": 1, common.PAGERANK_FLD: 1000},
	} {
		t.Run(name, func(t *testing.T) {
			got := applyRankFeatureScores(chunks, append([]float64(nil), sim...), rf)
			want := wantRankFeature(0, map[string]float64{"tag_a": 1}, 0, rf)
			if math.Abs(got[0]-want) > 1e-9 {
				t.Fatalf("got %v, want %v (pagerank must not enter qDenor)", got[0], want)
			}
		})
	}
}

func TestApplyRankFeatureScores_TagFeasFormats(t *testing.T) {
	rankFeature := map[string]float64{"tag_a": 2}
	cases := []struct {
		name string
		form interface{}
		feas map[string]float64
	}{
		{"map interface", map[string]interface{}{"tag_a": 0.5}, map[string]float64{"tag_a": 0.5}},
		{"map float64", map[string]float64{"tag_a": 0.5}, map[string]float64{"tag_a": 0.5}},
		{"map int", map[string]int{"tag_a": 1}, map[string]float64{"tag_a": 1}},
		{"json string", `{"tag_a": 0.5}`, map[string]float64{"tag_a": 0.5}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := wantRankFeature(0, tc.feas, 0, rankFeature)
			got := applyRankFeatureScores(
				[]map[string]interface{}{{common.TAG_FLD: tc.form}},
				[]float64{0},
				rankFeature,
			)
			if math.Abs(got[0]-want) > 1e-9 {
				t.Fatalf("got %v, want %v", got[0], want)
			}
		})
	}
}

func TestApplyRankFeatureScores_ZeroDenominator(t *testing.T) {
	cases := map[string]interface{}{
		"empty map":  map[string]float64{},
		"all zero":   map[string]float64{"tag_a": 0},
		"absent":     nil,
		"empty json": "{}",
	}
	rankFeature := map[string]float64{"tag_a": 1}
	for name, tagFeas := range cases {
		t.Run(name, func(t *testing.T) {
			chunk := map[string]interface{}{common.PAGERANK_FLD: 1.5}
			if tagFeas != nil {
				chunk[common.TAG_FLD] = tagFeas
			}
			got := applyRankFeatureScores([]map[string]interface{}{chunk}, []float64{0.5}, rankFeature)
			if math.Abs(got[0]-2.0) > 1e-9 {
				t.Fatalf("got %v, want pagerank-only 2.0", got[0])
			}
		})
	}
}

func TestApplyRankFeatureScores_NoNaNOrInf(t *testing.T) {
	rankFeature := map[string]float64{"tag_a": 2, common.PAGERANK_FLD: 10}
	chunks := []map[string]interface{}{
		{common.PAGERANK_FLD: 1},
		{common.TAG_FLD: map[string]float64{"tag_a": 3}, common.PAGERANK_FLD: 0},
		{common.TAG_FLD: map[string]float64{"tag_b": 3}},
	}
	got := applyRankFeatureScores(chunks, []float64{0.1, 0.2, 0.3}, rankFeature)
	for i, v := range got {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Fatalf("score[%d] is non-finite: %v", i, got)
		}
	}
}

func TestApplyRankFeatureScores_LengthMismatch(t *testing.T) {
	chunks := []map[string]interface{}{{common.PAGERANK_FLD: 1}}
	sim := []float64{0, 5}
	got := applyRankFeatureScores(chunks, sim, nil)
	if !floatsClose(got, []float64{1, 5}, 1e-9) {
		t.Fatalf("got %v, want [1 5] (trailing score preserved)", got)
	}
}

func TestApplyRankFeatureScoresForIDs_MissingAndEquivalent(t *testing.T) {
	rankFeature := map[string]float64{"tag_a": 2}
	chunks := []map[string]interface{}{
		{common.TAG_FLD: map[string]float64{"tag_a": 1}, common.PAGERANK_FLD: 0.5},
		{common.PAGERANK_FLD: 1},
	}
	ids := []string{"a", "missing"}
	field := map[string]map[string]interface{}{"a": chunks[0]}
	sim := []float64{0.1, 0.2}

	byIDs := applyRankFeatureScoresForIDs(ids, field, append([]float64(nil), sim...), rankFeature)
	// The missing id is scored as pagerank 0, tag 0.
	if math.Abs(byIDs[1]-0.2) > 1e-9 {
		t.Fatalf("missing id score = %v, want 0.2", byIDs[1])
	}

	byChunks := applyRankFeatureScores(chunks, append([]float64(nil), sim...), rankFeature)
	if math.Abs(byIDs[0]-byChunks[0]) > 1e-9 {
		t.Fatalf("ids version = %v, chunks version = %v", byIDs[0], byChunks[0])
	}
}

func TestApplyRankFeatureScoresForIDs_LengthMismatch(t *testing.T) {
	ids := []string{"a"}
	field := map[string]map[string]interface{}{"a": {common.PAGERANK_FLD: 2}}
	sim := []float64{0, 7}
	got := applyRankFeatureScoresForIDs(ids, field, sim, nil)
	if !floatsClose(got, []float64{2, 7}, 1e-9) {
		t.Fatalf("got %v, want [2 7] (trailing score preserved)", got)
	}
}

