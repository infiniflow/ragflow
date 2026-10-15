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
	"fmt"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/entity"
)

// setupConnectorTestDB initializes an in-memory SQLite database for Connector DAO tests.
func setupConnectorTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to get sql DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

	if err := db.AutoMigrate(&entity.Connector{}, &entity.Connector2Kb{}); err != nil {
		t.Fatalf("failed to migrate connector tables: %v", err)
	}
	return db
}

// TestListByDatasetIDTxCollapsesDuplicateLinks covers duplicate connector2kb rows
// for the same (connector, dataset) pair, which the unique-index-free schema allows.
func TestListByDatasetIDTxCollapsesDuplicateLinks(t *testing.T) {
	db := setupConnectorTestDB(t)

	connectors := []entity.Connector{
		{ID: "connector-a", TenantID: "tenant-1", Name: "A", Source: "s3", Status: "schedule", Config: entity.JSONMap{}},
		{ID: "connector-b", TenantID: "tenant-1", Name: "B", Source: "oss", Status: "schedule", Config: entity.JSONMap{}},
	}
	if err := db.Create(&connectors).Error; err != nil {
		t.Fatalf("create connectors: %v", err)
	}
	links := []entity.Connector2Kb{
		{ID: "link-a", ConnectorID: "connector-a", KbID: "dataset-1", AutoParse: "1"},
		{ID: "link-a-duplicate", ConnectorID: "connector-a", KbID: "dataset-1", AutoParse: "1"},
		{ID: "link-b", ConnectorID: "connector-b", KbID: "dataset-1", AutoParse: "0"},
		{ID: "link-other", ConnectorID: "connector-a", KbID: "dataset-2", AutoParse: "1"},
	}
	if err := db.Create(&links).Error; err != nil {
		t.Fatalf("create connector links: %v", err)
	}

	got, err := NewConnectorDAO().ListByDatasetIDTx(context.Background(), db, "dataset-1")
	if err != nil {
		t.Fatalf("ListByDatasetIDTx failed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 connectors for dataset-1, got %d: %#v", len(got), got)
	}

	seen := make(map[string]string, len(got))
	for _, item := range got {
		if _, ok := seen[item.ID]; ok {
			t.Fatalf("duplicate connector %q returned", item.ID)
		}
		seen[item.ID] = item.AutoParse
	}
	if seen["connector-a"] != "1" || seen["connector-b"] != "0" {
		t.Fatalf("unexpected auto_parse values: %#v", seen)
	}
}
