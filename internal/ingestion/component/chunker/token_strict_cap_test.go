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
	"unicode/utf8"

	"ragflow/internal/ingestion/component/schema"
)

// wordCount is a deterministic tokenizer stand-in used only via
// splitOversizedUnitWith in unit-level helper tests.
func wordCount(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	return len(strings.Fields(s))
}

func charCount(s string) int { return utf8.RuneCountInString(s) }

func TestSplitOversizedUnit_WhitespacePacksToBudget(t *testing.T) {
	// 100 words, budget 30 → must yield multiple pieces, each ≤ 30 words.
	text := strings.TrimSpace(strings.Repeat("word ", 100))
	pieces := splitOversizedUnitWith(text, 30, wordCount)
	if len(pieces) < 2 {
		t.Fatalf("want multiple pieces, got %d: %#v", len(pieces), pieces)
	}
	total := 0
	for _, p := range pieces {
		n := wordCount(p)
		if n > 30 {
			t.Errorf("piece exceeds budget: tokens=%d text=%q", n, p)
		}
		total += n
	}
	if total != 100 {
		t.Errorf("word count not preserved: got %d want 100", total)
	}
}

func TestSplitOversizedUnit_UnbrokenAtomFallsBackToCharWindows(t *testing.T) {
	// Unbroken run with char-as-token counting — must sub-split on runes.
	atom := strings.Repeat("a", 80)
	pieces := splitOversizedUnitWith(atom, 50, charCount)
	if len(pieces) < 2 {
		t.Fatalf("want >=2 pieces for unbroken atom, got %d", len(pieces))
	}
	joined := strings.Join(pieces, "")
	if joined != atom {
		t.Errorf("content not preserved: got %q", joined)
	}
	for _, p := range pieces {
		if charCount(p) > 50 {
			t.Errorf("piece exceeds budget: %d runes in %q", charCount(p), p)
		}
	}
}

func TestSplitOversizedUnit_WithinBudgetUnchanged(t *testing.T) {
	text := "hello world"
	pieces := splitOversizedUnitWith(text, 100, wordCount)
	if len(pieces) != 1 || pieces[0] != text {
		t.Fatalf("within-budget text must be returned as-is, got %#v", pieces)
	}
}

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
	got := mergeByTokenSizeFromJSON([][]schema.ChunkDoc{sections}, budget, 0)
	merged := got[0]
	if len(merged) < 3 {
		t.Fatalf("want >=3 chunks, got %d", len(merged))
	}
	for i, ck := range merged {
		n := tokenizeStr(ck.Text)
		if n > budget {
			t.Errorf("chunk %d exceeds budget: tokens=%d text_len=%d", i, n, len(ck.Text))
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
	got := mergeByTokenSizeFromJSON([][]schema.ChunkDoc{sections}, budget, 20)
	for i, ck := range got[0] {
		if n := tokenizeStr(ck.Text); n > budget {
			t.Errorf("chunk %d exceeds budget with overlap: tokens=%d", i, n)
		}
	}
}

func TestMergeByTokenSizeFromJSON_OversizedUnitIsSubSplit(t *testing.T) {
	// A single unit larger than the budget must be atom-split before merge.
	const budget = 30
	long := strings.TrimSpace(strings.Repeat("word ", 100))
	items := [][]schema.ChunkDoc{{
		{Text: long, DocType: "text", CKType: "text", TKNums: intPtr(tokenizeStr(long))},
	}}
	got := mergeByTokenSizeFromJSON(items, budget, 0)
	if len(got[0]) < 2 {
		t.Fatalf("oversized unit must yield multiple chunks, got %d", len(got[0]))
	}
	for i, ck := range got[0] {
		if n := tokenizeStr(ck.Text); n > budget {
			t.Errorf("chunk %d exceeds budget: tokens=%d", i, n)
		}
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
		if n := tokenizeStr(text); n > budget {
			t.Errorf("chunk %d exceeds budget: tokens=%d", i, n)
		}
	}
}

func TestMergeByTokenSize_UnbrokenAtomStrictCap(t *testing.T) {
	// Unbroken dense string (no whitespace / sentence delim) must still
	// hard-cap via the character-window fallback inside splitOversizedUnit.
	const budget = 20
	// Use many distinct ASCII letters so cl100k does not collapse the whole
	// run into a handful of tokens.
	var b strings.Builder
	for i := 0; i < 400; i++ {
		b.WriteByte(byte('a' + i%26))
	}
	text := b.String()
	if tokenizeStr(text) <= budget {
		t.Skipf("tokenizer collapsed unbroken atom to %d tokens (<= budget)", tokenizeStr(text))
	}
	comp, err := NewTokenChunker(map[string]any{
		"delimiter_mode":   "token_size",
		"chunk_token_size": budget,
	})
	if err != nil {
		t.Fatalf("NewTokenChunker: %v", err)
	}
	tc := comp.(*TokenChunkerComponent)
	out := tc.mergeByTokenSize(text, nil)
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) < 2 {
		t.Fatalf("want multiple chunks for unbroken atom, got %d (total_tokens=%d)", len(chunks), tokenizeStr(text))
	}
	var joined strings.Builder
	for i, ck := range chunks {
		s, _ := ck["text"].(string)
		joined.WriteString(s)
		if n := tokenizeStr(s); n > budget {
			t.Errorf("chunk %d exceeds budget: tokens=%d text=%q", i, n, s)
		}
	}
	// mergeByTokenSize prefixes "\n" on sections; stripping newlines recovers
	// the original unbroken atom.
	if strings.ReplaceAll(joined.String(), "\n", "") != text {
		t.Errorf("content not preserved after stripping newlines: got %q", joined.String())
	}
}

func TestInvokeTextPayload_StrictCapEndToEnd(t *testing.T) {
	const budget = 32
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
		if n := tokenizeStr(text); n > budget {
			t.Errorf("chunk %d exceeds budget: tokens=%d", i, n)
		}
	}
}
