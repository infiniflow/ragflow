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

package service

import (
	"context"
	"testing"
	"time"

	"ragflow/internal/entity"
	"ragflow/internal/ingestion"
	"ragflow/internal/progressstore"
)

type recordedBegin struct {
	id, docID, datasetID, attemptID string
	startedAt                       time.Time
}

type fakeProgressBeginner struct{ calls []recordedBegin }

func (f *fakeProgressBeginner) BeginAttempt(_ context.Context, id, docID, datasetID, attemptID string, startedAt time.Time) bool {
	f.calls = append(f.calls, recordedBegin{id, docID, datasetID, attemptID, startedAt})
	return true
}

func TestBeginProgressAttemptUsesContextToken(t *testing.T) {
	logID := "run1"
	e := &Ingestor{deps: &ingestion.Deps{}}
	task := &entity.IngestionTask{DocumentID: "doc1", DatasetID: "ds1", PipelineLogID: &logID}
	beginner := &fakeProgressBeginner{}

	e.beginProgressAttempt(progressstore.WithAttempt(context.Background(), "attempt-7"), task, beginner)

	if len(beginner.calls) != 1 {
		t.Fatalf("BeginAttempt calls = %d, want 1", len(beginner.calls))
	}
	got := beginner.calls[0]
	if got.id != "run1" || got.docID != "doc1" || got.datasetID != "ds1" || got.attemptID != "attempt-7" {
		t.Fatalf("begin call = %+v, want run1/doc1/ds1/attempt-7", got)
	}
	if got.startedAt.IsZero() {
		t.Fatal("attempt start time must be stamped")
	}
}

func TestBeginProgressAttemptSkipsWithoutTokenOrRun(t *testing.T) {
	logID := "run1"
	e := &Ingestor{deps: &ingestion.Deps{}}
	beginner := &fakeProgressBeginner{}

	// No attempt token means this caller predates attempt fencing.
	e.beginProgressAttempt(context.Background(), &entity.IngestionTask{DocumentID: "doc1", DatasetID: "ds1", PipelineLogID: &logID}, beginner)
	// No run identity means there is no KV key to open.
	e.beginProgressAttempt(progressstore.WithAttempt(context.Background(), "attempt-7"), &entity.IngestionTask{DocumentID: "doc1", DatasetID: "ds1"}, beginner)

	if len(beginner.calls) != 0 {
		t.Fatalf("BeginAttempt calls = %+v, want none", beginner.calls)
	}
}

func TestPhaseLabelMapping(t *testing.T) {
	cases := []struct {
		phase uint8
		want  string
	}{
		{0, "running"},
		{1, "done"},
		{2, "failed"},
		{255, ""}, // out-of-range must not corrupt the stored phase
	}
	for _, tc := range cases {
		if got := phaseLabel(tc.phase); got != tc.want {
			t.Errorf("phaseLabel(%d) = %q, want %q", tc.phase, got, tc.want)
		}
	}
}

func TestProgressTerminalStatusMapping(t *testing.T) {
	cases := []struct {
		status string
		want   string
	}{
		{string(entity.TaskStatusDone), progressstore.StatusCompleted},
		{string(entity.TaskStatusFail), progressstore.StatusFailed},
		{string(entity.TaskStatusCancel), progressstore.StatusStopped},
		{"UNKNOWN", ""}, // non-terminal must not flip the snapshot
	}
	for _, tc := range cases {
		if got := progressTerminalStatus(tc.status); got != tc.want {
			t.Errorf("progressTerminalStatus(%q) = %q, want %q", tc.status, got, tc.want)
		}
	}
}
