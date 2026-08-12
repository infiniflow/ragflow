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

// e2e integration tests for service.RunAgent covering the full
// production chain.
//
// These tests pin the production chain end-to-end: loadCanvasForUser
// → versionDAO.GetLatest → decodeCanvasFromDSL → canvas.Compile →
// cc.Workflow.Invoke → orchestrator answer extraction. They also
// cover the failure paths (compile error, invoke error). If any of
// these tests fails, the RunAgent path has regressed.
//
// The file's name changed from runagent_phase_4_4_v2_test.go
// because Phase 4.4 V2 is now a closed development phase (per
// gap-analysis v3.6.1) and the test surface has grown well
// beyond that scope.
//
// Test isolation: every test installs its own sqlDB (in-memory
// sqlite) and pushes it as dao.DB. Tests do NOT use t.Parallel()
// because they all touch the global RunAgent code path; isolation
// is per-test via fresh DB + fresh canvas rows, not goroutine
// parallelism.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"ragflow/internal/agent/canvas"
	_ "ragflow/internal/agent/component" // blank import: registers factories via component.init()
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/storage"
)

// makeCanvasWithDSL inserts a canvas + tenant + user + a published
// version whose DSL is the supplied map. The returned version id is
// what RunAgent consumes when the caller does not pass an explicit
// version (it falls through to GetLatest).
func makeCanvasWithDSL(t *testing.T, canvasID, userID, tenantID, versionID string, dsl map[string]any) {
	t.Helper()
	dao.DB.Create(&entity.User{ID: userID, Nickname: "owner", Email: userID + "@test.com"})
	dao.DB.Create(&entity.Tenant{ID: tenantID, Name: sptr(tenantID)})
	dao.DB.Create(&entity.UserTenant{ID: tenantID + "-" + userID, UserID: userID, TenantID: tenantID, Role: "owner", Status: sptr("1")})
	dao.DB.Create(&entity.UserCanvas{
		ID:             canvasID,
		UserID:         userID,
		Title:          sptr("Phase 4.4 V2 canvas"),
		Permission:     "me",
		CanvasType:     sptr("agent"),
		CanvasCategory: "agent_canvas",
	})
	dao.DB.Create(&entity.UserCanvasVersion{
		ID:           versionID,
		UserCanvasID: canvasID,
		Title:        sptr("v1"),
		DSL:          entity.JSONMap(dsl),
	})
}

// drainAgentEvents drains the channel returned by RunAgent and
// collects the typed events into the two buckets. The 5-second
// deadline protects against driver deadlocks — a successful run
// always closes the channel within milliseconds.
func drainAgentEvents(t *testing.T, events <-chan canvas.RunEvent) (messages []canvas.MessageEvent, errors_ []canvas.ErrorEvent, done bool) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				done = true
				return
			}
			switch ev.Type {
			case "message":
				var m canvas.MessageEvent
				if err := json.Unmarshal([]byte(ev.Data), &m); err != nil {
					t.Fatalf("drain: bad message payload: %v", err)
				}
				messages = append(messages, m)
			case "error":
				var e canvas.ErrorEvent
				if err := json.Unmarshal([]byte(ev.Data), &e); err != nil {
					t.Fatalf("drain: bad error payload: %v", err)
				}
				errors_ = append(errors_, e)
			case "done":
				done = true
			}
		case <-deadline:
			t.Fatal("RunAgent channel did not close within 5s — driver deadlocked?")
			return
		}
	}
}

func collectEventTypes(t *testing.T, events <-chan canvas.RunEvent) (types []string) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return types
			}
			types = append(types, ev.Type)
		case <-deadline:
			t.Fatal("RunAgent channel did not close within 5s — driver deadlocked?")
			return types
		}
	}
}

// TestRunAgent_RealCanvas_BeginMessage is the load-bearing happy-path
// test for Phase 4.4 V2. It publishes a 2-component DSL (Begin →
// Message where Message.text = "hello {{sys.query}}"), invokes
// RunAgent with user_input="world", and asserts the SSE surface
// emits one message whose Content is "hello world".
//
// This is what the V1 placeholder got wrong — its [V1 PLACEHOLDER]
// synthesised answer never reflected the actual template resolution
// path. If this test passes against the real Compile/Invoke, the
// production chain is no longer a placeholder.
func TestRunAgent_RealCanvas_BeginMessage(t *testing.T) {
	testDB := setupServiceTestDB(t)
	if err := testDB.AutoMigrate(
		&entity.UserCanvas{},
		&entity.UserCanvasVersion{},
		&entity.APIToken{},
		&entity.API4Conversation{},
		&entity.TenantModelProvider{},
		&entity.TenantModelInstance{},
		&entity.TenantModel{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	orig := dao.DB
	dao.DB = testDB
	t.Cleanup(func() { dao.DB = orig })

	dsl := map[string]any{
		"globals": map[string]any{"sys.files": []any{"stale file"}},
		"components": map[string]any{
			"begin_0": map[string]any{
				"obj": map[string]any{
					"component_name": "Begin",
					"params":         map[string]any{},
				},
				"downstream": []any{"message_0"},
			},
			"message_0": map[string]any{
				"obj": map[string]any{
					"component_name": "Message",
					"params":         map[string]any{"text": "hello {{sys.query}}"},
				},
				"upstream": []any{"begin_0"},
			},
		},
		"path": []any{"begin_0", "message_0"},
	}
	makeCanvasWithDSL(t, "canvas-hello", "user-1", "tenant-1", "v-hello", dsl)

	svc := NewAgentService()
	events, err := svc.RunAgent(
		context.Background(),
		"user-1",
		"canvas-hello",
		"session-hello",
		"", // latest version
		"world", nil)
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	messages, errs, done := drainAgentEvents(t, events)
	if len(errs) > 0 {
		t.Fatalf("unexpected error events: %+v", errs)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message event, got %d", len(messages))
	}
	if !strings.Contains(messages[0].Content, "hello world") {
		t.Errorf("Content = %q, want substring %q", messages[0].Content, "hello world")
	}
	if !done {
		t.Error("missing terminator done event")
	}
}

func TestRunAgent_SessionHistoryFeedsSysHistoryAndPersists(t *testing.T) {
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

	dsl := map[string]any{
		"globals": map[string]any{
			"sys.conversation_turns": 0,
			"sys.files":              []any{"existing file"},
			"sys.history":            []any{},
			"sys.user_id":            "",
		},
		"history": []any{},
		"memory":  []any{},
		"components": map[string]any{
			"begin_0": map[string]any{
				"obj": map[string]any{
					"component_name": "Begin",
					"params":         map[string]any{},
				},
				"downstream": []any{"history_0"},
			},
			"history_0": map[string]any{
				"obj": map[string]any{
					"component_name": "ListOperations",
					"params": map[string]any{
						"query":      "sys.history",
						"operations": "sort",
					},
				},
				"upstream":   []any{"begin_0"},
				"downstream": []any{"message_0"},
			},
			"message_0": map[string]any{
				"obj": map[string]any{
					"component_name": "Message",
					"params": map[string]any{
						"text": "{{history_0@result}}",
					},
				},
				"upstream": []any{"history_0"},
			},
		},
		"path": []any{"begin_0", "history_0", "message_0"},
	}
	makeCanvasWithDSL(t, "canvas-history", "user-1", "tenant-1", "v-history", dsl)
	if err := testDB.Create(&entity.API4Conversation{
		ID:        "session-history",
		DialogID:  "canvas-history",
		UserID:    "user-1",
		Message:   json.RawMessage(`[]`),
		Reference: json.RawMessage(`[]`),
	}).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}

	svc := NewAgentService()
	run := func(input string) canvas.MessageEvent {
		t.Helper()
		events, err := svc.RunAgent(context.Background(), "user-1", "canvas-history", "session-history", "", input, nil)
		if err != nil {
			t.Fatalf("RunAgent(%q): %v", input, err)
		}
		messages, errs, done := drainAgentEvents(t, events)
		if len(errs) != 0 || !done {
			t.Fatalf("RunAgent(%q): messages=%+v errors=%+v done=%v", input, messages, errs, done)
		}
		if len(messages) != 1 {
			t.Fatalf("RunAgent(%q): message count = %d, want 1", input, len(messages))
		}
		return messages[0]
	}

	first := run("hi")
	if first.Content != `["user: hi"]` {
		t.Fatalf("first content = %q, want JSON-rendered sys.history", first.Content)
	}
	var afterFirst entity.API4Conversation
	if err := testDB.Where("id = ?", "session-history").First(&afterFirst).Error; err != nil {
		t.Fatalf("reload session after first run: %v", err)
	}
	if components, ok := afterFirst.DSL["components"].(map[string]any); !ok || len(components) != 3 {
		t.Fatalf("persisted DSL lost runtime components: %#v", afterFirst.DSL)
	}
	second := run("again")
	var secondHistory []string
	if err := json.Unmarshal([]byte(second.Content), &secondHistory); err != nil {
		t.Fatalf("second content = %q, want a JSON string list: %v", second.Content, err)
	}
	if len(secondHistory) != 3 {
		t.Fatalf("second history = %#v, want assistant plus two user entries", secondHistory)
	}
	if !strings.HasPrefix(secondHistory[0], "assistant: ") ||
		!strings.Contains(secondHistory[0], `'content': '["user: hi"]'`) ||
		!strings.Contains(secondHistory[0], `'downloads': []`) {
		t.Fatalf("assistant history entry = %q, want persisted Message content and downloads", secondHistory[0])
	}
	if secondHistory[1] != "user: again" || secondHistory[2] != "user: hi" {
		t.Fatalf("sorted user history = %#v, want [user: again, user: hi]", secondHistory[1:])
	}

	var session entity.API4Conversation
	if err := testDB.Where("id = ?", "session-history").First(&session).Error; err != nil {
		t.Fatalf("reload session: %v", err)
	}
	history, ok := session.DSL["history"].([]any)
	if !ok || len(history) != 4 {
		t.Fatalf("persisted history = %#v, want four user/assistant entries", session.DSL["history"])
	}
	globals, _ := session.DSL["globals"].(map[string]any)
	sysHistory, ok := globals["sys.history"].([]any)
	if !ok || len(sysHistory) != 4 {
		t.Fatalf("persisted sys.history = %#v, want four rendered entries", globals["sys.history"])
	}
	if turns := globals["sys.conversation_turns"]; turns != 2 && turns != float64(2) {
		t.Fatalf("persisted sys.conversation_turns = %#v, want 2", turns)
	}
	if globals["sys.user_id"] != "user-1" {
		t.Fatalf("persisted sys.user_id = %#v, want user-1", globals["sys.user_id"])
	}
	files, ok := globals["sys.files"].([]any)
	if !ok || len(files) != 1 || files[0] != "existing file" {
		t.Fatalf("persisted sys.files = %#v, want existing file", globals["sys.files"])
	}
}

func TestRunAgent_NewSessionPersistsHistoryForNextTurn(t *testing.T) {
	testDB := setupServiceTestDB(t)
	if err := testDB.AutoMigrate(
		&entity.UserCanvas{},
		&entity.UserCanvasVersion{},
		&entity.API4Conversation{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	orig := dao.DB
	dao.DB = testDB
	t.Cleanup(func() { dao.DB = orig })

	dsl := map[string]any{
		"globals": map[string]any{"sys.history": []any{}},
		"history": []any{},
		"memory":  []any{},
		"components": map[string]any{
			"begin_0": map[string]any{
				"obj":        map[string]any{"component_name": "Begin", "params": map[string]any{}},
				"downstream": []any{"message_0"},
			},
			"message_0": map[string]any{
				"obj":      map[string]any{"component_name": "Message", "params": map[string]any{"text": "{{sys.history}}"}},
				"upstream": []any{"begin_0"},
			},
		},
		"path": []any{"begin_0", "message_0"},
	}
	makeCanvasWithDSL(t, "canvas-new-session", "user-1", "tenant-1", "v-new-session", dsl)

	svc := NewAgentService()
	events, err := svc.RunAgent(context.Background(), "user-1", "canvas-new-session", "", "", "1", nil)
	if err != nil {
		t.Fatalf("first RunAgent: %v", err)
	}
	firstMessages, errs, done := drainAgentEvents(t, events)
	if len(errs) != 0 || !done || len(firstMessages) != 1 {
		t.Fatalf("first run: messages=%+v errors=%+v done=%v", firstMessages, errs, done)
	}
	if firstMessages[0].Content != `["user: 1"]` {
		t.Fatalf("first content = %q", firstMessages[0].Content)
	}

	var session entity.API4Conversation
	if err := testDB.Where("dialog_id = ? AND user_id = ?", "canvas-new-session", "user-1").First(&session).Error; err != nil {
		t.Fatalf("new session was not persisted: %v", err)
	}
	if session.ID == "" {
		t.Fatal("persisted session has an empty ID")
	}

	events, err = svc.RunAgent(context.Background(), "user-1", "canvas-new-session", session.ID, "", "1", nil)
	if err != nil {
		t.Fatalf("second RunAgent: %v", err)
	}
	secondMessages, errs, done := drainAgentEvents(t, events)
	if len(errs) != 0 || !done || len(secondMessages) != 1 {
		t.Fatalf("second run: messages=%+v errors=%+v done=%v", secondMessages, errs, done)
	}
	var history []string
	if err := json.Unmarshal([]byte(secondMessages[0].Content), &history); err != nil {
		t.Fatalf("second content = %q: %v", secondMessages[0].Content, err)
	}
	if len(history) != 3 || history[0] != "user: 1" || !strings.HasPrefix(history[1], "assistant: ") || history[2] != "user: 1" {
		t.Fatalf("second history = %#v, want first user, assistant, current user", history)
	}
}

func TestRunAgent_RejectsSessionOwnedByAnotherUser(t *testing.T) {
	testDB := setupServiceTestDB(t)
	if err := testDB.AutoMigrate(
		&entity.UserCanvas{},
		&entity.UserCanvasVersion{},
		&entity.API4Conversation{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	orig := dao.DB
	dao.DB = testDB
	t.Cleanup(func() { dao.DB = orig })

	dsl := map[string]any{
		"components": map[string]any{
			"begin_0": map[string]any{
				"obj":        map[string]any{"component_name": "Begin", "params": map[string]any{}},
				"downstream": []any{"message_0"},
			},
			"message_0": map[string]any{
				"obj":      map[string]any{"component_name": "Message", "params": map[string]any{"text": "safe"}},
				"upstream": []any{"begin_0"},
			},
		},
		"path": []any{"begin_0", "message_0"},
	}
	makeCanvasWithDSL(t, "canvas-session-owner", "user-1", "tenant-1", "v-session-owner", dsl)
	foreignMessage := json.RawMessage(`[{"role":"assistant","content":"foreign"}]`)
	if err := testDB.Create(&entity.API4Conversation{
		ID:        "session-foreign",
		DialogID:  "canvas-session-owner",
		UserID:    "user-2",
		Message:   foreignMessage,
		Reference: json.RawMessage(`[]`),
		DSL:       entity.JSONMap(dsl),
	}).Error; err != nil {
		t.Fatalf("create foreign session: %v", err)
	}

	events, err := NewAgentService().RunAgent(
		context.Background(),
		"user-1",
		"canvas-session-owner",
		"session-foreign",
		"",
		"attempted overwrite",
		nil,
	)
	if err == nil {
		t.Fatal("RunAgent accepted a session owned by another user")
	}
	if events != nil {
		t.Fatalf("events = %#v, want nil for rejected session", events)
	}
	if !errors.Is(err, dao.ErrUserCanvasNotFound) {
		t.Fatalf("error = %v, want not-found authorization sentinel", err)
	}

	var unchanged entity.API4Conversation
	if err := testDB.Where("id = ?", "session-foreign").First(&unchanged).Error; err != nil {
		t.Fatalf("reload foreign session: %v", err)
	}
	if string(unchanged.Message) != string(foreignMessage) {
		t.Fatalf("foreign session message was overwritten: %s", unchanged.Message)
	}
}

// TestRunAgent_RealCanvas_GroupedParallelOuterFollower pins the grouped
// Parallel-subgraph compile/runtime path end-to-end. The grouped child
// nodes must stay inside the Parallel macro body, while the outer Message
// follower must remain outside and consume the Parallel node's output.
func TestRunAgent_RealCanvas_GroupedParallelOuterFollower(t *testing.T) {
	testDB := setupServiceTestDB(t)
	if err := testDB.AutoMigrate(
		&entity.UserCanvas{},
		&entity.UserCanvasVersion{},
		&entity.APIToken{},
		&entity.API4Conversation{},
		&entity.TenantModelProvider{},
		&entity.TenantModelInstance{},
		&entity.TenantModel{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	orig := dao.DB
	dao.DB = testDB
	t.Cleanup(func() { dao.DB = orig })

	dsl := map[string]any{
		"components": map[string]any{
			"begin": map[string]any{
				"obj": map[string]any{
					"component_name": "Begin",
					"params":         map[string]any{},
				},
				"downstream": []any{"split"},
			},
			"split": map[string]any{
				"obj": map[string]any{
					"component_name": "StringTransform",
					"params": map[string]any{
						"method":     "split",
						"split_ref":  "sys.query",
						"delimiters": []any{","},
					},
				},
				"downstream": []any{"parallel"},
				"upstream":   []any{"begin"},
			},
			"parallel": map[string]any{
				"obj": map[string]any{
					"component_name": "Parallel",
					"params": map[string]any{
						"items_ref": "split@result",
						"outputs": map[string]any{
							"lines": map[string]any{
								"ref": "fmt@result",
							},
						},
					},
				},
				"downstream": []any{"done"},
				"upstream":   []any{"split"},
			},
			"iter_start": map[string]any{
				"obj": map[string]any{
					"component_name": "IterationItem",
					"params":         map[string]any{},
				},
				"downstream": []any{"fmt"},
				"upstream":   []any{"parallel"},
				"parent_id":  "parallel",
			},
			"fmt": map[string]any{
				"obj": map[string]any{
					"component_name": "StringTransform",
					"params": map[string]any{
						"method":     "merge",
						"script":     "{{item}}",
						"delimiters": []any{"|"},
					},
				},
				"upstream":  []any{"iter_start"},
				"parent_id": "parallel",
			},
			"done": map[string]any{
				"obj": map[string]any{
					"component_name": "Message",
					"params":         map[string]any{"content": []any{"{parallel@lines}"}},
				},
				"upstream": []any{"parallel"},
			},
		},
		"path": []any{"begin", "split", "parallel", "done"},
		"graph": map[string]any{
			"nodes": []any{
				map[string]any{"id": "iter_start", "parentId": "parallel"},
				map[string]any{"id": "fmt", "parentId": "parallel"},
			},
		},
	}
	makeCanvasWithDSL(t, "canvas-parallel", "user-1", "tenant-1", "v-parallel", dsl)

	svc := NewAgentService()
	events, err := svc.RunAgent(
		context.Background(),
		"user-1",
		"canvas-parallel",
		"session-parallel",
		"",
		"a,b,c", nil)
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	messages, errs, done := drainAgentEvents(t, events)
	if len(errs) > 0 {
		t.Fatalf("unexpected error events: %+v", errs)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message event, got %d", len(messages))
	}
	if !strings.Contains(messages[0].Content, "a") || !strings.Contains(messages[0].Content, "b") || !strings.Contains(messages[0].Content, "c") {
		t.Fatalf("parallel outer follower content = %q, want ordered item output", messages[0].Content)
	}
	if !done {
		t.Error("missing terminator done event")
	}
}

// TestRunAgent_RealCanvas_CompileFails pins the schema-failure
// branch: when the DSL references a component name that is not
// registered against runtime.DefaultFactory, canvas.Compile fails
// (buildNodeBody returns 'factory: component: unknown component'),
// and RunAgent must surface that as a wrapped ErrAgentStorageError
// so mapAgentError classifies it as CodeServerError (500) with a
// sanitized message — NOT the raw build error string.
func TestRunAgent_RealCanvas_CompileFails(t *testing.T) {
	testDB := setupServiceTestDB(t)
	if err := testDB.AutoMigrate(
		&entity.UserCanvas{},
		&entity.UserCanvasVersion{},
		&entity.APIToken{},
		&entity.API4Conversation{},
		&entity.TenantModelProvider{},
		&entity.TenantModelInstance{},
		&entity.TenantModel{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	orig := dao.DB
	dao.DB = testDB
	t.Cleanup(func() { dao.DB = orig })

	dsl := map[string]any{
		"components": map[string]any{
			"begin_0": map[string]any{
				"obj": map[string]any{
					"component_name": "Begin",
					"params":         map[string]any{},
				},
				"downstream": []any{"bogus_0"},
			},
			"bogus_0": map[string]any{
				"obj": map[string]any{
					"component_name": "NonExistentComponent",
					"params":         map[string]any{},
				},
			},
		},
		"path": []any{"begin_0", "bogus_0"},
	}
	makeCanvasWithDSL(t, "canvas-bogus", "user-1", "tenant-1", "v-bogus", dsl)

	svc := NewAgentService()
	events, err := svc.RunAgent(
		context.Background(),
		"user-1",
		"canvas-bogus",
		"session-bogus",
		"",
		"hello", nil)
	if err != nil {
		t.Fatalf("RunAgent returned sync error: %v", err)
	}
	_, errs, _ := drainAgentEvents(t, events)
	if len(errs) == 0 {
		t.Fatal("expected error event from Compile of DSL with unknown component name")
	}
	// The error message should mention ErrAgentStorageError but NOT
	// contain the raw factory error substring (sanitised at the
	// service layer). The factory error is wrapped inside the
	// buildNodeBody / BuildWorkflow chain — its full text is
	// preserved for the logs but not echoed as the SSE message.
	if !strings.Contains(errs[0].Message, "agent storage error") {
		t.Errorf("error message %q does not mention sanitised label", errs[0].Message)
	}
}

// TestRunAgent_RealCanvas_InvokeFails pins the runtime-failure
// branch: a DSL that compiles cleanly (registry is happy) but
// fails at runtime — using a DataOperations component whose query
// ref hits the GetVar "invalid variable reference" default branch
// (no @, not sys.X / env.X / item / index). DataOperations.Invoke
// propagates the wrapped error and the workflow terminates with
// an error event.
//
// Note: we deliberately do NOT use a Message with an unresolvable
// ref here. Message is a display node that now uses the tolerant
// ResolveTemplateForDisplay (renders nil refs as empty string,
// matching the Python canvas.py soft-fail). Parameter-binding
// sites keep runtime.ResolveTemplate's loud-fail contract.
//
// A component invocation error must retain its invoke context without being
// misclassified as an agent storage failure.
func TestRunAgent_RealCanvas_InvokeFails(t *testing.T) {
	testDB := setupServiceTestDB(t)
	if err := testDB.AutoMigrate(
		&entity.UserCanvas{},
		&entity.UserCanvasVersion{},
		&entity.APIToken{},
		&entity.API4Conversation{},
		&entity.TenantModelProvider{},
		&entity.TenantModelInstance{},
		&entity.TenantModel{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	orig := dao.DB
	dao.DB = testDB
	t.Cleanup(func() { dao.DB = orig })

	dsl := map[string]any{
		"components": map[string]any{
			"begin_0": map[string]any{
				"obj": map[string]any{
					"component_name": "Begin",
					"params":         map[string]any{},
				},
				"downstream": []any{"data_ops_0"},
			},
			"data_ops_0": map[string]any{
				"obj": map[string]any{
					"component_name": "DataOperations",
					// "this is not a valid ref" — no @, no sys./env.
					// prefix, not item/index. state.GetVar returns the
					// "invalid variable reference" error and
					// DataOperations.Invoke propagates it.
					"params": map[string]any{
						"operations": "literal_eval",
						"query":      []any{"this is not a valid ref"},
					},
				},
				"upstream": []any{"begin_0"},
			},
		},
		"path": []any{"begin_0", "data_ops_0"},
	}
	makeCanvasWithDSL(t, "canvas-invoke-fail", "user-1", "tenant-1", "v-invoke-fail", dsl)

	svc := NewAgentService()
	events, err := svc.RunAgent(
		context.Background(),
		"user-1",
		"canvas-invoke-fail",
		"session-invoke-fail",
		"",
		"hello", nil)
	if err != nil {
		t.Fatalf("RunAgent returned sync error: %v", err)
	}
	_, errs, _ := drainAgentEvents(t, events)
	if len(errs) == 0 {
		t.Fatal("expected error event from Invoke of DSL with bad component query ref")
	}
	if !strings.Contains(errs[0].Message, "canvas invoke:") {
		t.Errorf("error message %q does not mention invoke context", errs[0].Message)
	}
	if strings.Contains(errs[0].Message, "agent storage error") {
		t.Errorf("error message %q misclassifies invoke failure as storage failure", errs[0].Message)
	}
}

// TestRunAgent_FilesPopulateIteration verifies the full upload object ->
// sys.files -> Parallel/Iteration item path.
func TestRunAgent_FilesPopulateIteration(t *testing.T) {
	ctx := t.Context()
	testDB := setupServiceTestDB(t)
	if err := testDB.AutoMigrate(
		&entity.UserCanvas{},
		&entity.UserCanvasVersion{},
		&entity.UserTenant{},
		&entity.APIToken{},
		&entity.API4Conversation{},
		&entity.TenantModelProvider{},
		&entity.TenantModelInstance{},
		&entity.TenantModel{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	orig := dao.DB
	dao.DB = testDB
	t.Cleanup(func() { dao.DB = orig })

	canvasID := "canvas-files-e2e"
	sessionID := "session-files-e2e"
	versionID := "v-files-e2e"
	memory := storage.NewMemoryStorage()
	if err := memory.Put(ctx, "user-1-downloads", "upload-1", []byte("iteration payload")); err != nil {
		t.Fatalf("put upload: %v", err)
	}
	factory := storage.GetStorageFactory()
	originalStorage := factory.GetStorage()
	factory.SetStorage(memory)
	t.Cleanup(func() { factory.SetStorage(originalStorage) })

	dsl := map[string]any{
		"globals": map[string]any{
			"sys.files":   []any{},
			"sys.user_id": "",
		},
		"components": map[string]any{
			"begin_0": map[string]any{
				"obj": map[string]any{
					"component_name": "Begin",
					"params":         map[string]any{"layout_recognize": "Plain Text"},
				},
				"downstream": []any{"parallel_0"},
			},
			"parallel_0": map[string]any{
				"obj": map[string]any{
					"component_name": "Parallel",
					"params": map[string]any{
						"items_ref": "sys.files",
						"outputs": map[string]any{
							"lines": map[string]any{"ref": "format_0@result"},
						},
					},
				},
				"upstream":   []any{"begin_0"},
				"downstream": []any{"message_0"},
			},
			"iteration_item_0": map[string]any{
				"obj": map[string]any{
					"component_name": "IterationItem",
					"params":         map[string]any{},
				},
				"upstream":   []any{"parallel_0"},
				"downstream": []any{"format_0"},
				"parent_id":  "parallel_0",
			},
			"format_0": map[string]any{
				"obj": map[string]any{
					"component_name": "StringTransform",
					"params": map[string]any{
						"method":     "merge",
						"script":     "{item}",
						"delimiters": []any{"|"},
					},
				},
				"upstream":  []any{"iteration_item_0"},
				"parent_id": "parallel_0",
			},
			"message_0": map[string]any{
				"obj": map[string]any{
					"component_name": "Message",
					"params":         map[string]any{"content": []any{"{parallel_0@lines}"}},
				},
				"upstream": []any{"parallel_0"},
			},
		},
		"path": []any{"begin_0", "parallel_0", "message_0"},
		"graph": map[string]any{
			"nodes": []any{
				map[string]any{"id": "iteration_item_0", "parentId": "parallel_0"},
				map[string]any{"id": "format_0", "parentId": "parallel_0"},
			},
		},
	}
	makeCanvasWithDSL(t, canvasID, "user-1", "tenant-1", versionID, dsl)

	testFiles := []map[string]interface{}{
		{
			"id":         "upload-1",
			"name":       "notes.txt",
			"mime_type":  "text/plain",
			"created_by": "user-1",
		},
	}

	svc := NewAgentService()
	events, err := svc.RunAgent(
		ctx,
		"user-1",
		canvasID,
		sessionID,
		"",
		"hello-files", testFiles)
	if err != nil {
		t.Fatalf("RunAgent with files: %v", err)
	}
	messages, errs, done := drainAgentEvents(t, events)
	if len(errs) > 0 {
		t.Fatalf("unexpected error events with files: %+v", errs)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message event with files, got %d", len(messages))
	}
	if !strings.Contains(messages[0].Content, "File: notes.txt") || !strings.Contains(messages[0].Content, "iteration payload") {
		t.Errorf("Content = %q, want parsed upload from iteration", messages[0].Content)
	}
	if !done {
		t.Error("missing terminator done event with files")
	}
}

func TestRunAgent_MissingUploadEmitsError(t *testing.T) {
	testDB := setupServiceTestDB(t)
	if err := testDB.AutoMigrate(
		&entity.UserCanvas{},
		&entity.UserCanvasVersion{},
		&entity.UserTenant{},
		&entity.APIToken{},
		&entity.API4Conversation{},
		&entity.TenantModelProvider{},
		&entity.TenantModelInstance{},
		&entity.TenantModel{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	origDB := dao.DB
	dao.DB = testDB
	t.Cleanup(func() { dao.DB = origDB })

	memory := storage.NewMemoryStorage()
	factory := storage.GetStorageFactory()
	originalStorage := factory.GetStorage()
	factory.SetStorage(memory)
	t.Cleanup(func() { factory.SetStorage(originalStorage) })

	dsl := map[string]any{
		"components": map[string]any{
			"begin": map[string]any{
				"obj":        map[string]any{"component_name": "Begin", "params": map[string]any{}},
				"downstream": []any{"message"},
			},
			"message": map[string]any{
				"obj":      map[string]any{"component_name": "Message", "params": map[string]any{"text": "should not run"}},
				"upstream": []any{"begin"},
			},
		},
		"path": []any{"begin", "message"},
	}
	makeCanvasWithDSL(t, "canvas-missing-upload", "user-1", "tenant-1", "v-missing-upload", dsl)

	events, err := NewAgentService().RunAgent(
		context.Background(),
		"user-1",
		"canvas-missing-upload",
		"session-missing-upload",
		"",
		"hello",
		[]map[string]interface{}{{
			"id":         "missing",
			"name":       "missing.txt",
			"mime_type":  "text/plain",
			"created_by": "user-1",
		}},
	)
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	messages, errs, done := drainAgentEvents(t, events)
	if len(messages) != 0 {
		t.Fatalf("messages = %+v, want none", messages)
	}
	if len(errs) != 1 || !strings.Contains(errs[0].Message, "parse agent files") {
		t.Fatalf("errors = %+v, want parse agent files error", errs)
	}
	if !done {
		t.Fatal("missing done event")
	}
}

// TestRunAgent_NoFilesRunsNormally verifies that the files-aware
// RunAgent path does not regress when no files are passed (nil
// parameter). This is a counterpart to the existing
// TestRunAgent_RealCanvas_BeginMessage to ensure backward
// compatibility.
func TestRunAgent_NoFilesRunsNormally(t *testing.T) {
	testDB := setupServiceTestDB(t)
	if err := testDB.AutoMigrate(
		&entity.UserCanvas{},
		&entity.UserCanvasVersion{},
		&entity.UserTenant{},
		&entity.APIToken{},
		&entity.API4Conversation{},
		&entity.TenantModelProvider{},
		&entity.TenantModelInstance{},
		&entity.TenantModel{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	orig := dao.DB
	dao.DB = testDB
	t.Cleanup(func() { dao.DB = orig })

	canvasID := "canvas-nofiles-e2e"
	sessionID := "session-nofiles-e2e"
	versionID := "v-nofiles-e2e"

	dsl := map[string]any{
		"components": map[string]any{
			"begin_0": map[string]any{
				"obj": map[string]any{
					"component_name": "Begin",
					"params":         map[string]any{},
				},
				"downstream": []any{"message_0"},
			},
			"message_0": map[string]any{
				"obj": map[string]any{
					"component_name": "Message",
					"params":         map[string]any{"text": "echo: {{sys.query}}"},
				},
				"upstream": []any{"begin_0"},
			},
		},
		"path": []any{"begin_0", "message_0"},
	}
	makeCanvasWithDSL(t, canvasID, "user-1", "tenant-1", versionID, dsl)

	svc := NewAgentService()
	events, err := svc.RunAgent(
		context.Background(),
		"user-1",
		canvasID,
		sessionID,
		"",
		"hello-nofiles", nil)
	if err != nil {
		t.Fatalf("RunAgent without files: %v", err)
	}
	messages, errs, done := drainAgentEvents(t, events)
	if len(errs) > 0 {
		t.Fatalf("unexpected error events without files: %+v", errs)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message event without files, got %d", len(messages))
	}
	if !strings.Contains(messages[0].Content, "hello-nofiles") {
		t.Errorf("Content = %q, want substring %q", messages[0].Content, "hello-nofiles")
	}
	if !done {
		t.Error("missing terminator done event without files")
	}
}
