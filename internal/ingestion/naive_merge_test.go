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

package ingestion

import (
	"strings"
	"testing"

	"ragflow/internal/tokenizer"
)

// naiveMergeTests defines a table-driven test case for NaiveMerge.
type naiveMergeTests struct {
	name             string
	texts            []string
	chunkTokenNum    int
	delimiter        string
	overlappedPct    float64

	// Expectations
	minChunks   int     // at least this many chunks
	maxChunks   int     // at most this many chunks
	maxTokens   int     // no single chunk should exceed this token count
	contains    []string // each chunk should contain these substrings
	notContains []string // no chunk should contain these
}

func TestNaiveMerge_TableDriven(t *testing.T) {
	tests := []naiveMergeTests{
		// ── Empty & nil inputs ──────────────────────────────────────────
		{
			name:          "nil texts",
			texts:         nil,
			chunkTokenNum: 128,
			delimiter:     "\n。；！？",
			overlappedPct: 0,
			maxChunks:     0,
		},
		{
			name:          "empty texts",
			texts:         []string{},
			chunkTokenNum: 128,
			delimiter:     "\n。；！？",
			overlappedPct: 0,
			maxChunks:     0,
		},

		// ── Single short text ───────────────────────────────────────────
		{
			name:          "single short text",
			texts:         []string{"Hello World"},
			chunkTokenNum: 128,
			delimiter:     "\n。；！？",
			overlappedPct: 0,
			minChunks:     1,
			maxChunks:     1,
			contains:      []string{"Hello World"},
		},

		// ── Multiple short texts merged into one chunk ──────────────────
		{
			name:          "multiple short texts merge into one",
			texts:         []string{"Short A", "Short B", "Short C"},
			chunkTokenNum: 128,
			delimiter:     "\n。；！？",
			overlappedPct: 0,
			minChunks:     1,
			maxChunks:     1,
			contains:      []string{"Short A", "Short B", "Short C"},
		},

		// ── Large text split at delimiters ──────────────────────────────
		{
			name:          "long text split at delimiter",
			texts:         []string{strings.Repeat("A。", 100)}, // 100 Chinese sentences
			chunkTokenNum: 128,
			delimiter:     "\n。；！？",
			overlappedPct: 0,
			minChunks:     2, // should be split at '。'
			maxChunks:     20,
		},

		// ── Chinese delimiters ──────────────────────────────────────────
		{
			name:          "chinese sentence delimiters",
			texts:         []string{"第一句话。第二句话！第三句话？"},
			chunkTokenNum: 128,
			delimiter:     "\n。；！？",
			overlappedPct: 0,
			minChunks:     1,
			maxChunks:     3,
		},

		// ── Overlap ─────────────────────────────────────────────────────
		{
			name: "overlap between chunks",
			texts: []string{
				strings.Repeat("A", 100) + "。",
				strings.Repeat("B", 100) + "。",
				strings.Repeat("C", 100) + "。",
			},
			chunkTokenNum: 50,  // small enough to force multiple chunks
			delimiter:     "\n。；！？",
			overlappedPct: 20,  // 20% overlap
			minChunks:     2,
			maxChunks:     10,
		},

		// ── Custom delimiter (backtick-quoted) ──────────────────────────
		{
			name:          "custom backtick delimiter",
			texts:         []string{"Section 1 `CHAPTER` Section 2"},
			chunkTokenNum: 128,
			delimiter:     "`CHAPTER`\n。；！？",
			overlappedPct: 0,
			minChunks:     2,
			maxChunks:     2,
			contains:      []string{"Section 1", "Section 2"},
			notContains:   []string{"CHAPTER"},
		},

		// ── Empty delimiter — no splitting ─────────────────────────────
		{
			name:          "empty delimiter",
			texts:         []string{"Keep As Is Without Split"},
			chunkTokenNum: 128,
			delimiter:     "",
			overlappedPct: 0,
			minChunks:     1,
			maxChunks:     1,
		},

		// ── Multiple texts, some oversized, some not ────────────────────
		{
			name: "mixed sizes",
			texts: []string{
				"Tiny。",
				strings.Repeat("Long text that exceeds the token limit and should be split。", 20),
				"Another tiny。",
			},
			chunkTokenNum: 128,
			delimiter:     "\n。；！？",
			overlappedPct: 0,
			minChunks:     2, // oversized text split, small ones may merge
			maxChunks:     30,
		},

		// ── All texts empty strings ─────────────────────────────────────
		{
			name:          "all empty strings",
			texts:         []string{"", "", ""},
			chunkTokenNum: 128,
			delimiter:     "\n。；！？",
			overlappedPct: 0,
			maxChunks:     0,
		},

		// ── Very small chunk_token_num forces many chunks ───────────────
		{
			name:          "tiny token limit",
			texts:         []string{"一。二。三。四。五。六。"},
			chunkTokenNum: 5,
			delimiter:     "\n。；！？",
			overlappedPct: 0,
			minChunks:     2, // should be split into multiple chunks
			maxChunks:     10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := NaiveMerge(tt.texts, tt.chunkTokenNum, tt.delimiter, tt.overlappedPct)

			// Check min/max count
			if len(chunks) < tt.minChunks {
				t.Errorf("got %d chunks, want at least %d", len(chunks), tt.minChunks)
			}
			if len(chunks) > tt.maxChunks {
				t.Errorf("got %d chunks, want at most %d", len(chunks), tt.maxChunks)
			}

			// Check max tokens per chunk
			if tt.maxTokens > 0 {
				for i, c := range chunks {
					tk := tokenizer.NumTokensFromString(c.Text)
					if tk > tt.maxTokens {
						t.Errorf("chunk[%d] has %d tokens, exceeds max %d: %q", i, tk, tt.maxTokens, c.Text)
					}
				}
			}

			// Check contains
			for _, substr := range tt.contains {
				found := false
				for _, c := range chunks {
					if strings.Contains(c.Text, substr) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("no chunk contains %q", substr)
				}
			}

			// Check not contains
			for _, substr := range tt.notContains {
				for _, c := range chunks {
					if strings.Contains(c.Text, substr) {
						t.Errorf("chunk should not contain %q: %q", substr, c.Text)
					}
				}
			}
		})
	}
}

// ── Specific behavior tests ────────────────────────────────────────────────

func TestNaiveMerge_OverlapContent(t *testing.T) {
	// Create texts where we can verify overlap behavior.
	// Each text is a complete sentence with delimiter.
	texts := []string{
		"AAAAA。",
		"BBBBB。",
		"CCCCC。",
		"DDDDD。",
	}
	chunks := NaiveMerge(texts, 10, "\n。；！？", 30) // 30% overlap

	// With token limit 10 and 30% overlap, chunks should reuse end of previous.
	// Verify we got chunks and they look reasonable.
	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks with overlap, got %d", len(chunks))
	}

	// Verify chunks are non-empty and contain content
	for i, c := range chunks {
		if c.Text == "" {
			t.Errorf("chunk[%d] is empty", i)
		}
	}
}

func TestNaiveMerge_DelimiterAsSplitPoint(t *testing.T) {
	// Delimiters are split points for oversized texts (>chunkTokenNum).
	// For texts under the token limit, no splitting occurs.
	// This test verifies that a large text IS split at delimiters.
	texts := []string{strings.Repeat("A。", 100)} // many sentences, over token limit
	chunks := NaiveMerge(texts, 20, "\n。；！？", 0)

	// Should be split into multiple chunks (token limit 20 is small)
	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
	}
	// No chunk should contain '。' (delimiter is removed)
	for _, c := range chunks {
		if strings.Contains(c.Text, "。") {
			t.Errorf("chunk should not contain delimiter '。': %q", c.Text)
		}
	}
}

func TestNaiveMerge_AllBelowTokenThreshold(t *testing.T) {
	// All texts below chunk_token_num → should merge into one chunk.
	texts := []string{"A", "B", "C", "D", "E"}
	chunks := NaiveMerge(texts, 128, "\n。；！？", 0)

	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	// Should contain all
	for _, letter := range []string{"A", "B", "C", "D", "E"} {
		if !strings.Contains(chunks[0].Text, letter) {
			t.Errorf("chunk missing %q", letter)
		}
	}
}

func TestNaiveMerge_PositionInfo(t *testing.T) {
	// Texts with position info should have positions carried through.
	texts := []string{"Page 1 content。", "Page 2 content。"}
	chunks := NaiveMerge(texts, 128, "\n。；！？", 0)

	// Verify SectionIndices are tracked
	for _, c := range chunks {
		if len(c.SectionIndices) == 0 {
			t.Error("chunk has no SectionIndices")
		}
		// Section indices should be within range
		for _, idx := range c.SectionIndices {
			if idx < 0 || idx >= len(texts) {
				t.Errorf("SectionIndex %d out of range [0, %d)", idx, len(texts))
			}
		}
	}
}

func TestNaiveMerge_CustomDelimiterExclusive(t *testing.T) {
	// Custom delimiter mode: each segment is its own chunk, no merging.
	texts := []string{"Part A `---` Part B `---` Part C"}
	chunks := NaiveMerge(texts, 128, "`---`", 0)

	// Should produce 3 chunks (one per segment between ---)
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks from custom delimiter split, got %d", len(chunks))
	}
	// No chunk should contain "---"
	for _, c := range chunks {
		if strings.Contains(c.Text, "---") {
			t.Errorf("chunk should not contain custom delimiter: %q", c.Text)
		}
	}
}

func TestNaiveMerge_LeadingNewline(t *testing.T) {
	// Python adds "\n" prefix to each text before adding as chunk.
	texts := []string{"Hello World"}
	chunks := NaiveMerge(texts, 128, "\n。；！？", 0)

	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	// Should start with "\n" (matching Python behavior)
	if !strings.HasPrefix(chunks[0].Text, "\n") {
		t.Errorf("expected chunk to start with newline, got %q", chunks[0].Text)
	}
}

func TestNaiveMerge_SectionIndicesCorrect(t *testing.T) {
	// Verify that merged chunks track which input sections they came from.
	texts := []string{"First。", "Second。", "Third。", "Fourth。"}
	chunks := NaiveMerge(texts, 40, "\n。；！？", 0)

	// Collect all section indices across all chunks
	allIndices := make(map[int]bool)
	for _, c := range chunks {
		for _, idx := range c.SectionIndices {
			allIndices[idx] = true
		}
	}

	// Every input section should appear in exactly one chunk
	for i := range texts {
		if !allIndices[i] {
			t.Errorf("section %d (%q) not found in any chunk", i, texts[i])
		}
	}
}
