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

package observability

import (
	"context"
	"testing"
	"time"

	"ragflow/internal/entity"
)

// TestIsBeginRelevantUsesStatus is the adversarial fixture for the scope
// repurpose: with event_type now carrying scope ("task"), claim time must be
// attached based on the task status, not the scope string. The old keying on
// event_type ("task") returns false for every status here.
func TestIsBeginRelevantUsesStatus(t *testing.T) {
	cases := []struct {
		status string
		want   bool
	}{
		{"RUNNING", true},
		{"COMPLETED", true},
		{"FAILED", true},
		{"STOPPED", true},
		{"CREATED", false},
		{"SCHEDULED", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := isBeginRelevant(tc.status); got != tc.want {
			t.Errorf("isBeginRelevant(%q) = %v, want %v", tc.status, got, tc.want)
		}
	}
}

// TestBuildIngestLogAttachesClaimByStatus is the end-to-end guard: a task-scope
// running event with a valid claim time must carry BeginTime so parse duration
// can be derived. Keying on the scope string "task" would drop it.
func TestBuildIngestLogAttachesClaimByStatus(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	claim := now.Add(2 * time.Minute)
	task := &entity.IngestionTask{ID: "t", DocumentID: "d", DatasetID: "kb", UserID: "u"}
	doc := &entity.Document{ProcessBeginAt: &claim}
	run := &entity.PipelineOperationLog{
		CreateDate:      &now,
		ProcessBeginAt:  &claim,
		ProcessDuration: 600,
	}
	log := BuildIngestLog(context.Background(), task, doc, run, "task", "RUNNING", "")
	if log.BeginTime.IsZero() {
		t.Error("BeginTime must be attached for a RUNNING task-scope event")
	}
	if log.ParseDurationS <= 0 {
		t.Errorf("ParseDurationS = %v, want > 0", log.ParseDurationS)
	}
}

// TestBuildIngestLogTerminalEndTimeFallbackByStatus is the adversarial fixture
// for review finding F1: with event_type now fixed to "task", the terminal
// end-time fallback must key on status, not the old lowercase event type. A run
// with no ProcessDuration but a claim time must still get EndTime == EventTime
// for a terminal status, so parse_duration_s is not silently zero.
func TestBuildIngestLogTerminalEndTimeFallbackByStatus(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	claim := now.Add(2 * time.Minute)
	task := &entity.IngestionTask{ID: "t", DocumentID: "d", DatasetID: "kb", UserID: "u"}
	doc := &entity.Document{ProcessBeginAt: &claim}
	// run has CreateDate + claim time but NO ProcessDuration -> EndTime unset.
	run := &entity.PipelineOperationLog{
		CreateDate:     &now,
		ProcessBeginAt: &claim,
		// ProcessDuration deliberately 0
	}
	for _, status := range []string{"FAILED", "COMPLETED", "STOPPED"} {
		log := BuildIngestLog(context.Background(), task, doc, run, "task", status, "")
		if log.EndTime.IsZero() {
			t.Errorf("status %s: EndTime must fall back to EventTime when no ProcessDuration", status)
		}
		if !log.EndTime.Equal(log.EventTime) {
			t.Errorf("status %s: EndTime = %v, want EventTime %v", status, log.EndTime, log.EventTime)
		}
	}
	// adversarial: non-terminal status must NOT get the fallback
	running := BuildIngestLog(context.Background(), task, doc, run, "task", "RUNNING", "")
	if !running.EndTime.IsZero() {
		t.Errorf("RUNNING: EndTime = %v, want zero (no terminal fallback)", running.EndTime)
	}
}
