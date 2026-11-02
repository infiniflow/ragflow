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

import "testing"

// runParagraph builds a SplitOperator for the "paragraph" strategy and runs it.
func runParagraph(t *testing.T, text string) []ChunkData {
	t.Helper()
	op, err := NewSplitOperator(map[string]interface{}{"strategy": "paragraph"})
	if err != nil {
		t.Fatalf("NewSplitOperator returned error: %v", err)
	}
	ctx := &ChunkContext{TextAfterPreprocess: text}
	if err := op.Execute(ctx); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	return ctx.SplitChunks
}

func TestSplitByParagraphSkipsBlankLines(t *testing.T) {
	chunks := runParagraph(t, "第一段\n\n\n第二段\n \n第三段")

	want := []string{"第一段", "第二段", "第三段"}
	if len(chunks) != len(want) {
		t.Fatalf("got %d chunks, want %d: %#v", len(chunks), len(want), chunks)
	}
	for i, w := range want {
		if chunks[i].Content != w {
			t.Errorf("chunk %d = %q, want %q", i, chunks[i].Content, w)
		}
	}
}

func TestSplitByParagraphIndexMatchesPosition(t *testing.T) {
	// A skipped blank line used to leave a hole in the Index sequence: the
	// second chunk of this document came back as 3 instead of 1.
	chunks := runParagraph(t, "第一段\n\n\n第二段\n \n第三段")

	if len(chunks) != 3 {
		t.Fatalf("got %d chunks, want 3", len(chunks))
	}
	for i, c := range chunks {
		if c.Index != i {
			t.Errorf("chunk %d (%q) Index = %d, want %d", i, c.Content, c.Index, i)
		}
	}
}

func TestSplitByParagraphBlankInput(t *testing.T) {
	for _, text := range []string{"", "\n\n\n", "  \n\t\n  "} {
		if chunks := runParagraph(t, text); len(chunks) != 0 {
			t.Errorf("%q produced %d chunks, want 0", text, len(chunks))
		}
	}
}
