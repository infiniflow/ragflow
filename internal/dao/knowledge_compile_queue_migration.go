package dao

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/entity"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const knowledgeCompileQueueTargetVersion = "v1.0.0-rc3.dev5"

// migrateKnowledgeCompileQueues runs with queue producers and workers stopped.
// Each dataset is copied transactionally; rerunning replaces its copied rows
// until the source column is dropped, so interruption cannot duplicate events.
// It runs after the earlier versioned migrations so recording its version
// cannot skip their data changes through the shared database version marker.
func migrateKnowledgeCompileQueues(ctx context.Context, db *gorm.DB) error {
	db = db.WithContext(ctx)
	if !db.Migrator().HasTable(&entity.KnowledgeCompileDataset{}) {
		return nil
	}
	currentVersion, err := GetDatabaseMigrationVersion(ctx, db)
	if err != nil {
		return fmt.Errorf("read database migration version: %w", err)
	}
	for _, queue := range []struct {
		column string
		table  string
	}{
		{"backlog_doc_ids", (entity.KnowledgeCompileBacklog{}).TableName()},
		{"inflight_doc_ids", (entity.KnowledgeCompileInflight{}).TableName()},
	} {
		if !db.Migrator().HasColumn(&entity.KnowledgeCompileDataset{}, queue.column) {
			continue
		}
		var totalDatasets int64
		if err := db.Model(&entity.KnowledgeCompileDataset{}).Count(&totalDatasets).Error; err != nil {
			return fmt.Errorf("count datasets for knowledge_compile_docs.%s migration: %w", queue.column, err)
		}
		common.Info("Migrating knowledge compilation queue",
			zap.String("source_column", queue.column), zap.String("target_table", queue.table),
			zap.String("target_version", knowledgeCompileQueueTargetVersion),
			zap.Int64("datasets_total", totalDatasets))
		var datasetsProcessed, entriesCopied int64
		lastDatasetID := ""
		firstBatch := true
		for {
			var datasetIDs []string
			query := db.Model(&entity.KnowledgeCompileDataset{}).Order("dataset_id ASC").Limit(100)
			if !firstBatch {
				query = query.Where("dataset_id > ?", lastDatasetID)
			}
			if err := query.Pluck("dataset_id", &datasetIDs).Error; err != nil {
				return err
			}
			if len(datasetIDs) == 0 {
				break
			}
			common.Info("Copying knowledge compilation queue batch",
				zap.String("source_column", queue.column), zap.String("target_table", queue.table),
				zap.String("first_dataset_id", datasetIDs[0]), zap.String("last_dataset_id", datasetIDs[len(datasetIDs)-1]),
				zap.Int64("datasets_processed", datasetsProcessed), zap.Int64("entries_copied", entriesCopied))
			for _, datasetID := range datasetIDs {
				var position int64
				if err := db.Transaction(func(tx *gorm.DB) error {
					var source struct {
						Payload *string `gorm:"column:payload"`
					}
					if err := tx.Model(&entity.KnowledgeCompileDataset{}).
						Clauses(clause.Locking{Strength: "UPDATE"}).
						Select(queue.column+" AS payload").Where("dataset_id = ?", datasetID).Take(&source).Error; err != nil {
						return err
					}
					if err := tx.Table(queue.table).Where("dataset_id = ?", datasetID).
						Delete(&entity.KnowledgeCompileEntry{}).Error; err != nil {
						return err
					}
					if source.Payload == nil || strings.TrimSpace(*source.Payload) == "" || strings.TrimSpace(*source.Payload) == "null" {
						return nil
					}
					decoder := json.NewDecoder(strings.NewReader(*source.Payload))
					token, err := decoder.Token()
					if err != nil {
						return err
					}
					if token != json.Delim('[') {
						return fmt.Errorf("expected an event array")
					}
					for decoder.More() {
						entries := make([]entity.KnowledgeCompileEntry, 0, 100)
						for len(entries) < 100 && decoder.More() {
							var entry entity.KnowledgeCompileEntry
							if err := decoder.Decode(&entry); err != nil {
								return err
							}
							position++
							entry.DatasetID = datasetID
							entry.QueuePosition = position
							entries = append(entries, entry)
						}
						if err := tx.Table(queue.table).Create(&entries).Error; err != nil {
							return err
						}
					}
					if _, err := decoder.Token(); err != nil {
						return err
					}
					if err := decoder.Decode(new(json.RawMessage)); err != io.EOF {
						return fmt.Errorf("unexpected trailing data after event array")
					}
					return nil
				}); err != nil {
					common.Warn("Knowledge compilation queue migration failed",
						zap.String("source_column", queue.column), zap.String("target_table", queue.table),
						zap.String("dataset_id", datasetID), zap.Int64("datasets_processed", datasetsProcessed),
						zap.Int64("entries_copied", entriesCopied), zap.Error(err))
					return fmt.Errorf("split knowledge_compile_docs.%s for dataset %s: %w", queue.column, datasetID, err)
				}
				datasetsProcessed++
				entriesCopied += position
			}
			lastDatasetID = datasetIDs[len(datasetIDs)-1]
			firstBatch = false
			common.Info("Copied knowledge compilation queue batch",
				zap.String("source_column", queue.column), zap.String("target_table", queue.table),
				zap.String("last_dataset_id", lastDatasetID), zap.Int64("datasets_processed", datasetsProcessed),
				zap.Int64("datasets_total", totalDatasets), zap.Int64("datasets_remaining", totalDatasets-datasetsProcessed),
				zap.Int64("entries_copied", entriesCopied))
		}
		if err := db.Migrator().DropColumn(&entity.KnowledgeCompileDataset{}, queue.column); err != nil {
			return fmt.Errorf("drop knowledge_compile_docs.%s: %w", queue.column, err)
		}
		if db.Migrator().HasColumn(&entity.KnowledgeCompileDataset{}, queue.column) {
			return fmt.Errorf("knowledge_compile_docs.%s still exists after migration", queue.column)
		}
		common.Info("Dropped migrated knowledge compilation queue column",
			zap.String("source_column", queue.column), zap.String("target_table", queue.table),
			zap.Int64("datasets_processed", datasetsProcessed), zap.Int64("entries_copied", entriesCopied))
	}
	if !shouldSkipMigration(currentVersion, knowledgeCompileQueueTargetVersion) {
		if err := setDatabaseMigrationVersion(ctx, db, knowledgeCompileQueueTargetVersion); err != nil {
			return fmt.Errorf("mark knowledge compilation queue migration version: %w", err)
		}
		currentVersion = knowledgeCompileQueueTargetVersion
	}
	common.Info("Knowledge compilation queue migration completed",
		zap.String("version", currentVersion), zap.String("target_version", knowledgeCompileQueueTargetVersion))
	return nil
}
