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
func TestMergeByTokenSizeFromJSON_OverlapStripsTags(t *testing.T) {
	aText := strings.Repeat("word ", 20) + "@@1\t2.3## tail"
	items := [][]schema.ChunkDoc{
		{
			{Text: aText, DocType: "text", CKType: "text", TKNums: intPtr(100)},
			{Text: "body", DocType: "text", CKType: "text", TKNums: intPtr(5)},
		},
	}
	got := mergeByTokenSizeFromJSON(items, 128, 0.3)
	merged := got[0]
	if len(merged) != 2 {
		t.Fatalf("want 2 merged chunks (overlap path), got %d", len(merged))
	}
	// The overlap prefix is prepended to the SECOND chunk. The original
	// first chunk legitimately keeps its own parser tag; only the overlap
	// region (merged[1]) must be tag-free (diff Chunker-2.2).
	if strings.Contains(merged[1].Text, "@@") || strings.Contains(merged[1].Text, "##") {
		t.Errorf("overlap prefix leaked parser tag into chunk 1: %q", merged[1].Text)
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
	got := mergeByTokenSizeFromJSON(items, 128, 0)
	merged := got[0]
	if len(merged) != 1 {
		t.Fatalf("want 1 merged chunk, got %d", len(merged))
	}
	if merged[0].Text != "keepme" {
		t.Errorf("empty previous chunk dropped incoming text; got %q", merged[0].Text)
	}
}

// TestTakeFromEndRespectsTokenCount and TestTakeFromStartRespectsTokenCount
// exercise migration diff Chunker-2.4: takeFromEnd/takeFromStart used a
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
