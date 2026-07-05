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

// Tests for the conversation-history round-trip helpers used by
// BotService.ChatbotCompletion. Locks in review Finding 8 — a resumed
// session_id must carry prior turns (assistant prologue + earlier
// user/assistant exchanges) into the next LLM call so multi-turn
// chatbot clients retain context.

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"

	modelModule "ragflow/internal/entity/models"
)

func TestHistoryToMessages_Empty(t *testing.T) {
	// A freshly-seeded session with no prior turns returns an empty
	// slice. Caller appends the new user turn; LLM receives only
	// the current prompt. Matches python conversation_service seed.
	got := historyToMessages(nil)
	if len(got) != 0 {
		t.Fatalf("nil raw: want 0 messages, got %d", len(got))
	}
	got = historyToMessages(json.RawMessage(`[]`))
	if len(got) != 0 {
		t.Fatalf("empty array: want 0 messages, got %d", len(got))
	}
}

func TestHistoryToMessages_RoundTrip(t *testing.T) {
	// Simulate a session with: 1 prologue assistant turn + 1 prior
	// user/assistant pair. The LLM must see all 3 prior turns
	// before the new user turn is appended.
	turns := []map[string]any{
		{"role": "assistant", "content": "Hello, how can I help?", "created_at": 1},
		{"role": "user", "content": "What is Go?", "created_at": 2},
		{"role": "assistant", "content": "Go is a compiled language.", "created_at": 3},
	}
	raw, err := json.Marshal(turns)
	if err != nil {
		t.Fatalf("marshal seed: %v", err)
	}
	msgs := historyToMessages(raw)
	if len(msgs) != 3 {
		t.Fatalf("want 3 prior messages, got %d", len(msgs))
	}
	if msgs[0].Role != "assistant" || msgs[0].Content != "Hello, how can I help?" {
		t.Errorf("turn 0: role=%q content=%q", msgs[0].Role, msgs[0].Content)
	}
	if msgs[1].Role != "user" || msgs[1].Content != "What is Go?" {
		t.Errorf("turn 1: role=%q content=%q", msgs[1].Role, msgs[1].Content)
	}
	if msgs[2].Role != "assistant" || msgs[2].Content != "Go is a compiled language." {
		t.Errorf("turn 2: role=%q content=%q", msgs[2].Role, msgs[2].Content)
	}
}

func TestHistoryToMessages_Malformed(t *testing.T) {
	// Malformed JSON must not panic; returns nil so caller falls back
	// to a fresh single-turn LLM call rather than failing the request.
	got := historyToMessages(json.RawMessage(`not json`))
	if got != nil {
		t.Fatalf("malformed raw: want nil, got %v", got)
	}
}

func TestHistoryToMessages_SkipsEmptyFields(t *testing.T) {
	// Defensive: turns missing role or content are dropped, not
	// passed to the LLM as empty messages.
	turns := []map[string]any{
		{"role": "assistant", "content": "valid", "created_at": 1},
		{"role": "", "content": "no role", "created_at": 2},
		{"role": "user", "content": "", "created_at": 3},
		{"role": "user", "content": "second valid", "created_at": 4},
	}
	raw, _ := json.Marshal(turns)
	msgs := historyToMessages(raw)
	if len(msgs) != 2 {
		t.Fatalf("want 2 valid turns, got %d", len(msgs))
	}
	if msgs[0].Content != "valid" || msgs[1].Content != "second valid" {
		t.Errorf("got %+v", msgs)
	}
}

func TestHistoryFromMessages_PreservesOrder(t *testing.T) {
	// The LLM driver returns messages in the same order the input
	// was provided. The round-trip must preserve that order so the
	// next call to ChatbotCompletion sees a coherent history.
	msgs := []modelModule.Message{
		{Role: "assistant", Content: "first"},
		{Role: "user", Content: "second"},
		{Role: "assistant", Content: "third"},
	}
	turns := historyFromMessages(msgs)
	if len(turns) != 3 {
		t.Fatalf("want 3 turns, got %d", len(turns))
	}
	for i, want := range []string{"first", "second", "third"} {
		if turns[i]["content"] != want {
			t.Errorf("turn %d content = %v, want %q", i, turns[i]["content"], want)
		}
		if turns[i]["role"] != msgs[i].Role {
			t.Errorf("turn %d role = %v, want %q", i, turns[i]["role"], msgs[i].Role)
		}
	}
}

func TestHistoryRoundTrip_PreservesPriorTurns(t *testing.T) {
	// End-to-end: prior JSON → history → back to JSON must be
	// semantically identical (modulo the created_at monotonic
	// adjustment that historyFromMessages applies for ordering).
	turns := []map[string]any{
		{"role": "assistant", "content": "p1", "created_at": int64(100)},
		{"role": "user", "content": "p2", "created_at": int64(200)},
	}
	raw, _ := json.Marshal(turns)

	msgs := historyToMessages(raw)
	// Caller appends a new user turn (the current request).
	msgs = append(msgs, modelModule.Message{Role: "user", Content: "current"})

	// Round-trip back to JSON for storage.
	newTurns := historyFromMessages(msgs)
	raw2, err := json.Marshal(newTurns)
	if err != nil {
		t.Fatalf("marshal round-trip: %v", err)
	}

	var got []map[string]any
	if err := json.Unmarshal(raw2, &got); err != nil {
		t.Fatalf("unmarshal round-trip: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 turns after round-trip, got %d", len(got))
	}
	expected := []struct{ role, content string }{
		{"assistant", "p1"},
		{"user", "p2"},
		{"user", "current"},
	}
	for i, want := range expected {
		if got[i]["role"] != want.role || got[i]["content"] != want.content {
			t.Errorf("turn %d: got role=%v content=%v, want role=%q content=%q",
				i, got[i]["role"], got[i]["content"], want.role, want.content)
		}
	}
}

// TestBotService_AgentbotInputs_CrossTenantDenied mirrors PR
// #15457: when a beta API token authenticates a caller with
// tenantID, that caller must not be able to read an agent
// belonging to a different tenant. The Go guard runs inside
// loadCanvas (called at the entry of AgentbotInputs and
// AgentbotCompletion) and returns ErrUserCanvasNotFound — same
// 404-equivalent shape as the python fix returns "Can't find
// agent by ID: <id>". This test seeds a canvas under tenant-A
// and asks for it via tenant-B; the call must fail with the
// not-found error and never expose the canvas.
func TestBotService_AgentbotInputs_CrossTenantDenied(t *testing.T) {
	db := setupServiceTestDB(t)
	if err := db.AutoMigrate(
		&entity.UserCanvas{},
		&entity.UserTenant{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	// Seed tenant-A and a canvas owned by user-A.
	if err := db.Create(&entity.UserTenant{
		ID:       "ut-A",
		UserID:   "user-A",
		TenantID: "tenant-A",
		Role:     "owner",
	}).Error; err != nil {
		t.Fatalf("seed tenant-A: %v", err)
	}
	if err := db.Create(&entity.UserCanvas{
		ID:             "agent-victim",
		UserID:         "user-A",
		Title:          sptr("Victim Agent"),
		CanvasCategory: "agent_canvas",
	}).Error; err != nil {
		t.Fatalf("seed victim canvas: %v", err)
	}

	// Seed tenant-B (the attacker's tenant).
	if err := db.Create(&entity.UserTenant{
		ID:       "ut-B",
		UserID:   "user-B",
		TenantID: "tenant-B",
		Role:     "owner",
	}).Error; err != nil {
		t.Fatalf("seed tenant-B: %v", err)
	}

	svc := NewBotService(nil, nil)

	// Attacker (tenant-B) asks for victim (tenant-A's canvas).
	title, _, _, _, _, code, err := svc.AgentbotInputs(context.Background(),
		"tenant-B", "agent-victim")
	if !errors.Is(err, dao.ErrUserCanvasNotFound) {
		t.Errorf("cross-tenant: want ErrUserCanvasNotFound, got %v", err)
	}
	if code != common.CodeDataError {
		t.Errorf("cross-tenant: want code %d, got %d", common.CodeDataError, code)
	}
	if title != "" {
		t.Errorf("cross-tenant: title should be empty, got %q (data leak)", title)
	}
}

// TestWriteChatbotRunEvent_UserInputsEvent guards PR #14589: the
// SSE envelope must carry the canvas event type so the front-end
// can distinguish interactive "user_inputs" / "workflow_finished"
// events (which need a UserFillUp form) from plain "message"
// events (assistant text). Without the `event` field the form
// UI never appears and the canvas appears to hang.
func TestWriteChatbotRunEvent_UserInputsEvent(t *testing.T) {
	rec := &recordingResponseWriter{header: http.Header{}}
	if err := WriteChatbotRunEvent(rec, canvas.RunEvent{
		Type:      "user_inputs",
		MessageID: "msg-1",
		TaskID:    "task-1",
		Data:      `{"components":[{"id":"email","type":"text","required":true}]}`,
		SessionID: "sess-1",
	}); err != nil {
		t.Fatalf("WriteChatbotRunEvent: %v", err)
	}
	body := rec.body.String()
	if !strings.Contains(body, `"event":"user_inputs"`) {
		t.Errorf("body missing event=user_inputs: %s", body)
	}
	if !strings.Contains(body, `"message_id":"msg-1"`) {
		t.Errorf("body missing message_id: %s", body)
	}
	if !strings.Contains(body, `"task_id":"task-1"`) {
		t.Errorf("body missing task_id: %s", body)
	}
	if !strings.Contains(body, `"session_id":"sess-1"`) {
		t.Errorf("body missing session_id: %s", body)
	}
	if strings.Contains(body, `"answer":"`) {
		t.Errorf("body should not wrap run events in data.answer: %s", body)
	}
}

// TestWriteChatbotRunEvent_WorkflowFinishedEvent covers the second
// new event type from PR #14589. The envelope must also carry
// "workflow_finished" verbatim.
func TestWriteChatbotRunEvent_WorkflowFinishedEvent(t *testing.T) {
	rec := &recordingResponseWriter{header: http.Header{}}
	if err := WriteChatbotRunEvent(rec, canvas.RunEvent{
		Type:      "workflow_finished",
		Data:      `{"outputs":"done"}`,
		SessionID: "sess-2",
	}); err != nil {
		t.Fatalf("WriteChatbotRunEvent: %v", err)
	}
	body := rec.body.String()
	if !strings.Contains(body, `"event":"workflow_finished"`) {
		t.Errorf("body missing event=workflow_finished: %s", body)
	}
	if !strings.Contains(body, `"outputs":"done"`) {
		t.Errorf("body missing workflow output payload: %s", body)
	}
}

// TestWriteChatbotRunEvent_MessageEventCarriesEvent ensures the
// existing "message" path also carries the event field. The
// front-end can rely on `data.event` to distinguish message
// frames from user_inputs / workflow_finished frames without
// a separate header.
func TestWriteChatbotRunEvent_MessageEventCarriesEvent(t *testing.T) {
	rec := &recordingResponseWriter{header: http.Header{}}
	if err := WriteChatbotRunEvent(rec, canvas.RunEvent{
		Type:      "message",
		MessageID: "msg-3",
		Data:      `{"content":"hi"}`,
		SessionID: "sess-3",
	}); err != nil {
		t.Fatalf("WriteChatbotRunEvent: %v", err)
	}
	body := rec.body.String()
	if !strings.Contains(body, `"event":"message"`) {
		t.Errorf("message frame should carry event=message: %s", body)
	}
	if !strings.Contains(body, `"message_id":"msg-3"`) {
		t.Errorf("message frame should carry top-level message_id: %s", body)
	}
	if !strings.Contains(body, `"content":"hi"`) {
		t.Errorf("message frame should carry data.content: %s", body)
	}
}

// recordingResponseWriter is a minimal http.ResponseWriter stub
// for SSE frame tests. Tracks writes so the test can assert the
// emitted frame contents.
type recordingResponseWriter struct {
	header http.Header
	body   bytes.Buffer
}

func (r *recordingResponseWriter) Header() http.Header {
	return r.header
}
func (r *recordingResponseWriter) Write(b []byte) (int, error) {
	return r.body.Write(b)
}
func (r *recordingResponseWriter) WriteHeader(_ int) {}
