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

import (
	"slices"
	"testing"
)

// b1Text is the single long paragraph from parity case
// token__b1_count_sensitive. With no effective delimiter and a small
// chunk_token_size, a live tokenizer splits it into many chunks, while a
// dead (zero-counting) tokenizer collapses it into one. These two tests
// pin both behaviours so the running-sum alignment in splitOversizedUnitWith
// (token.go) stays byte-identical to Python rag/nlp._split_oversized_unit
// and a silently dead tokenizer is caught.
const b1Text = "RAGFlow is a retrieval augmented generation engine that ingests heterogeneous documents and slices them into retrievable passages. The chunker must respect a token budget so that each passage fits the context window of the downstream language model without truncation. Token counting is the linchpin of this budget because every merge decision reads the per segment token length. When the encoder is missing the chunker silently reports zero tokens for every string and the budget is never exceeded so an entire document collapses into one oversized chunk. That degenerate behaviour is invisible to a test that only checks for a non empty result because zero is a number like any other. The only way to notice is to compare the actual chunk count against a reference implementation that counts tokens correctly. This case pins that comparison with a long single paragraph and a deliberately small budget so the correct count yields many chunks and a dead encoder yields exactly one. Retrieval quality depends on passages being coherent and appropriately sized so the guard is not merely cosmetic but protects the core promise of the system."

// TestSplitOversizedUnitRunningSumMatchesPython pins the whitespace-atom
// sub-split of splitOversizedUnitWith against Python's
// rag/nlp._split_oversized_unit. Go now uses the same running-sum flush
// (token.go: currentTokens+aTokens > budget), so the emitted pieces must
// match Python exactly. This is the positive guard that compensates the
// slack=1 relaxation in token_strict_cap_test.go: the oversized unit is
// sub-split on the same boundaries Python uses, not merely kept under a
// loose cap.
func TestSplitOversizedUnitRunningSumMatchesPython(t *testing.T) {
	const budget = 50
	got := splitOversizedUnitWith(b1Text, budget, tokenizeStr)

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

// TestSplitOversizedUnitDeadTokenizerCollapses guards against a silently
// dead tokenizer. With a zero-counting function the running-sum flush never
// triggers, so the whole paragraph collapses into exactly one chunk —
// diverging from the live multi-chunk baseline above. A test that only
// checks for a non-empty result would miss this, so we assert the exact
// collapse count.
func TestSplitOversizedUnitDeadTokenizerCollapses(t *testing.T) {
	const budget = 50
	got := splitOversizedUnitWith(b1Text, budget, func(string) int { return 0 })
	if len(got) != 1 {
		t.Fatalf("dead tokenizer must collapse paragraph into exactly one chunk, got %d", len(got))
	}
}
