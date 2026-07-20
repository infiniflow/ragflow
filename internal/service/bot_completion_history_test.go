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

// Tests for the message-building / flag-normalisation helpers used
// by BotService.ChatbotCompletion. Locks in review Finding 8 — a
// resumed session_id must carry prior turns (assistant prologue +
// earlier user/assistant exchanges) into the next pipeline call so
// multi-turn chatbot clients retain context, and the filtering must
// match python async_iframe_completion (drop system turns and the
// leading assistant prologue).

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
)

func TestBuildChatbotPipelineMessages_Empty(t *testing.T) {
	// A freshly-seeded session with no prior turns produces just
	// the new user question. Matches python conversation_service.
	msgs := buildChatbotPipelineMessages(nil, "hi", "msg-1")
	if len(msgs) != 1 {
		t.Fatalf("nil raw: want 1 message, got %d", len(msgs))
	}
	if msgs[0]["role"] != "user" || msgs[0]["content"] != "hi" || msgs[0]["id"] != "msg-1" {
		t.Errorf("got %+v", msgs[0])
	}

	msgs = buildChatbotPipelineMessages(json.RawMessage(`[]`), "hi", "msg-1")
	if len(msgs) != 1 {
		t.Fatalf("empty array: want 1 message, got %d", len(msgs))
	}
}

func TestBuildChatbotPipelineMessages_DropsLeadingAssistantAndSystem(t *testing.T) {
	// Prologue (leading assistant) and system turns must not reach
	// the pipeline; later assistant turns are kept so the LLM sees
	// prior exchanges.
	turns := []map[string]any{
		{"role": "assistant", "content": "Hello, how can I help?", "created_at": 1},
		{"role": "system", "content": "hidden", "created_at": 2},
		{"role": "user", "content": "What is Go?", "created_at": 3},
		{"role": "assistant", "content": "Go is a compiled language.", "created_at": 4},
	}
	raw, err := json.Marshal(turns)
	if err != nil {
		t.Fatalf("marshal seed: %v", err)
	}
	msgs := buildChatbotPipelineMessages(raw, "and Rust?", "msg-2")
	if len(msgs) != 3 {
		t.Fatalf("want 3 messages (user, assistant, new user), got %d: %+v", len(msgs), msgs)
	}
	if msgs[0]["role"] != "user" || msgs[0]["content"] != "What is Go?" {
		t.Errorf("turn 0: %+v", msgs[0])
	}
	if msgs[1]["role"] != "assistant" || msgs[1]["content"] != "Go is a compiled language." {
		t.Errorf("turn 1: %+v", msgs[1])
	}
	if msgs[2]["role"] != "user" || msgs[2]["content"] != "and Rust?" || msgs[2]["id"] != "msg-2" {
		t.Errorf("turn 2: %+v", msgs[2])
	}
}

func TestBuildChatbotPipelineMessages_Malformed(t *testing.T) {
	// Malformed JSON must not panic; falls back to just the new
	// question rather than failing the request.
	msgs := buildChatbotPipelineMessages(json.RawMessage(`not json`), "hi", "msg-3")
	if len(msgs) != 1 || msgs[0]["content"] != "hi" {
		t.Fatalf("malformed raw: want single user turn, got %+v", msgs)
	}
}

func TestNormalizeBotBoolFlag(t *testing.T) {
	cases := []struct {
		in    any
		value bool
		ok    bool
	}{
		{true, true, true},
		{false, false, true},
		{float64(1), true, true},
		{float64(0), false, true},
		{1, true, true},
		{0, false, true},
		{nil, false, false},
		{"yes", false, false},
		{float64(2), false, false},
	}
	for _, c := range cases {
		value, ok := normalizeBotBoolFlag(c.in)
		if value != c.value || ok != c.ok {
			t.Errorf("normalizeBotBoolFlag(%v) = (%v, %v), want (%v, %v)",
				c.in, value, ok, c.value, c.ok)
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
