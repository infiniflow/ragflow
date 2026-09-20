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
	"fmt"
	"strings"

	"ragflow/internal/rag/agentic-rag/runtime"
	"ragflow/internal/rag/agentic-rag/runtime/orchestrator"
)

// The rewrite context: what a follow-up round needs to know about the round before it.
//
// These three functions used to live in graph_sca.go, next to the review they were written for.
// The review is gone; what it left behind is the part that was never about judging — turning the
// round's own RECORD (its unresolved plan slots, its attempt ledger, the passages that carry
// names it confirmed) into something the next round can act on. That is what a rewrite is.

// unresolvedClueGaps is the gap source: the first two question_clues of every unresolved slot
// become (what, hint) gaps.
//
// It is the ONLY gap source now. The reviewer used to contribute a second one — its own
// sub_queries — and the two were merged (the gaps the review extracted were tried first, because
// they were written against the draft it had just read). With no review, the run's own record is
// what is left, and it is the more conservative of the two: a slot is unresolved because the
// round said so, not because a model judged the answer incomplete.
func unresolvedClueGaps(st *AgenticState) []orchestrator.MissingPiece {
	var gaps []orchestrator.MissingPiece
	for _, us := range st.UnresolvedSlots {
		clues, ok := us["question_clues"].([]string)
		if !ok {
			continue
		}
		for i, qc := range clues {
			if i >= 2 {
				break
			}
			qc = strings.TrimSpace(qc)
			if qc == "" {
				continue
			}
			gaps = append(gaps, orchestrator.MissingPiece{What: qc, SearchHint: qc})
		}
	}
	return gaps
}

// renderResearchContext is the query_rewrite node's context build: the attempted-query ledger
// with outcomes, plus the passages behind members the round already confirmed.
//
// Full VISIBILITY is the point — what was tried (with outcomes), what the pool holds — so the
// rewriter aims at uncovered angles itself instead of being handed a rule for dedupe.
func renderResearchContext(st *AgenticState) string {
	var historyLines []string
	for _, e := range st.Attempted {
		if e == nil {
			continue
		}
		q := truncateRunes(anyString(e["q"]), 120)
		r := anyString(e["r"])
		if r == "" {
			r = "?"
		}
		outcome := "no new passages"
		switch {
		case e["bound"] != nil:
			// A research round's row carries what ITS session bound. There is no
			// per-session "new to the pool" number to report: a round's sessions run
			// concurrently over ONE pool (see RunSlotResearchPass).
			if n, ok := toIntStrict(e["bound"]); ok {
				outcome = fmt.Sprintf("%d passage(s) bound", n)
			}
		default:
			// A prefetch/rewrite row: those DO know how many passages the fetch added.
			if n, ok := toIntStrict(e["new"]); ok && n != 0 {
				outcome = fmt.Sprintf("%d new passage(s)", n)
			}
		}
		historyLines = append(historyLines, fmt.Sprintf("- %s (round %s: %s)", q, r, outcome))
	}
	// The passages behind the members already confirmed come FIRST, because they
	// are the only place the rewriter can see (a) the wording this text uses for
	// the relation and (b) the names that are still missing. Those passages are
	// exactly the windows the seats were narrowed to (see
	// Kbinfos.RecordReachedTerm), so a name inside one of them is readable.
	//
	// The pool-head lines below are the fallback for a round that has confirmed
	// nothing yet. They are FIRST LINES only, so a name in the middle of a chunk
	// is invisible through them — which is why they are the fallback and not the
	// main channel.
	var memberLines []string
	for i, m := range memberLinesOf(st) {
		if i >= MemberWindowMax {
			break
		}
		memberLines = append(memberLines, "- "+m)
	}
	var poolLines []string
	if len(memberLines) == 0 && st.KB != nil {
		for i, c := range st.KB.Chunks {
			if i >= PoolHeadLines {
				break
			}
			// Read `content` FIRST here — unlike the content_with_weight-first order used
			// elsewhere. The pool-head lines are rewriter prompt content, so this order is
			// deliberate.
			first := anyString(c["content"])
			if first == "" {
				first = anyString(c["content_with_weight"])
			}
			if idx := strings.IndexByte(first, '\n'); idx >= 0 {
				first = first[:idx]
			}
			if first = strings.TrimSpace(first); first != "" {
				poolLines = append(poolLines, "- "+truncateRunes(first, 140))
			}
		}
	}
	var parts []string
	if len(historyLines) > 0 {
		parts = append(parts, "Previously searched queries and their outcomes:\n"+strings.Join(historyLines, "\n"))
	}
	if len(memberLines) > 0 {
		parts = append(parts, "Passages that carry names the searches ALREADY confirmed (read them for the wording this text uses for the relation, and for other names they mention — any of those can be asked about directly):\n"+strings.Join(memberLines, "\n"))
	}
	if len(poolLines) > 0 {
		parts = append(parts, "Evidence currently at hand (first lines of top stored snippets):\n"+strings.Join(poolLines, "\n"))
	}
	// The round's own record of what its probes ASKED and never reached. The
	// rewriter reads it for one reason: re-asking a name the corpus already came
	// back empty on is the loop's most common waste, and the productive move from
	// a dead name is a different ANGLE (the act, the relationship, the place),
	// which the rewriter cannot choose unless it knows the name is dead.
	if absent := st.KB.ProbedAbsentTerms(); len(absent) > 0 {
		parts = append(parts, "Terms already asked for and NOT reached by any passage (do NOT re-ask these on their own; ask for the act / relationship / place instead):\n"+strings.Join(absent, "、"))
	}
	return strings.Join(parts, "\n\n")
}

// memberLinesOf renders one line per confirmed member: the name, and the window
// that carries it, looked up in the pool by the id the seat recorded.
//
// It reads the round's own record rather than re-searching anything: a seat IS a
// probe of one individual that came back with a passage, so the pair (name,
// passage) is already established by the time the rewrite runs.
func memberLinesOf(st *AgenticState) []string {
	if st == nil || st.KB == nil {
		return nil
	}
	byID := make(map[string]map[string]any, len(st.KB.Chunks))
	for _, c := range st.KB.Chunks {
		if id := runtime.ChunkIDOf(c); id != "" {
			byID[id] = c
		}
	}
	var out []string
	seen := map[string]bool{}
	for _, m := range st.KB.ReachedTerms() {
		if seen[m.ChunkID] {
			continue
		}
		c, ok := byID[m.ChunkID]
		if !ok {
			continue
		}
		text := strings.TrimSpace(runtime.ChunkTextOf(c))
		if text == "" {
			continue
		}
		seen[m.ChunkID] = true
		out = append(out, fmt.Sprintf("%s: %s", m.Term, truncateRunes(text, MemberWindowChars)))
	}
	return out
}
