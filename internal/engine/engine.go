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

// SearchRequest is an alias for types.SearchRequest
type SearchRequest = types.SearchRequest

// SearchResponse is an alias for types.SearchResponse
type SearchResponse = types.SearchResponse

// DocEngine document storage engine interface
type DocEngine interface {
	// Search
	Search(ctx context.Context, req interface{}) (interface{}, error)

	// Index operations
	CreateIndex(ctx context.Context, indexName string, mapping interface{}) error
	DeleteIndex(ctx context.Context, indexName string) error
	IndexExists(ctx context.Context, indexName string) (bool, error)

	// Document operations
	IndexDocument(ctx context.Context, indexName, docID string, doc interface{}) error
	BulkIndex(ctx context.Context, indexName string, docs []interface{}) (interface{}, error)
	GetDocument(ctx context.Context, indexName, docID string) (interface{}, error)
	DeleteDocument(ctx context.Context, indexName, docID string) error

	// Health check
	Ping(ctx context.Context) error
	Close() error
}

// Type returns the engine type (helper method for runtime type checking)
// This is a workaround since we can't import elasticsearch or infinity packages directly
func Type(docEngine DocEngine) EngineType {
	// Type checking through interface methods is not straightforward
	// This is a placeholder that should be implemented differently
	// or rely on configuration to know the type
	return EngineType("unknown")
}
