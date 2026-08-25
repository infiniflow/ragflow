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

// Hard-cap contract tests: no text chunk may exceed the token target. The
// merge decision uses the RUNNING SUM of per-unit counts (#17948), so the
// assertions below pin that sum; the end-to-end tests additionally assert the
// re-tokenized emitted text stays within the budget for their fixtures.

func TestMergeByTokenSizeFromJSON_HardCapNoOvershoot(t *testing.T) {
	// Eight 25-token-ish sections under a 50-token budget must pack without
	// any chunk's running sum exceeding the budget (no one-unit overflow).
	const budget = 50
	sections := make([]schema.ChunkDoc, 0, 8)
	for i := 0; i < 8; i++ {
		text := strings.TrimSpace(strings.Repeat("w ", 25))
		sections = append(sections, schema.ChunkDoc{
			Text: text, DocType: "text", CKType: "text", TKNums: intPtr(tokenizeStr(text)),
		})
	}
	got := mergeByTokenSizeFromJSON([][]schema.ChunkDoc{sections}, budget, 0)
	merged := got[0]
	if len(merged) < 3 {
		t.Fatalf("want >=3 chunks, got %d", len(merged))
	}
	for i, ck := range merged {
		if n := intValue(ck.TKNums); n > budget {
			t.Errorf("chunk %d running sum %d exceeds budget %d", i, n, budget)
		}
	}
}

func TestMergeByTokenSizeFromJSON_OverlapStrictCap(t *testing.T) {
	// With overlap the overlap prefix is trimmed so it never pushes a chunk
	// over the budget.
	const budget = 25
	sections := make([]schema.ChunkDoc, 0, 20)
	for i := 0; i < 20; i++ {
		text := strings.TrimSpace(strings.Repeat("w ", 10))
		sections = append(sections, schema.ChunkDoc{
			Text: text, DocType: "text", CKType: "text", TKNums: intPtr(tokenizeStr(text)),
		})
	}
	got := mergeByTokenSizeFromJSON([][]schema.ChunkDoc{sections}, budget, 20)
	for i, ck := range got[0] {
		if n := intValue(ck.TKNums); n > budget {
			t.Errorf("chunk %d exceeds budget with overlap: tokens=%d (cap=%d)", i, n, budget)
		}
	}
}

func TestMergeByTokenSizeFromJSON_OversizedUnitSplit(t *testing.T) {
	// A single over-budget unit is no longer kept whole (#17799 replaced by the
	// hard cap): it is re-split into <= budget pieces and the concatenated
	// pieces reproduce the original text exactly (lossless).
	const budget = 30
	long := strings.TrimSpace(strings.Repeat("word ", 100))
	items := [][]schema.ChunkDoc{{
		{Text: long, DocType: "text", CKType: "text", TKNums: intPtr(tokenizeStr(long))},
	}}
	got := mergeByTokenSizeFromJSON(items, budget, 0)
	merged := got[0]
	if len(merged) < 2 {
		t.Fatalf("oversized unit must be split, got %d chunk(s)", len(merged))
	}
	var joined string
	for i, ck := range merged {
		if n := tokenizeStr(ck.Text); n > budget {
			t.Errorf("chunk %d exceeds budget: tokens=%d (cap=%d)", i, n, budget)
		}
		joined += ck.Text
	}
	if joined != long {
		t.Errorf("split not lossless:\n got %q\nwant %q", joined, long)
	}
}

func TestMergeByTokenSize_TextPathStrictCap(t *testing.T) {
	// End-to-end text path: long multi-paragraph input under a tight budget.
	const budget = 40
	var b strings.Builder
	for i := 0; i < 30; i++ {
		b.WriteString(strings.TrimSpace(strings.Repeat("word ", 15)))
		b.WriteString("\n\n")
	}
	comp, err := NewTokenChunker(map[string]any{
		"delimiter_mode":   "delimiter",
		"chunk_token_size": budget,
	})
	if err != nil {
		t.Fatalf("NewTokenChunker: %v", err)
	}
	tc := comp.(*TokenChunkerComponent)
	out := tc.mergeByTokenSize(b.String(), nil, nil)
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) < 2 {
		t.Fatalf("want multiple chunks, got %d", len(chunks))
	}
	for i, ck := range chunks {
		text, _ := ck["text"].(string)
		if n := tokenizeStr(text); n > budget {
			t.Errorf("chunk %d exceeds budget: tokens=%d (cap=%d)", i, n, budget)
		}
	}
}

func TestMergeByTokenSize_OversizedBoundarylessRunSplit(t *testing.T) {
	// A single boundary-less run longer than chunk_token_size must be hard
	// token-split into <= budget pieces (never kept whole).
	const budget = 30
	long := strings.TrimSpace(strings.Repeat("word ", 100))
	comp, err := NewTokenChunker(map[string]any{
		"delimiter_mode":   "delimiter",
		"chunk_token_size": budget,
	})
	if err != nil {
		t.Fatalf("NewTokenChunker: %v", err)
	}
	out := comp.(*TokenChunkerComponent).mergeByTokenSize(long, nil, nil)
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) < 2 {
		t.Fatalf("oversized run must be split, got %d chunk(s)", len(chunks))
	}
	for i, ck := range chunks {
		text, _ := ck["text"].(string)
		if n := tokenizeStr(text); n > budget {
			t.Errorf("chunk %d exceeds budget: tokens=%d (cap=%d)", i, n, budget)
		}
	}
}

func TestInvokeTextPayload_HardCapEndToEnd(t *testing.T) {
	const budget = 32
	var b strings.Builder
	for i := 0; i < 20; i++ {
		b.WriteString(strings.TrimSpace(strings.Repeat("alpha ", 12)))
		b.WriteByte('\n')
	}
	comp, err := NewTokenChunker(map[string]any{
		"delimiter_mode":   "delimiter",
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
		if n := tokenizeStr(text); n > budget {
			t.Errorf("chunk %d exceeds budget: tokens=%d (cap=%d)", i, n, budget)
		}
	}
}

func TestInvokeJSONPayload_HardCapEndToEnd(t *testing.T) {
	const budget = 32
	var items []map[string]any
	for i := 0; i < 24; i++ {
		items = append(items, map[string]any{
			"text":         strings.TrimSpace(strings.Repeat("alpha ", 12)),
			"doc_type_kwd": "text",
		})
	}
	comp, err := NewTokenChunker(map[string]any{
		"delimiter_mode":   "delimiter",
		"delimiters":       []string{"\n"},
		"chunk_token_size": budget,
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
	if len(chunks) < 2 {
		t.Fatalf("want multiple chunks, got %d", len(chunks))
	}
	for i, ck := range chunks {
		text, _ := ck["text"].(string)
		if n := tokenizeStr(text); n > budget {
			t.Errorf("chunk %d exceeds budget: tokens=%d (cap=%d)", i, n, budget)
		}
	}
}

// TestMergeUnits_UnderCapBoundaryDecision pins the boundary semantics of the
// hard-cap merge independent of BPE text-token-count fluctuations: an exact-fit
// join (prev+incoming == target) merges; an overflowing join starts a fresh
// chunk; an oversized incoming unit is expanded into <= target pieces instead of
// standing alone.
func TestMergeUnits_UnderCapBoundaryDecision(t *testing.T) {
	units := []schema.ChunkDoc{
		{Text: "prev text", TKNums: intPtr(10), CKType: "text"},
		{Text: "incoming text", TKNums: intPtr(10), CKType: "text"},
	}
	// prev+incoming == target: merge into one chunk.
	got := mergeUnits(units, 20, 0, "\n")
	if len(got) != 1 {
		t.Fatalf("exact-fit join: want 1 merged chunk, got %d", len(got))
	}
	if sum := intValue(got[0].TKNums); sum != 20 {
		t.Errorf("merged chunk running sum: want 20, got %d", sum)
	}

	// prev+incoming > target: fresh chunk, no overflow.
	over := mergeUnits(units, 19, 0, "\n")
	if len(over) != 2 {
		t.Fatalf("overflowing join: want 2 chunks, got %d", len(over))
	}

	// An oversized incoming unit is expanded (split) instead of standing whole.
	big := []schema.ChunkDoc{
		{Text: "prev text", TKNums: intPtr(10), CKType: "text"},
		{Text: strings.Repeat("word ", 100), TKNums: intPtr(100), CKType: "text"},
	}
	bigMerged := mergeUnits(big, 30, 0, "\n")
	if len(bigMerged) < 2 {
		t.Fatalf("oversized incoming unit must be split, got %d chunk(s)", len(bigMerged))
	}
	for i, ck := range bigMerged {
		if n := intValue(ck.TKNums); n > 30 {
			t.Errorf("chunk %d exceeds target: tokens=%d", i, n)
		}
	}
}
