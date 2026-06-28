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
	"encoding/json"
	"testing"

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
