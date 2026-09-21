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
// a shorter input fixes. A model that declares a tokenizer whose asset is not on
// disk is refused before the first attempt, because counting it with the calibrated
// cl100k estimate is exactly what lets an oversized request through.
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
	// The window the provider enforces: the first rung of the ladder is exactly the
	// budget a single cut would use, so the common case is one call.
	window := limiter.Limit(m.ResolveMaxTokens())

	var lastErr error
	for _, budget := range tokenizer.OverLimitLadder(window) {
		attempt := req
		attempt.Texts = make([]string, len(req.Texts))
		for i, text := range req.Texts {
			attempt.Texts[i] = counter.TrimToLimit(text, budget)
		}
		embeds, err := m.ModelDriver.Embed(ctx, m.ModelName, attempt, m.APIConfig, embeddingConfig, usage)
		if err == nil {
			return embeds, nil
		}
		if !tokenizer.IsOverLimitError(err) {
			return nil, err
		}
		// The rejection proves the real count exceeded the window while ours said
		// OwnTokenMax: record it, so the next call - and the next document - starts
		// from a tighter budget instead of re-learning the same thing.
		cal.ObserveOverLimit(calKey, tokenizer.OwnTokenMax(attempt.Texts, counter), window)
		lastErr = err
	}
	return nil, fmt.Errorf("embedding input does not fit the model window even at %d tokens: %w", tokenizer.OverLimitFloorTokens, lastErr)
}
