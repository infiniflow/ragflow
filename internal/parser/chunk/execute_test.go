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

func TestRun_SentenceSplitHandlesASCIIBoundaries(t *testing.T) {
	ctx, err := Run("One. Two! Three? Four;", ChunkOptions{SplitStrategy: "sentence"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(ctx.ResultChunks) != 4 {
		t.Fatalf("got %d chunks, want 4", len(ctx.ResultChunks))
	}
	want := []string{"One", "Two", "Three", "Four"}
	for i, chunk := range ctx.ResultChunks {
		if chunk.Content != want[i] {
			t.Errorf("chunk[%d] = %q, want %q", i, chunk.Content, want[i])
		}
	}
}

func TestRun_PostprocessHonorsTypedNumericOptions(t *testing.T) {
	ctx, err := Run("a\nbb\nccc", ChunkOptions{
		SplitStrategy:    "paragraph",
		MergeTargetSize:  100,
		FilterMinLength:  2,
		RemoveEmptyLines: true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(ctx.ResultChunks) != 1 {
		t.Fatalf("got %d chunks, want 1", len(ctx.ResultChunks))
	}
	if ctx.ResultChunks[0].Content != "a bb ccc" {
		t.Errorf("merged content = %q, want %q", ctx.ResultChunks[0].Content, "a bb ccc")
	}
}
