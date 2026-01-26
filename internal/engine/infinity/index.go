package infinity

import (
	"context"
	"fmt"
)

// CreateIndex creates a table/index
func (e *infinityEngine) CreateIndex(ctx context.Context, indexName string, mapping interface{}) error {
	return fmt.Errorf("infinity create table not implemented: waiting for official Go SDK")
}

// DeleteIndex deletes a table/index
func (e *infinityEngine) DeleteIndex(ctx context.Context, indexName string) error {
	return fmt.Errorf("infinity drop table not implemented: waiting for official Go SDK")
}

// IndexExists checks if table/index exists
func (e *infinityEngine) IndexExists(ctx context.Context, indexName string) (bool, error) {
	return false, fmt.Errorf("infinity check table existence not implemented: waiting for official Go SDK")
}
