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
	"reflect"
	"testing"
)

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

// TestRun_FilterDroppingEveryChunkYieldsNoChunks pins that a postprocess
// stage's empty output is the answer. The split chunks stayed reachable
// through the "no result yet" fallback, so a min_length above every chunk's
// length re-emitted all of them, whitespace chunks included.
func TestRun_FilterDroppingEveryChunkYieldsNoChunks(t *testing.T) {
	ctx, err := Run("a b c", ChunkOptions{
		SplitStrategy:   "char",
		FilterMinLength: 5,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(ctx.SplitChunks) != 5 {
		t.Fatalf("split chunks = %d, want 5 as the fixture premise", len(ctx.SplitChunks))
	}
	if len(ctx.ResultChunks) != 0 {
		t.Fatalf("got %d chunks, want none: every chunk is 1 rune and below min_length 5 (%q)",
			len(ctx.ResultChunks), chunkContents(ctx.ResultChunks))
	}
}

// TestRun_FilterKeepsOnlyQualifyingChunks pins the surviving half of the same
// contract: the filter selects, it does not pass everything through.
func TestRun_FilterKeepsOnlyQualifyingChunks(t *testing.T) {
	ctx, err := Run("hi\nthis one is long enough to survive", ChunkOptions{
		SplitStrategy:   "paragraph",
		FilterMinLength: 20,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := []string{"this one is long enough to survive"}
	if got := chunkContents(ctx.ResultChunks); !reflect.DeepEqual(got, want) {
		t.Fatalf("chunks = %q, want %q", got, want)
	}
}

// TestRun_SplitOnlyPipelineStillYieldsChunks keeps the fallback that serves a
// pipeline with no postprocess stage.
func TestRun_SplitOnlyPipelineStillYieldsChunks(t *testing.T) {
	ctx, err := Run("a b c", ChunkOptions{SplitStrategy: "char"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(ctx.ResultChunks) != len(ctx.SplitChunks) || len(ctx.ResultChunks) == 0 {
		t.Fatalf("result chunks = %q, want the split output (%d) passed through",
			chunkContents(ctx.ResultChunks), len(ctx.SplitChunks))
	}
}

func chunkContents(chunks []ChunkData) []string {
	out := make([]string, len(chunks))
	for i, c := range chunks {
		out[i] = c.Content
	}
	return out
}
