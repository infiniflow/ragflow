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
