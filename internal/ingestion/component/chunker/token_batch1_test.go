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
	"reflect"
	"regexp"
	"strings"
	"testing"

	"ragflow/internal/ingestion/component/schema"
)

// TestSentenceDelimiterMatchesBangAndQuestion exercises migration diff
// Chunker-2.1: the sentence/clause boundary regex used to split oversized
// sections must also break on ASCII "!" and "?" (Python's default delimiter
// is "\n。；！？"). The legacy Go pattern `(\n|[。；！？]|\.\s)` missed the
// ASCII variants, so English fragments like "Hi!" / "Really?" were not
// treated as boundaries.
func TestSentenceDelimiterMatchesBangAndQuestion(t *testing.T) {
	// The package-level sentenceDelimiter (introduced by Fix 2.1) must
	// match ASCII bang/question.
	if !sentenceDelimiter.MatchString("Hi!") {
		t.Errorf("sentenceDelimiter should split on '!': %q", "Hi!")
	}
	if !sentenceDelimiter.MatchString("Really?") {
		t.Errorf("sentenceDelimiter should split on '?': %q", "Really?")
	}

	// Guard: the OLD pattern must NOT match these, proving the test would
	// have failed before the fix.
	old := regexp.MustCompile(`(\n|[。；！？]|\.\s)`)
	if old.MatchString("Hi!") || old.MatchString("Really?") {
		t.Errorf("guard broken: old pattern unexpectedly matches ASCII !/?")
	}
}

// TestMergeByTokenSizeFromJSON_OverlapStripsTags exercises migration diff
// Chunker-2.2: when a new chunk is started, its overlap prefix must be taken
// from the previous chunk AFTER remove_tag, otherwise parser tags (e.g.
// "@@1\t2.3##") leak into the overlap region. Mirrors Python
// nlp/__init__.py:1181 (remove_tag applied before overlap).
//
// After the strict-cap fix, a new chunk is started only when the projected
// join exceeds the budget — so the first unit must already sit near the
// budget and the second unit must not fit alongside it.
func TestMergeByTokenSizeFromJSON_OverlapStripsTags(t *testing.T) {
	// OVER_CAP (Python's canonical default) merges an overflowing unit into
	// the previous chunk and then closes it, so a chunk can exceed budget by
	// at most one unit. To exercise the overlap path we use three units:
	//   - a carries a parser tag and fits the budget alone,
	//   - b makes a+b overflow, so a+b merge-then-close into chunk0,
	//   - c starts a fresh chunk (prevClosed) with an overlap prefix from
	//     chunk0, which is where the tag-stripping must hold.
	aText := strings.Repeat("word ", 18) + "@@1\t2.3## tail"
	bText := strings.Repeat("word ", 18)
	cText := strings.Repeat("word ", 6)
	aN, bN, cN := tokenizeStr(aText), tokenizeStr(bText), tokenizeStr(cText)
	// The merge decision is now the running sum of per-unit token counts
	// (aN + bN), faithful to Python's _merge_text_chunks_by_token_size, NOT the
	// re-tokenized a+b join. Budget just below the a+b running sum so a and b
	// cannot merge without overflowing (forcing mergeThenClose on chunk0), but
	// a alone and c alone fit, and an overlap prefix carved from chunk0 fits
	// ahead of c (overlap path is exercised on chunk 1).
	budget := aN + bN - 1
	if budget < aN {
		budget = aN
	}
	if budget < cN {
		budget = cN
	}
	if aN+bN <= budget {
		t.Fatalf("could not derive tight budget (a=%d b=%d sum=%d budget=%d)", aN, bN, aN+bN, budget)
	}
	items := [][]schema.ChunkDoc{
		{
			{Text: aText, DocType: "text", CKType: "text", TKNums: intPtr(aN)},
			{Text: bText, DocType: "text", CKType: "text", TKNums: intPtr(bN)},
			{Text: cText, DocType: "text", CKType: "text", TKNums: intPtr(cN)},
		},
	}
	got := mergeByTokenSizeFromJSON(items, budget, 30.0, schema.MergeOverCap)
	merged := got[0]
	if len(merged) != 2 {
		t.Fatalf("want 2 chunks (overflow-closed + overlap chunk), got %d (a=%d b=%d c=%d budget=%d)", len(merged), aN, bN, cN, budget)
	}
	// The overlap prefix must actually be prepended to chunk 1; otherwise the
	// test would pass even if prevClosed started c without any overlap.
	overlap, _ := computeOverlapPrefix(merged[0].Text, 30.0)
	if overlap == "" {
		t.Fatal("expected a non-empty overlap prefix")
	}
	if !strings.HasPrefix(merged[1].Text, overlap) {
		t.Errorf("chunk 1 missing overlap prefix %q: %q", overlap, merged[1].Text)
	}
	// The overlap prefix is prepended to the SECOND chunk. The first chunk
	// legitimately keeps its own parser tag; only the overlap region
	// (merged[1]) must be tag-free.
	if strings.Contains(merged[1].Text, "@@") || strings.Contains(merged[1].Text, "##") {
		t.Errorf("overlap prefix leaked parser tag into chunk 1: %q", merged[1].Text)
	}
	if n := tokenizeStr(merged[1].Text); n > budget {
		t.Errorf("overlap pushed second chunk over budget: tokens=%d (cap=%d)", n, budget)
	}
}

// TestMergeByTokenSizeFromJSON_NonTextBoundaryResetsPrevClosed is a TDD
// regression test for the OVER_CAP boundary-overflow flag (prevClosed) leaking
// across a non-text chunk. mergeByTokenSizeFromJSON must reset prevClosed when
// the previous merged chunk is non-text; otherwise the first text chunk after a
// non-text chunk carries the stale flag and forces the NEXT text chunk to start
// a fresh chunk even though it should merge with its predecessor.
//
// Sequence: T1, T2 (overflow-merge-close into chunk0), N (non-text), T3, T4.
// With a correct reset, T3 is the first text after N (new chunk) and T4 merges
// back into T3 -> 3 chunks total. With the bug, prevClosed survives the N
// boundary and T4 is wrongly forced into its own chunk -> 4 chunks.
func TestMergeByTokenSizeFromJSON_NonTextBoundaryResetsPrevClosed(t *testing.T) {
	t1 := strings.Repeat("word ", 18)
	t2 := strings.Repeat("word ", 18)
	t3 := strings.Repeat("word ", 9)
	t4 := strings.Repeat("word ", 9)
	t1N, t2N := tokenizeStr(t1), tokenizeStr(t2)
	joined12 := tokenizeStr(t1 + "\n" + t2)
	// Budget just below the T1+T2 join so T1 and T2 cannot merge without
	// overflowing, forcing mergeThenClose on chunk0 (prevClosed=true).
	budget := joined12 - 1
	if budget < t1N {
		budget = t1N
	}
	t34 := tokenizeStr(t3 + "\n" + t4)
	if t34 > budget {
		t.Fatalf("T3+T4 must fit budget to exercise the merge; t34=%d budget=%d", t34, budget)
	}
	if joined12 <= budget {
		t.Fatalf("could not derive tight budget (t1=%d t2=%d joined=%d budget=%d)", t1N, t2N, joined12, budget)
	}

	items := [][]schema.ChunkDoc{
		{
			{Text: t1, DocType: "text", CKType: "text", TKNums: intPtr(t1N)},
			{Text: t2, DocType: "text", CKType: "text", TKNums: intPtr(t2N)},
			{Text: "[image]", DocType: "image", CKType: "image", TKNums: intPtr(1)},
			{Text: t3, DocType: "text", CKType: "text", TKNums: intPtr(tokenizeStr(t3))},
			{Text: t4, DocType: "text", CKType: "text", TKNums: intPtr(tokenizeStr(t4))},
		},
	}
	got := mergeByTokenSizeFromJSON(items, budget, 0.0, schema.MergeOverCap)
	merged := got[0]
	// Expect: chunk0 (T1+T2, overflow-closed), N (non-text), chunk1 (T3+T4 merged).
	if len(merged) != 3 {
		var texts []string
		for _, c := range merged {
			texts = append(texts, c.CKType+":"+c.Text)
		}
		t.Fatalf("want 3 chunks (overflow-closed + non-text + merged), got %d: %v", len(merged), texts)
	}
	// Lock the overflow-closed precondition: T1+T2 must have merged into
	// chunk0 with prevClosed set, otherwise the later assertions could pass
	// without exercising the boundary-overflow path at all.
	if merged[0].Text != t1+"\n"+t2 {
		t.Errorf("chunk[0] should be the overflow-closed T1+T2 chunk: got %q", merged[0].Text)
	}
	if merged[1].CKType != "image" {
		t.Errorf("chunk[1] should be the non-text image chunk, got CKType=%q text=%q", merged[1].CKType, merged[1].Text)
	}
	wantMerged := t3 + "\n" + t4
	if merged[2].Text != wantMerged {
		t.Errorf("chunk[2] should merge T3+T4: want %q got %q", wantMerged, merged[2].Text)
	}
}

// TestMergeByTokenSizeFromJSON_UnderCapNoOverflow exercises the UNDER_CAP
// strategy (schema.MergeUnderCap): a projected join that would exceed
// the target must start a fresh chunk instead of merging-then-closing. This is
// the seam that lets Go follow Python's no-overflow (UNDER_CAP) strategy. Under
// OVER_CAP the same input merges a+b and overflows chunk0; here a, b, c must
// stay as three separate chunks, each within budget.
func TestMergeByTokenSizeFromJSON_UnderCapNoOverflow(t *testing.T) {
	aText := strings.Repeat("word ", 18)
	bText := strings.Repeat("word ", 18)
	cText := strings.Repeat("word ", 18)
	aN, bN, cN := tokenizeStr(aText), tokenizeStr(bText), tokenizeStr(cText)
	joinedAB := tokenizeStr(aText + "\n" + bText)
	// Budget just below the a+b join so a and b cannot merge without
	// overflowing; a alone and c alone fit.
	budget := joinedAB - 1
	if budget < aN {
		budget = aN
	}
	if budget < cN {
		budget = cN
	}
	if joinedAB <= budget {
		t.Fatalf("could not derive tight budget (a=%d b=%d joined=%d budget=%d)", aN, bN, joinedAB, budget)
	}
	items := [][]schema.ChunkDoc{
		{
			{Text: aText, DocType: "text", CKType: "text", TKNums: intPtr(aN)},
			{Text: bText, DocType: "text", CKType: "text", TKNums: intPtr(bN)},
			{Text: cText, DocType: "text", CKType: "text", TKNums: intPtr(cN)},
		},
	}
	got := mergeByTokenSizeFromJSON(items, budget, 0.0, schema.MergeUnderCap)
	merged := got[0]
	if len(merged) != 3 {
		t.Fatalf("UNDER_CAP want 3 chunks (a, b, c separate), got %d", len(merged))
	}
	for i, ck := range merged {
		if n := tokenizeStr(ck.Text); n > budget {
			t.Errorf("UNDER_CAP chunk %d exceeds target: tokens=%d (cap=%d)", i, n, budget)
		}
	}
}

// TestMergeByTokenSizeFromJSON_ClampsOverlappedPct locks the review finding
// from yuzhichang (PR #17396): mergeByTokenSizeFromJSON clamps an out-of-range
// overlappedPct to [0,100] so the merge math never yields a negative/inverted
// threshold. Out-of-range values must not panic and must behave identically to
// their clamped-in-range equivalent (150 == 100, -5 == 0, and the same for
// huge magnitudes that would otherwise overflow the float->int slice index).
// clampOverlapFixture returns a fresh input for mergeByTokenSizeFromJSON.
// A new slice must be built per invocation: mergeByTokenSizeFromJSON mutates
// its perItem argument in place (token.go: perItem[idx] = merged) and returns
// the same backing array. Reusing one fixture across calls lets later calls
// reprocess already-merged chunks, and — because the result aliases the input
// — silently overwrites earlier results, making the clamp assertions vacuous
// (see code review on PR #17396).
//
// The first chunk carries TKNums 130 — just above chunk_token_size 128 — so
// the two clamp directions are both exercised: at pct=0 (threshold 128) the
// chunks stay split, while an UNCLAMPED negative pct raises the threshold
// (e.g. -5 -> 134.4) and merges them. With TKNums=100 both cases merge
// identically, so the lower-clamp assertions would pass even if the clamp
// were removed (review: coderabbitai on PR #17416).
func clampOverlapFixture() [][]schema.ChunkDoc {
	return [][]schema.ChunkDoc{
		{
			{Text: strings.Repeat("word ", 20), DocType: "text", CKType: "text", TKNums: intPtr(130)},
			{Text: "body", DocType: "text", CKType: "text", TKNums: intPtr(5)},
		},
	}
}

func TestMergeByTokenSizeFromJSON_ClampsOverlappedPct(t *testing.T) {
	at100 := mergeByTokenSizeFromJSON(clampOverlapFixture(), 128, 100, schema.MergeOverCap)
	if at100 == nil || len(at100) == 0 {
		t.Fatalf("overlappedPct=100: nil/empty result")
	}
	at150 := mergeByTokenSizeFromJSON(clampOverlapFixture(), 128, 150, schema.MergeOverCap)
	atHuge := mergeByTokenSizeFromJSON(clampOverlapFixture(), 128, 1e300, schema.MergeOverCap)
	if !reflect.DeepEqual(at100, at150) {
		t.Errorf("overlappedPct=150 should clamp to 100; output differs from 100")
	}
	if !reflect.DeepEqual(at100, atHuge) {
		t.Errorf("overlappedPct=1e300 should clamp to 100; output differs from 100")
	}

	at0 := mergeByTokenSizeFromJSON(clampOverlapFixture(), 128, 0, schema.MergeOverCap)
	if at0 == nil || len(at0) == 0 {
		t.Fatalf("overlappedPct=0: nil/empty result")
	}
	atNeg := mergeByTokenSizeFromJSON(clampOverlapFixture(), 128, -5, schema.MergeOverCap)
	atNegHuge := mergeByTokenSizeFromJSON(clampOverlapFixture(), 128, -1e300, schema.MergeOverCap)
	if !reflect.DeepEqual(at0, atNeg) {
		t.Errorf("overlappedPct=-5 should clamp to 0; output differs from 0")
	}
	if !reflect.DeepEqual(at0, atNegHuge) {
		t.Errorf("overlappedPct=-1e300 should clamp to 0; output differs from 0")
	}
}

// TestMergeByTokenSizeFromJSON_EmptyPrevKeepsChunk exercises migration diff
// Chunker-2.11: merging a non-empty chunk into an empty previous chunk must
// assign the text directly instead of being skipped. The legacy guard
// `if prev.Text != ""` silently dropped the incoming chunk when the previous
// one had empty text. Mirrors Python token_chunker.py:236-239.
func TestMergeByTokenSizeFromJSON_EmptyPrevKeepsChunk(t *testing.T) {
	items := [][]schema.ChunkDoc{
		{
			{Text: "", DocType: "text", CKType: "text", TKNums: intPtr(5)},
			{Text: "keepme", DocType: "text", CKType: "text", TKNums: intPtr(5)},
		},
	}
	got := mergeByTokenSizeFromJSON(items, 128, 0, schema.MergeOverCap)
	merged := got[0]
	if len(merged) != 1 {
		t.Fatalf("want 1 merged chunk, got %d", len(merged))
	}
	if merged[0].Text != "keepme" {
		t.Errorf("empty previous chunk dropped incoming text; got %q", merged[0].Text)
	}
}

// TestTakeFromEndRespectsTokenCount and TestTakeFromStartRespectsTokenCount
// covers takeFromEnd/takeFromStart used a
// fixed 4-bytes-per-token heuristic which badly over-counts for CJK text
// (≈3 bytes/char, 1-2 tokens/char). They must now count tokens exactly via
// tokenizeStr so the returned slice is close to the requested token budget.
func TestTakeFromEndRespectsTokenCount(t *testing.T) {
	const target = 20
	s := strings.Repeat("中", 60)
	got := takeFromEnd(s, target)
	if !strings.HasSuffix(s, got) {
		t.Fatalf("takeFromEnd result must be a suffix of input")
	}
	n := tokenizeStr(got)
	if n < target-3 || n > target+3 {
		t.Errorf("takeFromEnd(%d tokens) returned slice with %d tokens (want ~%d)", target, n, target)
	}
}

func TestTakeFromStartRespectsTokenCount(t *testing.T) {
	const target = 20
	s := strings.Repeat("中", 60)
	got := takeFromStart(s, target)
	if !strings.HasPrefix(s, got) {
		t.Fatalf("takeFromStart result must be a prefix of input")
	}
	n := tokenizeStr(got)
	if n < target-3 || n > target+3 {
		t.Errorf("takeFromStart(%d tokens) returned slice with %d tokens (want ~%d)", target, n, target)
	}
}
