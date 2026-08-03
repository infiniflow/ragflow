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

package chunker

// TestSplitOversizedUnitMatchesPython pins the whitespace-atom sub-split
// fallback of splitOversizedUnitWith against Python's rag/nlp._split_oversized_unit.
//
// Both ports tokenise identically, but they historically used different fit
// checks: Python accumulates a running sum of per-atom token counts and flushes
// when current_tokens + a_tokens > budget, whereas Go used the exact token
// count of the joined string (countFn(current+atom) > budget). Because cl100k
// token counting is not additive across whitespace joins, the two formulas
// disagree by one atom at the boundary, so Go and Python produced the same
// chunk count but shifted boundaries. The exact oracle below was produced by
// Python's _split_oversized_unit on the B1 case text and must match exactly.
import (
	"slices"
	"testing"
)

func TestSplitOversizedUnitMatchesPython(t *testing.T) {
	tc := loadCase(t, "testdata/parity/cases/token__b1_count_sensitive.json")
	text, _ := tc.Input["text"].(string)
	budget := int(tc.Param["chunk_token_size"].(float64))

	got := splitOversizedUnitWith(text, budget, tokenizeStr)

	want := []string{
		"RAGFlow is a retrieval augmented generation engine that ingests heterogeneous documents and slices them into retrievable passages. The chunker must respect a token budget so that each passage fits the context window of the ",
		"downstream language model without truncation. Token counting is the linchpin of this budget because every merge decision reads the per segment token length. When the encoder is missing the chunker silently reports zero tokens for every string and the budget ",
		"is never exceeded so an entire document collapses into one oversized chunk. That degenerate behaviour is invisible to a test that only checks for a non empty result because zero is a number like any other. The only way to notice is ",
		"to compare the actual chunk count against a reference implementation that counts tokens correctly. This case pins that comparison with a long single paragraph and a deliberately small budget so the correct count yields many chunks and a dead encoder yields exactly ",
		"one. Retrieval quality depends on passages being coherent and appropriately sized so the guard is not merely cosmetic but protects the core promise of the system.",
	}

	if !slices.Equal(got, want) {
		t.Errorf("splitOversizedUnitWith diverges from Python _split_oversized_unit: got %d pieces, want %d", len(got), len(want))
		for i := 0; i < max(len(got), len(want)); i++ {
			if i >= len(got) || i >= len(want) || got[i] != want[i] {
				gt, wt := "", ""
				if i < len(got) {
					gt = got[i]
				}
				if i < len(want) {
					wt = want[i]
				}
				t.Errorf("piece[%d]:\n  got:  %q\n  want: %q", i, gt, wt)
			}
		}
	}
}
