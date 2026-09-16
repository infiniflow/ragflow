//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package dao

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"ragflow/internal/common"
)

// conversationHistoryTargetVersion is the database version a completed split
// brings the database to. It is stored in system_settings under
// mysql_migration.database.version, the marker shared with the tenant model
// migration, so both steps read the same progress record instead of keeping one
// each. The dev prerelease keeps it below v1.0.0-rc1, which owns the schema
// this step only prepares.
const conversationHistoryTargetVersion = "v1.0.0-rc1.dev1"

// conversationHistoryBatchSize bounds how many conversations are read and
// written per transaction, so an interrupted run resumes from a committed
// batch instead of reprocessing the whole table.
const conversationHistoryBatchSize = 100

// legacyConversationHistory addresses conversation.message and
// conversation.reference. ChatSession drops both columns with gorm:"-", so a
// probe model is the only way to reach them.
type legacyConversationHistory struct {
	ID        string  `gorm:"column:id;primaryKey;size:32"`
	Message   *string `gorm:"column:message"`
	Reference *string `gorm:"column:reference"`
}

func (legacyConversationHistory) TableName() string {
	return "conversation"
}

// legacyAPI4ConversationHistory is the api_4_conversation counterpart of
// legacyConversationHistory.
type legacyAPI4ConversationHistory struct {
	ID        string  `gorm:"column:id;primaryKey;size:32"`
	Message   *string `gorm:"column:message"`
	Reference *string `gorm:"column:reference"`
}

func (legacyAPI4ConversationHistory) TableName() string {
	return "api_4_conversation"
}

// legacyHistoryRow is one parent row read for backfill. Columns that the
// parent table does not carry stay nil.
type legacyHistoryRow struct {
	ID        string  `gorm:"column:id"`
	Message   *string `gorm:"column:message"`
	Reference *string `gorm:"column:reference"`
}

// conversationHistorySplit maps one parent table onto its two child tables.
type conversationHistorySplit struct {
	parentTable    string
	probe          interface{}
	messageTable   string
	referenceTable string
}

func (s conversationHistorySplit) childTables() []string {
	return []string{s.messageTable, s.referenceTable}
}

// migrateConversationHistory backfills conversation_message,
// conversation_reference and their api_4_conversation counterparts from the
// message and reference payload columns the parent tables still hold.
//
// It runs after AutoMigrate: unlike RunMigrations, which has to observe the
// legacy schema, it writes to tables AutoMigrate creates.
func migrateConversationHistory(ctx context.Context, db *gorm.DB) error {
	currentVersion, err := GetDatabaseMigrationVersion(ctx, db)
	if err != nil {
		return fmt.Errorf("read database migration version: %w", err)
	}
	if shouldSkipMigration(currentVersion, conversationHistoryTargetVersion) {
		common.Info("Conversation history already split, skipping",
			zap.String("current_version", currentVersion),
			zap.String("target_version", conversationHistoryTargetVersion))
		return nil
	}
	splits := []conversationHistorySplit{
		{
			parentTable:    "conversation",
			probe:          &legacyConversationHistory{},
			messageTable:   conversationMessageTable,
			referenceTable: conversationReferenceTable,
		},
		{
			parentTable:    "api_4_conversation",
			probe:          &legacyAPI4ConversationHistory{},
			messageTable:   apiConversationMessageTable,
			referenceTable: apiConversationReferenceTable,
		},
	}
	for _, split := range splits {
		if err := migrateConversationHistorySplit(ctx, db, split); err != nil {
			return err
		}
	}
	if err := setDatabaseMigrationVersion(ctx, db, conversationHistoryTargetVersion); err != nil {
		return fmt.Errorf("mark database migration version %s: %w", conversationHistoryTargetVersion, err)
	}
	common.Info("Conversation history migration completed", zap.String("version", conversationHistoryTargetVersion))
	return nil
}

func migrateConversationHistorySplit(ctx context.Context, db *gorm.DB, split conversationHistorySplit) error {
	scoped := db.WithContext(ctx)
	if !scoped.Migrator().HasTable(split.parentTable) {
		return nil
	}
	hasMessage := scoped.Migrator().HasColumn(split.probe, "message")
	hasReference := scoped.Migrator().HasColumn(split.probe, "reference")
	if !hasMessage && !hasReference {
		// The Go schema never carried these columns, so there is nothing to split.
		return nil
	}
	columns := []string{"id"}
	if hasMessage {
		columns = append(columns, "message")
	}
	if hasReference {
		columns = append(columns, "reference")
	}

	lastID := ""
	migrated := 0
	for {
		var batch []legacyHistoryRow
		err := scoped.Table(split.parentTable).Select(columns).
			Where("id > ?", lastID).Order("id").Limit(conversationHistoryBatchSize).Find(&batch).Error
		if err != nil {
			return fmt.Errorf("read %s history payloads: %w", split.parentTable, err)
		}
		if len(batch) == 0 {
			break
		}
		err = scoped.Transaction(func(tx *gorm.DB) error {
			for _, row := range batch {
				written, err := splitConversationHistory(ctx, tx, split, row)
				if err != nil {
					return err
				}
				if written {
					migrated++
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		lastID = batch[len(batch)-1].ID
	}
	common.Info("Split conversation history",
		zap.String("table", split.parentTable), zap.Int("conversations", migrated))
	return nil
}

// splitConversationHistory writes one parent row's payloads into the child
// tables. Rows written after the split took effect win over the payload, so a
// rerun never duplicates or clobbers them.
func splitConversationHistory(ctx context.Context, db *gorm.DB, split conversationHistorySplit, row legacyHistoryRow) (bool, error) {
	for _, table := range split.childTables() {
		hasRows, err := conversationHistoryHasRows(ctx, db, table, row.ID)
		if err != nil {
			return false, err
		}
		if hasRows {
			return false, nil
		}
	}
	written := false
	if row.Message != nil {
		rows, err := historyRows("message", row.ID, json.RawMessage(*row.Message))
		if err != nil {
			return false, fmt.Errorf("split %s %s message payload: %w", split.parentTable, row.ID, err)
		}
		if len(rows) > 0 {
			if err := db.WithContext(ctx).Table(split.messageTable).CreateInBatches(rows, 100).Error; err != nil {
				return false, err
			}
			written = true
		}
	}
	if row.Reference != nil {
		rows, err := historyRows("reference", row.ID, json.RawMessage(*row.Reference))
		if err != nil {
			return false, fmt.Errorf("split %s %s reference payload: %w", split.parentTable, row.ID, err)
		}
		if len(rows) > 0 {
			if err := db.WithContext(ctx).Table(split.referenceTable).CreateInBatches(rows, 100).Error; err != nil {
				return false, err
			}
			written = true
		}
	}
	return written, nil
}

func conversationHistoryHasRows(ctx context.Context, db *gorm.DB, table, conversationID string) (bool, error) {
	var count int64
	err := db.WithContext(ctx).Table(table).Where("conversation_id = ?", conversationID).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
