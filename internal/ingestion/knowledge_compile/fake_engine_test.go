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

package knowledge_compile

import (
	"context"
	"fmt"

	"ragflow/internal/common"
	"ragflow/internal/engine/types"

	"gorm.io/gorm"
)

// fakeEngine is a minimal engine.DocEngine for unit tests. It records the
// Search filter/SelectFields and the DeleteChunks condition so tests can assert
// the structural scope of a reader/writer call, and returns a canned chunk set.
type fakeEngine struct {
	// engineType overrides GetType; defaults to "fake" (unsupported for
	// dataset-structure writes). Tests of structure write/delete paths set
	// this to "infinity" so the guard permits the operation.
	engineType string
	// searchChunks is the result returned by Search.
	searchChunks []map[string]interface{}
	// lastSearchReq captures the most recent SearchRequest for assertions.
	lastSearchReq *types.SearchRequest
	// searchReqs accumulates every SearchRequest in call order, so tests can
	// assert on multi-search paths (e.g. chunked batch reads).
	searchReqs []*types.SearchRequest
	// searchCalls counts Search invocations.
	searchCalls int
	// windowed emulates the backend max_result_window: a request with no
	// OrderBy whose from+size exceeds common.MAX_RESULT_WINDOW is rejected, the
	// way Elasticsearch rejects from+size beyond index.max_result_window. With a
	// stable OrderBy set, deep pagination routes to search_after and is allowed.
	windowed bool
	// lastDeleteCond captures the most recent DeleteChunks condition.
	lastDeleteCond map[string]interface{}
	// deleteConds accumulates every DeleteChunks condition in call order, so a
	// test can assert on a multi-delete clean (e.g. the full-set clean that
	// sweeps compile_kwd buckets and the scope_kwd="dataset" bucket separately).
	deleteConds []map[string]interface{}
	// deleteCount is the number returned by DeleteChunks.
	deleteCount    int64
	insertedChunks []map[string]interface{}
}

func (f *fakeEngine) Search(_ context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	f.lastSearchReq = req
	f.searchReqs = append(f.searchReqs, req)
	f.searchCalls++
	// Honour the subset of filters the production engine applies, so tests can
	// assert real filtering behaviour (doc_id IN + exists) instead of a raw
	// pass-through of every canned chunk.
	chunks := applyEngineFilters(f.searchChunks, req.Filter)
	if f.windowed {
		// Faithfully emulate the Elasticsearch/OpenSearch index.max_result_window:
		// a request whose from+size exceeds the window is rejected. The real
		// engines only relax this via a stable Sort + search_after; our batch
		// reader deliberately avoids that path, so a pathological batch simply
		// fails loudly instead of silently truncating or looping. The rejection
		// does NOT depend on OrderBy being present, because the engine would
		// reject the deep from+size regardless once search_after is unavailable.
		if req.Offset+req.Limit > common.MAX_RESULT_WINDOW {
			return nil, fmt.Errorf("Result window is too large, from + size must be <= %d", common.MAX_RESULT_WINDOW)
		}
		start := req.Offset
		if start > len(chunks) {
			start = len(chunks)
		}
		end := start + req.Limit
		if end > len(chunks) {
			end = len(chunks)
		}
		return &types.SearchResult{Chunks: chunks[start:end]}, nil
	}
	return &types.SearchResult{Chunks: chunks}, nil
}

// applyEngineFilters mirrors the production engine's bool.filter handling for the
// clauses the knowledge_compile reader relies on: doc_id IN (terms) and exists.
// Other clauses (scope_kwd, type_kwd, slug_kwd, available_int, …) are
// pass-through. It never mutates the input slice.
func applyEngineFilters(chunks []map[string]interface{}, filter map[string]interface{}) []map[string]interface{} {
	if len(filter) == 0 {
		return chunks
	}
	out := chunks
	if raw, ok := filter["doc_id"]; ok {
		ids := docIDSet(raw)
		if len(ids) > 0 {
			kept := make([]map[string]interface{}, 0, len(out))
			for _, c := range out {
				if id, _ := c["doc_id"].(string); ids[id] {
					kept = append(kept, c)
				}
			}
			out = kept
		}
	}
	if field, ok := filter["exists"].(string); ok && field != "" {
		kept := make([]map[string]interface{}, 0, len(out))
		for _, c := range out {
			if _, present := c[field]; present {
				kept = append(kept, c)
			}
		}
		out = kept
	}
	return out
}

// docIDSet normalises a doc_id filter value (string, []string, or []interface{} of
// strings) into a lookup set.
func docIDSet(raw interface{}) map[string]bool {
	out := map[string]bool{}
	switch v := raw.(type) {
	case string:
		out[v] = true
	case []string:
		for _, s := range v {
			out[s] = true
		}
	case []interface{}:
		for _, s := range v {
			if str, ok := s.(string); ok {
				out[str] = true
			}
		}
	}
	return out
}

func (f *fakeEngine) DeleteChunks(_ context.Context, condition map[string]interface{}, _, _ string) (int64, error) {
	f.lastDeleteCond = condition
	f.deleteConds = append(f.deleteConds, condition)
	return f.deleteCount, nil
}

// --- stubs: the reader/writer path under test never exercises these. ---

func (f *fakeEngine) CreateChunkStore(context.Context, string, string, int, string, string) error {
	return nil
}
func (f *fakeEngine) InsertChunks(_ context.Context, chunks []map[string]interface{}, _, _, _ string) ([]string, error) {
	f.insertedChunks = append(f.insertedChunks, chunks...)
	return nil, nil
}
func (f *fakeEngine) UpdateChunks(context.Context, map[string]interface{}, map[string]interface{}, string, string) error {
	return nil
}
func (f *fakeEngine) GetChunk(context.Context, string, string, []string) (interface{}, error) {
	return nil, nil
}
func (f *fakeEngine) DropChunkStore(context.Context, string, string) error { return nil }
func (f *fakeEngine) ChunkStoreExists(context.Context, string, string) (bool, error) {
	return true, nil
}
func (f *fakeEngine) CreateMetadataStore(context.Context, string) error { return nil }
func (f *fakeEngine) InsertMetadata(context.Context, []map[string]interface{}, string) ([]string, error) {
	return nil, nil
}
func (f *fakeEngine) UpdateMetadata(context.Context, string, string, map[string]interface{}, string) error {
	return nil
}
func (f *fakeEngine) DeleteMetadata(context.Context, map[string]interface{}, string) (int64, error) {
	return 0, nil
}
func (f *fakeEngine) DeleteMetadataKeys(context.Context, string, string, []string, string) error {
	return nil
}
func (f *fakeEngine) DropMetadataStore(context.Context, string) error           { return nil }
func (f *fakeEngine) MetadataStoreExists(context.Context, string) (bool, error) { return true, nil }
func (f *fakeEngine) SearchMetadata(context.Context, *types.SearchMetadataRequest) (*types.SearchMetadataResult, error) {
	return nil, nil
}
func (f *fakeEngine) IndexDocument(context.Context, string, string, interface{}) error { return nil }
func (f *fakeEngine) DeleteDocument(context.Context, string, string) error             { return nil }
func (f *fakeEngine) BulkIndex(context.Context, string, []interface{}) (interface{}, error) {
	return nil, nil
}
func (f *fakeEngine) GetFields([]map[string]interface{}, []string) map[string]map[string]interface{} {
	return nil
}
func (f *fakeEngine) GetAggregation([]map[string]interface{}, string) []map[string]interface{} {
	return nil
}
func (f *fakeEngine) GetHighlight([]map[string]interface{}, []string, string) map[string]string {
	return nil
}
func (f *fakeEngine) RunSQL(context.Context, string, string) ([]map[string]interface{}, error) {
	return nil, nil
}
func (f *fakeEngine) GetChunkIDs([]map[string]interface{}) []string { return nil }
func (f *fakeEngine) KNNScores(context.Context, []map[string]interface{}, []float64, int) (map[string]interface{}, error) {
	return nil, nil
}
func (f *fakeEngine) GetScores(map[string]interface{}) map[string]float64 { return nil }
func (f *fakeEngine) Ping(context.Context) error                          { return nil }
func (f *fakeEngine) Close() error                                        { return nil }

// GetType reports the engine type. It defaults to "fake"; tests that exercise
// dataset-structure write/delete paths override engineType to "infinity" so
// the datasetStructureSupported guard (which mirrors the production engine
// type) lets the operation proceed.
func (f *fakeEngine) GetType() string        { return f.engineType }
func (f *fakeEngine) SupportsPageRank() bool { return false }
func (f *fakeEngine) FilterDocIdsByMetaPushdown(context.Context, *gorm.DB, []string, []map[string]interface{}, string) []string {
	return nil
}
