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

package runtime

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// The shared accumulation store and the retrieval abstraction.
//
// The accumulation store is the typed Kbinfos struct (with the Merge helper), which holds
// the chunks / doc_aggs / memory that retrieval, the action session, SCA and the final answer
// all share. It lives in the runtime package because chunkKey/chunkText and Merge are
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
// runtime is decoupled from the concrete retrieval backend.
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
	// readChunks is the READ ledger: the chunks the run has actually READ (deep-read through
	// list_chunks), as opposed to the ones it has only been SHOWN as a search snippet.
	//
	// The distinction is the difference between a preview and evidence, and it is the model's to
	// act on — a search result is a ranked guess about where the answer might be, and a document
	// page is what that document says. Nothing else in the run can tell them apart once both are
	// in the pool, which is why the pool holds the record: the tool result for every later call
	// marks each passage with which of the two it is (see markReadState), and the session's seed
	// reports how far into each document the run has read.
	//
	// Guarded by mu, like Chunks: the sessions of a round run concurrently, and this is written
	// from inside a tool call and read while a tool result is being rendered.
	readChunks map[string]bool
	// docRead is the per-document progress behind readChunks: how many pages of a document have
	// been delivered, how many distinct chunks, where the last page started, and whether the last
	// page said the document continues.
	docRead map[string]*DocRead
	// Memory is the lossless store of raw retrieved chunks backing the (lossy)
	// Chunks list that feeds the LLM; the memory add/grep helpers maintain it.
	Memory []map[string]any
	// PreSummary is the merged claim-report summary produced by the action
	// session; the final-answer call reads it when set.
	PreSummary string
	// SessionAnswer is the answer the RESEARCH SESSION wrote, in its own words, when it
	// concluded it had read enough (the session is the researcher AND the answerer).
	//
	// It is kept here rather than only on the graph state because the terminal composition runs
	// outside the graph (deps.Finalize) and has no access to it: what the answerer read and what
	// it wrote are both properties of the RUN, and the pool is the run's shared record.
	SessionAnswer string
	// SessionEvidenceRefs is the session's evidence registry in first-seen order (the chunk ids
	// the model was shown as [ID:0], [ID:1], …). When SessionAnswer stands as the answer, THIS
	// list is the citation list — the [ID:n] the model wrote index into it.
	SessionEvidenceRefs []string
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
	// citedChunks are the passages the enumerated items rest on (see NoteCitedChunks).
	// Guarded by ledgerMu.
	citedChunks []string
	// anchoredRefs are the enumerated members with their passages (see NoteAnchoredRefs).
	// Guarded by ledgerMu.
	anchoredRefs []AnchoredRef
	// sufficiencyUnchecked records that the completeness review never ran (see
	// NoteSufficiencyUnchecked). Guarded by ledgerMu.
	sufficiencyUnchecked bool
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
//	the call-local dedup (`seen`) and payload/ids bookkeeping;
//	p.Add(c)   → dedup against the live pool and append.
//
// There is no capacity check to do first: the pool has no ceiling, so nothing is ever
// skipped for want of room (see the note below).
//
// INVARIANT — fn must not call anything that takes k.mu itself (Admit, Merge,
// MergeDocAggs, RetireClaimsCoveredBy…): the mutex
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

// There is deliberately NO storage ceiling on the evidence pool.
//
// The cap that used to live here (evidencePoolCap, 200) was a STORAGE number, but its only
// reachable effect was on the MODEL's evidence: what it refused was the passage that would
// have reached the last members of an enumeration — the ones named late, after the pool
// was already large (see the old comment on evidencePoolCap, which said exactly this and
// then kept the cap anyway). Nothing renders the pool whole any more (the SCA reads a
// ranked view, the session seed injects a bounded digest, the draft is bounded), so the
// ceiling bought no prompt budget and cost evidence.
//
// What it dragged behind it was worse than the ceiling itself: an EXEMPTION (PoolAdmitter
// .Novelty) so a probe's burst could still land past the cap, plus a slack constant bounding
// that exemption, plus a once-per-process "pool FULL" line — three mechanisms and their
// logging, all of them only meaningful while a cap existed. They are gone with it: the pool
// takes what retrieval finds, and what the model READS is decided by the model (progress
// lines on tool results, list_chunks paging), not by a number here.

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

// NoteCitedChunks records the passages the enumerated items rest on (see
// runtime.AnchoredItemChunks): the compose prompt puts them in front of the answer, which has to
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

// AnchoredRef is one enumerated member and the passage it rests on: the name the answer
// states, and the chunk the naming node matched that name to.
type AnchoredRef struct {
	Name    string
	ChunkID string
	// Quote is the words the naming node matched the member to. The compose renders an anchored
	// member's block from THIS rather than from the whole passage: the evidence budget is spent
	// per block, so a ~1200-char chunk per member fits only the first handful of them, and the
	// members past that are members the model cannot see (and so cannot cite).
	Quote string
}

// NoteAnchoredRefs records the members behind the cited passages, in table order.
//
// The naming node has already matched every member to the passage that states its deed, so
// the member→passage pointer is the RUNTIME's to know. The compose reads it back through
// CiteAnchoredMembers to attach a citation to every member the answer states: a model that
// cites only the handful of blocks it happened to read leaves the rest of the list
// unreachable, and no prompt can make it cite a block it never saw (the evidence budget
// admits only the first few whole chunks).
func (k *Kbinfos) NoteAnchoredRefs(refs []AnchoredRef) {
	if k == nil {
		return
	}
	k.ledgerMu.Lock()
	k.anchoredRefs = append([]AnchoredRef(nil), refs...)
	k.ledgerMu.Unlock()
}

// AnchoredRefs is the list recorded by NoteAnchoredRefs, empty when nothing was enumerated.
func (k *Kbinfos) AnchoredRefs() []AnchoredRef {
	if k == nil {
		return nil
	}
	k.ledgerMu.Lock()
	defer k.ledgerMu.Unlock()
	return append([]AnchoredRef(nil), k.anchoredRefs...)
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

// termsNotCarried reports which of terms no chunk's text carries
// (case-insensitively), preserving the caller's order.
//
// It is the coverage half of every "did we find that name?" question in the
// runtime, and it has exactly two callers because there are exactly two sets it
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

// DocRead is how far into ONE document the run has read (see Kbinfos.readChunks).
type DocRead struct {
	// Pages is how many list_chunks pages this document has been read in.
	Pages int
	// Chunks is how many DISTINCT chunks of it have been read.
	Chunks int
	// LastOffset is where the last page started.
	LastOffset int
	// Continues says the last page reported more of the document behind it. False means either
	// the document ended or no page has said it continues.
	Continues bool
}

// NoteChunksRead records a deep read: the page a list_chunks call delivered, and whether the
// document continues behind it. Called by the tool, not by the model — the model's part is that it
// ASKED for the page.
func (k *Kbinfos) NoteChunksRead(docID string, offset int, ids []string, continues bool) {
	if k == nil || len(ids) == 0 {
		return
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.readChunks == nil {
		k.readChunks = map[string]bool{}
	}
	if k.docRead == nil {
		k.docRead = map[string]*DocRead{}
	}
	added := 0
	for _, id := range ids {
		if id == "" || k.readChunks[id] {
			continue
		}
		k.readChunks[id] = true
		added++
	}
	if docID == "" {
		return
	}
	d := k.docRead[docID]
	if d == nil {
		d = &DocRead{}
		k.docRead[docID] = d
	}
	d.Pages++
	d.Chunks += added
	d.LastOffset = offset
	d.Continues = continues
}

// WasRead reports whether the run has READ this chunk (as opposed to only having been shown it as
// a search snippet).
func (k *Kbinfos) WasRead(id string) bool {
	if k == nil || id == "" {
		return false
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.readChunks[id]
}

// ReadProgress renders what the run has read, one document per line, for the session's seed and the
// tool results: "doc id — 2 page(s), 41 chunk(s) read, from offset 30, more behind it".
//
// Sorted by document id so the seed is stable: an unordered map in a prompt changes the prompt (and
// with it the cache and the diff) on every run.
func (k *Kbinfos) ReadProgress() []string {
	if k == nil {
		return nil
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if len(k.docRead) == 0 {
		return nil
	}
	docs := make([]string, 0, len(k.docRead))
	for docID := range k.docRead {
		docs = append(docs, docID)
	}
	sort.Strings(docs)
	out := make([]string, 0, len(docs))
	for _, docID := range docs {
		d := k.docRead[docID]
		more := ""
		if d.Continues {
			more = ", more behind it"
		}
		out = append(out, fmt.Sprintf("%s — %d page(s), %d chunk(s) read, last page from offset %d%s",
			docID, d.Pages, d.Chunks, d.LastOffset, more))
	}
	return out
}

// Add appends c unless the LIVE pool already holds it, reporting whether it appended —
// i.e. whether the chunk was new to the shared pool.
//
// The test is against the live pool, not a snapshot taken when the call started. A snapshot
// would re-append a chunk another session just pooled, which must not happen under
// parallelism.
//
// There is no capacity check here, and no caller does one either: the pool has no ceiling
// (see the note above), so this is the single admission rule — identity, against the live
// contents.
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
// runtime reads (content_with_weight -> content -> text). Reading only the first
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
