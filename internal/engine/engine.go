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

package engine

import (
	"context"

	"ragflow/internal/engine/types"
)

// EngineType document engine type
type EngineType string

const (
	EngineElasticsearch EngineType = "elasticsearch"
	EngineInfinity      EngineType = "infinity"
)

// DocEngine document storage engine interface
type DocEngine interface {
	// Chunk operations
	CreateChunkStore(ctx context.Context, baseName, datasetID string, vectorSize int, parserID string) error
	InsertChunks(ctx context.Context, chunks []map[string]interface{}, baseName string, datasetID string) ([]string, error)
	UpdateChunks(ctx context.Context, condition map[string]interface{}, newValue map[string]interface{}, baseName string, datasetID string) error
	DeleteChunks(ctx context.Context, condition map[string]interface{}, baseName string, datasetID string) (int64, error)
	Search(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error)
	GetChunk(ctx context.Context, baseName, chunkID string, datasetIDs []string) (interface{}, error)
	DropChunkStore(ctx context.Context, baseName, datasetID string) error
	ChunkStoreExists(ctx context.Context, baseName, datasetID string) (bool, error)

	// Document metadata operations
	CreateMetadataStore(ctx context.Context, tenantID string) error
	InsertMetadata(ctx context.Context, metadata []map[string]interface{}, tenantID string) ([]string, error)
	UpdateMetadata(ctx context.Context, docID string, datasetID string, metaFields map[string]interface{}, tenantID string) error
	DeleteMetadata(ctx context.Context, condition map[string]interface{}, tenantID string) (int64, error)
	DeleteMetadataKeys(ctx context.Context, docID string, datasetID string, keys []string, tenantID string) error
	DropMetadataStore(ctx context.Context, tenantID string) error
	MetadataStoreExists(ctx context.Context, tenantID string) (bool, error)

	// Document operations (used by skill indexing)
	IndexDocument(ctx context.Context, indexName, docID string, doc interface{}) error
	DeleteDocument(ctx context.Context, indexName, docID string) error
	BulkIndex(ctx context.Context, indexName string, docs []interface{}) (interface{}, error)

	// Utility functions for search result processing
	GetFields(chunks []map[string]interface{}, fields []string) map[string]map[string]interface{}
	GetAggregation(chunks []map[string]interface{}, fieldName string) []map[string]interface{}
	GetHighlight(chunks []map[string]interface{}, keywords []string, fieldName string) map[string]string
	GetDocIDs(chunks []map[string]interface{}) []string

	// Health check
	Ping(ctx context.Context) error
	Close() error

	// GetType returns the engine type
	GetType() string
}

// Type returns the engine type (helper method for runtime type checking)
// This is a workaround since we can't import elasticsearch or infinity packages directly
func Type(docEngine DocEngine) EngineType {
	// Type checking through interface methods is not straightforward
	// This is a placeholder that should be implemented differently
	// or rely on configuration to know the type
	return EngineType("unknown")
}
