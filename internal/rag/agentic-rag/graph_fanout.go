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
	"sort"
	"strings"
	"sync"

	"ragflow/internal/rag/agentic-rag/runtime"
)

// The opening DECOMPOSITION is gone from this file. It used to be a stage of its own: one chat
// call asking for 2-5 "fan-out" sub-questions, a shape guard that rejected lines looking like
// answers rather than queries (fanoutLooksLikeQuery), and a parser that split a prose reply into
// lines. Its output then became the slot table's `fanout_hint` — while the slot-table call asked
// the same model, on the same question, for the very queries that stage had just produced.
//
// Two consequences, both measured on 2026-09-20: the planner phase cost TWO model calls per
// question (the second-biggest output-token consumer in the run), and the shape guard is the same
// class of rule the rest of this design removes — a line is not a query because of how it looks.
//
// The paper's opening move is ONE call (`first_move`: decompose → the queries to run), and the
// slot table's own call already produces those queries (`first_queries`). See plannerNode.

// Phase 2: programmatic fan-out search.

// FanoutSearch: the
// programmatic multi-query prefetch that seeds the snippet pool.
//
// It is a DUAL-CHANNEL search, and the two channels are deliberately different:
//
//   - channel A (exact): a keyword BM25 round over a wide pool (top_n=60) whose
//     hits are then narrowed by the query's own terms — the precision path. A
//     chunk only survives if it literally contains a query term.
//   - channel B (semantic bypass): a dense hybrid round (top_n=30) that
//     deliberately SKIPS narrowing, so a paraphrase-only hit can still enter the
//     pool. These are quarantined to a small per-fan-out quota
//     (fanoutSemanticQuota) because bypassing narrowing is what makes them
//     low-precision.
//
// Collapsing them into one call loses the split, and with it the ability to
// admit a paraphrase-only match without letting it displace an exact one.
//
// Admission order is the point: every fan-out's channel A is admitted before any
// fan-out's channel B, so an exact hit from a later fan-out outranks a semantic
// hit from an earlier one.
//
// Returns the number of NEW snippets admitted to the pool.
// FanoutSearch runs the opening's per-clue retrieval and admits what it finds.
//
// It returns how many passages were NEW to the pool, and the RANKED UNION of the legs' results as
// chunk ids (see rankOpening): the second value is the opening's product — the ordered preview list
// the session is handed (o_1) — and it is why the legs' depth is not wasted. Returning only the
// count left the rank order to be re-derived, a second time, by whoever looked at the pool.
func FanoutSearch(ctx context.Context, deps RAGTools, st *AgenticState, queries []string, topN int) (int, []string) {
	if len(queries) == 0 || st.KB == nil {
		return 0, nil
	}
	sd := deps.Search
	if sd.Backend == nil {
		return 0, nil
	}

	// Dedup against what kbinfos ALREADY holds. There is no room to compute and no early
	// return when the pool looks "full": the pool has NO ceiling (see the note in
	// runtime/kbinfos.go), so a call is bounded only by its own per-call quotas
	// (rawSnippetQuota / evidencePoolQuota) and by topN — never by how much the pool already
	// holds. The ceiling computed here is what once refused the passage that would have
	// reached an enumeration's last members, and what made a rich round's own rewrite land
	// nowhere (the caller passed the snippet-pool number while the pool held up to twice it).
	seen := make(map[string]bool, len(st.KB.Chunks))
	for _, c := range st.KB.Chunks {
		seen[runtime.ChunkIDOf(c)] = true
	}

	// The planner's output is already []string, but blank entries still have to be dropped.
	qs := make([]string, 0, len(queries))
	for _, q := range queries {
		if q = strings.TrimSpace(q); q != "" {
			qs = append(qs, q)
		}
	}
	if len(qs) == 0 {
		return 0, nil
	}
	// topN bounds what the CLAIM channel recalls per query (below) and nothing else: the raw
	// channels' width is their own (fanoutBM25TopN / fanoutHybridTopN).
	capPerQuery := topN
	if capPerQuery < 1 {
		capPerQuery = 1
	}

	// The LEGS run in parallel: each one only RETRIEVES — kbinfos is mutated once, below, in a
	// single admit stretch — and everything the retrievals do share is guarded (the
	// per-request search cache holds its own mutex). A clue list is what the planner exists to
	// produce, and searching nine clues one after another is nine round trips; the paper
	// parallelises its per-clue retrieval for the same reason.
	//
	// What this makes explicit: the RETRIEVER contract is concurrent. Production retrievers are
	// HTTP/DB clients and already are (the multi-session rounds called them concurrently), so
	// the fixtures that stand in for them must be too — a stub that appends to a field needs a
	// lock, and the fan-out fixtures carry one.
	//
	// Results are written BY INDEX, so the admission order below is the query order whatever the
	// schedule does — that order is a contract: every fan-out's exact channel is admitted
	// before any fan-out's semantic channel.
	pairs := make([]fanoutPair, len(qs))
	var wg sync.WaitGroup
	for i, q := range qs {
		wg.Add(1)
		go func(i int, q string) {
			defer wg.Done()
			exact, semantic := fanoutSearchQuery(ctx, sd, q)
			pairs[i] = fanoutPair{exact: exact, semantic: semantic}
		}(i, q)
	}
	wg.Wait()

	added := 0
	rawAdded := 0
	admit := func(batch []map[string]any) {
		for _, c := range batch {
			id := runtime.ChunkIDOf(c)
			isEvidence := strings.HasPrefix(id, "claim_")
			if id != "" {
				if seen[id] {
					continue
				}
				seen[id] = true
			}
			// Raw passages get their own budget;
			// anything left above it stays free for evidence rows, which are
			// far denser answer material. Evidence rows (claim_ prefix) bypass
			// the quota and never count against it.
			if !isEvidence && rawAdded >= rawSnippetQuota {
				continue
			}
			st.KB.Chunks = append(st.KB.Chunks, c)
			added++
			if !isEvidence {
				rawAdded++
			}
		}
	}
	// Channel 0: claim/evidence rows lead the pool —
	// they are the compact, verbatim-bearing proxy for the chunks they source.
	// Gated on the dataset having compiled rows at all; capped at
	// evidencePoolQuota rows across the whole prefetch; best effort.
	channel0 := [][]map[string]any{}
	channel0Rows := 0
	for _, q := range qs {
		select {
		case <-ctx.Done():
			return 0, nil
		default:
		}
		if !runtime.DatasetHasCompilation(ctx, sd) {
			break
		}
		// top_n=max(2, top_n) per query, so a single-fanout
		// question still recalls at least two claim rows.
		recallN := capPerQuery
		if recallN < 2 {
			recallN = 2
		}
		hits := runtime.RecallDatasetClaims(ctx, sd, q, recallN)
		if len(hits) == 0 {
			continue
		}
		pseudo := runtime.ClaimPseudoChunks(hits)
		kept := make([]map[string]any, 0, len(pseudo))
		for _, pc := range pseudo {
			if channel0Rows >= evidencePoolQuota {
				break
			}
			kept = append(kept, pc)
			channel0Rows++
		}
		if len(kept) == 0 {
			break
		}
		channel0 = append(channel0, kept)
		admit(kept)
	}
	// Evidence top-up: an evidence row carries a
	// verbatim quote but not its surrounding passage, so pull exactly the chunks
	// it cites instead of running another global recall. Deduped against the
	// pool (a chunk the claim already quotes verbatim adds nothing new) and
	// capped at evidenceTopUp ids. Best effort.
	if len(channel0) > 0 {
		var wanted []string
		inWanted := map[string]bool{}
		for _, pseudo := range channel0 {
			for _, c := range pseudo {
				ids, _ := c["source_chunk_ids"].([]string)
				for _, cid := range ids {
					cid = strings.TrimSpace(cid)
					if cid == "" || seen[cid] || inWanted[cid] {
						continue
					}
					inWanted[cid] = true
					wanted = append(wanted, cid)
				}
			}
		}
		if len(wanted) > evidenceTopUp {
			wanted = wanted[:evidenceTopUp]
		}
		if len(wanted) > 0 {
			fetched := runtime.LoadChunksForIDs(ctx, sd, wanted)
			if len(fetched) > 0 {
				admit(fetched)
			}
		}
	}
	// The RANKED UNION, computed BEFORE admission order matters (the pool keeps insertion order,
	// which is a contract for the citation list; the ranking is the opening's product and is
	// independent of it). Claims lead because they are verbatim-bearing and compact — the same
	// reason channel 0 admits them first.
	ranking := rankOpening(qs, pairs, channel0)

	// Channel A across every fan-out first, then channel B.
	for _, p := range pairs {
		admit(p.exact)
	}
	for _, p := range pairs {
		admit(p.semantic)
	}
	return added, ranking
}

// fanoutPair is one clue's two channels, written BY INDEX by the parallel legs (see FanoutSearch).
type fanoutPair struct {
	exact    []map[string]any
	semantic []map[string]any
}

// rankOpening fuses the opening's channels into ONE ranked list of chunk ids: reciprocal rank
// fusion over (clue × channel), claims first.
//
// RRF, not a score comparison, because the legs' scores are not comparable — a BM25 score, a vector
// similarity and a claim's fused rank live on different scales, and the codebase already fuses by
// reciprocal rank elsewhere (see the claim rows' q_<dim>_vec). The exact (keyword) channel is
// weighted above the semantic one: it is the deliberate surface probe, and its hits carry the
// corpus's own wording.
//
// This is the opening's o_1: the order the session is handed, and the order that decides which
// passages it reads first when its clock only affords a couple of calls.
func rankOpening(queries []string, pairs []fanoutPair, claims [][]map[string]any) []string {
	scores := make(map[string]float64)
	order := make([]string, 0, 64)
	add := func(list []map[string]any, weight float64) {
		for rank, c := range list {
			id := runtime.ChunkIDOf(c)
			if id == "" {
				continue
			}
			if _, seen := scores[id]; !seen {
				order = append(order, id)
			}
			scores[id] += weight / (openingRrfK + float64(rank+1))
		}
	}
	for _, batch := range claims {
		add(batch, claimChannelWeight)
	}
	for _, p := range pairs {
		add(p.exact, exactChannelWeight)
	}
	for _, p := range pairs {
		add(p.semantic, semanticChannelWeight)
	}
	// Stable sort by fused score, ties keeping first-seen order (clue order, then channel, then the
	// leg's own ranking) so the delivery is reproducible.
	sort.SliceStable(order, func(i, j int) bool { return scores[order[i]] > scores[order[j]] })
	return order
}

// The fusion weights: the keyword channel is the deliberate probe and outranks the semantic
// complement; a claim row is a verbatim, compact statement of the same text and leads both.
const (
	claimChannelWeight    = 3.0
	exactChannelWeight    = 2.0
	semanticChannelWeight = 1.0
)

// fanoutSearchQuery runs one fan-out's two channels. It only retrieves; the
// caller admits the results.
func fanoutSearchQuery(ctx context.Context, sd runtime.SearchDeps, fq string) (exact, semantic []map[string]any) {
	terms := runtime.QueryToTerms(fq)
	keyed := runtime.FanoutKeyedTerms(terms)
	termList := keyed
	if len(termList) == 0 {
		termList = terms
	}

	// Channel A — exact: BM25Search is the dedicated keyword-only entry point (vector weight
	// 0).
	candidates, _ := runtime.BM25Search(ctx, sd, runtime.SearchParams{
		Question: fq,
		Keywords: strings.Join(termList, " "),
		TopN:     fanoutBM25TopN,
	})
	if len(candidates) > 0 {
		res := runtime.NarrowByTerms(candidates, termList, nil, fq,
			runtime.NarrowContext{Before: 0, After: 1},
			fanoutNarrowMaxOutPerChunk, fanoutNarrowMaxOutTotal)
		exact = res.Kept
		// NO per-query cut here any more. `exact[:capPerQuery]` was the opening's own ceiling: the
		// legs RECALLED wide and the caller then kept the first eight passages of each clue, so the
		// union that got ranked was three clues wide and eight deep — measured 2026-09-20 (三国),
		// the candidates the session could ever reach, before it searched for anything.
	}

	// Channel B — semantic bypass: HybridSearch gives the vector leg weight 0.3 whenever an
	// embedder is configured. NO narrowing (a paraphrase-only hit has no query term to match, so
	// narrowing would erase it).
	hits, _ := runtime.HybridSearch(ctx, sd, runtime.SearchParams{
		Question: fq,
		TopN:     fanoutHybridTopN,
	})
	exactIDs := make(map[string]bool, len(exact))
	for _, c := range exact {
		exactIDs[runtime.ChunkIDOf(c)] = true
	}
	for _, c := range hits {
		if exactIDs[runtime.ChunkIDOf(c)] {
			continue
		}
		semantic = append(semantic, c)
		if len(semantic) >= fanoutSemanticQuota {
			break
		}
	}
	return exact, semantic
}
