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

package task

import (
	"context"
	"errors"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity/models"
	"ragflow/internal/tokenizer"
)

// siliconflowOverLimitErr is the exact rejection observed on document 78785.md:
// a generic "parameter is invalid" message that actually means "this input is
// longer than the model accepts".
const siliconflowOverLimitErr = `SILICONFLOW API error: 400 Bad Request, body: {"code":20015,"message":"The parameter is invalid. Please check again.","data":null}`

// tokenBoundDriver accepts a request only while every input stays under
// maxTokens tokens (counted with cl100k, which is what the ingest path counted
// with before this change). It mimics a provider whose window is maxTokens.
type tokenBoundDriver struct {
	stubDriver
	maxTokens   int
	batchOnly   bool // reject only multi-input requests that are too long
	rejects     int
	sawSingle   bool
	singleCalls int
}

func (d *tokenBoundDriver) Embed(ctx context.Context, modelName *string, request models.EmbedRequest, apiConfig *models.APIConfig, embeddingConfig *models.EmbeddingConfig, usage *common.ModelUsage) ([]models.EmbeddingData, error) {
	counter := tokenizer.CountCL100K()
	tooLong := false
	for _, text := range request.Texts {
		if counter.Count(text) > d.maxTokens {
			tooLong = true
			break
		}
	}
	if tooLong && !d.batchOnly {
		d.rejects++
		return nil, errors.New(siliconflowOverLimitErr)
	}
	if tooLong && d.batchOnly && len(request.Texts) > 1 {
		d.rejects++
		return nil, errors.New(siliconflowOverLimitErr)
	}
	if len(request.Texts) == 1 {
		d.sawSingle = true
	}
	d.singleCalls++
	// Report a request-level usage, exactly like a real provider.
	if usage != nil {
		usage.InputTokens = 7 * len(request.Texts)
	}
	return d.stubDriver.Embed(ctx, modelName, request, apiConfig, embeddingConfig, usage)
}

func newLimitEmbedder(driver models.ModelDriver, declaredMaxTokens int) *embedder {
	return &embedder{model: models.NewEmbeddingModel(driver, strPtr("stub"), &models.APIConfig{}, declaredMaxTokens)}
}

func TestIsOverLimitErrClassification(t *testing.T) {
	cases := []struct {
		err  string
		want bool
	}{
		{siliconflowOverLimitErr, true},
		{`OpenAI embeddings API error: 400 Bad Request, body: {"error":{"message":"This model's maximum context length is 8192 tokens"}}`, true},
		{`API error: 413 Request Entity Too Large, body: input is too long`, true},
		{`API error: 422 Unprocessable Entity, body: too many tokens`, true},
		// Rate limits and provider failures must not be mistaken for size.
		{tpmLimitErr, false},
		{`SILICONFLOW API error: 500 Internal Server Error, body: too long`, false},
		{`OpenAI embeddings API error: 401 Unauthorized, body: invalid api key`, false},
		{`failed to send request: dial tcp: connection refused`, false},
	}
	for _, c := range cases {
		if got := isOverLimitErr(errors.New(c.err)); got != c.want {
			t.Errorf("isOverLimitErr(%q) = %t, want %t", c.err, got, c.want)
		}
	}
	if isOverLimitErr(nil) {
		t.Error("isOverLimitErr(nil) = true, want false")
	}
}

func TestOverLimitLadderShrinksAndFloors(t *testing.T) {
	limits := overLimitLadder(8192)
	want := []int{8192, 6144, 4096, 2048, 1024}
	if len(limits) != len(want) {
		t.Fatalf("overLimitLadder(8192) = %v, want %v", limits, want)
	}
	for i := range want {
		if limits[i] != want[i] {
			t.Fatalf("overLimitLadder(8192) = %v, want %v", limits, want)
		}
	}
	// A small window must not shrink past the floor, and must stay monotone.
	small := overLimitLadder(100)
	for i, v := range small {
		if v < overLimitFloorTokens {
			t.Fatalf("ladder step %d = %d, below the floor %d", i, v, overLimitFloorTokens)
		}
		if i > 0 && v >= small[i-1] {
			t.Fatalf("ladder is not strictly shrinking: %v", small)
		}
	}
	if got := overLimitLadder(0); len(got) != 1 || got[0] != 0 {
		t.Fatalf("overLimitLadder(0) = %v, want [0]", got)
	}

	// The per-input isolation path appends the documented floor: an input that no
	// proportional step fits may still fit at 64 tokens, and without that step the
	// document is declared unembeddable.
	floored := overLimitLadderToFloor(8192)
	if len(floored) != len(want)+1 || floored[len(floored)-1] != overLimitFloorTokens {
		t.Fatalf("overLimitLadderToFloor(8192) = %v, want %v then the floor %d", floored, want, overLimitFloorTokens)
	}
	for i := range want {
		if floored[i] != want[i] {
			t.Fatalf("overLimitLadderToFloor(8192) = %v, want %v then the floor", floored, want)
		}
	}
	if got := overLimitLadderToFloor(overLimitFloorTokens); len(got) != 1 || got[0] != overLimitFloorTokens {
		t.Fatalf("overLimitLadderToFloor(%d) = %v, want no duplicate floor step", overLimitFloorTokens, got)
	}
}

func TestDistributeTokenCountSumsToProviderUsage(t *testing.T) {
	limiter := tokenizer.NewExactLimiter(tokenizer.CountCL100K())
	texts := []string{strings.Repeat("word ", 10), strings.Repeat("word ", 40), strings.Repeat("word ", 20)}
	embeds := make([]models.EmbeddingData, len(texts))

	got := distributeTokenCount(embeds, 100, texts, limiter)
	sum := 0
	largest, largestIdx := -1, -1
	for i, v := range got {
		sum += v.TokenCount
		if v.TokenCount > largest {
			largest, largestIdx = v.TokenCount, i
		}
	}
	if sum != 100 {
		t.Fatalf("distributed token counts sum to %d, want the provider total 100", sum)
	}
	if largestIdx != 1 {
		t.Fatalf("the remainder went to input %d, want the longest input (1)", largestIdx)
	}
}

func TestDistributeTokenCountFallsBackToOwnCountWithoutUsage(t *testing.T) {
	limiter := tokenizer.NewExactLimiter(tokenizer.CountCL100K())
	texts := []string{"alpha beta", "gamma delta epsilon zeta"}
	embeds := make([]models.EmbeddingData, len(texts))
	got := distributeTokenCount(embeds, 0, texts, limiter)
	for i := range got {
		want := limiter.Counter().Count(texts[i])
		if got[i].TokenCount != want {
			t.Errorf("input %d token count = %d, want our own count %d", i, got[i].TokenCount, want)
		}
	}
}

// TestEmbedderEncodeShrinksUntilTheProviderAccepts is the 78785.md scenario at
// unit scale: the provider's window is smaller than what our counter allowed, so
// the first attempt is rejected. The document must still be embedded — by
// shrinking — and the rejection must teach the calibration.
func TestEmbedderEncodeShrinksUntilTheProviderAccepts(t *testing.T) {
	freezeBackoff(t)
	driver := &tokenBoundDriver{maxTokens: 2000}
	emb := newLimitEmbedder(driver, 8192)

	long := strings.Repeat("the quick brown fox jumps over the lazy dog ", 500) // ~5500 cl100k tokens
	vecs, err := emb.Encode(context.Background(), []string{long, long, long})
	if err != nil {
		t.Fatalf("Encode returned %v; an over-limit rejection must not fail the document", err)
	}
	if len(vecs) != 3 {
		t.Fatalf("got %d vectors, want 3", len(vecs))
	}
	if driver.rejects == 0 {
		t.Fatal("the driver never rejected anything, so the shrink path was not exercised")
	}
	for i, text := range driver.capturedTexts {
		if got := tokenizer.CountCL100K().Count(text); got > driver.maxTokens {
			t.Errorf("attempt sent input %d with %d tokens, still above the provider window %d", i, got, driver.maxTokens)
		}
	}
	// The rejection is an observation: the next attempt must start tighter.
	if ratio := tokenizer.DefaultCalibration().RatioUpper(string(emb.quotaKey())); ratio <= 1 {
		t.Fatalf("calibration ratio after an over-limit rejection = %v, want > 1", ratio)
	}
	// Every input still gets a real token count (previously always 0).
	for i, v := range vecs {
		if v.TokenCount <= 0 {
			t.Errorf("vector %d has token count %d, want a positive count", i, v.TokenCount)
		}
	}
}

// TestEmbedderEncodeIsolatesPathologicalInput covers a batch where the size
// problem only shows up when several inputs travel together: shrinking the
// shared budget cannot help, so the embedder has to isolate the inputs instead of
// failing the whole document.
func TestEmbedderEncodeIsolatesPathologicalInput(t *testing.T) {
	freezeBackoff(t)
	driver := &tokenBoundDriver{maxTokens: 200, batchOnly: true}
	emb := newLimitEmbedder(driver, 8192)

	long := strings.Repeat("lorem ipsum dolor sit amet ", 200)
	vecs, err := emb.Encode(context.Background(), []string{"short one", long, "short two"})
	if err != nil {
		t.Fatalf("Encode returned %v; isolation should have saved the document", err)
	}
	if len(vecs) != 3 {
		t.Fatalf("got %d vectors, want 3", len(vecs))
	}
	if !driver.sawSingle {
		t.Fatal("the embedder never fell back to per-input calls")
	}
}

func TestEmbedderEncodeEmptyInput(t *testing.T) {
	emb := newLimitEmbedder(&stubDriver{}, 8192)
	vecs, err := emb.Encode(context.Background(), nil)
	if err != nil {
		t.Fatalf("Encode(nil) returned %v", err)
	}
	if len(vecs) != 0 {
		t.Fatalf("Encode(nil) returned %d vectors, want 0", len(vecs))
	}
}

// TestEmbedderTrimPrefersModelTokenizer pins the contract the component relies
// on: the embedder's Trim honours the declared window with a proportional margin
// (not a flat 10 tokens), and never hands back more than the limit allows.
func TestEmbedderTrimPrefersModelTokenizer(t *testing.T) {
	emb := newLimitEmbedder(&stubDriver{}, 8192)
	if got := emb.ResolveMaxTokens(); got != 8192 {
		t.Fatalf("ResolveMaxTokens() = %d, want the declared 8192", got)
	}
	text := strings.Repeat("token ", 20000)
	trimmed, tokens := emb.Trim(text)
	limit := tokenizer.EmbeddingTokenLimit(8192)
	if tokens > limit {
		t.Fatalf("Trim returned %d tokens, limit is %d", tokens, limit)
	}
	if got := tokenizer.CountCL100K().Count(trimmed); got > limit {
		t.Fatalf("trimmed text counts %d tokens, limit is %d", got, limit)
	}
	// Defaults are also honoured when a model declares nothing.
	unset := newLimitEmbedder(&stubDriver{}, 0)
	if got := unset.ResolveMaxTokens(); got != tokenizer.EmbeddingTokenLimitDefault {
		t.Fatalf("ResolveMaxTokens() with no declaration = %d, want %d", got, tokenizer.EmbeddingTokenLimitDefault)
	}
}

// TestEmbedderEncodeRefusesToSubstituteTheCalibratedEstimate pins that a model declaring a
// tokenizer it cannot load FAILS instead of counting with the calibrated cl100k estimate:
// that estimate belongs to a different tokenizer (cl100k under-counts XLM-R on some
// content), and an under-count is the direction that makes a provider answer 400. The id
// below is deliberately one nothing registers - the same path as an asset that never
// loaded - and it reaches the resolution through the same env override an operator uses to
// pin a family.
func TestEmbedderEncodeRefusesToSubstituteTheCalibratedEstimate(t *testing.T) {
	t.Setenv("TOKENIZER_EMBEDDING_TOKENIZER", "no-such-family")
	emb := newLimitEmbedder(&stubDriver{}, 8192)
	_, err := emb.Encode(context.Background(), []string{"hello"})
	if err == nil {
		t.Fatal("Encode succeeded with an unavailable declared tokenizer; it must refuse instead")
	}
	for _, want := range []string{`"no-such-family"`, "but its asset is unavailable", "refusing to count with the calibrated estimate"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err.Error(), want)
		}
	}
}
