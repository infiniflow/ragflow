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

// memory_extractor_test.go — timezone-semantics tests for the memory
// extractor's timestamp handling (valid_at / invalid_at persistence).

package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/testutil"

	"gorm.io/gorm"
)

// pinMemoryNow fixes the memory wall clock at the given instant for the
// duration of a test, restoring it afterwards.
func pinMemoryNow(t *testing.T, instant time.Time) {
	t.Helper()
	orig := memoryNow
	memoryNow = func() time.Time { return instant }
	t.Cleanup(func() { memoryNow = orig })
}

// pinMemoryTaskLeaseTimings shortens lease maintenance for focused lifecycle
// tests and restores the production values afterwards.
func pinMemoryTaskLeaseTimings(t *testing.T, interval, timeout time.Duration) {
	t.Helper()
	originalInterval := memoryTaskLeaseRenewInterval
	originalTimeout := memoryTaskLeaseRenewTimeout
	memoryTaskLeaseRenewInterval = interval
	memoryTaskLeaseRenewTimeout = timeout
	t.Cleanup(func() {
		memoryTaskLeaseRenewInterval = originalInterval
		memoryTaskLeaseRenewTimeout = originalTimeout
	})
}

// TestFormatMemoryTimeKeepsParsedWallClock: an offset-bearing or
// Z-suffixed ISO timestamp must keep its own wall clock — never be
// converted to UTC the way time.Time.UTC would.
func TestFormatMemoryTimeKeepsParsedWallClock(t *testing.T) {
	fallback := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name  string
		value string
		want  string
	}{
		{name: "offset iso", value: "2026-08-20T10:05:00+08:00", want: "2026-08-20 10:05:00"},
		{name: "offset iso with fractional seconds", value: "2026-08-20T10:05:00.123+08:00", want: "2026-08-20 10:05:00"},
		{name: "zulu iso", value: "2026-08-20T02:05:00Z", want: "2026-08-20 02:05:00"},
		{name: "naive iso", value: "2026-08-20T10:05:00", want: "2026-08-20 10:05:00"},
		{name: "storage layout", value: "2026-08-20 10:05:00", want: "2026-08-20 10:05:00"},
		{name: "date only", value: "2026-08-20", want: "2026-08-20 00:00:00"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := formatMemoryTime(test.value, fallback); got != test.want {
				t.Fatalf("formatMemoryTime(%q) = %q, want %q", test.value, got, test.want)
			}
		})
	}
}

// TestFormatMemoryTimeFallbackKeepsFallbackLocation: empty or unparseable
// input falls back to the supplied time formatted in its own location, so
// callers passing time.Now() persist server-local wall clock.
func TestFormatMemoryTimeFallbackKeepsFallbackLocation(t *testing.T) {
	fallback := time.Date(2026, 8, 20, 10, 5, 0, 0, time.FixedZone("UTC+8", 8*3600))
	for _, value := range []string{"", "   ", "not a timestamp"} {
		if got := formatMemoryTime(value, fallback); got != "2026-08-20 10:05:00" {
			t.Fatalf("formatMemoryTime(%q) = %q, want fallback wall clock %q", value, got, "2026-08-20 10:05:00")
		}
	}
}

// TestBuildExtractedMessageValidAtSemantics: extracted messages persist the
// LLM-supplied wall clock as-is and fall back to the (server-local) now.
func TestBuildExtractedMessageValidAtSemantics(t *testing.T) {
	msg := MemoryMessage{UserID: "u1", AgentID: "a1", SessionID: "s1"}
	now := time.Date(2026, 8, 20, 10, 5, 0, 0, time.Local)

	materialized := materializeMemoryExtraction(t.Context(), []extractedMemory{{
		MessageType: "fact",
		Content:     "likes coffee",
		ValidAt:     "2026-08-20T10:05:00+08:00",
	}}, now)
	withLLMTime := buildExtractedMessage(42, "mem-1", msg, materialized[0])
	if got := withLLMTime["valid_at"]; got != "2026-08-20 10:05:00" {
		t.Fatalf("valid_at = %v, want LLM wall clock %q", got, "2026-08-20 10:05:00")
	}

	materialized = materializeMemoryExtraction(t.Context(), []extractedMemory{{
		MessageType: "fact",
		Content:     "likes coffee",
	}}, now)
	fallbackOnly := buildExtractedMessage(42, "mem-1", msg, materialized[0])
	if got := fallbackOnly["valid_at"]; got != now.Format(memoryTimeLayout) {
		t.Fatalf("valid_at = %v, want fallback %q", got, now.Format(memoryTimeLayout))
	}

	materialized = materializeMemoryExtraction(t.Context(), []extractedMemory{{
		MessageType: "fact",
		Content:     "likes coffee",
		InvalidAt:   "   ",
	}}, now)
	blankInvalidAt := buildExtractedMessage(42, "mem-1", msg, materialized[0])
	if got := blankInvalidAt["invalid_at"]; got != nil {
		t.Fatalf("invalid_at = %#v, want nil for whitespace-only input", got)
	}
}

// TestBuildExtractedMessageUsesStandardDocumentID ensures extracted items
// use the standard memoryID_messageID identity expected by GetMessageContent.
func TestBuildExtractedMessageUsesStandardDocumentID(t *testing.T) {
	msg := MemoryMessage{UserID: "u1", AgentID: "a1", SessionID: "s1"}
	now := time.Date(2026, 8, 20, 10, 5, 0, 0, time.Local)
	item := extractedMemory{
		MessageID:   8,
		MessageType: "fact",
		Content:     "likes coffee",
		ValidAt:     now.Format(memoryTimeLayout),
	}

	extracted := buildExtractedMessage(42, "mem-1", msg, item)
	gotID, ok := extracted["id"].(string)
	if !ok || gotID != "mem-1_8" {
		t.Fatalf("extracted message id = %#v, want %q", extracted["id"], "mem-1_8")
	}
}

// TestMemoryExtractionCheckpointRoundTripPreservesMaterializedFields verifies
// retries reuse the same message id and timestamps.
func TestMemoryExtractionCheckpointRoundTripPreservesMaterializedFields(t *testing.T) {
	want := []extractedMemory{{
		MessageID:   1700000000000000123,
		MessageType: "fact",
		Content:     "likes coffee",
		ValidAt:     "2026-08-20 10:05:00",
	}}
	encoded, err := encodeMemoryExtraction(want)
	if err != nil {
		t.Fatalf("encodeMemoryExtraction: %v", err)
	}
	got, err := decodeMemoryExtraction(encoded)
	if err != nil {
		t.Fatalf("decodeMemoryExtraction: %v", err)
	}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("checkpoint round trip = %+v, want %+v", got, want)
	}
}

// TestHandleSaveToMemoryTaskResumesStoredCheckpoint verifies durable state,
// rather than stale UI progress, determines where execution resumes.
func TestHandleSaveToMemoryTaskResumesStoredCheckpoint(t *testing.T) {
	pinMemoryNow(t, time.Date(2026, 8, 20, 10, 5, 0, 0, time.UTC))
	db := testutil.SetupTestDB(t, &entity.Task{}, &entity.MemoryTask{})
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	progressMsg := "stale UI state"
	if err := db.Create(&entity.Task{
		ID:          "task-1",
		DocID:       "memory-1",
		TaskType:    "memory",
		Progress:    -1,
		ProgressMsg: &progressMsg,
	}).Error; err != nil {
		t.Fatalf("create generic task: %v", err)
	}
	if err := db.Create(&entity.MemoryTask{
		TaskID:   "task-1",
		MemoryID: "memory-1",
		SourceID: 42,
		Input:    entity.JSONMap{},
		State:    entity.MemoryTaskStateStored,
	}).Error; err != nil {
		t.Fatalf("create memory task: %v", err)
	}

	svc := NewMemoryMessageService(nil)
	disposition, err := svc.HandleSaveToMemoryTask(t.Context(), "task-1", "worker-1")
	if err != nil || disposition != MemoryTaskAcknowledge {
		t.Fatalf("HandleSaveToMemoryTask disposition=%v err=%v", disposition, err)
	}
	stored, err := svc.memoryTaskDAO.GetByID(t.Context(), db, "task-1")
	if err != nil {
		t.Fatalf("load memory task: %v", err)
	}
	if stored.State != entity.MemoryTaskStateCompleted {
		t.Fatalf("memory task state = %q, want completed", stored.State)
	}
	var task entity.Task
	if err = db.First(&task, "id = ?", "task-1").Error; err != nil {
		t.Fatalf("load generic task: %v", err)
	}
	if task.Progress != 1 {
		t.Fatalf("generic task progress = %v, want 1", task.Progress)
	}
}

// TestHandleSaveToMemoryTaskRetriesCompletionFromStored verifies a failed UI
// progress write leaves the durable checkpoint at stored and the next attempt
// performs only the final transaction.
func TestHandleSaveToMemoryTaskRetriesCompletionFromStored(t *testing.T) {
	now := time.Date(2026, 8, 20, 10, 5, 0, 0, time.UTC)
	originalNow := memoryNow
	memoryNow = func() time.Time { return now }
	t.Cleanup(func() { memoryNow = originalNow })

	db := testutil.SetupTestDB(t, &entity.Task{}, &entity.MemoryTask{})
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	progressMsg := "stored"
	if err := db.Create(&entity.Task{
		ID:          "task-completion-retry",
		DocID:       "memory-1",
		TaskType:    "memory",
		Progress:    0.5,
		ProgressMsg: &progressMsg,
	}).Error; err != nil {
		t.Fatalf("create generic task: %v", err)
	}
	if err := db.Create(&entity.MemoryTask{
		TaskID:   "task-completion-retry",
		MemoryID: "memory-1",
		SourceID: 42,
		Input:    entity.JSONMap{},
		State:    entity.MemoryTaskStateStored,
	}).Error; err != nil {
		t.Fatalf("create memory task: %v", err)
	}
	if err := db.Exec(`
		CREATE TRIGGER fail_memory_task_completion_progress
		BEFORE UPDATE ON task
		BEGIN
			SELECT RAISE(FAIL, 'forced completion progress failure');
		END
	`).Error; err != nil {
		t.Fatalf("create update trigger: %v", err)
	}

	svc := NewMemoryMessageService(nil)
	disposition, err := svc.HandleSaveToMemoryTask(t.Context(), "task-completion-retry", "worker-1")
	if err == nil || disposition != MemoryTaskAcknowledge {
		t.Fatalf("first attempt disposition=%v err=%v, want acknowledged scheduled retry", disposition, err)
	}
	stored, err := svc.memoryTaskDAO.GetByID(t.Context(), db, "task-completion-retry")
	if err != nil {
		t.Fatalf("load stored checkpoint: %v", err)
	}
	if stored.State != entity.MemoryTaskStateStored || stored.NextRetryAt == nil {
		t.Fatalf("first attempt state/retry = %q/%v, want stored with retry", stored.State, stored.NextRetryAt)
	}
	if err = db.Exec("DROP TRIGGER fail_memory_task_completion_progress").Error; err != nil {
		t.Fatalf("drop update trigger: %v", err)
	}
	now = *stored.NextRetryAt

	disposition, err = svc.HandleSaveToMemoryTask(t.Context(), "task-completion-retry", "worker-2")
	if err != nil || disposition != MemoryTaskAcknowledge {
		t.Fatalf("retry disposition=%v err=%v, want completed acknowledgement", disposition, err)
	}
	completed, err := svc.memoryTaskDAO.GetByID(t.Context(), db, "task-completion-retry")
	if err != nil {
		t.Fatalf("load completed task: %v", err)
	}
	if completed.State != entity.MemoryTaskStateCompleted {
		t.Fatalf("retry state = %q, want completed", completed.State)
	}
}

// TestHandleSaveToMemoryTaskResumesExtractedWithoutLLM verifies an extracted
// checkpoint skips extraction even when no memory/LLM dependency is available.
func TestHandleSaveToMemoryTaskResumesExtractedWithoutLLM(t *testing.T) {
	pinMemoryNow(t, time.Date(2026, 8, 20, 10, 5, 0, 0, time.UTC))
	db := testutil.SetupTestDB(t, &entity.Task{}, &entity.MemoryTask{})
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	progressMsg := "extracted"
	if err := db.Create(&entity.Task{
		ID:          "task-extracted-resume",
		DocID:       "memory-1",
		TaskType:    "memory",
		Progress:    0.5,
		ProgressMsg: &progressMsg,
	}).Error; err != nil {
		t.Fatalf("create generic task: %v", err)
	}
	if err := db.Create(&entity.MemoryTask{
		TaskID:     "task-extracted-resume",
		MemoryID:   "memory-1",
		SourceID:   42,
		Input:      entity.JSONMap{"agent_id": "agent-1"},
		State:      entity.MemoryTaskStateExtracted,
		Extraction: entity.JSONSlice{},
	}).Error; err != nil {
		t.Fatalf("create memory task: %v", err)
	}

	svc := NewMemoryMessageService(nil)
	disposition, err := svc.HandleSaveToMemoryTask(t.Context(), "task-extracted-resume", "worker-1")
	if err != nil || disposition != MemoryTaskAcknowledge {
		t.Fatalf("HandleSaveToMemoryTask disposition=%v err=%v", disposition, err)
	}
	stored, err := svc.memoryTaskDAO.GetByID(t.Context(), db, "task-extracted-resume")
	if err != nil {
		t.Fatalf("load memory task: %v", err)
	}
	if stored.State != entity.MemoryTaskStateCompleted {
		t.Fatalf("memory task state = %q, want completed", stored.State)
	}
}

// TestPersistMemoryTaskFailureSchedulesRetry verifies a transient execution
// failure is durably scheduled before the current delivery is acknowledged.
func TestPersistMemoryTaskFailureSchedulesRetry(t *testing.T) {
	now := time.Date(2026, 8, 20, 10, 5, 0, 0, time.UTC)
	pinMemoryNow(t, now)
	db := testutil.SetupTestDB(t, &entity.Task{}, &entity.MemoryTask{})
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	if err := db.Create(&entity.MemoryTask{
		TaskID:   "task-retry",
		MemoryID: "memory-1",
		SourceID: 42,
		Input:    entity.JSONMap{},
		State:    entity.MemoryTaskStatePending,
	}).Error; err != nil {
		t.Fatalf("create memory task: %v", err)
	}

	svc := NewMemoryMessageService(nil)
	claimed, acquired, err := svc.memoryTaskDAO.Claim(t.Context(), db, "task-retry", "worker-1", now, memoryTaskLeaseTTL)
	if err != nil || !acquired {
		t.Fatalf("Claim acquired=%v err=%v, want acquired", acquired, err)
	}
	disposition, err := svc.persistMemoryTaskFailure(t.Context(), claimed, "worker-1", errors.New("temporary failure"))
	if err == nil || disposition != MemoryTaskAcknowledge {
		t.Fatalf("persistMemoryTaskFailure disposition=%v err=%v, want acknowledged failure", disposition, err)
	}
	stored, err := svc.memoryTaskDAO.GetByID(t.Context(), db, "task-retry")
	if err != nil {
		t.Fatalf("load memory task: %v", err)
	}
	if stored.State != entity.MemoryTaskStatePending {
		t.Fatalf("memory task state = %q, want pending", stored.State)
	}
	wantRetryAt := now.Add(memoryTaskRetryInitialDelay)
	if stored.NextRetryAt == nil || !stored.NextRetryAt.Equal(wantRetryAt) {
		t.Fatalf("next retry at = %v, want %v", stored.NextRetryAt, wantRetryAt)
	}
	if stored.LeaseOwner != "" || stored.LeaseExpiresAt != nil {
		t.Fatalf("lease = %q/%v, want released", stored.LeaseOwner, stored.LeaseExpiresAt)
	}
	if stored.AttemptCount != 1 || stored.LastError == "" {
		t.Fatalf("attempt/error = %d/%q, want one failed attempt", stored.AttemptCount, stored.LastError)
	}
}

// TestPersistMemoryTaskFailureLeavesDeliveryUnsettledWhenRetryWriteFails
// verifies the broker message is preserved when no durable retry decision can
// be recorded.
func TestPersistMemoryTaskFailureLeavesDeliveryUnsettledWhenRetryWriteFails(t *testing.T) {
	now := time.Date(2026, 8, 20, 10, 5, 0, 0, time.UTC)
	pinMemoryNow(t, now)
	db := testutil.SetupTestDB(t, &entity.MemoryTask{})
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	if err := db.Create(&entity.MemoryTask{
		TaskID:   "task-retry-write-failure",
		MemoryID: "memory-1",
		SourceID: 42,
		Input:    entity.JSONMap{},
		State:    entity.MemoryTaskStatePending,
	}).Error; err != nil {
		t.Fatalf("create memory task: %v", err)
	}

	svc := NewMemoryMessageService(nil)
	claimed, acquired, err := svc.memoryTaskDAO.Claim(t.Context(), db, "task-retry-write-failure", "worker-1", now, memoryTaskLeaseTTL)
	if err != nil || !acquired {
		t.Fatalf("Claim acquired=%v err=%v, want acquired", acquired, err)
	}
	if err = db.Exec(`
		CREATE TRIGGER fail_memory_task_retry_update
		BEFORE UPDATE ON memory_task
		BEGIN
			SELECT RAISE(FAIL, 'forced retry update failure');
		END
	`).Error; err != nil {
		t.Fatalf("create retry trigger: %v", err)
	}

	disposition, err := svc.persistMemoryTaskFailure(t.Context(), claimed, "worker-1", errors.New("temporary failure"))
	if err == nil || disposition != MemoryTaskLeaveUnsettled {
		t.Fatalf("persistMemoryTaskFailure disposition=%v err=%v, want unsettled failure", disposition, err)
	}
}

// TestRunClaimedMemoryTaskStopsLeaseRenewalOnPanic verifies the cleanup defer
// cancels and joins the renewal loop before a state-machine panic propagates.
func TestRunClaimedMemoryTaskStopsLeaseRenewalOnPanic(t *testing.T) {
	now := time.Date(2026, 8, 20, 10, 5, 0, 0, time.UTC)
	pinMemoryNow(t, now)
	pinMemoryTaskLeaseTimings(t, 10*time.Millisecond, 10*time.Millisecond)
	db := testutil.SetupTestDB(t, &entity.MemoryTask{})
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	expiresAt := now.Add(time.Minute)
	task := &entity.MemoryTask{
		TaskID:         "task-panic-cleanup",
		MemoryID:       "memory-1",
		SourceID:       42,
		Input:          entity.JSONMap{},
		State:          entity.MemoryTaskStatePending,
		LeaseOwner:     "worker-1",
		LeaseExpiresAt: &expiresAt,
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("create memory task: %v", err)
	}

	svc := NewMemoryMessageService(nil)
	svc.resumeTask = func(context.Context, *entity.MemoryTask, string) error {
		panic("simulated state-machine panic")
	}
	func() {
		defer func() {
			if recovered := recover(); recovered == nil {
				t.Fatal("runClaimedMemoryTask panic = nil, want propagated panic")
			}
		}()
		_, _ = svc.runClaimedMemoryTask(t.Context(), task, "worker-1")
	}()

	time.Sleep(3 * memoryTaskLeaseRenewInterval)
	stored, err := svc.memoryTaskDAO.GetByID(t.Context(), db, task.TaskID)
	if err != nil {
		t.Fatalf("load memory task: %v", err)
	}
	if stored.LeaseExpiresAt == nil || !stored.LeaseExpiresAt.Equal(expiresAt) {
		t.Fatalf("lease expiry = %v, want unchanged %v", stored.LeaseExpiresAt, expiresAt)
	}
}

// TestRenewMemoryTaskLeaseTimesOutBlockedUpdate verifies an unresponsive lease
// write cancels execution before the durable lease can silently expire.
func TestRenewMemoryTaskLeaseTimesOutBlockedUpdate(t *testing.T) {
	now := time.Date(2026, 8, 20, 10, 5, 0, 0, time.UTC)
	pinMemoryNow(t, now)
	pinMemoryTaskLeaseTimings(t, 5*time.Millisecond, 10*time.Millisecond)
	db := testutil.SetupTestDB(t, &entity.MemoryTask{})
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	expiresAt := now.Add(time.Minute)
	if err := db.Create(&entity.MemoryTask{
		TaskID:         "task-renew-timeout",
		MemoryID:       "memory-1",
		SourceID:       42,
		Input:          entity.JSONMap{},
		State:          entity.MemoryTaskStatePending,
		LeaseOwner:     "worker-1",
		LeaseExpiresAt: &expiresAt,
	}).Error; err != nil {
		t.Fatalf("create memory task: %v", err)
	}
	if err := db.Callback().Update().Before("gorm:update").Register("block_memory_task_lease_renewal", func(tx *gorm.DB) {
		<-tx.Statement.Context.Done()
		tx.AddError(tx.Statement.Context.Err())
	}); err != nil {
		t.Fatalf("register blocking update callback: %v", err)
	}

	svc := NewMemoryMessageService(nil)
	runCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	renewErr := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		svc.renewMemoryTaskLease(runCtx, cancel, "task-renew-timeout", "worker-1", renewErr)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("renewMemoryTaskLease did not stop after update timeout")
	}
	if runCtx.Err() == nil {
		t.Fatal("run context was not canceled after lease renewal timeout")
	}
	select {
	case err := <-renewErr:
		if err == nil || !strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
			t.Fatalf("renew error = %v, want deadline exceeded", err)
		}
	default:
		t.Fatal("renewMemoryTaskLease did not report the timeout")
	}
}

// TestHandleSaveToMemoryTaskMarksInvalidInputFailed verifies corrupt durable
// input reaches the terminal state and updates the UI progress projection.
func TestHandleSaveToMemoryTaskMarksInvalidInputFailed(t *testing.T) {
	pinMemoryNow(t, time.Date(2026, 8, 20, 10, 5, 0, 0, time.UTC))
	db := testutil.SetupTestDB(t, &entity.Task{}, &entity.MemoryTask{})
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	progressMsg := "queued"
	if err := db.Create(&entity.Task{
		ID:          "task-invalid",
		DocID:       "memory-1",
		TaskType:    "memory",
		Progress:    0,
		ProgressMsg: &progressMsg,
	}).Error; err != nil {
		t.Fatalf("create generic task: %v", err)
	}
	if err := db.Create(&entity.MemoryTask{
		TaskID:   "task-invalid",
		MemoryID: "memory-1",
		SourceID: 42,
		Input:    entity.JSONMap{},
		State:    entity.MemoryTaskStatePending,
	}).Error; err != nil {
		t.Fatalf("create memory task: %v", err)
	}

	svc := NewMemoryMessageService(nil)
	disposition, err := svc.HandleSaveToMemoryTask(t.Context(), "task-invalid", "worker-1")
	if err == nil || disposition != MemoryTaskAcknowledge {
		t.Fatalf("HandleSaveToMemoryTask disposition=%v err=%v, want acknowledged terminal failure", disposition, err)
	}
	stored, err := svc.memoryTaskDAO.GetByID(t.Context(), db, "task-invalid")
	if err != nil {
		t.Fatalf("load memory task: %v", err)
	}
	if stored.State != entity.MemoryTaskStateFailed || stored.NextRetryAt != nil {
		t.Fatalf("memory task state/retry = %q/%v, want failed without retry", stored.State, stored.NextRetryAt)
	}
	var task entity.Task
	if err = db.First(&task, "id = ?", "task-invalid").Error; err != nil {
		t.Fatalf("load generic task: %v", err)
	}
	if task.Progress != -1 || task.ProgressMsg == nil || !strings.Contains(*task.ProgressMsg, "agent_id is required") {
		t.Fatalf("generic task progress/message = %v/%v, want terminal input error", task.Progress, task.ProgressMsg)
	}
}

// TestMemoryTaskRetryDelay verifies retries use bounded exponential backoff.
func TestMemoryTaskRetryDelay(t *testing.T) {
	for _, test := range []struct {
		attemptCount int
		want         time.Duration
	}{
		{attemptCount: 0, want: 5 * time.Second},
		{attemptCount: 1, want: 5 * time.Second},
		{attemptCount: 2, want: 10 * time.Second},
		{attemptCount: 7, want: 5 * time.Minute},
		{attemptCount: 100, want: 5 * time.Minute},
	} {
		if got := memoryTaskRetryDelay(test.attemptCount); got != test.want {
			t.Errorf("memoryTaskRetryDelay(%d) = %v, want %v", test.attemptCount, got, test.want)
		}
	}
}

// TestUpdateTaskProgressReturnsPersistenceError verifies UI projection writes
// surface persistence failures without becoming execution checkpoints.
func TestUpdateTaskProgressReturnsPersistenceError(t *testing.T) {
	db := testutil.SetupTestDB(t, &entity.IngestionTask{})
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	svc := &MemoryMessageService{taskDAO: dao.NewTaskDAO()}
	if err := svc.updateTaskProgress(t.Context(), "mem-task-1", 1.0, "completed"); err == nil {
		t.Fatal("updateTaskProgress() error = nil, want persistence error")
	}
}

// TestTypeInstructionsStayInLockstepWithPython pins the rules that
// memory/utils/prompt_util.py carries and the Go map historically dropped:
// per-type Examples lines, the full "Timestamp Rules for <Type> Knowledge:"
// titles, and the semantic Default line. If a substring fails here, the two
// implementations are handing the extracting LLM different contracts (the
// drift this map was merged with — see #18415).
func TestTypeInstructionsStayInLockstepWithPython(t *testing.T) {
	want := map[string][]string{
		"semantic": {
			"- Examples: \"The capital of France is Paris\", \"Water boils at 100°C\"",
			"**Timestamp Rules for Semantic Knowledge:**",
			"- valid_at: When the fact became true (e.g., law enactment, discovery)",
			"- invalid_at: When it becomes false (e.g., repeal, disproven) or empty if still true",
			"- Default: valid_at = conversation time, invalid_at = \"\" for timeless facts",
		},
		"episodic": {
			"- Examples: \"Yesterday I fixed the bug\", \"User reported issue last week\"",
			"**Timestamp Rules for Episodic Knowledge:**",
			"- Extract explicit times: \"at 3 PM\", \"last Monday\", \"from X to Y\"",
		},
		"procedural": {
			"- Examples: \"To reset password, click...\", \"Debugging steps: 1)...\"",
			"**Timestamp Rules for Procedural Knowledge:**",
			"- For version-specific: use release dates",
			"- For best practices: invalid_at = \"\"",
		},
	}
	for memoryType, lines := range want {
		got, ok := TYPE_INSTRUCTIONS[memoryType]
		if !ok {
			t.Fatalf("TYPE_INSTRUCTIONS has no entry for %q", memoryType)
		}
		for _, line := range lines {
			if !strings.Contains(got, line) {
				t.Errorf("TYPE_INSTRUCTIONS[%q] is missing the Python-side rule: %s", memoryType, line)
			}
		}
	}
}
