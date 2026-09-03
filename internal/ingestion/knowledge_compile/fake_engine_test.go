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

	"ragflow/internal/engine/types"

	"gorm.io/gorm"
)

// fakeEngine is a minimal engine.DocEngine for unit tests. It records the
// Search filter/SelectFields and the DeleteChunks condition so tests can assert
// the structural scope of a reader/writer call, and returns a canned chunk set.
type fakeEngine struct {
	// searchChunks is the result returned by Search.
	searchChunks []map[string]interface{}
	// lastSearchReq captures the most recent SearchRequest for assertions.
	lastSearchReq *types.SearchRequest
	// lastDeleteCond captures the most recent DeleteChunks condition.
	lastDeleteCond map[string]interface{}
	// deleteConds accumulates every DeleteChunks condition in call order, so a
	// test can assert on a multi-delete clean (e.g. the full-set clean that
	// sweeps compile_kwd buckets and the scope_kwd="dataset" bucket separately).
	deleteConds []map[string]interface{}
	// deleteCount is the number returned by DeleteChunks.
	deleteCount int64
}

func (f *fakeEngine) Search(_ context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	f.lastSearchReq = req
	return &types.SearchResult{Chunks: f.searchChunks}, nil
}

func (f *fakeEngine) DeleteChunks(_ context.Context, condition map[string]interface{}, _, _ string) (int64, error) {
	f.lastDeleteCond = condition
	f.deleteConds = append(f.deleteConds, condition)
	return f.deleteCount, nil
}

// --- stubs: the reader/writer path under test never exercises these. ---

func (f *fakeEngine) CreateChunkStore(context.Context, string, string, int, string) error {
	return nil
}
func (f *fakeEngine) InsertChunks(context.Context, []map[string]interface{}, string, string) ([]string, error) {
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
func (f *fakeEngine) RunSQL(context.Context, string, string, []string, string) ([]map[string]interface{}, error) {
	return nil, nil
}
func (f *fakeEngine) GetChunkIDs([]map[string]interface{}) []string { return nil }
func (f *fakeEngine) KNNScores(context.Context, []map[string]interface{}, []float64, int) (map[string]interface{}, error) {
	return nil, nil
}
func (f *fakeEngine) GetScores(map[string]interface{}) map[string]float64 { return nil }
func (f *fakeEngine) Ping(context.Context) error                          { return nil }
func (f *fakeEngine) Close() error                                        { return nil }
func (f *fakeEngine) GetType() string                                     { return "fake" }
func (f *fakeEngine) SupportsPageRank() bool                              { return false }
func (f *fakeEngine) FilterDocIdsByMetaPushdown(context.Context, *gorm.DB, []string, []map[string]interface{}, string) []string {
	return nil
}
