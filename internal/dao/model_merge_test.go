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
	"fmt"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupTenantModelMergeDB(t *testing.T, modelTypeDefinition string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	ddl := fmt.Sprintf(`CREATE TABLE tenant_model (
		id VARCHAR(32) NOT NULL PRIMARY KEY,
		model_name VARCHAR(128),
		provider_id VARCHAR(32) NOT NULL,
		instance_id VARCHAR(32) NOT NULL,
		model_type %s NOT NULL,
		status VARCHAR(32) DEFAULT 'active',
		extra VARCHAR(1024) DEFAULT '{}',
		create_time BIGINT,
		create_date DATETIME,
		update_time BIGINT,
		update_date DATETIME
	)`, modelTypeDefinition)
	if err = db.Exec(ddl).Error; err != nil {
		t.Fatalf("failed to create tenant_model: %v", err)
	}
	return db
}

func insertTenantModelMergeRow(t *testing.T, db *gorm.DB, id, model, provider, instance, modelType, status string) {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO tenant_model (id, model_name, provider_id, instance_id, model_type, status, extra, create_time, create_date, update_time, update_date) VALUES (?, ?, ?, ?, ?, ?, '{}', 1000, NULL, 1000, NULL)`,
		id, model, provider, instance, modelType, status,
	).Error; err != nil {
		t.Fatalf("failed to seed tenant_model row: %v", err)
	}
}

func TestMergeTenantModelTypes(t *testing.T) {
	db := setupTenantModelMergeDB(t, "VARCHAR(32)")
	ctx := t.Context()

	insertTenantModelMergeRow(t, db, "1", "gpt-4o", "p1", "i1", "chat", "active")
	insertTenantModelMergeRow(t, db, "2", "gpt-4o", "p1", "i1", "vision", "active")
	insertTenantModelMergeRow(t, db, "3", "m1", "p2", "i2", "embedding", "unsupported")
	insertTenantModelMergeRow(t, db, "4", "m1", "p2", "i2", "chat", "active")
	insertTenantModelMergeRow(t, db, "5", "m2", "p3", "i3", "chat", "inactive")
	// The seeding stage writes the merged integer into the text column.
	insertTenantModelMergeRow(t, db, "6", "m3", "p4", "i4", "1", "active")

	if err := mergeTenantModelTypes(ctx, db); err != nil {
		t.Fatalf("mergeTenantModelTypes() error = %v", err)
	}

	type mergedRow struct {
		ModelName string `gorm:"column:model_name"`
		ModelType int    `gorm:"column:model_type"`
		Status    string `gorm:"column:status"`
	}
	var rows []mergedRow
	if err := db.Raw("SELECT model_name, model_type, status FROM tenant_model ORDER BY model_name").Scan(&rows).Error; err != nil {
		t.Fatalf("failed to read merged rows: %v", err)
	}

	want := []mergedRow{
		{ModelName: "gpt-4o", ModelType: 9, Status: "active"}, // chat | vision
		{ModelName: "m1", ModelType: 1, Status: "active"},     // chat, embedding dropped as unsupported
		{ModelName: "m3", ModelType: 1, Status: "active"},     // numeric text preserved
	}
	if len(rows) != len(want) {
		t.Fatalf("merged row count = %d, want %d (%+v)", len(rows), len(want), rows)
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Fatalf("merged row %d = %+v, want %+v", i, rows[i], want[i])
		}
	}
}

func TestMergeTenantModelTypesSkipsIntegerColumn(t *testing.T) {
	db := setupTenantModelMergeDB(t, "INT")
	ctx := t.Context()

	insertTenantModelMergeRow(t, db, "1", "gpt-4o", "p1", "i1", "1", "active")
	insertTenantModelMergeRow(t, db, "2", "gpt-4o", "p1", "i1", "1", "active")

	if err := mergeTenantModelTypes(ctx, db); err != nil {
		t.Fatalf("mergeTenantModelTypes() error = %v", err)
	}

	var count int64
	if err := db.Raw("SELECT COUNT(*) FROM tenant_model").Scan(&count).Error; err != nil {
		t.Fatalf("failed to count rows: %v", err)
	}
	if count != 2 {
		t.Fatalf("row count = %d, want 2 (integer column must be left untouched)", count)
	}
}

func TestMergeTenantModelTypesPreservesTimestamps(t *testing.T) {
	db := setupTenantModelMergeDB(t, "VARCHAR(32)")
	ctx := t.Context()

	if err := db.Exec(
		`INSERT INTO tenant_model (id, model_name, provider_id, instance_id, model_type, status, extra, create_time, create_date, update_time, update_date) VALUES (?, ?, ?, ?, ?, ?, '{}', ?, ?, ?, ?)`,
		"1", "gpt-4o", "p1", "i1", "chat", "active", 1700000000000, "2023-11-14 22:13:20", 1700000000000, "2023-11-14 22:13:20",
	).Error; err != nil {
		t.Fatalf("failed to seed tenant_model row: %v", err)
	}

	if err := mergeTenantModelTypes(ctx, db); err != nil {
		t.Fatalf("mergeTenantModelTypes() error = %v", err)
	}

	var row struct {
		CreateTime int64 `gorm:"column:create_time"`
		UpdateTime int64 `gorm:"column:update_time"`
	}
	if err := db.Raw("SELECT create_time, update_time FROM tenant_model").Scan(&row).Error; err != nil {
		t.Fatalf("failed to read merged row: %v", err)
	}
	if row.CreateTime != 1700000000000 || row.UpdateTime != 1700000000000 {
		t.Fatalf("merged timestamps = (%d, %d), want (1700000000000, 1700000000000)", row.CreateTime, row.UpdateTime)
	}
}

func TestMergeTenantModelTypesClearsDroppedRows(t *testing.T) {
	db := setupTenantModelMergeDB(t, "VARCHAR(32)")
	ctx := t.Context()

	insertTenantModelMergeRow(t, db, "1", "gpt-4o", "p1", "i1", "chat", "inactive")

	if err := mergeTenantModelTypes(ctx, db); err != nil {
		t.Fatalf("mergeTenantModelTypes() error = %v", err)
	}

	var count int64
	if err := db.Raw("SELECT COUNT(*) FROM tenant_model").Scan(&count).Error; err != nil {
		t.Fatalf("failed to count rows: %v", err)
	}
	if count != 0 {
		t.Fatalf("row count = %d, want 0 (dropped groups must not stay in their text shape)", count)
	}
}

func TestEnsureTenantModelTables(t *testing.T) {
	t.Run("creates the missing tables without touching a text tenant_model", func(t *testing.T) {
		db := setupTenantModelMergeDB(t, "VARCHAR(32)")
		if err := db.Exec("CREATE TABLE tenant_llm (id VARCHAR(32) NOT NULL PRIMARY KEY)").Error; err != nil {
			t.Fatalf("failed to create tenant_llm: %v", err)
		}
		insertTenantModelMergeRow(t, db, "1", "gpt-4o", "p1", "i1", "chat", "active")

		if err := ensureTenantModelTables(t.Context(), db); err != nil {
			t.Fatalf("ensureTenantModelTables() error = %v", err)
		}

		for _, table := range []string{"tenant_model_provider", "tenant_model_instance"} {
			if !db.Migrator().HasTable(table) {
				t.Fatalf("%s was not created", table)
			}
		}
		var modelType string
		if err := db.Raw("SELECT model_type FROM tenant_model").Scan(&modelType).Error; err != nil {
			t.Fatalf("failed to read model_type: %v", err)
		}
		if modelType != "chat" {
			t.Fatalf("model_type = %q, want the untouched text value \"chat\"", modelType)
		}
	})

	t.Run("leaves the database alone without tenant_llm", func(t *testing.T) {
		db := setupTenantModelMergeDB(t, "VARCHAR(32)")

		if err := ensureTenantModelTables(t.Context(), db); err != nil {
			t.Fatalf("ensureTenantModelTables() error = %v", err)
		}
		if db.Migrator().HasTable("tenant_model_provider") {
			t.Fatal("tenant_model_provider must not be created without tenant_llm")
		}
	})
}

func TestLoadTenantModelRowMapParsesTextModelType(t *testing.T) {
	db := setupTenantModelMergeDB(t, "VARCHAR(32)")
	insertTenantModelMergeRow(t, db, "1", "gpt-4o", "p1", "i1", "chat", "active")
	insertTenantModelMergeRow(t, db, "2", "m1", "p2", "i2", "9", "active")

	rows, err := loadTenantModelRowMap(db)
	if err != nil {
		t.Fatalf("loadTenantModelRowMap() error = %v", err)
	}
	if got := rows["p1\x00i1\x00gpt-4o"].modelType; got != 1 {
		t.Fatalf("gpt-4o modelType = %d, want 1 (text name)", got)
	}
	if got := rows["p2\x00i2\x00m1"].modelType; got != 9 {
		t.Fatalf("m1 modelType = %d, want 9 (numeric text)", got)
	}
}

func TestTenantModelModelTypeIsInteger(t *testing.T) {
	cases := []struct {
		name       string
		definition string
		want       bool
	}{
		{name: "text", definition: "VARCHAR(32)", want: false},
		{name: "integer", definition: "INT", want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupTenantModelMergeDB(t, tc.definition)
			got, err := tenantModelModelTypeIsInteger(db)
			if err != nil {
				t.Fatalf("tenantModelModelTypeIsInteger() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("tenantModelModelTypeIsInteger() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMergeTenantModelTypeBits(t *testing.T) {
	cases := []struct {
		raw  string
		want int
	}{
		{raw: "chat", want: 1},
		{raw: "Vision", want: 8},
		{raw: "speech2text", want: 4},
		{raw: " 7 ", want: 7},
		{raw: "unknown", want: 0},
		{raw: "", want: 0},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			if got := mergeTenantModelTypeBits(tc.raw); got != tc.want {
				t.Fatalf("mergeTenantModelTypeBits(%q) = %d, want %d", tc.raw, got, tc.want)
			}
		})
	}
}
