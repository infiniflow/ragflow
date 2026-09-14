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
	"fmt"
	"strings"
	"sync"
)

// The shared accumulation store and the retrieval abstraction.
//
// In Python this is not a harness module: it is the plain dict
// `self.kbinfos = {"chunks": [], "doc_aggs": [], "memory": []}` carried by the
// RAGTools instance in, threaded through retrieval, the
// action session, SCA, and the final answer via tools.kbinfos. The Go port
// reifies that dict into the typed Kbinfos struct (with the Merge helper) and
// keeps it in the harness package because chunkKey/chunkText and Merge are
// unexported helpers shared by tool_search.go, tool_exploration.go, and
// tool_compiled_expansion.go — a reification, not a 1:1 file mirror.
//
// This file also owns the chunk identity helpers (chunkKey/chunkText) that the
// merge logic depends on.

// SearchParams mirrors Python tools/search.py::hybrid_search(tools, query,
// kb_ids, top_n, doc_scope, keywords, retrieval_query, use_compiled).
//
// Question is the search query; Keywords is used ONLY to narrow retrieved
// chunks (never to build the query) unless RetrievalQuery is empty.
type SearchParams struct {
	Question string
	// Keywords narrows retrieved chunks to the sentences that mention them.
	// When RetrievalQuery is empty it also falls back into the query text.
	Keywords string
	// RetrievalQuery is the entity/qualifier-weighted query (entity x3,
	// qualifier x3) appended to the bare Question: a problem-level search over
	// the raw question is exactly where the entity must dominate the ranking.
	RetrievalQuery string
	// UseCompiled enables compiled-structure expansion (page index / tree /
	// mindmap / knowledge graph / wiki synthesis pages).
	UseCompiled bool
	// KbIDs restricts the search to these dataset ids. Empty falls back to the
	// session's bound datasets.
	KbIDs []string
	// DocScope restricts the search to these document ids (e.g. the doc_id
	// routed by navigate_tree).
	DocScope []string
	// TopN is the result count. <= 0 selects the configured default.
	TopN int
	// Channel is retained for backward compatibility with callers that still
	// poke the unified HybridSearch with an explicit channel. It is DEPRECATED:
	// each Python entry point is now its own function (HybridSearch /
	// VectorSearch / BM25Search / GrepSearch / RetrieveSearch) and selects its
	// vector weight, similarity threshold and compiled-row exclusion
	// internally. New code should call the specific function instead of setting
	// Channel on HybridSearch. runSearch does NOT read this field.
	Channel SearchChannel
}

// SearchFn performs one hybrid search and returns chunks + doc aggs, so the
// harness is decoupled from the concrete retrieval backend.
// Mirrors the Python side's settings.retriever.retrieval call.
type SearchFn func(ctx context.Context, p SearchParams) ([]map[string]any, []map[string]any)

// Kbinfos is the shared accumulation store.
//
// ONE Kbinfos is shared by a round's CONCURRENT sessions (SessionDeps.KB; see
// RunSlotResearchPass), so its mutable state is guarded by mu. Python needs no
// lock — its sessions are coroutines and every admission stretch is await-free
// (action_session.py:691-700), so asyncio cannot interleave two sessions'
// batches. Go runs the same sessions on goroutines, so the critical sections
// Python gets for free have to be locked explicitly: see Admit.
type Kbinfos struct {
	mu      sync.Mutex
	Chunks  []map[string]any
	DocAggs []map[string]any
	// Memory is the lossless store of raw retrieved chunks backing the (lossy)
	// Chunks list that feeds the LLM. Mirrors Python kbinfos["memory"], which
	// memory.add/grep maintain.
	Memory []map[string]any
	// PreSummary is the merged claim-report summary produced by the action
	// session; the final-answer call reads it when set.
	PreSummary string
	// cache is the per-request retrieval cache (Python tools.search_cache). It
	// is initialised lazily via cacheOnce so a zero-value Kbinfos is usable.
	cache     *searchCache
	cacheOnce sync.Once
}

// HasChunks mirrors Python `bool(kbinfos.get("chunks"))`.
func (k *Kbinfos) HasChunks() bool { return len(k.Chunks) > 0 }

// Admit runs fn as ONE critical section over the evidence pool.
//
// The granularity is the caller's, and it must span the stretch Python leaves
// await-free: ONE query's candidate batch, because the awaits sit in the OUTER
// query loop (action_session.py:691-700). Locking per chunk would let two
// sessions' batches interleave into pool orders Python cannot produce, and the
// pool order is observable — it drives the rendered prompt order and the `ID: n`
// numbering, extractRelevantEvidence's first-4 pick, and which chunks make it in
// under the cap.
//
// Inside fn do, per chunk, exactly what _admit_evidence does, in its order:
//
//	p.Full()   → skip the chunk. The cap check precedes the per-call dedup
//	             (:638-646), so a rejected chunk is NOT marked seen and is
//	             retried once room frees.
//	then the call-local dedup (`seen`) and payload/ids bookkeeping;
//	p.Add(c)   → dedup against the live pool and append.
func (k *Kbinfos) Admit(fn func(p *PoolAdmitter)) {
	if k == nil {
		// No pool bound: the callback still runs — a tool hands its passages back
		// either way — but nothing is pooled and the cap never blocks, which is
		// what the callers' old `if deps.KB != nil` guard did.
		fn(&PoolAdmitter{})
		return
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	fn(&PoolAdmitter{k: k})
}

// PoolAdmitter is the pool handle, valid only inside Kbinfos.Admit's callback.
type PoolAdmitter struct{ k *Kbinfos }

// Full mirrors the early stop at the top of _admit_evidence: at the cap the
// chunk is skipped, and nothing about it is recorded (so a rejected chunk is
// retried once room frees).
//
// The "pool FULL" line belongs here for the same reason it lives inside
// _admit_evidence (:639-645), and it is emitted once per PROCESS — Python
// documents _EVIDENCE_POOL_STATE as a "Per-process flag" (:62-64). sync.Once
// because concurrent sessions call this under their own pool lock.
func (p *PoolAdmitter) Full() bool {
	if p.k == nil || len(p.k.Chunks) < evidencePoolCap {
		return false
	}
	evidencePoolFullLogged.Do(func() {
		_LOG.Printf("[Action Session] evidence pool FULL (%d chunks >= cap %d); early-stopping admit of further chunks.", len(p.k.Chunks), evidencePoolCap)
	})
	return true
}

// evidencePoolFullLogged mirrors Python _EVIDENCE_POOL_STATE["full_logged"]: a
// PER-PROCESS flag (action_session.py:62-64), so the line is emitted once per
// process — not once per rejected chunk, and NOT once per session. The pool never
// shrinks mid-process, so no reset is needed.
var evidencePoolFullLogged sync.Once

// Add appends c unless the LIVE pool already holds it, reporting whether it
// appended — Python's "new to the shared pool" return.
//
// The test is against the live pool, not a snapshot taken when the call started
// (Python's `kb_seen`, :690). That snapshot is exact only because nothing can
// interleave with it; under Go's parallelism it would re-append a chunk another
// session just pooled, which Python can never do.
//
// There is deliberately NO cap check here: the compiled-expansion path appends
// uncapped in Python too, which is what pushes the pool past _EVIDENCE_POOL_CAP
// (see the comment on evidencePoolCap). Callers that admit user-facing search
// hits check Full() first.
func (p *PoolAdmitter) Add(c map[string]any) bool {
	if p.k == nil {
		return false
	}
	if p.Index(c) >= 0 {
		return false
	}
	p.k.Chunks = append(p.k.Chunks, c)
	return true
}

// Index is the pool position of c's identity, or -1 when the pool lacks it (and
// for an unbound pool): the positional contract of Merge — a deduped chunk still
// reports its position.
func (p *PoolAdmitter) Index(c map[string]any) int {
	if p.k == nil {
		return -1
	}
	return indexOfChunk(p.k.Chunks, chunkKey(c))
}

// ClaimCoveredIDs returns the chunk ids already represented VERBATIM by a claim
// pseudo-chunk in the LIVE pool (Python _claim_covered_ids,
// action_session.py:619-631): a claim carries its own verbatim quote plus the
// ids of the chunks it was distilled from, so admitting those passages again
// is duplicate payload — the answer material is already in the pool at a
// fraction of the size. Call it ONCE per Admit batch and test each chunk's id
// against the result (Python computes the set once per _admit_evidence call).
func (p *PoolAdmitter) ClaimCoveredIDs() map[string]bool {
	if p.k == nil {
		return nil
	}
	covered := map[string]bool{}
	for _, c := range p.k.Chunks {
		if id, ok := c["chunk_id"].(string); !ok || !strings.HasPrefix(id, "claim_") {
			continue
		}
		for _, cid := range toStringList(c["source_chunk_ids"]) {
			if cid != "" {
				covered[cid] = true
			}
		}
	}
	return covered
}

// CoveredByClaim mirrors the _admit_evidence skip (action_session.py:672-676):
// a non-table chunk whose id a pooled claim already quotes verbatim must NOT
// enter the pool again. Table chunks are exempt: their answer rows survive
// only in full text.
func (p *PoolAdmitter) CoveredByClaim(cid string, covered map[string]bool, isTable bool) bool {
	return !isTable && cid != "" && covered[cid]
}

// Merge appends the given chunks/aggs, deduplicating by chunkKey, and returns the
// GLOBAL index (position in k.Chunks) of every contributed chunk — deduped ones
// included.
//
// Mirrors orchestrator/direct.py:_merge_kbinfos, including its early return on an
// empty chunk list (neither chunks nor doc_aggs are merged).
func (k *Kbinfos) Merge(chunks, aggs []map[string]any) []int {
	if len(chunks) == 0 {
		return nil
	}
	added := make([]int, 0, len(chunks))
	// One critical section for the whole merge: the dedup is against the live
	// pool, so a concurrent session cannot slip the same chunk in between.
	k.Admit(func(p *PoolAdmitter) {
		for _, c := range chunks {
			p.Add(c)
			// Deduped chunks get an entry too: the return value is positional.
			added = append(added, p.Index(c))
		}
	})
	k.MergeDocAggs(aggs)
	return added
}

// RetireClaimsCoveredBy removes claim pseudo-chunks whose source_chunk_ids
// intersect the given read ids (Python _exec_list_chunks:928-934): a deep read
// COVERS its claims — once the full chunk text is in the pool, the claim's
// 1200-char quote of the same passage is duplicated tokens in every later
// prompt. The claim already did its job (it pointed here). Returns how many
// entries were retired. The claim_ prefix and the "source_chunk_ids listed
// under the read ids" test mirror Python verbatim.
func (k *Kbinfos) RetireClaimsCoveredBy(readIDs []string) int {
	if k == nil || len(readIDs) == 0 {
		return 0
	}
	read := make(map[string]bool, len(readIDs))
	for _, id := range readIDs {
		if id != "" {
			read[id] = true
		}
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	removed := 0
	out := make([]map[string]any, 0, len(k.Chunks))
	for _, c := range k.Chunks {
		id, ok := c["chunk_id"].(string)
		if ok && strings.HasPrefix(id, "claim_") {
			retired := false
			for _, src := range toStringList(c["source_chunk_ids"]) {
				if read[src] {
					retired = true
					break
				}
			}
			if retired {
				removed++
				continue
			}
		}
		out = append(out, c)
	}
	if removed > 0 {
		k.Chunks = out
	}
	return removed
}

// MergeDocAggs appends doc aggregations, deduplicating by doc_id: the first agg
// per doc_id wins (Python _merge_kbinfos' dseen set).
//
// Separate from Merge, whose early return on an empty chunk list would skip them:
// the search tool must record a search's aggregations even when every chunk it
// returned was already pooled.
func (k *Kbinfos) MergeDocAggs(aggs []map[string]any) {
	if k == nil || len(aggs) == 0 {
		return
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	dseen := make(map[string]bool, len(k.DocAggs)+len(aggs))
	for _, d := range k.DocAggs {
		dseen[docAggKey(d)] = true
	}
	for _, d := range aggs {
		key := docAggKey(d)
		if dseen[key] {
			continue
		}
		dseen[key] = true
		k.DocAggs = append(k.DocAggs, d)
	}
}

// docAggKey mirrors Python's dedup key for doc_aggs: the raw doc_id, with a
// missing/empty doc_id mapping to "" (so only the first such agg is kept).
func docAggKey(d map[string]any) string {
	if id, ok := d["doc_id"].(string); ok {
		return id
	}
	return ""
}

// indexOfChunk returns the global index of the chunk whose key matches kk.
func indexOfChunk(chunks []map[string]any, kk string) int {
	for i, c := range chunks {
		if chunkKey(c) == kk {
			return i
		}
	}
	return -1
}

// chunkText mirrors Python harness/chunk_utils._chunk_text: the searchable
// text of a chunk, preferring the weighted (reranked) "content_with_weight"
// over the raw "content", then falling back to "text". Python's root
// chunk_utils._chunk_text and grep_sed_narrow._chunk_text both read in this
// order; memory._chunk_text is a two-level variant (no "text") and is NOT what
// this mirrors.
func chunkText(c map[string]any) string {
	if t, ok := c["content_with_weight"].(string); ok && t != "" {
		return t
	}
	if t, ok := c["content"].(string); ok && t != "" {
		return t
	}
	if t, ok := c["text"].(string); ok {
		return t
	}
	return ""
}

// chunkKey returns a stable dedup key for a chunk. Prefers chunk_id/id when
// present; otherwise falls back to a content-derived key so chunks without an
// id still dedup correctly (and never collide on a shared empty key).
//
// The content branch reuses chunkText — the SAME alias chain the rest of the
// harness reads (content_with_weight -> content -> text). Reading only the first
// two made two id-less chunks that carry just "text" fall through to the
// doc-level fallback and share one key, so Merge/MemoryAdd discarded distinct
// evidence.
//
// This diverges from Python _chunk_key (`chunk_id or id or id(ck)`): Python's
// final fallback is `id(ck)` — the dict object's memory address — which means
// two equivalent chunks returned as distinct objects never share a key, so
// deduplication silently fails for id-less chunks. Go instead keys on the chunk
// text, so identical content still merges across retrieval calls.
func chunkKey(c map[string]any) string {
	if id, ok := c["chunk_id"].(string); ok && id != "" {
		return "cid:" + id
	}
	if id, ok := c["id"].(string); ok && id != "" {
		return "id:" + id
	}
	content := chunkText(c)
	if content == "" {
		// No stable identity: fall back to the doc reference so at least
		// per-document grouping is preserved (rarely reached).
		content = fmt.Sprintf("%s|%s", anyString(c["doc_id"]), anyString(c["docnm_kwd"]))
	}
	return "h:" + fnv64(content)
}

func anyString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// fnv64 is a deterministic non-crypto hash for dedup keys.
func fnv64(s string) string {
	h := uint64(14695981039346656037)
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return fmt.Sprintf("%016x", h)
}
