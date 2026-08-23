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
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/engine"
	enginetypes "ragflow/internal/engine/types"
)

// grepQueryTimeout bounds a single engine query issued by GrepAdapter. It also
// guards against eino's streaming ReAct handing the tool an already-canceled
// context right after the model emits tool_calls: when ctx.Err() is already
// canceled we fall back to a fresh bounded context so the engine query is not
// killed by that streaming artifact. The agent's smartReasoningTimeout still
// caps the whole run.
var grepQueryTimeout = 30 * time.Second

// Elasticsearch it pushes the regex down to a native `regexp` query; on engines
// without native regex support it falls back to a broad recall + in-memory RE2
// filter. It is stateless and safe to share across goroutines.
type GrepAdapter struct {
	docEngine engine.DocEngine
}

// NewGrepAdapter wraps a doc engine behind the GrepService interface.
func NewGrepAdapter(docEngine engine.DocEngine) *GrepAdapter {
	return &GrepAdapter{docEngine: docEngine}
}

// regexpSearchable is the narrow engine capability for native regex search.
// *elasticsearch.Engine implements it; other engines do not.
type regexpSearchable interface {
	SearchByRegexp(ctx context.Context, req *enginetypes.RegexpSearchRequest) (*enginetypes.SearchResult, error)
}

// ListByDocIDs reads the full original chunks belonging to the given document
// ids, skipping graph (relation/entity/location) chunks so callers get the
// actual document text. It backs the list_chunks deep-read runtime.
// Because content_with_weight is now a searchable keyword field, it reads the
// docs' chunks directly via the regexp pushdown (match-all "." scoped by
// doc_id), no longer needing a content_ltks-based general recall.
func (g *GrepAdapter) ListByDocIDs(ctx context.Context, req runtime.GrepRequest) ([]runtime.RetrievalChunk, error) {
	if g == nil || g.docEngine == nil {
		return nil, runtime.ErrGrepServiceMissing
	}
	docIDs := nonEmptyStrings(req.DocScope)
	if len(docIDs) == 0 {
		return nil, nil
	}
	limit := req.Limit
	if limit <= 0 {
		limit = grepChunksDefaultLimit
	}
	offset := max(req.Offset, 0)

	// Use a bounded engine context, same rationale as Grep.
	ectx, ecancel := engineCallContext(ctx)
	defer ecancel()

	se, ok := g.docEngine.(regexpSearchable)
	if !ok {
		return nil, runtime.ErrRegexpNotSupported
	}
	// Match-all on the (now keyword) content_with_weight, scoped to the docs,
	// restricted to ordinary text chunks (available_int=1, no compile_kwd). When
	// the caller requests a sort (e.g. reading order), push it down so ES applies
	// offset/limit over the deterministically-ordered set — no over-fetching.
	res, err := se.SearchByRegexp(ectx, &enginetypes.RegexpSearchRequest{
		TenantID:     req.TenantID,
		KbIDs:        req.DatasetIDs,
		Offset:       offset,
		Limit:        limit,
		Pattern:      ".*",
		Sort:         sortExprFromFields(req.Sort), // reading order: doc_id, page_num_int, chunk_order_int
		SelectFields: req.SelectFields,
		Filter: map[string]interface{}{
			"available_int": 1,
			"doc_id":        docIDs,
			"must_not":      map[string]interface{}{"exists": "compile_kwd"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("list_chunks: deep-read recall: %w", err)
	}

	out := make([]runtime.RetrievalChunk, 0, limit)
	for _, raw := range res.Chunks {
		content := contentWithWeightFromRaw(raw)
		if content == "" || isGraphChunkContent(content) {
			continue
		}
		out = append(out, runtime.RetrievalChunk{
			ID:           runtime.FirstStringFromMap(raw, "id", "_id"),
			Content:      content,
			DocumentID:   runtime.StringFromMap(raw, "doc_id"),
			DocumentName: runtime.StringFromMap(raw, "docnm_kwd"),
			DatasetID:    runtime.StringFromMap(raw, "kb_id"),
			ChunkIndex:   runtime.IntFromMap(raw, "chunk_order_int"),
			PageNum:      runtime.IntFromMap(raw, "page_num_int"),
		})
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// Grep regex-matches chunk content within the given scope.
func (g *GrepAdapter) Grep(ctx context.Context, req runtime.GrepRequest) ([]runtime.RetrievalChunk, error) {
	if g == nil || g.docEngine == nil {
		return nil, runtime.ErrGrepServiceMissing
	}
	if strings.TrimSpace(req.Pattern) == "" {
		return nil, fmt.Errorf("grep: pattern cannot be empty")
	}
	// Validate the regex on the Go side regardless of pushdown path.
	if _, err := regexp.Compile("(?i)" + req.Pattern); err != nil {
		return nil, fmt.Errorf("grep: invalid regex: %w", err)
	}

	limit := req.Limit
	if limit <= 0 {
		limit = grepChunksDefaultLimit
	}

	// Only search ordinary document text chunks: available_int=1 and no
	// compile_kwd (which marks knowledge-compiled products like wiki_page,
	// hypergraph, mindmap, list, timeline). Knowledge products are derived
	// content, not the original document prose the user asked about, and several
	// of them exceed ES's keyword term length limit.
	filter := map[string]interface{}{
		"available_int": 1,
		"must_not":      map[string]interface{}{"exists": "compile_kwd"},
	}
	if len(req.DocScope) > 0 {
		filter["doc_id"] = req.DocScope
	}

	// Use a bounded engine context. eino's streaming ReAct can hand the tool an
	// already-canceled context right after the model emits tool_calls; a canceled
	// parent must not kill the engine query, so we fall back to a fresh bounded
	// context in that case. The agent's smartReasoningTimeout caps the whole run.
	ectx, ecancel := engineCallContext(ctx)
	defer ecancel()

	// Path 1: native regex pushdown (Elasticsearch). With content_with_weight
	// mapped as a searchable keyword field, ES regexp matching is the sole path;
	// there is intentionally no in-memory RE2 fallback (a pushdown failure is a
	// real error the caller should surface, not silently "no matches").
	if se, ok := g.docEngine.(regexpSearchable); ok {
		res, err := se.SearchByRegexp(ectx, &enginetypes.RegexpSearchRequest{
			TenantID:     req.TenantID,
			KbIDs:        req.DatasetIDs,
			Limit:        limit,
			Pattern:      req.Pattern,
			Sort:         sortExprFromFields(req.Sort), // same reading order as list_chunks
			SelectFields: req.SelectFields,
			Filter:       filter,
		})
		if err != nil {
			return nil, fmt.Errorf("grep: regexp pushdown failed: %w", err)
		}
		return translateGrepChunks(res.Chunks), nil
	}

	// Non-ES engines do not implement regex matching on chunk content; surface
	// an explicit error rather than returning (empty) regex "matches".
	return nil, runtime.ErrRegexpNotSupported
}

// engineCallContext returns a bounded context for a single engine query. If the
// incoming context is already canceled (a streaming-ReAct artifact where eino
// cancels the tool context right after the model emits tool_calls), it derives
// from context.Background() so the query still runs; otherwise it derives from
// the caller's context so genuine cancellation/deadline is honored.
func engineCallContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx.Err() == nil {
		return context.WithTimeout(ctx, grepQueryTimeout)
	}
	return context.WithTimeout(context.Background(), grepQueryTimeout)
}

// translateGrepChunks converts ES regexp-search result maps into RetrievalChunk.
func translateGrepChunks(chunks []map[string]interface{}) []runtime.RetrievalChunk {
	out := make([]runtime.RetrievalChunk, 0, len(chunks))
	for _, raw := range chunks {
		out = append(out, runtime.RetrievalChunk{
			ID:           runtime.FirstStringFromMap(raw, "id", "_id"),
			Content:      contentWithWeightFromRaw(raw),
			DocumentID:   runtime.StringFromMap(raw, "doc_id"),
			DocumentName: runtime.StringFromMap(raw, "docnm_kwd"),
			DatasetID:    runtime.StringFromMap(raw, "kb_id"),
			ChunkIndex:   runtime.IntFromMap(raw, "chunk_order_int"),
			PageNum:      runtime.IntFromMap(raw, "page_num_int"),
		})
	}
	return out
}

// contentWithWeightFromRaw returns the chunk's content string, preferring
// content_with_weight and falling back to content.
func contentWithWeightFromRaw(raw map[string]interface{}) string {
	if v := runtime.StringFromMap(raw, "content_with_weight"); v != "" {
		return v
	}
	return runtime.StringFromMap(raw, "content")
}

// sortExprFromFields builds an ascending OrderByExpr from an ordered list of
// field names. It is shared by Grep (grep_chunks) and ListByDocIDs (list_chunks)
// so both push the same reading-order sort down to ES.
func sortExprFromFields(fields []string) *enginetypes.OrderByExpr {
	var expr *enginetypes.OrderByExpr
	for _, field := range nonEmptyStrings(fields) {
		if expr == nil {
			expr = &enginetypes.OrderByExpr{}
		}
		expr.Asc(field)
	}
	return expr
}

// nonEmptyStrings drops empty/whitespace-only strings, preserving order.
func nonEmptyStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

// isGraphChunkContent reports whether a chunk's content looks like a knowledge
// graph relation/entity/location JSON payload (e.g.
// {"head":"何进","relation_type":"鸩杀","tail":"董太后","type":"relation"}) rather
// than original document text. Deep-read and retrieval tools skip such chunks so
// the model reads actual document prose, not extracted graph triples.
func isGraphChunkContent(content string) bool {
	trimmed := strings.TrimSpace(content)
	if len(trimmed) < 2 || trimmed[0] != '{' || trimmed[len(trimmed)-1] != '}' {
		return false
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(trimmed), &m); err != nil {
		return false
	}
	if t, _ := m["type"].(string); t == "relation" || t == "entity" || t == "location" || t == "dataset_graph" {
		return true
	}
	_, hasHead := m["head"]
	_, hasTail := m["tail"]
	if hasHead || hasTail {
		return true
	}
	_, hasName := m["name"]
	_, hasSubject := m["subject"]
	_, hasPredicate := m["predicate"]
	_, hasObject := m["object"]
	return hasName || hasSubject || hasPredicate || hasObject
}
