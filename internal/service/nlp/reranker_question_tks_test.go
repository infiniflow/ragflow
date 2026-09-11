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
		&models.RerankModel{ModelDriver: driver},
		[]map[string]interface{}{chunk},
		ids,
		field,
		"epsilon",
		0.3,
		0.7,
		"content_ltks",
		nil,
		nil,
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

// TestRerankByModelNaturalTextKeepsAccentedSpacing guards against
// RemoveRedundantSpaces being applied to natural text: it treats every
// non-ASCII letter as punctuation and would collapse "sécurité des" into
// "sécuritédes".
func TestRerankByModelNaturalTextKeepsAccentedSpacing(t *testing.T) {
	chunk := fullChunk()
	chunk["content_with_weight"] = "sécurité des données"

	doc := rerankOneChunk(t, chunk)

	if doc != "sécurité des données" {
		t.Errorf("natural text was altered: got %q", doc)
	}
	if got := RemoveRedundantSpaces("sécurité des données"); got == "sécurité des données" {
		t.Errorf("test premise broken: RemoveRedundantSpaces no longer mangles accented text (%q)", got)
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
