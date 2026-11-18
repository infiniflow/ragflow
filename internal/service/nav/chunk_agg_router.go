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

// The chunk_agg navigation-tree router, the Go mirror of Python
// dataset_api_service.py `_search_layers_nav_chunk_agg` under the default
// _NAV_TREE_ROUTER = "chunk_agg". Where the nav_doc router descends compiled
// nav rows, chunk_agg routes documents by aggregating raw-chunk retrieval hits:
// every document that surfaces a relevant chunk is scored with a PageIndex
// DocScore and returned with its nav_doc summary.
//
// It lives in the nav service package (not the agentic harness) because the
// Python source of this logic is dataset_api_service.py, not harness/. The
// harness tool layer only consumes it through the agentic NavTreeRouter
// interface and supplies the retriever/summariser via dependency injection.

package nav

import (
	"context"
	"math"
	"sort"
	"strings"
)

const (
	// chunkAggDocFocus is _NAV_DOC_FOCUS_LIMIT: how many documents chunk_agg returns.
	chunkAggDocFocus = 3
	// chunkAggPool is _NAV_CHUNK_AGG_POOL: how many raw chunks to retrieve before
	// rolling them up per document.
	chunkAggPool = 256
	// chunkAggVecWeight is _NAV_CHUNK_AGG_VEC_WEIGHT: hybrid balance when no
	// explicit dense weight is supplied.
	chunkAggVecWeight = 0.3
)

// chunkAggregate is the per-document roll-up of chunk hits.
type chunkAggregate struct {
	DocID string
	Total float64
	Best  float64
	Hits  int
}

// docScore mirrors _nav_doc_score: hit scores summed, damped by hit count. The
// sum rewards a document matching several chunks; the square-root denominator
// damps it so a long document cannot win on volume alone.
func docScore(total float64, hits int) float64 {
	return total / math.Sqrt(float64(hits)+1)
}

// aggregateChunks mirrors _nav_aggregate_chunks: roll chunk hits up per document.
// Only map chunks carrying a doc_id participate.
func aggregateChunks(chunks []map[string]any) map[string]*chunkAggregate {
	agg := map[string]*chunkAggregate{}
	for _, c := range chunks {
		if c == nil {
			continue
		}
		docID := ""
		if id, ok := c["doc_id"].(string); ok {
			docID = strings.TrimSpace(id)
		}
		if docID == "" {
			continue
		}
		score := 0.0
		if s, ok := c["similarity"].(float64); ok {
			score = s
		} else if s, ok := c["score"].(float64); ok {
			score = s
		}
		e, ok := agg[docID]
		if !ok {
			agg[docID] = &chunkAggregate{DocID: docID, Total: score, Best: score, Hits: 1}
			continue
		}
		e.Total += score
		e.Hits++
		if score > e.Best {
			e.Best = score
		}
	}
	return agg
}

// rankedAggregates sorts per-doc aggregates by descending docScore (best chunk
// similarity as tie-break) and returns them ordered, mirroring the chunk_agg
// sort keyed on _nav_doc_score.
func rankedAggregates(agg map[string]*chunkAggregate) []*chunkAggregate {
	out := make([]*chunkAggregate, 0, len(agg))
	for _, e := range agg {
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool {
		si := docScore(out[i].Total, out[i].Hits)
		sj := docScore(out[j].Total, out[j].Hits)
		if si != sj {
			return si > sj
		}
		return out[i].Best > out[j].Best
	})
	return out
}

// ChunkRetriever retrieves raw chunks for chunk_agg. It is injected so the
// agentic harness stays free of the nlp/service layer; production wiring
// supplies a hybrid retriever that excludes compiled rows (compile_kwd) — see
// the navigation_tree retrieval contract.
//
// The error is part of the contract: a retrieval FAILURE must not be reported as
// an empty route. Python's _search_layers_nav_chunk_agg returns
// ok=False/SERVER_ERROR on a retrieval exception and reserves ok=True/total=0
// for "the pool aggregated to nothing", and the orchestrator picks its fallback
// from exactly that distinction.
type ChunkRetriever func(ctx context.Context, tenantID, kbID, query string, docScope []string, topN int, vecWeight float64) ([]map[string]any, error)

// DocSummarizer loads the nav_doc "description" (the document's overall summary)
// for a set of doc_ids, mirroring Python _nav_doc_summaries. Injected for the
// same reason as ChunkRetriever.
type DocSummarizer func(ctx context.Context, tenantID, kbID string, docIDs []string) map[string]string

// ChunkAggRouter is a NavTreeRouter that routes documents by aggregating
// raw-chunk retrieval hits (Python default _NAV_TREE_ROUTER="chunk_agg"). It
// returns the top documents by PageIndex docScore, each labelled with its
// nav_doc summary.
type ChunkAggRouter struct {
	Retrieve  ChunkRetriever
	Summarize DocSummarizer
}

// Route implements the agentic NavTreeRouter contract. It returns (nil, nil)
// when no retriever is installed (treat as "no compiled tree", mirroring
// NavServiceRouter's contract), an empty non-nil slice when a structure exists
// but no chunk routed to a doc, and a non-nil error when retrieval itself failed
// — mirroring NavServiceRouter, which propagates a backend failure instead of
// reporting it as an empty route.
func (r *ChunkAggRouter) Route(ctx context.Context, tenantID, kbID, query string, docScope []string, topK int) ([][2]string, error) {
	if r.Retrieve == nil {
		return nil, nil
	}
	query = strings.TrimSpace(query)
	if query == "" || kbID == "" {
		return nil, nil
	}
	if topK <= 0 {
		topK = chunkAggDocFocus
	}
	// Pull a wide pool, then roll up per document (a focused doc may surface on
	// many chunks, so a small pool starves the aggregation).
	chunks, err := r.Retrieve(ctx, tenantID, kbID, query, docScope, chunkAggPool, chunkAggVecWeight)
	if err != nil {
		// A retrieval failure is NOT "no document routed": the caller must be
		// able to take its fallback instead of re-phrasing the query.
		return nil, err
	}
	agg := aggregateChunks(chunks)
	if len(agg) == 0 {
		return make([][2]string, 0), nil // structure exists, nothing routed
	}
	ranked := rankedAggregates(agg)
	if len(ranked) > topK {
		ranked = ranked[:topK]
	}
	ids := make([]string, 0, len(ranked))
	for _, e := range ranked {
		ids = append(ids, e.DocID)
	}
	summaries := map[string]string{}
	if r.Summarize != nil {
		summaries = r.Summarize(ctx, tenantID, kbID, ids)
	}
	routed := make([][2]string, 0, len(ranked))
	for _, e := range ranked {
		routed = append(routed, [2]string{e.DocID, strings.TrimSpace(summaries[e.DocID])})
	}
	return routed, nil
}

// NewChunkAggRouter returns a ChunkAggRouter wired to retrieve and summarise.
func NewChunkAggRouter(retrieve ChunkRetriever, summarize DocSummarizer) *ChunkAggRouter {
	return &ChunkAggRouter{Retrieve: retrieve, Summarize: summarize}
}

// ChunkAggSummarize adapts the nav service into the chunk_agg summarize leg,
// reading each routed document's nav_doc summary (Python _nav_doc_summaries).
func ChunkAggSummarize() DocSummarizer {
	return func(ctx context.Context, tenantID, kbID string, docIDs []string) map[string]string {
		ns := GetNavService()
		if ns == nil {
			return map[string]string{}
		}
		return ns.SummariesByDocIDs(ctx, tenantID, kbID, docIDs)
	}
}
