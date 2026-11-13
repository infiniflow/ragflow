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

// captureDriver records the documents RerankByModel hands to the cross-encoder,
// which is the only observable the token assembly produces. The embedded
// interface satisfies the rest of models.ModelDriver; it is never called.
type captureDriver struct {
	models.ModelDriver
	documents []string
}

func (d *captureDriver) Rerank(_ context.Context, _ *string, request models.RerankRequest, _ *models.APIConfig, _ *models.RerankConfig, _ *common.ModelUsage) (*models.RerankResponse, error) {
	d.documents = request.Documents
	return &models.RerankResponse{}, nil
}

// rerankOneChunk runs RerankByModel over a single chunk and returns the
// document text the reranker model received for it.
func rerankOneChunk(t *testing.T, chunk map[string]interface{}) string {
	t.Helper()

	driver := &captureDriver{}
	ids := []string{"c1"}
	field := map[string]map[string]interface{}{"c1": chunk}

	RerankByModel(
		context.Background(),
		models.NewRerankModel(driver, nil, nil, 0),
		[]map[string]interface{}{chunk},
		ids,
		field,
		"epsilon",
		0.3,
		0.7,
		"content_ltks",
		nil,
		nil,
		"",
	)

	if len(driver.documents) != 1 {
		t.Fatalf("expected 1 document to reach the reranker model, got %d", len(driver.documents))
	}
	return driver.documents[0]
}

func fullChunk() map[string]interface{} {
	return map[string]interface{}{
		"content_ltks":  "alpha beta",
		"title_tks":     "gamma",
		"important_kwd": []string{"delta"},
		"question_tks":  "epsilon zeta",
	}
}

func TestRerankStandardScoresQuestionOnlyMatch(t *testing.T) {
	chunks := []map[string]interface{}{
		{"content_ltks": "alpha", "question_tks": "quux"},
		{"content_ltks": "alpha"},
	}

	_, tsim, _ := RerankStandard(
		chunks, []string{"quux"}, nil, "quux", 1, 0,
		"content_ltks", NewQueryBuilder(), nil, "",
	)

	if len(tsim) != 2 {
		t.Fatalf("term similarities = %v, want two scores", tsim)
	}
	if !(tsim[0] > 0.5 && tsim[0] <= 1) {
		t.Errorf("question-only match term similarity = %v, want a positive match above 0.5", tsim[0])
	}
	if !(tsim[1] >= 0 && tsim[1] < 1e-6) {
		t.Errorf("term similarity without question_tks = %v, want near zero", tsim[1])
	}
}

func TestRerankWithKNNIncludesQuestionTks(t *testing.T) {
	chunks := []map[string]interface{}{
		{"content_ltks": "alpha", "question_tks": "quux"},
		{"content_ltks": "alpha"},
	}
	ids := []string{"with-question", "without-question"}
	field := map[string]map[string]interface{}{
		ids[0]: chunks[0],
		ids[1]: chunks[1],
	}

	_, tsim, _ := RerankWithKNN(
		t.Context(), chunks, ids, field,
		map[string]float64{ids[0]: 0.5, ids[1]: 0.5},
		"quux", 1, 0, "content_ltks", NewQueryBuilder(), nil, "",
	)

	if len(tsim) != 2 {
		t.Fatalf("term similarities = %v, want two scores", tsim)
	}
	if !(tsim[0] > 0.5 && tsim[0] <= 1) {
		t.Errorf("question-only match term similarity = %v, want a positive match above 0.5", tsim[0])
	}
	if !(tsim[1] >= 0 && tsim[1] < 1e-6) {
		t.Errorf("term similarity without question_tks = %v, want near zero", tsim[1])
	}
}

// TestRerankByModelIncludesQuestionTks pins the fix: RerankStandard and
// RerankWithKNN both fold question_tks into their token lists, so a chunk
// whose generated questions carry the match was scored on those tokens on
// every path except this one, where the cross-encoder never saw them.
func TestRerankByModelIncludesQuestionTks(t *testing.T) {
	doc := rerankOneChunk(t, fullChunk())

	for _, tk := range []string{"alpha", "beta", "gamma", "delta", "epsilon", "zeta"} {
		if !strings.Contains(doc, tk) {
			t.Errorf("document %q is missing token %q", doc, tk)
		}
	}
}

// TestRerankByModelDoesNotRepeatFields guards the deliberate difference from
// RerankStandard/RerankWithKNN, which weight fields by repeating them: these
// tokens are joined back into a document for a cross-encoder, so a repeated
// field would distort the model's own scoring.
func TestRerankByModelDoesNotRepeatFields(t *testing.T) {
	doc := rerankOneChunk(t, fullChunk())

	for _, tk := range []string{"gamma", "delta", "epsilon", "zeta"} {
		if got := strings.Count(doc, tk); got != 1 {
			t.Errorf("token %q appears %d times in %q, want 1", tk, got, doc)
		}
	}
}

// TestRerankByModelWithoutQuestionTks covers chunks that carry no generated
// questions, which is the common case: the field is absent rather than empty,
// so extractQuestionTokens must contribute nothing at all.
func TestRerankByModelWithoutQuestionTks(t *testing.T) {
	chunk := fullChunk()
	delete(chunk, "question_tks")

	doc := rerankOneChunk(t, chunk)

	if want := "alpha beta gamma delta"; doc != want {
		t.Errorf("got %q, want %q", doc, want)
	}
}

// TestRerankByModelPrefersNaturalText pins the reranker input contract: the
// cross-encoder is scored on content_with_weight (natural text, markup
// preserved, passed through untouched), never on the tokenized fields.
func TestRerankByModelPrefersNaturalText(t *testing.T) {
	chunk := fullChunk()
	chunk["content_with_weight"] = "<p>La sécurité des données : voir l'annexe A.</p>"

	doc := rerankOneChunk(t, chunk)

	if want := chunk["content_with_weight"].(string); doc != want {
		t.Errorf("got %q, want %q", doc, want)
	}
}

// TestRerankByModelNaturalTextKeepsAccentedSpacing guards against the
// redundant-space cleanup being applied to natural text: an ASCII-only word
// class treats every non-Latin letter as punctuation and collapses
// "sécurité des" into "sécuritédes". The cleanup itself is pinned in
// common.TestRemoveRedundantSpaces.
func TestRerankByModelNaturalTextKeepsAccentedSpacing(t *testing.T) {
	chunk := fullChunk()
	chunk["content_with_weight"] = "sécurité des données"

	doc := rerankOneChunk(t, chunk)

	if doc != "sécurité des données" {
		t.Errorf("natural text was altered: got %q", doc)
	}
}

// TestRerankByModelFallsBackToTokensWithoutNaturalText covers chunks with
// no content_with_weight (or an empty one): the tokenized fields are joined
// and cleaned exactly as before.
func TestRerankByModelFallsBackToTokensWithoutNaturalText(t *testing.T) {
	for name, chunk := range map[string]map[string]interface{}{
		"absent": fullChunk(),
		"empty":  func() map[string]interface{} { c := fullChunk(); c["content_with_weight"] = ""; return c }(),
	} {
		doc := rerankOneChunk(t, chunk)
		if want := "alpha beta gamma delta epsilon zeta"; doc != want {
			t.Errorf("%s: got %q, want %q", name, doc, want)
		}
	}
}

// TestRerankByModelUsesTokenizedFieldsForTermSimilarity pins the split between
// the two consumers in RerankByModel: the cross-encoder scores the natural text,
// while term similarity must keep reading the tokenized content_ltks. Both
// chunks carry the same natural text containing the query term, so only the
// tokenized field can separate them. Weights are pinned to term similarity
// alone (tkWeight=1, vtWeight=0, no rank features) so neither the model score
// nor pagerank can mask a wrong field choice.
func TestRerankByModelUsesTokenizedFieldsForTermSimilarity(t *testing.T) {
	const naturalText = "quux appears in the natural text"
	chunks := []map[string]interface{}{
		{"content_with_weight": naturalText, "content_ltks": "quux"},
		{"content_with_weight": naturalText, "content_ltks": "alpha"},
	}
	ids := []string{"with-quux", "without-quux"}
	field := map[string]map[string]interface{}{
		ids[0]: chunks[0],
		ids[1]: chunks[1],
	}
	driver := &captureDriver{}

	sim, tsim, _, err := RerankByModel(
		t.Context(),
		models.NewRerankModel(driver, nil, nil, 0),
		chunks,
		ids,
		field,
		"quux",
		1,
		0,
		"content_ltks",
		NewQueryBuilder(),
		nil,
		"",
	)
	if err != nil {
		t.Fatalf("RerankByModel failed: %v", err)
	}
	if len(tsim) != 2 {
		t.Fatalf("term similarities = %v, want two scores", tsim)
	}
	if len(sim) != 2 {
		t.Fatalf("similarities = %v, want two scores", sim)
	}
	if len(driver.documents) != 2 {
		t.Fatalf("model documents = %v, want two documents", driver.documents)
	}
	for i, doc := range driver.documents {
		if doc != naturalText {
			t.Errorf("model document[%d] = %q, want unchanged natural text %q", i, doc, naturalText)
		}
	}
	if !(tsim[0] > 0.5 && tsim[0] <= 1) {
		t.Errorf("term similarity for the matching content_ltks = %v, want a positive match above 0.5", tsim[0])
	}
	if !(tsim[1] >= 0 && tsim[1] < 1e-6) {
		t.Errorf("term similarity for the non-matching content_ltks = %v, want near zero", tsim[1])
	}
}

// TestRerankByModelCleansTokenizedFallback pins the fallback formatting: when no
// natural text exists, the joined tokens are passed through
// common.RemoveRedundantSpaces so punctuation does not keep the spaces its
// tokenization introduced.
func TestRerankByModelCleansTokenizedFallback(t *testing.T) {
	doc := rerankOneChunk(t, map[string]interface{}{
		"content_ltks": "( alpha ) beta !",
	})

	if want := "(alpha) beta!"; doc != want {
		t.Errorf("got %q, want %q", doc, want)
	}
}
