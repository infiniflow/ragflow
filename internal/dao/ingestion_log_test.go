//
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
//

package dao

import (
	"testing"

	"ragflow/internal/entity"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestListEventsPageByPipelineLogIDUsesStableKeysets(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.IngestionTaskLog{}); err != nil {
		t.Fatalf("migrate event table: %v", err)
	}
	for i := 1; i <= 5; i++ {
		if err := db.Exec(`INSERT INTO ingestion_task_log
			(task_id, pipeline_log_id, event_type, checkpoint, component, phase, message)
			VALUES (?, ?, ?, '{}', '', 0, ?)`, "task-1", "run-1", EventTypeMessage, "event").Error; err != nil {
			t.Fatalf("insert event %d: %v", i, err)
		}
	}

	page, err := NewIngestionTaskLogDAO().ListEventsPageByPipelineLogID(t.Context(), db, "run-1", 2, nil, nil)
	if err != nil {
		t.Fatalf("list first page: %v", err)
	}
	assertEventIDs(t, page.Events, 4, 5)
	if !page.HasMoreBefore || page.HasMoreAfter {
		t.Fatalf("first page flags = before:%t after:%t, want true/false", page.HasMoreBefore, page.HasMoreAfter)
	}

	afterID := 2
	page, err = NewIngestionTaskLogDAO().ListEventsPageByPipelineLogID(t.Context(), db, "run-1", 2, &afterID, nil)
	if err != nil {
		t.Fatalf("list after page: %v", err)
	}
	assertEventIDs(t, page.Events, 3, 4)
	if !page.HasMoreBefore || !page.HasMoreAfter {
		t.Fatalf("after page flags = before:%t after:%t, want true/true", page.HasMoreBefore, page.HasMoreAfter)
	}

	beforeID := 4
	page, err = NewIngestionTaskLogDAO().ListEventsPageByPipelineLogID(t.Context(), db, "run-1", 2, nil, &beforeID)
	if err != nil {
		t.Fatalf("list before page: %v", err)
	}
	assertEventIDs(t, page.Events, 2, 3)
	if !page.HasMoreBefore || !page.HasMoreAfter {
		t.Fatalf("before page flags = before:%t after:%t, want true/true", page.HasMoreBefore, page.HasMoreAfter)
	}

	deletedCursor := 99
	page, err = NewIngestionTaskLogDAO().ListEventsPageByPipelineLogID(t.Context(), db, "run-1", 2, &deletedCursor, nil)
	if err != nil {
		t.Fatalf("list with missing cursor: %v", err)
	}
	if len(page.Events) != 0 || !page.HasMoreBefore || page.HasMoreAfter {
		t.Fatalf("missing-cursor page = %+v, want empty with only older history", page)
	}
}

func assertEventIDs(t *testing.T, events []*entity.IngestionTaskLog, want ...int) {
	t.Helper()
	if len(events) != len(want) {
		t.Fatalf("event count = %d, want %d (%v)", len(events), len(want), want)
	}
	for i, event := range events {
		if event.ID != want[i] {
			t.Fatalf("event id at %d = %d, want %d", i, event.ID, want[i])
		}
	}
}

func TestAggregateProgressByPipelineLogIDIgnoresMessagesAndOtherRuns(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.IngestionTaskLog{}); err != nil {
		t.Fatalf("migrate event table: %v", err)
	}
	runOne := "run-1"
	runTwo := "run-2"
	events := []struct {
		pipelineLogID string
		eventType     int
		component     string
		phase         int
		message       string
	}{
		{runOne, EventTypeLifecycle, "Parser", 1, ""},
		{runOne, EventTypeMessage, "Parser", 0, "parser detail"},
		{runOne, EventTypeLifecycle, "Chunker", 0, ""},
		{runTwo, EventTypeLifecycle, "Parser", 1, ""},
	}
	for _, event := range events {
		if err := db.Exec(`INSERT INTO ingestion_task_log
			(task_id, pipeline_log_id, event_type, checkpoint, component, phase, message)
			VALUES (?, ?, ?, '{}', ?, ?, ?)`, "task-1", event.pipelineLogID, event.eventType, event.component, event.phase, event.message).Error; err != nil {
			t.Fatalf("create event: %v", err)
		}
	}

	progress, err := NewIngestionTaskLogDAO().AggregateProgressByPipelineLogID(t.Context(), db, runOne, 2)
	if err != nil {
		t.Fatalf("aggregate progress: %v", err)
	}
	if progress.Done != 1 || progress.Running != 1 || progress.Failed != 0 || progress.Percent != 50 {
		t.Fatalf("progress = %+v, want Done=1 Running=1 Failed=0 Percent=50", progress)
	}
}
