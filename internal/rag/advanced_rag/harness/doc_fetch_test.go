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

package harness

import (
	"context"
	"strings"
	"testing"
)

// docChunkStub serves one document's chunks in reading order, paged.
type docChunkStub struct {
	chunks []map[string]any
}

func (d *docChunkStub) DocChunks(_ context.Context, req DocChunksRequest) ([]map[string]any, error) {
	if req.Offset >= len(d.chunks) {
		return nil, nil
	}
	end := req.Offset + req.Limit
	if end > len(d.chunks) {
		end = len(d.chunks)
	}
	return d.chunks[req.Offset:end], nil
}

func docFetchDeps(pool, docChunks []map[string]any) SearchDeps {
	return SearchDeps{
		KbIDs:     []string{"kb-1"},
		TenantID:  "tenant-1",
		KB:        &Kbinfos{Chunks: pool},
		DocChunks: &docChunkStub{chunks: docChunks},
	}
}

// TestSummarizeDocumentReturnsBlocksForAlreadyPooledChunks covers the case the
// pre-merge chunk count gets wrong on its own: Merge deduplicates a chunk that is
// already pooled, so the pool does not grow and a slice taken at the pre-merge
// count starts past the end of the rendered blocks — the tool then reported that
// a document it had just read had no readable chunks.
func TestSummarizeDocumentReturnsBlocksForAlreadyPooledChunks(t *testing.T) {
	pool := []map[string]any{{"chunk_id": "c1", "content": "Pooled body."}}
	docChunks := []map[string]any{{"chunk_id": "c1", "content": "Pooled body."}}

	got := summarizeDocument(context.Background(), docFetchDeps(pool, docChunks), "doc-1", 100000)

	if len(got) != 1 {
		t.Fatalf("blocks = %d (%v), want the document's block even though it was already pooled", len(got), got)
	}
	if !strings.Contains(got[0], "Pooled body.") {
		t.Errorf("block = %q, want the document's content", got[0])
	}
}

// TestSummarizeDocumentSelectsBlocksBySourceChunk covers a skipped chunk: KBPrompt
// renders no block for a chunk without content, so every later block is one
// position earlier than its chunk. Slicing by the chunk count dropped the first
// document block.
func TestSummarizeDocumentSelectsBlocksBySourceChunk(t *testing.T) {
	pool := []map[string]any{
		{"chunk_id": "empty", "content": ""}, // renders no block
	}
	docChunks := []map[string]any{
		{"chunk_id": "c1", "content": "First document body."},
		{"chunk_id": "c2", "content": "Second document body."},
	}

	got := summarizeDocument(context.Background(), docFetchDeps(pool, docChunks), "doc-1", 100000)

	if len(got) != 2 {
		t.Fatalf("blocks = %d (%v), want both document chunks", len(got), got)
	}
	if !strings.Contains(got[0], "First document body.") || !strings.Contains(got[1], "Second document body.") {
		t.Errorf("blocks = %v, want the document's chunks in reading order", got)
	}
}

// TestSummarizeDocumentReturnsNothingWhenNoChunkRenders keeps the guard: when the
// fetched chunks contribute no renderable block there is nothing to hand back,
// and in particular no unrelated pooled block.
func TestSummarizeDocumentReturnsNothingWhenNoChunkRenders(t *testing.T) {
	pool := []map[string]any{{"chunk_id": "p0", "content": "Pooled evidence."}}
	docChunks := []map[string]any{{"chunk_id": "c1", "content": "   "}}

	got := summarizeDocument(context.Background(), docFetchDeps(pool, docChunks), "doc-1", 100000)

	if len(got) != 0 {
		t.Fatalf("blocks = %v, want nothing for a document with no renderable chunk", got)
	}
}

// TestSummarizeDocumentDeduplicatesRepeatedSourcePositions covers the paging
// hazard: offset paging over an unstable order can serve the same chunk on two
// pages, and Merge reports a position per OCCURRENCE, not per distinct chunk. The
// block of that one pooled chunk was therefore appended twice — paying its tokens
// into the prompt twice, on top of a budget that was already spent.
func TestSummarizeDocumentDeduplicatesRepeatedSourcePositions(t *testing.T) {
	pool := []map[string]any{{"chunk_id": "p0", "content": "Pooled evidence."}}
	docChunks := []map[string]any{
		{"chunk_id": "c1", "content": "Document body."},
		{"chunk_id": "c1", "content": "Document body."}, // same chunk served twice
		{"chunk_id": "c2", "content": "Second body."},
	}

	got := summarizeDocument(context.Background(), docFetchDeps(pool, docChunks), "doc-1", 100000)

	if len(got) != 2 {
		t.Fatalf("blocks = %d (%v), want one block per distinct fetched chunk", len(got), got)
	}
	if !strings.Contains(got[0], "Document body.") || !strings.Contains(got[1], "Second body.") {
		t.Errorf("blocks = %v, want the document's chunks in reading order, each once", got)
	}
}
