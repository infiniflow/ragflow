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
	"ragflow/internal/entity/models"
)

// scoringDriver returns fixed relevance scores keyed by response index and
// records the documents it was given. The embedded interface satisfies the rest
// of models.ModelDriver; it is never called.
type scoringDriver struct {
	models.ModelDriver
	results   []models.RerankResult
	documents []string
}

func (d *scoringDriver) Rerank(_ context.Context, _ *string, request models.RerankRequest, _ *models.APIConfig, _ *models.RerankConfig, _ *common.ModelUsage) (*models.RerankResponse, error) {
	d.documents = request.Documents
	return &models.RerankResponse{Data: d.results}, nil
}

func TestRerankByModel_Blend(t *testing.T) {
	qb := NewQueryBuilder()
	driver := &scoringDriver{results: []models.RerankResult{
		{Index: 0, RelevanceScore: 0.9},
		{Index: 1, RelevanceScore: 0.2},
	}}
	ids := []string{"a", "b"}
	field := map[string]map[string]interface{}{
		"a": {
			"content_ltks":      "alpha",
			common.PAGERANK_FLD: 0.5,
		},
		"b": {"content_ltks": "beta"},
	}
	model := models.NewRerankModel(driver, nil, nil, 0)

	sim, tsim, vsim := mustRerankByModel(t, model, chunksFor(ids), ids, field, "alpha", 0.3, 0.7, qb, nil)

	// In-range provider scores pass through NormalizeRerankScores unchanged.
	if !floatsClose(vsim, []float64{0.9, 0.2}, 1e-9) {
		t.Fatalf("vsim = %v, want [0.9 0.2]", vsim)
	}
	_, keywords := qb.Question("alpha", "qa", 0.6, "")
	wantTsim := TokenSimilarity(keywords, [][]string{
		buildRerankTokens(field["a"], "content_ltks", 1, 1, 1),
		buildRerankTokens(field["b"], "content_ltks", 1, 1, 1),
	}, qb, "")
	if !floatsClose(tsim, wantTsim, 1e-9) {
		t.Fatalf("tsim = %v, want %v", tsim, wantTsim)
	}
	for i := range sim {
		want := 0.3*tsim[i] + 0.7*vsim[i]
		if i == 0 {
			want += 0.5
		}
		if math.Abs(sim[i]-want) > 1e-9 {
			t.Fatalf("sim[%d] = %v, want %v", i, sim[i], want)
		}
	}
}

func TestRerankByModel_NormalizationApplied(t *testing.T) {
	qb := NewQueryBuilder()
	// NVIDIA-style logits: out of range and mixed sign.
	driver := &scoringDriver{results: []models.RerankResult{
		{Index: 0, RelevanceScore: 10},
		{Index: 1, RelevanceScore: -3},
	}}
	ids := []string{"a", "b"}
	field := map[string]map[string]interface{}{
		"a": {"content_ltks": "alpha"},
		"b": {"content_ltks": "beta"},
	}
	model := models.NewRerankModel(driver, nil, nil, 0)

	_, _, vsim := mustRerankByModel(t, model, chunksFor(ids), ids, field, "alpha", 0.3, 0.7, qb, nil)

	want := NormalizeRerankScores([]float64{10, -3})
	if !floatsClose(vsim, want, 1e-9) {
		t.Fatalf("vsim = %v, want normalized %v", vsim, want)
	}
	for _, v := range vsim {
		if v < 0 || v > 1 {
			t.Fatalf("normalized score escaped [0,1]: %v", vsim)
		}
	}
}

func TestRerankByModel_IndexOutOfRangeIgnored(t *testing.T) {
	qb := NewQueryBuilder()
	driver := &scoringDriver{results: []models.RerankResult{
		{Index: 5, RelevanceScore: 0.9},  // past the end
		{Index: -1, RelevanceScore: 0.8}, // negative
		{Index: 0, RelevanceScore: 0.3},
	}}
	ids := []string{"a"}
	field := map[string]map[string]interface{}{
		"a": {"content_ltks": "alpha"},
	}
	model := models.NewRerankModel(driver, nil, nil, 0)

	sim, _, vsim := mustRerankByModel(t, model, chunksFor(ids), ids, field, "alpha", 0.3, 0.7, qb, nil)

	if len(sim) != 1 || len(vsim) != 1 {
		t.Fatalf("lengths = %d/%d, want 1/1", len(sim), len(vsim))
	}
	if math.Abs(vsim[0]-0.3) > 1e-9 {
		t.Fatalf("vsim[0] = %v, want 0.3 (out-of-range indexes ignored)", vsim[0])
	}
}

func TestRerankByModel_ChunksFallbackAndEmptyIDs(t *testing.T) {
	qb := NewQueryBuilder()

	t.Run("empty ids", func(t *testing.T) {
		driver := &scoringDriver{}
		model := models.NewRerankModel(driver, nil, nil, 0)
		sim, tsim, vsim := mustRerankByModel(t, model, chunksFor([]string{"a"}), nil, nil, "alpha", 0.3, 0.7, qb, nil)
		if len(sim) != 0 || len(tsim) != 0 || len(vsim) != 0 {
			t.Fatalf("got %v/%v/%v, want all empty", sim, tsim, vsim)
		}
	})

	t.Run("field miss falls back to chunks", func(t *testing.T) {
		driver := &scoringDriver{results: []models.RerankResult{{Index: 0, RelevanceScore: 0.6}}}
		model := models.NewRerankModel(driver, nil, nil, 0)
		chunks := []map[string]interface{}{{"content_ltks": "alpha"}}
		sim, _, vsim := mustRerankByModel(t, model, chunks, []string{"missing"}, nil, "alpha", 0.3, 0.7, qb, nil)
		if len(sim) != 1 || math.Abs(vsim[0]-0.6) > 1e-9 {
			t.Fatalf("sim=%v vsim=%v, want one score 0.6", sim, vsim)
		}
	})
}

// chunksFor builds a non-empty chunk list: RerankByModel returns early when
// chunks is nil, even though it ultimately scores ids+field.
func chunksFor(ids []string) []map[string]interface{} {
	if len(ids) == 0 {
		return []map[string]interface{}{{"content_ltks": ""}}
	}
	chunks := make([]map[string]interface{}, len(ids))
	for i := range chunks {
		chunks[i] = map[string]interface{}{"content_ltks": ""}
	}
	return chunks
}

func mustRerankByModel(
	t *testing.T,
	model *models.RerankModel,
	chunks []map[string]interface{},
	ids []string,
	field map[string]map[string]interface{},
	query string,
	tkWeight, vtWeight float64,
	qb *QueryBuilder,
	rankFeature map[string]float64,
) ([]float64, []float64, []float64) {
	t.Helper()
	sim, tsim, vsim, err := RerankByModel(
		context.Background(), model, chunks, ids, field, query,
		tkWeight, vtWeight, "content_ltks", qb, rankFeature, "",
	)
	if err != nil {
		t.Fatalf("RerankByModel: %v", err)
	}
	return sim, tsim, vsim
}
