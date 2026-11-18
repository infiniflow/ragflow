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

package agentic_rag

import (
	"context"
	"testing"

	"ragflow/internal/agent/runtime"
)

// TestListChunksTwoPhase_IndexLightweightFullFetch pins the two-pass contract:
// Pass 1 (ListDocumentChunkIndex) returns every chunk ordered with empty
// Content; Pass 2 (FetchChunksByID) narrows by ids via a term filter and
// returns full prose, skipping graph payloads.
func TestListChunksTwoPhase_IndexLightweightFullFetch(t *testing.T) {
	fe := &grepFakeEngine{
		regexpChunks: []map[string]interface{}{
			{"id": "c1", "content_with_weight": "prose one", "doc_id": "d1", "docnm_kwd": "doc1", "kb_id": "kb1", "chunk_order_int": float64(1)},
			{"id": "c0", "content_with_weight": `{"head":"A","tail":"B","type":"relation"}`, "doc_id": "d1", "docnm_kwd": "doc1", "kb_id": "kb1", "chunk_order_int": float64(0)},
			{"id": "c2", "content_with_weight": "prose two", "doc_id": "d1", "docnm_kwd": "doc1", "kb_id": "kb1", "chunk_order_int": float64(2)},
		},
	}
	ad := NewGrepAdapter(fe)
	ctx := context.Background()
	scope := runtime.GrepRequest{
		TenantID:   "t",
		DatasetIDs: []string{"kb1"},
		DocScope:   []string{"d1"},
	}

	// Pass 1: full ordered index, content stays empty, graph chunk INCLUDED.
	idx, err := ad.ListDocumentChunkIndex(ctx, scope)
	if err != nil {
		t.Fatalf("ListDocumentChunkIndex: %v", err)
	}
	if len(idx) != 3 {
		t.Fatalf("index len = %d, want 3 (graph chunk occupies an index position)", len(idx))
	}
	for i, wantID := range []string{"c0", "c1", "c2"} {
		if idx[i].ID != wantID {
			t.Errorf("index[%d].ID = %q, want %q", i, idx[i].ID, wantID)
		}
		if idx[i].Content != "" {
			t.Errorf("index[%d] must not carry content (lightweight pass), got %q", i, idx[i].Content)
		}
	}

	// Pass 2: fetch two selected ids; the graph chunk's payload is skipped.
	fetched, err := ad.FetchChunksByID(ctx, runtime.GrepRequest{
		TenantID:   scope.TenantID,
		DatasetIDs: scope.DatasetIDs,
		DocScope:   scope.DocScope,
		ChunkScope: []string{"c0", "c2"},
	})
	if err != nil {
		t.Fatalf("FetchChunksByID: %v", err)
	}
	if len(fetched) != 1 || fetched[0].ID != "c2" {
		t.Fatalf("fetched = %+v, want only c2 (graph payload of c0 excluded)", fetched)
	}
	if fetched[0].Content != "prose two" {
		t.Errorf("content = %q, want prose two", fetched[0].Content)
	}
}
