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
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
	"unicode/utf8"

	"ragflow/internal/ingestion/component/schema"
)

// TestMergeByTokenSizeFromJSON_ExtendsPDFPositions is the TDD test for
// migration diffs Chunker-2.5 / 2.3: when two JSON text items carrying
// `_pdf_positions` / `positions` are merged into one chunk, the merged
// chunk must extend (not drop) the coordinate lists — mirroring Python
// token_chunker.py:240 `merged[prev][PDF_POSITIONS_KEY].extend(...)`.
func TestMergeByTokenSizeFromJSON_ExtendsPDFPositions(t *testing.T) {
	posA := json.RawMessage(`[[1,10,20,30,40]]`)
	posB := json.RawMessage(`[[2,15,25,35,45]]`)
	items := [][]schema.ChunkDoc{
		{
			{Text: "alpha", DocType: "text", CKType: "text", TKNums: intPtr(5), PDFPositions: posA},
			{Text: "beta", DocType: "text", CKType: "text", TKNums: intPtr(5), PDFPositions: posB},
		},
	}
	got := mergeByTokenSizeFromJSON(items, 128, 0)
	merged := got[0]
	if len(merged) != 1 {
		t.Fatalf("want 1 merged chunk, got %d", len(merged))
	}
	combined := string(merged[0].PDFPositions)
	if !strings.Contains(combined, "1,10,20,30,40") {
		t.Errorf("merged chunk lost first item _pdf_positions: %s", combined)
	}
	if !strings.Contains(combined, "2,15,25,35,45") {
		t.Errorf("merged chunk dropped second item _pdf_positions (not extended): %s", combined)
	}
}

// TestMergeByTokenSizeFromJSON_ExtendsPositions covers the parallel
// `positions` field (diff 2.3).
func TestMergeByTokenSizeFromJSON_ExtendsPositions(t *testing.T) {
	posA := json.RawMessage(`[[1,2,3]]`)
	posB := json.RawMessage(`[[4,5,6]]`)
	items := [][]schema.ChunkDoc{
		{
			{Text: "a", DocType: "text", CKType: "text", TKNums: intPtr(5), Positions: posA},
			{Text: "b", DocType: "text", CKType: "text", TKNums: intPtr(5), Positions: posB},
		},
	}
	got := mergeByTokenSizeFromJSON(items, 128, 0)
	combined := string(got[0][0].Positions)
	if !strings.Contains(combined, "1,2,3") || !strings.Contains(combined, "4,5,6") {
		t.Errorf("merged chunk dropped/omitted `positions`: %s", combined)
	}
}

// TestCloneChunkDoc_DeepCopiesPDFPositions ensures cloneChunkDoc does not
// alias the underlying _pdf_positions / positions byte slices (diff 2.5
// defensive fix).
func TestCloneChunkDoc_DeepCopiesPDFPositions(t *testing.T) {
	pos := json.RawMessage(`[[1,2,3,4,5]]`)
	orig := schema.ChunkDoc{Text: "x", PDFPositions: pos, Positions: pos}
	cp := cloneChunkDoc(orig)
	// Mutate the source's backing array after the clone.
	pos[0] = '9'
	if string(cp.PDFPositions) != "[[1,2,3,4,5]]" {
		t.Errorf("clone shares _pdf_positions backing array: %s", string(cp.PDFPositions))
	}
	if string(cp.Positions) != "[[1,2,3,4,5]]" {
		t.Errorf("clone shares positions backing array: %s", string(cp.Positions))
	}
}

// TestMergeByTokenSizeFromJSON_PositionsDecodeToMatrix verifies the
// chunker-side contract for diff 1.4: preserved `positions` must decode
// (via ChunkDoc.ToMap → decodeStructuredValue) to a [][]float64 matrix so
// the downstream task-layer processChunkPositions → AddPositions can
// convert it to page_num_int / top_int / position_int. The coordinate
// conversion itself lives in internal/ingestion/task (processChunkPositions),
// not in the chunker.
func TestMergeByTokenSizeFromJSON_PositionsDecodeToMatrix(t *testing.T) {
	posA := json.RawMessage(`[[1,2,3,4,5]]`)
	posB := json.RawMessage(`[[6,7,8,9,10]]`)
	items := [][]schema.ChunkDoc{
		{
			{Text: "a", DocType: "text", CKType: "text", TKNums: intPtr(5), Positions: posA},
			{Text: "b", DocType: "text", CKType: "text", TKNums: intPtr(5), Positions: posB},
		},
	}
	got := mergeByTokenSizeFromJSON(items, 128, 0)
	m := got[0][0].ToMap()
	raw, ok := m["positions"]
	if !ok {
		t.Fatal("positions missing from ToMap output")
	}
	matrix, ok := raw.([][]float64)
	if !ok {
		t.Fatalf("positions decoded to %T, want [][]float64", raw)
	}
	if len(matrix) != 2 {
		t.Fatalf("positions matrix has %d groups, want 2 (both merged items)", len(matrix))
	}
}

// TestMergeByTokenSizeFromJSON_OverlapPrefixCarriesPrevPositions is a TDD test
// for issue #18148. Python's token_chunker drops overlap-head PDF coordinates,
// and the Go mergeUnits overlap branch has the SAME defect: when a fresh chunk
// starts and overlap>0, the tail of the previous chunk is prepended to the new
// chunk's text (computeOverlapPrefix, token.go:839 / :860), but the new chunk's
// PDFPositions is left as only cur's coordinates (token.go:836-848 and
// :854-868). The overlap prefix is part of the chunk's visible/displayed
// content, so its coordinates must be carried forward — exactly like the
// merge-into-prev path extends positions (token.go:877). On the buggy code the
// overlap text is shown but NOT highlighted.
//
// overlappedPct=100 forces the overlap prefix to be the ENTIRE previous chunk,
// so the expectation is crisp: every new chunk must carry the previous chunk's
// full coordinates. This test is RED until the overlap branch carries
// coordinates.
func TestMergeByTokenSizeFromJSON_OverlapPrefixCarriesPrevPositions(t *testing.T) {
	posA := json.RawMessage(`[[1,0,10,0,5]]`)
	posB := json.RawMessage(`[[2,0,20,0,8]]`)
	posC := json.RawMessage(`[[3,0,30,0,12]]`)
	// At overlapPct=100 the scaled threshold is 0, so every unit after the
	// first starts a fresh chunk carrying the WHOLE previous chunk as overlap.
	items := [][]schema.ChunkDoc{
		{
			{Text: "alpha", DocType: "text", CKType: "text", TKNums: intPtr(5), PDFPositions: posA},
			{Text: "beta", DocType: "text", CKType: "text", TKNums: intPtr(5), PDFPositions: posB},
			{Text: "gamma", DocType: "text", CKType: "text", TKNums: intPtr(5), PDFPositions: posC},
		},
	}
	got := mergeByTokenSizeFromJSON(items, 20, 100)
	merged := got[0]
	if len(merged) != 3 {
		t.Fatalf("want 3 chunks (each unit starts fresh at overlapPct=100), got %d", len(merged))
	}

	// chunk[1] starts with the overlap prefix copied from chunk[0] ("alpha").
	if !strings.Contains(merged[1].Text, "alpha") {
		t.Errorf("chunk[1] missing overlap prefix from prev chunk: text=%q", merged[1].Text)
	}
	// The overlap prefix is shown, so chunk[1] must also carry chunk[0]'s
	// coordinates. BUG: only chunk[1]'s own (posB) coordinates survive today.
	if !strings.Contains(string(merged[1].PDFPositions), "1,0,10,0,5") {
		t.Errorf("chunk[1] dropped overlap-head coordinates (prev chunk[0] posA): pdf_positions=%s", string(merged[1].PDFPositions))
	}
	if !strings.Contains(string(merged[1].PDFPositions), "2,0,20,0,8") {
		t.Errorf("chunk[1] lost its own coordinates: pdf_positions=%s", string(merged[1].PDFPositions))
	}

	// chunk[2]'s overlap prefix is the full chunk[1] text; its coordinates must
	// include chunk[0], chunk[1], and its own (the overlap chain is carried).
	if !strings.Contains(merged[2].Text, "alphabeta") {
		t.Errorf("chunk[2] missing overlap prefix from prev chunk: text=%q", merged[2].Text)
	}
	for _, want := range []string{"1,0,10,0,5", "2,0,20,0,8", "3,0,30,0,12"} {
		if !strings.Contains(string(merged[2].PDFPositions), want) {
			t.Errorf("chunk[2] missing coordinates %s (overlap chain not carried): pdf_positions=%s", want, string(merged[2].PDFPositions))
		}
	}
}

// TestMergeByTokenSizeFromJSON_PartialOverlapPrefixCarriesOnlyTailPositions is
// a partial-overlap companion to
// TestMergeByTokenSizeFromJSON_OverlapPrefixCarriesPrevPositions (#18148).
// overlappedPct=100 (the full-overlap test) forces the ENTIRE previous chunk
// into the overlap prefix; here overlappedPct=20 means the overlap prefix is
// only the TAIL ~20% of the previous chunk. The coordinates carried must be
// exactly the previous chunk's tail items whose span intersects that tail --
// NOT the whole previous chunk. This locks the per-item tail-selection now
// done by overlapTailItems: a regression that carried the entire previous
// chunk's coordinates (over-inflating the highlight box) or dropped overlap
// coordinates entirely would both fail this test.
func TestMergeByTokenSizeFromJSON_PartialOverlapPrefixCarriesOnlyTailPositions(t *testing.T) {
	posA := json.RawMessage(`[[1,0,10,0,5]]`)
	posB := json.RawMessage(`[[2,0,20,0,8]]`)
	posC := json.RawMessage(`[[3,0,30,0,12]]`)
	posD := json.RawMessage(`[[4,0,40,0,16]]`)
	posE := json.RawMessage(`[[5,0,50,0,20]]`)
	posF := json.RawMessage(`[[6,0,60,0,24]]`)
	// 6 single-token items. With chunkTokens=9 (5 item tokens + 4 joinSep "\n"
	// tokens), items 0..4 merge into one chunk — the re-tokenize guard lets the
	// joined text fill the cap exactly (9 tokens), and item5 starts a fresh
	// chunk. Its overlap prefix (overlappedPct=20) is the last ~20% of the
	// 5-item previous chunk's text => only the last item ("e", posE) intersects
	// the tail. So chunk[1] must carry posE (tail) + posF (own), but NOT
	// posA/posB/posC/posD. (Texts are single-token so the re-tokenize guard's
	// actual-count check agrees with the declared TKNums.)
	items := [][]schema.ChunkDoc{
		{
			{Text: "a", DocType: "text", CKType: "text", TKNums: intPtr(1), PDFPositions: posA},
			{Text: "b", DocType: "text", CKType: "text", TKNums: intPtr(1), PDFPositions: posB},
			{Text: "c", DocType: "text", CKType: "text", TKNums: intPtr(1), PDFPositions: posC},
			{Text: "d", DocType: "text", CKType: "text", TKNums: intPtr(1), PDFPositions: posD},
			{Text: "e", DocType: "text", CKType: "text", TKNums: intPtr(1), PDFPositions: posE},
			{Text: "f", DocType: "text", CKType: "text", TKNums: intPtr(1), PDFPositions: posF},
		},
	}
	got := mergeByTokenSizeFromJSON(items, 9, 20)
	merged := got[0]
	if len(merged) != 2 {
		t.Fatalf("want 2 chunks (5 items merge, 6th starts fresh with partial overlap), got %d", len(merged))
	}

	// The new chunk's overlap text is the tail of the previous chunk.
	if !strings.Contains(merged[1].Text, "e") {
		t.Errorf("chunk[1] missing overlap tail text from prev chunk: text=%q", merged[1].Text)
	}
	// The tail item's coordinates MUST be carried.
	pdf := string(merged[1].PDFPositions)
	if !strings.Contains(pdf, "5,0,50,0,20") {
		t.Errorf("chunk[1] dropped tail-item coordinates (prev posE): pdf_positions=%s", pdf)
	}
	if !strings.Contains(pdf, "6,0,60,0,24") {
		t.Errorf("chunk[1] lost its own coordinates (posF): pdf_positions=%s", pdf)
	}
	// Partial overlap: the head items of the previous chunk must NOT be carried
	// (that would over-inflate the highlight box). A whole-prev carry bug or a
	// no-carry bug both fail here.
	for _, absent := range []string{"1,0,10,0,5", "2,0,20,0,8", "3,0,30,0,12", "4,0,40,0,16"} {
		if strings.Contains(pdf, absent) {
			t.Errorf("chunk[1] over-carried non-overlap head coordinates %s: pdf_positions=%s", absent, pdf)
		}
	}
}

// TestChunkFromItem_SlicesPositionsAcrossDelimiterPieces pins the
// chunk-screenshot-mismatch fix on the JSON delimiter-split path
// (chunkFromItem): an item whose text contains the active delimiter must NOT
// copy its whole bbox to every piece. Each piece's positions must be a
// vertical slice proportional to its rune share, and adjacent slices must
// abut so the pieces jointly tile the original box.
func TestChunkFromItem_SlicesPositionsAcrossDelimiterPieces(t *testing.T) {
	it := schema.ChunkDoc{
		Text:         "AAAA\nBBBB",
		DocType:      "text",
		CKType:       "text",
		PDFPositions: json.RawMessage(`[[1,0,200,0,40]]`),
	}
	got := chunkFromItem(it, compileDelimPattern([]string{"`\n`"}))
	if len(got) != 2 {
		t.Fatalf("want 2 delimiter-split pieces, got %d", len(got))
	}
	p0 := matrixOfRaw(t, got[0].PDFPositions)
	p1 := matrixOfRaw(t, got[1].PDFPositions)
	if len(p0) != 1 || len(p1) != 1 {
		t.Fatalf("single-row input must yield single-row slices: %v / %v", p0, p1)
	}
	if p0[0][3] != 0 || math.Abs(p0[0][4]-20) > 1e-9 {
		t.Errorf("piece 0 bounds = [%v,%v], want [0,20]", p0[0][3], p0[0][4])
	}
	if math.Abs(p1[0][3]-20) > 1e-9 || p1[0][4] != 40 {
		t.Errorf("piece 1 bounds = [%v,%v], want [20,40]", p1[0][3], p1[0][4])
	}
}

// TestSplitOversizedText_SlicedPositionsTrackPieceTextShare pins the core
// user-facing invariant of the screenshot-mismatch fix: each hard-split
// piece's cropped region height is proportional to its OWN text share (its
// rune count relative to all siblings), so the thumbnail matches the chunk
// text instead of showing the whole paragraph. The synthetic leading "\n"
// glue carries no visual height and must not inflate the first slice.
//
// This test uses REAL tokenizer behavior only to drive WHERE the oversized
// unit splits; the ratio assertions are derived from the emitted piece texts,
// so they hold regardless of cut points.
func TestSplitOversizedText_SlicedPositionsTrackPieceTextShare(t *testing.T) {
	const boxTop, boxBottom = 50.0, 80.0 // height 30
	build := func(text string) schema.ChunkDoc {
		return schema.ChunkDoc{
			Text:         text,
			DocType:      "text",
			CKType:       "text",
			TKNums:       intPtr(tokenizeStr(text)),
			PDFPositions: json.RawMessage(`[[1,10,200,50,80]]`),
		}
	}
	run := func(ck schema.ChunkDoc, lead bool) {
		t.Helper()
		got := splitOversizedText(ck, 30)
		if len(got) < 2 {
			t.Fatalf("oversized unit must be split, got %d piece(s)", len(got))
		}
		counts := make([]int, len(got))
		total := 0
		for i, p := range got {
			n := utf8.RuneCountInString(p.Text)
			if i == 0 && lead {
				n -= utf8.RuneCountInString("\n") // leading glue: no visual height
				if n < 0 {
					n = 0
				}
			}
			counts[i] = n
			total += n
		}
		prevBottom := boxTop
		for i, p := range got {
			m := matrixOfRaw(t, p.PDFPositions)
			if len(m) != 1 {
				t.Fatalf("piece %d: want one sliced row, got %v", i, m)
			}
			wantHeight := float64(counts[i]) / float64(total) * (boxBottom - boxTop)
			if math.Abs((m[0][4]-m[0][3])-wantHeight) > 1e-6*float64(len(got)) {
				t.Errorf("piece %d height = %v, want %v (text share %d/%d)",
					i, m[0][4]-m[0][3], wantHeight, counts[i], total)
			}
			if math.Abs(m[0][3]-prevBottom) > 1e-9 {
				t.Errorf("piece %d top = %v, want the previous bottom %v (slices must abut)", i, m[0][3], prevBottom)
			}
			prevBottom = m[0][4]
		}
		if math.Abs(prevBottom-boxBottom) > 1e-9 {
			t.Errorf("last piece bottom = %v, want %v (box fully covered)", prevBottom, boxBottom)
		}
	}

	body := strings.Repeat("word ", 100) // ~100 tokens, no sentence boundary
	// No lead glue: raw rune shares apply.
	run(build(body), false)
	// Text-path units are built as "\n"+paragraph (mergeByTokenSize); the
	// leading glue must be excluded from the first piece's share.
	run(build("\n"+body), true)
}

// TestChunkFromItem_SlicesPositionsIgnoresPositionTags ensures the
// position-slicing ratio is based on visible text. Parser tags
// (@@...##) carry no visual height and must not shift crop boundaries.
// This is the regression for the tag-aware fix (token.go:649).
func TestChunkFromItem_SlicesPositionsIgnoresPositionTags(t *testing.T) {
	// Visible "AAAA"(4) vs "CCCC"(4) => 0.5 split => [0,20]/[20,40].
	// Raw first piece contains a 16-rune tag; if counted it would shift
	// the boundary to ~33.3, reproducing the screenshot mismatch.
	it := schema.ChunkDoc{
		Text:         "AAAA@@1\t0\t200\t0\t40##\nCCCC",
		DocType:      "text",
		CKType:       "text",
		PDFPositions: json.RawMessage(`[[1,0,200,0,40]]`),
	}
	got := chunkFromItem(it, compileDelimPattern([]string{"`\n`"}))
	if len(got) != 2 {
		t.Fatalf("want 2 delimiter-split pieces, got %d", len(got))
	}
	p0 := matrixOfRaw(t, got[0].PDFPositions)
	p1 := matrixOfRaw(t, got[1].PDFPositions)
	if len(p0) != 1 || len(p1) != 1 {
		t.Fatalf("single-row input must yield single-row slices: %v / %v", p0, p1)
	}
	if math.Abs(p0[0][4]-20) > 1e-9 {
		t.Errorf("piece 0 bottom = %v, want 20 (tag runes must not shift ratio)", p0[0][4])
	}
	if math.Abs(p1[0][3]-20) > 1e-9 {
		t.Errorf("piece 1 top = %v, want 20 (tag runes must not shift ratio)", p1[0][3])
	}
}

// TestSplitOversizedText_SlicesPositionsIgnoreTags verifies the hard-cap
// path also uses tag-free visible runes for its ratio, mirroring the
// delimiter path. The leading "\n" glue and tags both carry no height.
func TestSplitOversizedText_SlicesPositionsIgnoreTags(t *testing.T) {
	const boxTop, boxBottom = 0.0, 40.0
	tag := "@@1\t0\t200\t0\t40##"
	body := strings.Repeat("word ", 30) + tag + strings.Repeat("word ", 30)
	ck := schema.ChunkDoc{
		Text:         body,
		DocType:      "text",
		CKType:       "text",
		TKNums:       intPtr(tokenizeStr(body)),
		PDFPositions: json.RawMessage(`[[1,0,200,0,40]]`),
	}
	got := splitOversizedText(ck, 30)
	if len(got) < 2 {
		t.Fatalf("oversized unit must be split, got %d piece(s)", len(got))
	}
	// Ratio must be computed from visible (tag-free) text.
	totalVisible := 0
	counts := make([]int, len(got))
	for i, p := range got {
		n := utf8.RuneCountInString(removeTag(p.Text))
		counts[i] = n
		totalVisible += n
	}
	prevBottom := boxTop
	for i, p := range got {
		m := matrixOfRaw(t, p.PDFPositions)
		if len(m) != 1 {
			t.Fatalf("piece %d: want one sliced row, got %v", i, m)
		}
		wantHeight := float64(counts[i]) / float64(totalVisible) * (boxBottom - boxTop)
		if math.Abs((m[0][4]-m[0][3])-wantHeight) > 1e-6*float64(len(got)) {
			t.Errorf("piece %d height = %v, want %v (visible share %d/%d)", i, m[0][4]-m[0][3], wantHeight, counts[i], totalVisible)
		}
		if math.Abs(m[0][3]-prevBottom) > 1e-9 {
			t.Errorf("piece %d top = %v, want previous bottom %v (slices must abut)", i, m[0][3], prevBottom)
		}
		prevBottom = m[0][4]
	}
	if math.Abs(prevBottom-boxBottom) > 1e-9 {
		t.Errorf("last piece bottom = %v, want %v (box fully covered)", prevBottom, boxBottom)
	}
}

// TestMergeByTokenSizeFromJSON_ChainedOverlapNoHeadAccumulation is the TDD test
// for the chained-overlap coordinate defect (CodeRabbit review #2 on PR #18227).
// The single-overlap tests above lock one overlap boundary, but when EVERY unit
// starts a fresh chunk (each unit over budget for the token cap with
// overlapped_percent>0) the overlap prefix is prepended at EVERY boundary. The
// buggy code stored each new chunk as ONE fused merged_item
// ({Text: overlap+cur, PDFPositions: <all prev-tail boxes>+cur}), so on the NEXT
// overlap the whole fused item was selected and the previous chunk's HEAD boxes
// were carried forward again -- accumulating {unit0, unit1, ...} coordinates into
// the final chunk while its text only contains the tail+cur.
//
// The fix keeps the tail items and cur as SEPARATE merged_items entries (with
// the tail item texts truncated to the overlap portion), so the next overlap
// selects only the true tail and the window converges to a sliding window
// instead of the full accumulation. This test fails (RED) until merged_items
// entries are stored separately.
func TestMergeByTokenSizeFromJSON_ChainedOverlapNoHeadAccumulation(t *testing.T) {
	pos := func(page int) json.RawMessage {
		return json.RawMessage(fmt.Sprintf("[[%d,0,%d,0,%d]]", page, page*10, page*5))
	}
	// Many single-token units so they merge into several chunks, each joined to
	// the previous one with a 20% overlap prefix. This exercises the CHAINED
	// overlap path (every chunk boundary prepends the previous chunk's tail):
	// the buggy fused single-item storage would carry the previous chunk's HEAD
	// coordinates forward on every overlap, accumulating the earliest units'
	// boxes into the final chunk. The fix stores tail items separately so the
	// window converges to a sliding window.
	const n = 24
	var row []schema.ChunkDoc
	for i := 0; i < n; i++ {
		row = append(row, schema.ChunkDoc{
			Text:         string(rune('a' + i)),
			DocType:      "text",
			CKType:       "text",
			TKNums:       intPtr(1),
			PDFPositions: pos(i + 1),
		})
	}
	items := [][]schema.ChunkDoc{row}
	got := mergeByTokenSizeFromJSON(items, 9, 20)
	merged := got[0]
	if len(merged) < 3 {
		t.Fatalf("want several chained chunks (>2), got %d", len(merged))
	}
	last := string(merged[len(merged)-1].PDFPositions)
	// The final chunk must NOT still carry unit0's (page 1) coordinates: the
	// overlap window converges to only the tail of the previous chunk + its own.
	if strings.Contains(last, "1,0,10,0,5") {
		t.Errorf("final chunk accumulated unit0 head coords (chained over-carry): pdf_positions=%s", last)
	}
	// Sanity: the final chunk must still carry the most recent unit's coords.
	if !strings.Contains(last, fmt.Sprintf("%d,0,%d,0,%d", n, n*10, n*5)) {
		t.Errorf("final chunk lost its own coordinates (unit%d): pdf_positions=%s", n, last)
	}
}

// TestOverlapTailItemsTruncatesToOverlapPortion locks the new behavior of
// overlapTailItems: each returned item's Text is truncated to exactly the span
// that lies within the overlap tail, and only items intersecting the tail are
// returned (the previous chunk's head items are excluded). This is what stops
// the chained over-carry: storing the full fused item would drag head
// coordinates into the next overlap.
func TestOverlapTailItemsTruncatesToOverlapPortion(t *testing.T) {
	prev := []mergeItem{
		{Text: "aaaaa", PDFPositions: json.RawMessage("[[1,0,10,0,5]]"), Positions: json.RawMessage("[[1,0,10,0,5]]")},
		{Text: "bbbbb", PDFPositions: json.RawMessage("[[2,0,20,0,8]]"), Positions: json.RawMessage("[[2,0,20,0,8]]")},
	}
	// total = 10 visible runes; overlap tail is [8,10).
	tail := overlapTailItems(prev, 8, "")
	if len(tail) != 1 {
		t.Fatalf("want 1 tail item in overlap [8,10), got %d: %+v", len(tail), tail)
	}
	if tail[0].Text != "bb" {
		t.Errorf("tail item Text not truncated to overlap portion: got %q, want %q", tail[0].Text, "bb")
	}
	if string(tail[0].PDFPositions) != "[[2,0,20,0,8]]" {
		t.Errorf("tail item carried wrong PDF positions: %s", tail[0].PDFPositions)
	}
	// The head item (page 1) must not appear in the tail.
	for _, it := range tail {
		if string(it.PDFPositions) == "[[1,0,10,0,5]]" {
			t.Errorf("head item (page1) leaked into overlap tail: %+v", it)
		}
	}
}

// TestOverlapTailItems_JoinSepNoSeparatorBeforeCur is the TDD (RED) test for the
// joinSep offset-model defect (CodeRabbit review #1 on PR #19068). The overlap
// path stores the previous chunk's merged_items as [tailItems..., cur] and builds
// cp.Text = overlap + cp.Text -- there is NO separator between the final tail
// item and cur. But overlapTailItems reconstructs the rune offsets by inserting
// joinSep before EVERY item, including before cur. On the JSON path (joinSep="\n")
// that phantom separator shifts the offsets by one rune, so the tail slice is
// truncated one rune too long (and, in a chained overlap, the window does not
// converge as tightly as the displayed text implies).
//
// This test feeds overlapTailItems the structure the overlap path actually
// produces -- tail item "aaaaa" followed by cur "bbbbb" with NO separator in the
// real text (10 visible runes), overlap tail [7,10) -- and expects the tail of
// cur to be truncated to exactly "bbb" (the last 3 runes). The buggy code inserts
// a "\n" between "aaaaa" and "bbbbb", inflating the model to 11 runes and
// returning "bbbb" instead. It fails (RED) until the offset model drops the
// separator before the final (cur) item.
func TestOverlapTailItems_JoinSepNoSeparatorBeforeCur(t *testing.T) {
	prev := []mergeItem{
		{Text: "aaaaa", PDFPositions: json.RawMessage("[[1,0,10,0,5]]"), Positions: json.RawMessage("[[1,0,10,0,5]]")},
		{Text: "bbbbb", PDFPositions: json.RawMessage("[[2,0,20,0,8]]"), Positions: json.RawMessage("[[2,0,20,0,8]]")},
	}
	// Real overlap-chunk text = "aaaaa" + "bbbbb" (10 visible runes, no sep).
	// Overlap tail is the last 3 runes [7,10) of cur.
	tail := overlapTailItems(prev, 7, "\n")
	if len(tail) != 1 {
		t.Fatalf("want 1 tail item in overlap [7,10), got %d: %+v", len(tail), tail)
	}
	if tail[0].Text != "bbb" {
		t.Errorf("tail item Text not truncated to overlap portion under joinSep: got %q, want %q (phantom separator shifted the offset)", tail[0].Text, "bbb")
	}
	if string(tail[0].PDFPositions) != "[[2,0,20,0,8]]" {
		t.Errorf("tail item carried wrong PDF positions: %s", tail[0].PDFPositions)
	}
	// The head item (page 1) must not appear in the tail.
	for _, it := range tail {
		if string(it.PDFPositions) == "[[1,0,10,0,5]]" {
			t.Errorf("head item (page1) leaked into overlap tail: %+v", it)
		}
	}
}

// TestOverlapTailItems_TagBearingUsesVisibleRuneOffsets pins that overlapTailItems
// measures and slices on the TAG-FREE visible text, so a coordinate tag in an
// earlier item does not shift the boundaries of later items (which would pull
// head coordinates into the overlap tail). This mirrors the contract already
// locked for the previous fused-item storage but for the separate-item storage.
func TestOverlapTailItems_TagBearingUsesVisibleRuneOffsets(t *testing.T) {
	prev := []mergeItem{
		// Visible text "hello world" (11 runes); the tag inflates the RAW length
		// to 23 runes. If the code measured RAW length, item0 would span [0,23)
		// and overlapStart=12 would fall inside it (leaking head coords). Using
		// the tag-free visible length, item0 spans [0,11) and item1 owns [11,15).
		{Text: "hello@@1\t2\t3\t4## world", PDFPositions: json.RawMessage(`[["p0"]]`), Positions: json.RawMessage(`[["q0"]]`)},
		{Text: "tail", PDFPositions: json.RawMessage(`[["p1"]]`), Positions: json.RawMessage(`[["q1"]]`)},
	}
	// Visible layout (no separator before cur, matching the overlap path):
	// item0 [0,11), item1 [11,15). overlapStart=12 lands inside item1 only, so
	// item0's head coordinates must NOT be included and item1 is truncated to
	// the overlap portion [12,15) == "ail" (the last 3 runes of "tail").
	tail := overlapTailItems(prev, 12, "\n")
	if len(tail) != 1 {
		t.Fatalf("want 1 tail item in overlap [12,15), got %d: %+v", len(tail), tail)
	}
	if string(tail[0].PDFPositions) != `[["p1"]]` {
		t.Errorf("pdf positions = %s, want only item1 [\"p1\"] (item0 head must be excluded)", tail[0].PDFPositions)
	}
	if tail[0].Text != "ail" {
		t.Errorf("tail item Text = %q, want %q (visible rune offsets must ignore the tag and truncate to the overlap portion)", tail[0].Text, "ail")
	}
}

// TestMergeByTokenSizeFromJSON_ChainedOverlapPerChunkConvergence strengthens the
// chained-overlap regression: not only must the FINAL chunk avoid head
// coordinates, EVERY chunk's carried PDF positions must contain only the true
// tail of its predecessor plus its own unit -- never any earlier (head) unit.
// This locks convergence at every boundary, not just at the end of the chain.
func TestMergeByTokenSizeFromJSON_ChainedOverlapPerChunkConvergence(t *testing.T) {
	pos := func(page int) json.RawMessage {
		return json.RawMessage(fmt.Sprintf("[[%d,0,%d,0,%d]]", page, page*10, page*5))
	}
	const n = 24
	var row []schema.ChunkDoc
	for i := 0; i < n; i++ {
		row = append(row, schema.ChunkDoc{
			Text:         string(rune('a' + i)),
			DocType:      "text",
			CKType:       "text",
			TKNums:       intPtr(1),
			PDFPositions: pos(i + 1),
		})
	}
	items := [][]schema.ChunkDoc{row}
	merged := mergeByTokenSizeFromJSON(items, 9, 20)[0]
	if len(merged) < 3 {
		t.Fatalf("want several chained chunks (>2), got %d", len(merged))
	}
	// The first chunk legitimately owns unit0 (page 1); every LATER chunk must
	// NOT carry it. The overlap window converges to only the previous chunk's
	// tail + the current unit, so the ultimate head (unit0) must never reappear
	// after chunk 0. unit1 (page 2) must not reappear after chunk 1 either.
	for k := 1; k < len(merged); k++ {
		p := string(merged[k].PDFPositions)
		if strings.Contains(p, "1,0,10,0,5") {
			t.Errorf("chunk %d carries stale head coords (unit0/page1): pdf_positions=%s", k, p)
		}
		if k >= 2 && strings.Contains(p, "2,0,20,0,8") {
			t.Errorf("chunk %d carries stale head coords (unit1/page2): pdf_positions=%s", k, p)
		}
	}
	// Sanity: the final chunk must still carry its own most-recent unit's coords.
	last := string(merged[len(merged)-1].PDFPositions)
	if !strings.Contains(last, fmt.Sprintf("%d,0,%d,0,%d", n, n*10, n*5)) {
		t.Errorf("final chunk lost its own coordinates (unit%d): pdf_positions=%s", n, last)
	}
}
