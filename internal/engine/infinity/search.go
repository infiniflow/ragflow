package infinity

import (
	"context"
	"fmt"
)

// SearchRequest Infinity search request
type SearchRequest struct {
	TableName    string
	ColumnNames  []string
	MatchText    *MatchTextExpr
	MatchDense   *MatchDenseExpr
	Fusion       *FusionExpr
	Offset       int
	Limit        int
	Filter       map[string]interface{}
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
	FusionParams map[string]interface{}
}

// Search executes search
func (e *infinityEngine) Search(ctx context.Context, req interface{}) (interface{}, error) {
	return nil, fmt.Errorf("infinity search not implemented: waiting for official Go SDK")
}
