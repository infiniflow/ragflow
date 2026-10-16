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
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/agent/component"
	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"github.com/cloudwego/eino/schema"
	"github.com/redis/go-redis/v9"
)

func TestRunAgentExecutionPersistsDeferredThinking(t *testing.T) {
	content := runStubbedAgentCanvas(t, "deferred-one", map[string]string{
		"final answer": "reasoned from the final agent",
	}, agentCanvas(
		agentNode("agent_0", "final answer", []any{"message_0"}, []any{"begin_0"}),
		messageNode("message_0", "{{agent_0@content}}", nil, []any{"agent_0"}),
	))
	if content != "<think>reasoned from the final agent</think>final answer" {
		t.Fatalf("persisted content = %q", content)
	}
}

func TestRunAgentExecutionUsesSelectedBranchThinking(t *testing.T) {
	content := runStubbedAgentCanvas(t, "deferred-branches", map[string]string{
		"answer-a": "think-a",
		"answer-b": "think-b",
	}, agentCanvas(
		agentNode("agent_a", "answer-a", []any{"message_0"}, []any{"begin_0"}),
		agentNode("agent_b", "answer-b", []any{"message_1"}, []any{"begin_0"}),
		messageNode("message_0", "{{agent_a@content}}", nil, []any{"agent_a"}),
		messageNode("message_1", "{{agent_b@content}}", nil, []any{"agent_b"}),
	))
	if content != "<think>think-a</think>answer-a" {
		t.Fatalf("persisted content = %q, want the upstream thinking of message_0", content)
	}
}

func TestRunAgentExecutionKeepsEmptyDeferredThinking(t *testing.T) {
	content := runStubbedAgentCanvas(t, "deferred-empty", map[string]string{
		"earlier answer": "earlier thinking",
		"final answer":   "",
	}, agentCanvas(
		agentNode("agent_a", "earlier answer", []any{"agent_b"}, []any{"begin_0"}),
		agentNode("agent_b", "final answer", []any{"message_0"}, []any{"agent_a"}),
		messageNode("message_0", "{{agent_b@content}}", nil, []any{"agent_b"}),
	))
	if content != "final answer" {
		t.Fatalf("persisted content = %q, want the final agent's empty thinking", content)
	}
}

func TestRunAgentExecutionKeepsThinkingAcrossResume(t *testing.T) {
	var calls atomic.Int32
	restore := component.SetAgentRunnerForTest(func(_ context.Context, p component.AgentParam) (*schema.Message, error) {
		calls.Add(1)
		return &schema.Message{
			Role:             schema.Assistant,
			Content:          p.UserPrompt,
			ReasoningContent: "before interrupt",
		}, nil
	})
	t.Cleanup(restore)

	testDB := setupServiceTestDB(t)
	pushServiceDB(t, testDB)
	const (
		canvasID  = "canvas-resume-thinking"
		sessionID = "session-resume-thinking"
	)
	dsl := map[string]any{
		"components": map[string]any{
			"begin_0": beginNode([]any{"agent_0"}),
			"agent_0": map[string]any{
				"obj": map[string]any{"component_name": "Agent", "params": map[string]any{
					"model_id": "stub", "user_prompt": "resumed answer", "max_rounds": 1,
				}},
				"upstream":   []any{"begin_0"},
				"downstream": []any{"fillup"},
			},
			"fillup": map[string]any{
				"obj":        map[string]any{"component_name": "UserFillUp", "params": map[string]any{"enable_tips": true}},
				"upstream":   []any{"agent_0"},
				"downstream": []any{"message_0"},
			},
			"message_0": map[string]any{
				"obj":      map[string]any{"component_name": "Message", "params": map[string]any{"text": "{{agent_0@content}}"}},
				"upstream": []any{"fillup"},
			},
		},
		"path":  []any{"begin_0", "agent_0", "fillup", "message_0"},
		"graph": map[string]any{"nodes": []any{}, "edges": []any{}},
	}
	createAgentSession(t, canvasID, sessionID, dsl)
	svc := newCheckpointedAgentService(t)
	events := drainRunEvents(t)
	persistAgentQuestion(t, svc, canvasID, sessionID, "m1", "question")

	_, err := svc.buildRunFunc(canvasID, nil, dsl)(t.Context(), map[string]any{
		"__events__":     events,
		"__message_id__": "m1",
		"__session_id__": sessionID,
		"user_id":        "user-1",
		"user_input":     "question",
	})
	if !canvas.IsInterruptError(err) {
		t.Fatalf("first run error = %v, want interrupt", err)
	}
	resumeID := canvas.RootInterruptID(canvas.ExtractInterruptContexts(err))
	if resumeID == "" {
		t.Fatal("interrupt id is empty")
	}
	if calls.Load() != 1 {
		t.Fatalf("agent calls before resume = %d, want 1", calls.Load())
	}
	persistAgentQuestion(t, svc, canvasID, sessionID, "m2", "follow-up")

	state, err := svc.buildRunFunc(canvasID, nil, dsl)(t.Context(), map[string]any{
		"__events__":              events,
		"__message_id__":          "m2",
		"__session_id__":          sessionID,
		"user_id":                 "user-1",
		"user_input":              "follow-up",
		"__resume_interrupt_id__": resumeID,
		"__resume_data__":         "follow-up",
	})
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("agent calls after resume = %d, want 1 (the pre-interrupt agent must not rerun)", calls.Load())
	}
	sys, _, _ := state.SnapshotNamespaces()
	if _, ok := sys[agentRunStartedAtSysKey].(string); !ok {
		t.Fatal("resumed state lost the original run start")
	}
	got := lastAssistantContent(t, sessionID)
	if got != "<think>before interrupt</think>resumed answer" {
		t.Fatalf("persisted content = %q", got)
	}
}

func runStubbedAgentCanvas(t *testing.T, name string, thinkingByPrompt map[string]string, dsl map[string]any) string {
	t.Helper()
	restore := component.SetAgentRunnerForTest(func(_ context.Context, p component.AgentParam) (*schema.Message, error) {
		thinking, ok := thinkingByPrompt[p.UserPrompt]
		if !ok {
			t.Errorf("unexpected prompt %q", p.UserPrompt)
		}
		return &schema.Message{
			Role:             schema.Assistant,
			Content:          p.UserPrompt,
			ReasoningContent: thinking,
		}, nil
	})
	t.Cleanup(restore)

	testDB := setupServiceTestDB(t)
	pushServiceDB(t, testDB)
	canvasID := "canvas-" + name
	sessionID := "session-" + name
	createAgentSession(t, canvasID, sessionID, dsl)
	svc := NewAgentService()
	persistAgentQuestion(t, svc, canvasID, sessionID, "m1", "question")
	events := drainRunEvents(t)
	state, err := svc.buildRunFunc(canvasID, nil, dsl)(t.Context(), map[string]any{
		"__events__":     events,
		"__message_id__": "m1",
		"__session_id__": sessionID,
		"user_id":        "user-1",
		"user_input":     "question",
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if state == nil {
		t.Fatal("run returned nil state")
	}
	return lastAssistantContent(t, sessionID)
}

func agentCanvas(nodes ...map[string]any) map[string]any {
	components := map[string]any{
		"begin_0": beginNode(downstreamIDs(nodes)),
	}
	for _, node := range nodes {
		for id, record := range node {
			components[id] = record
		}
	}
	return map[string]any{
		"components": components,
		"path":       []any{"begin_0"},
		"graph":      map[string]any{"nodes": []any{}, "edges": []any{}},
	}
}

func downstreamIDs(nodes []map[string]any) []any {
	var ids []any
	for _, node := range nodes {
		for id, record := range node {
			rec, _ := record.(map[string]any)
			upstream, _ := rec["upstream"].([]any)
			for _, up := range upstream {
				if up == "begin_0" {
					ids = append(ids, id)
				}
			}
		}
	}
	return ids
}

func beginNode(downstream []any) map[string]any {
	return map[string]any{
		"obj":        map[string]any{"component_name": "Begin", "params": map[string]any{}},
		"downstream": downstream,
	}
}

func agentNode(id, prompt string, downstream, upstream []any) map[string]any {
	return map[string]any{
		id: map[string]any{
			"obj": map[string]any{"component_name": "Agent", "params": map[string]any{
				"model_id":    "stub",
				"user_prompt": prompt,
				"max_rounds":  1,
			}},
			"upstream":   upstream,
			"downstream": downstream,
		},
	}
}

func messageNode(id, text string, downstream, upstream []any) map[string]any {
	record := map[string]any{
		"obj":      map[string]any{"component_name": "Message", "params": map[string]any{"text": text}},
		"upstream": upstream,
	}
	if downstream != nil {
		record["downstream"] = downstream
	}
	return map[string]any{id: record}
}

func persistAgentQuestion(t *testing.T, svc *AgentService, canvasID, sessionID, messageID, text string) {
	t.Helper()
	if err := svc.persistAgentRunQuestion(t.Context(), canvasID, "user-1", sessionID, messageID, text, 1); err != nil {
		t.Fatalf("persist question: %v", err)
	}
}

func createAgentSession(t *testing.T, canvasID, sessionID string, dsl map[string]any) {
	t.Helper()
	if err := dao.NewAPI4ConversationDAO().Create(t.Context(), dao.DB, &entity.API4Conversation{
		ID:        sessionID,
		DialogID:  canvasID,
		UserID:    "user-1",
		Message:   json.RawMessage(`[]`),
		Reference: json.RawMessage(`[]`),
		DSL:       entity.JSONMap(dsl),
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
}

func newCheckpointedAgentService(t *testing.T) *AgentService {
	t.Helper()
	tracker, mr := newRunTrackerForTest(t, time.Hour)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewAgentServiceWithOptions(canvas.NewKvrocksCheckPointStoreWithClient(client, time.Hour), nil, tracker)
}

func drainRunEvents(t *testing.T) chan canvas.RunEvent {
	t.Helper()
	events := make(chan canvas.RunEvent, 32)
	go func() {
		for range events {
		}
	}()
	t.Cleanup(func() { close(events) })
	return events
}

func lastAssistantContent(t *testing.T, sessionID string) string {
	t.Helper()
	conv := getAPIConversationForTest(t, dao.DB, sessionID)
	var last string
	found := false
	for _, msg := range parseMessages(conv.Message) {
		if msg["role"] != "assistant" {
			continue
		}
		content, _ := msg["content"].(string)
		last = content
		found = true
	}
	if !found {
		t.Fatalf("session %s has no assistant message: %s", sessionID, conv.Message)
	}
	return last
}
