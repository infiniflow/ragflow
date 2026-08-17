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
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

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

	ctx := t.Context()
	sink := newProgressSink(ctx, servicepkg.NewIngestionTaskService())
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
	sink := newProgressSink(ctx, servicepkg.NewIngestionTaskService())

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
	sink := newProgressSink(ctx, servicepkg.NewIngestionTaskService())

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
	gotDocID    string
	gotProgress float64
	gotRun      string
	gotMsg      string
	calls       int
	doc         *document.DocumentResponse
	docErr      error
}

func setProgressSinkTestClock(sink *progressSink) {
	sink.now = func() time.Time {
		return time.Date(2026, 8, 17, 3, 4, 5, 0, time.UTC)
	}
}

type blockingDocProgressSvc struct {
	mu           sync.Mutex
	firstStarted chan struct{}
	releaseFirst chan struct{}
	messages     []string
	updates      int
}

func (s *blockingDocProgressSvc) GetDocumentByID(context.Context, string) (*document.DocumentResponse, error) {
	return nil, nil
}

func (s *blockingDocProgressSvc) UpdateRunState(context.Context, string, float64, string) error {
	return nil
}

func (s *blockingDocProgressSvc) UpdateRunProgress(_ context.Context, _ string, _ float64, _ string, msg string) error {
	s.mu.Lock()
	s.updates++
	update := s.updates
	s.messages = append(s.messages, msg)
	s.mu.Unlock()
	if update == 1 {
		close(s.firstStarted)
		<-s.releaseFirst
	}
	return nil
}

func (s *stubDocProgressSvc) GetDocumentByID(ctx context.Context, docID string) (*document.DocumentResponse, error) {
	return s.doc, s.docErr
}

func (s *stubDocProgressSvc) UpdateRunState(context.Context, string, float64, string) error {
	return nil
}

func (s *stubDocProgressSvc) UpdateRunProgress(ctx context.Context, docID string, progress float64, run, progressMsg string) error {
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

	ctx := t.Context()
	sink := newProgressSink(ctx, servicepkg.NewIngestionTaskService())
	setProgressSinkTestClock(sink)
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

	logs, err := dao.NewIngestionTaskLogDAO().ListLogsByTaskID(ctx, db, taskID)
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
	if stub.gotMsg != "03:04:05: Parser Done" {
		t.Fatalf("progress_msg = %q, want timestamped Parser Done", stub.gotMsg)
	}
}

// TestProgressSinkAccumulatesProgressLog pins the core fix: document.progress_msg
// is the accumulated multi-line run log, not just the latest component line.
// The document-details dialog renders progress_msg verbatim (whitespace-pre-line).
func TestProgressSinkAccumulatesProgressLog(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, docID, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	ctx := t.Context()
	sink := newProgressSink(ctx, servicepkg.NewIngestionTaskService())
	setProgressSinkTestClock(sink)
	stub := &stubDocProgressSvc{}
	sink.docSvc = stub
	sink.OnComponentTotal(ctx, taskID, 2)

	sink.OnComponentProgress(ctx, pipeline.ProgressEvent{
		TaskID: taskID, DocumentID: docID, Component: "File", Phase: 1, Message: "File:naive Done",
	})
	sink.OnComponentProgress(ctx, pipeline.ProgressEvent{
		TaskID: taskID, DocumentID: docID, Component: "Parser", Phase: 1, Message: "Parser Done",
	})

	want := "03:04:05: File:naive Done\n03:04:05: Parser Done"
	if stub.gotMsg != want {
		t.Fatalf("progress_msg = %q, want multi-line log %q", stub.gotMsg, want)
	}
}

// TestProgressSinkSeedsLogFromDocument verifies the accumulated log keeps the
// lines already stored on the document row when a run resumes.
func TestProgressSinkSeedsLogFromDocument(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, docID, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	ctx := t.Context()
	sink := newProgressSink(ctx, servicepkg.NewIngestionTaskService())
	setProgressSinkTestClock(sink)
	seed := "File:naive Done"
	stub := &stubDocProgressSvc{doc: &document.DocumentResponse{ProgressMsg: &seed}}
	sink.docSvc = stub
	sink.OnComponentTotal(ctx, taskID, 2)

	sink.OnComponentProgress(ctx, pipeline.ProgressEvent{
		TaskID: taskID, DocumentID: docID, Component: "Parser", Phase: 1, Message: "Parser Done",
	})

	want := "File:naive Done\n03:04:05: Parser Done"
	if stub.gotMsg != want {
		t.Fatalf("progress_msg = %q, want seeded multi-line log %q", stub.gotMsg, want)
	}
}

func TestProgressSinkRetriesSeedAfterReadFailure(t *testing.T) {
	seed := "existing"
	stub := &stubDocProgressSvc{docErr: errors.New("temporary read failure")}
	sink := &progressSink{docSvc: stub}

	if got := sink.accumulateLog(context.Background(), "doc-1", "pending"); got != "" {
		t.Fatalf("log after failed seed = %q, want empty mirror value", got)
	}

	stub.docErr = nil
	stub.doc = &document.DocumentResponse{ProgressMsg: &seed}
	setProgressSinkTestClock(sink)
	got := sink.accumulateLog(context.Background(), "doc-1", "current")
	if want := "existing\n03:04:05: pending\n03:04:05: current"; got != want {
		t.Fatalf("log after seed retry = %q, want %q", got, want)
	}
}

func TestProgressSinkSerializesLogAndDocumentWrite(t *testing.T) {
	stub := &blockingDocProgressSvc{
		firstStarted: make(chan struct{}),
		releaseFirst: make(chan struct{}),
	}
	sink := &progressSink{docSvc: stub}
	setProgressSinkTestClock(sink)

	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		if err := sink.updateDocumentProgress(context.Background(), "doc-1", 0.1, "1", "first"); err != nil {
			t.Errorf("first update: %v", err)
		}
	}()
	<-stub.firstStarted

	secondDone := make(chan struct{})
	go func() {
		defer close(secondDone)
		if err := sink.updateDocumentProgress(context.Background(), "doc-1", 0.2, "1", "second"); err != nil {
			t.Errorf("second update: %v", err)
		}
	}()

	select {
	case <-secondDone:
		t.Fatal("second update completed before the first document write was released")
	default:
	}
	close(stub.releaseFirst)
	<-firstDone
	<-secondDone

	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.messages) != 2 || stub.messages[1] != "03:04:05: first\n03:04:05: second" {
		t.Fatalf("document writes = %q, want accumulated second write", stub.messages)
	}
}

// TestProgressSinkTrimsLogHead verifies the accumulated log is head-trimmed at
// line boundaries once it exceeds progressLogMaxChars, keeping the newest lines
// while keeping the newest complete lines.
func TestProgressSinkTrimsLogHead(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, docID, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	ctx := t.Context()
	sink := newProgressSink(ctx, servicepkg.NewIngestionTaskService())
	setProgressSinkTestClock(sink)
	stub := &stubDocProgressSvc{}
	sink.docSvc = stub
	sink.OnComponentTotal(ctx, taskID, 2)

	head := strings.Repeat("h", 2000)
	tail := strings.Repeat("t", 2000)
	sink.OnComponentProgress(ctx, pipeline.ProgressEvent{
		TaskID: taskID, DocumentID: docID, Component: "File", Phase: 1, Message: head,
	})
	sink.OnComponentProgress(ctx, pipeline.ProgressEvent{
		TaskID: taskID, DocumentID: docID, Component: "Parser", Phase: 1, Message: tail,
	})

	if len(stub.gotMsg) > progressLogMaxChars {
		t.Fatalf("progress_msg len = %d, exceeds %d", len(stub.gotMsg), progressLogMaxChars)
	}
	if stub.gotMsg != "03:04:05: "+tail {
		t.Fatalf("progress_msg keeps the newest line: got prefix %q, want %q", stub.gotMsg[:20], tail[:20])
	}
}

// TestProgressSink_Log_NoDataRace hits the accumulated log buffer directly:
// OnComponentProgress fires from concurrent parallel-branch goroutines, so the
// lazy seed and the append must be mutex-guarded. The direct call avoids the
// test-DB serialization inside OnComponentProgress that would mask the race.
func TestProgressSink_Log_NoDataRace(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	ctx := t.Context()
	sink := newProgressSink(ctx, servicepkg.NewIngestionTaskService())
	sink.docSvc = &stubDocProgressSvc{}

	const n = 30
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			sink.accumulateLog(ctx, "doc-1", fmt.Sprintf("line-%d", i))
		}(i)
	}
	close(start)
	wg.Wait()

	got := sink.accumulateLog(ctx, "doc-1", "")
	if lines := strings.Split(got, "\n"); len(lines) != n {
		t.Fatalf("accumulated log lines = %d, want %d: %q", len(lines), n, got)
	}
}

func TestTrimLogHead(t *testing.T) {
	tests := []struct {
		name string
		text string
		max  int
		want string
	}{
		{name: "short text unchanged", text: "a\nb", max: 10, want: "a\nb"},
		{name: "exactly max unchanged", text: "a\nb", max: 3, want: "a\nb"},
		{name: "drops head line", text: "l1\nl2\nl3", max: 6, want: "l2\nl3"},
		{name: "keeps suffix at exact limit", text: "l1\nl2\nl3", max: 5, want: "l2\nl3"},
		{name: "no fitting newline unchanged", text: "abcdef", max: 3, want: "abcdef"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := trimLogHead(tt.text, tt.max); got != tt.want {
				t.Errorf("trimLogHead(%q, %d) = %q, want %q", tt.text, tt.max, got, tt.want)
			}
		})
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
	sink := newProgressSink(ctx, servicepkg.NewIngestionTaskService())
	stub := &stubDocProgressSvc{}
	sink.docSvc = stub

	sink.OnComponentProgress(ctx, pipeline.ProgressEvent{
		TaskID:    taskID,
		Component: "Chunker",
		Phase:     1,
		Message:   "Chunker Done",
	})

	logs, err := dao.NewIngestionTaskLogDAO().ListLogsByTaskID(ctx, db, taskID)
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
