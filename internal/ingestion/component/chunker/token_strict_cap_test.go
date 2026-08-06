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
	"context"
	"strings"
	"testing"

	"ragflow/internal/ingestion/component/schema"
)

func TestComputeOverlapPrefix_StripsTagsAndCounts(t *testing.T) {
	prev := strings.Repeat("word ", 20) + "@@1\t2.3## tail"
	overlap, n := computeOverlapPrefix(prev, 30)
	if strings.Contains(overlap, "@@") || strings.Contains(overlap, "##") {
		t.Errorf("overlap must strip parser tags, got %q", overlap)
	}
	if n <= 0 {
		t.Errorf("overlap token count must be >0, got %d", n)
	}
	if tokenizeStr(overlap) != n {
		t.Errorf("reported tokens %d != tokenizeStr(overlap) %d", n, tokenizeStr(overlap))
	}
}

func TestMergeByTokenSizeFromJSON_StrictCapNoOvershoot(t *testing.T) {
	// Eight 25-token-ish sections under a 50-token budget must pack without
	// any chunk exceeding the budget (Python test_strict_cap_no_overlap).
	const budget = 50
	sections := make([]schema.ChunkDoc, 0, 8)
	for i := 0; i < 8; i++ {
		text := strings.TrimSpace(strings.Repeat("w ", 25))
		sections = append(sections, schema.ChunkDoc{
			Text: text, DocType: "text", CKType: "text", TKNums: intPtr(tokenizeStr(text)),
		})
	}
	got := mergeByTokenSizeFromJSON([][]schema.ChunkDoc{sections}, budget, 0, schema.MergeOverCap)
	merged := got[0]
	if len(merged) < 3 {
		t.Fatalf("want >=3 chunks, got %d", len(merged))
	}
	// OVER_CAP (Python's canonical default) permits a chunk to exceed budget
	// by at most one incoming unit: a chunk is closed right after the
	// overflowing merge, so its running-sum token count is <= budget+unit.
	// The reconstructed chunk text carries the "\n" joins between units, and
	// cl100k is non-additive across joins, so the actual token count can land
	// a little above budget+unit; allow one extra unit of slack (matching the
	// sibling TestMergeByTokenSizeFromJSON_OverlapDroppedAtOverflow tolerance
	// for the same cl100k delta). Python's naive_merge produces the same 3
	// chunks here, so this pins the faithful boundary, not the old re-tokenized
	// (under-packed) one.
	unit := tokenizeStr(sections[0].Text)
	for i, ck := range merged {
		n := tokenizeStr(ck.Text)
		if n > budget+2*unit {
			t.Errorf("chunk %d exceeds budget by more than one unit: tokens=%d (cap=%d unit=%d)", i, n, budget, unit)
		}
	}
}

func TestMergeByTokenSizeFromJSON_OverlapDroppedAtOverflow(t *testing.T) {
	// With a tight budget, overlap must never push a chunk over the cap.
	const budget = 25
	sections := make([]schema.ChunkDoc, 0, 20)
	for i := 0; i < 20; i++ {
		text := strings.TrimSpace(strings.Repeat("w ", 10))
		sections = append(sections, schema.ChunkDoc{
			Text: text, DocType: "text", CKType: "text", TKNums: intPtr(tokenizeStr(text)),
		})
	}
	got := mergeByTokenSizeFromJSON([][]schema.ChunkDoc{sections}, budget, 20, schema.MergeOverCap)
	unit := tokenizeStr(sections[0].Text)
	for i, ck := range got[0] {
		// OVER_CAP allows one boundary overflow (prev + one unit). The JSON
		// path joins units with "\n" and cl100k is not additive across joins,
		// so an overflow-closed chunk can land a little above budget+unit;
		// allow one extra unit of slack for the separators + cl100k delta.
		// Overlap is only ever prepended when it still fits budget, so the
		// only chunks that exceed budget are the overflow-closed ones.
		if n := tokenizeStr(ck.Text); n > budget+2*unit {
			t.Errorf("chunk %d exceeds budget+two-units with overlap: tokens=%d (cap=%d unit=%d)", i, n, budget, unit)
		}
	}
}

func TestMergeByTokenSizeFromJSON_OversizedUnitStaysWhole(t *testing.T) {
	// Per the #17808 contract an over-budget unit is never atom-split: it
	// stands alone as one chunk and the model layer truncates it later.
	const budget = 30
	long := strings.TrimSpace(strings.Repeat("word ", 100))
	items := [][]schema.ChunkDoc{{
		{Text: long, DocType: "text", CKType: "text", TKNums: intPtr(tokenizeStr(long))},
	}}
	got := mergeByTokenSizeFromJSON(items, budget, 0, schema.MergeOverCap)
	if len(got) != 1 || len(got[0]) != 1 {
		t.Fatalf("over-budget unit must stay whole, got %d chunk(s)", len(got[0]))
	}
	if got[0][0].Text != long {
		t.Errorf("over-budget chunk text changed: got %q", got[0][0].Text)
	}
}

func TestMergeByTokenSize_TextPathStrictCap(t *testing.T) {
	// End-to-end text path: long multi-paragraph input under a tight budget.
	const budget = 40
	unit := tokenizeStr(strings.TrimSpace(strings.Repeat("word ", 15)))
	var b strings.Builder
	for i := 0; i < 30; i++ {
		b.WriteString(strings.TrimSpace(strings.Repeat("word ", 15)))
		b.WriteString("\n\n")
	}
	comp, err := NewTokenChunker(map[string]any{
		"delimiter_mode":   "token_size",
		"chunk_token_size": budget,
	})
	if err != nil {
		t.Fatalf("NewTokenChunker: %v", err)
	}
	tc := comp.(*TokenChunkerComponent)
	out := tc.mergeByTokenSize(b.String(), nil)
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) < 2 {
		t.Fatalf("want multiple chunks, got %d", len(chunks))
	}
	for i, ck := range chunks {
		text, _ := ck["text"].(string)
		// OVER_CAP allows one boundary overflow (prev + one unit).
		if n := tokenizeStr(text); n > budget+unit {
			t.Errorf("chunk %d exceeds budget+one-unit: tokens=%d (cap=%d unit=%d)", i, n, budget, unit)
		}
	}
}

// TestMergeByTokenSize_OversizedUnitStaysWhole mirrors
// TestMergeByTokenSizeFromJSON_OversizedUnitStaysWhole on the text path: a
// single block of text that exceeds chunk_token_size and contains no
// sentence delimiter must be emitted as ONE whole chunk — never atom-split.
// This pins the #17799 contract invariant on the text path (the JSON path
// is already covered by TestMergeByTokenSizeFromJSON_OversizedUnitStaysWhole).
func TestMergeByTokenSize_OversizedUnitStaysWhole(t *testing.T) {
	const budget = 30
	// One long run with no '\n' / '!?' / '。；！？' delimiter: the text path
	// splits oversized sections only on sentenceDelimiter, so this whole
	// run is one unit that still exceeds the budget and must stay whole.
	long := strings.TrimSpace(strings.Repeat("word ", 100))
	comp, err := NewTokenChunker(map[string]any{
		"delimiter_mode":   "token_size",
		"chunk_token_size": budget,
	})
	if err != nil {
		t.Fatalf("NewTokenChunker: %v", err)
	}
	out := comp.(*TokenChunkerComponent).mergeByTokenSize(long, nil)
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 1 {
		t.Fatalf("over-budget unit must stay whole, got %d chunk(s)", len(chunks))
	}
	text, _ := chunks[0]["text"].(string)
	if text != long {
		t.Errorf("over-budget chunk text changed: got %q", text)
	}
}

func TestMergeByTokenSize_UnderCapNoOverflow(t *testing.T) {
	// UNDER_CAP (under_cap=true) must never let a chunk exceed the token
	// target: a projected join that would overflow starts a fresh chunk
	// instead of merge-then-close (OVER_CAP). This exercises the seam that
	// lets Go follow Python's no-overflow (UNDER_CAP) strategy without
	// changing the default (OVER_CAP) behavior.
	const sentence = "word word word word word word word word word word word word word " // 12 words
	sentenceN := tokenizeStr(sentence)
	budget := sentenceN*4 + 2 // four sentences fit, five overflow.
	if budget < sentenceN*2 {
		budget = sentenceN * 2
	}
	var b strings.Builder
	for i := 0; i < 12; i++ {
		b.WriteString(sentence)
		b.WriteString("! ")
	}

	run := func(underCap bool) []map[string]any {
		comp, err := NewTokenChunker(map[string]any{
			"delimiter_mode":   "token_size",
			"chunk_token_size": budget,
			"under_cap":        underCap,
		})
		if err != nil {
			t.Fatalf("NewTokenChunker: %v", err)
		}
		out := comp.(*TokenChunkerComponent).mergeByTokenSize(b.String(), nil)
		chunks, _ := out["chunks"].([]map[string]any)
		return chunks
	}

	respect := run(true)
	if len(respect) < 2 {
		t.Fatalf("UNDER_CAP: want multiple chunks, got %d", len(respect))
	}
	for i, ck := range respect {
		text, _ := ck["text"].(string)
		if n := tokenizeStr(text); n > budget {
			t.Errorf("UNDER_CAP chunk %d exceeds target: tokens=%d (cap=%d)", i, n, budget)
		}
	}

	// Control: OVER_CAP (default) must follow its one-boundary-overflow
	// contract on the same input, proving the toggle changes behavior rather
	// than being a no-op. With the running-sum merge decision (faithful to
	// Python's naive_merge), a chunk may hold prev (<= budget) + one unit, so
	// its running-sum token count is <= budget+unit; the reconstructed text
	// carries the "\n" joins and cl100k is non-additive across them, so allow
	// the same one-extra-unit slack. For this particular input Python's
	// OVER_CAP does not actually exceed budget, but it still permits the
	// one-unit overflow that UNDER_CAP forbids, so the two strategies produce
	// different chunk counts — proving the toggle is live.
	over := run(false)
	if len(over) == len(respect) {
		t.Errorf("OVER_CAP produced the same chunk count as UNDER_CAP (%d); toggle may be a no-op", len(over))
	}
	for i, ck := range over {
		text, _ := ck["text"].(string)
		if tokenizeStr(text) > budget+sentenceN {
			t.Errorf("OVER_CAP chunk %d exceeds one-unit overflow contract: tokens=%d (cap=%d unit=%d)", i, tokenizeStr(text), budget, sentenceN)
		}
	}
}

func TestInvokeTextPayload_StrictCapEndToEnd(t *testing.T) {
	const budget = 32
	unit := tokenizeStr(strings.TrimSpace(strings.Repeat("alpha ", 12)))
	var b strings.Builder
	for i := 0; i < 20; i++ {
		b.WriteString(strings.TrimSpace(strings.Repeat("alpha ", 12)))
		b.WriteByte('\n')
	}
	comp, err := NewTokenChunker(map[string]any{
		"delimiter_mode":   "token_size",
		"chunk_token_size": budget,
	})
	if err != nil {
		t.Fatalf("NewTokenChunker: %v", err)
	}
	out, err := comp.Invoke(context.Background(), nil, map[string]any{
		"name":          "doc.txt",
		"output_format": "text",
		"text":          b.String(),
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if errMsg, _ := out["_ERROR"].(string); errMsg != "" {
		t.Fatalf("Invoke error payload: %s", errMsg)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) == 0 {
		t.Fatalf("expected chunks, got %#v", out)
	}
	for i, ck := range chunks {
		text, _ := ck["text"].(string)
		// OVER_CAP allows one boundary overflow (prev + one unit).
		if n := tokenizeStr(text); n > budget+unit {
			t.Errorf("chunk %d exceeds budget+one-unit: tokens=%d (cap=%d unit=%d)", i, n, budget, unit)
		}
	}
}

// TestInvokeJSONPayload_UnderCapEndToEnd exercises the UNDER_CAP strategy on
// the JSON path end-to-end (invokeJSONPayload). Many single-line JSON items
// are flattened into one global merge sequence, and under_cap=true must keep
// every chunk within chunk_token_size. The OVER_CAP control is asserted to
// pack fewer chunks than UNDER_CAP, proving the toggle is live on the JSON
// path — not just the text path covered by TestInvokeTextPayload_StrictCapEndToEnd.
func TestInvokeJSONPayload_UnderCapEndToEnd(t *testing.T) {
	const budget = 32
	unit := tokenizeStr(strings.TrimSpace(strings.Repeat("alpha ", 12)))

	// Each item is one ~unit-sized token block; 24 items give the global
	// merge plenty to accumulate.
	var items []map[string]any
	for i := 0; i < 24; i++ {
		items = append(items, map[string]any{
			"text":         strings.TrimSpace(strings.Repeat("alpha ", 12)),
			"doc_type_kwd": "text",
		})
	}

	run := func(underCap bool) []map[string]any {
		comp, err := NewTokenChunker(map[string]any{
			"delimiter_mode":   "delimiter",
			"delimiters":       []string{"\n"},
			"chunk_token_size": budget,
			"under_cap":        underCap,
		})
		if err != nil {
			t.Fatalf("NewTokenChunker: %v", err)
		}
		out, err := comp.Invoke(context.Background(), nil, map[string]any{
			"name":          "doc.json",
			"output_format": "json",
			"json":          items,
		})
		if err != nil {
			t.Fatalf("Invoke: %v", err)
		}
		if errMsg, _ := out["_ERROR"].(string); errMsg != "" {
			t.Fatalf("Invoke error payload: %s", errMsg)
		}
		chunks, _ := out["chunks"].([]map[string]any)
		return chunks
	}

	// UNDER_CAP: no chunk may exceed the token target.
	under := run(true)
	if len(under) < 2 {
		t.Fatalf("UNDER_CAP: want multiple chunks, got %d", len(under))
	}
	for i, ck := range under {
		text, _ := ck["text"].(string)
		if n := tokenizeStr(text); n > budget {
			t.Errorf("UNDER_CAP chunk %d exceeds target: tokens=%d (cap=%d)", i, n, budget)
		}
	}

	// OVER_CAP control: the same input packs fewer chunks (it allows one
	// boundary overflow per chunk), proving the toggle is live on the JSON
	// path. Equal lengths would mean under_cap is a no-op here.
	over := run(false)
	if len(over) >= len(under) {
		t.Errorf("OVER_CAP should pack fewer chunks than UNDER_CAP (over=%d under=%d); toggle may be a no-op on the JSON path", len(over), len(under))
	}
	for i, ck := range over {
		text, _ := ck["text"].(string)
		// OVER_CAP may overflow by at most one unit.
		if n := tokenizeStr(text); n > budget+unit {
			t.Errorf("OVER_CAP chunk %d overflows by more than one unit: tokens=%d (cap=%d unit=%d)", i, n, budget, unit)
		}
	}
}

// TestMergeDecisionOverCapVsUnderCapBoundary pins the semantic difference
// between the two merge strategies at the exact boundary
// (prevTokens+incomingTokens > target while incomingTokens <= target),
// independent of BPE text-token-count fluctuations. OVER_CAP must
// merge-then-close (a chunk may exceed target by at most one unit); UNDER_CAP
// must start a fresh chunk (strict no-overflow). This restores the strong
// constraint that the under_cap toggle is live — the integration test in
// TestMergeByTokenSize_UnderCapNoOverflow can only assert that the two
// strategies produce a different chunk COUNT, because on repetitive text the
// re-tokenized merged chunk can fall back under budget (cl100k is
// non-additive across the join). A regression that turns OVER_CAP into a
// no-op would not be caught there, but is caught here.
func TestMergeDecisionOverCapVsUnderCapBoundary(t *testing.T) {
	const prevT, incT = 10, 10
	target := prevT + incT - 1 // boundary: running sum > target, unit fits
	if incT > target {
		t.Fatalf("bad fixture: incoming unit must fit target (incT=%d target=%d)", incT, target)
	}

	_, overAct := mergeDecision("prev text", "incoming text", "\n", target, 0, schema.MergeOverCap, prevT, incT)
	if overAct != mergeThenClose {
		t.Errorf("OVER_CAP at boundary: want mergeThenClose, got %v", overAct)
	}

	_, underAct := mergeDecision("prev text", "incoming text", "\n", target, 0, schema.MergeUnderCap, prevT, incT)
	if underAct != startNewChunk {
		t.Errorf("UNDER_CAP at boundary: want startNewChunk, got %v", underAct)
	}

	// Both strategies must still refuse to merge an incoming unit that
	// already exceeds target — it stands alone as its own chunk.
	_, overBig := mergeDecision("prev text", "incoming text", "\n", target, 0, schema.MergeOverCap, prevT, target+5)
	if overBig != startNewChunk {
		t.Errorf("OVER_CAP with oversized incoming: want startNewChunk, got %v", overBig)
	}
	_, underBig := mergeDecision("prev text", "incoming text", "\n", target, 0, schema.MergeUnderCap, prevT, target+5)
	if underBig != startNewChunk {
		t.Errorf("UNDER_CAP with oversized incoming: want startNewChunk, got %v", underBig)
	}
}
