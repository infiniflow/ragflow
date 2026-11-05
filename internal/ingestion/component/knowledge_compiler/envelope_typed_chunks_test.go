// Copyright 2025 Infiniflow, Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package knowledge_compiler

import (
	"testing"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
	"ragflow/internal/ingestion/component/schema"
)

// TestKnowledgeCompiler_BuildInputsAcceptsTypedChunkDocs: once chunkers emit
// typed []ChunkDoc through the envelope, buildInputs must accept the typed
// carrier in addition to the legacy []map[string]any / []any forms, without
// dropping every chunk on a type mismatch.
func TestKnowledgeCompiler_BuildInputsAcceptsTypedChunkDocs(t *testing.T) {
	in, err := buildInputs(map[string]any{
		"chunks": []schema.ChunkDoc{
			{ID: "c1", Text: "甲"},
			{ID: "c2", Text: "乙"},
		},
	}, common.Param{})
	if err != nil {
		t.Fatalf("buildInputs (typed []ChunkDoc): %v", err)
	}
	if len(in.Chunks) != 2 {
		t.Fatalf("buildInputs produced %d chunks, want 2 (typed []ChunkDoc carrier)", len(in.Chunks))
	}
	if in.Chunks[0].ID != "c1" || in.Chunks[0].Text != "甲" {
		t.Fatalf("chunk[0] = %+v, want id=c1 text=甲", in.Chunks[0])
	}
}

// TestKnowledgeCompiler_MergeChunksAcceptsTypedChunkDocs: mergeChunks must
// forward the upstream typed []ChunkDoc carrier (converted to the wire map
// form) ahead of the freshly compiled chunks, rather than silently dropping
// the upstream chunks on an unexpected type.
func TestKnowledgeCompiler_MergeChunksAcceptsTypedChunkDocs(t *testing.T) {
	out := mergeChunks(map[string]any{
		"chunks": []schema.ChunkDoc{{ID: "c1", Text: "甲"}},
	}, []schema.ChunkDoc{{ID: "d1", Text: "编译"}})

	merged, ok := out["chunks"].([]any)
	if !ok {
		t.Fatalf("mergeChunks output chunks type = %T, want []any", out["chunks"])
	}
	if len(merged) != 2 {
		t.Fatalf("mergeChunks produced %d chunks, want 2 (1 upstream + 1 compiled)", len(merged))
	}
	first, ok := merged[0].(map[string]any)
	if !ok {
		t.Fatalf("merged[0] type = %T, want map[string]any", merged[0])
	}
	if first["id"] != "c1" || first["text"] != "甲" {
		t.Fatalf("merged[0] = %+v, want upstream chunk id=c1 text=甲 (must not be dropped)", first)
	}
	second, ok := merged[1].(map[string]any)
	if !ok {
		t.Fatalf("merged[1] type = %T, want map[string]any", merged[1])
	}
	if second["id"] != "d1" {
		t.Fatalf("merged[1] = %+v, want compiled chunk id=d1", second)
	}
}
