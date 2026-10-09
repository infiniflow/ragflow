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
	"time"

	"ragflow/internal/common"
	"ragflow/internal/entity"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// schemaConvergedMarker is the system_settings key recording which exact build
// last converged the full schema: the manual migrations, the per-model
// AutoMigrate loop, and the runtime-table ensure steps. When a later process
// (the standalone --migrate action or any server mode) starts with the same
// build string, all of that work is known to be a no-op and is skipped: on a
// converged database it costs hundreds of information_schema round trips per
// process while changing nothing.
const schemaConvergedMarker = "schema.converged.code_version"

// schemaConvergedForBuild reports whether the database schema was already
// converged by a --migrate run of this exact build. Any uncertainty answers
// false, which simply keeps the historical run-everything behaviour:
//
//   - development mode never skips (entity edits between runs share a version)
//   - an unreadable or unrecordable build string ("", "unknown") never skips
//   - a missing system_settings table or marker row never skips
func schemaConvergedForBuild(ctx context.Context, db *gorm.DB, codeVersion string) bool {
	if common.DevModeEnabled() {
		return false
	}
	if !recordableBuildVersion(codeVersion) {
		return false
	}
	marker, err := readSchemaConvergedVersion(ctx, db)
	if err != nil {
		common.Warn("Failed to read schema convergence marker, running full schema setup",
			zap.String("error", err.Error()))
		return false
	}
	return marker != "" && marker == codeVersion
}

// markSchemaConverged records that this build's --migrate run converged the
// schema. It is a no-op for builds whose version string cannot identify them,
// and in development mode, so a later non-dev run cannot be lulled into
// skipping real work. The marker tracks the last converging build: a
// downgrade-then-upgrade cycle simply re-runs the full path once per side,
// which is always safe.
func markSchemaConverged(ctx context.Context, db *gorm.DB, codeVersion string) error {
	if common.DevModeEnabled() || !recordableBuildVersion(codeVersion) {
		return nil
	}
	scoped := db.WithContext(ctx)
	if !scoped.Migrator().HasTable("system_settings") {
		return nil
	}
	nowMs := time.Now().UnixMilli()
	now := time.Now()
	setting := entity.SystemSettings{
		Name:     schemaConvergedMarker,
		Source:   "migration",
		DataType: "string",
		Value:    codeVersion,
		BaseModel: entity.BaseModel{
			CreateTime: &nowMs,
			CreateDate: &now,
			UpdateTime: &nowMs,
			UpdateDate: &now,
		},
	}
	return scoped.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "name"}},
		DoUpdates: clause.AssignmentColumns([]string{"source", "data_type", "value", "update_time", "update_date"}),
	}).Create(&setting).Error
}

// recordableBuildVersion reports whether a build version string identifies the
// build precisely enough to key schema convergence on it. GetRAGFlowVersion
// falls back to "unknown" when neither a VERSION file nor git metadata is
// available; such a build must never write or match the marker.
func recordableBuildVersion(codeVersion string) bool {
	return codeVersion != "" && codeVersion != "unknown"
}

// readSchemaConvergedVersion returns the stored marker, or "" when the
// system_settings table or the marker row is absent.
func readSchemaConvergedVersion(ctx context.Context, db *gorm.DB) (string, error) {
	scoped := db.WithContext(ctx)
	if !scoped.Migrator().HasTable("system_settings") {
		return "", nil
	}
	var value string
	err := scoped.Raw("SELECT `value` FROM `system_settings` WHERE `name` = ?", schemaConvergedMarker).Row().Scan(&value)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return value, nil
}
