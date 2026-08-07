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

// TestMergeByTokenSizeFromJSON_TKNumsConsistency exercises the JSON merge path
// with overlap > 0, which previously had NO coverage, and pins the TKNums
// accounting so the running-sum merge decision cannot silently regress to the
// old re-tokenized-join count.
//
// Fixture: three text units a/b/c with a and b small enough to each fit the
// budget but a+b's running sum overflows it (so a+b merge-then-close into
// chunk0), and c starts a fresh chunk (prevClosed) carrying an overlap prefix
// from chunk0.
//
// TKNums accounting (see token.go mergeByTokenSizeFromJSON):
//   - on the merge path (chunk0) TKNums is the RUNNING SUM of per-unit counts
//     (aN+bN), never tokenizeStr(chunk0.Text);
//   - on the boundary path (chunk1, via prevClosed) TKNums is reset to
//     tokenizeStr(chunk1.Text) — i.e. the two paths use different caliber.
//     This mixed accounting is documented in token.go; the assertions below
//     lock the current behavior so any future unification is a deliberate
//     change, not a silent drift.
func TestMergeByTokenSizeFromJSON_TKNumsConsistency(t *testing.T) {
	for _, overlapPct := range []float64{0, 30} {
		t.Run(overlapName(overlapPct), func(t *testing.T) {
			aText := strings.Repeat("word ", 18)
			bText := strings.Repeat("word ", 18)
			cText := strings.Repeat("word ", 6)
			aN, bN, cN := tokenizeStr(aText), tokenizeStr(bText), tokenizeStr(cText)
			// Budget just below the a+b running sum so a and b cannot merge
			// without overflowing (forcing mergeThenClose on chunk0), while a
			// alone and c alone fit.
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
			got := mergeByTokenSizeFromJSON(items, budget, overlapPct, schema.MergeOverCap)
			merged := got[0]
			if len(merged) != 2 {
				t.Fatalf("want 2 chunks (overflow-closed + overlap/fresh chunk), got %d (a=%d b=%d c=%d budget=%d)", len(merged), aN, bN, cN, budget)
			}

			// chunk0 is the merge-then-close of a and b.
			if merged[0].Text != aText+"\n"+bText {
				t.Errorf("chunk0 text mismatch:\n got=%q\nwant=%q", merged[0].Text, aText+"\n"+bText)
			}
			// Merge path: TKNums is the running sum, not the re-tokenized text.
			if got0 := intValue(merged[0].TKNums); got0 != aN+bN {
				t.Errorf("chunk0 TKNums: running sum want %d, got %d (tokenizeStr(chunk0.Text)=%d)", aN+bN, got0, tokenizeStr(merged[0].Text))
			}

			// chunk1 is c (fresh, via prevClosed). With overlap>0 it carries a
			// prefix carved from chunk0; verify the overlap path actually ran.
			if overlapPct > 0 {
				overlap, _ := computeOverlapPrefix(merged[0].Text, overlapPct)
				if overlap == "" {
					t.Fatal("expected a non-empty overlap prefix from chunk0")
				}
				if !strings.HasPrefix(merged[1].Text, overlap) {
					t.Errorf("chunk1 should start with overlap prefix %q, got %q", overlap, merged[1].Text)
				}
			}
			// Boundary path: TKNums is the re-tokenized chunk1 text count.
			if got1 := intValue(merged[1].TKNums); got1 != tokenizeStr(merged[1].Text) {
				t.Errorf("chunk1 TKNums: want tokenizeStr(chunk1.Text)=%d, got %d", tokenizeStr(merged[1].Text), got1)
			}
		})
	}
}

func overlapName(p float64) string {
	if p == 0 {
		return "overlap0"
	}
	return "overlap30"
}
