package types

// SearchRequest unified search request for all engines
type SearchRequest struct {
	// Common fields
	IndexNames    []string  // For ES: index names; For Infinity: treated as table names
	Question      string    // Search query text
	Vector        []float64 // Embedding vector (optional, for hybrid search)

	// Query analysis results (from QueryBuilder.Question)
	MatchText string   // Processed match text for ES query_string
	Keywords  []string // Extracted keywords from question

	// Filters
	KbIDs  []string // Knowledge base IDs filter
	DocIDs []string // Document IDs filter

	// Pagination
	Page int // Page number (1-based)
	Size int // Page size
	TopK int // Number of candidates for retrieval

	// Search mode
	KeywordOnly bool // If true, only do keyword search (no vector search)

	// Scoring parameters
	SimilarityThreshold    float64 // Minimum similarity score (default: 0.1)
	VectorSimilarityWeight float64 // Weight for vector vs keyword (default: 0.3)

	// Engine-specific options (optional, for advanced use)
	Options map[string]interface{}
}

// SearchResponse unified search response for all engines
type SearchResponse struct {
	Chunks []map[string]interface{} // Search results
	Total  int64                    // Total number of matches
}
