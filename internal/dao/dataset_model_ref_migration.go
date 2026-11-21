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
	"errors"
	"fmt"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

// datasetModelRefTargetVersion is the version this step brings the database to.
// It sorts above the other post-AutoMigrate steps that share the same marker
// (v1.0.0-rc2 and v1.0.0-rc2.dev1), which is what keeps the marker monotonic.
const datasetModelRefTargetVersion = "v1.0.0-rc3"

// migrateDatasetModelRefs fills dataset_model_ref from every stored dataset
// config.
//
// The table is maintained by the dataset write path from now on; this is the
// one-time pass that gives the datasets written before it the same coverage.
// Skipping it would leave the model library reporting zero affected datasets
// for models that are in use — the silent zero the count exists to avoid.
//
// It runs after AutoMigrate, which creates the table it writes to, and records
// its own version in the shared marker, so work already done is skipped.
func migrateDatasetModelRefs(ctx context.Context, db *gorm.DB) error {
	currentVersion, err := GetDatabaseMigrationVersion(ctx, db)
	if err != nil {
		return fmt.Errorf("read database migration version: %w", err)
	}
	if shouldSkipMigration(currentVersion, datasetModelRefTargetVersion) {
		if !datasetModelRefsNeedReindex(ctx, db) {
			common.Info("Dataset model references already indexed, skipping",
				zap.String("current_version", currentVersion),
				zap.String("target_version", datasetModelRefTargetVersion))
			return nil
		}
		// The marker says this step ran, but a database whose marker was
		// stamped by a build that did not run it — or whose table was created
		// by a server start and never filled — arrives here with an empty
		// index. Trusting the marker would leave the model library reporting
		// zero affected datasets for models that are in use, which is the
		// silent zero this table exists to prevent.
		common.Warn("Dataset model reference index is empty although the migration is marked done; reindexing",
			zap.String("current_version", currentVersion),
			zap.String("target_version", datasetModelRefTargetVersion))
	}
	if !db.WithContext(ctx).Migrator().HasTable("knowledgebase") ||
		!db.WithContext(ctx).Migrator().HasTable("dataset_model_ref") {
		return nil
	}

	common.Info("Running dataset model reference migration",
		zap.String("current_version", currentVersion),
		zap.String("target_version", datasetModelRefTargetVersion))
	indexed, err := backfillDatasetModelRefs(ctx, db)
	if err != nil {
		return err
	}
	if err := setDatabaseMigrationVersion(ctx, db, datasetModelRefTargetVersion); err != nil {
		return fmt.Errorf("mark database migration version %s: %w", datasetModelRefTargetVersion, err)
	}
	common.Info("Dataset model reference migration completed",
		zap.Int("datasets", indexed),
		zap.String("version", datasetModelRefTargetVersion))
	return nil
}

// datasetModelRefsNeedReindex reports whether a database that the marker calls
// indexed actually is.
//
// Both probes are single-row lookups and this only runs on the --migrate
// action: being wrong here is not a slow start, it is a model library that
// answers "no dataset is affected" for models that are in use.
func datasetModelRefsNeedReindex(ctx context.Context, db *gorm.DB) bool {
	if !db.WithContext(ctx).Migrator().HasTable("knowledgebase") ||
		!db.WithContext(ctx).Migrator().HasTable("dataset_model_ref") {
		return false
	}
	var ref entity.DatasetModelRef
	err := db.WithContext(ctx).Model(&entity.DatasetModelRef{}).Take(&ref).Error
	if err == nil {
		return false
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return false
	}
	var dataset entity.Knowledgebase
	if err := db.WithContext(ctx).Model(&entity.Knowledgebase{}).Take(&dataset).Error; err != nil {
		return false
	}
	return true
}

// datasetModelRefRow reads one dataset's config as raw text, so a malformed
// payload is skipped instead of failing the stage.
type datasetModelRefRow struct {
	ID           string  `gorm:"column:id"`
	TenantID     string  `gorm:"column:tenant_id"`
	ParserConfig *string `gorm:"column:parser_config"`
}

// backfillDatasetModelRefs derives and stores the references of every stored
// dataset, returning how many were indexed.
func backfillDatasetModelRefs(ctx context.Context, db *gorm.DB) (int, error) {
	indexed := 0
	var rows []datasetModelRefRow
	result := db.WithContext(ctx).Model(&entity.Knowledgebase{}).
		Select("id", "tenant_id", "parser_config").Order("id").
		FindInBatches(&rows, parserConfigMigrationBatchSize, func(batchDB *gorm.DB, _ int) error {
			return batchDB.Transaction(func(tx *gorm.DB) error {
				for _, row := range rows {
					config, ok := parseParserConfig(row.ParserConfig)
					if !ok {
						continue
					}
					if err := ReplaceDatasetModelRefs(ctx, tx, row.ID, row.TenantID, config); err != nil {
						return fmt.Errorf("index model references of dataset %q: %w", row.ID, err)
					}
					indexed++
				}
				return nil
			})
		})
	if result.Error != nil {
		return indexed, fmt.Errorf("read dataset parser configs: %w", result.Error)
	}
	return indexed, nil
}
