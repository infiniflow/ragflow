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

import (
	"errors"
	"fmt"

	"go.uber.org/zap"

	"ragflow/internal/common"
)

var ErrDocumentNotFound = errors.New("document not found")

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

// SearchMetadataResult unified search result for metadata indices
type SearchMetadataResult struct {
	MetadataRecords []map[string]interface{} // Metadata search results
	Total           int64                    // Total number of matches
}

// SearchMetadataRequest unified search request for metadata indices
type SearchMetadataRequest struct {
	TenantID     string                 // Tenant ID (index name derived: ragflow_doc_meta_{tenantID})
	Offset       int                    // Pagination offset
	Limit        int                    // Pagination limit
	SelectFields []string               // List of field names to return (nil means all fields)
	Filter       map[string]interface{} // Filters for search
	OrderBy      *OrderByExpr           // Order by expression
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

// LogSearchRequest logs SearchRequest in debug mode
func LogSearchRequest(engineName string, req *SearchRequest) {
	common.Info(fmt.Sprintf("Search in %s started", engineName), zap.Any("indexNames", req.IndexNames))

	if !common.IsDebugEnabled() {
		return
	}

	var matchExprsStr string
	for i, expr := range req.MatchExprs {
		switch e := expr.(type) {
		case *MatchTextExpr:
			matchExprsStr += fmt.Sprintf("    [%d] MatchTextExpr: fields=%v, matchingText=%s, topN=%d, extraOptions=%v\n", i, e.Fields, e.MatchingText, e.TopN, e.ExtraOptions)
		case *MatchDenseExpr:
			matchExprsStr += fmt.Sprintf("    [%d] MatchDenseExpr: vectorColumn=%s, vectorSize=%d, topN=%d, extraOptions=%v\n", i, e.VectorColumnName, len(e.EmbeddingData), e.TopN, e.ExtraOptions)
		case *FusionExpr:
			matchExprsStr += fmt.Sprintf("    [%d] FusionExpr: method=%s, topN=%d, fusionParams=%v\n", i, e.Method, e.TopN, e.FusionParams)
		default:
			matchExprsStr += fmt.Sprintf("    [%d] unknown type\n", i)
		}
	}

	common.Debug(fmt.Sprintf("Search request:\n"+
		"    indexNames=%v\n"+
		"    KbIDs=%v\n"+
		"    offset=%d, limit=%d\n"+
		"    SelectFields=%v\n"+
		"    Filter=%v\n"+
		"    MatchExprs:\n%s    orderBy=%v\n"+
		"    RankFeature=%v",
		req.IndexNames, req.KbIDs, req.Offset, req.Limit, req.SelectFields, req.Filter, matchExprsStr, req.OrderBy, req.RankFeature))
}
