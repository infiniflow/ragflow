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
	"database/sql"
	"errors"
	"fmt"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/entity"

	"gorm.io/gorm/clause"
)

const rc2MigrationTargetVersion = "v1.0.0-rc2"

// migrateRC2Data coordinates all rc2 data migrations (parser config conversion,
// retired parser config cleanup, and tenant model max_tokens backfill) under a single
// database transaction and version gate, ensuring the completion marker is only
// persisted once all steps have succeeded.
func migrateRC2Data(ctx context.Context, db *gorm.DB) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		version, err := getDatabaseMigrationVersionForUpdate(ctx, tx)
		if err != nil {
			return fmt.Errorf("read database migration version: %w", err)
		}
		if shouldSkipMigration(version, rc2MigrationTargetVersion) {
			common.Info("RC2 data already migrated, skipping",
				zap.String("current_version", version),
				zap.String("target_version", rc2MigrationTargetVersion))
			return nil
		}

		common.Info("Running rc2 data migration",
			zap.String("current_version", version),
			zap.String("target_version", rc2MigrationTargetVersion))

		// 1. Ingestion parser configs conversion (knowledgebase and document)
		if err := runIngestionParserConfigMigration(ctx, tx); err != nil {
			return err
		}

		// 2. Knowledgebase cleanup: remove retired keys (raptor, graphrag)
		if tx.Migrator().HasTable("knowledgebase") {
			if _, err := removeParserConfigKeys(ctx, tx); err != nil {
				return fmt.Errorf("cleanup retired knowledgebase parser config keys: %w", err)
			}
		}

		// 3. Tenant model max_tokens backfill
		if _, err := backfillTenantModelMaxTokens(ctx, tx); err != nil {
			return err
		}

		// 4. Mark completion
		if err := setDatabaseMigrationVersion(ctx, tx, rc2MigrationTargetVersion); err != nil {
			return fmt.Errorf("mark database migration version %s: %w", rc2MigrationTargetVersion, err)
		}
		common.Info("RC2 data migration completed", zap.String("version", rc2MigrationTargetVersion))
		return nil
	})
}

func getDatabaseMigrationVersionForUpdate(ctx context.Context, db *gorm.DB) (string, error) {
	scoped := db.WithContext(ctx)
	if !scoped.Migrator().HasTable("system_settings") {
		common.Info("Table 'system_settings' does not exist, migration version marker is unavailable")
		return "", nil
	}

	var setting entity.SystemSettings
	err := scoped.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("name = ?", migrationDBVersionMarker).Take(&setting).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return setting.Value, nil
}

func runIngestionParserConfigMigration(ctx context.Context, db *gorm.DB) error {
	for _, target := range []struct {
		table string
		model any
	}{
		{"knowledgebase", &entity.Knowledgebase{}},
		{"document", &entity.Document{}},
	} {
		if !db.WithContext(ctx).Migrator().HasTable(target.model) {
			continue
		}
		count, err := migrateIngestionConfigRows(ctx, db, target.model, target.table)
		if err != nil {
			return fmt.Errorf("migrate %s parser configs: %w", target.table, err)
		}
		common.Info("Migrated Python parser configs", zap.String("table", target.table),
			zap.String("version", rc2MigrationTargetVersion), zap.Int("rows", count))
	}
	return nil
}
