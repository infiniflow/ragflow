package infinity

import (
	"context"
	"fmt"
)

// IndexDocument indexes a single document
func (e *infinityEngine) IndexDocument(ctx context.Context, tableName, docID string, doc interface{}) error {
	return fmt.Errorf("infinity insert not implemented: waiting for official Go SDK")
}

// BulkIndex indexes documents in bulk
func (e *infinityEngine) BulkIndex(ctx context.Context, tableName string, docs []interface{}) (interface{}, error) {
	return nil, fmt.Errorf("infinity bulk insert not implemented: waiting for official Go SDK")
}

// BulkResponse bulk operation response
type BulkResponse struct {
	Inserted int
}

// GetDocument gets a document
func (e *infinityEngine) GetDocument(ctx context.Context, tableName, docID string) (interface{}, error) {
	return nil, fmt.Errorf("infinity get document not implemented: waiting for official Go SDK")
}

// DeleteDocument deletes a document
func (e *infinityEngine) DeleteDocument(ctx context.Context, tableName, docID string) error {
	return fmt.Errorf("infinity delete not implemented: waiting for official Go SDK")
}
