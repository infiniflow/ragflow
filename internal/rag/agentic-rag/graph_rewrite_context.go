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
	"strings"

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

// renderResearchContext and memberLinesOf used to live here: they rendered the block the
// gap→query rewriter read (the attempted-query ledger with outcomes, plus the passages behind the
// names the run had already reached). Both went with the rewriter node — with no machine writing
// the next round's queries, there is no reader for that block. The DIRECTION the next round's
// session is handed is what carries the record instead (see graph_slots.go), and the session reads
// the pool through its tools.
