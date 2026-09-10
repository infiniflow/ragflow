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
	"errors"
	"strings"
	"testing"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/testutil"
)

// pinMemoryNow fixes the memory wall clock at the given instant for the
// duration of a test, restoring it afterwards.
func pinMemoryNow(t *testing.T, instant time.Time) {
	t.Helper()
	orig := memoryNow
	memoryNow = func() time.Time { return instant }
	t.Cleanup(func() { memoryNow = orig })
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
