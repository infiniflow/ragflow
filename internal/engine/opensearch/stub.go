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

package opensearch

import (
	"context"
	"fmt"
	"ragflow/internal/engine/types"

	"gorm.io/gorm"
)

// Placeholder stubs satisfying engine.DocEngine during incremental milestones.
// Each method will be replaced by concrete implementations in Commits 5-9.

func (e *Engine) CreateChunkStore(ctx context.Context, baseName, datasetID string, vectorSize int, parserID, language string) error {
	return fmt.Errorf("CreateChunkStore pending Commit 5")
}

func (e *Engine) InsertChunks(ctx context.Context, chunks []map[string]interface{}, baseName string, datasetID string, language string) ([]string, error) {
	return nil, fmt.Errorf("InsertChunks pending Commit 5")
}

func (e *Engine) UpdateChunks(ctx context.Context, condition map[string]interface{}, newValue map[string]interface{}, baseName string, datasetID string) error {
	return fmt.Errorf("UpdateChunks pending Commit 5")
}

func (e *Engine) DeleteChunks(ctx context.Context, condition map[string]interface{}, baseName string, datasetID string) (int64, error) {
	return 0, fmt.Errorf("DeleteChunks pending Commit 5")
}

func (e *Engine) Search(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	return nil, fmt.Errorf("Search pending Commits 7-8")
}

func (e *Engine) GetChunk(ctx context.Context, baseName, chunkID string, datasetIDs []string) (interface{}, error) {
	return nil, fmt.Errorf("GetChunk pending Commit 5")
}

func (e *Engine) DropChunkStore(ctx context.Context, baseName, datasetID string) error {
	return fmt.Errorf("DropChunkStore pending Commit 5")
}

func (e *Engine) ChunkStoreExists(ctx context.Context, baseName, datasetID string) (bool, error) {
	return false, fmt.Errorf("ChunkStoreExists pending Commit 5")
}

func (e *Engine) CreateMetadataStore(ctx context.Context, tenantID string) error {
	return fmt.Errorf("CreateMetadataStore pending Commit 6")
}

func (e *Engine) InsertMetadata(ctx context.Context, metadata []map[string]interface{}, tenantID string) ([]string, error) {
	return nil, fmt.Errorf("InsertMetadata pending Commit 6")
}

func (e *Engine) UpdateMetadata(ctx context.Context, docID string, datasetID string, metaFields map[string]interface{}, tenantID string) error {
	return fmt.Errorf("UpdateMetadata pending Commit 6")
}

func (e *Engine) DeleteMetadata(ctx context.Context, condition map[string]interface{}, tenantID string) (int64, error) {
	return 0, fmt.Errorf("DeleteMetadata pending Commit 6")
}

func (e *Engine) DeleteMetadataKeys(ctx context.Context, docID string, datasetID string, keys []string, tenantID string) error {
	return fmt.Errorf("DeleteMetadataKeys pending Commit 6")
}

func (e *Engine) DropMetadataStore(ctx context.Context, tenantID string) error {
	return fmt.Errorf("DropMetadataStore pending Commit 6")
}

func (e *Engine) MetadataStoreExists(ctx context.Context, tenantID string) (bool, error) {
	return false, fmt.Errorf("MetadataStoreExists pending Commit 6")
}

func (e *Engine) SearchMetadata(ctx context.Context, req *types.SearchMetadataRequest) (*types.SearchMetadataResult, error) {
	return nil, fmt.Errorf("SearchMetadata pending Commit 6")
}

func (e *Engine) IndexDocument(ctx context.Context, indexName, docID string, doc interface{}) error {
	return fmt.Errorf("IndexDocument pending Commit 6")
}

func (e *Engine) DeleteDocument(ctx context.Context, indexName, docID string) error {
	return fmt.Errorf("DeleteDocument pending Commit 6")
}

func (e *Engine) BulkIndex(ctx context.Context, indexName string, docs []interface{}) (interface{}, error) {
	return nil, fmt.Errorf("BulkIndex pending Commit 6")
}

func (e *Engine) GetFields(chunks []map[string]interface{}, fields []string) map[string]map[string]interface{} {
	return nil
}

func (e *Engine) GetAggregation(chunks []map[string]interface{}, fieldName string) []map[string]interface{} {
	return nil
}

func (e *Engine) GetHighlight(chunks []map[string]interface{}, keywords []string, fieldName string) map[string]string {
	return nil
}

func (e *Engine) RunSQL(ctx context.Context, tableName string, sqlText string, kbIDs []string, format string) ([]map[string]interface{}, error) {
	return nil, fmt.Errorf("RunSQL pending Commit 9")
}

func (e *Engine) GetChunkIDs(chunks []map[string]interface{}) []string {
	return nil
}

func (e *Engine) KNNScores(ctx context.Context, chunks []map[string]interface{}, queryVector []float64, topK int) (map[string]interface{}, error) {
	return nil, fmt.Errorf("KNNScores pending Commit 8")
}

func (e *Engine) GetScores(searchResult map[string]interface{}) map[string]float64 {
	return nil
}

func (e *Engine) FilterDocIdsByMetaPushdown(ctx context.Context, db *gorm.DB, docIDs []string, conditions []map[string]interface{}, tenantID string) []string {
	return nil
}
