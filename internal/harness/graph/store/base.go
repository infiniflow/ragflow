package store

import (
	"context"
	"time"
)

// BaseStore is the abstract interface for storing and retrieving data.
// It supports namespaced storage with get/put/search/index operations.
type BaseStore interface {
	// Get retrieves a value from the store by namespace and key.
	// Returns the value if found, nil if not found.
	Get(ctx context.Context, namespace []string, key string) (map[string]interface{}, error)

	// Put stores a value in the store under the given namespace and key.
	Put(ctx context.Context, namespace []string, key string, value map[string]interface{}) error

	// Delete removes a value from the store by namespace and key.
	Delete(ctx context.Context, namespace []string, key string) error

	// Search searches for values in the given namespace that match the query.
	// The query format is implementation-specific.
	Search(ctx context.Context, namespace []string, query string, limit int) ([]map[string]interface{}, error)

	// List lists all keys in the given namespace.
	List(ctx context.Context, namespace []string, limit int) ([]string, error)

	// Batch executes multiple operations atomically.
	Batch(ctx context.Context, ops []Op) ([]Result, error)

	// GetItem retrieves a value with metadata (created_at, updated_at).
	GetItem(ctx context.Context, namespace []string, key string, refreshTTL *bool) (*Item, error)

	// PutItem stores a value with TTL and indexing options.
	PutItem(ctx context.Context, namespace []string, key string, value map[string]interface{},
		index interface{}, ttl *time.Duration) error

	// SearchItems searches for items with advanced filtering and natural language query.
	SearchItems(ctx context.Context, namespace []string, query *string, filter map[string]interface{},
		limit, offset int, refreshTTL *bool) ([]*SearchItem, error)

	// ListNamespaces lists all namespaces matching given conditions.
	ListNamespaces(ctx context.Context, conditions []MatchCondition, maxDepth *int,
		limit, offset int) ([][]string, error)
}

// Op represents a storage operation.
type Op interface{}

// GetOp represents a get operation.
type GetOp struct {
	Namespace  []string
	Key        string
	RefreshTTL bool
}

// PutOp represents a put operation.
type PutOp struct {
	Namespace []string
	Key       string
	Value     map[string]interface{}
	Index     interface{} // false, nil, or []string
	TTL       *time.Duration
}

// SearchOp represents a search operation.
type SearchOp struct {
	NamespacePrefix []string
	Filter          map[string]interface{}
	Limit           int
	Offset          int
	Query           *string // natural language query
	RefreshTTL      bool
}

// ListNamespacesOp represents a list namespaces operation.
type ListNamespacesOp struct {
	MatchConditions []MatchCondition
	MaxDepth        *int
	Limit           int
	Offset          int
}

// Result represents the result of an operation.
type Result struct {
	Value interface{}
	Error error
}

// Item represents a stored item with metadata.
type Item struct {
	Value     map[string]interface{}
	Key       string
	Namespace []string
	CreatedAt time.Time
	UpdatedAt time.Time
	ExpiresAt *time.Time
}

// SearchItem represents a search result with score.
type SearchItem struct {
	*Item
	Score *float64
}

// MatchCondition defines a condition for matching namespaces.
type MatchCondition struct {
	MatchType string // "prefix" or "suffix"
	Path      []string
}

// TTLConfig configures TTL behavior.
type TTLConfig struct {
	RefreshOnRead      bool
	DefaultTTL         *time.Duration
	SweepInterval      *time.Duration
}

// IndexConfig configures semantic search indexing.
type IndexConfig struct {
	Dims   int
	Embed  interface{} // embedding function
	Fields []string
}

// PutOperation represents a single put operation (deprecated, use PutOp).
type PutOperation struct {
	Namespace []string
	Key       string
	Value     map[string]interface{}
}

// SearchOptions provides options for search operations (deprecated).
type SearchOptions struct {
	Limit    int
	Offset   int
	Filter   map[string]interface{}
	SortBy   string
	SortDesc bool
}
