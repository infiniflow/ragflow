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
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity/models"
)

type recordingRerankDriver struct {
	models.ModelDriver
	gotDocs []string
}

func (d *recordingRerankDriver) Rerank(ctx context.Context, modelName *string, request models.RerankRequest, apiConfig *models.APIConfig, rerankConfig *models.RerankConfig, modelUsage *common.ModelUsage) (*models.RerankResponse, error) {
	d.gotDocs = request.Documents
	resp := &models.RerankResponse{}
	for i := range request.Documents {
		resp.Data = append(resp.Data, models.RerankResult{Index: i, RelevanceScore: 0.5})
	}
	return resp, nil
}

// TestRerankByModel_QuestionTksReached pins the parity fix: question_tks
// must flow into both the token-similarity input and the document that is
// sent to the reranker model, matching the Python rerank path.
func TestRerankByModel_QuestionTksReached(t *testing.T) {
	chunks := []map[string]interface{}{
		{
			"content_ltks":  "alpha beta",
			"title_tks":     "gamma",
			"important_kwd": []interface{}{"delta"},
			"question_tks":  "epsilon zeta",
			"page_rank_int": 0,
		},
	}
	ids := []string{"chunk-0"}
	field := map[string]map[string]interface{}{
		"chunk-0": chunks[0],
	}

	driver := &recordingRerankDriver{}
	modelName := "test-reranker"
	model := &models.RerankModel{
		ModelDriver: driver,
		ModelName:   &modelName,
	}

	sim, _, modelSim := RerankByModel(context.Background(), model, chunks, ids, field, "query", 0.3, 0.7, "content_ltks", nil, nil)

	if len(sim) != 1 || len(modelSim) != 1 {
		t.Fatalf("unexpected result lengths: sim=%d modelSim=%d", len(sim), len(modelSim))
	}

	if len(driver.gotDocs) != 1 {
		t.Fatalf("expected exactly one reranker document, got %d", len(driver.gotDocs))
	}
	if !strings.Contains(driver.gotDocs[0], "epsilon") || !strings.Contains(driver.gotDocs[0], "zeta") {
		t.Errorf("reranker document is missing question_tks tokens: %q", driver.gotDocs[0])
	}
}
