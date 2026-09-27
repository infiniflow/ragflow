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
	"fmt"

	"ragflow/internal/common"
	"ragflow/internal/tokenizer"
)

// EmbedWithinLimit embeds req after cutting every text to what this model accepts,
// and retries with a smaller budget if the provider still rejects an input as over
// its window. It is the embedding counterpart of RerankModel.Rerank, which cuts its
// documents the same way for the same reason.
//
// Callers use it instead of ModelDriver.Embed because a provider does NOT truncate
// an over-window input: it rejects the whole request (SiliconFlow answers 400 with
// code 20015 for an input 0.55% over spec). The document-side callers - the
// dataset-nav summary, the knowledge-compiler products, chunk and memory writes -
// have no length bound of their own, so without this cut the request is sent as-is
// and, because such a batch is retried unchanged, fails forever. Python solves it
// one layer down in BaseEmbedding.encode; Go has no such layer, so the model
// carries it. Query-side callers keep calling the driver directly: their texts are
// short, and they must keep working on a deployment whose tokenizer assets are
// missing.
//
// Only over-limit rejections are retried, each one with a strictly smaller budget:
// a rate limit belongs to the driver's own retry policy and a 5xx is not something
// a shorter input fixes. When even the batch ladder is rejected, each input is
// embedded on its own down to the floor, so one pathological text neither fails the
// whole batch nor drags its healthy neighbours down to the floor with it. A model
// that declares a tokenizer whose asset is not on disk is refused before the first
// attempt, because counting it with the calibrated cl100k estimate is exactly what
// lets an oversized request through.
func (m *EmbeddingModel) EmbedWithinLimit(ctx context.Context, req EmbedRequest, embeddingConfig *EmbeddingConfig, usage *common.ModelUsage) ([]EmbeddingData, error) {
	if m == nil || m.ModelDriver == nil {
		return nil, fmt.Errorf("embedding model: driver is nil")
	}
	if len(req.Texts) == 0 {
		return []EmbeddingData{}, nil
	}
	tokenizerID, quotaKey := m.ResolveTokenizerID(), m.QuotaKey()
	if err := tokenizer.RefuseUnavailableCounter(tokenizerID, quotaKey); err != nil {
		return nil, err
	}
	limiter := tokenizer.LimiterFor(tokenizerID, quotaKey, tokenizer.DefaultCalibration())
	counter := limiter.Counter()
	cal, calKey := limiter.Calibration()
	// maxTokens is what the provider declares and window is the local budget derived
	// from it (calibrated ratio plus margin). The first rung cuts to window, so the
	// common case is a single call; a rejection is evidence about maxTokens, which is
	// what the calibration has to be told.
	maxTokens := m.ResolveMaxTokens()
	window := limiter.Limit(maxTokens)

	for _, budget := range tokenizer.OverLimitLadder(window) {
		embeds, err := m.embedCut(ctx, req, embeddingConfig, usage, counter, budget)
		if err == nil {
			return embeds, nil
		}
		if !tokenizer.IsOverLimitError(err) {
			return nil, err
		}
		// Our count said OwnTokenMax tokens for an input the provider just rejected:
		// record it, so the next call - and the next document - starts from a tighter
		// budget instead of re-learning the same thing.
		cal.ObserveOverLimit(calKey, tokenizer.OwnTokenMax(req.Texts, counter), maxTokens)
	}

	// The batch ladder could not fit the batch, which usually means one input is
	// pathological next to healthy ones. Isolate instead of failing: each input gets
	// its own walk down to OverLimitFloorTokens, and the healthy ones keep the budget
	// they had. A failure here is reported as isolation's own - it names the input and
	// the floor it reached, while the batch rejection above says only "the whole
	// request was too big".
	return m.embedIsolating(ctx, req, embeddingConfig, usage, counter, cal, calKey, maxTokens, window)
}

// embedCut trims every text to budget and performs exactly one embedding call.
func (m *EmbeddingModel) embedCut(ctx context.Context, req EmbedRequest, embeddingConfig *EmbeddingConfig, usage *common.ModelUsage, counter tokenizer.Counter, budget int) ([]EmbeddingData, error) {
	attempt := req
	attempt.Texts = make([]string, len(req.Texts))
	for i, text := range req.Texts {
		attempt.Texts[i] = counter.TrimToLimit(text, budget)
	}
	return m.ModelDriver.Embed(ctx, m.ModelName, attempt, m.APIConfig, embeddingConfig, usage)
}

// embedIsolating embeds each input of req on its own, walking the ladder to the
// floor for the ones that need it. It returns one embedding per input, in the
// original order, and never a partial result: an input that does not fit even at the
// floor fails the call.
func (m *EmbeddingModel) embedIsolating(ctx context.Context, req EmbedRequest, embeddingConfig *EmbeddingConfig, usage *common.ModelUsage, counter tokenizer.Counter, cal *tokenizer.Calibration, calKey string, maxTokens, window int) ([]EmbeddingData, error) {
	out := make([]EmbeddingData, len(req.Texts))
	for i, text := range req.Texts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		single := req
		single.Texts = []string{text}
		var lastErr error
		fitted := false
		for _, budget := range tokenizer.OverLimitLadderToFloor(window) {
			embeds, err := m.embedCut(ctx, single, embeddingConfig, usage, counter, budget)
			if err == nil {
				if len(embeds) != 1 {
					return nil, fmt.Errorf("embedding input %d: driver returned %d embeddings for one text", i, len(embeds))
				}
				out[i], fitted = embeds[0], true
				break
			}
			if !tokenizer.IsOverLimitError(err) {
				return nil, err
			}
			cal.ObserveOverLimit(calKey, counter.Count(counter.TrimToLimit(text, budget)), maxTokens)
			lastErr = err
		}
		if !fitted {
			return nil, fmt.Errorf("embedding input %d does not fit the model window even at %d tokens: %w", i, tokenizer.OverLimitFloorTokens, lastErr)
		}
	}
	return out, nil
}
