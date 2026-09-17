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
// The accumulation store is the typed Kbinfos struct (with the Merge helper), which holds
// the chunks / doc_aggs / memory that retrieval, the action session, SCA and the final answer
// all share. It lives in the harness package because chunkKey/chunkText and Merge are
// unexported helpers shared by tool_search.go, tool_exploration.go, and
// tool_compiled_expansion.go.
//
// This file also owns the chunk identity helpers (chunkKey/chunkText) that the
// merge logic depends on.

// SearchParams are the inputs of one search.
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
	// SkipReachLedger keeps this search's terms OUT of the reach ledger.
	//
	// The ledger is the record's "names you proved reachable and never recorded"
	// list, and it is read as a to-do list, so what enters it must be a NAME the
	// caller probed. The runtime's completeness pass queries ask for the actor and
	// the act words (see RunCompletenessPass), which are not names and are searched
	// by construction: without the flag every entry is a query word, and not one of them
	// is a member the round is actually missing.
	SkipReachLedger bool
	// Channel is retained for backward compatibility with callers that still
	// poke the unified HybridSearch with an explicit channel. It is DEPRECATED:
	// each entry point is now its own function (HybridSearch / VectorSearch /
	// BM25Search / GrepSearch / RetrieveSearch) and selects its vector weight,
	// similarity threshold and compiled-row exclusion internally. New code should
	// call the specific function instead of setting Channel on HybridSearch.
	// runSearch does NOT read this field.
	Channel SearchChannel
}

// SearchFn performs one hybrid search and returns chunks + doc aggs, so the
// harness is decoupled from the concrete retrieval backend.
// It abstracts the concrete retrieval backend.
type SearchFn func(ctx context.Context, p SearchParams) ([]map[string]any, []map[string]any)

// Kbinfos is the shared accumulation store.
//
// ONE Kbinfos is shared by a round's CONCURRENT sessions (SessionDeps.KB; see
// RunSlotResearchPass), so its mutable state is guarded by mu. The sessions run on
// goroutines, so the critical sections have to be locked explicitly: see Admit.
type Kbinfos struct {
	mu      sync.Mutex
	Chunks  []map[string]any
	DocAggs []map[string]any
	// ledgerMu guards the run's SEARCH RECORD (ProbedAbsent / Reached).
	//
	// It is deliberately NOT mu. The pool lock is held across admit batches, and
	// whoever holds it is free to write down what the batch asked and answered —
	// which is exactly what a seat does. Sharing one mutex made that a
	// self-deadlock: Go mutexes are not reentrant, so a record written inside an
	// admit batch parked the goroutine forever on a lock it already held, and the
	// wait ignores context cancellation, so the run never returned.
	//
	// Lock ORDER, when both are needed: mu first, then ledgerMu — never the other
	// way round. Nothing takes mu while holding ledgerMu, and the record methods
	// that run inside an admit batch (see Admit) are the only place the two nest.
	ledgerMu sync.Mutex
	// Memory is the lossless store of raw retrieved chunks backing the (lossy)
	// Chunks list that feeds the LLM; the memory add/grep helpers maintain it.
	Memory []map[string]any
	// PreSummary is the merged claim-report summary produced by the action
	// session; the final-answer call reads it when set.
	PreSummary string
	// Record is the slot table rendered for the ANSWER prompt — the facts the
	// research settled, without the machine fields (strength, terminal type,
	// evidence ids) that belong to the SCA's draft.
	//
	// The two are separate on purpose. A draft is written to be VERIFIED against
	// the passages that produced it; the answer prompt is written to be ANSWERED
	// from. Reusing one as the other hands the answer model the runtime's bookkeeping,
	// and the answer quotes the bookkeeping verbatim:
	// ("slot 1 [entity] … (strength=0.90) [terminal=state, evidence_ids=[…]]").
	Record string
	// novelAdmitted counts the chunks the cap EXEMPTION below has let in beyond
	// evidencePoolCap. It is what keeps the exemption bounded rather than
	// open-ended (see evidencePoolNoveltySlack). Guarded by mu, like Chunks.
	novelAdmitted int
	// ProbedAbsent is the run's record of NAMED terms a probe asked about and
	// nothing reached (see RecordProbedAbsent). The pool holds what was found;
	// this holds what was asked and not found, and the pair is what a rewrite
	// reads before choosing its next angle. Guarded by ledgerMu.
	ProbedAbsent []string
	// Reached is the run's record of NAMED terms a probe asked about and DID
	// reach, each with the pool chunk that carries it (see RecordReachedTerm):
	// the confirmed members, with their evidence. Guarded by ledgerMu.
	Reached []ReachedTerm
	// setDirection records that this run carries a SET/COUNT direction — a
	// question whose answer is a list of members (see MarkSetDirection, which
	// also says why the retrieval executor reads it). Guarded by ledgerMu.
	setDirection bool
	// actWords / actorForms are the direction's declaration (see MarkCoverage). Guarded by ledgerMu.
	actWords   []string
	actorForms []string
	// citedChunks are the passages the enumerated items rest on (see NoteCitedChunks).
	// Guarded by ledgerMu.
	citedChunks []string
	// sufficiencyUnchecked records that the completeness review never ran (see
	// NoteSufficiencyUnchecked). Guarded by ledgerMu.
	sufficiencyUnchecked bool
	// coverageSet is what this QUESTION's enumeration found, and coverageReady records
	// that it ran at all (see StoreCoverageSet): the windows it admits stay in the pool
	// under the same chunk ids, so a later round reuses the same evidence rather than
	// asking the corpus the same operand queries and putting back what is already there.
	// Guarded by ledgerMu.
	coverageSet   CoverageSet
	coverageReady bool
	// CiteChunkIDs is the ordered id list of the chunks the final-answer call
	// rendered as numbered evidence — Python's tools._rag_cite_chunk_ids
	// (agentic_rag_graph.py:870). The renderer puts the passages behind enumerated
	// items first and fills the rest by similarity, capped (citeChunkCap), so this
	// list is NOT Chunks in pool order, and a "[ID:n]" the model writes refers to
	// position n in THIS list — one entry per RENDERED block, so a chunk the
	// renderer skipped (no content) holds no position here either. The chat pipeline
	// resolves citations against it
	// (RunResponse.CiteChunkIDs → HarnessResult.CiteChunkIDs →
	// decorateHarnessAnswer); without it, a marker was resolved against Chunks by
	// position and landed on the wrong chunk — or past the end when the pool held
	// fewer chunks than the cap.
	//
	// Written once per final-answer call, on the goroutine that composes it (the
	// concurrent research slots have already joined), and read by the caller after
	// the call returns — it is not part of the pool lock's protected state.
	CiteChunkIDs []string
	// cache is the per-request retrieval cache. It is initialised lazily via
	// cacheOnce so a zero-value Kbinfos is usable.
	cache     *searchCache
	cacheOnce sync.Once
}

// HasChunks reports whether the pool holds any chunk.
func (k *Kbinfos) HasChunks() bool { return len(k.Chunks) > 0 }

// PoolSize is how many chunks the shared pool holds.
//
// It takes the pool lock, so it must NOT be called from inside an Admit callback
// (that mutex is not reentrant — see Admit's invariant); the record rendering runs
// after the batch for exactly that reason.
func (k *Kbinfos) PoolSize() int {
	if k == nil {
		return 0
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	return len(k.Chunks)
}

// ChunkByID returns the pool chunk carrying this id, or nil.
//
// It takes the pool lock, so it must NOT be called from inside an Admit callback
// (that mutex is not reentrant — see Admit's invariant).
func (k *Kbinfos) ChunkByID(id string) map[string]any {
	if k == nil || id == "" {
		return nil
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	for _, c := range k.Chunks {
		if ChunkIDOf(c) == id {
			return c
		}
	}
	return nil
}

// ChunksFrom returns up to limit chunks starting at from, as a copy of the slice
// header.
//
// It exists so a READER can walk the pool the round has already paid for without
// holding the pool lock (chunks are only ever appended, never rewritten in place,
// so the header it copies is stable). The maps themselves stay shared and must be
// read only. Readers are also why this is a copy of the header and not the pool:
// the pool keeps growing under them while they scan.
func (k *Kbinfos) ChunksFrom(from, limit int) []map[string]any {
	if k == nil || limit <= 0 {
		return nil
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if from < 0 || from >= len(k.Chunks) {
		return nil
	}
	end := min(from+limit, len(k.Chunks))
	return append([]map[string]any(nil), k.Chunks[from:end]...)
}

// Admit runs fn as ONE critical section over the evidence pool.
//
// The granularity is the caller's, and it must span ONE query's candidate batch,
// because the awaits sit in the OUTER query loop. Locking per chunk would let two
// sessions' batches interleave into unsupported pool orders, and the pool order is
// observable — it drives the rendered prompt order and the `ID: n` numbering,
// extractRelevantEvidence's first-4 pick, and which chunks make it in under the cap.
//
// Inside fn do, per chunk, exactly the admission steps, in this order:
//
//	p.Full()   → skip the chunk. The cap check precedes the per-call dedup
//	             (:638-646), so a rejected chunk is NOT marked seen and is
//	             retried once room frees.
//	then the call-local dedup (`seen`) and payload/ids bookkeeping;
//	p.Add(c)   → dedup against the live pool and append.
//
// INVARIANT — fn must not call anything that takes k.mu itself (Admit, Merge,
// MergeDocAggs, RetireClaimsCoveredBy, PoolAdmitter.Full's siblings…): the mutex
// is NOT reentrant, so that call parks the goroutine forever on a lock it already
// holds, and a mutex wait ignores context cancellation, so the run never returns and
// nothing is logged: the goroutine parks silently.
//
// What fn MAY call is the search record (RecordReachedTerm / RecordProbedAbsent):
// those live behind ledgerMu, precisely so the batch that answered a probe can
// write the answer down while the pool is still held.
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

// Full is the early stop at the top of admission: at the cap the chunk is skipped, and
// nothing about it is recorded (so a rejected chunk is retried once room frees).
//
// The "pool FULL" line belongs here, and it is emitted once per PROCESS. sync.Once
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

// evidencePoolFullLogged: ["full_logged"]: a
// PER-PROCESS flag, so the line is emitted once per
// process — not once per rejected chunk, and NOT once per session. The pool never
// shrinks mid-process, so no reset is needed.
var evidencePoolFullLogged sync.Once

// evidencePoolNoveltySlack bounds the cap EXEMPTION: the pool may pass
// evidencePoolCap by at most this many chunks, and not one more.
//
// The cap is a STORAGE number, and it is the right number for evidence a later
// search could find again. A PROBE's result is not that kind of evidence: a
// batch probe ("荀正|管亥|车胄") is the model's question "does each of these exist,
// and where?", and the per-name window it returns is the only place that answer
// ever lives — no later search can reconstruct WHICH names came back empty.
//
// A batch probe can come back with passages for some of the names it asked about and none
// for others the corpus does hold, so a cap that drops those windows converts a successful
// probe into "nothing new", which the model then reads as "not a member".
//
// The slack is what keeps the exemption from being open-ended: it is charged per
// exempted chunk, and beyond cap+slack even a novel term is refused.
const evidencePoolNoveltySlack = 40

// Novelty is the cap exemption for ONE admit batch: the batch's own terms that
// the LIVE pool cannot answer yet.
//
// Build it once per batch (like ClaimCoveredIDs), then ask it per candidate:
// a candidate that carries one of the absent terms may enter an ALREADY-FULL
// pool, and that term is then consumed — one seat per term, so the pool grows by
// at most one chunk per unanswered probe term. A batch with no terms (or a
// batch whose terms the pool already carries) gets no exemption at all, which is
// exactly the pre-existing cap behaviour.
type Novelty struct {
	p      *PoolAdmitter
	absent []string
}

// Novelty derives the batch's exemption from the terms the call asked for.
//
// Callers pass the terms of a PROBE (the model's own alternation, see
// probeTerms in tool_executor.go) and nothing else: a topic query states no
// list of individuals, so its hits get no exemption and the cap behaves as it
// always did.
//
// The pool-wide scan happens ONCE here, over lowered pool text, rather than once
// per candidate.
func (p *PoolAdmitter) Novelty(terms []string) *Novelty {
	n := &Novelty{p: p}
	if p.k == nil {
		return n
	}
	asked := make([]string, 0, len(terms))
	for _, t := range terms {
		if t = strings.TrimSpace(t); t == "" || len(asked) >= GrepTermsMax {
			continue
		}
		asked = append(asked, t)
	}
	n.absent = termsNotCarried(p.k.Chunks, asked)
	return n
}

// ProbedAbsentTerms returns a copy of the named terms nothing reached, in the
// order they were first probed.
func (k *Kbinfos) ProbedAbsentTerms() []string {
	if k == nil {
		return nil
	}
	k.ledgerMu.Lock()
	defer k.ledgerMu.Unlock()
	return append([]string(nil), k.ProbedAbsent...)
}

// ReachedTerm is one confirmed member: the term a probe asked about, and the pool
// chunk that carries it.
type ReachedTerm struct {
	Term    string
	ChunkID string
}

// RecordReachedTerm notes a named term that a probe asked about AND came back
// with, together with the pool chunk that carries it.
//
// It is the mirror of RecordProbedAbsent, and the two together are the run's
// record of its own search: this one is the CONFIRMED MEMBERS, each with the
// passage that proves it. Two things read it:
//
//   - the rewrite context, which shows the rewriter the passage behind each
//     confirmed member — the wording this text uses for the relation is in those
//     passages, and so are the names that are still missing;
//   - the loop, which can then ask whether the LIST is still growing rather than
//     whether the pool got bigger (a pool that grew by forty passages of the same
//     famous scene has learned nothing about the list).
func (k *Kbinfos) RecordReachedTerm(term, chunkID string) {
	term = strings.TrimSpace(term)
	if k == nil || term == "" || chunkID == "" {
		return
	}
	k.ledgerMu.Lock()
	defer k.ledgerMu.Unlock()
	for _, seen := range k.Reached {
		if strings.EqualFold(seen.Term, term) {
			return
		}
	}
	if len(k.Reached) >= reachedTermsMax {
		return
	}
	k.Reached = append(k.Reached, ReachedTerm{Term: term, ChunkID: chunkID})
}

// MarkSetDirection records that this run carries a SET/COUNT direction.
//
// It is set by the SAME gate that hands the model the set method — the declared
// shape of the direction's table, or the first batch the caller writes, whichever
// comes first — and read by the retrieval executor, which bounds how many of the
// caller's queries run per call:
//
// on a direction whose answer is a LIST, the caller's queries are facets of that
// list rather than rephrasings of one question, so a query that is dropped is not
// a spared repeat — it is a member nobody searched, and the names the answer turns out to
// be missing were often named only in a dropped one. On a VALUE direction the queries ARE
// rephrasings of one question and the small cap stays.
func (k *Kbinfos) MarkSetDirection() {
	if k == nil {
		return
	}
	k.ledgerMu.Lock()
	k.setDirection = true
	k.ledgerMu.Unlock()
}

// IsSetDirection reports whether a SET/COUNT direction has declared itself on this
// run (see MarkSetDirection). Safe on a nil pool: a run with no pool is not one.
func (k *Kbinfos) IsSetDirection() bool {
	if k == nil {
		return false
	}
	k.ledgerMu.Lock()
	defer k.ledgerMu.Unlock()
	return k.setDirection
}

// NoteSufficiencyUnchecked records that the review which judges completeness never ran, so the
// record the answer reads can say so. It is not a verdict, and not a reason to mark the answer
// partial.
func (k *Kbinfos) NoteSufficiencyUnchecked() {
	if k == nil {
		return
	}
	k.ledgerMu.Lock()
	k.sufficiencyUnchecked = true
	k.ledgerMu.Unlock()
}

// SufficiencyUnchecked reports whether the review that would have judged completeness
// never ran (see NoteSufficiencyUnchecked). Safe on a nil pool.
func (k *Kbinfos) SufficiencyUnchecked() bool {
	if k == nil {
		return false
	}
	k.ledgerMu.Lock()
	defer k.ledgerMu.Unlock()
	return k.sufficiencyUnchecked
}

// MarkCoverage records the direction's declaration — the actor's forms and the act words — for the
// readers that have the pool but not the slot table (see CoverageDecl). Where StoreCoverageSet
// holds what the corpus answered, this holds what the run was asking with.
func (k *Kbinfos) MarkCoverage(cov Coverage) {
	if k == nil {
		return
	}
	k.ledgerMu.Lock()
	defer k.ledgerMu.Unlock()
	k.actWords = append([]string(nil), cov.Acts...)
	k.actorForms = cov.Actors()
}

// CoverageDecl is the direction's declaration (see MarkCoverage). Empty when no enumeration
// direction declared itself, and its probes are read by shape alone.
func (k *Kbinfos) CoverageDecl() (acts, actors []string) {
	if k == nil {
		return nil, nil
	}
	k.ledgerMu.Lock()
	defer k.ledgerMu.Unlock()
	return append([]string(nil), k.actWords...), append([]string(nil), k.actorForms...)
}

// NoteCitedChunks records the passages the enumerated items rest on (see
// harness.AnchoredItemChunks): the compose prompt puts them in front of the answer, which has to
// cite one passage per element. Guarded by ledgerMu.
func (k *Kbinfos) NoteCitedChunks(ids []string) {
	if k == nil {
		return
	}
	k.ledgerMu.Lock()
	k.citedChunks = append([]string(nil), ids...)
	k.ledgerMu.Unlock()
}

// CitedChunks is the passages recorded by NoteCitedChunks, empty when no enumeration wrote items.
func (k *Kbinfos) CitedChunks() []string {
	if k == nil {
		return nil
	}
	k.ledgerMu.Lock()
	defer k.ledgerMu.Unlock()
	return append([]string(nil), k.citedChunks...)
}

// StoreCoverageSet records what this question's enumeration found (see
// EnumerateCoverage / CoverageSet).
func (k *Kbinfos) StoreCoverageSet(set CoverageSet) {
	if k == nil {
		return
	}
	k.ledgerMu.Lock()
	k.coverageSet = set
	k.coverageReady = true
	k.ledgerMu.Unlock()
}

// CoverageSet returns the enumeration a previous round ran for this question, and whether
// it ran at all.
//
// The unit is the REQUEST, not the round: the windows the enumeration admitted are in the
// pool under the same chunk ids, so a second round asking the corpus the same operand
// queries would spend the same store legs to re-admit what is already there, and the seed
// would show the same windows it already showed (see StoreCoverageSet). The SET is kept
// rather than its rendering because the last node resolves the windows themselves.
func (k *Kbinfos) CoverageSet() (CoverageSet, bool) {
	if k == nil {
		return CoverageSet{}, false
	}
	k.ledgerMu.Lock()
	defer k.ledgerMu.Unlock()
	return k.coverageSet, k.coverageReady
}

// ReachedTerms returns a copy of the confirmed members and the chunk that carries
// each.
func (k *Kbinfos) ReachedTerms() []ReachedTerm {
	if k == nil {
		return nil
	}
	k.ledgerMu.Lock()
	defer k.ledgerMu.Unlock()
	return append([]ReachedTerm(nil), k.Reached...)
}

// reachedTermsMax bounds the confirmed-member ledger (see RecordReachedTerm):
// it is rendered into the rewrite context, so it may not grow with the number of
// probes a long round happens to make.
const reachedTermsMax = 24

// EvidencePoolCap is the shared pool's ceiling. It is exported so that a caller
// sizing the room for its own admissions (the rewrite round's prefetch) measures
// it against the SAME number the admitter enforces.
//
// Measuring against a smaller constant is how a round came to report "retrieval
// saturated" while the pool still had room to take evidence: the prefetch's room
// was computed against the snippet-pool ceiling (60) while the pool itself holds
// up to twice that, so on any rich round the rewrite's own queries had nowhere to
// land and the round discarded itself.
func EvidencePoolCap() int { return evidencePoolCap }

// termsNotCarried reports which of terms no chunk's text carries
// (case-insensitively), preserving the caller's order.
//
// It is the coverage half of every "did we find that name?" question in the
// harness, and it has exactly two callers because there are exactly two sets it
// can be asked about:
//
//   - a SEARCH's own results — which of the terms this call named did this
//     retrieval not reach? (the seat pass in tool_executor.go, which then buys
//     each unreached term its own search), and
//   - the POOL — which named terms can the pool not answer at all? (Novelty,
//     which exempts those passages from the cap).
func termsNotCarried(chunks []map[string]any, terms []string) []string {
	_, _, absent := termReach(chunks, terms)
	return absent
}

// termReach answers, per term, HOW MANY chunks carry it — the reach of one
// retrieval, term by term.
//
// It is the fact a caller cannot read off a result set: every name in a batch
// looks the same whether the search found it or never looked, and the wording
// matters, because "not reached by THIS query" is not "absent from the corpus".
// A model that reads a miss as absence stops enumerating.
//
// located/counts are parallel and in the caller's term order; absent holds the
// terms no chunk carried.
func termReach(chunks []map[string]any, terms []string) (located []string, counts []int, absent []string) {
	if len(terms) == 0 {
		return nil, nil, nil
	}
	texts := make([]string, 0, len(chunks))
	for _, c := range chunks {
		if text := strings.ToLower(ChunkTextOf(c)); text != "" {
			texts = append(texts, text)
		}
	}
	for _, term := range terms {
		low := strings.ToLower(strings.TrimSpace(term))
		if low == "" {
			continue
		}
		n := 0
		for _, text := range texts {
			if strings.Contains(text, low) {
				n++
			}
		}
		if n == 0 {
			absent = append(absent, term)
			continue
		}
		located = append(located, term)
		counts = append(counts, n)
	}
	return located, counts, absent
}

// RecordProbedAbsent notes a named term that a probe asked about and nothing
// reached, deduped, under the pool lock.
//
// The pool holds what was FOUND; ProbedAbsent holds what was ASKED and not
// found. Together they are the round's record of its own search, which is the
// fact a rewrite needs: re-asking a name the corpus already came back empty on
// is the loop's most common waste, and "these came back empty" is also what
// tells the model to change the ANGLE (ask for the act instead of the name)
// rather than to ask the same thing louder.
func (k *Kbinfos) RecordProbedAbsent(term string) {
	term = strings.TrimSpace(term)
	if k == nil || term == "" {
		return
	}
	k.ledgerMu.Lock()
	defer k.ledgerMu.Unlock()
	for _, seen := range k.ProbedAbsent {
		if strings.EqualFold(seen, term) {
			return
		}
	}
	if len(k.ProbedAbsent) >= probedAbsentMax {
		// Bounded storage: the record is carried into the rewrite context, so it
		// may not grow with the number of probes a long round happens to make.
		return
	}
	k.ProbedAbsent = append(k.ProbedAbsent, term)
}

// probedAbsentMax bounds ProbedAbsent (see RecordProbedAbsent).
const probedAbsentMax = 16

// Admits reports whether c may enter the pool although the pool is FULL, because
// c is the first evidence for one of the batch's unanswered terms.
//
// It answers only the novelty half of the admit decision: the caller still
// checks Full() first, so a pool under the cap never consults this and behaves
// as before. A nil Novelty (or one built from no terms) admits nothing.
func (n *Novelty) Admits(c map[string]any) bool {
	if n == nil || n.p == nil || n.p.k == nil || len(n.absent) == 0 {
		return false
	}
	// The exemption is bounded: past cap+slack the probe's windows are refused
	// like everything else, so a runaway enumeration cannot grow storage without
	// limit.
	if len(n.p.k.Chunks) >= evidencePoolCap+evidencePoolNoveltySlack {
		return false
	}
	text := strings.ToLower(ChunkTextOf(c))
	if text == "" {
		return false
	}
	for i, t := range n.absent {
		if !strings.Contains(text, strings.ToLower(t)) {
			continue
		}
		// ONE seat per term: this passage answers it, so the next passage
		// carrying only that term is refused again.
		n.absent = append(n.absent[:i], n.absent[i+1:]...)
		n.p.k.novelAdmitted++
		_LOG.Printf("[Action Session] pool at the cap %d but the batch asked about %q; admitting the passage that reaches it (pool %d, novelty slack %d/%d).",
			evidencePoolCap, t, len(n.p.k.Chunks)+1, n.p.k.novelAdmitted, evidencePoolNoveltySlack)
		return true
	}
	return false
}

// Add appends c unless the LIVE pool already holds it, reporting whether it appended —
// i.e. whether the chunk was new to the shared pool.
//
// The test is against the live pool, not a snapshot taken when the call started. A snapshot
// would re-append a chunk another session just pooled, which must not happen under
// parallelism.
//
// There is deliberately NO cap check here: the compiled-expansion path appends uncapped,
// which is what pushes the pool past evidencePoolCap (see the comment on evidencePoolCap).
// Callers that admit user-facing search
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

// ClaimCoveredIDs returns the chunk ids already represented VERBATIM by a claim pseudo-chunk
// in the LIVE pool: a claim carries its own verbatim quote plus the ids of the chunks it was
// distilled from, so admitting those passages again is duplicate payload — the answer material
// is already in the pool at a fraction of the size. Call it ONCE per Admit batch and test each
// chunk's id against the result (the set is computed once per admission call).
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

// CoveredByClaim mirrors the _admit_evidence skip:
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
// intersect the given read ids: a deep read
// COVERS its claims — once the full chunk text is in the pool, the claim's
// 1200-char quote of the same passage is duplicated tokens in every later
// prompt. The claim already did its job (it pointed here). Returns how many
// entries were retired. The claim_ prefix and the "source_chunk_ids listed
// under the read ids" test are exact.
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
// per doc_id wins.
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

// docAggKey is the dedup key for doc_aggs: the raw doc_id, with a missing/empty doc_id
// mapping to "" (so only the first such agg is kept).
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

// chunkText is the searchable text of a chunk, preferring the weighted (reranked)
// "content_with_weight" over the raw "content", then falling back to "text". Both the
// chunk-utils and grep-sed-narrow readers use this order; the memory variant is two-level
// (no "text") and is deliberately NOT what this mirrors.
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
// A memory-address fallback is the alternative: with `id(ck)` as the final key, two
// equivalent chunks returned as distinct objects never share a key, so deduplication silently
// fails for id-less chunks. Keying on the chunk text instead means identical content still
// merges across retrieval calls.
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
