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
	"testing"

	"ragflow/internal/engine/types"
)

// countingMetadataEngine records each SearchMetadata call and returns metadata
// for every document named in the request's id filter.
type countingMetadataEngine struct {
	fakeChatDocEngine
	requests []*types.SearchMetadataRequest
}

func (e *countingMetadataEngine) SearchMetadata(_ context.Context, req *types.SearchMetadataRequest) (*types.SearchMetadataResult, error) {
	e.requests = append(e.requests, req)
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
