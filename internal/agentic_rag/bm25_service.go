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

package agentic_rag

import (
	"context"
	"fmt"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/engine"
	enginetypes "ragflow/internal/engine/types"
)

// Bm25Adapter backs search_bm25_chunks with pure lexical full-text (BM25)
// ranking: it issues engine Search requests carrying only a MatchTextExpr over
// the default tokenized fields (title_tks^2, title_sm_tks^2, important_*,
// question_tks^20, content_ltks^2, content_sm_ltks) — no dense vector, no
// fusion — so engines score with native BM25. Like GrepAdapter it is stateless
// and safe to share across goroutines.
type Bm25Adapter struct {
	docEngine engine.DocEngine
	// queryTokenizer, when set, converts each raw query into a TOKENIZED
	// MatchTextExpr before it reaches the engine. This mirrors the ingestion
	// side: content_ltks/content_sm_ltks are indexed with the ragflow NLP
	// tokenizer's stemmed/subword tokens ("mineralization" -> "mineraliza
	// tion"), while the fields' search analyzers are plain whitespace — a raw
	// query word whose stemmed form differs from its raw form ("mineralizer",
	// "mineralization") can therefore NEVER match its own indexed token
	// without this tokenization step.
	queryTokenizer QueryTokenizer
}

// QueryTokenizer converts a raw query string into a tokenized match
// expression over the tokenized fields. Implemented by the service/nlp
// QueryBuilder (the same tokenizer family the ingestion pipeline used).
type QueryTokenizer interface {
	Question(txt string, tbl string, minMatch float64) (*enginetypes.MatchTextExpr, []string)
}

// SetQueryBuilder attaches the query tokenizer. Called at boot, after
// nlp.InitQueryBuilderFromTokenizer has run.
func (b *Bm25Adapter) SetQueryBuilder(qt QueryTokenizer) {
	if b == nil {
		return
	}
	b.queryTokenizer = qt
}

// NewBm25Adapter wraps a doc engine behind the Bm25Service interface.
func NewBm25Adapter(docEngine engine.DocEngine) *Bm25Adapter {
	return &Bm25Adapter{docEngine: docEngine}
}

// SearchBm25 runs each query as an independent BM25 text match, then merges
// results by chunk id (first occurrence wins, preserving BM25-best order) and
// drops knowledge-compiled products and graph payloads, mirroring the other
// retrieval tools so the model always reads original document prose.
func (b *Bm25Adapter) SearchBm25(ctx context.Context, req runtime.Bm25Request) ([]runtime.RetrievalChunk, error) {
	if b == nil || b.docEngine == nil {
		return nil, runtime.ErrBm25ServiceMissing
	}
	queries := nonEmptyStrings(req.Queries)
	if len(queries) == 0 {
		return nil, fmt.Errorf("search_bm25_chunks: queries must contain at least one non-empty query")
	}
	topN := req.TopN
	if topN <= 0 {
		topN = searchChunksDefaultTopN
	}
	if topN > 50 {
		topN = 50
	}

	filter := map[string]interface{}{
		"available_int": 1,
		"must_not":      map[string]interface{}{"exists": "compile_kwd"},
	}
	if len(req.DocScope) > 0 {
		filter["doc_id"] = req.DocScope
	}

	// Use a derived engine context with no per-query timeout, same rationale
	// as Grep.
	ectx, ecancel := engineCallContext(ctx)
	defer ecancel()

	seen := map[string]struct{}{}
	var merged []runtime.RetrievalChunk
	for _, q := range queries {
		// Tokenize the query through the same tokenizer family the ingestion
		// pipeline used (see Bm25Adapter.queryTokenizer): raw query words
		// cannot match their stemmed/subword index tokens. Falls back to the
		// raw text match when no query tokenizer is attached.
		matchExpr := b.buildMatchExpr(q, topN)
		res, err := b.docEngine.Search(ectx, &enginetypes.SearchRequest{
			IndexNames: []string{fmt.Sprintf("ragflow_%s", req.TenantID)},
			KbIDs:      req.DatasetIDs,
			Limit:      topN,
			// Fields left nil: engines apply their default tokenized field set
			// with per-field boosts. Text-only — no MatchDenseExpr, no
			// FusionExpr — so no vector leg exists and scores are raw BM25.
			MatchExprs: []interface{}{
				matchExpr,
			},
			Filter:       filter,
			SelectFields: grepChunksSelectFields,
		})
		if err != nil {
			return nil, fmt.Errorf("search_bm25_chunks: query %q failed: %w", q, err)
		}
		for _, raw := range res.Chunks {
			id := runtime.FirstStringFromMap(raw, "id", "_id")
			if _, dup := seen[id]; dup {
				continue
			}
			content := contentWithWeightFromRaw(raw)
			if content == "" || isGraphChunkContent(content) {
				continue
			}
			seen[id] = struct{}{}
			merged = append(merged, runtime.RetrievalChunk{
				ID:           id,
				Content:      content,
				DocumentID:   runtime.StringFromMap(raw, "doc_id"),
				DocumentName: runtime.StringFromMap(raw, "docnm_kwd"),
				DatasetID:    runtime.StringFromMap(raw, "kb_id"),
				ChunkIndex:   runtime.IntFromMap(raw, "chunk_order_int"),
				PageNum:      runtime.IntFromMap(raw, "page_num_int"),
				Score:        floatFromMap(raw, "_score"),
			})
		}
	}
	return merged, nil
}

// floatFromMap returns m[key] as float64 when present and numeric.
func floatFromMap(m map[string]interface{}, key string) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	if v, ok := m[key].(int); ok {
		return float64(v)
	}
	if v, ok := m[key].(int64); ok {
		return float64(v)
	}
	return 0
}

// buildMatchExpr converts a raw query into the match expression SearchBm25
// sends to the engine: tokenized via the attached QueryTokenizer when one is
// registered, otherwise the historical raw-text match.
func (b *Bm25Adapter) buildMatchExpr(q string, topN int) interface{} {
	if b == nil || b.queryTokenizer == nil {
		return &enginetypes.MatchTextExpr{MatchingText: q, TopN: topN}
	}
	expr, _ := b.queryTokenizer.Question(q, "", 0.0)
	if expr == nil {
		return &enginetypes.MatchTextExpr{MatchingText: q, TopN: topN}
	}
	// Question() hardcodes a topN of 100 for its own retrieval path; the
	// caller's topN governs here.
	expr.TopN = topN
	return expr
}
