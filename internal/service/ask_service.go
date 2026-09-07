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

package service

import (
	"context"

	"ragflow/internal/common"
	modelModule "ragflow/internal/entity/models"

	"go.uber.org/zap"
)

// Defaults for the Ask pipeline — match Python bot_api.py.
const (
	DefaultAskPage                   = 1
	DefaultAskPageSize               = 12
	DefaultAskTopK                   = 1024
	DefaultAskSimilarityThreshold    = 0.1
	DefaultAskVectorSimilarityWeight = 0.3
	DefaultAskTokenBudget            = 4096
	DefaultAskStreamMinTokens        = 16
)

// AskDeltaKind classifies a streaming event emitted by AskService.
type AskDeltaKind int

const (
	AskDeltaAnswer AskDeltaKind = iota // visible answer text delta
	AskDeltaMarker                     // <think> or </think> boundary
	AskDeltaError                      // non-fatal error message / early stop
	AskDeltaFinal                      // final event with references
)

// AskDelta is a single streaming event from AskService.Stream.
type AskDelta struct {
	Kind  AskDeltaKind
	Value string
	Refs  interface{} // populated on AskDeltaFinal: {chunks, doc_aggs}
}

// Retriever abstracts chunk retrieval for AskService.
type Retriever interface {
	RetrievalTest(req *RetrievalTestRequest, userID string) (*RetrievalTestResponse, error)
}

// StreamingLLM abstracts streaming chat for AskService.
type StreamingLLM interface {
	ChatStream(ctx context.Context, messages []modelModule.Message, config *modelModule.ChatConfig) (<-chan string, error)
}

// AskService performs retrieval-augmented Q&A with streaming output.
// Embedder may be nil; if nil, citation insertion is skipped.
type AskService struct {
	retriever       Retriever
	embedder        Embedder
	tokenBudget     int
	minStreamTokens int
}

// NewAskService creates an AskService.
func NewAskService(retriever Retriever, embedder Embedder, tokenBudget, minStreamTokens int) *AskService {
	if tokenBudget <= 0 {
		tokenBudget = DefaultAskTokenBudget
	}
	if minStreamTokens <= 0 {
		minStreamTokens = DefaultAskStreamMinTokens
	}
	return &AskService{
		retriever:       retriever,
		embedder:        embedder,
		tokenBudget:     tokenBudget,
		minStreamTokens: minStreamTokens,
	}
}

// Stream runs the full ask pipeline.  llm must not be nil.  The returned
// channel is closed when the pipeline completes or ctx is cancelled.
func (s *AskService) Stream(ctx context.Context, llm StreamingLLM, userID, question string, kbIDs []string) <-chan AskDelta {
	out := make(chan AskDelta, 32)
	go func() {
		defer close(out)
		s.run(ctx, llm, userID, question, kbIDs, out)
	}()
	return out
}

func (s *AskService) run(ctx context.Context, llm StreamingLLM, userID, question string, kbIDs []string, out chan<- AskDelta) {
	// Phase 1: Retrieval.
	req := &RetrievalTestRequest{
		Datasets:               common.StringSlice(kbIDs),
		Question:               question,
		TopK:                   ptrInt(DefaultAskTopK),
		SimilarityThreshold:    ptrFloat64(DefaultAskSimilarityThreshold),
		VectorSimilarityWeight: ptrFloat64(DefaultAskVectorSimilarityWeight),
	}
	page := DefaultAskPage
	ps := DefaultAskPageSize
	req.Page = &page
	req.Size = &ps

	result, err := s.retriever.RetrievalTest(req, userID)
	if err != nil {
		common.Warn("AskService retrieval failed", zap.Error(err))
		s.sendOrCancel(out, AskDelta{Kind: AskDeltaError, Value: "retrieval failed"}, ctx)
		return
	}
	if result == nil || len(result.Chunks) == 0 {
		s.sendOrCancel(out, AskDelta{Kind: AskDeltaError, Value: "Sorry, no relevant information provided."}, ctx)
		return
	}

	chunks := NewSourcedChunks(result.Chunks)

	// Phase 2: Build system prompt.
	knowledge := KbPrompt(chunks, s.tokenBudget)
	prompt, err := LoadPrompt("ask_summary")
	if err != nil {
		common.Warn("AskService failed to load prompt", zap.Error(err))
		s.sendOrCancel(out, AskDelta{Kind: AskDeltaError, Value: "prompt configuration error"}, ctx)
		return
	}
	sysPrompt := RenderPrompt(prompt, map[string]interface{}{"knowledge": knowledge})

	messages := []modelModule.Message{
		{Role: "system", Content: sysPrompt},
		{Role: "user", Content: question},
	}
	genConf := &modelModule.ChatConfig{Temperature: ptrFloat64(0.1)}

	ch, err := llm.ChatStream(ctx, messages, genConf)
	if err != nil {
		common.Warn("AskService LLM stream failed", zap.Error(err))
		s.sendOrCancel(out, AskDelta{Kind: AskDeltaError, Value: "LLM call failed"}, ctx)
		return
	}

	// Phase 3: Stream LLM output with think-tag processing.
	var fullAnswer string
	for delta := range StreamThinkTagDelta(ctx, ch, s.minStreamTokens) {
		switch delta.Kind {
		case ThinkDeltaMarker:
			s.sendOrCancel(out, AskDelta{Kind: AskDeltaMarker, Value: delta.Value}, ctx)
		case ThinkDeltaText:
			fullAnswer += delta.Value
			s.sendOrCancel(out, AskDelta{Kind: AskDeltaAnswer, Value: delta.Value}, ctx)
		}
	}

	// Phase 4: Finalize — citation insertion + reference formatting.
	visible := ExtractVisibleAnswer(fullAnswer)
	chunkRefs := ChunksFormat(chunks)

	// Attempt citation insertion if embedder is available.
	chunkVectors := ExtractChunkVectors(result.Chunks)
	if len(chunkVectors) > 0 && s.embedder != nil {
		if decorated, cited := InsertCitations(visible, chunks, s.embedder, chunkVectors); len(cited) > 0 {
			visible = decorated
		}
	}

	refs := map[string]interface{}{
		"chunks":   chunkRefs,
		"doc_aggs": result.DocAggs,
	}
	s.sendOrCancel(out, AskDelta{Kind: AskDeltaFinal, Value: visible, Refs: refs}, ctx)
}

func (s *AskService) sendOrCancel(out chan<- AskDelta, d AskDelta, ctx context.Context) {
	select {
	case out <- d:
	case <-ctx.Done():
	}
}

// ExtractChunkVectors extracts float64 vectors from retrieval result chunks.
// Returns nil for chunks that have no, empty, or all-zero vectors.
func ExtractChunkVectors(chunks []map[string]interface{}) [][]float64 {
	if len(chunks) == 0 {
		return nil
	}
	out := make([][]float64, 0, len(chunks))
	for _, ck := range chunks {
		v := toFloat64Slice(ck["vector"])
		if len(v) == 0 || common.IsZeroVector(v) {
			out = append(out, nil)
		} else {
			out = append(out, v)
		}
	}
	return out
}

func toFloat64Slice(v interface{}) []float64 {
	switch val := v.(type) {
	case []float64:
		out := make([]float64, len(val))
		copy(out, val)
		return out
	case []interface{}:
		out := make([]float64, len(val))
		for i, x := range val {
			if f, ok := x.(float64); ok {
				out[i] = f
			}
		}
		return out
	default:
		return nil
	}
}

func ptrInt(v int) *int          { return &v }
func ptrFloat64(v float64) *float64 { return &v }
