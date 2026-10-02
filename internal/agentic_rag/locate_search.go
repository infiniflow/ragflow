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
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	"ragflow/internal/dao"
)

// runLocateSearch is the retrieval body shared by the two Semantic Bridge legs:
// search_chunks (hybrid, keyword weight chosen by the model, default 0.7) and
// search_semantic_chunks (pure vector, weight fixed at 0 — the model cannot
// blend, which is the whole point of that tool).
//
// Only the fusion weight and the tool's own name differ between the two; the
// validation bounds, the merge/dedupe, the graph-chunk filter, the outage
// reporting and the snippet/preview rendering must stay identical, so they live
// here once instead of being copied per tool. A copy would drift — and a
// drifted bound is a tool the model cannot reason about.
type locateSearchSpec struct {
	// tool is the tool name used in every message, log field and <tool_error>.
	tool string
	// queries are the model's 1-5 semantic queries (untrimmed, unvalidated).
	queries []string
	// datasetIDs is the model's optional scope request; boundDatasetIDs is the
	// conversation's server-bound scope it must stay inside.
	datasetIDs      []string
	boundDatasetIDs []string
	docScope        []string
	// topN is the per-query result count (<=0 means the tool default).
	topN int
	// similarityThreshold is the model's optional floor (nil means the default).
	similarityThreshold *float64
	// weight is the keyword leg's share of the fusion, already clamped to
	// [0,1] by the caller: 0 = pure vector, 1 = pure keyword.
	weight float64
	// vectorOnly asks the retrieval service for the dense leg ALONE (see
	// runtime.RetrievalRequest.VectorOnly). It is not the same as weight == 0:
	// weight 0 still sends the BM25 clause to the engine, which then uses it as
	// the kNN's filter, so results stay restricted to chunks that match the
	// query's words. vectorOnly removes the text expression entirely.
	vectorOnly bool
	tenantID   string
}

func runLocateSearch(ctx context.Context, spec locateSearchSpec) (string, error) {
	queries := make([]string, 0, len(spec.queries))
	for _, q := range spec.queries {
		if q = strings.TrimSpace(q); q != "" {
			queries = append(queries, q)
		}
	}
	if len(queries) == 0 {
		return "", fmt.Errorf("%s: queries must contain 1-5 non-empty semantic questions", spec.tool)
	}
	if len(queries) > 5 {
		return "", fmt.Errorf("%s: queries must contain at most 5 questions, got %d", spec.tool, len(queries))
	}

	// Enforce the declared input bounds before touching production retrieval: a
	// hostile or confused model could otherwise ask for top_n in the millions and
	// forward an unbounded TopK downstream.
	topN := spec.topN
	if topN <= 0 {
		topN = searchChunksDefaultTopN
	}
	if topN > 50 {
		topN = 50
	}
	similarityThreshold := searchChunksDefaultSimilarityThreshold
	if spec.similarityThreshold != nil {
		similarityThreshold = clampFloat01(*spec.similarityThreshold)
	}
	weight := clampFloat01(spec.weight)
	datasetIDs, err := resolveDatasetScope(spec.boundDatasetIDs, spec.datasetIDs)
	if err != nil {
		return "", fmt.Errorf("%s: %w", spec.tool, err)
	}
	if len(datasetIDs) > 10 {
		datasetIDs = datasetIDs[:10]
	}
	docScope := spec.docScope
	if len(docScope) > 10 {
		docScope = docScope[:10]
	}

	svc := runtime.GetRetrievalService()
	tenantID := spec.tenantID
	if svc == nil || tenantID == "" || len(datasetIDs) == 0 {
		return formatLocateResultsXML(ctx, spec.tool, strings.Join(queries, " | "), nil), nil
	}

	// Engine context with no per-query timeout (the run-level budget caps the
	// whole run): eino's streaming ReAct may hand the tool an already-canceled
	// context right after the model emits tool_calls; a canceled parent must
	// not kill the retrieval. Fall back to a fresh context in that case.
	ectx, ecancel := engineCallContext(ctx)
	defer ecancel()

	seen := map[string]struct{}{}
	var merged []runtime.RetrievalChunk
	var failed []error
	// Over-fetching candidates is a FUSION need: the weighted sum needs a pool
	// larger than the page to re-rank inside. A vector-only request returns
	// exactly what the kNN ranked, so pulling 4x and discarding three quarters
	// is pure latency — and the second-pass KNN scoring is skipped too.
	topK := topN * 4
	if spec.vectorOnly {
		topK = topN
	}
	for _, q := range queries {
		chunks, err := svc.Search(ectx, dao.DB, runtime.RetrievalRequest{
			Query:                    q,
			DatasetIDs:               datasetIDs,
			TopN:                     topN,
			TopK:                     topK,
			SimilarityThreshold:      &similarityThreshold,
			KeywordsSimilarityWeight: &weight,
			DocScope:                 docScope,
			TenantID:                 tenantID,
			VectorOnly:               spec.vectorOnly,
			SelectFields:             grepChunksSelectFields,
			// Same "only ordinary prose" filter as grep_chunks: exclude
			// compiled products so the model reads original document text, not
			// derived graph content.
			OnlyOriginalText: true,
		})
		if err != nil {
			// Record and fall back on failure, keep other queries' results.
			// (A query embedding failure lands here: the semantic bridge is
			// dead for that query, but the others may still return hits.)
			failed = append(failed, fmt.Errorf("%q: %w", q, err))
			continue
		}
		for _, c := range chunks {
			if _, dup := seen[c.ID]; dup {
				continue
			}
			seen[c.ID] = struct{}{}
			// Skip graph relation/entity/location chunks so the model reads
			// actual document prose rather than extracted graph triples
			// (which are sparse and miss events like "何进斩马元义").
			if isGraphChunkContent(c.Content) {
				continue
			}
			merged = append(merged, c)
		}
	}

	// Every query failed: the tool cannot claim "no results" — that would
	// silently silence the whole semantic bridge. Surfacing the reason is the
	// difference between "the corpus has nothing" (true empty) and "the
	// embedding backend is down" (an outage the caller must see). Observed on
	// browsecomp: a SiliconFlow balance drain zeroed 339 semantic searches
	// across two days before anyone noticed.
	if len(merged) == 0 && len(failed) == len(queries) {
		rootCause := failed[0].Error()
		// ERROR level: an outage must be visible in the operator's log — a
		// warn would hide it among per-question noise.
		// The message keeps the tool name inline (not only as a field) so an
		// operator grepping the log for a tool still finds these lines.
		common.ErrorCtx(ctx, "agentic_rag: "+spec.tool+" semantic retrieval unavailable - embedding/retrieval backend failed for every query",
			errors.Join(failed...),
			zap.String("tool", spec.tool),
			zap.Int("failed_queries", len(failed)),
			zap.String("root_cause", rootCause))
		// ...and in the TOOL RESULT: the model reads the result, not the
		// log, so the reason plus a hard "do not retry" instruction goes
		// here, in the canonical <tool_error> shape. Returning an error
		// instead would invite the agent to retry the same dead tool; a
		// result notice redirects it to the lexical tools, which still work
		// without embeddings.
		return toolErrorXML(spec.tool, "error",
			fmt.Sprintf("SEMANTIC RETRIEVAL UNAVAILABLE: all %d queries failed at the embedding/retrieval backend. Root cause: %s. "+
				"Do NOT call %s again while this notice is active - it will keep failing. "+
				"Use grep_chunks (regex over literal wording) or search_bm25_chunks (lexical keywords) instead, and deep-read (list_chunks) what they surface.",
				len(queries), rootCause, spec.tool),
			[2]string{"failed_queries", fmt.Sprintf("%d", len(failed))}), nil
	}
	if len(failed) > 0 {
		common.WarnCtx(ctx, "agentic_rag: "+spec.tool+" partial failure",
			zap.Int("failed_queries", len(failed)), zap.Error(failed[0]))
	}

	common.DebugCtx(ctx, "agentic_rag: "+spec.tool+" result",
		zap.String("tool", spec.tool),
		zap.Float64("keywords_similarity_weight", weight),
		zap.Int("chunks", len(merged)),
		zap.Strings("queries", queries),
	)
	// Snippet anchoring mirrors grep/bm25: term boundaries over the queries
	// (earliest hit − N .. latest hit + N, shared half-window). A semantic hit
	// whose text lacks any query term renders without a <match_snippet> —
	// consistent with the other locate tools.
	var terms []string
	for _, q := range queries {
		terms = append(terms, bm25TermTokens([]string{q})...)
	}
	hits := make([]snippetHit, 0, len(merged))
	for _, c := range merged {
		snippet, truncated, preview := "", false, false
		if first, last, ok := termMatchSpan(terms, c.Content); ok {
			snippet, truncated = snippetForMatches(c.Content, first, last)
		} else {
			// A semantic hit need not contain the query terms verbatim: ship the
			// chunk's opening and label it a preview instead of an empty snippet.
			snippet, truncated = previewForChunk(c.Content)
			preview = snippet != ""
		}
		hits = append(hits, snippetHit{chunk: c, snippet: snippet, truncated: truncated, preview: preview})
	}
	result := formatLocateResultsXML(ctx, spec.tool, strings.Join(queries, " | "), hits)
	// Partial failure rides on the result in the canonical shape
	// (severity="warn"): the surviving hits stay usable, but the model must
	// know those queries returned nothing because the backend errored — not
	// because the corpus is empty.
	if len(failed) > 0 {
		reason := failed[0].Error()
		if r := []rune(reason); len(r) > 200 {
			reason = string(r[:200]) + "…"
		}
		result += "\n" + toolErrorXML(spec.tool, "warn", reason,
			[2]string{"failed_queries", fmt.Sprintf("%d", len(failed))},
			[2]string{"total_queries", fmt.Sprintf("%d", len(queries))})
	}
	return result, nil
}
