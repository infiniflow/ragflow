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

package models

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/tokenizer"
)

// overWindowError is what SiliconFlow answers for an input over the model window:
// a 400 whose body carries code 20015.
func overWindowError() error {
	return errors.New(`SILICONFLOW API error: 400 Bad Request, body: {"code":20015,"message":"The parameter is invalid. Please check again.","data":null}`)
}

// recordingEmbedDriver implements only the Embed half of ModelDriver: it records the
// texts it was handed and can reject them the way an over-window provider does.
type recordingEmbedDriver struct {
	ModelDriver
	attempts [][]string
	// rejectFor rejects the first N attempts.
	rejectFor int
	// rejectBatch rejects any request carrying more than one text, which is how a
	// pathological input behaves: the batch ladder cannot fit the batch, while each
	// input on its own can be embedded.
	rejectBatch bool
	// rejectLongerThan rejects any request holding a text longer than that many
	// bytes, so a small text passes while a large one fails even after the trim.
	rejectLongerThan int
	failWith         error
}

func (d *recordingEmbedDriver) Embed(_ context.Context, _ *string, request EmbedRequest, _ *APIConfig, _ *EmbeddingConfig, _ *common.ModelUsage) ([]EmbeddingData, error) {
	d.attempts = append(d.attempts, request.Texts)
	if d.rejectBatch && len(request.Texts) > 1 {
		return nil, overWindowError()
	}
	if d.rejectLongerThan > 0 {
		for _, text := range request.Texts {
			if len(text) > d.rejectLongerThan {
				return nil, overWindowError()
			}
		}
	}
	if len(d.attempts) <= d.rejectFor {
		return nil, overWindowError()
	}
	if d.failWith != nil {
		return nil, d.failWith
	}
	return make([]EmbeddingData, len(request.Texts)), nil
}

// newEmbedModel pins the inputs that decide the window and the counter, so a test
// never depends on the machine's catalog or environment: the tokenizer id and the
// window come from the env overrides, and the api key (the test's own name) gives the
// model its own quota/calibration key. That last part matters because the default
// Calibration is process-global and only ever ratchets up - one test's over-limit
// observation must not shrink another test's window.
func newEmbedModel(t *testing.T, driver ModelDriver, tokenizerID string, maxTokens int) *EmbeddingModel {
	t.Helper()
	t.Setenv("TOKENIZER_EMBEDDING_TOKENIZER", tokenizerID)
	t.Setenv("TOKENIZER_EMBEDDING_MAX_TOKENS", strconv.Itoa(maxTokens))
	apiKey := t.Name()
	return NewEmbeddingModel(driver, nil, &APIConfig{ApiKey: &apiKey}, maxTokens)
}

// embedBudget is the budget a single cut would use, computed the same way
// EmbedWithinLimit computes its first rung.
func embedBudget(t *testing.T, model *EmbeddingModel) (int, tokenizer.Counter) {
	t.Helper()
	limiter := tokenizer.LimiterFor(model.ResolveTokenizerID(), model.QuotaKey(), tokenizer.DefaultCalibration())
	return limiter.Limit(model.ResolveMaxTokens()), limiter.Counter()
}

// TestEmbedWithinLimitCutsToTheWindow pins what the dataset-nav, knowledge-compiler,
// chunk and memory callers rely on: what reaches the driver fits the window, order is
// preserved, a short text is untouched, and a first-attempt success is not retried.
//
// The empty tokenizer id is the calibrated path (a model whose tokenizer the catalog
// does not name); xlmr-spm is covered when its asset is present because it is the
// counter behind the reported failure (a nav summary against bge-m3, 400/20015).
func TestEmbedWithinLimitCutsToTheWindow(t *testing.T) {
	long := strings.Repeat("中文 hello ", 5000)
	for _, id := range []string{"", tokenizer.CounterXLMRSentence} {
		if id != "" && !tokenizer.CounterExact(id) {
			t.Logf("counter %s is unavailable in this environment; skipping it", id)
			continue
		}
		driver := &recordingEmbedDriver{}
		model := newEmbedModel(t, driver, id, 8192)
		embeds, err := model.EmbedWithinLimit(t.Context(), EmbedRequest{Texts: []string{long, "short"}}, nil, nil)
		if err != nil {
			t.Fatalf("tokenizer %q: EmbedWithinLimit: %v", id, err)
		}
		if len(embeds) != 2 {
			t.Fatalf("tokenizer %q: got %d embeddings, want 2", id, len(embeds))
		}
		if len(driver.attempts) != 1 {
			t.Fatalf("tokenizer %q: %d attempts for an input the provider accepts", id, len(driver.attempts))
		}
		got := driver.attempts[0]
		if got[1] != "short" {
			t.Errorf("tokenizer %q: a text inside the window was modified: %q", id, got[1])
		}
		if got[0] == long {
			t.Errorf("tokenizer %q: the over-window text was not cut", id)
		}
		budget, counter := embedBudget(t, model)
		if n := counter.Count(got[0]); n > budget {
			t.Errorf("tokenizer %q: the text handed to the driver counts %d tokens, budget is %d", id, n, budget)
		}
	}
}

// TestEmbedWithinLimitRetriesWithASmallerBudget is the property that turns "this
// batch fails forever" into "this batch succeeds": the retried text must be strictly
// shorter, and the caller must not see the rejection.
func TestEmbedWithinLimitRetriesWithASmallerBudget(t *testing.T) {
	driver := &recordingEmbedDriver{rejectFor: 1}
	model := newEmbedModel(t, driver, "", 8192)
	if _, err := model.EmbedWithinLimit(t.Context(), EmbedRequest{Texts: []string{strings.Repeat("中", 20000)}}, nil, nil); err != nil {
		t.Fatalf("EmbedWithinLimit: %v", err)
	}
	if len(driver.attempts) != 2 {
		t.Fatalf("attempts = %d, want 2", len(driver.attempts))
	}
	if len(driver.attempts[1][0]) >= len(driver.attempts[0][0]) {
		t.Fatalf("the retry did not shrink the input: %d then %d bytes", len(driver.attempts[0][0]), len(driver.attempts[1][0]))
	}
}

// TestEmbedWithinLimitDoesNotRetryUnrelatedErrors keeps the retry narrow: a 5xx is
// not something a shorter input fixes, so it comes back on the first attempt.
func TestEmbedWithinLimitDoesNotRetryUnrelatedErrors(t *testing.T) {
	boom := errors.New("SILICONFLOW API error: 503 Service Unavailable")
	driver := &recordingEmbedDriver{failWith: boom}
	model := newEmbedModel(t, driver, "", 8192)
	_, err := model.EmbedWithinLimit(t.Context(), EmbedRequest{Texts: []string{"hello"}}, nil, nil)
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want the provider's own error", err)
	}
	if len(driver.attempts) != 1 {
		t.Fatalf("attempts = %d, want 1", len(driver.attempts))
	}
}

// TestEmbedWithinLimitRefusesAnUnavailableDeclaredTokenizer is the half of the
// contract that keeps the retry from hiding a provisioning gap: a model that declares
// a tokenizer we cannot load fails before the first attempt.
func TestEmbedWithinLimitRefusesAnUnavailableDeclaredTokenizer(t *testing.T) {
	driver := &recordingEmbedDriver{}
	model := newEmbedModel(t, driver, "no-such-family", 8192)
	_, err := model.EmbedWithinLimit(t.Context(), EmbedRequest{Texts: []string{"hello"}}, nil, nil)
	if err == nil {
		t.Fatal("expected an error for a declared tokenizer whose asset is unavailable")
	}
	for _, want := range []string{`"no-such-family"`, "but its asset is unavailable", "refusing to count with the calibrated estimate"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err.Error(), want)
		}
	}
	if len(driver.attempts) != 0 {
		t.Errorf("the driver was called %d times despite the refusal", len(driver.attempts))
	}
}

// TestEmbedWithinLimitIsolatesAPathologicalInput is the property that keeps one bad
// input from failing the whole batch: when no batch budget fits, every input is
// embedded on its own, in order, and the healthy ones keep their content.
func TestEmbedWithinLimitIsolatesAPathologicalInput(t *testing.T) {
	driver := &recordingEmbedDriver{rejectBatch: true}
	model := newEmbedModel(t, driver, "", 8192)
	// Read the ladder length before the call: the call's own rejections ratchet the
	// calibration, which would shrink the budget this number is derived from.
	budget, _ := embedBudget(t, model)
	batchRungs := len(tokenizer.OverLimitLadder(budget))
	texts := []string{strings.Repeat("a", 40), "second"}
	embeds, err := model.EmbedWithinLimit(t.Context(), EmbedRequest{Texts: texts}, nil, nil)
	if err != nil {
		t.Fatalf("EmbedWithinLimit: %v", err)
	}
	if len(embeds) != 2 {
		t.Fatalf("got %d embeddings, want 2", len(embeds))
	}
	if len(driver.attempts) != batchRungs+2 {
		t.Fatalf("attempts = %d, want %d batch rungs plus 2 isolated calls", len(driver.attempts), batchRungs+2)
	}
	isolated := driver.attempts[batchRungs:]
	for i, attempt := range isolated {
		if len(attempt) != 1 {
			t.Fatalf("isolated call %d carried %d texts, want 1", i, len(attempt))
		}
		// Both inputs fit the first budget, so isolation must not have modified them.
		if attempt[0] != texts[i] {
			t.Errorf("isolated call %d got %q, want %q", i, attempt[0], texts[i])
		}
	}
}

// TestEmbedWithinLimitFailsWhenNoBudgetFits keeps the floor honest: the ladder ends
// above OverLimitFloorTokens and only isolation reaches it, so the error may only be
// raised after the floor was actually tried - and never with a partial result.
func TestEmbedWithinLimitFailsWhenNoBudgetFits(t *testing.T) {
	driver := &recordingEmbedDriver{rejectLongerThan: 8}
	model := newEmbedModel(t, driver, "", 8192)
	budget, _ := embedBudget(t, model)
	embeds, err := model.EmbedWithinLimit(t.Context(), EmbedRequest{Texts: []string{strings.Repeat("中", 500)}}, nil, nil)
	if err == nil {
		t.Fatal("expected an error when not even the floor fits")
	}
	if embeds != nil {
		t.Fatalf("partial results returned: %d embeddings", len(embeds))
	}
	if !strings.Contains(err.Error(), "even at 64 tokens") {
		t.Errorf("error %q does not say the floor was tried", err)
	}
	want := len(tokenizer.OverLimitLadder(budget)) + len(tokenizer.OverLimitLadderToFloor(budget))
	if len(driver.attempts) != want {
		t.Errorf("attempts = %d, want %d (batch ladder then isolation down to the floor)", len(driver.attempts), want)
	}
}

// TestEmbedWithinLimitRecordsTheDeclaredLimitInTheCalibration pins the third argument
// of ObserveOverLimit: it is the provider's declared window, not the local budget
// derived from it. Passing the (smaller) budget would understate the inferred ratio
// and leave the calibration effectively unchanged, so the next call would repeat the
// same rejection.
func TestEmbedWithinLimitRecordsTheDeclaredLimitInTheCalibration(t *testing.T) {
	driver := &recordingEmbedDriver{rejectFor: 1}
	const declared = 8192
	model := newEmbedModel(t, driver, "", declared)
	if _, err := model.EmbedWithinLimit(t.Context(), EmbedRequest{Texts: []string{strings.Repeat("a", 40000)}}, nil, nil); err != nil {
		t.Fatalf("EmbedWithinLimit: %v", err)
	}
	if len(driver.attempts) == 0 {
		t.Fatal("no attempt recorded")
	}
	_, counter := embedBudget(t, model)
	own := tokenizer.OwnTokenMax(driver.attempts[0], counter)
	want := float64(declared) / float64(own) * 1.01
	if got := tokenizer.DefaultCalibration().RatioUpper(model.QuotaKey()); got < want-1e-9 {
		t.Fatalf("calibration ratio = %v, want at least %v (inferred from the declared limit)", got, want)
	}
}

// TestEmbedWithinLimitEmptyTexts: nothing to embed is a no-op, not a call.
func TestEmbedWithinLimitEmptyTexts(t *testing.T) {
	driver := &recordingEmbedDriver{}
	model := newEmbedModel(t, driver, "", 8192)
	embeds, err := model.EmbedWithinLimit(t.Context(), EmbedRequest{}, nil, nil)
	if err != nil {
		t.Fatalf("EmbedWithinLimit: %v", err)
	}
	if len(embeds) != 0 || len(driver.attempts) != 0 {
		t.Fatalf("embeds = %d, attempts = %d, want 0 and 0", len(embeds), len(driver.attempts))
	}
}
