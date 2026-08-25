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
// with overlap > 0 and pins the TKNums accounting so the running-sum merge
// decision cannot silently regress to the old re-tokenized-join count.
//
// Fixture: three text units a/b/c with a+b fitting the budget (merge path into
// chunk0) and a+b+c overflowing it (so c starts a fresh chunk carrying an
// overlap prefix from chunk0).
//
// TKNums accounting (see token.go mergeByTokenSizeFromJSON):
//   - on the merge path (chunk0) TKNums is the RUNNING SUM of per-unit counts
//     (aN+bN), never tokenizeStr(chunk0.Text);
//   - on the boundary path (chunk1, fresh) TKNums is reset to
//     tokenizeStr(chunk1.Text) — i.e. the two paths use different caliber.
//     This mixed accounting is documented in token.go; the assertions below
//     lock the current behavior so any future unification is a deliberate
//     change, not a silent drift.
func TestMergeByTokenSizeFromJSON_TKNumsConsistency(t *testing.T) {
	for _, overlapPct := range []float64{0, 30} {
		t.Run(overlapName(overlapPct), func(t *testing.T) {
			aText := strings.Repeat("word ", 6)
			bText := strings.Repeat("word ", 6)
			cText := strings.Repeat("word ", 6)
			aN, bN, cN := tokenizeStr(aText), tokenizeStr(bText), tokenizeStr(cText)
			// Budget: a+b fits (merge), a+b+c overflows (fresh chunk), c alone
			// fits. For overlap>0 the overlap prefix is trimmed to fit, so the
			// exact prefix length varies; we only pin the accounting, not the
			// prefix.
			budget := aN + bN
			if budget < cN {
				budget = cN
			}
			if aN+bN+cN <= budget {
				t.Fatalf("could not derive tight budget (a=%d b=%d c=%d sum=%d budget=%d)", aN, bN, cN, aN+bN+cN, budget)
			}
			items := [][]schema.ChunkDoc{
				{
					{Text: aText, DocType: "text", CKType: "text", TKNums: intPtr(aN)},
					{Text: bText, DocType: "text", CKType: "text", TKNums: intPtr(bN)},
					{Text: cText, DocType: "text", CKType: "text", TKNums: intPtr(cN)},
				},
			}
			got := mergeByTokenSizeFromJSON(items, budget, overlapPct)
			merged := got[0]
			if len(merged) != 2 {
				t.Fatalf("want 2 chunks (merged a+b + fresh c), got %d (a=%d b=%d c=%d budget=%d)", len(merged), aN, bN, cN, budget)
			}

			// chunk0 is the merge of a and b.
			if merged[0].Text != aText+"\n"+bText {
				t.Errorf("chunk0 text mismatch:\n got=%q\nwant=%q", merged[0].Text, aText+"\n"+bText)
			}
			// Merge path: TKNums is the running sum, not the re-tokenized text.
			if got0 := intValue(merged[0].TKNums); got0 != aN+bN {
				t.Errorf("chunk0 TKNums: running sum want %d, got %d (tokenizeStr(chunk0.Text)=%d)", aN+bN, got0, tokenizeStr(merged[0].Text))
			}

			// chunk1 is c (fresh). With overlap>0 it carries a prefix carved
			// from chunk0 (trimmed to fit the hard cap); c's text must still
			// be present as the suffix.
			if overlapPct > 0 {
				if !strings.HasSuffix(merged[1].Text, cText) {
					t.Errorf("chunk1 should end with c text %q, got %q", cText, merged[1].Text)
				}
			}
			// Boundary path: TKNums is the re-tokenized chunk1 text count.
			if got1 := intValue(merged[1].TKNums); got1 != tokenizeStr(merged[1].Text) {
				t.Errorf("chunk1 TKNums: want tokenizeStr(chunk1.Text)=%d, got %d", tokenizeStr(merged[1].Text), got1)
			}
			if n := intValue(merged[1].TKNums); n > budget {
				t.Errorf("chunk1 exceeds budget after overlap trim: tokens=%d (cap=%d)", n, budget)
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
