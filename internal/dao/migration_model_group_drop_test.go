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
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func openModelGroupMigrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return db
}

func createModelGroupTable(t *testing.T, db *gorm.DB, table string) {
	t.Helper()
	schema := "CREATE TABLE " + table + " (id TEXT PRIMARY KEY, name TEXT)"
	if err := db.Exec(schema).Error; err != nil {
		t.Fatalf("create %s: %v", table, err)
	}
	if err := db.Exec("INSERT INTO " + table + " (id, name) VALUES ('row-1', 'kept')").Error; err != nil {
		t.Fatalf("seed %s: %v", table, err)
	}
}

func assertTablePresence(t *testing.T, db *gorm.DB, table string, want bool, label string) {
	t.Helper()
	if got := db.Migrator().HasTable(table); got != want {
		t.Fatalf("%s: HasTable(%s) = %v, want %v", label, table, got, want)
	}
}

func TestDropAbandonedModelGroupTablesDropsLegacyLeftovers(t *testing.T) {
	db := openModelGroupMigrationDB(t)
	createModelGroupTable(t, db, "tenant_model_group_mapping")
	createModelGroupTable(t, db, "tenant_model_group")

	if err := dropAbandonedModelGroupTables(context.Background(), db); err != nil {
		t.Fatalf("drop abandoned model group tables: %v", err)
	}

	assertTablePresence(t, db, "tenant_model_group_mapping", false, "legacy mapping must be dropped")
	assertTablePresence(t, db, "tenant_model_group", false, "stale group table without the feature map must be dropped")
}

func TestDropAbandonedModelGroupTablesKeepsLiveFeatureTables(t *testing.T) {
	db := openModelGroupMigrationDB(t)
	createModelGroupTable(t, db, "tenant_model_group")
	createModelGroupTable(t, db, "tenant_model_group_map")

	if err := dropAbandonedModelGroupTables(context.Background(), db); err != nil {
		t.Fatalf("drop abandoned model group tables: %v", err)
	}

	assertTablePresence(t, db, "tenant_model_group", true, "live group table with the feature map must be kept")
	assertTablePresence(t, db, "tenant_model_group_map", true, "feature map table must be kept")
}

func TestDropAbandonedModelGroupTablesNoopOnFreshDatabase(t *testing.T) {
	db := openModelGroupMigrationDB(t)

	if err := dropAbandonedModelGroupTables(context.Background(), db); err != nil {
		t.Fatalf("drop abandoned model group tables: %v", err)
	}

	assertTablePresence(t, db, "tenant_model_group_mapping", false, "fresh database must stay untouched")
	assertTablePresence(t, db, "tenant_model_group", false, "fresh database must stay untouched")
}
