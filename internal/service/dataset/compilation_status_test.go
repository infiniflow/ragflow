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

package dataset

import (
	"testing"
	"time"

	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// setupCompilationStatusTestDB migrates the minimal schema for
// GetDatasetCompilationStatus (Knowledgebase for the Accessible check plus the
// KnowledgeCompileDataset scheduling row) and pushes it onto dao.DB.
func setupCompilationStatusTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := setupServiceTestDB(t)
	if err := db.AutoMigrate(&entity.KnowledgeCompileDataset{}, &entity.KnowledgeCompileBacklog{}, &entity.KnowledgeCompileInflight{}); err != nil {
		t.Fatalf("migrate knowledge_compile_docs: %v", err)
	}
	pushServiceDB(t, db)
	return db
}

func TestCompilationQueueCountsPreserveDuplicatesAndRouting(t *testing.T) {
	db := setupCompilationStatusTestDB(t)
	entries := []entity.KnowledgeCompileEntry{
		{DatasetID: "kb", DocID: "same", QueuePosition: 1, TaskTypes: []string{"Graph"}},
		{DatasetID: "kb", DocID: "same", QueuePosition: 2, TaskTypes: []string{"Graph"}},
		{DatasetID: "kb", DocID: "timeline", QueuePosition: 3, Variants: []string{"structure"}, TaskTypes: []string{"Timeline"}},
		{DatasetID: "kb", DocID: "structure", QueuePosition: 4, Variants: []string{"structure"}},
		{DatasetID: "kb", DocID: "wiki", QueuePosition: 5, Variants: []string{"wiki"}},
		{DatasetID: "kb", DocID: "unscoped", QueuePosition: 6},
		{DatasetID: "other-kb", DocID: "same", QueuePosition: 1, TaskTypes: []string{"Graph"}},
	}
	for _, table := range []string{(entity.KnowledgeCompileBacklog{}).TableName(), (entity.KnowledgeCompileInflight{}).TableName()} {
		if err := db.Table(table).Create(&entries).Error; err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct {
			kind string
			want int
		}{
			{"", 6}, {"Graph", 4}, {"Timeline", 3}, {"PageIndex", 2}, {"Wiki", 2},
		} {
			count, err := countCompilationStatusEntries(db, table, "kb", tc.kind)
			if err != nil || count != tc.want {
				t.Fatalf("%s kind=%q count=%d want=%d err=%v", table, tc.kind, count, tc.want, err)
			}
		}
	}
}

// insertCompilationOwnerKB inserts a valid KB owned by userID (TenantID ==
// userID, so Accessible returns true).
func insertCompilationOwnerKB(t *testing.T, kbID, userID string) {
	t.Helper()
	status := string(entity.StatusValid)
	kb := &entity.Knowledgebase{
		ID:         kbID,
		TenantID:   userID,
		Name:       "compile-status-kb",
		EmbdID:     "BAAI/bge-large-zh-v1.5@Builtin",
		CreatedBy:  userID,
		Permission: string(entity.TenantPermissionMe),
		Status:     &status,
	}
	if err := dao.DB.Create(kb).Error; err != nil {
		t.Fatalf("insert kb: %v", err)
	}
	ensureDatasetTestMembership(t, userID, userID, "owner")
}

func testCompilationStatusService() *DatasetService {
	return &DatasetService{kbDAO: dao.NewKnowledgebaseDAO()}
}

// TestGetDatasetCompilationStatus_NoRowIsIdle verifies a dataset with no
// scheduling row reports the idle state with zero counts.
func TestGetDatasetCompilationStatus_NoRowIsIdle(t *testing.T) {
	setupCompilationStatusTestDB(t)
	insertCompilationOwnerKB(t, "kb-no-row", "user-1")

	st, code, err := testCompilationStatusService().GetDatasetCompilationStatus(
		withDatasetTestContext(t.Context(), "user-1", "user-1"), "user-1", "kb-no-row", "graph")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("code=%d want %d", code, common.CodeSuccess)
	}
	if st.State != entity.DatasetStateIdle {
		t.Fatalf("state=%q want idle", st.State)
	}
	if st.Kind != "graph" {
		t.Fatalf("kind=%q want graph", st.Kind)
	}
	if st.Inflight != 0 || st.Backlog != 0 {
		t.Fatalf("expected zero counts for idle, got inflight=%d backlog=%d", st.Inflight, st.Backlog)
	}
	if st.Error != "" {
		t.Fatalf("expected empty error, got %q", st.Error)
	}
}

// TestGetDatasetCompilationStatus_FullOutput locks the complete response
// mapping from the MySQL row: state, inflight/backlog counts, error diagnostic
// and last_completed_at.
func TestGetDatasetCompilationStatus_FullOutput(t *testing.T) {
	db := setupCompilationStatusTestDB(t)
	insertCompilationOwnerKB(t, "kb-full", "user-1")

	// A running row with 2 inflight + 1 backlog entries and a retained error
	// diagnostic (error is NOT a fifth state: state stays running).
	lastDone := time.Now().Add(-time.Hour).UTC()
	row := entity.KnowledgeCompileDataset{
		DatasetID:       "kb-full",
		TenantID:        "user-1",
		State:           entity.DatasetStateRunning,
		ErrorMsg:        "merge failed: boom",
		Progress:        0.72,
		CurrentPhase:    "routing_pages",
		ProgressMsg:     "Routing 55 Wiki pages",
		LastCompletedAt: &lastDone,
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("insert scheduling row: %v", err)
	}
	if err := db.Create(&entity.KnowledgeCompileBacklog{KnowledgeCompileEntry: entity.KnowledgeCompileEntry{
		DatasetID: row.DatasetID, DocID: "d3", QueuePosition: 1, EventType: "completed", TaskTypes: []string{"Timeline"},
	}}).Error; err != nil {
		t.Fatalf("insert backlog: %v", err)
	}
	if err := db.Create(&[]entity.KnowledgeCompileInflight{
		{KnowledgeCompileEntry: entity.KnowledgeCompileEntry{DatasetID: row.DatasetID, DocID: "d1", QueuePosition: 1, EventType: "completed", TaskTypes: []string{"Graph"}}},
		{KnowledgeCompileEntry: entity.KnowledgeCompileEntry{DatasetID: row.DatasetID, DocID: "d2", QueuePosition: 2, EventType: "completed", TaskTypes: []string{"Graph"}}},
	}).Error; err != nil {
		t.Fatalf("insert inflight: %v", err)
	}

	st, code, err := testCompilationStatusService().GetDatasetCompilationStatus(
		withDatasetTestContext(t.Context(), "user-1", "user-1"), "user-1", "kb-full", "graph")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("code=%d want %d", code, common.CodeSuccess)
	}
	if st.State != entity.DatasetStateRunning {
		t.Fatalf("state=%q want running", st.State)
	}
	if st.Kind != "graph" {
		t.Fatalf("kind=%q want graph", st.Kind)
	}
	if st.Inflight != 2 || st.Backlog != 0 {
		t.Fatalf("want inflight=2 backlog=0, got inflight=%d backlog=%d", st.Inflight, st.Backlog)
	}
	if st.Error != "merge failed: boom" {
		t.Fatalf("error=%q want %q", st.Error, "merge failed: boom")
	}
	if st.Progress != 0.72 || st.CurrentPhase != "routing_pages" || st.ProgressMsg != "Routing 55 Wiki pages" {
		t.Fatalf("progress fields not mapped: progress=%v phase=%q msg=%q", st.Progress, st.CurrentPhase, st.ProgressMsg)
	}
	if st.LastCompletedAt == nil || !st.LastCompletedAt.Equal(lastDone) {
		t.Fatalf("last_completed_at=%v want %v", st.LastCompletedAt, lastDone)
	}
}

// TestGetDatasetCompilationStatus_Unauthorized verifies a user who does not
// own the dataset is rejected before reading the scheduling row.
func TestGetDatasetCompilationStatus_Unauthorized(t *testing.T) {
	setupCompilationStatusTestDB(t)
	// Owner is user-1; a different user-2 must be denied.
	insertCompilationOwnerKB(t, "kb-other", "user-1")
	if err := dao.DB.Create(&entity.KnowledgeCompileDataset{
		DatasetID: "kb-other",
		TenantID:  "user-1",
		State:     entity.DatasetStatePending,
	}).Error; err != nil {
		t.Fatalf("insert scheduling row: %v", err)
	}

	st, code, err := testCompilationStatusService().GetDatasetCompilationStatus(
		withDatasetTestContext(t.Context(), "user-2", "user-1"), "user-2", "kb-other", "graph")
	if err == nil {
		t.Fatalf("expected authorization error, got nil (status=%+v)", st)
	}
	if code != common.CodeForbidden {
		t.Fatalf("code=%d want %d", code, common.CodeForbidden)
	}
}

// TestGetDatasetCompilationStatus_EmptyID validates the required-field guard.
func TestGetDatasetCompilationStatus_EmptyID(t *testing.T) {
	setupCompilationStatusTestDB(t)
	_, code, err := testCompilationStatusService().GetDatasetCompilationStatus(
		t.Context(), "user-1", "", "graph")
	if err == nil {
		t.Fatal("expected error for empty dataset_id")
	}
	if code != common.CodeDataError {
		t.Fatalf("code=%d want %d", code, common.CodeDataError)
	}
}
