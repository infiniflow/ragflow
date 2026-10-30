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
	"context"
	"math"
	"testing"

	"ragflow/internal/common"
)

func TestBuildRerankTokens_OrderAndMultipliers(t *testing.T) {
	chunk := map[string]interface{}{
		"content_ltks":  "a b a",
		"title_tks":     "t1 t2",
		"important_kwd": []string{"i1", "i2"},
		"question_tks":  "q1 q2",
	}

	t.Run("standard multipliers", func(t *testing.T) {
		got := buildRerankTokens(chunk, "content_ltks", titleRepeat, importantRepeat, questionRepeat)
		want := []string{
			"a", "b", // content is deduplicated
			"t1", "t2", "t1", "t2",
			"i1", "i2", "i1", "i2", "i1", "i2", "i1", "i2", "i1", "i2",
			"q1", "q2", "q1", "q2", "q1", "q2", "q1", "q2", "q1", "q2", "q1", "q2",
		}
		if len(got) != len(want) {
			t.Fatalf("length = %d, want %d: %v", len(got), len(want), got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("token[%d] = %q, want %q (full: %v)", i, got[i], want[i], got)
			}
		}
	})

	t.Run("no repetition", func(t *testing.T) {
		got := buildRerankTokens(chunk, "content_ltks", 1, 1, 1)
		want := []string{"a", "b", "t1", "t2", "i1", "i2", "q1", "q2"}
		if len(got) != len(want) {
			t.Fatalf("length = %d, want %d: %v", len(got), len(want), got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("token[%d] = %q, want %q", i, got[i], want[i])
			}
		}
	})
}

func TestBuildRerankTokens_MultiplierCounts(t *testing.T) {
	chunk := map[string]interface{}{
		"content_ltks":  "c",
		"title_tks":     "t",
		"important_kwd": []string{"i"},
		"question_tks":  "q",
	}
	got := buildRerankTokens(chunk, "content_ltks", titleRepeat, importantRepeat, questionRepeat)
	counts := make(map[string]int)
	for _, tk := range got {
		counts[tk]++
	}
	want := map[string]int{"c": 1, "t": titleRepeat, "i": importantRepeat, "q": questionRepeat}
	for tk, n := range want {
		if counts[tk] != n {
			t.Fatalf("token %q appears %d times, want %d", tk, counts[tk], n)
		}
	}
	if titleRepeat != 2 || importantRepeat != 5 || questionRepeat != 6 {
		t.Fatalf("multipliers changed: title=%d important=%d question=%d", titleRepeat, importantRepeat, questionRepeat)
	}
}

func TestBuildRerankTokens_EmptyFieldsNonNil(t *testing.T) {
	got := buildRerankTokens(map[string]interface{}{}, "content_ltks", titleRepeat, importantRepeat, questionRepeat)
	if got == nil {
		t.Fatal("expected non-nil empty slice")
	}
	if len(got) != 0 {
		t.Fatalf("got %v, want empty", got)
	}
}

func TestBuildRerankTokens_DedupSemantics(t *testing.T) {
	chunk := map[string]interface{}{
		"content_ltks":  "x x y",
		"title_tks":     "x x",
		"important_kwd": []string{"x", "x"},
		"question_tks":  "x x",
	}
	got := buildRerankTokens(chunk, "content_ltks", 1, 1, 1)
	want := []string{"x", "y", "x", "x", "x", "x", "x", "x"}
	if len(got) != len(want) {
		t.Fatalf("length = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("token[%d] = %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
}

func TestRerankStandard_EmptyChunks(t *testing.T) {
	sim, tsim, vsim := RerankStandard(nil, nil, nil, "alpha", 0.3, 0.7, "content_ltks", nil, nil, "")
	if len(sim) != 0 || len(tsim) != 0 || len(vsim) != 0 {
		t.Fatalf("got sim=%v tsim=%v vsim=%v, want all empty", sim, tsim, vsim)
	}
}

func TestRerankStandard_VectorAndPagerankWiring(t *testing.T) {
	chunks := []map[string]interface{}{
		{
			"content_ltks":      "alpha",
			"q_2_vec":           []float64{1, 0},
			common.PAGERANK_FLD: 0.5,
		},
		{
			"content_ltks": "beta",
			"q_2_vec":      []float64{0, 1},
		},
	}
	sim, tsim, vsim := RerankStandard(
		chunks, nil, []float64{1, 0}, "alpha",
		0.3, 0.7, "content_ltks", nil, nil, "",
	)
	wantVsim := []float64{1, 0}
	if !floatsClose(vsim, wantVsim, 1e-9) {
		t.Fatalf("vsim = %v, want %v", vsim, wantVsim)
	}
	// qb == nil -> no token score, so the blend is purely vector + pagerank.
	wantSim := []float64{0.7 + 0.5, 0}
	if !floatsClose(sim, wantSim, 1e-9) {
		t.Fatalf("sim = %v, want %v", sim, wantSim)
	}
	if !floatsClose(tsim, []float64{0, 0}, 1e-9) {
		t.Fatalf("tsim = %v, want zeros", tsim)
	}
}

func TestRerankStandard_TokenWiring(t *testing.T) {
	qb := NewQueryBuilder()
	keywords := []string{"alpha", "gamma"}
	chunks := []map[string]interface{}{
		{
			"content_ltks":      "alpha beta",
			"title_tks":         "gamma",
			"important_kwd":     []string{"delta"},
			"question_tks":      "epsilon",
			"q_2_vec":           []float64{1, 0},
			common.PAGERANK_FLD: 0.25,
		},
	}
	sim, tsim, vsim := RerankStandard(
		chunks, keywords, []float64{1, 0}, "alpha",
		0.3, 0.7, "content_ltks", qb, nil, "",
	)

	wantTsim := TokenSimilarity(keywords, [][]string{
		buildRerankTokens(chunks[0], "content_ltks", titleRepeat, importantRepeat, questionRepeat),
	}, qb, "")
	if !floatsClose(tsim, wantTsim, 1e-9) {
		t.Fatalf("tsim = %v, want %v", tsim, wantTsim)
	}
	wantSim := []float64{0.3*wantTsim[0] + 0.7*vsim[0] + 0.25}
	if !floatsClose(sim, wantSim, 1e-9) {
		t.Fatalf("sim = %v, want %v", sim, wantSim)
	}
}

func TestRerankStandard_TSimNotPollutedByRankFeature(t *testing.T) {
	qb := NewQueryBuilder()
	chunks := []map[string]interface{}{
		{
			"content_ltks":      "alpha",
			common.PAGERANK_FLD: 2.5,
		},
	}
	keywords := []string{"alpha"}
	sim, tsim, vsim := RerankStandard(
		chunks, keywords, nil, "alpha",
		0.3, 0.7, "content_ltks", qb, nil, "",
	)
	if len(vsim) != 1 || vsim[0] != 0 {
		t.Fatalf("vsim = %v, want [0] for a nil query vector", vsim)
	}
	wantTsim := TokenSimilarity(keywords, [][]string{
		buildRerankTokens(chunks[0], "content_ltks", titleRepeat, importantRepeat, questionRepeat),
	}, qb, "")
	if !floatsClose(tsim, wantTsim, 1e-9) {
		t.Fatalf("tsim = %v, want %v (rank feature must not leak into term_similarity)", tsim, wantTsim)
	}
	// All-zero vectors fall back to tsim, then pagerank is added to sim only.
	if math.Abs(sim[0]-(wantTsim[0]+2.5)) > 1e-9 {
		t.Fatalf("sim = %v, want %v", sim[0], wantTsim[0]+2.5)
	}
}

func TestRerankWithKNN_EmptyIDs(t *testing.T) {
	sim, tsim, vsim := RerankWithKNN(
		context.Background(), nil, nil, nil, nil, "alpha",
		0.3, 0.7, "content_ltks", nil, nil, "",
	)
	if len(sim) != 0 || len(tsim) != 0 || len(vsim) != 0 {
		t.Fatalf("got sim=%v tsim=%v vsim=%v, want all empty", sim, tsim, vsim)
	}
}

func TestRerankWithKNN_KNNMapping(t *testing.T) {
	qb := NewQueryBuilder()
	ids := []string{"a", "b", "c"}
	field := map[string]map[string]interface{}{
		"a": {"content_ltks": "alpha"},
		"b": {"content_ltks": "beta"},
		"c": {"content_ltks": "gamma"},
	}
	knnScores := map[string]float64{"a": 0.9, "c": 0.4}

	_, _, vsim := RerankWithKNN(
		context.Background(), nil, ids, field, knnScores, "alpha",
		0.3, 0.7, "content_ltks", qb, nil, "",
	)
	want := []float64{0.9, 0, 0.4}
	if !floatsClose(vsim, want, 1e-9) {
		t.Fatalf("vsim = %v, want %v (missing id must be 0)", vsim, want)
	}
}

func TestRerankWithKNN_FallbackAndMissing(t *testing.T) {
	qb := NewQueryBuilder()
	// "a" is in field; "b" falls back to chunks[i]; "c" has neither.
	ids := []string{"a", "b", "c"}
	field := map[string]map[string]interface{}{
		"a": {"content_ltks": "alpha"},
	}
	chunks := []map[string]interface{}{
		{"content_ltks": "unused"},
		{"content_ltks": "beta"},
	}
	sim, tsim, vsim := RerankWithKNN(
		context.Background(), chunks, ids, field, nil, "alpha",
		0.3, 0.7, "content_ltks", qb, nil, "",
	)
	if len(sim) != 3 || len(tsim) != 3 || len(vsim) != 3 {
		t.Fatalf("lengths = %d/%d/%d, want 3/3/3", len(sim), len(tsim), len(vsim))
	}
	for i, v := range vsim {
		if v != 0 {
			t.Fatalf("vsim[%d] = %v, want 0 with no knn scores", i, v)
		}
	}
}

func TestRerankWithKNN_ImportantKwdNormalization(t *testing.T) {
	qb := NewQueryBuilder()

	asString := map[string]map[string]interface{}{
		"a": {"content_ltks": "alpha", "important_kwd": "delta"},
	}
	asSlice := map[string]map[string]interface{}{
		"a": {"content_ltks": "alpha", "important_kwd": []string{"delta"}},
	}

	simString, tsimString, _ := RerankWithKNN(
		context.Background(), nil, []string{"a"}, asString, nil, "alpha",
		0.3, 0.7, "content_ltks", qb, nil, "",
	)
	simSlice, tsimSlice, _ := RerankWithKNN(
		context.Background(), nil, []string{"a"}, asSlice, nil, "alpha",
		0.3, 0.7, "content_ltks", qb, nil, "",
	)
	if !floatsClose(simString, simSlice, 1e-9) || !floatsClose(tsimString, tsimSlice, 1e-9) {
		t.Fatalf("string vs []string differ: sim %v/%v tsim %v/%v",
			simString, simSlice, tsimString, tsimSlice)
	}
	if _, ok := asString["a"]["important_kwd"].([]string); !ok {
		t.Fatalf("important_kwd was not normalized in place: %T", asString["a"]["important_kwd"])
	}
}

func TestRerankWithKNN_TokenWiring(t *testing.T) {
	qb := NewQueryBuilder()
	ids := []string{"a"}
	field := map[string]map[string]interface{}{
		"a": {
			"content_ltks":      "alpha beta",
			"title_tks":         "gamma",
			"important_kwd":     []string{"delta"},
			"question_tks":      "epsilon",
			common.PAGERANK_FLD: 0.5,
		},
	}
	knnScores := map[string]float64{"a": 0.8}
	rankFeature := map[string]float64{"tag_a": 1}

	sim, tsim, vsim := RerankWithKNN(
		context.Background(), nil, ids, field, knnScores, "alpha",
		0.3, 0.7, "content_ltks", qb, rankFeature, "",
	)
	if !floatsClose(vsim, []float64{0.8}, 1e-9) {
		t.Fatalf("vsim = %v, want [0.8]", vsim)
	}

	_, keywords := qb.Question("alpha", "qa", 0.6, "")
	wantTsim := TokenSimilarity(keywords, [][]string{
		buildRerankTokens(field["a"], "content_ltks", titleRepeat, importantRepeat, questionRepeat),
	}, qb, "")
	if !floatsClose(tsim, wantTsim, 1e-9) {
		t.Fatalf("tsim = %v, want %v", tsim, wantTsim)
	}
	base := 0.3*wantTsim[0] + 0.7*0.8
	want := base + 0.5 // pagerank only; no tag_feas means no tag score
	if math.Abs(sim[0]-want) > 1e-9 {
		t.Fatalf("sim = %v, want %v", sim[0], want)
	}
}
