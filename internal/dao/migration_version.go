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
	"strconv"
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
const modelMigrationTargetVersion = "v0.27.2"

// devPrereleaseIdentifier is the prerelease identifier that marks a version as
// a development build of the version it is appended to.
const devPrereleaseIdentifier = "dev"

// GetDatabaseMigrationVersion reads the stored migration version. It returns an
// empty string when the marker or the system_settings table is absent.
func GetDatabaseMigrationVersion(ctx context.Context, db *gorm.DB) (string, error) {
	return getDatabaseMigrationVersion(ctx, db, migrationDBVersionMarker)
}

func getDatabaseMigrationVersion(ctx context.Context, db *gorm.DB, marker string) (string, error) {
	scoped := db.WithContext(ctx)
	if !scoped.Migrator().HasTable("system_settings") {
		common.Info("Table 'system_settings' does not exist, migration version marker is unavailable")
		return "", nil
	}

	var value string
	err := scoped.Raw("SELECT `value` FROM `system_settings` WHERE `name` = ?", marker).Row().Scan(&value)
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
	return setDatabaseMigrationVersionForMarker(ctx, db, migrationDBVersionMarker, version)
}

func setDatabaseMigrationVersionForMarker(ctx context.Context, db *gorm.DB, marker, version string) error {
	scoped := db.WithContext(ctx)
	if !scoped.Migrator().HasTable("system_settings") {
		common.Warn("Table 'system_settings' does not exist, migration version marker was not saved")
		return nil
	}

	nowMs := time.Now().UnixMilli()
	now := time.Now()
	setting := entity.SystemSettings{
		Name:     marker,
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
// missing or unparseable version never skips the migration. Both sides share
// the mysql_migration.database.version marker, so they have to order versions
// the same way, which for Go means compareMigrationVersion instead of raw
// semver.
func shouldSkipMigration(currentDBVersion, targetVersion string) bool {
	current := parseMigrationVersion(currentDBVersion)
	target := parseMigrationVersion(targetVersion)
	if current == "" || target == "" {
		return false
	}
	return compareMigrationVersion(current, target) >= 0
}

// compareMigrationVersion orders two canonical migration versions, returning -1,
// 0 or 1 as current sorts before, equal to, or after target.
//
// It follows semver except for development and post-release versions, where it
// matches PEP 440, the ordering the Python migration gets from
// packaging.version. A trailing ".devN" marks a build leading the version it is
// attached to, while a numeric "-N" suffix marks a post-release version.
func compareMigrationVersion(current, target string) int {
	currentCore := migrationVersionCore(current)
	targetCore := migrationVersionCore(target)
	if cmp := semver.Compare(currentCore, targetCore); cmp != 0 {
		return cmp
	}
	currentPost, isCurrentPost := postReleaseNumber(current)
	targetPost, isTargetPost := postReleaseNumber(target)
	if isCurrentPost || isTargetPost {
		switch {
		case isCurrentPost && isTargetPost:
			return comparePostRelease(currentPost, targetPost)
		case isCurrentPost:
			return 1
		default:
			return -1
		}
	}
	currentBase, currentDev := splitDevPrerelease(semver.Prerelease(current))
	targetBase, targetDev := splitDevPrerelease(semver.Prerelease(target))
	if cmp := semver.Compare(currentCore+currentBase, targetCore+targetBase); cmp != 0 {
		return cmp
	}
	switch {
	case currentDev == targetDev:
		return 0
	case currentDev < 0:
		// The base version outranks every dev build leading up to it.
		return 1
	case targetDev < 0:
		return -1
	case currentDev < targetDev:
		return -1
	}
	return 1
}

type postReleaseVersion struct {
	number    int
	devNumber int
	isDev     bool
}

// postReleaseNumber recognizes the PEP 440 "-N" post-release shorthand, such
// as v1.0.0-1. semver parses it as a numeric prerelease, which sorts before rc
// versions and would make a later database migration appear older.
func postReleaseNumber(version string) (postReleaseVersion, bool) {
	prerelease := strings.TrimPrefix(semver.Prerelease(version), "-")
	if prerelease == "" {
		return postReleaseVersion{}, false
	}
	parts := strings.Split(prerelease, ".")
	number, err := strconv.Atoi(parts[0])
	if err != nil {
		return postReleaseVersion{}, false
	}
	post := postReleaseVersion{number: number}
	if len(parts) == 1 {
		return post, true
	}
	if len(parts) != 2 {
		return postReleaseVersion{}, false
	}
	dev, ok := devIdentifierNumber(parts[1])
	if !ok {
		return postReleaseVersion{}, false
	}
	post.devNumber = dev
	post.isDev = true
	return post, true
}

func comparePostRelease(current, target postReleaseVersion) int {
	if current.number < target.number {
		return -1
	}
	if current.number > target.number {
		return 1
	}
	if current.isDev && !target.isDev {
		return -1
	}
	if !current.isDev && target.isDev {
		return 1
	}
	if current.devNumber < target.devNumber {
		return -1
	}
	if current.devNumber > target.devNumber {
		return 1
	}
	return 0
}

// migrationVersionCore strips prerelease and build metadata, leaving the
// "vMAJOR.MINOR.PATCH" core the two versions are first compared on.
func migrationVersionCore(version string) string {
	core := strings.TrimSuffix(version, semver.Build(version))
	return strings.TrimSuffix(core, semver.Prerelease(version))
}

// splitDevPrerelease splits a "-rc1.dev1" prerelease into its "-rc1" base and
// the trailing development number. dev is -1 when there is no dev identifier.
func splitDevPrerelease(prerelease string) (base string, dev int) {
	trimmed := strings.TrimPrefix(prerelease, "-")
	if trimmed == "" {
		return "", -1
	}
	identifiers := strings.Split(trimmed, ".")
	number, ok := devIdentifierNumber(identifiers[len(identifiers)-1])
	if !ok {
		return prerelease, -1
	}
	base = strings.Join(identifiers[:len(identifiers)-1], ".")
	if base != "" {
		base = "-" + base
	}
	return base, number
}

// devIdentifierNumber reads the N out of a "devN" prerelease identifier.
func devIdentifierNumber(identifier string) (int, bool) {
	number, ok := strings.CutPrefix(identifier, devPrereleaseIdentifier)
	if !ok {
		return 0, false
	}
	value, err := strconv.Atoi(number)
	if err != nil {
		return 0, false
	}
	return value, true
}

// parseMigrationVersion normalizes a version to the semver form used for
// ordering, returning an empty string when it cannot be parsed. A numeric
// fourth component is a post-release counter: v1.0.0.1 becomes v1.0.0-1.
func parseMigrationVersion(version string) string {
	normalized := strings.TrimSpace(version)
	normalized = strings.TrimPrefix(normalized, "v")
	normalized = strings.TrimPrefix(normalized, "V")
	if normalized == "" {
		return ""
	}
	normalized = "v" + normalized
	if !semver.IsValid(normalized) {
		parts := strings.Split(normalized, ".")
		if len(parts) == 4 {
			if _, err := strconv.Atoi(parts[3]); err == nil {
				candidate := strings.Join(parts[:3], ".") + "-" + parts[3]
				if semver.IsValid(candidate) {
					normalized = candidate
				}
			}
		}
	}
	if !semver.IsValid(normalized) {
		common.Warn("Invalid migration version format", zap.String("version", version))
		return ""
	}
	return normalized
}
