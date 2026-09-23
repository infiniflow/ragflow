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
)

// The rewrite context: what a follow-up round needs to know about the round before it.
//
// It turns the round's own RECORD (its unresolved plan slots, its attempt ledger, the passages
// that carry names it confirmed) into something the next round can act on. That is what a
// rewrite is.

// unresolvedClueGaps is the ONLY gap source: the first two question_clues of every unresolved
// slot become (what, hint) gaps. A slot is unresolved because the round said so, not because a
// model judged the answer incomplete.
func unresolvedClueGaps(st *AgenticState) []missingPiece {
	var gaps []missingPiece
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
			gaps = append(gaps, missingPiece{What: qc, SearchHint: qc})
		}
	}
	return gaps
}

// missingPiece is one gap: what is missing, and a hint for searching for it. The DIRECTION the
// next round's session is handed is what reads it (see graph_slots.go); the session writes its
// own queries.
type missingPiece struct {
	What       string
	SearchHint string
}
