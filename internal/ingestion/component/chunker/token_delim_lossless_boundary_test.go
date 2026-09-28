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

// Boundary / whitespace-edge regression tests for the lossless delimiter
// contract. These pin behaviour the headline tests (token_delim_lossless_test.go)
// do NOT exercise:
//   - a chunk boundary that lands exactly on a retained delimiter (the text path
//     used to TrimSpace every merged chunk, dropping the boundary newline);
//   - whitespace-only pieces produced by repeated delimiters ("A\n\nB") in the
//     children-split paths, which used to be discarded;
//   - a leading delimiter / whitespace-only piece, which used to vanish from
//     the head of the first segment.

package chunker

import (
	"fmt"
	"strings"
	"testing"

	"ragflow/internal/ingestion/component/schema"
)

// TestDelimiterLossless_BoundaryReconstruction drives the TEXT path with a
// small chunk_token_size so several chunks form and every chunk boundary falls
// on a retained "\n" delimiter. Concatenating the emitted chunks must reproduce
// the source exactly — the boundary newline must survive (it used to be
// TrimSpaced away by mergeByTokenSize).
func TestDelimiterLossless_BoundaryReconstruction(t *testing.T) {
	lines := make([]string, 0, 12)
	for i := 1; i <= 12; i++ {
		lines = append(lines, fmt.Sprintf("This is sentence number %d of the document.", i))
	}
	src := strings.Join(lines, "\n")
	input := map[string]any{"name": "t", "output_format": "text", "text": src}
	chunks := driveChunker(t, map[string]any{
		"chunk_token_size": float64(24),
		"delimiter":        "\n",
	}, input)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks to exercise boundary delimiters, got %d", len(chunks))
	}
	got := joinChunks(chunks)
	if got != src {
		t.Errorf("boundary lossless reconstruction failed:\n got=%q\nwant=%q", got, src)
	}
}

// TestSplitByChildrenKeepsWhitespaceOnlyPiece pins that a whitespace-only piece
// from consecutive delimiters ("A\n\nB" -> ["A\n","\n","B\n"]) is folded into the
// adjacent child rather than dropped, so the child texts still reconstruct "A\n\nB".
func TestSplitByChildrenKeepsWhitespaceOnlyPiece(t *testing.T) {
	pat := compileDelimPattern([]string{"\n"})
	doc := schema.ChunkDoc{Text: "A\n\nB", DocType: "text", CKType: "text"}
	out := splitByChildren([]schema.ChunkDoc{doc}, pat)
	var got strings.Builder
	for _, c := range out {
		got.WriteString(c.Text)
	}
	if want := "A\n\nB"; got.String() != want {
		t.Errorf("splitByChildren dropped the repeated newline: got=%q want=%q (children=%v)",
			got.String(), want, childDocTexts(out))
	}
}

// TestApplyChildrenDelimTextKeepsWhitespaceOnlyPiece mirrors the above for the
// text-children split helper.
func TestApplyChildrenDelimTextKeepsWhitespaceOnlyPiece(t *testing.T) {
	pat := compileDelimPattern([]string{"\n"})
	doc := schema.ChunkDoc{Text: "A\n\nB", CKType: "text"}
	out := applyChildrenDelimText([]schema.ChunkDoc{doc}, pat)
	var got strings.Builder
	for _, c := range out {
		got.WriteString(c.Text)
	}
	if want := "A\n\nB"; got.String() != want {
		t.Errorf("applyChildrenDelimText dropped the repeated newline: got=%q want=%q (children=%v)",
			got.String(), want, childDocTexts(out))
	}
}

// TestChunkPerSegmentKeepsLeadingWhitespace pins that a source starting with a
// delimiter (custom backtick "\n") keeps its leading newline: "\nsecond line"
// must not become "second line".
func TestChunkPerSegmentKeepsLeadingWhitespace(t *testing.T) {
	input := map[string]any{"name": "t", "output_format": "text", "text": "\nsecond line"}
	chunks := driveChunker(t, map[string]any{
		"chunk_token_size": float64(512),
		"delimiters":       []string{`\n`},
	}, input)
	got := joinChunks(chunks)
	if want := "\nsecond line"; got != want {
		t.Errorf("leading newline dropped: got=%q want=%q", got, want)
	}
}

// childDocTexts is a small local helper for failure messages.
func childDocTexts(chunks []schema.ChunkDoc) []string {
	out := make([]string, 0, len(chunks))
	for _, c := range chunks {
		out = append(out, c.Text)
	}
	return out
}
