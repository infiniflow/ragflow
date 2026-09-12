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

package harness

import (
	"context"
	"math"
	"sort"
	"strings"

	"ragflow/internal/service/nav"
)

// The claim_agg navigation-tree router: the Go mirror of Python
// dataset_api_service.py `_search_layers_navigation_tree(router="claim_agg")`.
//
// The claim leg runs first and decides the ranking outright when it hits — a
// claim is an atomic proposition carrying its own vector and verbatim
// evidence, so matching one means the document asserts that fact. No claim
// rows, or an empty claim leg, falls back to the chunk leg (PageIndex style:
// hybrid chunk recall rolled up per document).
//
// Like chunk_agg (internal/service/nav), the algorithm consumes the agentic
// claim-recall capability through the NavTreeRouter interface and keeps the
// summarizer injected, so the harness stays free of the service layer.

// claimAggPool mirrors Python _NAV_CLAIM_POOL: the per-leg store limit for the
// claim routing leg, wide enough for a document to collect several claim hits
// before the per-document roll-up (the aggregation rewards multi-hit docs).
const claimAggPool = 256

// claimDocScore mirrors _nav_doc_score (PageIndex): hit scores summed, damped
// by hit count. The sum rewards a document matching several claims; the
// square-root denominator damps it so a document cannot win on volume alone.
func claimDocScore(total float64, hits int) float64 {
	return total / math.Sqrt(float64(hits)+1)
}

// ClaimAggRouter is a NavTreeRouter that routes documents through the
// compilers' claim rows first, falling back to the chunk_agg router when the
// claim leg comes back empty (a raptor-less or pre-claim dataset is a
// legitimate empty leg, not a failure).
type ClaimAggRouter struct {
	// Deps carries the claim-recall wiring (doc engine, embedder, index).
	// Route overrides only the per-call tenant/dataset.
	Deps SearchDeps
	// Fallback is the chunk_agg router used when the claim leg is empty.
	Fallback NavTreeRouter
	// Summarize loads the nav_doc summary per routed document (Python
	// _nav_doc_summaries). Nil leaves the summaries empty.
	Summarize nav.DocSummarizer
}

// claimAggBucket is the per-document roll-up of claim hits (Python
// _nav_bucket_compiled_rows): total, best and hit count per doc.
type claimAggBucket struct {
	docID string
	total float64
	best  float64
	hits  int
}

// Route implements the agentic NavTreeRouter contract with the claim-first
// strategy. It returns (nil, nil) when the fallback router reports no compiled
// tree, an empty non-nil slice when a structure exists but nothing routed, and
// a non-nil error when the fallback's retrieval itself failed — the claim leg
// never errors (recall failures read as an empty leg, which is exactly the
// fallback trigger).
func (r *ClaimAggRouter) Route(ctx context.Context, tenantID, kbID, query string, docScope []string, topK int) ([][2]string, error) {
	fallback := func() ([][2]string, error) {
		if r.Fallback == nil {
			return nil, nil
		}
		return r.Fallback.Route(ctx, tenantID, kbID, query, docScope, topK)
	}
	deps := r.Deps
	deps.TenantID = tenantID
	deps.KbIDs = []string{kbID}
	// Python queries the two compilers SEPARATELY, never as one mixed
	// condition (:4230-4239): tree first — it is the compiled benchmark path —
	// then page_index only when the tree pass came back empty. Each pass pins
	// compile_kwd and row_types=("claim",).
	claims := recallDatasetClaimsFiltered(ctx, deps, query, claimAggPool, []string{"tree"}, []string{"claim"})
	if len(claims) == 0 {
		claims = recallDatasetClaimsFiltered(ctx, deps, query, claimAggPool, []string{"page_index", "pageindex"}, []string{"claim"})
	}
	if len(claims) == 0 {
		// No claim rows (or the recall failed): a legitimate empty leg — the
		// chunk leg decides (Python _search_layers_navigation_tree:4053-4057).
		return fallback()
	}

	// Roll the claim hits up per document (Python _nav_bucket_compiled_rows):
	// the doc score lives on each claim's similarity.
	agg := map[string]*claimAggBucket{}
	for _, c := range claims {
		did := strings.TrimSpace(c.DocID)
		if did == "" {
			continue
		}
		b, ok := agg[did]
		if !ok {
			agg[did] = &claimAggBucket{docID: did, total: c.Score, best: c.Score, hits: 1}
			continue
		}
		b.total += c.Score
		b.hits++
		if c.Score > b.best {
			b.best = c.Score
		}
	}

	// doc_scope: Python filters the store condition before the recall
	// (:4186-4187); post-filtering the rolled-up buckets is equivalent here
	// because the recall is threshold-free.
	scope := make(map[string]bool, len(docScope))
	for _, d := range docScope {
		if d = strings.TrimSpace(d); d != "" {
			scope[d] = true
		}
	}
	ranked := make([]*claimAggBucket, 0, len(agg))
	for _, b := range agg {
		if len(scope) > 0 && !scope[b.docID] {
			continue
		}
		ranked = append(ranked, b)
	}
	if len(ranked) == 0 {
		// A scoped claim leg that reaches nothing is still an empty leg — the
		// chunk leg (which applies the same scope) may reach further.
		return fallback()
	}
	// Order by the damped DocScore (Python _nav_rank_compiled_buckets), best
	// similarity as the tie-break.
	sort.SliceStable(ranked, func(i, j int) bool {
		si := claimDocScore(ranked[i].total, ranked[i].hits)
		sj := claimDocScore(ranked[j].total, ranked[j].hits)
		if si != sj {
			return si > sj
		}
		return ranked[i].best > ranked[j].best
	})
	if topK > 0 && len(ranked) > topK {
		ranked = ranked[:topK]
	}
	// Same focus cap as the sibling routers (Python _NAV_DOC_FOCUS_LIMIT = 3):
	// routing to many documents makes the agent carry that many times the
	// evidence in every later round.
	const navDocFocusLimit = 3
	if navDocFocusLimit > 0 && len(ranked) > navDocFocusLimit {
		ranked = ranked[:navDocFocusLimit]
	}

	ids := make([]string, 0, len(ranked))
	for _, b := range ranked {
		ids = append(ids, b.docID)
	}
	summaries := map[string]string{}
	if r.Summarize != nil {
		summaries = r.Summarize(ctx, tenantID, kbID, ids)
	}
	routed := make([][2]string, 0, len(ranked))
	for _, b := range ranked {
		routed = append(routed, [2]string{b.docID, strings.TrimSpace(summaries[b.docID])})
	}
	return routed, nil
}
