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
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/entity"
)

func setupMigrationVersionTestDB(t *testing.T, migrate bool) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if migrate {
		if err = db.AutoMigrate(&entity.SystemSettings{}); err != nil {
			t.Fatalf("failed to migrate system_settings: %v", err)
		}
	}
	return db
}

func TestShouldSkipMigration(t *testing.T) {
	cases := []struct {
		name    string
		current string
		target  string
		want    bool
	}{
		{name: "missing current", current: "", target: "v0.27.1", want: false},
		{name: "older current", current: "v0.26.0", target: "v0.27.1", want: false},
		{name: "equal", current: "v0.27.1", target: "v0.27.1", want: true},
		{name: "newer current", current: "v0.28.0", target: "v0.27.1", want: true},
		{name: "missing v prefix", current: "0.27.1", target: "v0.27.1", want: true},
		{name: "invalid current", current: "not-a-version", target: "v0.27.1", want: false},
		{name: "invalid target", current: "v0.27.1", target: "not-a-version", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldSkipMigration(tc.current, tc.target); got != tc.want {
				t.Fatalf("shouldSkipMigration(%q, %q) = %v, want %v", tc.current, tc.target, got, tc.want)
			}
		})
	}
}

func TestGetAndSetDatabaseMigrationVersion(t *testing.T) {
	db := setupMigrationVersionTestDB(t, true)
	ctx := t.Context()

	version, err := getDatabaseMigrationVersion(ctx, db)
	if err != nil {
		t.Fatalf("getDatabaseMigrationVersion() error = %v", err)
	}
	if version != "" {
		t.Fatalf("initial version = %q, want empty", version)
	}

	if err = setDatabaseMigrationVersion(ctx, db, "v0.27.1"); err != nil {
		t.Fatalf("setDatabaseMigrationVersion() error = %v", err)
	}
	version, err = getDatabaseMigrationVersion(ctx, db)
	if err != nil {
		t.Fatalf("getDatabaseMigrationVersion() after set error = %v", err)
	}
	if version != "v0.27.1" {
		t.Fatalf("version = %q, want v0.27.1", version)
	}

	// Upsert must update the existing marker instead of inserting a second row.
	if err = setDatabaseMigrationVersion(ctx, db, "v0.28.0"); err != nil {
		t.Fatalf("setDatabaseMigrationVersion() second error = %v", err)
	}
	version, err = getDatabaseMigrationVersion(ctx, db)
	if err != nil {
		t.Fatalf("getDatabaseMigrationVersion() second error = %v", err)
	}
	if version != "v0.28.0" {
		t.Fatalf("version = %q, want v0.28.0", version)
	}

	var count int64
	if err = db.Model(&entity.SystemSettings{}).Where("name = ?", migrationDBVersionMarker).Count(&count).Error; err != nil {
		t.Fatalf("count markers: %v", err)
	}
	if count != 1 {
		t.Fatalf("marker count = %d, want 1", count)
	}
}

func TestGetDatabaseMigrationVersionWithoutTable(t *testing.T) {
	db := setupMigrationVersionTestDB(t, false)
	ctx := t.Context()

	version, err := getDatabaseMigrationVersion(ctx, db)
	if err != nil {
		t.Fatalf("getDatabaseMigrationVersion() error = %v", err)
	}
	if version != "" {
		t.Fatalf("version = %q, want empty", version)
	}
	if err = setDatabaseMigrationVersion(ctx, db, "v0.27.1"); err != nil {
		t.Fatalf("setDatabaseMigrationVersion() without table error = %v", err)
	}
}

func TestEnsureMigrationVersionTable(t *testing.T) {
	db := setupMigrationVersionTestDB(t, false)
	ctx := t.Context()

	if err := ensureMigrationVersionTable(ctx, db); err != nil {
		t.Fatalf("ensureMigrationVersionTable() error = %v", err)
	}
	if !db.Migrator().HasTable("system_settings") {
		t.Fatal("system_settings was not created")
	}

	// Idempotent: the existing table is left alone.
	if err := ensureMigrationVersionTable(ctx, db); err != nil {
		t.Fatalf("ensureMigrationVersionTable() second error = %v", err)
	}
}

func TestMigrateModelDataPersistsVersionOnFirstRun(t *testing.T) {
	db := setupMigrationVersionTestDB(t, false)
	ctx := t.Context()

	if err := migrateModelData(ctx, db); err != nil {
		t.Fatalf("migrateModelData() error = %v", err)
	}
	version, err := getDatabaseMigrationVersion(ctx, db)
	if err != nil {
		t.Fatalf("getDatabaseMigrationVersion() error = %v", err)
	}
	if version != modelMigrationTargetVersion {
		t.Fatalf("version = %q, want %q: the marker must be persisted on the first run so a populated database is never migrated twice",
			version, modelMigrationTargetVersion)
	}
}
