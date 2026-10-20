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
	"sync"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity/models"
	"ragflow/internal/tokenizer"
)

// countingSpy wraps a real counter and records how often the embedder path
// counts and trims. The embedder must count each candidate text exactly once
// per ladder attempt (in trimAll); usage distribution, calibration and the
// over-limit observation reuse that number instead of re-running the
// tokenizer over the same texts.
type countingSpy struct {
	tokenizer.Counter
	mu     sync.Mutex
	counts int
	trims  int
}

func (c *countingSpy) Count(text string) int {
	c.mu.Lock()
	c.counts++
	c.mu.Unlock()
	return c.Counter.Count(text)
}

func (c *countingSpy) TrimToLimit(text string, limit int) string {
	c.mu.Lock()
	c.trims++
	c.mu.Unlock()
	return c.Counter.TrimToLimit(text, limit)
}

func (c *countingSpy) stats() (counts, trims int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.counts, c.trims
}

// installCountingSpy swaps the process-wide cl100k counter for a spy. The
// embedder under test declares no tokenizer of its own, so both the embedder
// and the model layer resolve cl100k; routing it through the spy makes every
// count observable. The original is restored on cleanup; if cl100k was never
// registered the spy stays installed, which is behaviorally identical because
// it delegates to the real counter.
func installCountingSpy(t *testing.T) *countingSpy {
	t.Helper()
	spy := &countingSpy{Counter: tokenizer.CountCL100K()}
	if orig, ok := tokenizer.CounterByID(tokenizer.CounterCL100K); ok {
		t.Cleanup(func() { tokenizer.RegisterCounter(tokenizer.CounterCL100K, orig) })
	}
	tokenizer.RegisterCounter(tokenizer.CounterCL100K, spy)
	return spy
}

// usageDriver reports a request-level usage like a real provider without
// counting anything itself, so the spy only observes the embedder's counting.
type usageDriver struct {
	stubDriver
	perText int
}

func (d *usageDriver) Embed(ctx context.Context, modelName *string, request models.EmbedRequest, apiConfig *models.APIConfig, embeddingConfig *models.EmbeddingConfig, usage *common.ModelUsage) ([]models.EmbeddingData, error) {
	if usage != nil {
		usage.InputTokens = d.perText * len(request.Texts)
	}
	return d.stubDriver.Embed(ctx, modelName, request, apiConfig, embeddingConfig, usage)
}

// TestEncodeSuccessPathCountsEachTextOnce is the headline count-once guard:
// on the success path the embedder may run the tokenizer once per text (the
// trim) and must count each trimmed text exactly once — the provider-usage
// distribution and the usage-based calibration reuse that count. The model
// layer's own trim of the same texts is out of scope here and not asserted.
func TestEncodeSuccessPathCountsEachTextOnce(t *testing.T) {
	spy := installCountingSpy(t)

	emb := newLimitEmbedder(&usageDriver{perText: 7}, 8192)
	texts := []string{"alpha beta", "gamma delta epsilon", "zeta"}
	vecs, err := emb.Encode(context.Background(), texts)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if len(vecs) != len(texts) {
		t.Fatalf("got %d vectors, want %d", len(vecs), len(texts))
	}
	counts, _ := spy.stats()
	if counts != len(texts) {
		t.Errorf("Count called %d times, want %d (exactly one count per text on the success path)", counts, len(texts))
	}
}

// TestTrimAllCountsEachTextOnce pins the extraction itself: trimAll trims and
// counts every text exactly once and reports counts that match the trimmed
// texts the distribute/calibration paths will reuse.
func TestTrimAllCountsEachTextOnce(t *testing.T) {
	cases := []struct {
		name  string
		texts []string
		limit int
	}{
		{"nil input", nil, 100},
		{"empty input", []string{}, 100},
		{"single text", []string{"alpha beta"}, 100},
		{"multiple texts", []string{"alpha beta", "gamma delta epsilon zeta", "eta"}, 100},
		{"empty string", []string{""}, 100},
		{"zero limit", []string{"alpha beta", "gamma"}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			spy := &countingSpy{Counter: tokenizer.CountCL100K()}
			out, own := trimAll(c.texts, spy, c.limit)

			counts, trims := spy.stats()
			if counts != len(c.texts) || trims != len(c.texts) {
				t.Fatalf("Count called %d times, TrimToLimit %d times, want %d each", counts, trims, len(c.texts))
			}
			if len(out) != len(c.texts) || len(own) != len(c.texts) {
				t.Fatalf("got (%d texts, %d counts), want %d each", len(out), len(own), len(c.texts))
			}
			for i := range c.texts {
				want := tokenizer.CountCL100K().Count(out[i])
				if own[i] != want {
					t.Errorf("own[%d] = %d, want the count of the trimmed text %d", i, own[i], want)
				}
			}
		})
	}
}

// TestRecordUsageObservesSummedOwnCount pins that the calibration observation
// uses the precomputed per-input counts: the provider total against their sum,
// with the documented early returns preserved. The assertion is deliberately
// tight (not just "> 1") so it catches a future change that stops summing or
// silently no-ops.
func TestRecordUsageObservesSummedOwnCount(t *testing.T) {
	// Use an isolated calibration so the assertion is not masked by the
	// process-wide DefaultCalibration's starting bound (1.05) nor by other
	// tests observing the same key.
	cal := tokenizer.NewCalibration(tokenizer.DefaultUncountedRatioUpper)
	const key = "spy-record-usage-key"
	emb := &embedder{}

	// own=[2,3,1] sums to 6 against usage 21 -> ratio 3.5. A weak "> 1"
	// assertion would also pass if recordUsage did nothing (default 1.05),
	// so pin the specific ratio.
	emb.recordUsage(cal, key, []int{2, 3, 1}, 21)
	if got := cal.RatioUpper(key); got < 3 || got > 4 {
		t.Fatalf("RatioUpper after observing own sum=6 vs usage=21 = %v, want ~3.5", got)
	}

	// Summation, not max/first element: the same total delivered as a single
	// element must produce the identical ratio. This fails if recordUsage ever
	// switches to max() or to the first element instead of the sum.
	const keySingle = "spy-record-usage-key-single"
	emb.recordUsage(cal, keySingle, []int{6}, 21)
	if got, want := cal.RatioUpper(keySingle), cal.RatioUpper(key); got != want {
		t.Fatalf("RatioUpper for summed [2,3,1] = %v but for [6] = %v; recordUsage must use the sum", want, got)
	}

	// Non-positive usage and all-zero own counts must not move the ratio.
	before := cal.RatioUpper(key)
	emb.recordUsage(cal, key, []int{2, 3, 1}, 0) // realCount <= 0 -> no-op
	emb.recordUsage(cal, key, []int{0, 0}, 21)   // ownCount <= 0 -> no-op
	if after := cal.RatioUpper(key); after != before {
		t.Fatalf("RatioUpper moved from %v to %v on no-op observations", before, after)
	}
}

// TestEncodeTokenCountsFallBackToOwnCount pins that the count-once refactor
// preserves the accounting contract: when the provider reports no usage, each
// vector still carries our own count of the text that was actually sent.
func TestEncodeTokenCountsFallBackToOwnCount(t *testing.T) {
	installCountingSpy(t)

	emb := newLimitEmbedder(&stubDriver{}, 8192) // stubDriver reports no usage
	texts := []string{"alpha beta gamma", "delta epsilon"}
	vecs, err := emb.Encode(context.Background(), texts)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	for i, v := range vecs {
		want := tokenizer.CountCL100K().Count(texts[i])
		if v.TokenCount != want {
			t.Errorf("vector %d token count = %d, want our own count %d", i, v.TokenCount, want)
		}
	}
}

// TestEncodeTokenCountsSumToProviderUsage pins the distribution contract on
// the refactored path: the per-input counts still sum to the provider total.
func TestEncodeTokenCountsSumToProviderUsage(t *testing.T) {
	installCountingSpy(t)

	emb := newLimitEmbedder(&usageDriver{perText: 7}, 8192)
	texts := []string{"alpha beta", "gamma delta epsilon", "zeta"}
	vecs, err := emb.Encode(context.Background(), texts)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	sum := 0
	for _, v := range vecs {
		if v.TokenCount <= 0 {
			t.Errorf("vector token count = %d, want positive", v.TokenCount)
		}
		sum += v.TokenCount
	}
	if want := 7 * len(texts); sum != want {
		t.Errorf("token counts sum to %d, want the provider total %d", sum, want)
	}
}
