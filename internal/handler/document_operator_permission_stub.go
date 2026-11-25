//go:build !ee

package handler

import (
	"context"

	"ragflow/internal/entity"
)

func documentOperatorPermissions(context.Context, []*entity.DocumentListItem) (map[string]int32, error) {
	return nil, nil
}

func documentOperatorPermission(context.Context, string) (int32, error) {
	return 0, nil
}
