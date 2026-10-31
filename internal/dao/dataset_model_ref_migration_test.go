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

	"ragflow/internal/entity"
)

// The version marker can say the backfill ran on a database where it did not:
// the table gets created by a server start (autoMigrateRuntimeModels) or by a
// build that stamped the marker without filling the table. Trusting the marker
// there leaves the model library answering "no dataset is affected" for models
// that are in use, so the step has to look at the table, not only the marker.
func TestMigrateDatasetModelRefsReindexesWhenMarkerAlreadyStamped(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err = db.AutoMigrate(
		&entity.SystemSettings{},
		&entity.Knowledgebase{},
		&entity.DatasetModelRef{},
	); err != nil {
		t.Fatalf("failed to migrate tables: %v", err)
	}

	ctx := context.Background()
	if err = setDatabaseMigrationVersion(ctx, db, datasetModelRefTargetVersion); err != nil {
		t.Fatalf("stamp migration version: %v", err)
	}

	dataset := &entity.Knowledgebase{
		ID:       "kb-1",
		TenantID: "tenant-1",
		Name:     "Dataset",
		ParserID: "naive",
		EmbdID:   "embedding",
		ParserConfig: entity.JSONMap{
			"Parser:Compiler": map[string]interface{}{
				"compilation_template_group_id": "group-1",
			},
		},
	}
	if err = db.Create(dataset).Error; err != nil {
		t.Fatalf("create dataset: %v", err)
	}

	if err = migrateDatasetModelRefs(ctx, db); err != nil {
		t.Fatalf("migrateDatasetModelRefs: %v", err)
	}

	var refs []entity.DatasetModelRef
	if err = db.Where("dataset_id = ?", "kb-1").Find(&refs).Error; err != nil {
		t.Fatalf("read refs: %v", err)
	}
	if len(refs) != 1 || refs[0].RefType != "group" || refs[0].ModelRef != "group-1" {
		t.Fatalf("refs = %#v, want the group the dataset config names", refs)
	}
}

// A database that is marked indexed and is indexed stays untouched, and one
// without datasets is not probed into a backfill.
func TestDatasetModelRefsNeedReindexOnlyForIndexedClaimsThatAreEmpty(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err = db.AutoMigrate(&entity.Knowledgebase{}, &entity.DatasetModelRef{}); err != nil {
		t.Fatalf("failed to migrate tables: %v", err)
	}
	ctx := context.Background()

	// No datasets: nothing to rebuild, and probing must not invent work.
	if datasetModelRefsNeedReindex(ctx, db) {
		t.Fatal("an empty database was reported as needing a reindex")
	}

	if err = db.Create(&entity.Knowledgebase{ID: "kb-1", TenantID: "tenant-1", Name: "Dataset"}).Error; err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	if err = db.Create(&entity.DatasetModelRef{ID: "ref-1", TenantID: "tenant-1", DatasetID: "kb-1", RefType: "group", ModelRef: "group-1"}).Error; err != nil {
		t.Fatalf("create ref: %v", err)
	}
	if datasetModelRefsNeedReindex(ctx, db) {
		t.Fatal("an indexed database was reported as needing a reindex")
	}
}
