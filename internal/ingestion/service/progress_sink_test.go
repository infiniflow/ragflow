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
	"runtime"
	"sync"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/pipeline"
	"ragflow/internal/ingestion/testutil"
	servicepkg "ragflow/internal/service"
	"ragflow/internal/service/document"
)

// TestProgressSink_CanConstructDocumentServiceWithoutServerConfig ensures the
// sink's DocumentService dependency can be built in a headless/test environment
// where server config is not initialized. NewDocumentService historically read
// server.GetConfig().DocEngine.Type, which nil-dereferenced without config; the
// sink must not pull the process-wide config just to mirror run progress.
func TestProgressSink_CanConstructDocumentServiceWithoutServerConfig(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	// No server config is initialized in the test env; this must not panic.
	svc := document.NewDocumentService()
	if svc == nil {
		t.Fatal("expected non-nil DocumentService")
	}
}

// TestProgressSink_EagerlyConstructsDocumentService ensures the sink builds its
// DocumentService at construction time rather than lazily on the first progress
// event. Lazy construction is a data race under eino's parallel-branch progress
// callbacks (see TestProgressSink_OnComponentProgress_NoDataRace); eager
// construction makes docSvc immutable after newProgressSink returns.
func TestProgressSink_EagerlyConstructsDocumentService(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	sink := newProgressSink(servicepkg.NewIngestionTaskService())
	if sink.docSvc == nil {
		t.Fatal("expected sink to eagerly construct its DocumentService, got nil (lazy)")
	}
}

// TestProgressSink_DocService_NoDataRace guards against regressing to lazy
// DocumentService construction. eino's compose graph runs parallel branches
// concurrently (compose/chain_parallel.go, branch.go), so the progress callback
// can fire from multiple goroutines; docSvc must be a pre-built, immutable
// DocumentService, not lazily check-then-act on s.docSvc.
//
// The race is hit directly on docSvc rather than through OnComponentProgress
// because the latter serializes on the single test-DB connection before
// reaching docSvc, which masks the race.
func TestProgressSink_DocService_NoDataRace(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	// Deliberately do NOT inject a stub docSvc: the sink's own DocumentService
	// must already be constructed (not lazily built mid-call) when the
	// goroutines below race into docSvc.
	sink := newProgressSink(servicepkg.NewIngestionTaskService())

	const n = 30
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_ = sink.docSvc
		}()
	}
	close(start)
	wg.Wait()
}

// TestProgressSink_Total_NoDataRace guards the total denominator against being
// a non-atomic shared field. OnComponentTotal (writer, Run goroutine) and
// OnComponentProgress (reader, concurrent eino branches) share total; a plain
// int is a data race per the Go memory model. The read is hit directly on the
// field rather than through OnComponentProgress because the latter serializes
// on the single test-DB connection before reaching the read, masking the race.
func TestProgressSink_Total_NoDataRace(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, _, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	sink := newProgressSink(servicepkg.NewIngestionTaskService())

	const n = 30
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			sink.OnComponentTotal(taskID, 5) // writes s.total
		}()
	}
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			v := sink.total.Load() // reads s.total atomically
			runtime.KeepAlive(v)
		}()
	}
	close(start)
	wg.Wait()
}

type stubDocProgressSvc struct {
	gotDocID    string
	gotProgress float64
	gotRun      string
	gotMsg      string
	calls       int
}

func (s *stubDocProgressSvc) UpdateRunProgress(docID string, progress float64, run, progressMsg string) error {
	s.calls++
	s.gotDocID = docID
	s.gotProgress = progress
	s.gotRun = run
	s.gotMsg = progressMsg
	return nil
}

// TestProgressSinkPersistsViaService verifies the sink is the single writer of
// ingestion_task.component_total, ingestion_task_log, and document run-progress
// - all through the service layer, not the DAO.
func TestProgressSinkPersistsViaService(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, docID, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	sink := newProgressSink(servicepkg.NewIngestionTaskService())
	stub := &stubDocProgressSvc{}
	sink.docSvc = stub

	sink.OnComponentTotal(taskID, 2)
	task, err := dao.NewIngestionTaskDAO().GetByID(taskID)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.ComponentTotal != 2 {
		t.Fatalf("component_total = %d, want 2", task.ComponentTotal)
	}

	sink.OnComponentProgress(pipeline.ProgressEvent{
		TaskID:     taskID,
		DocumentID: docID,
		Component:  "Parser",
		Phase:      1,
		Message:    "Parser Done",
	})

	logs, err := dao.NewIngestionTaskLogDAO().ListLogsByTaskID(taskID)
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 component-progress row, got %d", len(logs))
	}
	if logs[0].Component != "Parser" || logs[0].Phase != 1 || logs[0].Message != "Parser Done" {
		t.Fatalf("unexpected log row: %+v", logs[0])
	}

	// 1 of 2 components done -> RUNNING (run "1"), progress 0.5.
	if stub.calls != 1 {
		t.Fatalf("UpdateRunProgress calls = %d, want 1", stub.calls)
	}
	if stub.gotDocID != docID {
		t.Fatalf("docID = %q, want %q", stub.gotDocID, docID)
	}
	if stub.gotProgress != 0.5 {
		t.Fatalf("progress = %v, want 0.5", stub.gotProgress)
	}
	if stub.gotRun != "1" {
		t.Fatalf("run = %q, want 1 (RUNNING)", stub.gotRun)
	}
	if stub.gotMsg != "Parser Done" {
		t.Fatalf("progress_msg = %q, want Parser Done", stub.gotMsg)
	}
}

// TestProgressSinkEmptyDocumentIDSkipsMirror verifies the log row is still
// recorded when no owning document is bound, but the document mirror is skipped.
func TestProgressSinkEmptyDocumentIDSkipsMirror(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, _, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	sink := newProgressSink(servicepkg.NewIngestionTaskService())
	stub := &stubDocProgressSvc{}
	sink.docSvc = stub

	sink.OnComponentProgress(pipeline.ProgressEvent{
		TaskID:    taskID,
		Component: "Chunker",
		Phase:     1,
		Message:   "Chunker Done",
	})

	logs, err := dao.NewIngestionTaskLogDAO().ListLogsByTaskID(taskID)
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 component-progress row, got %d", len(logs))
	}
	if stub.calls != 0 {
		t.Fatalf("UpdateRunProgress calls = %d, want 0 (no document bound)", stub.calls)
	}
}

// TestDeriveDocumentProgress exercises every branch of the run-label derivation
// logic. The function is called from OnComponentProgress with a non-nil agg
// (guarded by the caller), so the nil case is documented as a known panic.
func TestDeriveDocumentProgress(t *testing.T) {
	tests := []struct {
		name     string
		agg      *dao.TaskProgress
		total    int
		wantRun  string
		wantProg float64
	}{
		{
			name:     "failed component → fail",
			agg:      &dao.TaskProgress{Failed: 1, Done: 0, Running: 0, Percent: 0},
			total:    5,
			wantRun:  string(entity.TaskStatusFail),
			wantProg: 0.0,
		},
		{
			name:     "all done → done",
			agg:      &dao.TaskProgress{Failed: 0, Done: 5, Running: 0, Percent: 100},
			total:    5,
			wantRun:  string(entity.TaskStatusDone),
			wantProg: 1.0,
		},
		{
			name:     "partial done → running",
			agg:      &dao.TaskProgress{Failed: 0, Done: 3, Running: 0, Percent: 60},
			total:    5,
			wantRun:  string(entity.TaskStatusRunning),
			wantProg: 0.6,
		},
		{
			name:     "running only → running",
			agg:      &dao.TaskProgress{Failed: 0, Done: 0, Running: 2, Percent: 0},
			total:    5,
			wantRun:  string(entity.TaskStatusRunning),
			wantProg: 0.0,
		},
		{
			name:     "nothing started → unstart",
			agg:      &dao.TaskProgress{Failed: 0, Done: 0, Running: 0, Percent: 0},
			total:    5,
			wantRun:  string(entity.TaskStatusUnstart),
			wantProg: 0.0,
		},
		{
			name:     "total zero, nothing done → done (0==0)",
			agg:      &dao.TaskProgress{Failed: 0, Done: 0, Running: 0, Percent: 0},
			total:    0,
			wantRun:  string(entity.TaskStatusDone),
			wantProg: 0.0,
		},
		{
			name:     "failed overrides done=total",
			agg:      &dao.TaskProgress{Failed: 1, Done: 5, Running: 0, Percent: 100},
			total:    5,
			wantRun:  string(entity.TaskStatusFail),
			wantProg: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prog, run := deriveDocumentProgress(tt.agg, tt.total)
			if prog != tt.wantProg {
				t.Errorf("progress = %v, want %v", prog, tt.wantProg)
			}
			if run != tt.wantRun {
				t.Errorf("run = %q, want %q", run, tt.wantRun)
			}
		})
	}
}
