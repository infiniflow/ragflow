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
	"strings"
	"testing"

	"gorm.io/gorm"

	"ragflow/internal/entity"
)

func seedPipelineOperationLog(t *testing.T, db *gorm.DB, log *entity.PipelineOperationLog) {
	t.Helper()
	if err := db.Create(log).Error; err != nil {
		t.Fatalf("seed pipeline operation log %s: %v", log.ID, err)
	}
}

func pipelineOperationLogFixture(id string, dsl entity.JSONMap) *entity.PipelineOperationLog {
	runCount := 1
	return &entity.PipelineOperationLog{
		ID:              id,
		DocumentID:      "doc-" + id,
		RunCount:        &runCount,
		TenantID:        "tenant-1",
		KbID:            "kb-1",
		ParserID:        "naive",
		DocumentName:    id + ".txt",
		DocumentSuffix:  "txt",
		DocumentType:    "document",
		SourceFrom:      "local",
		Progress:        1,
		ProcessDuration: 1,
		DSL:             dsl,
		TaskType:        string(entity.PipelineTaskTypeParse),
		OperationStatus: string(entity.TaskStatusDone),
	}
}

func TestPipelineOperationLogDAOResolvesExactDSLVersion(t *testing.T) {
	db := setupPipelineDSLVersionTestDB(t)
	if err := db.AutoMigrate(&entity.PipelineOperationLog{}); err != nil {
		t.Fatalf("auto-migrate pipeline operation logs: %v", err)
	}
	versions := []*entity.PipelineDSLVersion{
		{DSLID: "pipeline-1", Version: 1, DSL: entity.JSONMap{"revision": float64(1)}},
		{DSLID: "pipeline-1", Version: 2, DSL: entity.JSONMap{"revision": float64(2)}},
	}
	if err := db.Create(&versions).Error; err != nil {
		t.Fatalf("seed pipeline DSL versions: %v", err)
	}

	dslID := "pipeline-1"
	versionOne := int64(1)
	versionTwo := int64(2)
	first := pipelineOperationLogFixture("version-1", entity.JSONMap{"stale": true})
	first.DSLID = &dslID
	first.DSLVersion = &versionOne
	second := pipelineOperationLogFixture("version-2", entity.JSONMap{})
	second.DSLID = &dslID
	second.DSLVersion = &versionTwo
	legacy := pipelineOperationLogFixture("legacy", entity.JSONMap{"legacy": true})
	seedPipelineOperationLog(t, db, first)
	seedPipelineOperationLog(t, db, second)
	seedPipelineOperationLog(t, db, legacy)

	logDAO := NewPipelineOperationLogDAO()
	got, err := logDAO.GetByIDAndKBIDWithDSL(t.Context(), db, first.ID, first.KbID)
	if err != nil {
		t.Fatalf("get referenced pipeline operation log: %v", err)
	}
	if got.DSL["revision"] != float64(1) {
		t.Fatalf("resolved DSL = %#v, want version 1", got.DSL)
	}

	logs, count, err := logDAO.GetFileLogsByKBID(
		t.Context(), db, "kb-1", 1, 30, nil, "", "", nil, "", "",
	)
	if err != nil {
		t.Fatalf("list pipeline operation logs: %v", err)
	}
	if count != 3 {
		t.Fatalf("pipeline operation log count = %d, want 3", count)
	}
	byID := make(map[string]*entity.PipelineOperationLog, len(logs))
	for _, log := range logs {
		byID[log.ID] = log
	}
	if byID[first.ID].DSL["revision"] != float64(1) {
		t.Fatalf("first listed DSL = %#v, want version 1", byID[first.ID].DSL)
	}
	if byID[second.ID].DSL["revision"] != float64(2) {
		t.Fatalf("second listed DSL = %#v, want version 2", byID[second.ID].DSL)
	}
	if byID[legacy.ID].DSL["legacy"] != true {
		t.Fatalf("legacy listed DSL = %#v, want embedded DSL", byID[legacy.ID].DSL)
	}
}

func TestPipelineOperationLogDAOStrictLookupRejectsBrokenDSLReference(t *testing.T) {
	db := setupPipelineDSLVersionTestDB(t)
	if err := db.AutoMigrate(&entity.PipelineOperationLog{}); err != nil {
		t.Fatalf("auto-migrate pipeline operation logs: %v", err)
	}

	dslID := "pipeline-1"
	incomplete := pipelineOperationLogFixture("incomplete", entity.JSONMap{"stale": true})
	incomplete.DSLID = &dslID
	seedPipelineOperationLog(t, db, incomplete)

	logDAO := NewPipelineOperationLogDAO()
	if _, err := logDAO.GetByIDAndKBIDWithDSL(t.Context(), db, incomplete.ID, incomplete.KbID); err == nil || !strings.Contains(err.Error(), "incomplete DSL reference") {
		t.Fatalf("incomplete reference error = %v", err)
	}

	missingVersion := int64(3)
	missing := pipelineOperationLogFixture("missing", entity.JSONMap{"stale": true})
	missing.DSLID = &dslID
	missing.DSLVersion = &missingVersion
	seedPipelineOperationLog(t, db, missing)
	if _, err := logDAO.GetByIDAndKBIDWithDSL(t.Context(), db, missing.ID, missing.KbID); err == nil || !strings.Contains(err.Error(), "missing pipeline DSL version") {
		t.Fatalf("missing reference error = %v", err)
	}

	for _, log := range []*entity.PipelineOperationLog{incomplete, missing} {
		got, err := logDAO.GetByID(t.Context(), db, log.ID)
		if err != nil {
			t.Fatalf("get metadata for %s: %v", log.ID, err)
		}
		if got.ID != log.ID {
			t.Fatalf("metadata ID = %q, want %q", got.ID, log.ID)
		}
	}
}

func TestPipelineOperationLogDAOListIsolatesBrokenDSLReferences(t *testing.T) {
	db := setupPipelineDSLVersionTestDB(t)
	if err := db.AutoMigrate(&entity.PipelineOperationLog{}); err != nil {
		t.Fatalf("auto-migrate pipeline operation logs: %v", err)
	}
	if err := db.Create(&entity.PipelineDSLVersion{
		DSLID: "pipeline-1", Version: 1, DSL: entity.JSONMap{"revision": float64(1)},
	}).Error; err != nil {
		t.Fatalf("seed pipeline DSL version: %v", err)
	}

	dslID := "pipeline-1"
	versionOne := int64(1)
	missingVersion := int64(2)
	healthy := pipelineOperationLogFixture("healthy", entity.JSONMap{})
	healthy.DSLID = &dslID
	healthy.DSLVersion = &versionOne
	incompleteID := pipelineOperationLogFixture("incomplete-id", entity.JSONMap{"stale": true})
	incompleteID.DSLID = &dslID
	incompleteVersion := pipelineOperationLogFixture("incomplete-version", entity.JSONMap{"stale": true})
	incompleteVersion.DSLVersion = &versionOne
	missing := pipelineOperationLogFixture("missing", entity.JSONMap{"stale": true})
	missing.DSLID = &dslID
	missing.DSLVersion = &missingVersion
	legacy := pipelineOperationLogFixture("legacy", entity.JSONMap{"legacy": true})
	for _, log := range []*entity.PipelineOperationLog{healthy, incompleteID, incompleteVersion, missing, legacy} {
		seedPipelineOperationLog(t, db, log)
	}

	logs, count, err := NewPipelineOperationLogDAO().GetFileLogsByKBID(
		t.Context(), db, "kb-1", 1, 30, nil, "", "", nil, "", "",
	)
	if err != nil {
		t.Fatalf("list pipeline operation logs: %v", err)
	}
	if count != 5 || len(logs) != 5 {
		t.Fatalf("listed %d of %d logs, want 5", len(logs), count)
	}

	byID := make(map[string]*entity.PipelineOperationLog, len(logs))
	for _, log := range logs {
		byID[log.ID] = log
	}
	if got := byID[healthy.ID]; got.DSL["revision"] != float64(1) || got.DSLResolutionError != "" {
		t.Fatalf("healthy log = %#v, want resolved version", got)
	}
	for _, incomplete := range []*entity.PipelineOperationLog{incompleteID, incompleteVersion} {
		if got := byID[incomplete.ID]; got.DSL != nil || !strings.Contains(got.DSLResolutionError, "incomplete DSL reference") {
			t.Fatalf("incomplete log = %#v, want isolated resolution error", got)
		}
	}
	if got := byID[missing.ID]; got.DSL != nil || !strings.Contains(got.DSLResolutionError, "missing pipeline DSL version") {
		t.Fatalf("missing log = %#v, want isolated resolution error", got)
	}
	if got := byID[legacy.ID]; got.DSL["legacy"] != true || got.DSLResolutionError != "" {
		t.Fatalf("legacy log = %#v, want embedded DSL", got)
	}
}

func TestPipelineOperationLogDAOListHandlesOnlyIncompleteDSLReferences(t *testing.T) {
	db := setupPipelineDSLVersionTestDB(t)
	if err := db.AutoMigrate(&entity.PipelineOperationLog{}); err != nil {
		t.Fatalf("auto-migrate pipeline operation logs: %v", err)
	}

	dslID := "pipeline-1"
	version := int64(1)
	idOnly := pipelineOperationLogFixture("id-only", entity.JSONMap{"stale": true})
	idOnly.DSLID = &dslID
	versionOnly := pipelineOperationLogFixture("version-only", entity.JSONMap{"stale": true})
	versionOnly.DSLVersion = &version
	seedPipelineOperationLog(t, db, idOnly)
	seedPipelineOperationLog(t, db, versionOnly)

	logs, count, err := NewPipelineOperationLogDAO().GetFileLogsByKBID(
		t.Context(), db, "kb-1", 1, 30, nil, "", "", nil, "", "",
	)
	if err != nil {
		t.Fatalf("list incomplete pipeline operation logs: %v", err)
	}
	if count != 2 || len(logs) != 2 {
		t.Fatalf("listed %d of %d logs, want 2", len(logs), count)
	}
	for _, log := range logs {
		if log.DSL != nil || !strings.Contains(log.DSLResolutionError, "incomplete DSL reference") {
			t.Fatalf("incomplete log = %#v, want isolated resolution error", log)
		}
	}
}

func TestPipelineOperationLogDAOListReturnsDSLVersionQueryError(t *testing.T) {
	db := setupPipelineDSLVersionTestDB(t)
	if err := db.AutoMigrate(&entity.PipelineOperationLog{}); err != nil {
		t.Fatalf("auto-migrate pipeline operation logs: %v", err)
	}
	dslID := "pipeline-1"
	version := int64(1)
	log := pipelineOperationLogFixture("referenced", entity.JSONMap{})
	log.DSLID = &dslID
	log.DSLVersion = &version
	seedPipelineOperationLog(t, db, log)
	if err := db.Migrator().DropTable(&entity.PipelineDSLVersion{}); err != nil {
		t.Fatalf("drop pipeline DSL version table: %v", err)
	}

	_, _, err := NewPipelineOperationLogDAO().GetFileLogsByKBID(
		t.Context(), db, "kb-1", 1, 30, nil, "", "", nil, "", "",
	)
	if err == nil || !strings.Contains(err.Error(), "load pipeline DSL versions") {
		t.Fatalf("list error = %v, want DSL version query error", err)
	}
}
