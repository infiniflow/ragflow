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
//     the claims answer the question, the turn ends without reading a chunk.
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
	compiledPrefetchClaims = 12
	// compiledNoticeClaims is the claim budget attached to ONE locate result.
	// Smaller than the prefetch budget on purpose: a result set is read
	// alongside its snippets, and the loop may issue several searches.
	compiledNoticeClaims = 6
	// compiledClaimsPerAnchor is the candidate budget per anchored chunk. The
	// lookup is filter-only, so the store returns rows in ITS order, not the
	// retrieval's; a pool smaller than anchors x claims-per-chunk lets the chunks
	// the store happened to list first crowd out the best-ranked anchor entirely.
	// Measured on the FRAMES corpus: 10 anchors carried 138 claim rows, up to 36
	// on a single chunk.
	compiledClaimsPerAnchor = 8
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
		key := strings.ToLower(strings.TrimSpace(c.name))
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
// Only claims that name at least one of these chunks survive: the lookup is
// chunk-anchored, so a claim compiled from some other part of the corpus is not
// evidence for what the search returned, however well it matches the question.
//
// The anchor is the retrieval's rank, so the best-ranked chunk's claims come
// first; within one chunk the claim closest to the question wins. The coverage
// floor orders the page (claims that name the question's own terms lead) rather
// than filtering it: an anchored chunk is relevant even when its claims are
// worded differently, and the caller's verdict is what decides sufficiency.
func compiledClaimsForChunks(ctx context.Context, tenantID string, datasetIDs, chunkIDs []string, question string, limit int) []claimHit {
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
	if len(raw) == 0 {
		return nil
	}
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
		key := strings.ToLower(strings.TrimSpace(h.name))
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
	if len(cands) == 0 {
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

	kept := make([]claimHit, 0, limit)
	for _, c := range cands {
		if c.coverage < navClaimMinCoverage {
			continue
		}
		kept = append(kept, c.hit)
		if len(kept) >= limit {
			return kept
		}
	}
	// Budget left over: fill it with the remaining anchored claims instead of
	// leaving it unspent. A chunk the retrieval ranked highly is relevant even
	// when its claims are worded differently from the question, and the caller
	// asks the model whether they answer it - an empty page would turn a
	// paraphrase miss into "the corpus has no compiled evidence".
	for _, c := range cands {
		if c.coverage >= navClaimMinCoverage {
			continue
		}
		kept = append(kept, c.hit)
		if len(kept) >= limit {
			break
		}
	}
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
	claims := compiledClaimsForChunks(ctx, tenantID, datasetIDs, chunkIDs, question, compiledPrefetchClaims)
	pf := compiledPrefetch{chunks: chunkIDs, claims: claims}
	covered := coveredAnchorChunks(claims)
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
	common.InfoCtx(ctx, "agentic_rag: compiled prefetch ready",
		zap.String("query", question),
		zap.Strings("datasets", datasetIDs),
		zap.Int("chunks", len(chunkIDs)),
		zap.Int("covered_chunks", len(covered)),
		zap.Int("claims", len(claims)),
		zap.Strings("docs", claimDocsInOrder(claims)),
		zap.Int("evidence_bytes", len(pf.evidence)))
	return pf
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
func claimEvidenceBlock(ctx context.Context, claims []claimHit) string {
	var b strings.Builder
	current := ""
	for _, c := range claims {
		if c.docID != current {
			if current != "" {
				b.WriteString("</doc>\n")
			}
			if c.docID != "" {
				fmt.Fprintf(&b, "<doc doc_id=\"%s\">\n", xmlEscape(c.docID))
			}
			current = c.docID
		}
		b.WriteString(claimOutlineLine(ctx, c))
		b.WriteString("\n")
	}
	if current != "" {
		b.WriteString("</doc>\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// compiledEvidenceNotice renders the claims compiled from ONE locate result
// set, as the leading block of that tool result.
//
// It returns "" for a corpus without claims, for a result set that carries no
// corpus chunk (web hits), and for claims this turn has already shown — so a
// non-compiled corpus pays one cached existence probe per turn and nothing
// else.
func compiledEvidenceNotice(ctx context.Context, tool string, hits []snippetHit) string {
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
	claims := compiledClaimsForChunks(ctx, run.tenantID, run.datasetIDs, chunkIDs, queryFrom(ctx), compiledNoticeClaims*3)
	claims = run.takeUnseen(claims, compiledNoticeClaims)
	if len(claims) == 0 {
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
		zap.Int("claims", len(claims)))
	var b strings.Builder
	fmt.Fprintf(&b, "<compiled_evidence count=\"%d\">\n", len(claims))
	b.WriteString("<note>These statements are the compiled artifacts of the chunks below: each one is an atomic fact ")
	b.WriteString("with the verbatim quote it was compiled from, and ref=n is the handle to cite that source chunk by. ")
	b.WriteString("If they already state what the question needs, answer from them now and cite their ref handles - do NOT ")
	b.WriteString("call list_chunks for those chunks. Read a raw chunk only when the fact you need is not stated here.</note>\n")
	b.WriteString(strings.Join(lines, "\n"))
	b.WriteString("\n</compiled_evidence>")
	return b.String()
}
