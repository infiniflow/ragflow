package dao

import (
	"context"

	"gorm.io/gorm"

	"ragflow/internal/entity"
)

// ListKnowledgeCompileEntries returns the routing metadata of every queue
// entry for datasetID. Variants and TaskTypes use serializer:json, so Find
// loads and deserializes them; callers apply kind filtering in Go to stay
// portable across databases.
func ListKnowledgeCompileEntries(ctx context.Context, db *gorm.DB, table, datasetID string) ([]entity.KnowledgeCompileEntry, error) {
	var entries []entity.KnowledgeCompileEntry
	err := db.WithContext(ctx).Table(table).
		Where("dataset_id = ?", datasetID).
		Select("variants", "task_types").
		Find(&entries).Error
	return entries, err
}

// CountKnowledgeCompileEntries counts all queue entries for datasetID using
// a SQL aggregate. Duplicates are intentional and must be counted separately.
func CountKnowledgeCompileEntries(ctx context.Context, db *gorm.DB, table, datasetID string) (int, error) {
	var count int64
	err := db.WithContext(ctx).Table(table).Where("dataset_id = ?", datasetID).Count(&count).Error
	return int(count), err
}
