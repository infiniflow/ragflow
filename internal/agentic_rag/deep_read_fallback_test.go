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
	"errors"
	"testing"

	"ragflow/internal/agent/runtime"
)

// deepReadFixture is one document, out of reading order on purpose so the
// ordering assertion means something.
func deepReadFixture() []map[string]interface{} {
	return []map[string]interface{}{
		{"id": "c2", "content_with_weight": "second", "doc_id": "d1", "docnm_kwd": "doc", "kb_id": "kb1", "chunk_order_int": 2},
		{"id": "c1", "content_with_weight": "first", "doc_id": "d1", "docnm_kwd": "doc", "kb_id": "kb1", "chunk_order_int": 1},
	}
}

// TestFetchScopedPrefersRegexpPushdown pins the healthy path: when the engine
// implements regexp pushdown and accepts the query, the deep read must not issue
// a second, slower filtered search.
func TestFetchScopedPrefersRegexpPushdown(t *testing.T) {
	fe := &grepFakeEngine{regexpChunks: deepReadFixture()}
	adapter := NewGrepAdapter(fe)

	got, err := adapter.fetchScoped(context.Background(), runtime.GrepRequest{
		TenantID:   "t",
		DatasetIDs: []string{"kb1"},
		DocScope:   []string{"d1"},
	}, []string{"id", "content_with_weight", "doc_id", "docnm_kwd", "kb_id", "chunk_order_int"}, true)
	if err != nil {
		t.Fatalf("fetchScoped: %v", err)
	}
	if fe.searchCalls != 0 {
		t.Errorf("Search called %d times; the pushdown succeeded so the filter path must not run", fe.searchCalls)
	}
	if len(got) != 2 || got[0].ID != "c1" || got[1].ID != "c2" {
		t.Fatalf("chunks = %v, want [c1 c2] in reading order", chunkIDs(got))
	}
}

// TestFetchScopedFallsBackToMetadataFilter is the reason the fallback exists: an
// index whose content_with_weight is not a searchable keyword rejects the
// match-all regexp, and the deep read has to still return the document's chunks
// rather than failing the tool call.
func TestFetchScopedFallsBackToMetadataFilter(t *testing.T) {
	fe := &grepFakeEngine{
		searchByRegexpErr: errors.New(`Cannot search on field [content_with_weight] since it is not indexed`),
		searchChunks:      deepReadFixture(),
	}
	adapter := NewGrepAdapter(fe)

	got, err := adapter.fetchScoped(context.Background(), runtime.GrepRequest{
		TenantID:   "t",
		DatasetIDs: []string{"kb1"},
		DocScope:   []string{"d1"},
	}, []string{"id", "content_with_weight", "doc_id", "docnm_kwd", "kb_id", "chunk_order_int"}, true)
	if err != nil {
		t.Fatalf("fetchScoped must degrade, not fail: %v", err)
	}
	if fe.searchCalls != 1 {
		t.Errorf("Search called %d times, want exactly 1 after the pushdown was rejected", fe.searchCalls)
	}
	if len(got) != 2 || got[0].ID != "c1" || got[1].ID != "c2" {
		t.Fatalf("chunks = %v, want [c1 c2] in reading order", chunkIDs(got))
	}
	if fe.lastReq == nil || fe.lastReq.Filter["doc_id"] == nil {
		t.Errorf("fallback request lost the doc scope: %+v", fe.lastReq)
	}
}

func chunkIDs(chunks []runtime.RetrievalChunk) []string {
	out := make([]string, 0, len(chunks))
	for _, c := range chunks {
		out = append(out, c.ID)
	}
	return out
}
