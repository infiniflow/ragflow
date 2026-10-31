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

// runChar builds a SplitOperator for the "char" strategy and runs it.
func runChar(t *testing.T, text string) []ChunkData {
	t.Helper()
	op, err := NewSplitOperator(map[string]interface{}{"strategy": "char"})
	if err != nil {
		t.Fatalf("NewSplitOperator returned error: %v", err)
	}
	ctx := &ChunkContext{TextAfterPreprocess: text}
	if err := op.Execute(ctx); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	return ctx.SplitChunks
}

func TestSplitByCharContentAndIndex(t *testing.T) {
	chunks := runChar(t, "中文ab")

	want := []string{"中", "文", "a", "b"}
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
		if !utf8.ValidString(chunks[i].Content) {
			t.Errorf("chunk %d %q is not valid UTF-8", i, chunks[i].Content)
		}
	}
}

func TestSplitByCharLanguageDescribesTheChunk(t *testing.T) {
	// The language metadata belongs to the chunk, not to the text that is
	// still unconsumed: "文" used to come out as "en" because the suffix
	// "文abc" holds one Han character against three Latin letters.
	chunks := runChar(t, "中文abc")

	want := []string{"zh", "zh", "en", "en", "en"}
	if len(chunks) != len(want) {
		t.Fatalf("got %d chunks, want %d", len(chunks), len(want))
	}
	for i, w := range want {
		if got, ok := chunks[i].Metadata["language"]; !ok || got != w {
			t.Errorf("chunk %d (%q) language = %v, want %q", i, chunks[i].Content, got, w)
		}
	}
}

func BenchmarkSplitByChar(b *testing.B) {
	// Guards the cost of the strategy: scanning the unconsumed text once per
	// rune made it quadratic, so a 22k-rune input took seconds per pass while
	// the per-chunk detection below finishes in milliseconds.
	text := strings.Repeat("人工智能使生活更美好。", 2000)
	op, err := NewSplitOperator(map[string]interface{}{"strategy": "char"})
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := &ChunkContext{TextAfterPreprocess: text}
		if err := op.Execute(ctx); err != nil {
			b.Fatal(err)
		}
	}
}
