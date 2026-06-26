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

// retrieval_nlp.go — NLPRetrievalAdapter wiring.
//
// The agent tool layer (tool/retrieval_service.go) declares a
// minimal RetrievalService interface. Until this file landed, the
// only registered implementation was the stub that returns
// ErrRetrievalServiceMissing. NLPRetrievalAdapter bridges the
// agent-side interface to the production nlp.RetrievalService —
// the same service that powers chat / dataset search / chunk
// retrieval across the rest of the codebase.
//
// Wiring is one line at boot:
//
//	tool.SetRetrievalService(tool.NewNLPRetrievalAdapter(
//	    nlp.NewRetrievalService(docEngine, documentDAO),
//	))
//
// Translation rules:
//
//   tool.RetrievalRequest.Query      → nlp.RetrievalRequest.Question
//   tool.RetrievalRequest.DatasetIDs → nlp.RetrievalRequest.KbIDs
//   tool.RetrievalRequest.TopN       → nlp.RetrievalRequest.PageSize
//                                       (Page=1, Top=TopN*4 so rerank
//                                        has headroom)
//   tool.RetrievalRequest.UseKG      → ErrGraphRAGNotSupported (out of
//                                       scope per plan  + §9 Q3)
//
// Chunk shape translation: nlp's Chunks are []map[string]any with
// keys chunk_id, doc_id, docnm_kwd, content_with_weight,
// content_ltks, similarity, term_similarity, vector_similarity. The
// tool side wants a flat RetrievalChunk{ID, Content, DocumentID,
// Score}. We pick the most user-facing fields:
//   - ID         ← chunk_id
//   - Content    ← content_with_weight (fallback to content_ltks)
//   - DocumentID ← doc_id
//   - Score      ← similarity (fallback to avg of term+vector)
//
// Defensive defaults: missing or wrong-typed chunk fields become
// empty strings / 0.0 rather than panicking — a single malformed
// chunk from the doc engine shouldn't take down the whole
// retrieval call.

package tool

import (
	"context"

	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/service/nlp"
)

// NLPRetrievalAdapter wraps *nlp.RetrievalService behind the
// agent-tool RetrievalService interface. The adapter is safe to
// share across goroutines — the wrapped service is stateless
// beyond its docEngine + documentDAO handles, both of which the
// nlp package treats as concurrent-safe.
type NLPRetrievalAdapter struct {
	svc *nlp.RetrievalService
}

// NewNLPRetrievalAdapter wraps an already-constructed
// *nlp.RetrievalService.
func NewNLPRetrievalAdapter(svc *nlp.RetrievalService) *NLPRetrievalAdapter {
	return &NLPRetrievalAdapter{svc: svc}
}

// NewNLPRetrievalAdapterFromDeps is the convenience constructor
// for the common boot path:
//
//	tool.SetRetrievalService(tool.NewNLPRetrievalAdapterFromDeps(docEngine, docDAO))
//
// matches chat_session.go's newChatSessionServiceWithRetrieval
// call site.
func NewNLPRetrievalAdapterFromDeps(docEngine engine.DocEngine, documentDAO *dao.DocumentDAO) *NLPRetrievalAdapter {
	return &NLPRetrievalAdapter{svc: nlp.NewRetrievalService(docEngine, documentDAO)}
}

// Search implements RetrievalService. The translation rules live
// at the top of this file.
func (a *NLPRetrievalAdapter) Search(ctx context.Context, req RetrievalRequest) ([]RetrievalChunk, error) {
	if a == nil || a.svc == nil {
		return nil, ErrRetrievalServiceMissing
	}
	if req.UseKG {
		// Plan  + §9 Q3: GraphRAG is out of scope for the
		// Go Canvas. The tool layer also returns the error; we
		// surface it here so any future direct caller of the
		// adapter (bypassing the tool envelope) sees the same
		// contract.
		return nil, ErrGraphRAGNotSupported
	}
	if req.Query == "" {
		return nil, nil
	}
	topN := req.TopN
	if topN <= 0 {
		topN = 8
	}

	// nlp.Retrieval applies its own defaults for SimilarityThreshold
	// (0.2), VectorSimilarityWeight (0.3), RankFeature, etc. We
	// surface only the fields the agent tool actually controls:
	// Page=1, PageSize=TopN, KbIDs=DatasetIDs, Top=TopN*4 (rerank
	// headroom — matches the chat_session.go call pattern).
	nlpReq := &nlp.RetrievalRequest{
		Question:  req.Query,
		KbIDs:     append([]string(nil), req.DatasetIDs...),
		Page:      1,
		PageSize:  topN,
		Aggs:      boolPtr(false),
		Highlight: boolPtr(false),
	}
	if topN > 0 {
		rerankBudget := topN * 4
		nlpReq.Top = &rerankBudget
	}
	if req.SimilarityThreshold > 0 {
		nlpReq.SimilarityThreshold = &req.SimilarityThreshold
	}

	res, err := a.svc.Retrieval(ctx, nlpReq)
	if err != nil {
		return nil, err
	}
	if res == nil || len(res.Chunks) == 0 {
		return []RetrievalChunk{}, nil
	}
	out := make([]RetrievalChunk, 0, len(res.Chunks))
	for _, raw := range res.Chunks {
		out = append(out, translateChunk(raw))
	}
	return out, nil
}

// translateChunk converts one nlp chunk map into a RetrievalChunk.
// Tolerates missing fields (returns zero values) and wrong types
// (returns zero values) so a single bad chunk from the doc engine
// can't break the whole result list.
func translateChunk(raw map[string]any) RetrievalChunk {
	return RetrievalChunk{
		ID:         stringFromMap(raw, "chunk_id"),
		Content:    contentFromMap(raw),
		DocumentID: stringFromMap(raw, "doc_id"),
		Score:      scoreFromMap(raw),
	}
}

// stringFromMap returns raw[key].(string) or "" if missing / wrong
// type. Keeps the translator compact.
func stringFromMap(raw map[string]any, key string) string {
	if v, ok := raw[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// contentFromMap picks the most user-facing content field. nlp
// chunks carry content_with_weight (the highlightable string) and
// content_ltks (the tokenised form). content_with_weight is what
// the model sees in Python; we use it here too. Empty / missing →
// fall back to content_ltks; both empty → empty string.
func contentFromMap(raw map[string]any) string {
	if v := stringFromMap(raw, "content_with_weight"); v != "" {
		return v
	}
	return stringFromMap(raw, "content_ltks")
}

// scoreFromMap returns the chunk's similarity score. nlp populates
// three fields — similarity (combined), term_similarity (BM25),
// vector_similarity (cosine). We prefer similarity; if absent or
// zero, average the two sub-scores. Wrong-type values → fall through
// to sub-scores; missing sub-scores → 0.
func scoreFromMap(raw map[string]any) float64 {
	if f, ok := numberFromMap(raw, "similarity"); ok {
		return f
	}
	term, termOK := numberFromMap(raw, "term_similarity")
	vec, vecOK := numberFromMap(raw, "vector_similarity")
	if termOK && vecOK {
		return (term + vec) / 2
	}
	if termOK {
		return term
	}
	if vecOK {
		return vec
	}
	return 0
}

// numberFromMap returns raw[key].(float64) with a tolerant path
// for ints. JSON unmarshaling can produce either.
func numberFromMap(raw map[string]any, key string) (float64, bool) {
	v, ok := raw[key]
	if !ok {
		return 0, false
	}
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	}
	return 0, false
}

func boolPtr(b bool) *bool { return &b }
