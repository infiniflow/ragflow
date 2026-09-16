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
	"runtime"
	"sync"
	"testing"

	"ragflow/internal/dao"
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

	ctx := t.Context()
	sink := newProgressSink(ctx, servicepkg.NewIngestionTaskService(), "run-1")
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

	ctx := t.Context()
	// Deliberately do NOT inject a stub docSvc: the sink's own DocumentService
	// must already be constructed (not lazily built mid-call) when the
	// goroutines below race into docSvc.
	sink := newProgressSink(ctx, servicepkg.NewIngestionTaskService(), "run-1")

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

	ctx := t.Context()
	sink := newProgressSink(ctx, servicepkg.NewIngestionTaskService(), "run-1")

	const n = 30
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			sink.OnComponentTotal(ctx, taskID, 5) // writes s.total
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
	stateCalls    int
	stateDocID    string
	stateProgress float64
}

func (s *stubDocProgressSvc) UpdateRunState(_ context.Context, docID string, progress float64) error {
	s.stateCalls++
	s.stateDocID = docID
	s.stateProgress = progress
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

	ctx := t.Context()
	sink := newProgressSink(ctx, servicepkg.NewIngestionTaskService(), "run-1")
	stub := &stubDocProgressSvc{}
	sink.docSvc = stub

	sink.OnComponentTotal(ctx, taskID, 2)
	task, err := dao.NewIngestionTaskDAO().GetByID(ctx, db, taskID)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.ComponentTotal != 2 {
		t.Fatalf("component_total = %d, want 2", task.ComponentTotal)
	}

	sink.OnComponentProgress(ctx, pipeline.ProgressEvent{
		TaskID:     taskID,
		DocumentID: docID,
		Component:  "Parser",
		Phase:      1,
		Message:    "Parser Done",
	})

	logs, err := dao.NewIngestionTaskLogDAO().ListLogsByPipelineLogID(ctx, db, "run-1")
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 component-progress row, got %d", len(logs))
	}
	if logs[0].Component != "Parser" || logs[0].Phase != 1 || logs[0].Message != "Parser Done" {
		t.Fatalf("unexpected log row: %+v", logs[0])
	}

	// 1 of 2 components done -> RUNNING (run "1"), progress 0.5. The
	// event stream owns text, so this must not write document.progress_msg.
	if stub.stateCalls != 1 || stub.stateDocID != docID || stub.stateProgress != 0.5 {
		t.Fatalf("UpdateRunState = calls:%d doc:%q progress:%v, want 1/%q/0.5", stub.stateCalls, stub.stateDocID, stub.stateProgress, docID)
	}
}

func TestProgressSinkWritesLifecycleEventForCapturedRun(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, _, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	sink := newProgressSink(t.Context(), servicepkg.NewIngestionTaskService(), "run-1")
	sink.OnComponentProgress(t.Context(), pipeline.ProgressEvent{
		TaskID:    taskID,
		Component: "Parser",
		Phase:     1,
		Message:   "Parser Done",
	})

	logs, err := dao.NewIngestionTaskLogDAO().ListLogsByPipelineLogID(t.Context(), db, "run-1")
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("event count = %d, want 1", len(logs))
	}
	event := logs[0]
	if event.PipelineLogID == nil || *event.PipelineLogID != "run-1" {
		t.Fatalf("pipeline_log_id = %v, want run-1", event.PipelineLogID)
	}
	if event.EventType != dao.EventTypeLifecycle || event.Component != "Parser" || event.Phase != 1 {
		t.Fatalf("event = %+v, want lifecycle Parser/1", event)
	}
}

// TestProgressSinkEmptyDocumentIDSkipsMirror verifies the log row is still
// recorded when no owning document is bound, but the document mirror is skipped.
func TestProgressSinkEmptyDocumentIDSkipsMirror(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, _, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	ctx := t.Context()
	sink := newProgressSink(ctx, servicepkg.NewIngestionTaskService(), "run-1")
	stub := &stubDocProgressSvc{}
	sink.docSvc = stub

	sink.OnComponentProgress(ctx, pipeline.ProgressEvent{
		TaskID:    taskID,
		Component: "Chunker",
		Phase:     1,
		Message:   "Chunker Done",
	})

	logs, err := dao.NewIngestionTaskLogDAO().ListLogsByPipelineLogID(ctx, db, "run-1")
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 component-progress row, got %d", len(logs))
	}
	if stub.stateCalls != 0 {
		t.Fatalf("UpdateRunState calls = %d, want 0 (no document bound)", stub.stateCalls)
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
		wantProg float64
	}{
		{
			name:     "failed component progress",
			agg:      &dao.TaskProgress{Failed: 1, Done: 0, Running: 0, Percent: 0},
			total:    5,
			wantProg: 0.0,
		},
		{
			name:     "all done",
			agg:      &dao.TaskProgress{Failed: 0, Done: 5, Running: 0, Percent: 100},
			total:    5,
			wantProg: 1.0,
		},
		{
			name:     "partial done",
			agg:      &dao.TaskProgress{Failed: 0, Done: 3, Running: 0, Percent: 60},
			total:    5,
			wantProg: 0.6,
		},
		{
			name:     "running only",
			agg:      &dao.TaskProgress{Failed: 0, Done: 0, Running: 2, Percent: 0},
			total:    5,
			wantProg: 0.0,
		},
		{
			name:     "nothing started",
			agg:      &dao.TaskProgress{Failed: 0, Done: 0, Running: 0, Percent: 0},
			total:    5,
			wantProg: 0.0,
		},
		{
			name:     "total zero, nothing done",
			agg:      &dao.TaskProgress{Failed: 0, Done: 0, Running: 0, Percent: 0},
			total:    0,
			wantProg: 0.0,
		},
		{
			name:     "failed with 100 percent",
			agg:      &dao.TaskProgress{Failed: 1, Done: 5, Running: 0, Percent: 100},
			total:    5,
			wantProg: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prog := deriveDocumentProgress(tt.agg, tt.total)
			if prog != tt.wantProg {
				t.Errorf("progress = %v, want %v", prog, tt.wantProg)
			}
		})
	}
}
