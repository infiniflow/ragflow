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
