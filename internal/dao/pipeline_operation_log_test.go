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

func setupPipelineOperationLogTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.PipelineOperationLog{}); err != nil {
		t.Fatalf("auto-migrate pipeline operation log: %v", err)
	}
	return db
}

func TestCreateOpenLogPersistsPipelineVersion(t *testing.T) {
	db := setupPipelineOperationLogTestDB(t)
	dao := NewPipelineOperationLogDAO()
	ctx := context.Background()

	run, err := dao.CreateOpenLog(ctx, db, OpenLogInput{
		DocumentID:        "doc-1",
		KbID:              "kb-1",
		TenantID:          "tenant-1",
		ParserID:          "naive",
		DocumentName:      "a.txt",
		DocumentSuffix:    ".txt",
		DocumentType:      "txt",
		SourceFrom:        "local",
		OperationStatus:   string(entity.TaskStatusUnstart),
		RunCount:          1,
		PipelineVersionID: "ver-1",
	})
	if err != nil {
		t.Fatalf("CreateOpenLog: %v", err)
	}
	if run.PipelineVersionID == nil || *run.PipelineVersionID != "ver-1" {
		t.Fatalf("PipelineVersionID = %v, want ver-1", run.PipelineVersionID)
	}

	unversioned, err := dao.CreateOpenLog(ctx, db, OpenLogInput{
		DocumentID:      "doc-2",
		KbID:            "kb-1",
		TenantID:        "tenant-1",
		ParserID:        "naive",
		DocumentName:    "b.txt",
		DocumentSuffix:  ".txt",
		DocumentType:    "txt",
		SourceFrom:      "local",
		OperationStatus: string(entity.TaskStatusUnstart),
		RunCount:        1,
	})
	if err != nil {
		t.Fatalf("CreateOpenLog without version: %v", err)
	}
	if unversioned.PipelineVersionID != nil {
		t.Fatalf("PipelineVersionID = %v, want nil", unversioned.PipelineVersionID)
	}
}

func TestSetResolvedPipelineOnlyUpdatesOpenRuns(t *testing.T) {
	db := setupPipelineOperationLogTestDB(t)
	dao := NewPipelineOperationLogDAO()
	ctx := context.Background()

	open := &entity.PipelineOperationLog{
		ID:              "open-run",
		DocumentID:      "doc-1",
		TenantID:        "tenant-1",
		KbID:            "kb-1",
		ParserID:        "naive",
		DocumentName:    "a.txt",
		DocumentSuffix:  ".txt",
		DocumentType:    "txt",
		SourceFrom:      "local",
		OperationStatus: string(entity.TaskStatusRunning),
	}
	if err := db.Create(open).Error; err != nil {
		t.Fatalf("seed open run: %v", err)
	}

	if err := dao.SetResolvedPipeline(ctx, db, "open-run", "canvas-1", "ver-1"); err != nil {
		t.Fatalf("SetResolvedPipeline: %v", err)
	}
	var openReloaded entity.PipelineOperationLog
	if err := db.First(&openReloaded, "id = ?", "open-run").Error; err != nil {
		t.Fatalf("load open run: %v", err)
	}
	if openReloaded.PipelineID == nil || *openReloaded.PipelineID != "canvas-1" {
		t.Fatalf("open PipelineID = %v, want canvas-1", openReloaded.PipelineID)
	}
	if openReloaded.PipelineVersionID == nil || *openReloaded.PipelineVersionID != "ver-1" {
		t.Fatalf("open PipelineVersionID = %v, want ver-1", openReloaded.PipelineVersionID)
	}

	if err := dao.SetResolvedPipeline(ctx, db, "open-run", "", ""); err != nil {
		t.Fatalf("clear resolved pipeline: %v", err)
	}
	var openCleared entity.PipelineOperationLog
	if err := db.First(&openCleared, "id = ?", "open-run").Error; err != nil {
		t.Fatalf("load cleared open run: %v", err)
	}
	if openCleared.PipelineID != nil || openCleared.PipelineVersionID != nil {
		t.Fatalf("cleared pipeline identity = (%v, %v), want both nil", openCleared.PipelineID, openCleared.PipelineVersionID)
	}

	terminal := &entity.PipelineOperationLog{
		ID:              "terminal-run",
		DocumentID:      "doc-2",
		TenantID:        "tenant-1",
		KbID:            "kb-1",
		ParserID:        "naive",
		DocumentName:    "b.txt",
		DocumentSuffix:  ".txt",
		DocumentType:    "txt",
		SourceFrom:      "local",
		OperationStatus: string(entity.TaskStatusDone),
	}
	if err := db.Create(terminal).Error; err != nil {
		t.Fatalf("seed terminal run: %v", err)
	}
	if err := dao.SetResolvedPipeline(ctx, db, "terminal-run", "canvas-2", "ver-2"); err != nil {
		t.Fatalf("SetResolvedPipeline on terminal run: %v", err)
	}

	var reloaded entity.PipelineOperationLog
	if err := db.First(&reloaded, "id = ?", "terminal-run").Error; err != nil {
		t.Fatalf("load terminal run: %v", err)
	}
	if reloaded.PipelineID != nil || reloaded.PipelineVersionID != nil {
		t.Fatalf("terminal pipeline identity = (%v, %v), want both nil", reloaded.PipelineID, reloaded.PipelineVersionID)
	}
}
