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
	"database/sql"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"ragflow/internal/common"
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
// callbacks (see TestProgressSink_DocService_NoDataRace); eager construction
// makes docSvc immutable after newProgressSink returns.
func TestProgressSink_EagerlyConstructsDocumentService(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	ctx := t.Context()
	sink := newProgressSink(ctx, servicepkg.NewIngestionTaskService(), "run-1")
	defer sink.Close()
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
	defer sink.Close()

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

// TestProgressSink_Total_NoDataRace guards the shared run-progress state
// against unsynchronized access. OnComponentTotal (writer, Run goroutine) and
// the flusher/Percent readers (concurrent eino branches, flusher goroutine)
// share the tracker; its mutex must make concurrent SetTotal/Percent safe.
func TestProgressSink_Total_NoDataRace(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, _, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	ctx := t.Context()
	sink := newProgressSink(ctx, servicepkg.NewIngestionTaskService(), "run-1")
	defer sink.Close()

	const n = 30
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			sink.OnComponentTotal(ctx, taskID, 5) // writes tracker total
		}()
	}
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			v := sink.progress.Percent() // reads tracker under mutex
			runtime.KeepAlive(v)
		}()
	}
	close(start)
	wg.Wait()
}

type stubDocProgressSvc struct {
	mu            sync.Mutex
	stateCalls    int
	stateDocID    string
	stateProgress float64
}

func (s *stubDocProgressSvc) UpdateRunState(_ context.Context, docID string, progress float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stateCalls++
	s.stateDocID = docID
	s.stateProgress = progress
	return nil
}

func (s *stubDocProgressSvc) snapshot() (calls int, docID string, progress float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stateCalls, s.stateDocID, s.stateProgress
}

// ctxAwareStubDocProgressSvc fails like a real DB call when the flush context
// is cancelled, so tests can prove the Close final flush (detached context)
// still succeeds after the run context is cancelled, while ticker flushes
// bound to the run context would fail.
type ctxAwareStubDocProgressSvc struct {
	stubDocProgressSvc
}

func (s *ctxAwareStubDocProgressSvc) UpdateRunState(ctx context.Context, docID string, progress float64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.stubDocProgressSvc.UpdateRunState(ctx, docID, progress)
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
	// Close joins the flusher and performs the final forced flush, making the
	// mirrored state deterministic regardless of ticker timing.
	sink.Close()

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

	// 1 of 2 components done -> progress 0.5 mirrored from the in-memory
	// tracker. The event stream owns text, so this must not write
	// document.progress_msg (UpdateRunState only touches progress/duration).
	calls, gotDocID, gotProgress := stub.snapshot()
	if calls < 1 || gotDocID != docID || gotProgress != 0.5 {
		t.Fatalf("UpdateRunState = calls:%d doc:%q progress:%v, want >=1/%q/0.5", calls, gotDocID, gotProgress, docID)
	}
}

func TestProgressSinkWritesLifecycleEventForCapturedRun(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, _, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	sink := newProgressSink(t.Context(), servicepkg.NewIngestionTaskService(), "run-1")
	defer sink.Close()
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
// recorded when no owning document is bound, but the document mirror is
// skipped - including the Close final flush.
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
	sink.Close()

	logs, err := dao.NewIngestionTaskLogDAO().ListLogsByPipelineLogID(ctx, db, "run-1")
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 component-progress row, got %d", len(logs))
	}
	if calls, _, _ := stub.snapshot(); calls != 0 {
		t.Fatalf("UpdateRunState calls = %d, want 0 (no document bound)", calls)
	}
}

// TestProgressSinkFractionsCoalesceIntoCloseFlush verifies high-frequency
// fraction reports never write per event: they only mutate the tracker, and
// the ticker plus the final flush on Close are the only writers.
func TestProgressSinkFractionsCoalesceIntoCloseFlush(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, docID, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	ctx := t.Context()
	sink := newProgressSink(ctx, servicepkg.NewIngestionTaskService(), "run-1")
	stub := &stubDocProgressSvc{}
	sink.docSvc = stub

	sink.OnComponentTotal(ctx, taskID, 2)
	// Bind the document without a lifecycle exit (enter phase, no MarkDone).
	sink.OnComponentProgress(ctx, pipeline.ProgressEvent{
		TaskID:     taskID,
		DocumentID: docID,
		Component:  "Parser",
		Phase:      0,
		Message:    "Parser Started",
	})
	for i := 1; i <= 100; i++ {
		sink.OnComponentFraction(ctx, "Parser", float64(i)/100)
	}
	sink.Close()

	// Ticker (at most one write, since a single value change is what the
	// ticker would see) + Close final (1) = at most 2 writes for 100 reports.
	calls, gotDocID, gotProgress := stub.snapshot()
	if calls > 2 {
		t.Fatalf("UpdateRunState calls = %d, want <= 2 (fractions must coalesce)", calls)
	}
	if gotDocID != docID {
		t.Fatalf("docID = %q, want %q", gotDocID, docID)
	}
	if gotProgress != 0.5 {
		t.Fatalf("progress = %v, want 0.5 (frac 1.0 of 2 components)", gotProgress)
	}
}

// TestProgressSinkTickerFlushesLifecycleProgressWithoutClose verifies lifecycle
// progress reaches the document row during the run instead of waiting for
// Close: the flusher's ticker mirrors a changed percent within one interval.
func TestProgressSinkTickerFlushesLifecycleProgressWithoutClose(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, docID, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	ctx := t.Context()
	sink := newProgressSink(ctx, servicepkg.NewIngestionTaskService(), "run-1")
	defer sink.Close()
	stub := &stubDocProgressSvc{}
	sink.docSvc = stub

	sink.OnComponentTotal(ctx, taskID, 4)
	sink.OnComponentProgress(ctx, pipeline.ProgressEvent{
		TaskID:     taskID,
		DocumentID: docID,
		Component:  "File",
		Phase:      1,
		Message:    "File Done",
	})

	deadline := time.Now().Add(3 * time.Second)
	for {
		if calls, gotDocID, gotProgress := stub.snapshot(); calls >= 1 {
			if gotDocID != docID || gotProgress != 0.25 {
				t.Fatalf("ticker flush = doc:%q progress:%v, want %q/0.25", gotDocID, gotProgress, docID)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("ticker did not flush within 3s")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestProgressSinkCloseFlushSurvivesCancelledRunContext verifies the final
// flush detaches from the run context: a stopped/cancelled run still mirrors
// its last in-memory percent instead of losing it to ctx.Err().
func TestProgressSinkCloseFlushSurvivesCancelledRunContext(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, docID, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	baseCtx, cancel := context.WithCancel(t.Context())
	cancel()
	sink := newProgressSink(baseCtx, servicepkg.NewIngestionTaskService(), "run-1")
	stub := &ctxAwareStubDocProgressSvc{}
	sink.docSvc = stub

	// Events run on a live context (the pipeline wraps sink calls with
	// progressSinkContext), but the flusher's base context is dead.
	sink.OnComponentTotal(t.Context(), taskID, 2)
	sink.OnComponentProgress(t.Context(), pipeline.ProgressEvent{
		TaskID:     taskID,
		DocumentID: docID,
		Component:  "Parser",
		Phase:      1,
		Message:    "Parser Done",
	})
	sink.Close()

	calls, gotDocID, gotProgress := stub.snapshot()
	if calls != 1 || gotDocID != docID || gotProgress != 0.5 {
		t.Fatalf("final flush = calls:%d doc:%q progress:%v, want 1/%q/0.5 (ticker flushes must fail on cancelled ctx, Close flush must succeed)", calls, gotDocID, gotProgress, docID)
	}
}

// failingDocProgressSvc fails every mirror write, so tests can drive the
// sink's flush-error branch deterministically.
type failingDocProgressSvc struct {
	err      error
	attempts int
}

func (s *failingDocProgressSvc) UpdateRunState(_ context.Context, _ string, _ float64) error {
	s.attempts++
	return s.err
}

// TestProgressSinkFlushCancellationDoesNotWarn pins the log level of the
// periodic-flush error branch. A stop cancels the run context underneath an
// in-flight UPDATE, so the call comes back as a cancellation (GORM joins
// ctx.Err() with sql.ErrTxDone, hence the errors.Is check - the reported error
// is not context.Canceled itself). That abort is the normal stop path and must
// stay at debug; a genuine write failure must still warn.
func TestProgressSinkFlushCancellationDoesNotWarn(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	prevLogger := common.Logger
	defer func() { common.Logger = prevLogger }()
	core, logs := observer.New(zapcore.WarnLevel)
	common.Logger = zap.New(core)

	ctx := t.Context()
	sink := newProgressSink(ctx, servicepkg.NewIngestionTaskService(), "run-1")
	defer sink.Close()
	sink.bindDocument("doc-1")

	// Cancellation, in the joined shape a cancelled DB call produces.
	cancelStub := &failingDocProgressSvc{
		err: errors.Join(context.Canceled, sql.ErrTxDone),
	}
	sink.docSvc = cancelStub
	sink.flush(ctx, true)
	if cancelStub.attempts != 1 {
		t.Fatalf("flush attempts = %d, want 1 (the branch under test must actually run)", cancelStub.attempts)
	}
	if logs.Len() != 0 {
		t.Fatalf("cancelled flush logged %d warn entries: %v", logs.Len(), logs.All())
	}

	// A real failure keeps warning.
	failStub := &failingDocProgressSvc{err: errors.New("db down")}
	sink.docSvc = failStub
	sink.flush(ctx, true)
	if failStub.attempts != 1 {
		t.Fatalf("flush attempts = %d, want 1", failStub.attempts)
	}
	if logs.Len() != 1 {
		t.Fatalf("write failure logged %d warn entries, want 1", logs.Len())
	}
	if entry := logs.All()[0]; entry.Level != zapcore.WarnLevel {
		t.Fatalf("write failure logged at %v, want warn", entry.Level)
	}
}
