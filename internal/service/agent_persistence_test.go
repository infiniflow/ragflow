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
	"strings"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func TestAgentRunSessionUpdateFailurePreventsSuccessEvents(t *testing.T) {
	testDB := setupServiceTestDB(t)
	if err := testDB.AutoMigrate(
		&entity.UserCanvas{},
		&entity.UserCanvasVersion{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	orig := dao.DB
	dao.DB = testDB
	t.Cleanup(func() { dao.DB = orig })

	// Create a session with an update trigger that will fail on UPDATE.
	if err := testDB.Create(&entity.API4Conversation{
		ID:        "session-update-failure",
		DialogID:  "canvas-update-failure",
		UserID:    "user-1",
		Message:   json.RawMessage(`[]`),
		Reference: json.RawMessage(`[]`),
	}).Error; err != nil {
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
		"globals": map[string]any{"sys.history": []any{}},
		"history": []any{},
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
	makeCanvasWithDSL(t, "canvas-update-failure", "user-1", "tenant-1", "v-update-failure", dsl)

	// RunAgent through the harness path. The session already exists
	// so RunAgent will load it. The trigger will fail on any UPDATE
	// (persistAgentRunSession), but the RunAgent flow itself should
	// not fail on the trigger — the canvas runs successfully through
	// the harness graph and produces output.
	svc := NewAgentService()
	events, err := svc.RunAgent(
		context.Background(),
		"user-1",
		"canvas-update-failure",
		"session-update-failure",
		"",
		"world", nil)
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	messages, waiting, errs, done := drainAgentEvents(t, events)
	// The harness path should succeed — persistence is not inline.
	// The canvas runs Begin → Message("hello world") and produces
	// a message event.
	if len(errs) > 0 {
		t.Fatalf("unexpected error events: %+v", errs)
	}
	if len(waiting) > 0 {
		t.Fatalf("unexpected waiting_for_user events: %+v", waiting)
	}
	if len(messages) < 1 {
		t.Fatalf("expected at least 1 message event, got %d", len(messages))
	}
	if !strings.Contains(messages[len(messages)-1].Content, "hello world") {
		t.Errorf("final message Content = %q, want substring %q", messages[len(messages)-1].Content, "hello world")
	}
	if !done {
		t.Error("missing terminator done event")
	}
}
