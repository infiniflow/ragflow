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
// The anchored leg is no longer the ONLY one. A claim is an atomic statement, so
// its own text is a better query target than the chunk it came from: a chunk is a
// table or a paragraph whose embedding is diluted by everything else it says.
// The objection above still holds for a leg that REPLACES the anchor - it would
// be a second ranking that can land elsewhere - and this leg is kept out of that
// way: the anchor's claims stay first, the text leg only ADDS claims the anchor
// did not produce, and a claim both legs found appears once. Measured offline on
// 20 FRAMES questions, the anchor alone covered 3 gold answers and the union
// covered 9; the anchor also had one answer the text leg missed, which is why
// neither leg replaces the other.
//
// Two consumers, one rule ("compiled first, raw chunk only when it is not
// enough"):
//
//   - the pre-loop probe: one search, an answer call over the claims it found,
//     then a verification of that answer. An answer that survives verification
//     ends the turn without a chunk ever being read; one that does not is dropped
//     and the ordinary loop runs, because a multi-hop question's second fact
//     lives in a document the question's own wording never reaches and the loop
//     is the only place that decides what to search next.
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
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	enginetypes "ragflow/internal/engine/types"
	"ragflow/internal/service/nav"
	"ragflow/internal/tokenizer"
)

const (
	// compiledAnchorTopN is the chunk page the pre-loop probe anchors on. It is
	// the retrieval's own top-N, not a claim count: the claims come from
	// whatever these chunks hold.
	compiledAnchorTopN = 10
	// compiledClaimsPerChunk sizes ONE claim lookup. It is a query bound, not a
	// cap on what is shown: a lookup has to name a row count, so it asks for the
	// most rows a chunk was measured to carry (38) for every chunk it asks
	// about, and every row that comes back is served. A chunk's claim set is that
	// chunk's representation - serving a part of it serves a part of the chunk,
	// and a model that is shown a part cannot judge whether the whole suffices.
	compiledClaimsPerChunk = 48
	// claimSearchTopK is the depth of the claim-TEXT leg. Measured offline on the
	// same 20 FRAMES questions: depth 20 covered 1 gold answer, depth 50 covered 8.
	// Depth is cheap here because the page is cut down again per line by the
	// decision filter, so shallow recall is the expensive choice, not deep recall.
	claimSearchTopK = 50
	// compiledNoticeShownChunks caps the pointer line that names hits whose
	// statements this turn already showed. Eight is where the line stays under
	// ~300 characters: it pays for itself if it averts a single deep read, and it
	// stops paying once it grows into a second block of text.
	compiledNoticeShownChunks = 8
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

// takeUnseen keeps every claim this run has not shown yet and records them as
// shown. A claim already served keeps its ref handle in the transcript, so
// repeating its text adds nothing the model does not have - and a claim the turn
// has NOT served is part of its chunk, so dropping it would serve half a chunk.
func (r *compiledRun) takeUnseen(claims []claimHit) []claimHit {
	out := make([]claimHit, 0, len(claims))
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

func readCompiledClaimsByChunk(ctx context.Context, tenantID string, datasetIDs, chunkIDs, docScope []string, limit int) []claimHit {
	if len(chunkIDs) == 0 || len(datasetIDs) == 0 {
		return nil
	}
	de := engine.Get()
	if de == nil {
		return nil
	}
	filter := map[string]interface{}{
		"entity_type_kwd":  []string{navClaimRowType},
		"scope_kwd":        []string{navClaimDocScope},
		"source_chunk_ids": chunkIDs,
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

// claimPageEnabled reports whether the claim page is served. Off means the
// prefetch serves the retrieved chunks as raw text: the control arm for measuring
// what the claim artifact is worth, and an escape hatch for a deployment whose
// ingestion has not compiled claims yet.
func claimPageEnabled() bool {
	return envBoolDefault(common.EnvClaimPage, true)
}

// compiledClaimsForChunks pages the claims compiled FROM a retrieved chunk set.
//
// The chunk set is the whole evidence boundary: a claim exists because some
// chunk stated it, so the page is exactly "the chunks the search returned, and
// what each of them states". No claim of a chunk the search did not return is
// offered - that would be evidence the retrieval rejected this turn.
//
// The anchor is the retrieval's rank, so the best-ranked chunk's claims come
// first; within one chunk the claim closest to the question wins. The coverage
// score ORDERS the claims (those that name the question's own terms lead) rather
// than dropping any: a chunk's claim set is that chunk's representation, so the
// page is every claim of every chunk it was asked about. Nothing is capped -
// dropping a claim would drop part of the chunk it belongs to, and the caller's
// verdict would then be judging an excerpt rather than the evidence.
//
// The second return names the chunks the store HAS claims for. It is not the
// same as the chunks the page covers: a claim whose statement another chunk
// already stated is deduped away, and a chunk whose every claim was such a
// duplicate still has claims - it must not be served as raw text on top of them.
func compiledClaimsForChunks(ctx context.Context, tenantID string, datasetIDs, chunkIDs []string, question string) ([]claimHit, map[string]struct{}) {
	if len(chunkIDs) == 0 {
		return nil, nil
	}
	docScope, _ := resolveDocumentScope(ctx, nil)
	// The pool is sized by the ANCHOR COUNT: the store returns filter hits in its
	// own order, so the query has to be wide enough to hold every anchor's whole
	// claim set before the local ranking sees it. Every chunk asked about is
	// queried and every row that comes back is served - a chunk's claim set is
	// that chunk's representation, so neither the chunk list nor the row count can
	// be trimmed without serving a part of a chunk as if it were the whole.
	pool := len(chunkIDs) * compiledClaimsPerChunk
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
	hasClaims := make(map[string]struct{}, len(chunkIDs))
	cands := make([]candidate, 0, len(raw))
	matched, dropped := 0, 0
	for _, h := range raw {
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
		// Recorded BEFORE the duplicate check: a chunk whose statements were all
		// already stated elsewhere still has claims, and must not be served as
		// raw text on top of them.
		hasClaims[bestID] = struct{}{}
		key := claimKey(h)
		if key == "" {
			continue
		}
		if _, dup := seen[key]; dup {
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

	if len(cands) == 0 {
		return nil, hasClaims
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

	// No budget, no rounds, no window: the page is every claim the chunks above
	// carry, best-ranked chunk first and closest-to-question first inside it. A
	// chunk is either on the page whole or not on it at all.
	kept := make([]claimHit, 0, len(cands))
	for _, c := range cands {
		kept = append(kept, c.hit)
	}
	// The text leg is a UNION, appended after the anchored claims and deduped
	// against them: the anchored order is the retrieval's own ranking and stays
	// authoritative, while a claim only the text leg found is still evidence the
	// anchor could not reach. hasClaims is deliberately untouched - it names the
	// chunks the page is responsible for, and a chunk this turn never retrieved is
	// not one of them.
	searched := searchClaimsForQuestion(ctx, tenantID, datasetIDs, docScope, question, claimSearchTopK)
	added := 0
	for _, c := range searched {
		key := claimKey(c)
		if key == "" {
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		kept = append(kept, c)
		added++
	}
	common.DebugCtx(ctx, "agentic_rag: claim page assembled",
		zap.Int("anchored", len(cands)),
		zap.Int("searched", len(searched)),
		zap.Int("search_added", added),
		zap.Int("kept", len(kept)))
	// The page is assembled whole and cut per LINE afterwards: a claim is an
	// atomic statement, so the sufficiency model can be asked about it on its
	// own, and one the question does not need leaves without taking its
	// neighbours' text with it. This is the only bound the page has - a chunk's
	// claim set can cost more than the chunk it stands for.
	return filterClaims(ctx, question, kept), hasClaims
}

// claimSearchEnabled gates the claim-text leg. On by default: the leg can only
// add claims the anchored page did not have, and it degrades to nothing when the
// deployment has no query-side embedder.
func claimSearchEnabled() bool {
	return envBoolDefault(common.EnvClaimSearch, true)
}

// searchClaimsForQuestion searches the claim rows BY THEIR OWN TEXT, which is the
// one lookup the anchored leg cannot make: a claim is reachable only through a
// chunk the retrieval already ranked, so a fact whose chunk ranked eleventh is
// invisible however well its claim matches the question.
//
// Every degraded path returns nil, and nil means "the anchored page stands": no
// engine, no nav service, no tenant embedder, an engine error. The leg is an
// extra, never a replacement, so a deployment that cannot embed is byte-for-byte
// the run it was before this existed.
func searchClaimsForQuestion(ctx context.Context, tenantID string, datasetIDs, docScope []string, question string, limit int) []claimHit {
	if limit <= 0 || !claimSearchEnabled() || len(datasetIDs) == 0 || tenantID == "" {
		return nil
	}
	question = strings.TrimSpace(question)
	if question == "" {
		return nil
	}
	// The tenant's default embedding model decides the column: the rows carry a
	// vector per model dimension, and addressing a column the rows do not have is
	// an empty result, not an error.
	vec := claimQueryVector(ctx, tenantID, question)
	if len(vec) == 0 {
		return nil
	}
	filter := map[string]interface{}{
		"entity_type_kwd": []string{navClaimRowType},
		"scope_kwd":       []string{navClaimDocScope},
	}
	if len(docScope) > 0 {
		filter["doc_id"] = docScope
	}
	rows, err := claimSearchRows(ctx, &enginetypes.SearchRequest{
		IndexNames: []string{indexNameFor(tenantID, "")},
		KbIDs:      datasetIDs,
		SelectFields: []string{
			"content_with_weight", "source_chunk_ids", "entity_type_kwd", "compile_kwd", "doc_id",
		},
		Filter:             filter,
		Limit:              limit,
		IncludeUnavailable: true,
		MatchExprs: []interface{}{&enginetypes.MatchDenseExpr{
			VectorColumnName:  fmt.Sprintf("q_%d_vec", len(vec)),
			EmbeddingData:     vec,
			EmbeddingDataType: "float",
			DistanceType:      "cosine",
			TopN:              limit,
		}},
	})
	if err != nil {
		common.DebugCtx(ctx, "agentic_rag: claim text search failed",
			zap.Strings("datasets", datasetIDs), zap.Error(err))
		return nil
	}
	out := make([]claimHit, 0, len(rows))
	for _, row := range rows {
		if h, ok := parseClaimHit(row); ok {
			out = append(out, h)
		}
	}
	return out
}

// claimQueryVector encodes the question with the tenant's default embedding
// model. It is a seam for the same reason the chunk search is: "nil means the
// anchored page stands" is the leg's whole contract, and a deployment without a
// query-side embedder has to be reachable from a unit test.
var claimQueryVector = encodeClaimQueryVector

// claimSearchRows runs the claim-KNN. A nil engine and an engine error are the
// same outcome - no extra claims - so neither can fail the page.
var claimSearchRows = searchClaimRows

func encodeClaimQueryVector(ctx context.Context, tenantID, question string) []float64 {
	svc := nav.GetNavService()
	if svc == nil {
		return nil
	}
	return svc.EncodeQueryVector(ctx, tenantID, question)
}

func searchClaimRows(ctx context.Context, req *enginetypes.SearchRequest) ([]map[string]interface{}, error) {
	de := engine.Get()
	if de == nil {
		return nil, nil
	}
	res, err := de.Search(ctx, req)
	if err != nil || res == nil {
		return nil, err
	}
	return res.Chunks, nil
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
	// The page is the retrieved chunks' own claims: no document-scoped leg (a
	// chunk the search did not return is not evidence this turn may offer), and
	// no cap of any kind - a chunk is on the page whole, with every claim it
	// carries, or it is not on the page at all.
	if !claimPageEnabled() {
		// Control arm: the same retrieval served as raw chunk text instead of its
		// claims. Only the artifact differs, so a comparison against this page
		// measures the claim page itself rather than a different search.
		bareAll := chunksWithoutClaims(chunks, nil)
		pf := compiledPrefetch{chunks: chunkIDs, evidence: chunksAsEvidence(ctx, bareAll)}
		common.InfoCtx(ctx, "agentic_rag: claim page disabled, serving chunks as text",
			zap.String("query", question),
			zap.Int("chunks", len(chunkIDs)),
			zap.Int("chunks_as_text", len(bareAll)),
			zap.Int("evidence_bytes", len(pf.evidence)))
		return pf
	}
	claims, hasClaims := compiledClaimsForChunks(ctx, tenantID, datasetIDs, chunkIDs, question)
	pf := compiledPrefetch{chunks: chunkIDs, claims: claims}
	if len(claims) == 0 {
		common.InfoCtx(ctx, "agentic_rag: compiled prefetch found no claims for the searched chunks",
			zap.String("query", question),
			zap.Strings("datasets", datasetIDs),
			zap.Int("chunks", len(chunkIDs)))
		return pf
	}
	// A retrieved chunk that carries no claims is served as its raw text in the
	// same page: the verdict is asked whether this evidence answers the question,
	// and a chunk that silently vanished from the page cannot be part of that
	// answer. "Claims where they exist, the chunk itself where they do not."
	bare := chunksWithoutClaims(chunks, hasClaims)
	pf.evidence = claimEvidenceBlock(ctx, claims)
	if raw := chunksAsEvidence(ctx, bare); raw != "" {
		if pf.evidence != "" {
			pf.evidence += "\n"
		}
		pf.evidence += raw
	}
	common.InfoCtx(ctx, "agentic_rag: compiled prefetch ready",
		zap.String("query", question),
		zap.Strings("datasets", datasetIDs),
		zap.Int("chunks", len(chunkIDs)),
		zap.Int("anchor_docs", len(anchorDocsOf(chunks))),
		zap.Int("chunks_with_claims", len(hasClaims)),
		zap.Int("chunks_as_text", len(bare)),
		zap.Int("claims", len(claims)),
		zap.Strings("docs", claimDocsInOrder(claims)),
		zap.Int("evidence_bytes", len(pf.evidence)))
	return pf
}

// chunksWithoutClaims returns the retrieved chunks the store has no claims for,
// in retrieval order. It takes the HAS-claims set rather than the surviving page,
// because a chunk whose every statement was deduped away still carries claims and
// must not be served as raw text on top of them.
func chunksWithoutClaims(chunks []runtime.RetrievalChunk, covered map[string]struct{}) []runtime.RetrievalChunk {
	out := make([]runtime.RetrievalChunk, 0, len(chunks))
	for _, c := range chunks {
		if c.ID == "" || strings.TrimSpace(c.Content) == "" {
			continue
		}
		if _, ok := covered[c.ID]; ok {
			continue
		}
		out = append(out, c)
	}
	return out
}

// chunksAsEvidence renders claim-less chunks as their own text, in the same
// <chunks> shape the deep-read tool uses, so the page reads uniformly. It skips
// the deep-read sufficiency filter: these chunks ARE part of the page the
// verdict is about to judge, so nothing may shorten them.
func chunksAsEvidence(ctx context.Context, chunks []runtime.RetrievalChunk) string {
	if len(chunks) == 0 {
		return ""
	}
	return renderChunksXML(ctx, "", chunks, "", "")
}

// anchorDocsOf returns the distinct documents of a retrieved chunk page, in
// retrieval order. It is the document scope the retrieval accepted, logged so a
// page that silently collapsed onto fewer documents is visible.
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
	// Every hit's every claim, deduped against what this turn already showed. The
	// result set's size is the retrieval's to decide: a cap here would serve some
	// chunks as a fragment of themselves, and the model has no way to tell a
	// chunk's full statement set from the part it was shown.
	page, _ := compiledClaimsForChunks(ctx, run.tenantID, run.datasetIDs, chunkIDs, need)
	claims := run.takeUnseen(page)
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
		lines = append(lines, xmlEscape(claimCompactLine(ctx, c)))
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
			"atomic fact, and ref=n is the handle to cite that source chunk by. "+
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
//
// It returns the block AND the chunks that block represents, so the caller can
// serve "claims where they exist, the chunk itself where they do not" in one
// answer instead of making the model ask twice.
func compiledEvidenceGate(ctx context.Context, chunkIDs []string) (string, []string) {
	if len(chunkIDs) == 0 {
		return "", nil
	}
	run := compiledRunFrom(ctx)
	if run == nil || !run.claimsPresent(ctx) {
		return "", nil
	}
	// Rank against what THIS call needs (its stated intent), not only against the
	// turn's question: a deep read in the middle of a multi-hop run exists to
	// fetch a link that is often irrelevant to the final question.
	need := strings.TrimSpace(intentFrom(ctx))
	if need == "" {
		need = queryFrom(ctx)
	}
	page, hasClaims := compiledClaimsForChunks(ctx, run.tenantID, run.datasetIDs, chunkIDs, need)
	claims := run.takeUnseen(page)
	if len(claims) == 0 {
		return "", nil
	}
	// hasClaims, not the survival set: a chunk whose statement another chunk
	// already stated is deduped off the page, and serving it as raw text would
	// repeat what the block above says.
	coveredSet := hasClaims
	covered := make([]string, 0, len(coveredSet))
	for _, id := range chunkIDs {
		if _, ok := coveredSet[id]; ok {
			covered = append(covered, id)
		}
	}
	common.InfoCtx(ctx, "agentic_rag: compiled evidence gated a raw read",
		zap.Strings("chunks", chunkIDs),
		zap.Int("claims", len(claims)),
		zap.Int("covered_chunks", len(covered)))

	lines := make([]string, 0, len(claims))
	for _, c := range claims {
		lines = append(lines, xmlEscape(claimCompactLine(ctx, c)))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<compiled_evidence count=\"%d\">\n", len(claims))
	b.WriteString("<note>These statements are the compiled artifacts of the chunks this call asked to read: each one is an ")
	b.WriteString("atomic fact, and ref=n is the handle for citing that chunk. ")
	b.WriteString("No raw text accompanies them. If these statements already state the fact this call needs, answer from them ")
	b.WriteString("now and cite their ref handles. If the passage itself is genuinely still needed, call list_chunks again ")
	b.WriteString("with raw=true and the original text is returned.</note>\n")
	b.WriteString(strings.Join(lines, "\n"))
	b.WriteString("\n</compiled_evidence>")
	return b.String(), covered
}

// Claim row shape, read by the chunk-anchored claim lookup.
const (
	// navClaimRowType is the entity_type_kwd every compiler gives its evidence
	// rows. They deliberately carry no knowledge_graph_kwd and no type_kwd, so
	// they never show up as artifacts-tree nodes — this filter is the only way
	// to find them.
	navClaimRowType = "claim"
	// navClaimDocScope pins the lookup to doc-level rows: the KB-wide merged
	// rows written by the Build button carry a doc_id too, and without this
	// filter they would answer for a single document.
	navClaimDocScope = "doc"
	// navClaimEvidenceChars caps the rendered verbatim quote of a claim line.
	// Generous on purpose: a thin quote makes the answer stage judge the context
	// insufficient, and one extra re-search turn costs a full prompt — far more
	// than the extra characters.
	navClaimEvidenceChars = 1200
)

// claimHit is one recalled claim row: an atomic proposition plus its verbatim
// evidence. chunkID is the FIRST source chunk id — the pointer the model feeds
// to list_chunks when it needs the surrounding passage. docID names the document
// the claim was compiled from, which is what makes document diversity checkable.
type claimHit struct {
	docID string
	// chunkID is the chunk this claim is ATTRIBUTED to: the source chunk the
	// caller matched it by, which is not necessarily sourceChunkIDs[0] — a claim
	// compiled from a multi-chunk passage names several. Rendering and citation
	// handles use this one.
	chunkID string
	// sourceChunkIDs is every chunk the claim was compiled from, as stored. A
	// chunk-anchored lookup must match against ALL of them: matching only the
	// first would drop a claim whose first source chunk is not in the retrieved
	// page while its second is.
	sourceChunkIDs []string
	name           string
	description    string
	quote          string
	score          float64
}

// parseClaimHit turns one claim row into a hit. A row that is not a usable
// claim — bad payload, unnamed, or carrying no chunk pointer — is dropped: a
// claim the model cannot open is not an answer anchor.
func parseClaimHit(row map[string]interface{}) (claimHit, bool) {
	payload, ok := decodeJSONObject(contentWithWeightFromRaw(row))
	if !ok {
		return claimHit{}, false
	}
	name := strings.TrimSpace(stringFromAny(payload["name"]))
	if name == "" {
		return claimHit{}, false
	}
	ids := navStringList(row["source_chunk_ids"])
	if len(ids) == 0 {
		ids = navStringList(payload["source_chunk_ids"])
	}
	if len(ids) == 0 {
		return claimHit{}, false
	}
	docID := strings.TrimSpace(fmt.Sprint(row["doc_id"]))
	if docID == "<nil>" {
		docID = ""
	}
	return claimHit{
		docID:          docID,
		chunkID:        ids[0],
		sourceChunkIDs: ids,
		name:           name,
		description:    strings.TrimSpace(stringFromAny(payload["description"])),
		quote:          firstEvidenceQuote(payload["evidence"]),
		score:          floatFromMap(row, "_score"),
	}, true
}

// claimTokens returns the set of content tokens of a text, the vocabulary the
// claim coverage score is measured against. It uses the corpus tokenizer when
// the pool is up (same analyzer the claim text was indexed with) and falls back
// to a rune-level splitter, so a deployment without the analyzer still gets a
// score instead of none. Single-rune tokens are dropped: they are stopwords in
// alphabetic languages.
func claimTokens(text string) map[string]struct{} {
	tokens := map[string]struct{}{}
	if tok, err := tokenizer.Tokenize(text); err == nil {
		for _, t := range strings.Fields(strings.ToLower(tok)) {
			if len([]rune(t)) >= 2 {
				tokens[t] = struct{}{}
			}
		}
	}
	if len(tokens) > 0 {
		return tokens
	}
	for _, t := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if len([]rune(t)) >= 2 {
			tokens[t] = struct{}{}
		}
	}
	return tokens
}

// claimCoverage is the share of the question's content tokens this claim states
// or quotes. It ORDERS a chunk's claims by how close they are to the question
// being asked; it is not a membership filter, because a chunk's claim set is
// that chunk's representation and dropping part of it would judge an excerpt.
func claimCoverage(question map[string]struct{}, c claimHit) float64 {
	if len(question) == 0 {
		return 1
	}
	text := c.name + " " + c.description + " " + c.quote
	claim := claimTokens(text)
	if len(claim) == 0 {
		return 0
	}
	matched := 0
	for t := range question {
		if _, ok := claim[t]; ok {
			matched++
		}
	}
	return float64(matched) / float64(len(question))
}

// firstEvidenceQuote returns the first non-empty verbatim quote of a claim's
// evidence list.
func firstEvidenceQuote(v any) string {
	list, _ := v.([]any)
	for _, item := range list {
		ev, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if q := strings.TrimSpace(stringFromAny(ev["quote"])); q != "" {
			return q
		}
	}
	return ""
}

// stringFromAny reads a scalar as a trimmed string, tolerating the numeric
// types engines decode JSON numbers into.
func stringFromAny(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case fmt.Stringer:
		return x.String()
	case nil:
		return ""
	default:
		return fmt.Sprint(x)
	}
}

// claimOutlineLine renders one claim hit: the statement, its verbatim evidence,
// and the chunk pointer that opens the surrounding passage.
//
// The line is ANSWER MATERIAL (a claim carries its own evidence, so the model is
// expected to cite it instead of re-reading the document), which makes the
// passage handle mandatory: an uncited claim line ships an answer the reference
// payload cannot resolve. The handle is appended unquoted (`ref=3`) on purpose -
// claim lines are rendered inside an escaped <structure> text node, where a
// `ref="3"` attribute would arrive as `ref=&quot;3&quot;`.
func claimOutlineLine(ctx context.Context, c claimHit) string {
	line := "- [claim] " + c.name
	if q := strings.TrimSpace(c.quote); q != "" {
		line += ` | Evidence: "` + snippetRunes(q, navClaimEvidenceChars) + `"`
	}
	if c.chunkID != "" {
		line += " [chunks: " + c.chunkID + "]"
	}
	if n := evidenceRegistryFrom(ctx).Stamp(c.chunkID); n >= 0 {
		line += " ref=" + strconv.Itoa(n)
	}
	return line
}

// claimCompactLine renders a claim hit as its statement plus the handle to the
// chunk it came from, and nothing else.
//
// It is the line used where claims are re-read on every later call of the turn
// (a locate result's block, the gate in front of a raw read). The quote and the
// chunk id are omitted there because both are carried by the handle: the handle
// resolves to the source chunk, so a quote repeated in the transcript buys
// nothing the model can act on while costing a line's worth of tokens in every
// subsequent call. The verdict page - the one call that decides whether the
// claims are enough - keeps the full form.
func claimCompactLine(ctx context.Context, c claimHit) string {
	line := "- [claim] " + c.name
	if n := evidenceRegistryFrom(ctx).Stamp(c.chunkID); n >= 0 {
		line += " ref=" + strconv.Itoa(n)
	}
	return line
}
