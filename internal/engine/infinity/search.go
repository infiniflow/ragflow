package infinity

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"ragflow/internal/engine/types"
)

// SearchRequest Infinity search request (legacy, kept for backward compatibility)
type SearchRequest struct {
	TableName   string
	ColumnNames []string
	MatchText   *MatchTextExpr
	MatchDense  *MatchDenseExpr
	Fusion      *FusionExpr
	Offset      int
	Limit       int
	Filter      map[string]interface{}
}

// SearchResponse Infinity search response
type SearchResponse struct {
	Rows  []map[string]interface{}
	Total int64
}

// MatchTextExpr text match expression
type MatchTextExpr struct {
	Fields       []string
	MatchingText string
	TopN         int
	ExtraOptions map[string]interface{}
}

// MatchDenseExpr vector match expression
type MatchDenseExpr struct {
	VectorColumnName  string
	EmbeddingData     []float64
	EmbeddingDataType string
	DistanceType      string
	TopN              int
	ExtraOptions      map[string]interface{}
}

// FusionExpr fusion expression
type FusionExpr struct {
	Method       string
	TopN         int
	Weights      []float64
	FusionParams map[string]interface{}
}

// Search executes search (supports both unified engine.SearchRequest and legacy SearchRequest)
func (e *infinityEngine) Search(ctx context.Context, req interface{}) (interface{}, error) {
	switch searchReq := req.(type) {
	case *types.SearchRequest:
		return e.searchUnified(ctx, searchReq)
	case *SearchRequest:
		return e.searchLegacy(ctx, searchReq)
	default:
		return nil, fmt.Errorf("invalid search request type: %T", req)
	}
}

// searchUnified handles the unified engine.SearchRequest
func (e *infinityEngine) searchUnified(ctx context.Context, req *types.SearchRequest) (*types.SearchResponse, error) {
	if len(req.IndexNames) == 0 {
		return nil, fmt.Errorf("index names cannot be empty")
	}

	// For Infinity, we use the first index name as table name
	tableName := req.IndexNames[0]

	// Get retrieval parameters with defaults
	similarityThreshold := req.SimilarityThreshold
	if similarityThreshold <= 0 {
		similarityThreshold = 0.1
	}

	topK := req.TopK
	if topK <= 0 {
		topK = 1024
	}

	vectorSimilarityWeight := req.VectorSimilarityWeight
	if vectorSimilarityWeight < 0 || vectorSimilarityWeight > 1 {
		vectorSimilarityWeight = 0.3
	}

	pageSize := req.Size
	if pageSize <= 0 {
		pageSize = 30
	}

	offset := (req.Page - 1) * pageSize
	if offset < 0 {
		offset = 0
	}

	// Build search request
	searchReq := &SearchRequest{
		TableName: tableName,
		Limit:     pageSize,
		Offset:    offset,
		Filter:    buildInfinityFilters(req.KbIDs, req.DocIDs),
	}

	// Add text match (question is always required)
	searchReq.MatchText = &MatchTextExpr{
		Fields:       []string{"title_tks", "content_ltks"},
		MatchingText: req.Question,
		TopN:         topK,
	}

	// Add vector match if vector is provided and not keyword-only mode
	if !req.KeywordOnly && len(req.Vector) > 0 {
		fieldName := buildInfinityVectorFieldName(req.Vector)
		searchReq.MatchDense = &MatchDenseExpr{
			VectorColumnName:  fieldName,
			EmbeddingData:     req.Vector,
			EmbeddingDataType: "float",
			DistanceType:      "cosine",
			TopN:              topK,
			ExtraOptions: map[string]interface{}{
				"similarity": similarityThreshold,
			},
		}
		// Infinity uses weighted_sum fusion with weights
		searchReq.Fusion = &FusionExpr{
			Method: "weighted_sum",
			TopN:   topK,
			Weights: []float64{
				1.0 - vectorSimilarityWeight, // text weight
				vectorSimilarityWeight,       // vector weight
			},
		}
	}

	// Execute the actual search (would call Infinity SDK here)
	// For now, return not implemented
	return nil, fmt.Errorf("infinity search unified not implemented: waiting for official Go SDK")
}

// searchLegacy handles the legacy infinity.SearchRequest (backward compatibility)
func (e *infinityEngine) searchLegacy(ctx context.Context, req *SearchRequest) (*SearchResponse, error) {
	// This would contain the actual Infinity search implementation
	return nil, fmt.Errorf("infinity search legacy not implemented: waiting for official Go SDK")
}

// buildInfinityFilters builds filter conditions for Infinity
func buildInfinityFilters(kbIDs []string, docIDs []string) map[string]interface{} {
	filters := make(map[string]interface{})

	// kb_id filter
	if len(kbIDs) > 0 {
		if len(kbIDs) == 1 {
			filters["kb_id"] = kbIDs[0]
		} else {
			filters["kb_id"] = kbIDs
		}
	}

	// doc_id filter
	if len(docIDs) > 0 {
		if len(docIDs) == 1 {
			filters["doc_id"] = docIDs[0]
		} else {
			filters["doc_id"] = docIDs
		}
	}

	// available_int filter (default to 1 for available chunks)
	filters["available_int"] = 1

	return filters
}

// buildInfinityVectorFieldName builds vector field name based on dimension
func buildInfinityVectorFieldName(vector []float64) string {
	dimension := len(vector)
	var fieldBuilder strings.Builder
	fieldBuilder.WriteString("q_")
	fieldBuilder.WriteString(strconv.Itoa(dimension))
	fieldBuilder.WriteString("_vec")
	return fieldBuilder.String()
}
