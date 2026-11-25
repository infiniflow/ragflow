//go:build !ee

package dataset

import (
	"context"

	"gorm.io/gorm"
)

func readableIngestionLogDocuments(context.Context, string) (*gorm.DB, error) { return nil, nil }

func checkIngestionLogDocumentRead(context.Context, string) error { return nil }
