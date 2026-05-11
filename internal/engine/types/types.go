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
type SearchRequest struct {
	// Search target
	IndexNames []string // For ES: index names; For Infinity: treated as table name prefixes
	KbIDs      []string // Knowledge base IDs filter

	// Pagination
	Offset int // Offset for pagination (0-based)
	Limit  int // Limit for pagination

	// Source fields (for ES: fields to return)
	SelectFields []string // List of field names to return

	// Filtering
	Filter map[string]interface{} // Filters for search

	// Match expressions
	MatchExprs []interface{} // List of match expressions: [matchText, matchDense, fusionExpr]

	// Sorting and ranking
	OrderBy     *OrderByExpr       // Order by expression (asc/desc on fields)
	RankFeature map[string]float64 // Rank features for learning to rank
}

// SearchResult unified search result for all engines
type SearchResult struct {
	Chunks []map[string]interface{} // Search results
	Total  int64                    // Total number of matches
}

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

// MatchTextExpr represents a text match expression
type MatchTextExpr struct {
	Fields       []string               // Field names to search (with optional boost, e.g., "title_tks^10")
	MatchingText string                 // Text to match
	TopN         int                    // Number of results to return
	ExtraOptions map[string]interface{} // Additional options (e.g., minimum_should_match, filter)
}

// MatchDenseExpr represents a dense vector match expression
type MatchDenseExpr struct {
	VectorColumnName  string
	EmbeddingData     []float64
	EmbeddingDataType string
	DistanceType      string
	TopN              int
	ExtraOptions      map[string]interface{}
}

// FusionExpr represents a fusion expression for hybrid search
type FusionExpr struct {
	Method       string                 // Fusion method (e.g., "weighted_sum")
	TopN         int                    // TopK for fusion
	FusionParams map[string]interface{} // Fusion parameters (e.g., {"weights": "0.05,0.95"})
}
