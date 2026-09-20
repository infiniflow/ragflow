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
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/cloudwego/eino/schema"
	"ragflow/internal/rag/agentic-rag/runtime"
)

// The fan-out stage: decompose the question into first-hop sub-questions, parse what
// the model wrote, and fetch them programmatically.
//
// Its parsing is shape-guarded rather than answer-keyed (see fanoutLooksLikeQuery):
// the model is asked for sub-questions and hands back whatever it produced, so what
// a line IS decides what the stage does with it.
// fanoutLooksLikeQuery: reject
// prose/answer lines before they can enter the retrieval + slot pipeline.
//
// Fan-outs are used verbatim as BM25/hybrid queries and as the slot table's
// fanout_hint, so an answered fact ("The woman was **X**") must never survive
// here: it both poisons retrieval and asserts a hallucinated entity as a known
// aspect.
func fanoutLooksLikeQuery(line string, loose bool) bool {
	s := strings.TrimSpace(line)
	if s == "" {
		return false
	}
	low := strings.ToLower(s)
	for _, mark := range fanoutAnswerMarks {
		if strings.Contains(low, mark) {
			return false
		}
	}
	// The cap counts characters, not bytes; a byte-based cap would reject a legitimate CJK
	// fan-out well below the 160-character limit.
	if utf8.RuneCountInString(s) > fanoutMaxChars {
		return false
	}
	words := len(strings.Fields(s))
	if loose && !strings.HasSuffix(s, "?") && words > fanoutLooseMaxWords {
		return false
	}
	return words <= fanoutMaxWords
}

// fanoutLineBreak reports whether r is a line boundary.
// Splitting on "\n" alone misses a lone "\r" and the other Unicode line breaks a model
// could emit, so the loose path would fuse several fan-out lines into one.
func fanoutLineBreak(r rune) bool {
	switch r {
	case '\n', '\r', '\v', '\f', 0x1c, 0x1d, 0x1e, 0x85, 0x2028, 0x2029:
		return true
	}
	return false
}

// parseFanouts: extract fan-outs from a
// model reply, validating every entry's shape.
func parseFanouts(text string) []string {
	var raw []string
	loose := false
	if data, ok := extractJSONObject(text).(map[string]any); ok {
		for _, f := range asSliceOfAny(data["fanouts"]) {
			if s := strings.TrimSpace(fmt.Sprint(f)); s != "" {
				raw = append(raw, s)
			}
		}
	} else {
		// Loose fallback: the model answered in prose. Only lines that still
		// look like a search query are kept — answer sentences and source
		// lists are dropped.
		loose = true
		// Split on every line boundary, not just "\n".
		for _, ln := range strings.FieldsFunc(text, fanoutLineBreak) {
			if strings.TrimSpace(ln) == "" {
				continue
			}
			// The bullet / numbering cutset is stripped from BOTH ends, then whitespace is
			// trimmed, so a trailing period/digit never leaks into the retrieval query.
			raw = append(raw, strings.TrimSpace(strings.Trim(ln, "-•0123456789. ")))
		}
	}
	kept := make([]string, 0, len(raw))
	for _, q := range raw {
		if fanoutLooksLikeQuery(q, loose) {
			kept = append(kept, q)
		}
	}
	if len(raw) > 0 && len(kept) == 0 {
		_LOG.Printf("[Planner] discarding %d fan-out candidate(s): none look like search queries", len(raw))
	}
	kept = dedupe(kept)
	if len(kept) > MaxFanouts {
		kept = kept[:MaxFanouts]
	}
	return kept
}

// ExpandFanouts: ONE chat call (no
// tools) producing 2-5 first-hop fan-outs, plus one strict retry when the
// reply was not parseable JSON (_expand_fanouts).
//
// Falls back to the raw question alone on any failure — a fan-out failure never
// blocks the pipeline.
func ExpandFanouts(ctx context.Context, deps RAGTools, question string) []string {
	if question == "" {
		return nil
	}
	if deps.Model == nil {
		return []string{question}
	}
	reply, err := deps.Model.Complete(ctx, []schema.Message{
		*schema.SystemMessage(fanoutPrompt),
		*schema.UserMessage("Question: " + question),
	}, nil)
	if err != nil {
		_LOG.Printf("[Planner] fan-out expansion failed; falling back to raw question: %v", err)
		return []string{question}
	}
	fanouts := parseFanouts(reply.Content)
	if len(fanouts) == 0 {
		// The model answered the question instead of decomposing it (no JSON,
		// or JSON that failed the shape guard). One strict retry, then give up.
		_LOG.Printf("[Planner] fan-out expansion produced no usable sub-question; retrying with a strict JSON instruction")
		if retry, rerr := deps.Model.Complete(ctx, []schema.Message{
			*schema.SystemMessage(fanoutPrompt + fanoutStrictRetry),
			*schema.UserMessage("Question: " + question),
		}, nil); rerr != nil {
			_LOG.Printf("[Planner] strict fan-out retry failed: %v", rerr)
		} else {
			fanouts = parseFanouts(retry.Content)
		}
	}
	if len(fanouts) == 0 {
		fanouts = []string{question}
	}
	_LOG.Printf("[Planner] fan-out expansion: %d sub-question(s): %v", len(fanouts), fanouts)
	return fanouts
}

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
func FanoutSearch(ctx context.Context, deps RAGTools, st *AgenticState, queries []string, topN int) int {
	if len(queries) == 0 || st.KB == nil {
		return 0
	}
	sd := deps.Search
	if sd.Backend == nil {
		return 0
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
		return 0
	}
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
	type fanoutPair struct {
		exact    []map[string]any
		semantic []map[string]any
	}
	pairs := make([]fanoutPair, len(qs))
	var wg sync.WaitGroup
	for i, q := range qs {
		wg.Add(1)
		go func(i int, q string) {
			defer wg.Done()
			exact, semantic := fanoutSearchQuery(ctx, sd, q, capPerQuery)
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
			return 0
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
	// Channel A across every fan-out first, then channel B.
	for _, p := range pairs {
		admit(p.exact)
	}
	for _, p := range pairs {
		admit(p.semantic)
	}
	return added
}

// fanoutSearchQuery runs one fan-out's two channels. It only retrieves; the
// caller admits the results.
func fanoutSearchQuery(ctx context.Context, sd runtime.SearchDeps, fq string, capPerQuery int) (exact, semantic []map[string]any) {
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
		if len(exact) > capPerQuery {
			exact = exact[:capPerQuery]
		}
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
