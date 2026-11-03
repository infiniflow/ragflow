// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dao

import (
	"context"
	"testing"

	"ragflow/internal/entity"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// TestMigrateIngestionLogRunIdentityAddsProcessEndAt is a regression test for the
// missing schema migration in #21711: the PR added PipelineOperationLog.ProcessEndAt
// (column process_end_at) and persists it on terminal settlement, but omitted the
// ALTER that adds the column. Without it, every enqueue fails with
// "Unknown column 'process_end_at' in 'field list'".
func TestMigrateIngestionLogRunIdentityAddsProcessEndAt(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	ctx := context.Background()
	if err := db.Exec(`CREATE TABLE pipeline_operation_log (
		id varchar(32) PRIMARY KEY,
		document_id varchar(32) NOT NULL
	)`).Error; err != nil {
		t.Fatalf("create legacy table: %v", err)
	}

	if err := migrateIngestionLogRunIdentity(ctx, db); err != nil {
		t.Fatalf("migrateIngestionLogRunIdentity: %v", err)
	}

	migrator := db.Migrator()
	if !migrator.HasColumn(&entity.PipelineOperationLog{}, "process_end_at") {
		t.Fatal("pipeline_operation_log missing process_end_at column after migration")
	}
	if !migrator.HasIndex(&entity.PipelineOperationLog{}, "idx_pipeline_operation_log_process_end_at") {
		t.Fatal("pipeline_operation_log missing process_end_at index after migration")
	}

	// Idempotent: a second run must not error and must keep the column.
	if err := migrateIngestionLogRunIdentity(ctx, db); err != nil {
		t.Fatalf("second migrateIngestionLogRunIdentity: %v", err)
	}
	if !migrator.HasColumn(&entity.PipelineOperationLog{}, "process_end_at") {
		t.Fatal("process_end_at column lost after second migration")
	}
}
