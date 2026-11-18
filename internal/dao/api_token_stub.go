//go:build !ee

package dao

import (
	"context"

	"gorm.io/gorm"
)

// GetAPITokenLimit returns the fixed community API key limit.
func GetAPITokenLimit(context.Context, *gorm.DB) (int, error) {
	return DefaultAPITokenLimit, nil
}
