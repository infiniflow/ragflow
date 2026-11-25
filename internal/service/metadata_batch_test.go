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

package service

import (
	"context"
	"fmt"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/engine/types"
)

// countingMetadataEngine records each SearchMetadata call and returns metadata
// for every document named in the request's id filter. failAt (1-based) makes
// one request fail, modelling a transient index error for that batch only.
type countingMetadataEngine struct {
	fakeChatDocEngine
	requests []*types.SearchMetadataRequest
	failAt   int
}

func (e *countingMetadataEngine) SearchMetadata(_ context.Context, req *types.SearchMetadataRequest) (*types.SearchMetadataResult, error) {
	e.requests = append(e.requests, req)
	if e.failAt > 0 && len(e.requests) == e.failAt {
		return nil, fmt.Errorf("metadata batch %d unavailable", e.failAt)
	}
	ids, _ := req.Filter["id"].([]string)
	records := make([]map[string]interface{}, 0, len(ids))
	for _, id := range ids {
		records = append(records, map[string]interface{}{
			"id":          id,
			"meta_fields": map[string]interface{}{"author": "Alice"},
		})
	}
	return &types.SearchMetadataResult{MetadataRecords: records}, nil
}

func TestFetchDocMetaByKBBatchesEngineCalls(t *testing.T) {
	eng := &countingMetadataEngine{}
	svc := NewMetadataServiceForTest(nil, eng)
	input := KBDocIDsMap{
		"kb-1": {"doc-1", "doc-2"},
		"kb-2": {"doc-3"},
		"kb-3": {"doc-4", "doc-5"},
	}

	got := svc.FetchDocMetaByKB(t.Context(), input, "tenant-1")

	// Every requested document must keep its metadata regardless of grouping.
	for _, docID := range []string{"doc-1", "doc-2", "doc-3", "doc-4", "doc-5"} {
		if _, ok := got[docID]; !ok {
			t.Fatalf("metadata for %s missing; got %v", docID, got)
		}
	}

	// One engine round-trip for all KBs instead of one per KB.
	if len(eng.requests) != 1 {
		t.Fatalf("SearchMetadata calls = %d for %d KBs, want 1 (batch); requests: %#v", len(eng.requests), len(input), eng.requests)
	}
	if ids, ok := eng.requests[0].Filter["id"].([]string); !ok || len(ids) != 5 {
		t.Fatalf("batch id filter = %v, want all 5 documents", eng.requests[0].Filter["id"])
	}
	if kbIDs, ok := eng.requests[0].Filter["kb_id"].([]string); !ok || len(kbIDs) != 3 {
		t.Fatalf("batch kb_id filter = %v, want all 3 KBs", eng.requests[0].Filter["kb_id"])
	}
	if eng.requests[0].Limit < 5 {
		t.Fatalf("batch limit = %d, want at least 5", eng.requests[0].Limit)
	}
}

// largeMetadataDocIDs builds more distinct documents than an engine result
// window can return in one request, so FetchDocMetaByKB must split the work.
func largeMetadataDocIDs(t *testing.T, n int) KBDocIDsMap {
	t.Helper()
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		ids = append(ids, fmt.Sprintf("doc-%05d", i))
	}
	return KBDocIDsMap{"kb-large": ids}
}

// A combined request larger than the engine's result window must be split into
// bounded requests; every document must still get its metadata.
func TestFetchDocMetaByKBSplitsLargeBatches(t *testing.T) {
	total := common.MAX_RESULT_WINDOW + 1
	eng := &countingMetadataEngine{}
	svc := NewMetadataServiceForTest(nil, eng)

	got := svc.FetchDocMetaByKB(t.Context(), largeMetadataDocIDs(t, total), "tenant-1")

	if len(got) != total {
		t.Fatalf("metadata documents = %d, want %d", len(got), total)
	}
	if len(eng.requests) != 2 {
		t.Fatalf("SearchMetadata calls = %d, want 2 for %d documents", len(eng.requests), total)
	}
	if eng.requests[0].Limit > common.MAX_RESULT_WINDOW || eng.requests[1].Limit > common.MAX_RESULT_WINDOW {
		t.Fatalf("batch limits = %d/%d, want each <= %d", eng.requests[0].Limit, eng.requests[1].Limit, common.MAX_RESULT_WINDOW)
	}
}

// Exactly at the window boundary a single request is sufficient.
func TestFetchDocMetaByKBAtWindowBoundaryStaysSingleBatch(t *testing.T) {
	eng := &countingMetadataEngine{}
	svc := NewMetadataServiceForTest(nil, eng)

	got := svc.FetchDocMetaByKB(t.Context(), largeMetadataDocIDs(t, common.MAX_RESULT_WINDOW), "tenant-1")

	if len(got) != common.MAX_RESULT_WINDOW {
		t.Fatalf("metadata documents = %d, want %d", len(got), common.MAX_RESULT_WINDOW)
	}
	if len(eng.requests) != 1 {
		t.Fatalf("SearchMetadata calls = %d, want 1 at the window boundary", len(eng.requests))
	}
}

// A failed batch must not discard metadata already gathered from other batches:
// the old per-KB loop skipped only the failing KB, not the whole request.
func TestFetchDocMetaByKBKeepsSuccessfulBatchesOnFailure(t *testing.T) {
	total := common.MAX_RESULT_WINDOW + 1
	eng := &countingMetadataEngine{failAt: 1}
	svc := NewMetadataServiceForTest(nil, eng)

	got := svc.FetchDocMetaByKB(t.Context(), largeMetadataDocIDs(t, total), "tenant-1")

	if len(eng.requests) != 2 {
		t.Fatalf("SearchMetadata calls = %d, want 2 even when the first batch fails", len(eng.requests))
	}
	// The first window's documents are lost, but the second window's document
	// must survive — the failure of one request cannot blank the whole map.
	if _, ok := got[fmt.Sprintf("doc-%05d", total-1)]; !ok {
		t.Fatalf("metadata from the successful batch was discarded: %v", got)
	}
}
