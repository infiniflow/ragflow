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

package types

// SearchRequest unified search request for all engines
// Fields in this struct are directly passed to the engine's search method
type SearchRequest struct {
	// Search target
	IndexNames []string // For ES: index names; For Infinity: treated as table names
	KbIDs      []string // Knowledge base IDs filter
	DocIDs     []string // Document IDs filter

	// Pagination
	TopK   int // Number of candidates for retrieval
	Offset int // Offset for pagination (0-based)
	Limit  int // Limit for pagination

	// Source fields (for ES: fields to return)
	Source []string // List of field names to return from ES

	// Filtering
	MetaDataFilter map[string]interface{} // Metadata filters for search

	// Highlighting
	HighlightFields []string // Highlight field names (e.g., ["content_ltks", "title_tks"])
	Keywords        []string // Keywords for highlighting (extracted from QueryBuilder)

	// Match expressions (for hybrid search: [matchText, matchDense, fusionExpr])
	MatchExprs []interface{} // List of match expressions: [matchText, matchDense, fusionExpr]

	// Sorting and ranking
	OrderBy     *OrderByExpr       // Order by expression (asc/desc on fields)
	RankFeature map[string]float64 // Rank features for learning to rank

	// Scoring
	SimilarityThreshold float64 // Minimum similarity score (default: 0.0)

	// Engine-specific options (optional, for advanced use)
	Options map[string]interface{}
}

// GetFilters builds a metadata filter map from individual filter fields.
// Corresponds to Python's rag/nlp/search.py:get_filters() (L62-72).
// Fields are added in order: kb_id, doc_id, knowledge_graph_kwd, available_int, entity_kwd,
// from_entity_kwd, to_entity_kwd, removed_kwd. Then any additional MetaDataFilter entries.
func (r *SearchRequest) GetFilters() map[string]interface{} {
	filters := make(map[string]interface{})

	// kb_ids -> kb_id (matching Python L64-66)
	if len(r.KbIDs) > 0 {
		filters["kb_id"] = r.KbIDs
	}

	// doc_ids -> doc_id (matching Python L64-66)
	if len(r.DocIDs) > 0 {
		filters["doc_id"] = r.DocIDs
	}

	// Additional filter fields (matching Python L68-71)
	for _, key := range []string{"knowledge_graph_kwd", "available_int", "entity_kwd", "from_entity_kwd", "to_entity_kwd", "removed_kwd"} {
		if val, ok := r.MetaDataFilter[key]; ok && val != nil {
			filters[key] = val
		}
	}

	// Merge any remaining MetaDataFilter entries (matching Python L72 - condition[key] = req[key])
	for key, val := range r.MetaDataFilter {
		if _, exists := filters[key]; !exists && val != nil {
			filters[key] = val
		}
	}

	return filters
}

// OrderByExpr represents ordering expression for search results.
// Corresponds to Python's common/doc_store/doc_store_base.py:OrderByExpr (L130-140).
type OrderByExpr struct {
	Fields []OrderByField
}

// OrderByField represents a single field ordering.
type OrderByField struct {
	Field string
	Type  OrderByType
}

// OrderByType represents ascending or descending order.
type OrderByType int

const (
	// SortAsc represents ascending order.
	SortAsc OrderByType = 0
	// SortDesc represents descending order.
	SortDesc OrderByType = 1
)

// Asc adds an ascending order field.
func (o *OrderByExpr) Asc(field string) *OrderByExpr {
	o.Fields = append(o.Fields, OrderByField{Field: field, Type: SortAsc})
	return o
}

// Desc adds a descending order field.
func (o *OrderByExpr) Desc(field string) *OrderByExpr {
	o.Fields = append(o.Fields, OrderByField{Field: field, Type: SortDesc})
	return o
}

// FusionExpr represents a fusion expression for hybrid search
// Corresponds to Python's FusionExpr("weighted_sum", topk, {"weights": "0.05,0.95"})
type FusionExpr struct {
	Method       string                 // Fusion method (e.g., "weighted_sum")
	TopN         int                    // TopK for fusion
	FusionParams map[string]interface{} // Fusion parameters (e.g., {"weights": "0.05,0.95"})
}

// MatchDenseExpr represents a dense vector match expression
// Corresponds to Python's MatchDenseExpr(vector_column, embedding_data, 'float', 'cosine', topk, {...})
type MatchDenseExpr struct {
	VectorColumnName  string
	EmbeddingData     []float64
	EmbeddingDataType string
	DistanceType      string
	TopN              int
	ExtraOptions      map[string]interface{}
}

// SearchResult unified search response for all engines (minimal, returned by docEngine.Search)
type SearchResult struct {
	Chunks []map[string]interface{} // Search results
	Total  int64                    // Total number of matches (for retry logic)
}
