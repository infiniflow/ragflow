//
//  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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

package nlp

import (
	"context"
	"math"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/engine"
)

// localRerankDocEngine records whether scoreSearchResult takes the engine's
// KNN path. The embedded interface satisfies the rest of engine.DocEngine and
// is never called by the local-rerank branches.
type localRerankDocEngine struct {
	engine.DocEngine
	engineType string
	knnCalls   int
	scoreCalls int
}

func (e *localRerankDocEngine) GetType() string { return e.engineType }

func (e *localRerankDocEngine) KNNScores(context.Context, []map[string]interface{}, []float64, int) (map[string]interface{}, error) {
	e.knnCalls++
	return map[string]interface{}{}, nil
}

func (e *localRerankDocEngine) GetScores(map[string]interface{}) map[string]float64 {
	e.scoreCalls++
	return nil
}

func TestRerankStandardIncludesQuestionTks(t *testing.T) {
	qb := NewQueryBuilder()
	withQuestions := map[string]interface{}{
		"content_ltks": "alpha",
		"question_tks": "epsilon",
	}
	withoutQuestions := map[string]interface{}{
		"content_ltks": "alpha",
	}

	_, withTsim, _ := RerankStandard(
		[]map[string]interface{}{withQuestions}, nil, nil, "epsilon",
		0.3, 0.7, "content_ltks", qb, nil, "",
	)
	_, withoutTsim, _ := RerankStandard(
		[]map[string]interface{}{withoutQuestions}, nil, nil, "epsilon",
		0.3, 0.7, "content_ltks", qb, nil, "",
	)

	if len(withTsim) != 1 || len(withoutTsim) != 1 {
		t.Fatalf("tsim lengths = %d/%d, want 1/1", len(withTsim), len(withoutTsim))
	}
	if withTsim[0] <= withoutTsim[0] {
		t.Fatalf("question_tks did not increase tsim: with=%v without=%v", withTsim[0], withoutTsim[0])
	}
}

func TestScoreSearchResultLocalRerankOceanBaseAndVastbase(t *testing.T) {
	oldQueryBuilder := globalQueryBuilder
	globalQueryBuilder = NewQueryBuilder()
	defer func() { globalQueryBuilder = oldQueryBuilder }()

	const (
		tkWeight = 0.3
		vtWeight = 0.7
	)
	weight := vtWeight
	rankFeature := map[string]float64{common.PAGERANK_FLD: 10}

	result := &RetrievalSearchResult{
		Chunks: []map[string]interface{}{
			{
				"id":                "a",
				"content_ltks":      "alpha",
				"_score":            0.99,
				common.PAGERANK_FLD: 0.5,
				"q_2_vec":           []float64{1, 0},
			},
			{
				"id":           "b",
				"content_ltks": "beta",
				"_score":       0.99,
				"q_2_vec":      []float64{0, 1},
			},
		},
		Total:       2,
		QueryVector: []float64{1, 0},
		IDs:         []string{"a", "b"},
		// Field carries deliberately different values: the OceanBase and
		// Vastbase branches must rerank from Chunks, not from Field.
		Field: map[string]map[string]interface{}{
			"a": {"_score": 0.99, common.PAGERANK_FLD: 100.0},
			"b": {"_score": 0.99, common.PAGERANK_FLD: 100.0},
		},
	}

	for _, tc := range []struct {
		name       string
		engineType string
	}{
		{name: "oceanbase", engineType: string(engine.EngineOceanBase)},
		{name: "vastbase", engineType: string(engine.EngineVastbase)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			docEngine := &localRerankDocEngine{engineType: tc.engineType}
			service := NewRetrievalService(docEngine, nil)

			sim, tsim, vsim, err := service.scoreSearchResult(t.Context(), &RetrievalRequest{
				Question:               "alpha",
				VectorSimilarityWeight: &weight,
				RankFeature:            &rankFeature,
			}, result)
			if err != nil {
				t.Fatalf("scoreSearchResult failed: %v", err)
			}
			if docEngine.knnCalls != 0 || docEngine.scoreCalls != 0 {
				t.Fatalf("KNN path called: KNNScores=%d GetScores=%d", docEngine.knnCalls, docEngine.scoreCalls)
			}
			if len(sim) != 2 || len(tsim) != 2 || len(vsim) != 2 {
				t.Fatalf("score lengths = %d/%d/%d, want 2/2/2", len(sim), len(tsim), len(vsim))
			}

			if math.Abs(vsim[0]-1) > 1e-9 || math.Abs(vsim[1]) > 1e-9 {
				t.Fatalf("vsim = %v, want [1 0] from Chunks vectors", vsim)
			}

			pageranks := []float64{0.5, 0}
			for i := range sim {
				want := tkWeight*tsim[i] + vtWeight*vsim[i] + pageranks[i]
				if math.Abs(sim[i]-want) > 1e-9 {
					t.Fatalf("sim[%d] = %v, want %v (tsim=%v vsim=%v)", i, sim[i], want, tsim, vsim)
				}
			}
		})
	}
}
