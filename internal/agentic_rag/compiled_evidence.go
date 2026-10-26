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

// Compiled-evidence-first retrieval.
//
// A compiled claim states one atomic proposition plus the verbatim quote it was
// compiled from, and it names the chunk that quote came from. For a model that
// only needs the proposition, reading the claim costs a fraction of reading the
// chunk — and it carries its own evidence, so the answer can cite the source
// chunk without the chunk ever entering the context.
//
// The leg is ANCHORED ON THE SEARCH, not on the question: the ordinary hybrid
// chunk retrieval runs first, and the claims looked up afterwards are exactly
// the ones compiled FROM the chunks that search returned. Recalling claims by
// their own text (the previous design) made the compiled leg a second,
// independent ranking that could land on a different document than the raw
// retrieval did; anchoring it to the chunks means the compiled evidence can
// never be about a document the retrieval rejected.
//
// Two consumers, one rule ("compiled first, raw chunk only when it is not
// enough"):
//
//   - the pre-loop probe: one search plus one verdict call. When the model says
//     the claims answer the question, the turn ends without reading a chunk;
//     when it names the fact that is absent, the probe searches THAT fact and
//     asks again, because a multi-hop question's second fact lives in a document
//     the question's own wording never reaches.
//   - every locate tool result: the claims compiled from THAT result set's
//     chunks are rendered ahead of the hits, so a mid-loop search can be
//     answered without list_chunks. Claims the run has already shown are not
//     repeated.
//
// A corpus without compiled claims pays one existence probe and nothing else.
//
// Coverage, measured on the FRAMES corpus: of the 68,677 chunks the retrieval
// can actually return (available_int=1), 51,222 (75%) are referenced by at least
// one claim, and the compiled rows carry 4-36 claims per chunk. So most hits can
// be answered from their compiled artifacts, but not all: a run whose
// covered_chunks/chunks ratio is low is looking at uncompiled chunks, and the raw
// chunk is the only evidence there is.

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	enginetypes "ragflow/internal/engine/types"
)

const (
	// compiledAnchorTopN is the chunk page the pre-loop probe anchors on. It is
	// the retrieval's own top-N, not a claim count: the claims come from
	// whatever these chunks hold.
	compiledAnchorTopN = 10
	// compiledAnchorMaxChunks bounds the chunk ids in one claim lookup (a terms
	// filter, so the bound is about request size, not correctness).
	compiledAnchorMaxChunks = 32
	// compiledPrefetchClaims is the claim budget of the pre-loop probe. The
	// block is the whole prompt of the verdict call, so it is bounded.
	compiledPrefetchClaims = 24
	// compiledNoticeClaims is the claim budget attached to ONE locate result.
	//
	// It is the lever that keeps raw chunks out of the context: a locate result
	// averages 22.5 hits, and every hit whose statement is NOT in the block is a
	// chunk the model has to deep-read to see what it says. At the old budget of
	// six (spent two-deep on the top three anchors) the block covered 2.8 of those
	// 22.5 hits; at 12 with one claim per hit it covers 12, for ~3.4k characters -
	// about one chunk of prose, deduplicated per turn, and only attached when the
	// statements are new. The budget is therefore bounded by what a read costs,
	// not by what a notice used to cost.
	compiledNoticeClaims = 12
	// compiledNoticeShownChunks caps the pointer line that names hits whose
	// statements this turn already showed. Eight is where the line stays under
	// ~300 characters: it pays for itself if it averts a single deep read, and it
	// stops paying once it grows into a second block of text.
	compiledNoticeShownChunks = 8
	// compiledClaimsPerAnchor is the candidate budget per anchored chunk. The
	// lookup is filter-only, so the store returns rows in ITS order, not the
	// retrieval's; a pool smaller than anchors x claims-per-chunk lets the chunks
	// the store happened to list first crowd out the best-ranked anchor entirely.
	// Measured on the FRAMES corpus: 10 anchors carried 138 claim rows, up to 36
	// on a single chunk.
	compiledClaimsPerAnchor = 8
	// compiledGateClaims is the claim budget of the gate that stands in front of
	// a raw read. Sizing it like a read is the point: the block replaces whole
	// chunk text, and 12 claims measured ~3.4k characters, i.e. about one chunk
	// of prose - so the gate can never cost more than the read it may avert.
	compiledGateClaims = 12
	// compiledAnchorDepth is how many claims ONE anchor may contribute before
	// the budget moves to the next anchor. Two: the first states the fact the
	// retrieval ranked the chunk for, the second is where a second-hop fact sits
	// (the traced case: "Robinson made his MLB debut on April 15, 1947" was the
	// second best-covering claim of anchor #5). A third claim per anchor only
	// buys restatements of the first.
	compiledAnchorDepth = 2
	// compiledAnchorMaxDocs bounds the documents in one doc-scoped claim lookup
	// (a terms filter, so the bound is about request size, not correctness).
	compiledAnchorMaxDocs = 24
	// compiledCandidateMax bounds the candidate pool read in one lookup.
	compiledCandidateMax = 400
)

// compiledRun is the per-turn compiled-evidence state: the scope claims are
// looked up in, whether the corpus has any, and what this turn already showed.
//
// It is per-run rather than per-tool because the same claim is commonly the
// compiled artifact of several retrieved chunks, and re-rendering it in a later
// search result pays for text the model already holds.
type compiledRun struct {
	tenantID   string
	datasetIDs []string

	mu      sync.Mutex
	probed  bool
	present bool
	shown   map[string]struct{}
}

type compiledRunKey struct{}

// withCompiledRun publishes the turn's compiled-evidence state. It is installed
// once per run by Run so the pre-loop probe and every locate tool share one
// scope and one "already shown" set.
func withCompiledRun(ctx context.Context, tenantID string, datasetIDs []string) context.Context {
	if len(datasetIDs) == 0 {
		return ctx
	}
	return context.WithValue(ctx, compiledRunKey{}, &compiledRun{
		tenantID:   tenantID,
		datasetIDs: append([]string(nil), datasetIDs...),
		shown:      map[string]struct{}{},
	})
}

func compiledRunFrom(ctx context.Context) *compiledRun {
	r, _ := ctx.Value(compiledRunKey{}).(*compiledRun)
	return r
}

// claimsPresent answers the corpus-level gate once per run. The probe is a
// single existence read, but a run issues several searches, and each one asking
// the same question would be pure latency.
func (r *compiledRun) claimsPresent(ctx context.Context) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	if r.probed {
		present := r.present
		r.mu.Unlock()
		return present
	}
	tenant, datasets := r.tenantID, r.datasetIDs
	r.mu.Unlock()

	present := claimRowsPresent(ctx, tenant, datasets)

	r.mu.Lock()
	r.probed = true
	r.present = present
	r.mu.Unlock()
	return present
}

// takeUnseen keeps the leading claims this run has not shown yet, up to limit,
// and records them as shown. A claim already served keeps its ref handle in the
// transcript, so repeating its text adds nothing the model does not have.
func (r *compiledRun) takeUnseen(claims []claimHit, limit int) []claimHit {
	out := make([]claimHit, 0, limit)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.shown == nil {
		r.shown = map[string]struct{}{}
	}
	for _, c := range claims {
		key := claimKey(c)
		if key == "" {
			continue
		}
		if _, dup := r.shown[key]; dup {
			continue
		}
		r.shown[key] = struct{}{}
		out = append(out, c)
		if len(out) >= limit {
			break
		}
	}
	return out
}

// claimShown reports whether this turn already put the claim's statement in
// front of the model. It reads the same set takeUnseen writes.
func (r *compiledRun) claimShown(c claimHit) bool {
	if r == nil {
		return false
	}
	key := claimKey(c)
	if key == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.shown[key]
	return ok
}

// compiledAnchorSearch is the chunk retrieval the compiled leg hangs off: the
// same hybrid retriever the search_chunks tool uses, over original document
// text only. It is a variable so a test can drive the whole strategy without a
// retrieval backend.
var compiledAnchorSearch = searchOriginalChunksForQuestion

// searchOriginalChunksForQuestion runs ONE hybrid query over the corpus's
// ordinary prose chunks. Compiled products are excluded for the same reason the
// locate tools exclude them: the anchors are the documents, and a claim row
// that matched the question would make the compiled leg anchor on itself.
func searchOriginalChunksForQuestion(ctx context.Context, tenantID string, datasetIDs, docScope []string, question string, topN int) []runtime.RetrievalChunk {
	question = strings.TrimSpace(question)
	if question == "" || tenantID == "" || len(datasetIDs) == 0 {
		return nil
	}
	svc := runtime.GetRetrievalService()
	if svc == nil {
		return nil
	}
	if topN <= 0 {
		topN = compiledAnchorTopN
	}
	if len(datasetIDs) > 10 {
		datasetIDs = datasetIDs[:10]
	}
	// Engine context with no per-query timeout: the run-level budget caps the
	// turn (see runLocateSearch for the same rationale).
	ectx, ecancel := engineCallContext(ctx)
	defer ecancel()

	threshold := searchChunksDefaultSimilarityThreshold
	weight := searchChunksDefaultKeywordsSimilarityWeight
	chunks, err := svc.Search(ectx, dao.DB, runtime.RetrievalRequest{
		Query:                    question,
		DatasetIDs:               datasetIDs,
		TopN:                     topN,
		TopK:                     topN * 4,
		SimilarityThreshold:      &threshold,
		KeywordsSimilarityWeight: &weight,
		DocScope:                 docScope,
		TenantID:                 tenantID,
		AllowDenseFallback:       new(false),
		SelectFields:             grepChunksSelectFields,
		OnlyOriginalText:         true,
	})
	if err != nil {
		common.DebugCtx(ctx, "agentic_rag: compiled evidence anchor search failed",
			zap.Strings("datasets", datasetIDs), zap.Error(err))
		return nil
	}
	out := make([]runtime.RetrievalChunk, 0, len(chunks))
	for _, c := range chunks {
		if c.ID == "" || isGraphChunkContent(c.Content) {
			continue
		}
		out = append(out, c)
	}
	return out
}

// compiledClaimsByChunk reads the claim rows compiled FROM the given chunks. A
// variable so a test can supply claims without an engine.
var compiledClaimsByChunk = readCompiledClaimsByChunk

// compiledClaimsByDoc reads the claim rows of the given DOCUMENTS, whatever chunk
// they were compiled from. A variable so a test can supply claims without an
// engine.
//
// It exists because a document the retrieval accepted is evidence as a whole: a
// claim compiled from a later chunk of the same document states the fact the
// anchored chunk only introduces, and a chunk-anchored lookup cannot reach it.
// Measured on 24 short-circuit failures: in 38% of them the answer's document WAS
// on the page and its claims DID state the answer entity, but the row lived on a
// chunk the retrieval had not anchored on.
var compiledClaimsByDoc = readCompiledClaimsByDoc

func readCompiledClaimsByDoc(ctx context.Context, tenantID string, datasetIDs, docIDs []string, text string, docScope []string, limit int) []claimHit {
	if len(docIDs) == 0 || len(datasetIDs) == 0 {
		return nil
	}
	de := engine.Get()
	if de == nil {
		return nil
	}
	ids := docIDs
	if len(ids) > compiledAnchorMaxDocs {
		ids = ids[:compiledAnchorMaxDocs]
	}
	// The turn's document scope wins: a doc the turn may not read must not leak
	// its claims through this leg either.
	if len(docScope) > 0 {
		allowed := make(map[string]struct{}, len(docScope))
		for _, id := range docScope {
			allowed[id] = struct{}{}
		}
		scoped := make([]string, 0, len(ids))
		for _, id := range ids {
			if _, ok := allowed[id]; ok {
				scoped = append(scoped, id)
			}
		}
		if len(scoped) == 0 {
			return nil
		}
		ids = scoped
	}
	filter := map[string]interface{}{
		"entity_type_kwd": []string{navClaimRowType},
		"scope_kwd":       []string{navClaimDocScope},
		"doc_id":          ids,
	}
	// Ranked by the need text, not read in store order: these documents carry
	// hundreds of claims each (measured: 206-405 per FRAMES document), and a
	// filter-only read of a few hundred rows would return whichever rows the store
	// listed first. The lexical leg is the ranking signal; the doc filter is what
	// keeps it inside documents the retrieval already accepted.
	var matchExprs []interface{}
	if need := strings.TrimSpace(text); need != "" {
		matchExprs = append(matchExprs, &enginetypes.MatchTextExpr{
			Fields:       []string{"content_ltks", "content_sm_ltks"},
			MatchingText: need,
			TopN:         limit,
		})
	}
	res, err := de.Search(ctx, &enginetypes.SearchRequest{
		IndexNames: []string{indexNameFor(tenantID, "")},
		KbIDs:      datasetIDs,
		SelectFields: []string{
			"content_with_weight", "source_chunk_ids", "entity_type_kwd", "compile_kwd", "doc_id",
		},
		Filter:             filter,
		MatchExprs:         matchExprs,
		Limit:              limit,
		IncludeUnavailable: true,
	})
	if err != nil || res == nil {
		common.DebugCtx(ctx, "agentic_rag: doc-scoped claim lookup failed",
			zap.Strings("datasets", datasetIDs), zap.Error(err))
		return nil
	}
	out := make([]claimHit, 0, len(res.Chunks))
	for _, row := range res.Chunks {
		if h, ok := parseClaimHit(row); ok {
			out = append(out, h)
		}
	}
	return out
}

func readCompiledClaimsByChunk(ctx context.Context, tenantID string, datasetIDs, chunkIDs, docScope []string, limit int) []claimHit {
	if len(chunkIDs) == 0 || len(datasetIDs) == 0 {
		return nil
	}
	de := engine.Get()
	if de == nil {
		return nil
	}
	ids := chunkIDs
	if len(ids) > compiledAnchorMaxChunks {
		ids = ids[:compiledAnchorMaxChunks]
	}
	filter := map[string]interface{}{
		"entity_type_kwd":  []string{navClaimRowType},
		"scope_kwd":        []string{navClaimDocScope},
		"source_chunk_ids": ids,
	}
	if len(docScope) > 0 {
		filter["doc_id"] = docScope
	}
	res, err := de.Search(ctx, &enginetypes.SearchRequest{
		IndexNames: []string{indexNameFor(tenantID, "")},
		KbIDs:      datasetIDs,
		SelectFields: []string{
			"content_with_weight", "source_chunk_ids", "entity_type_kwd", "compile_kwd", "doc_id",
		},
		Filter:             filter,
		Limit:              limit,
		IncludeUnavailable: true,
	})
	if err != nil || res == nil {
		common.DebugCtx(ctx, "agentic_rag: compiled claim lookup failed",
			zap.Strings("datasets", datasetIDs), zap.Error(err))
		return nil
	}
	out := make([]claimHit, 0, len(res.Chunks))
	for _, row := range res.Chunks {
		if h, ok := parseClaimHit(row); ok {
			out = append(out, h)
		}
	}
	return out
}

// compiledClaimsForChunks ranks the claims of a retrieved chunk page.
//
// Two sources, one page. Chunk-anchored claims come first: the lookup returns the
// rows compiled FROM the chunks the search returned. Doc-scoped claims fill what
// the budget has left: the anchors' documents were accepted by the retrieval, so
// their other claims are evidence for the same documents, and a chunk-anchored
// lookup structurally cannot reach a claim compiled from a sibling chunk.
//
// The anchor is the retrieval's rank, so the best-ranked chunk's claims come
// first; within one chunk the claim closest to the question wins. The coverage
// floor orders the page (claims that name the question's own terms lead) rather
// than filtering it: an anchored chunk is relevant even when its claims are
// worded differently, and the caller's verdict is what decides sufficiency.
//
// depth is how many claims ONE anchor may contribute before the budget moves to
// the next anchor, and it is the shape of the page, not a detail:
//
//   - depth 2 (the pre-loop verdict and the deep-read gate) reaches a second-hop
//     fact that sits on the anchor the retrieval ranked for the first hop.
//   - depth 1 (a search result's notice) spreads the budget over as many hits as
//     it can afford, because there the question is "which of these chunks still
//     need reading?" - a notice that spends everything on the top three answers
//     that question for three chunks out of twenty-two (measured: 22.5 hits per
//     result, 2.8 covered).
func compiledClaimsForChunks(ctx context.Context, tenantID string, datasetIDs, chunkIDs, anchorDocs []string, question string, limit, depth int) []claimHit {
	if depth < 1 {
		depth = 1
	}
	if len(chunkIDs) == 0 || limit <= 0 {
		return nil
	}
	docScope, _ := resolveDocumentScope(ctx, nil)
	// The pool is sized by the ANCHOR COUNT, not by the claim budget: the store
	// returns filter hits in its own order, so the candidates have to be wide
	// enough to hold every anchor's claims before the local ranking sees them.
	pool := max(len(chunkIDs)*compiledClaimsPerAnchor, limit*4)
	if pool > compiledCandidateMax {
		pool = compiledCandidateMax
	}
	raw := compiledClaimsByChunk(ctx, tenantID, datasetIDs, chunkIDs, docScope, pool)
	rank := make(map[string]int, len(chunkIDs))
	for i, id := range chunkIDs {
		if _, seen := rank[id]; !seen {
			rank[id] = i
		}
	}
	questionTokens := claimTokens(question)
	type candidate struct {
		hit      claimHit
		rank     int
		coverage float64
	}
	seen := map[string]struct{}{}
	cands := make([]candidate, 0, len(raw))
	matched, dropped := 0, 0
	for _, h := range raw {
		key := claimKey(h)
		if key == "" {
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		// Match against EVERY source chunk, and attribute the claim to the
		// best-ranked one that matched. A claim with no source chunk in the page
		// is not the compiled artifact of anything the search returned, so it is
		// dropped rather than appended: the whole point of the anchor is that the
		// compiled leg cannot speak about a document the retrieval rejected.
		best, bestID := -1, ""
		for _, id := range h.sourceChunkIDs {
			if r, ok := rank[id]; ok && (best < 0 || r < best) {
				best, bestID = r, id
			}
		}
		if best < 0 {
			if r, ok := rank[h.chunkID]; ok {
				best, bestID = r, h.chunkID
			}
		}
		if best < 0 {
			dropped++
			continue
		}
		seen[key] = struct{}{}
		matched++
		h.chunkID = bestID
		cands = append(cands, candidate{hit: h, rank: best, coverage: claimCoverage(questionTokens, h)})
	}
	common.DebugCtx(ctx, "agentic_rag: chunk-anchored claim lookup",
		zap.Int("anchors", len(chunkIDs)),
		zap.Int("rows_returned", len(raw)),
		zap.Int("rows_on_anchor", matched),
		zap.Int("rows_dropped", dropped))

	// Doc-scoped pool: same documents, other chunks. It is fetched only when the
	// caller names the anchors' documents, and it never outranks an anchored
	// claim - it is what the page falls back to when the anchored chunks' own
	// claims do not state the fact.
	docCands := make([]candidate, 0, limit)
	if len(anchorDocs) > 0 {
		docRank := make(map[string]int, len(anchorDocs))
		for i, id := range anchorDocs {
			if _, dup := docRank[id]; !dup {
				docRank[id] = i
			}
		}
		docPool := max(len(anchorDocs)*compiledClaimsPerAnchor, limit*2)
		if docPool > compiledCandidateMax {
			docPool = compiledCandidateMax
		}
		for _, h := range compiledClaimsByDoc(ctx, tenantID, datasetIDs, anchorDocs, question, docScope, docPool) {
			key := claimKey(h)
			if key == "" {
				continue
			}
			if _, dup := seen[key]; dup {
				continue
			}
			r, ok := docRank[h.docID]
			if !ok {
				continue
			}
			seen[key] = struct{}{}
			docCands = append(docCands, candidate{hit: h, rank: r, coverage: claimCoverage(questionTokens, h)})
		}
		// Order is the text leg's ranking, kept as returned: the leg was asked the
		// caller's own question, so its order already says which statement is
		// closest to the need. The coverage field only breaks ties inside a doc.
	}
	if len(cands) == 0 && len(docCands) == 0 {
		return nil
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].rank != cands[j].rank {
			return cands[i].rank < cands[j].rank
		}
		if cands[i].coverage != cands[j].coverage {
			return cands[i].coverage > cands[j].coverage
		}
		return cands[i].hit.name < cands[j].hit.name
	})

	// The page is spent in rounds over the first `anchorBudget / depth` anchors:
	// round one takes every anchor's best-covering claim, round two its second. A
	// mid-ranked chunk's second fact is therefore reachable, which is exactly
	// where a second-hop answer lives.
	//
	// Spending the budget in plain rank order instead lets the two or three
	// top-ranked chunks consume the whole page. Measured on the FRAMES corpus:
	// the top three anchors of a 30-chunk page carried 41 claim rows, so a
	// 12-claim budget never reached anchor #5 - the chunk whose SECOND
	// best-covering claim is "Robinson made his MLB debut on April 15, 1947" -
	// although the row was already in the pool the lookup returned.
	//
	// The question-token floor does not fix that, and must not be applied here:
	// the anchor IS the relevance proof (this chunk was retrieved for this
	// question), while a second-hop claim names the second-hop entity and scores
	// near zero against the question's own terms - that same claim scored 0.22
	// against a question the corpus answers with it. The floor stays where it
	// belongs, on the claim-text recall path, where nothing else vouches for the
	// row.
	byRank := make(map[int][]candidate, len(chunkIDs))
	ranks := make([]int, 0, len(chunkIDs))
	for _, c := range cands {
		if _, ok := byRank[c.rank]; !ok {
			ranks = append(ranks, c.rank)
		}
		byRank[c.rank] = append(byRank[c.rank], c)
	}
	// ranks ascends by construction: cands is sorted by rank first.

	// Reserve room for the doc-scoped pool BEFORE spending the page on anchors: a
	// page spent entirely on the anchored chunks' own claims is exactly the page
	// that misses the sibling-chunk fact, and no later stage can recover it.
	anchorBudget := limit
	if reserve := min(len(docCands), limit/3); reserve > 0 {
		anchorBudget = limit - reserve
	}
	kept := make([]claimHit, 0, limit)
	cursor := make(map[int]int, len(ranks))
	window := anchorBudget / depth
	if window < 1 {
		window = 1
	}
	if window > len(ranks) {
		window = len(ranks)
	}
	for round := 0; round < depth && len(kept) < anchorBudget; round++ {
		for i := 0; i < window && len(kept) < anchorBudget; i++ {
			r := ranks[i]
			if cursor[r] >= len(byRank[r]) {
				continue
			}
			kept = append(kept, byRank[r][cursor[r]].hit)
			cursor[r]++
		}
	}
	// A page with fewer claims than the budget, or anchors past the window with
	// budget left over, fills in rank order rather than shipping a thin page:
	// the caller asks the model whether these claims answer the question, so
	// another anchored claim is worth more than an empty slot.
	for _, r := range ranks {
		for cursor[r] < len(byRank[r]) && len(kept) < anchorBudget {
			kept = append(kept, byRank[r][cursor[r]].hit)
			cursor[r]++
		}
	}
	// Then the anchors' documents' other claims, best document first.
	for _, c := range docCands {
		if len(kept) >= limit {
			break
		}
		kept = append(kept, c.hit)
	}
	common.DebugCtx(ctx, "agentic_rag: claim page assembled",
		zap.Int("anchored", len(cands)),
		zap.Int("doc_scoped", len(docCands)),
		zap.Int("kept", len(kept)))
	return kept
}

// compiledPrefetch is the ONE compiled-evidence recall a turn performs before
// its loop: the claims of the chunks a plain search returns for the question.
type compiledPrefetch struct {
	// chunks are the anchor chunks the search returned.
	chunks []string
	// claims are their compiled statements, ranked.
	claims []claimHit
	// evidence is the rendered claim block, carrying the [ID:n] handles its
	// source chunks were stamped with. It is rendered only when claims exist,
	// because rendering is what publishes handles.
	evidence string
}

// claimKey is the identity of a claim across result sets: its statement text.
// Two rows stating the same thing are the same evidence however they were
// anchored, which is what lets a second sighting be dropped instead of re-billed.
func claimKey(c claimHit) string {
	return strings.ToLower(strings.TrimSpace(c.name))
}

// prefetchCompiledEvidence searches once and returns the compiled artifacts of
// what the search found. An empty result means the corpus has no compiled
// claims, or the search found none of them — both leave the ordinary loop as
// the only path.
func prefetchCompiledEvidence(ctx context.Context, tenantID string, datasetIDs []string, messages []*schema.Message) compiledPrefetch {
	question := strings.TrimSpace(lastUserQuestion(messages))
	if question == "" || len(datasetIDs) == 0 {
		return compiledPrefetch{}
	}
	run := compiledRunFrom(ctx)
	if run == nil {
		run = &compiledRun{tenantID: tenantID, datasetIDs: datasetIDs, shown: map[string]struct{}{}}
		ctx = context.WithValue(ctx, compiledRunKey{}, run)
	}
	// A compiled dataset does not imply claims: trees, timelines and mindmaps
	// all carry compile_kwd but no evidence rows. Probing first keeps the
	// search and the verdict call off a corpus that can only come back empty.
	if !run.claimsPresent(ctx) {
		common.DebugCtx(ctx, "agentic_rag: compiled prefetch skipped",
			zap.Strings("datasets", datasetIDs),
			zap.String("reason", "no claim rows in the bound datasets"))
		return compiledPrefetch{}
	}
	docScope, _ := resolveDocumentScope(ctx, nil)
	// A non-nil EMPTY scope means the turn allows no documents: an empty
	// docScope reads as "unrestricted" downstream, so stop before the search.
	if docScope != nil && len(docScope) == 0 {
		common.DebugCtx(ctx, "agentic_rag: compiled prefetch skipped",
			zap.Strings("datasets", datasetIDs),
			zap.String("reason", "no documents in the turn's scope"))
		return compiledPrefetch{}
	}

	chunks := compiledAnchorSearch(ctx, tenantID, datasetIDs, docScope, question, compiledAnchorTopN)
	chunkIDs := make([]string, 0, len(chunks))
	for _, c := range chunks {
		if c.ID != "" {
			chunkIDs = append(chunkIDs, c.ID)
		}
	}
	if len(chunkIDs) == 0 {
		common.InfoCtx(ctx, "agentic_rag: compiled prefetch found no anchor chunks",
			zap.String("query", question), zap.Strings("datasets", datasetIDs))
		return compiledPrefetch{}
	}
	claims := compiledClaimsForChunks(ctx, tenantID, datasetIDs, chunkIDs, anchorDocsOf(chunks), question, compiledPrefetchClaims, compiledAnchorDepth)
	pf := compiledPrefetch{chunks: chunkIDs, claims: claims}
	if len(claims) == 0 {
		common.InfoCtx(ctx, "agentic_rag: compiled prefetch found no claims for the searched chunks",
			zap.String("query", question),
			zap.Strings("datasets", datasetIDs),
			zap.Int("chunks", len(chunkIDs)))
		return pf
	}
	pf.evidence = claimEvidenceBlock(ctx, claims)
	// covered vs chunks is the coverage the strategy actually achieved: of the
	// chunks the search returned, how many carried compiled artifacts. A run
	// whose evidence is thin is a retrieval problem (the page missed the compiled
	// passages), not a claim-lookup problem, and only this ratio tells them apart.
	// It counts ANCHOR chunks only, so the doc-scoped fill cannot inflate it.
	common.InfoCtx(ctx, "agentic_rag: compiled prefetch ready",
		zap.String("query", question),
		zap.Strings("datasets", datasetIDs),
		zap.Int("chunks", len(chunkIDs)),
		zap.Int("anchor_docs", len(anchorDocsOf(chunks))),
		zap.Int("covered_chunks", anchorCoverage(claims, chunkIDs)),
		zap.Int("claims", len(claims)),
		zap.Int("claims_from_sibling_chunks", len(claims)-anchorCoverage(claims, chunkIDs)),
		zap.Strings("docs", claimDocsInOrder(claims)),
		zap.Int("evidence_bytes", len(pf.evidence)))
	return pf
}

// anchorCoverage counts the anchored chunks the page's claims are attributed to.
func anchorCoverage(claims []claimHit, anchors []string) int {
	if len(anchors) == 0 {
		return 0
	}
	want := make(map[string]struct{}, len(anchors))
	for _, id := range anchors {
		want[id] = struct{}{}
	}
	seen := make(map[string]struct{}, len(claims))
	for _, c := range claims {
		if _, ok := want[c.chunkID]; ok {
			seen[c.chunkID] = struct{}{}
		}
	}
	return len(seen)
}

// anchorDocsOf returns the distinct documents of a retrieved chunk page, in
// retrieval order. It is the scope of the doc-scoped claim leg: the documents the
// search accepted, which is what "the retrieval rejected this document" means.
func anchorDocsOf(chunks []runtime.RetrievalChunk) []string {
	seen := make(map[string]struct{}, len(chunks))
	out := make([]string, 0, len(chunks))
	for _, c := range chunks {
		id := strings.TrimSpace(c.DocumentID)
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// coveredAnchorChunks returns the distinct anchor chunks the surviving claims are
// attributed to. It is the numerator of the strategy's coverage: claims that
// matched nothing were already dropped, so this is exactly "how many of the
// retrieved chunks had compiled artifacts".
func coveredAnchorChunks(claims []claimHit) map[string]struct{} {
	covered := make(map[string]struct{}, len(claims))
	for _, c := range claims {
		if c.chunkID != "" {
			covered[c.chunkID] = struct{}{}
		}
	}
	return covered
}

// claimDocsInOrder returns the distinct source documents of a claim set, in
// first-appearance order, for logging and evidence grouping.
func claimDocsInOrder(claims []claimHit) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(claims))
	for _, c := range claims {
		if c.docID == "" {
			continue
		}
		if _, dup := seen[c.docID]; dup {
			continue
		}
		seen[c.docID] = struct{}{}
		out = append(out, c.docID)
	}
	return out
}

// claimEvidenceBlock renders the recalled claims as answer material, one <doc>
// element per source document so a multi-document question keeps its provenance
// and the answer can name which document each fact came from.
//
// Claims are grouped by document in first-appearance order rather than emitted in
// page order: a page mixes claims of the anchored chunks with claims of the same
// documents' other chunks, so emitting them in page order would open and close the
// same <doc> element several times.
func claimEvidenceBlock(ctx context.Context, claims []claimHit) string {
	order := make([]string, 0, 4)
	byDoc := make(map[string][]claimHit, 4)
	for _, c := range claims {
		if _, ok := byDoc[c.docID]; !ok {
			order = append(order, c.docID)
		}
		byDoc[c.docID] = append(byDoc[c.docID], c)
	}
	var b strings.Builder
	for _, doc := range order {
		if doc != "" {
			fmt.Fprintf(&b, "<doc doc_id=\"%s\">\n", xmlEscape(doc))
		}
		for _, c := range byDoc[doc] {
			b.WriteString(claimOutlineLine(ctx, c))
			b.WriteString("\n")
		}
		if doc != "" {
			b.WriteString("</doc>\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// compiledEvidenceNotice renders the claims compiled from ONE locate result
// set, as the leading block of that tool result.
//
// It returns "" for a corpus without claims, for a result set that carries no
// corpus chunk (web hits), and for a result set whose claims were ALL shown
// earlier — so a non-compiled corpus pays one cached existence probe per turn and
// nothing else, and a repeat result pays one pointer line naming the handles it
// already has.
func compiledEvidenceNotice(ctx context.Context, tool, query string, hits []snippetHit) string {
	run := compiledRunFrom(ctx)
	if run == nil || len(hits) == 0 {
		return ""
	}
	chunkIDs := make([]string, 0, len(hits))
	for _, h := range hits {
		if h.web || h.chunk.ID == "" {
			continue
		}
		chunkIDs = append(chunkIDs, h.chunk.ID)
	}
	if len(chunkIDs) == 0 {
		return ""
	}
	if !run.claimsPresent(ctx) {
		return ""
	}
	// Rank against the query THIS call just issued, not the turn's question: in a
	// multi-hop run the second search asks about the link, and ranking its result
	// set by the final question surfaces exactly the claims the previous notice
	// already showed - which takeUnseen then drops, leaving the new hits bare.
	need := strings.TrimSpace(query)
	if need == "" {
		need = strings.TrimSpace(intentFrom(ctx))
	}
	if need == "" {
		need = queryFrom(ctx)
	}
	page := compiledClaimsForChunks(ctx, run.tenantID, run.datasetIDs, chunkIDs, nil, need, compiledNoticeClaims*2, 1)
	claims := run.takeUnseen(page, compiledNoticeClaims)
	// Hits whose statements an EARLIER result already put in this turn: their text
	// is in the transcript under a ref handle, so the model needs the handle, not
	// the chunk. Without this line the result looks claim-less and the only way to
	// find out what those chunks say is a deep read of text the model already has.
	shown := shownClaimChunks(ctx, run, page, coveredAnchorChunks(claims))
	if len(claims) == 0 && len(shown) == 0 {
		return ""
	}
	lines := make([]string, 0, len(claims))
	for _, c := range claims {
		lines = append(lines, xmlEscape(claimOutlineLine(ctx, c)))
	}
	common.InfoCtx(ctx, "agentic_rag: compiled evidence attached to tool result",
		zap.String("tool", tool),
		zap.Int("hits", len(hits)),
		zap.Int("covered_chunks", len(coveredAnchorChunks(claims))),
		zap.Int("shown_chunks", len(shown)),
		zap.Int("claims", len(claims)))
	var b strings.Builder
	fmt.Fprintf(&b, "<compiled_evidence count=\"%d\">\n", len(claims))
	if len(lines) > 0 {
		fmt.Fprintf(&b, "<note>These statements are the compiled artifacts of %d of the %d chunks below: each one is an "+
			"atomic fact with the verbatim quote it was compiled from, and ref=n is the handle to cite that source chunk by. "+
			"If they already state what the question needs, answer from them now and cite their ref handles - do NOT "+
			"call list_chunks for those chunks. Read a raw chunk only when the fact you need is not stated here, "+
			"or when that chunk carries no statement above.</note>\n", len(coveredAnchorChunks(claims)), len(chunkIDs))
		b.WriteString(strings.Join(lines, "\n"))
		b.WriteString("\n")
	}
	if len(shown) > 0 {
		// Not repeated text - only where the text already is. This is the whole
		// point of tracking shown claims per chunk instead of only per statement.
		fmt.Fprintf(&b, "<already_shown>Compiled statements for %d more chunk(s) of this result set were shown "+
			"earlier in this conversation; cite them by these handles instead of reading the chunks again: %s.</already_shown>\n",
			len(shown), strings.Join(shown, ", "))
	}
	b.WriteString("</compiled_evidence>")
	return b.String()
}

// shownClaimChunks names the hits whose compiled statements this turn already
// showed, as "chunk_id=[ID:n]" pairs, skipping the chunks the fresh block already
// covers. It is capped so a large result set cannot turn the pointer line into
// the very cost it exists to avoid.
func shownClaimChunks(ctx context.Context, run *compiledRun, page []claimHit, covered map[string]struct{}) []string {
	if run == nil {
		return nil
	}
	reg := evidenceRegistryFrom(ctx)
	if reg == nil {
		return nil
	}
	seen := make(map[string]struct{}, len(page))
	out := make([]string, 0, compiledNoticeShownChunks)
	for _, c := range page {
		if c.chunkID == "" || len(out) >= compiledNoticeShownChunks {
			continue
		}
		if _, dup := covered[c.chunkID]; dup {
			continue
		}
		if _, dup := seen[c.chunkID]; dup {
			continue
		}
		if !run.claimShown(c) {
			continue
		}
		seen[c.chunkID] = struct{}{}
		out = append(out, fmt.Sprintf("%s=[ID:%d]", c.chunkID, reg.Stamp(c.chunkID)))
	}
	return out
}

// compiledEvidenceGate is the last station before raw text: a deep read that is
// about to open these chunks is answered with their compiled artifacts instead,
// as long as any of those statements is new to this turn.
//
// It returns "" - meaning "go ahead and serve the raw text" - when the corpus
// has no claims, when these chunks carry none, or when everything they carry was
// already offered earlier in the turn. That last case is the important one: a
// caller that still asks to read after the statements were put in front of it
// has judged them insufficient, so withholding the passage a second time would
// only stall the loop.
//
// The note names the way out (raw=true): a gate that does not offer one turns
// "the claims were not enough" into a dead end, and every line carries its ref
// handle so an answer given from this block is still citable.
func compiledEvidenceGate(ctx context.Context, chunkIDs []string) string {
	if len(chunkIDs) == 0 {
		return ""
	}
	run := compiledRunFrom(ctx)
	if run == nil || !run.claimsPresent(ctx) {
		return ""
	}
	// Rank against what THIS call needs (its stated intent), not only against the
	// turn's question: a deep read in the middle of a multi-hop run exists to
	// fetch a link that is often irrelevant to the final question.
	need := strings.TrimSpace(intentFrom(ctx))
	if need == "" {
		need = queryFrom(ctx)
	}
	claims := compiledClaimsForChunks(ctx, run.tenantID, run.datasetIDs, chunkIDs, nil, need, compiledGateClaims*2, compiledAnchorDepth)
	claims = run.takeUnseen(claims, compiledGateClaims)
	if len(claims) == 0 {
		return ""
	}
	covered := len(coveredAnchorChunks(claims))
	common.InfoCtx(ctx, "agentic_rag: compiled evidence gated a raw read",
		zap.Strings("chunks", chunkIDs),
		zap.Int("claims", len(claims)),
		zap.Int("covered_chunks", covered))

	lines := make([]string, 0, len(claims))
	for _, c := range claims {
		lines = append(lines, xmlEscape(claimOutlineLine(ctx, c)))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<compiled_evidence count=\"%d\">\n", len(claims))
	b.WriteString("<note>These statements are the compiled artifacts of the chunks this call asked to read: each one is an ")
	b.WriteString("atomic fact with the verbatim quote it was compiled from, and ref=n is the handle for citing that chunk. ")
	b.WriteString("The raw text was NOT fetched. If these statements already state the fact this call needs, answer from them ")
	b.WriteString("now and cite their ref handles. If the passage itself is genuinely still needed, call list_chunks again ")
	b.WriteString("with raw=true and the original text is returned.</note>\n")
	b.WriteString(strings.Join(lines, "\n"))
	b.WriteString("\n</compiled_evidence>")
	return b.String()
}
