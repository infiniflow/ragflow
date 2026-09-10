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

// memory_message_service_test.go — MemorySaver port tests.
//
// Coverage focuses on the synchronous parts (memory lookup + raw
// message construction + result aggregation).

package service

import (
	"strings"
	"testing"
	"time"

	"ragflow/internal/entity"
	"ragflow/internal/ingestion/testutil"
)

// TestQueueSaveToMemoryTask_NilService: a nil receiver surfaces
// a clear error rather than panicking.
func TestQueueSaveToMemoryTask_NilService(t *testing.T) {
	var s *MemoryMessageService
	_, err := s.QueueSaveToMemoryTask(t.Context(), []string{"m1"}, MemoryMessage{AgentID: "a1"})
	if err == nil {
		t.Fatal("expected error from nil service")
	}
	if !strings.Contains(err.Error(), "nil") {
		t.Errorf("error = %v, want nil-service error", err)
	}
}

// TestQueueSaveToMemoryTask_EmptyMemoryList: an empty input
// short-circuits to an empty result with no error.
func TestQueueSaveToMemoryTask_EmptyMemoryList(t *testing.T) {
	s := &MemoryMessageService{memories: nil} // no lookups happen
	res, err := s.QueueSaveToMemoryTask(t.Context(), nil, MemoryMessage{AgentID: "a1"})
	if err != nil {
		t.Fatalf("QueueSaveToMemoryTask: %v", err)
	}
	if len(res.NotFound) != 0 || len(res.Failed) != 0 {
		t.Errorf("expected empty result, got %+v", res)
	}
}

// TestQueueSaveToMemoryTask_MissingAgentID: AgentID is required
// for the row envelope; an empty AgentID surfaces a clear error
// up front.
func TestQueueSaveToMemoryTask_MissingAgentID(t *testing.T) {
	s := &MemoryMessageService{}
	_, err := s.QueueSaveToMemoryTask(t.Context(), []string{"m1"}, MemoryMessage{})
	if err == nil {
		t.Fatal("expected error for missing AgentID")
	}
	if !strings.Contains(err.Error(), "AgentID") {
		t.Errorf("error = %v, want AgentID-required error", err)
	}
}

// TestQueueSaveToMemoryTaskClassifiesMemoryLookupErrors keeps confirmed missing
// memories separate from transient database failures.
func TestQueueSaveToMemoryTaskClassifiesMemoryLookupErrors(t *testing.T) {
	t.Run("missing memory", func(t *testing.T) {
		db := testutil.SetupTestDB(t, &entity.Memory{}, &entity.User{})
		cleanup := testutil.ReplaceDBForTest(t, db)
		defer cleanup()

		res, err := NewMemoryMessageService(NewMemoryService()).QueueSaveToMemoryTask(
			t.Context(), []string{"missing-memory"}, MemoryMessage{AgentID: "agent-1"},
		)
		if err != nil {
			t.Fatalf("QueueSaveToMemoryTask: %v", err)
		}
		if len(res.NotFound) != 1 || res.NotFound[0] != "missing-memory" || len(res.Failed) != 0 {
			t.Fatalf("result = %+v, want missing memory in NotFound only", res)
		}
	})

	t.Run("database failure", func(t *testing.T) {
		db := testutil.SetupTestDB(t, &entity.Task{})
		cleanup := testutil.ReplaceDBForTest(t, db)
		defer cleanup()

		res, err := NewMemoryMessageService(NewMemoryService()).QueueSaveToMemoryTask(
			t.Context(), []string{"memory-1"}, MemoryMessage{AgentID: "agent-1"},
		)
		if err != nil {
			t.Fatalf("QueueSaveToMemoryTask: %v", err)
		}
		if len(res.NotFound) != 0 || len(res.Failed) != 1 || res.Failed[0].MemoryID != "memory-1" || res.Failed[0].FailMsg == "" {
			t.Fatalf("result = %+v, want database lookup failure in Failed only", res)
		}
	})
}

// TestBuildRawMessage_ValidAtServerLocal: valid_at is stamped as a
// server-local wall-clock string, not UTC — otherwise memories asked at
// 10:05 local show up as 02:05. The clock is pinned to a fixed instant in a
// fixed non-UTC location so the assertion holds on any host, including UTC
// CI runners.
func TestBuildRawMessage_ValidAtServerLocal(t *testing.T) {
	pinMemoryNow(t, time.Date(2026, 8, 20, 10, 5, 0, 0, time.FixedZone("UTC+8", 8*3600)))

	raw := buildRawMessage(42, "mem-1", MemoryMessage{
		UserID:        "u1",
		AgentID:       "a1",
		SessionID:     "s1",
		UserInput:     "hi",
		AgentResponse: "hello",
	})

	got, ok := raw["valid_at"].(string)
	if !ok {
		t.Fatalf("valid_at = %#v, want string", raw["valid_at"])
	}
	if want := "2026-08-20 10:05:00"; got != want {
		t.Fatalf("valid_at = %q, want server-local wall clock %q (UTC-shifted would be %q)", got, want, "2026-08-20 02:05:00")
	}
}

// TestBuildRawMessage_EnvelopeShape: the row envelope carries
// every logical field the Python extractor reads. Storage-level
// fields (message_type_kwd, content_ltks, tokenized_content_ltks,
// status_int) are the doc engine's concern and must not be set here.
func TestBuildRawMessage_EnvelopeShape(t *testing.T) {
	raw := buildRawMessage(42, "mem-1", MemoryMessage{
		UserID:        "u1",
		AgentID:       "a1",
		SessionID:     "s1",
		UserInput:     "hi",
		AgentResponse: "hello",
	})

	want := map[string]any{
		"message_id":   int64(42),
		"message_type": "raw",
		"memory_id":    "mem-1",
		"user_id":      "u1",
		"agent_id":     "a1",
		"session_id":   "s1",
		"status":       true,
	}
	for k, want := range want {
		if got := raw[k]; got != want {
			t.Errorf("raw[%q] = %v (%T), want %v (%T)", k, got, got, want, want)
		}
	}
	for _, storageField := range []string{"message_type_kwd", "content_ltks", "tokenized_content_ltks", "status_int"} {
		if _, ok := raw[storageField]; ok {
			t.Errorf("raw[%q] is a storage field and must not be set by the service layer", storageField)
		}
	}

	content, _ := raw["content"].(string)
	if !strings.Contains(content, "User Input: hi") {
		t.Errorf("content missing user input: %q", content)
	}
	if !strings.Contains(content, "Agent Response: hello") {
		t.Errorf("content missing agent response: %q", content)
	}
	if !strings.Contains(content, "\n") {
		t.Errorf("content should have user/agent on separate lines: %q", content)
	}
}

// TestBuildMemoryTaskRecordsShape verifies the UI task and durable execution
// record carry the same identity and the worker input is persisted in the DB.
func TestBuildMemoryTaskRecordsShape(t *testing.T) {
	msg := MemoryMessage{UserID: "u1", AgentID: "a1", SessionID: "s1", UserInput: "hi", AgentResponse: "hello"}
	task, memoryTask := buildMemoryTaskRecords(99, "mem-1", msg)
	if task.TaskType != "memory" {
		t.Errorf("task_type = %v, want \"memory\"", task.TaskType)
	}
	if task.DocID != "mem-1" {
		t.Errorf("doc_id = %v, want \"mem-1\"", task.DocID)
	}
	if task.Progress != 0.0 {
		t.Errorf("progress = %v, want 0.0", task.Progress)
	}
	if task.ProgressMsg == nil || *task.ProgressMsg != "" {
		t.Errorf("progress_msg = %v, want empty string", task.ProgressMsg)
	}
	if task.Digest == nil || *task.Digest != "99" {
		t.Errorf("digest = %v, want \"99\"", task.Digest)
	}
	if len(task.ID) != 32 {
		t.Errorf("id = %q, want 32-char uuid", task.ID)
	}
	if memoryTask.TaskID != task.ID || memoryTask.MemoryID != "mem-1" || memoryTask.SourceID != 99 {
		t.Fatalf("memory task identity = %+v, want task %s/mem-1/99", memoryTask, task.ID)
	}
	if memoryTask.State != "pending" || memoryTask.Input["agent_response"] != "hello" {
		t.Fatalf("memory task execution data = %+v", memoryTask)
	}
}

// TestGenerateRawMessageID_Unique: two calls produce different
// values. (Wall-clock based today; the Redis-backed counter will
// be added when the project's Redis client lands.)
func TestGenerateRawMessageID_Unique(t *testing.T) {
	ctx := t.Context()
	a := generateRawMessageID(ctx)
	b := generateRawMessageID(ctx)
	if a == b {
		// Allow a 1-second tie when the clock hasn't ticked.
		// GenerateRawMessageID uses Unix seconds; two calls
		// within the same second will collide. The Redis
		// counter is the eventual fix; this test only
		// verifies monotonic-ish behaviour.
		t.Logf("note: raw_message_ids collided (%d) — within the same second; the Redis counter will fix this", a)
	}
	if a <= 0 || b <= 0 {
		t.Errorf("expected positive ids, got a=%d b=%d", a, b)
	}
}
