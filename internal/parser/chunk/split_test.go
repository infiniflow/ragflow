//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package chunk

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// runLength builds a SplitOperator for the "length" strategy from a DSL-style
// config (numbers as float64, as JSON decoding produces) and runs it.
func runLength(t *testing.T, text string, chunkSize, overlap float64) []ChunkData {
	t.Helper()
	op, err := NewSplitOperator(map[string]interface{}{
		"strategy": "length",
		"params": map[string]interface{}{
			"chunk_size": chunkSize,
			"overlap":    overlap,
		},
	})
	if err != nil {
		t.Fatalf("NewSplitOperator returned error: %v", err)
	}
	ctx := &ChunkContext{TextAfterPreprocess: text}
	if err := op.Execute(ctx); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	return ctx.SplitChunks
}

func TestSplitByLengthBasicWindowing(t *testing.T) {
	chunks := runLength(t, "abcdefghij", 4, 0) // 10 runes, size 4, no overlap

	want := []string{"abcd", "efgh", "ij"}
	if len(chunks) != len(want) {
		t.Fatalf("got %d chunks, want %d: %#v", len(chunks), len(want), chunks)
	}
	for i, w := range want {
		if chunks[i].Content != w {
			t.Errorf("chunk %d = %q, want %q", i, chunks[i].Content, w)
		}
		if chunks[i].Index != i {
			t.Errorf("chunk %d Index = %d, want %d", i, chunks[i].Index, i)
		}
		if chunks[i].Size != utf8.RuneCountInString(w) {
			t.Errorf("chunk %d Size = %d, want %d", i, chunks[i].Size, utf8.RuneCountInString(w))
		}
	}
}

func TestSplitByLengthWithOverlap(t *testing.T) {
	chunks := runLength(t, "abcdefg", 4, 2) // size 4, overlap 2 -> step 2

	want := []string{"abcd", "cdef", "efg"}
	got := make([]string, len(chunks))
	for i, c := range chunks {
		got[i] = c.Content
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSplitByLengthOverlapClampedTerminates(t *testing.T) {
	// overlap >= chunk_size must be clamped so the window still advances and
	// the call terminates instead of looping forever.
	chunks := runLength(t, "abcde", 3, 9)
	if len(chunks) == 0 {
		t.Fatal("expected chunks, got none")
	}
	// step = chunkSize-(chunkSize-1) = 1, so windows advance one rune at a time.
	if chunks[0].Content != "abc" {
		t.Errorf("first chunk = %q, want %q", chunks[0].Content, "abc")
	}
	last := chunks[len(chunks)-1]
	if !strings.HasSuffix("abcde", last.Content) {
		t.Errorf("last chunk %q is not a suffix of the input", last.Content)
	}
}

func TestSplitByLengthDefaultSize(t *testing.T) {
	// chunk_size <= 0 falls back to the default window; short text => one chunk.
	chunks := runLength(t, "hello world", 0, 0)
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want 1", len(chunks))
	}
	if chunks[0].Content != "hello world" {
		t.Errorf("content = %q, want %q", chunks[0].Content, "hello world")
	}
}

func TestSplitByLengthRuneAware(t *testing.T) {
	// Multi-byte runes must be windowed by rune, never split mid-rune.
	text := "你好世界朋友" // 6 CJK runes, 3 bytes each
	chunks := runLength(t, text, 2, 0)

	want := []string{"你好", "世界", "朋友"}
	if len(chunks) != len(want) {
		t.Fatalf("got %d chunks, want %d", len(chunks), len(want))
	}
	for i, w := range want {
		if chunks[i].Content != w {
			t.Errorf("chunk %d = %q, want %q", i, chunks[i].Content, w)
		}
		if !utf8.ValidString(chunks[i].Content) {
			t.Errorf("chunk %d %q is not valid UTF-8 (rune was split)", i, chunks[i].Content)
		}
	}
}

func TestSplitByLengthNoOverlapReconstructs(t *testing.T) {
	// With no overlap, concatenating the chunks must reproduce the input exactly.
	text := "The quick brown fox jumps over the lazy dog."
	chunks := runLength(t, text, 7, 0)

	var sb strings.Builder
	for _, c := range chunks {
		sb.WriteString(c.Content)
	}
	if sb.String() != text {
		t.Errorf("reconstructed %q, want %q", sb.String(), text)
	}
}

func TestSplitByLengthEmptyInput(t *testing.T) {
	if chunks := runLength(t, "", 4, 0); len(chunks) != 0 {
		t.Errorf("empty input produced %d chunks, want 0", len(chunks))
	}
}

func TestNewSplitOperatorParsesLengthParams(t *testing.T) {
	op, err := NewSplitOperator(map[string]interface{}{
		"strategy": "length",
		"params": map[string]interface{}{
			"chunk_size": float64(128),
			"overlap":    float64(16),
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if op.chunkSize != 128 {
		t.Errorf("chunkSize = %d, want 128", op.chunkSize)
	}
	if op.overlap != 16 {
		t.Errorf("overlap = %d, want 16", op.overlap)
	}
}
