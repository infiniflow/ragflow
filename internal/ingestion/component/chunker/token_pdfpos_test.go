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
	"strings"
	"testing"

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
