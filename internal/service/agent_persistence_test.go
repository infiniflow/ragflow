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
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func TestAgentRunSessionUpdateFailurePreventsSuccessEvents(t *testing.T) {
	testDB := setupServiceTestDB(t)
	if err := testDB.AutoMigrate(&entity.API4Conversation{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	originalDB := dao.DB
	dao.DB = testDB
	t.Cleanup(func() { dao.DB = originalDB })

	if err := dao.NewAPI4ConversationDAO().Create(t.Context(), testDB, &entity.API4Conversation{
		ID:        "session-update-failure",
		DialogID:  "canvas-update-failure",
		UserID:    "user-1",
		Message:   json.RawMessage(`[]`),
		Reference: json.RawMessage(`[]`),
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := testDB.Exec(`
		CREATE TRIGGER fail_agent_session_update
		BEFORE UPDATE ON api_4_conversation
		BEGIN
			SELECT RAISE(FAIL, 'forced update failure');
		END
	`).Error; err != nil {
		t.Fatalf("create update trigger: %v", err)
	}

	dsl := map[string]any{
		"components": map[string]any{
			"begin_0": map[string]any{
				"obj":        map[string]any{"component_name": "Begin", "params": map[string]any{}},
				"downstream": []any{"message_0"},
			},
			"message_0": map[string]any{
				"obj":      map[string]any{"component_name": "Message", "params": map[string]any{"text": "hello {{sys.query}}"}},
				"upstream": []any{"begin_0"},
			},
		},
		"path": []any{"begin_0", "message_0"},
	}
	events := make(chan canvas.RunEvent, 32)
	_, err := NewAgentService().buildRunFunc("canvas-update-failure", nil, dsl)(context.Background(), map[string]any{
		"__events__":     events,
		"__message_id__": "message-1",
		"__session_id__": "session-update-failure",
		"user_id":        "user-1",
		"user_input":     "world",
	})
	if !errors.Is(err, ErrAgentStorageError) {
		t.Fatalf("run error = %v, want ErrAgentStorageError", err)
	}
	if !strings.Contains(err.Error(), "forced update failure") {
		t.Fatalf("run error = %v, want underlying update failure", err)
	}
	close(events)
	for event := range events {
		if event.Type == "message_end" || event.Type == "workflow_finished" {
			t.Fatalf("persistence failure emitted success event %q", event.Type)
		}
	}
}

func TestAgentSessionMessageContent(t *testing.T) {
	tests := []struct {
		name     string
		answer   string
		thinking string
		want     string
	}{
		{"no thinking keeps answer as-is", "final answer", "", "final answer"},
		{"thinking wrapped before answer", "final answer", "reasoning trace", "<think>reasoning trace</think>final answer"},
		{"empty answer keeps thinking section", "", "reasoning trace", "<think>reasoning trace</think>"},
		{"inline think tags preserved when no separate thinking", "pre <think>inline</think> post", "", "pre <think>inline</think> post"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := agentSessionMessageContent(tt.answer, tt.thinking); got != tt.want {
				t.Errorf("agentSessionMessageContent(%q, %q) = %q, want %q", tt.answer, tt.thinking, got, tt.want)
			}
		})
	}
}

// TestPersistAgentRunSessionPreservesThinking guards the chat.thought
// ("思考完成") indicator: the assistant message stored on the agent session
// must keep the reasoning segment wrapped in <think> tags, mirroring Python
// canvas_service.completion. The chat UI refetches the session message list
// after streaming and renders the thought section from those tags.
func TestPersistAgentRunSessionPreservesThinking(t *testing.T) {
	testDB := setupServiceTestDB(t)
	if err := testDB.AutoMigrate(&entity.API4Conversation{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	originalDB := dao.DB
	dao.DB = testDB
	t.Cleanup(func() { dao.DB = originalDB })

	if err := dao.NewAPI4ConversationDAO().Create(t.Context(), testDB, &entity.API4Conversation{
		ID:        "session-think",
		DialogID:  "canvas-think",
		UserID:    "user-1",
		Message:   json.RawMessage(`[]`),
		Reference: json.RawMessage(`[]`),
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}

	svc := NewAgentService()
	if err := svc.persistAgentRunQuestion(t.Context(), "canvas-think", "user-1", "session-think", "msg-think-1", "question", 1); err != nil {
		t.Fatalf("persist question: %v", err)
	}
	if err := svc.persistAgentRunSession(context.Background(), "canvas-think", "user-1", "session-think", "msg-think-1", "question", "final answer", "reasoning trace", map[string]interface{}{}, nil, nil, true); err != nil {
		t.Fatalf("persist: %v", err)
	}

	conv, err := dao.NewAPI4ConversationDAO().GetByID(t.Context(), testDB, "session-think")
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	var messages []map[string]any
	if err := json.Unmarshal(conv.Message, &messages); err != nil {
		t.Fatalf("decode messages: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("messages = %d, want 2 (user + assistant)", len(messages))
	}
	if role, _ := messages[1]["role"].(string); role != "assistant" {
		t.Fatalf("second message role = %q, want assistant", role)
	}
	if got, _ := messages[1]["content"].(string); got != "<think>reasoning trace</think>final answer" {
		t.Fatalf("persisted assistant content = %q, want %q", got, "<think>reasoning trace</think>final answer")
	}
	// Both requests must save their questions while the session is busy,
	// then complete without overwriting either answer or immutable reference.
	if err := testDB.AutoMigrate(&entity.UserCanvas{}, &entity.UserCanvasVersion{}); err != nil {
		t.Fatal(err)
	}
	dsl := entity.JSONMap{"components": map[string]any{
		"begin_0":   map[string]any{"obj": map[string]any{"component_name": "Begin", "params": map[string]any{}}, "downstream": []any{"message_0"}},
		"message_0": map[string]any{"obj": map[string]any{"component_name": "Message", "params": map[string]any{"text": "answer {{sys.query}}"}}, "upstream": []any{"begin_0"}},
	}, "path": []any{"begin_0", "message_0"}}
	if err := testDB.Create(&entity.UserCanvas{ID: "canvas-think", UserID: "user-1", DSL: dsl}).Error; err != nil {
		t.Fatal(err)
	}
	sqlDB, err := testDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	svc.activeSessions["session-think"] = &activeAgentRun{sessionID: "session-think"}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	results := make(chan error, 2)
	for _, question := range []string{"concurrent question 1", "concurrent question 2"} {
		go func(question string) {
			events, err := svc.RunAgent(ctx, "user-1", "canvas-think", "session-think", "", question, nil)
			if err == nil {
				for event := range events {
					if event.Type == "error" {
						err = errors.New(event.Data)
					}
				}
			}
			results <- err
		}(question)
	}
	wait := time.NewTicker(10 * time.Millisecond)
	defer wait.Stop()
	for {
		var questions int64
		if err := testDB.Model(&entity.API4ConversationMessage{}).Where("conversation_id = ? AND role = ?", "session-think", "user").Count(&questions).Error; err != nil {
			t.Fatal(err)
		}
		if questions == 3 {
			break
		}
		select {
		case err := <-results:
			t.Fatalf("queued request finished before the active run was released: %v", err)
		case <-ctx.Done():
			t.Fatal("concurrent questions were not saved immediately")
		case <-wait.C:
		}
	}
	svc.runMu.Lock()
	delete(svc.activeSessions, "session-think")
	svc.runMu.Unlock()
	for range 2 {
		if err := <-results; err != nil {
			t.Fatalf("concurrent run: %v", err)
		}
	}
	conv, err = dao.NewAPI4ConversationDAO().GetByID(ctx, testDB, "session-think")
	if err != nil {
		t.Fatal(err)
	}
	messages = parseMessages(conv.Message)
	if len(messages) != 6 || len(parseReferenceList(conv.Reference)) != 3 {
		t.Fatalf("concurrent turns were lost: messages=%s references=%s", conv.Message, conv.Reference)
	}
	for _, index := range []int{2, 4} {
		if messages[index]["role"] != "user" || messages[index+1]["role"] != "assistant" || messages[index]["id"] != messages[index+1]["id"] {
			t.Fatalf("answer is not associated with its question: %#v", messages[index:index+2])
		}
	}

}
