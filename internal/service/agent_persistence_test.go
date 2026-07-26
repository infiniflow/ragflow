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
