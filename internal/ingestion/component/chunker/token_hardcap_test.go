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
	"strings"
	"testing"

	"ragflow/internal/ingestion/component/schema"
)

// ---------------------------------------------------------------------------
// 方案 B: hard-cap merge contract.
//
// Every text chunk emitted by the TokenChunker merge must be <= chunk_token_size:
//  1. Oversized units are expanded first (sentence-boundary split, then a
//     hard token-split fallback for boundary-less runs) so no unit exceeds the
//     target.
//  2. The merge is UNDER_CAP: a projected join that would push the running sum
//     over the target starts a fresh chunk instead of merge-then-close.
//  3. The overlap prefix is trimmed so it never pushes a fresh chunk over the
//     target.
//  4. Non-text units pass through unchanged (atomic).
//  5. Expansion is lossless: sub-pieces reproduce the original text.
// ---------------------------------------------------------------------------

// TestMergeUnits_UnderCapRunningSum pins the UNDER_CAP decision rule with exact
// running sums (independent of BPE text-count noise): 25+25 fits 50 exactly,
// the next 25-unit must start a fresh chunk.
func TestMergeUnits_UnderCapRunningSum(t *testing.T) {
	const target = 50
	units := []schema.ChunkDoc{
		{Text: "u1", TKNums: intPtr(25), CKType: "text"},
		{Text: "u2", TKNums: intPtr(25), CKType: "text"},
		{Text: "u3", TKNums: intPtr(25), CKType: "text"},
		{Text: "u4", TKNums: intPtr(25), CKType: "text"},
	}
	got := mergeUnits(units, target, 0, "")
	if len(got) != 2 {
		t.Fatalf("want 2 chunks (25+25, 25+25), got %d", len(got))
	}
	for i, ck := range got {
		if sum := intValue(ck.TKNums); sum > target {
			t.Errorf("chunk %d running sum %d exceeds target %d", i, sum, target)
		}
	}
}

// TestMergeUnits_BoundaryExactFit pins the boundary: prev+incoming == target
// still merges (<= target), prev+incoming > target starts a fresh chunk.
func TestMergeUnits_BoundaryExactFit(t *testing.T) {
	fit := mergeUnits([]schema.ChunkDoc{
		{Text: "a", TKNums: intPtr(25), CKType: "text"},
		{Text: "b", TKNums: intPtr(25), CKType: "text"},
	}, 50, 0, "")
	if len(fit) != 1 {
		t.Fatalf("exact-fit join must merge, got %d chunks", len(fit))
	}
	over := mergeUnits([]schema.ChunkDoc{
		{Text: "a", TKNums: intPtr(30), CKType: "text"},
		{Text: "b", TKNums: intPtr(30), CKType: "text"},
	}, 50, 0, "")
	if len(over) != 2 {
		t.Fatalf("overflowing join must start a fresh chunk, got %d chunks", len(over))
	}
}

// TestMergeUnits_ExpandsOversizedUnit verifies a single boundary-less unit that
// exceeds the target is hard token-split into <= target pieces and the
// concatenated pieces reproduce the original text exactly (lossless).
func TestMergeUnits_ExpandsOversizedUnit(t *testing.T) {
	const target = 30
	body := strings.Repeat("word ", 100) // ~100 tokens, no sentence delimiter
	got := mergeUnits([]schema.ChunkDoc{
		{Text: body, CKType: "text", TKNums: intPtr(tokenizeStr(body))},
	}, target, 0, "")
	if len(got) < 2 {
		t.Fatalf("oversized unit must be split, got %d chunk(s)", len(got))
	}
	var joined string
	for i, ck := range got {
		if n := tokenizeStr(ck.Text); n > target {
			t.Errorf("chunk %d exceeds target: tokens=%d (cap=%d)", i, n, target)
		}
		joined += ck.Text
	}
	if joined != body {
		t.Errorf("hard split not lossless:\n got %q\nwant %q", joined, body)
	}
}

// TestMergeUnits_ExpandsOversizedSentence verifies an oversized unit with
// sentence delimiters is split on sentence boundaries first (delimiters
// preserved, lossless), each piece <= target.
func TestMergeUnits_ExpandsOversizedSentence(t *testing.T) {
	sentence := "word word word word word word。"
	body := strings.Repeat(sentence, 4)
	target := tokenizeStr(sentence) // each sentence fits the target; the whole body does not
	if target <= 0 {
		t.Fatal("fixture: tokenize returned 0 for sentence")
	}
	got := mergeUnits([]schema.ChunkDoc{
		{Text: body, CKType: "text", TKNums: intPtr(tokenizeStr(body))},
	}, target, 0, "")
	if len(got) < 2 {
		t.Fatalf("oversized unit must be sentence-split, got %d chunk(s)", len(got))
	}
	var joined string
	for i, ck := range got {
		if n := tokenizeStr(ck.Text); n > target {
			t.Errorf("chunk %d exceeds target: tokens=%d (cap=%d)", i, n, target)
		}
		joined += ck.Text
	}
	if joined != body {
		t.Errorf("sentence split not lossless:\n got %q\nwant %q", joined, body)
	}
}

// TestMergeUnits_SubChunksKeepPositions verifies that the original (coarse)
// positions attach to every sub-chunk of an expanded unit, so each piece
// still gets its page-region preview image.
func TestMergeUnits_SubChunksKeepPositions(t *testing.T) {
	const target = 30
	body := strings.Repeat("word ", 100)
	pos := []byte(`[{"page":1}]`)
	got := mergeUnits([]schema.ChunkDoc{
		{Text: body, CKType: "text", TKNums: intPtr(tokenizeStr(body)), PDFPositions: pos},
	}, target, 0, "")
	if len(got) < 2 {
		t.Fatalf("want >=2 chunks, got %d", len(got))
	}
	for i := range got {
		if string(got[i].PDFPositions) != string(pos) {
			t.Errorf("sub-chunk %d must carry the original positions, got %q", i, got[i].PDFPositions)
		}
	}
}

// TestMergeUnits_AtomicNonText verifies non-text chunks (tables/images) are
// never split even when their token count exceeds the target.
func TestMergeUnits_AtomicNonText(t *testing.T) {
	table := schema.ChunkDoc{Text: strings.Repeat("t", 500), CKType: "table", TKNums: intPtr(500)}
	got := mergeUnits([]schema.ChunkDoc{table}, 50, 0, "")
	if len(got) != 1 || got[0].Text != table.Text {
		t.Fatalf("table chunk must pass through unchanged, got %#v", got)
	}
}

// TestMergeUnits_ExpandedPiecesKeepDocType verifies that pieces produced by
// the oversized-unit expansion carry the source unit's DocType, so the emitted
// chunks keep doc_type_kwd="text" instead of silently dropping it.
func TestMergeUnits_ExpandedPiecesKeepDocType(t *testing.T) {
	const target = 30
	body := strings.Repeat("word ", 100)
	got := mergeUnits([]schema.ChunkDoc{
		{Text: body, DocType: "text", CKType: "text", TKNums: intPtr(tokenizeStr(body))},
	}, target, 0, "")
	if len(got) < 2 {
		t.Fatalf("oversized unit must be split, got %d chunk(s)", len(got))
	}
	for i, ck := range got {
		if ck.DocType != "text" {
			t.Errorf("chunk %d lost DocType: got %q, want %q", i, ck.DocType, "text")
		}
	}
}

// TestMergeUnits_HardSplitNeverSplitsCoordTag verifies the hard token-split
// never cuts through a @@...## coordinate tag: a piece either contains the
// complete tag or no part of it.
func TestMergeUnits_HardSplitNeverSplitsCoordTag(t *testing.T) {
	const target = 8
	const tag = "@@1\t2\t3\t4##"
	body := strings.Repeat("word ", 30) + tag + strings.Repeat("pad ", 30)
	got := mergeUnits([]schema.ChunkDoc{
		{Text: body, CKType: "text", TKNums: intPtr(tokenizeStr(body))},
	}, target, 0, "")
	if len(got) < 3 {
		t.Fatalf("want multiple pieces, got %d", len(got))
	}
	tagCount := 0
	for i, ck := range got {
		if strings.Contains(ck.Text, "@@") || strings.Contains(ck.Text, "##") {
			tagCount++
			if !strings.Contains(ck.Text, tag) {
				t.Errorf("piece %d contains a partial coord tag: %q", i, ck.Text)
			}
		}
	}
	if tagCount != 1 {
		t.Errorf("the coord tag must appear whole in exactly one piece, got %d", tagCount)
	}
}

// TestMergeUnits_OverlapStrictCap verifies the overlap prefix never pushes a
// fresh chunk over the target: overlap is trimmed to fit. Both the running sum
// (TKNums) and the re-tokenized emitted text are checked against the target.
func TestMergeUnits_OverlapStrictCap(t *testing.T) {
	const target = 40
	units := []schema.ChunkDoc{
		{Text: strings.Repeat("word ", 30), CKType: "text"}, // ~30 tokens
		{Text: strings.Repeat("pad ", 38), CKType: "text"},  // ~38 tokens, near target
	}
	got := mergeUnits(units, target, 20, "\n")
	if len(got) < 2 {
		t.Fatalf("want >=2 chunks, got %d", len(got))
	}
	for i, ck := range got {
		if n := intValue(ck.TKNums); n > target {
			t.Errorf("chunk %d exceeds target: tokens=%d (cap=%d)", i, n, target)
		}
		if n := tokenizeStr(ck.Text); n > target {
			t.Errorf("chunk %d emitted text exceeds target: tokens=%d (cap=%d)", i, n, target)
		}
	}
}

// TestMergeUnits_ExpansionPreservesWhitespaceFragments verifies that
// whitespace-only fragments from a repeated delimiter run (e.g. "\n\n") are
// carried into adjacent pieces, so the concatenated pieces reproduce the
// oversized unit text exactly (lossless).
func TestMergeUnits_ExpansionPreservesWhitespaceFragments(t *testing.T) {
	const target = 30
	body := strings.Repeat("word ", 60) + "\n\n" + strings.Repeat("pad ", 60)
	got := mergeUnits([]schema.ChunkDoc{
		{Text: body, CKType: "text", TKNums: intPtr(tokenizeStr(body))},
	}, target, 0, "")
	if len(got) < 2 {
		t.Fatalf("oversized unit must be split, got %d chunk(s)", len(got))
	}
	var joined string
	for _, ck := range got {
		joined += ck.Text
	}
	if joined != body {
		t.Errorf("split dropped text (whitespace or content):\n got %q\nwant %q", joined, body)
	}
}

// TestMergeUnits_ExpandedPiecesCarryMetadata verifies metadata inheritance:
// every sub-piece of an expanded unit keeps the fields the source unit
// carried (coarse positions plus item attributes), built from a clone of the
// source unit.
func TestMergeUnits_ExpandedPiecesCarryMetadata(t *testing.T) {
	const target = 30
	body := strings.Repeat("word ", 100)
	unit := schema.ChunkDoc{
		Text:          body,
		DocType:       "text",
		CKType:        "text",
		TKNums:        intPtr(tokenizeStr(body)),
		PDFPositions:  []byte(`[{"page":1}]`),
		Mom:           "parent-id",
		ImgID:         "img-1",
		Layout:        "text",
		Image:         "raw-image",
		PageNumber:    intPtr(3),
		TagKwd:        []string{"t1"},
		ChunkOrderInt: intPtr(7),
	}
	got := mergeUnits([]schema.ChunkDoc{unit}, target, 0, "\n")
	if len(got) < 2 {
		t.Fatalf("oversized unit must be split, got %d chunk(s)", len(got))
	}
	for i, ck := range got {
		if ck.Mom != "parent-id" || ck.ImgID != "img-1" || ck.Layout != "text" || ck.Image != "raw-image" {
			t.Errorf("sub-piece %d lost item metadata: %#v", i, ck)
		}
		if ck.PageNumber == nil || *ck.PageNumber != 3 {
			t.Errorf("sub-piece %d lost PageNumber: %#v", i, ck.PageNumber)
		}
		if len(ck.TagKwd) != 1 || ck.TagKwd[0] != "t1" {
			t.Errorf("sub-piece %d lost TagKwd: %#v", i, ck.TagKwd)
		}
		if ck.ChunkOrderInt == nil || *ck.ChunkOrderInt != 7 {
			t.Errorf("sub-piece %d lost ChunkOrderInt: %#v", i, ck.ChunkOrderInt)
		}
		if len(ck.PDFPositions) == 0 {
			t.Errorf("sub-piece %d must carry the original positions", i)
		}
	}
}

// TestMergeByTokenSizeFromJSON_HardCapStrict verifies the JSON path never emits
// a text chunk whose running sum exceeds the budget.
func TestMergeByTokenSizeFromJSON_HardCapStrict(t *testing.T) {
	const budget = 40
	var items []schema.ChunkDoc
	for i := 0; i < 12; i++ {
		text := strings.Repeat("w ", 12)
		items = append(items, schema.ChunkDoc{Text: text, DocType: "text", CKType: "text", TKNums: intPtr(tokenizeStr(text))})
	}
	got := mergeByTokenSizeFromJSON([][]schema.ChunkDoc{items}, budget, 0)
	if len(got[0]) < 2 {
		t.Fatalf("want multiple chunks, got %d", len(got[0]))
	}
	for i, ck := range got[0] {
		if n := intValue(ck.TKNums); n > budget {
			t.Errorf("chunk %d running sum %d exceeds budget %d", i, n, budget)
		}
	}
}

// TestMergeUnits_JoinSepAddsTokens pins the re-tokenize guard: joinSep (the
// JSON path's "\n") can add tokens beyond the running sum, so the emitted text
// of a merge that the running sum deems safe can still exceed target. The
// merge must re-check the actual joined text and start a fresh chunk instead.
func TestMergeUnits_JoinSepAddsTokens(t *testing.T) {
	a := "word"
	b := "pad"
	na, nb := tokenizeStr(a), tokenizeStr(b)
	target := na + nb // running sum exactly fits
	if joinedTK := tokenizeStr(a + "\n" + b); joinedTK <= target {
		t.Skipf("fixture: joinSep newline adds no token for these texts (joined=%d target=%d)", joinedTK, target)
	}
	got := mergeUnits([]schema.ChunkDoc{
		{Text: a, TKNums: intPtr(na), CKType: "text"},
		{Text: b, TKNums: intPtr(nb), CKType: "text"},
	}, target, 0, "\n")
	if len(got) != 2 {
		t.Fatalf("joined text exceeds target via joinSep: want 2 chunks, got %d", len(got))
	}
	for i, ck := range got {
		if n := tokenizeStr(ck.Text); n > target {
			t.Errorf("chunk %d exceeds target: tokens=%d (cap=%d) text=%q", i, n, target, ck.Text)
		}
	}
}

// TestExpandOversizedUnits_AllWhitespaceOverCap pins the all-whitespace branch:
// a whitespace-only unit whose token count exceeds the target must be hard-split
// like any other oversized unit, not emitted as a single over-cap chunk. The
// fixture avoids a leading "\n" (which the lead-glue branch already re-splits)
// so it exercises the all-whitespace else branch in splitOversizedText.
func TestExpandOversizedUnits_AllWhitespaceOverCap(t *testing.T) {
	ws := strings.Repeat(" \n", 20) // 40 runes; each "\n" is one cl100k token
	const target = 8
	if n := tokenizeStr(ws); n <= target {
		t.Skipf("fixture: whitespace tokenizes to %d <= target %d", n, target)
	}
	got := expandOversizedUnits([]schema.ChunkDoc{{Text: ws, CKType: "text"}}, target)
	if len(got) < 2 {
		t.Fatalf("all-whitespace unit over cap must be hard-split, got %d chunk(s)", len(got))
	}
	var joined string
	for i, ck := range got {
		if n := tokenizeStr(ck.Text); n > target {
			t.Errorf("chunk %d exceeds target: tokens=%d (cap=%d)", i, n, target)
		}
		joined += ck.Text
	}
	if joined != ws {
		t.Errorf("split not lossless:\n got=%q\nwant=%q", joined, ws)
	}
}
