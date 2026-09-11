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
	"strings"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/entity"

	"go.uber.org/zap"
	"golang.org/x/mod/semver"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// migrationDBVersionMarker is the system_settings key that records how far the
// database has been migrated. It is shared with tools/scripts/mysql_migration.py
// so the Python and Go migrations can skip work the other has already done.
const migrationDBVersionMarker = "mysql_migration.database.version"

// modelMigrationBaseVersion is the version the base tenant model table
// migration brings the database to. It mirrors the first --database-version used
// by tools/scripts/run_migrations.sh, so a database already carrying it came
// from a v0.26.0 release and only needs the remaining model data stages.
const modelMigrationBaseVersion = "v0.26.0"

// modelMigrationTargetVersion is the version the remaining tenant model data
// migration brings the database to. It mirrors the final --database-version used
// by tools/scripts/run_migrations.sh.
const modelMigrationTargetVersion = "v0.27.1"

// getDatabaseMigrationVersion reads the stored migration version. It returns an
// empty string when the marker or the system_settings table is absent.
func getDatabaseMigrationVersion(ctx context.Context, db *gorm.DB) (string, error) {
	scoped := db.WithContext(ctx)
	if !scoped.Migrator().HasTable("system_settings") {
		common.Info("Table 'system_settings' does not exist, migration version marker is unavailable")
		return "", nil
	}

	var value string
	err := scoped.Raw("SELECT `value` FROM `system_settings` WHERE `name` = ?", migrationDBVersionMarker).Row().Scan(&value)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return value, nil
}

// setDatabaseMigrationVersion upserts the migration version marker.
func setDatabaseMigrationVersion(ctx context.Context, db *gorm.DB, version string) error {
	scoped := db.WithContext(ctx)
	if !scoped.Migrator().HasTable("system_settings") {
		common.Warn("Table 'system_settings' does not exist, migration version marker was not saved")
		return nil
	}

	nowMs := time.Now().UnixMilli()
	now := time.Now()
	setting := entity.SystemSettings{
		Name:     migrationDBVersionMarker,
		Source:   "migration",
		DataType: "string",
		Value:    version,
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

// shouldSkipMigration mirrors should_skip_migration in the Python migration: a
// missing or unparseable version never skips the migration.
func shouldSkipMigration(currentDBVersion, targetVersion string) bool {
	current := parseMigrationVersion(currentDBVersion)
	target := parseMigrationVersion(targetVersion)
	if current == "" || target == "" {
		return false
	}
	return semver.Compare(current, target) >= 0
}

// parseMigrationVersion normalizes a version to the "vMAJOR.MINOR.PATCH" form
// understood by semver, returning an empty string when it cannot be parsed.
func parseMigrationVersion(version string) string {
	normalized := strings.TrimSpace(version)
	normalized = strings.TrimPrefix(normalized, "v")
	normalized = strings.TrimPrefix(normalized, "V")
	if normalized == "" {
		return ""
	}
	normalized = "v" + normalized
	if !semver.IsValid(normalized) {
		common.Warn("Invalid migration version format", zap.String("version", version))
		return ""
	}
	return normalized
}
