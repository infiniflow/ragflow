package dao

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"ragflow/internal/entity"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// migrateKnowledgeCompileQueues runs with queue producers and workers stopped.
// Each dataset is copied transactionally; rerunning replaces its copied rows
// until the source column is dropped, so interruption cannot duplicate events.
func migrateKnowledgeCompileQueues(ctx context.Context, db *gorm.DB) error {
	db = db.WithContext(ctx)
	if !db.Migrator().HasTable(&entity.KnowledgeCompileDataset{}) {
		return nil
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
			for _, datasetID := range datasetIDs {
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
					var position int64
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
					return fmt.Errorf("split knowledge_compile_docs.%s for dataset %s: %w", queue.column, datasetID, err)
				}
			}
			lastDatasetID = datasetIDs[len(datasetIDs)-1]
			firstBatch = false
		}
		if err := db.Migrator().DropColumn(&entity.KnowledgeCompileDataset{}, queue.column); err != nil {
			return fmt.Errorf("drop knowledge_compile_docs.%s: %w", queue.column, err)
		}
		if db.Migrator().HasColumn(&entity.KnowledgeCompileDataset{}, queue.column) {
			return fmt.Errorf("knowledge_compile_docs.%s still exists after migration", queue.column)
		}
	}
	return nil
}
