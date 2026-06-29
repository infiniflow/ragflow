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
// cover the boot-wiring surface (Redis-backed CheckPointStore +
// RunTracker) and the failure paths (compile error, invoke
// error, wait-for-user resume cycle). If any of these tests
// fails, the RunAgent path has regressed.
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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/agent/component"
	_ "ragflow/internal/agent/component" // blank import: registers factories via component.init()
	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
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
// collects the typed events into the three buckets. The 5-second
// deadline protects against driver deadlocks — a successful run
// always closes the channel within milliseconds.
func drainAgentEvents(t *testing.T, events <-chan canvas.RunEvent) (messages []canvas.MessageEvent, waiting []canvas.WaitingForUserEvent, errors_ []canvas.ErrorEvent, done bool) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return
			}
			switch ev.Type {
			case "message":
				var m canvas.MessageEvent
				if err := json.Unmarshal([]byte(ev.Data), &m); err != nil {
					t.Fatalf("drain: bad message payload: %v", err)
				}
				messages = append(messages, m)
			case "waiting_for_user":
				var w canvas.WaitingForUserEvent
				if err := json.Unmarshal([]byte(ev.Data), &w); err != nil {
					t.Fatalf("drain: bad waiting_for_user payload: %v", err)
				}
				waiting = append(waiting, w)
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
		&entity.User{},
		&entity.Tenant{},
		&entity.UserTenant{},
		&entity.UserCanvas{},
		&entity.UserCanvasVersion{},
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
		"world",
	)
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	messages, waiting, errs, done := drainAgentEvents(t, events)
	if len(errs) > 0 {
		t.Fatalf("unexpected error events: %+v", errs)
	}
	if len(waiting) > 0 {
		t.Fatalf("unexpected waiting_for_user events: %+v", waiting)
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

// TestRunAgent_RealCanvas_WaitForUserResume pins the resume path.
// It publishes a 3-component DSL (Begin → Message → UserFillUp),
// invokes RunAgent twice on the same (canvas, session), and asserts:
//
//	Run 1: UserFillUp emits a wait-for-user interrupt; the Runner
//	       emits a waiting_for_user SSE event.
//	Run 2: with user_input="my follow-up", the Runner injects the
//	       saved interrupt id, buildRunFunc decorates ctx with
//	       compose.ResumeWithData, UserFillUp resumes and emits
//	       the user's follow-up as its output, Message renders
//	       the answer.
//
// This is the regression test that the plan §6.2 demanded. Without
// it, the resume path could silently regress to "every resume
// starts a fresh Workflow.Invoke" — a bug the V1 placeholder got
// right by accident because it never actually invoked the workflow.
func TestRunAgent_RealCanvas_WaitForUserResume(t *testing.T) {
	testDB := setupServiceTestDB(t)
	if err := testDB.AutoMigrate(
		&entity.User{},
		&entity.Tenant{},
		&entity.UserTenant{},
		&entity.UserCanvas{},
		&entity.UserCanvasVersion{},
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
				"downstream": []any{"message_0"},
			},
			"message_0": map[string]any{
				"obj": map[string]any{
					"component_name": "Message",
					"params":         map[string]any{"text": "got: {{user_fill_up_0@user_input}}"},
				},
				"upstream": []any{"begin_0", "user_fill_up_0"},
			},
			"user_fill_up_0": map[string]any{
				"obj": map[string]any{
					"component_name": "UserFillUp",
					"params":         map[string]any{"enable_tips": true},
				},
				"downstream": []any{"message_0"},
			},
		},
		"path": []any{"begin_0", "user_fill_up_0", "message_0"},
	}
	makeCanvasWithDSL(t, "canvas-fillup", "user-1", "tenant-1", "v-fillup", dsl)

	// v3.6.1 Gap #1 fix: a real checkpoint store + serializer is
	// REQUIRED for the second Invoke to actually resume from
	// run 1's saved state. Without these, buildRunFunc's
	// `cpID = runID` branch is dead and compose.WithCheckPointID
	// is never passed to eino, so the second Invoke starts a
	// fresh execution that re-enters UserFillUp from scratch
	// (compose.GetResumeContext returns isResume=false on a
	// non-resume run, which is the correct eino behaviour but
	// the wrong assumption for our test). The minimal
	// eino-only repro at /tmp/eino-repro/repro_test.go confirms
	// eino's resume API works correctly when given a real
	// CheckPointStore + CheckPointID.
	tracker, mr := newRunTrackerForTest(t, 30*24*time.Hour)
	cpClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = cpClient.Close() })
	cp := canvas.NewRedisCheckPointStoreWithClient(cpClient, 30*24*time.Hour)
	// stateSerializer is intentionally nil so eino's default
	// InternalSerializer is used (which knows about CanvasState
	// via runtime.RegisterSerializableType[CanvasState]). The
	// RAGFlow plain-JSON CanvasStateSerializer is incompatible
	// with eino's internal checkpoint format — see cmd/server_main.go
	// buildAgentRunOptions for the production rationale.
	svc := NewAgentServiceWithOptions(cp, nil, tracker)

	// Run 1: should emit a waiting_for_user event.
	events1, err := svc.RunAgent(
		context.Background(),
		"user-1",
		"canvas-fillup",
		"session-fillup",
		"",
		"please ask",
	)
	if err != nil {
		t.Fatalf("RunAgent run 1: %v", err)
	}
	_, waiting1, errs1, _ := drainAgentEvents(t, events1)
	if len(errs1) > 0 {
		t.Fatalf("run 1: unexpected error events: %+v", errs1)
	}
	if len(waiting1) != 1 {
		t.Fatalf("run 1: expected 1 waiting_for_user event, got %d", len(waiting1))
	}
	if waiting1[0].CpnID == "" {
		t.Error("run 1: waiting_for_user event has empty cpn_id")
	}

	// Run 2: with the checkpoint store wired, eino loads the
	// saved state from run 1, the engine targets the UserFillUp
	// node via compose.ResumeWithData, UserFillUp returns the
	// resume data, and the downstream Message renders
	// "got: my follow-up". The previous v3.5.2 / v3.6 / v3.6.1
	// gap analyses misclassified this as an "eino limitation"
	// — the actual cause was the test running without the
	// production environment's checkpoint store.
	events2, err := svc.RunAgent(
		context.Background(),
		"user-1",
		"canvas-fillup",
		"session-fillup", // SAME sessionID as run 1
		"",
		"my follow-up",
	)
	if err != nil {
		t.Fatalf("RunAgent run 2: %v", err)
	}
	messages2, waiting2, errs2, done2 := drainAgentEvents(t, events2)
	if len(errs2) > 0 {
		t.Fatalf("run 2: unexpected error events: %+v", errs2)
	}
	if len(waiting2) > 0 {
		t.Errorf("run 2: did NOT expect a second waiting_for_user event; got %+v", waiting2)
	}
	if !done2 {
		t.Error("run 2: missing terminator done event")
	}
	if len(messages2) != 1 {
		t.Fatalf("run 2: expected 1 message event after resume, got %d", len(messages2))
	}
	if !strings.Contains(messages2[0].Content, "got: my follow-up") {
		t.Errorf("run 2: Content = %q, want substring %q", messages2[0].Content, "got: my follow-up")
	}
}

func TestRunAgent_RealCanvas_WaitForUserResume_EventSemantics(t *testing.T) {
	testDB := setupServiceTestDB(t)
	if err := testDB.AutoMigrate(
		&entity.User{},
		&entity.Tenant{},
		&entity.UserTenant{},
		&entity.UserCanvas{},
		&entity.UserCanvasVersion{},
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
				"downstream": []any{"user_fill_up_0"},
			},
			"user_fill_up_0": map[string]any{
				"obj": map[string]any{
					"component_name": "UserFillUp",
					"params":         map[string]any{"enable_tips": true},
				},
				"downstream": []any{"message_0"},
			},
			"message_0": map[string]any{
				"obj": map[string]any{
					"component_name": "Message",
					"params":         map[string]any{"text": "got: {{user_fill_up_0@user_input}}"},
				},
				"upstream": []any{"begin_0", "user_fill_up_0"},
			},
		},
		"path": []any{"begin_0", "user_fill_up_0", "message_0"},
	}
	makeCanvasWithDSL(t, "canvas-fillup-events", "user-1", "tenant-1", "v-fillup-events", dsl)

	tracker, mr := newRunTrackerForTest(t, 30*24*time.Hour)
	cpClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = cpClient.Close() })
	cp := canvas.NewRedisCheckPointStoreWithClient(cpClient, 30*24*time.Hour)
	svc := NewAgentServiceWithOptions(cp, nil, tracker)

	events1, err := svc.RunAgent(
		context.Background(),
		"user-1",
		"canvas-fillup-events",
		"session-fillup-events",
		"",
		"please ask",
	)
	if err != nil {
		t.Fatalf("RunAgent run 1: %v", err)
	}
	types1 := collectEventTypes(t, events1)
	if len(types1) == 0 || types1[0] != "workflow_started" {
		t.Fatalf("run 1: first event = %v, want workflow_started", types1)
	}
	for _, typ := range types1 {
		if typ == "workflow_finished" {
			t.Fatalf("run 1: unexpected workflow_finished before wait-for-user, events=%v", types1)
		}
	}
	if len(types1) == 0 || types1[len(types1)-2] != "waiting_for_user" || types1[len(types1)-1] != "done" {
		t.Fatalf("run 1: tail events = %v, want ... waiting_for_user, done", types1)
	}

	events2, err := svc.RunAgent(
		context.Background(),
		"user-1",
		"canvas-fillup-events",
		"session-fillup-events",
		"",
		"my follow-up",
	)
	if err != nil {
		t.Fatalf("RunAgent run 2: %v", err)
	}
	types2 := collectEventTypes(t, events2)
	for _, typ := range types2 {
		if typ == "workflow_started" {
			t.Fatalf("run 2: unexpected workflow_started on resume, events=%v", types2)
		}
	}
	if len(types2) == 0 || types2[len(types2)-2] != "workflow_finished" || types2[len(types2)-1] != "done" {
		t.Fatalf("run 2: tail events = %v, want ... workflow_finished, done", types2)
	}
}

// TestRunAgent_RealCanvas_GroupedParallelOuterFollower pins the grouped
// Parallel-subgraph compile/runtime path end-to-end. The grouped child
// nodes must stay inside the Parallel macro body, while the outer Message
// follower must remain outside and consume the Parallel node's output.
func TestRunAgent_RealCanvas_GroupedParallelOuterFollower(t *testing.T) {
	testDB := setupServiceTestDB(t)
	if err := testDB.AutoMigrate(
		&entity.User{},
		&entity.Tenant{},
		&entity.UserTenant{},
		&entity.UserCanvas{},
		&entity.UserCanvasVersion{},
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
		"a,b,c",
	)
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	messages, waiting, errs, done := drainAgentEvents(t, events)
	if len(errs) > 0 {
		t.Fatalf("unexpected error events: %+v", errs)
	}
	if len(waiting) > 0 {
		t.Fatalf("unexpected waiting_for_user events: %+v", waiting)
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

// TestRunAgent_AllFixture_LoopInterruptResume drives the real all.json
// fixture through the production RunAgent interrupt/resume path:
//
//  1. First input "loop" is consumed directly by UserFillUp:Menu,
//     routes through Switch:Route into Loop:InputUntil1, and pauses at
//     UserFillUp:LoopInput.
//  2. Second input "1" resumes LoopInput, satisfies Switch:LoopCheck's
//     exit condition, and the workflow finishes at Message:LoopDone.
func TestRunAgent_AllFixture_LoopInterruptResume(t *testing.T) {
	testDB := setupServiceTestDB(t)
	if err := testDB.AutoMigrate(
		&entity.User{},
		&entity.Tenant{},
		&entity.UserTenant{},
		&entity.UserCanvas{},
		&entity.UserCanvasVersion{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	orig := dao.DB
	dao.DB = testDB
	t.Cleanup(func() { dao.DB = orig })

	raw, err := os.ReadFile(filepath.Join("..", "agent", "dsl", "testdata", "all.json"))
	if err != nil {
		t.Fatalf("read all.json: %v", err)
	}
	var dsl map[string]any
	if err := json.Unmarshal(raw, &dsl); err != nil {
		t.Fatalf("parse all.json: %v", err)
	}
	makeCanvasWithDSL(t, "canvas-all", "user-1", "tenant-1", "v-all", dsl)

	tracker, mr := newRunTrackerForTest(t, 30*24*time.Hour)
	cpClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = cpClient.Close() })
	cp := canvas.NewRedisCheckPointStoreWithClient(cpClient, 30*24*time.Hour)
	svc := NewAgentServiceWithOptions(cp, nil, tracker)

	events1, err := svc.RunAgent(
		context.Background(),
		"user-1",
		"canvas-all",
		"session-all-loop",
		"",
		"loop",
	)
	if err != nil {
		t.Fatalf("RunAgent run 1: %v", err)
	}
	messages1, waiting1, errs1, done1 := drainAgentEvents(t, events1)
	if len(errs1) > 0 {
		t.Fatalf("run 1: unexpected error events: %+v", errs1)
	}
	if len(messages1) != 0 {
		t.Fatalf("run 1: expected 0 message events before resume, got %d", len(messages1))
	}
	if len(waiting1) != 1 {
		t.Fatalf("run 1: expected 1 waiting_for_user event, got %d", len(waiting1))
	}
	if waiting1[0].CpnID == "" {
		t.Fatal("run 1: waiting cpn_id is empty")
	}
	if waiting1[0].Tips != "请输入任意内容，输入 `1` 则退出循环：" {
		t.Fatalf("run 1: waiting tips = %q, want %q", waiting1[0].Tips, "请输入任意内容，输入 `1` 则退出循环：")
	}
	if len(waiting1[0].Inputs) == 0 {
		t.Fatal("run 1: waiting inputs is empty")
	}
	// The SSE channel always closes with the `done` terminator,
	// even when the run paused for user input — see
	// TestRunAgent_RealCanvas_WaitForUserResume_EventSemantics
	// for the channel-end contract.
	if !done1 {
		t.Error("run 1: expected done terminator after waiting_for_user")
	}
	if len(messages1) != 0 {
		t.Fatalf("run 1: expected 0 message events before resume, got %d", len(messages1))
	}

	events2, err := svc.RunAgent(
		context.Background(),
		"user-1",
		"canvas-all",
		"session-all-loop",
		"",
		"1",
	)
	if err != nil {
		t.Fatalf("RunAgent run 2: %v", err)
	}
	messages2, waiting2, errs2, done2 := drainAgentEvents(t, events2)
	if len(errs2) > 0 {
		t.Fatalf("run 2: unexpected error events: %+v", errs2)
	}
	if len(waiting2) > 0 {
		t.Fatalf("run 2: did not expect another waiting_for_user event, got %+v", waiting2)
	}
	if !done2 {
		t.Error("run 2: missing done event")
	}
	if len(messages2) != 1 {
		t.Fatalf("run 2: expected 1 message event after resume, got %d", len(messages2))
	}
	if !strings.Contains(messages2[0].Content, "循环结束") {
		t.Errorf("run 2: Content = %q, want substring %q", messages2[0].Content, "循环结束")
	}
	if !strings.Contains(messages2[0].Content, "1") {
		t.Errorf("run 2: Content = %q, want substring %q", messages2[0].Content, "1")
	}
}

func TestRunAgent_AllFixture_LoopInterruptResume_MultiTurn(t *testing.T) {
	testDB := setupServiceTestDB(t)
	if err := testDB.AutoMigrate(
		&entity.User{},
		&entity.Tenant{},
		&entity.UserTenant{},
		&entity.UserCanvas{},
		&entity.UserCanvasVersion{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	orig := dao.DB
	dao.DB = testDB
	t.Cleanup(func() { dao.DB = orig })

	raw, err := os.ReadFile(filepath.Join("..", "agent", "dsl", "testdata", "all.json"))
	if err != nil {
		t.Fatalf("read all.json: %v", err)
	}
	var dsl map[string]any
	if err := json.Unmarshal(raw, &dsl); err != nil {
		t.Fatalf("parse all.json: %v", err)
	}
	makeCanvasWithDSL(t, "canvas-all-multi", "user-1", "tenant-1", "v-all-multi", dsl)

	tracker, mr := newRunTrackerForTest(t, 30*24*time.Hour)
	cpClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = cpClient.Close() })
	cp := canvas.NewRedisCheckPointStoreWithClient(cpClient, 30*24*time.Hour)
	svc := NewAgentServiceWithOptions(cp, nil, tracker)

	sessionID := "session-all-loop-multi"
	inputs := []string{"loop", "aaa", "bbb", "1"}
	var allMessages []canvas.MessageEvent

	for i, input := range inputs {
		events, err := svc.RunAgent(
			context.Background(),
			"user-1",
			"canvas-all-multi",
			sessionID,
			"",
			input,
		)
		if err != nil {
			t.Fatalf("RunAgent run %d (%q): %v", i+1, input, err)
		}
		messages, waiting, errs, done := drainAgentEvents(t, events)
		if len(errs) > 0 {
			t.Fatalf("run %d (%q): unexpected error events: %+v", i+1, input, errs)
		}
		allMessages = append(allMessages, messages...)

		switch input {
		case "loop", "aaa", "bbb":
			if len(waiting) != 1 {
				t.Fatalf("run %d (%q): expected 1 waiting_for_user event, got %+v", i+1, input, waiting)
			}
			if waiting[0].Tips != "请输入任意内容，输入 `1` 则退出循环：" {
				t.Fatalf("run %d (%q): waiting tips = %q, want %q", i+1, input, waiting[0].Tips, "请输入任意内容，输入 `1` 则退出循环：")
			}
			if !done {
				t.Errorf("run %d (%q): missing done terminator after waiting_for_user", i+1, input)
			}
		case "1":
			if len(waiting) != 0 {
				t.Fatalf("run %d (%q): did not expect another waiting_for_user event, got %+v", i+1, input, waiting)
			}
			if !done {
				t.Fatalf("run %d (%q): missing done event", i+1, input)
			}
		}
	}

	if len(allMessages) != 3 {
		t.Fatalf("all messages len = %d, want 3", len(allMessages))
	}
	if !strings.Contains(allMessages[0].Content, "继续循环中") || !strings.Contains(allMessages[0].Content, "aaa") {
		t.Fatalf("message 1 = %q, want continue message for aaa", allMessages[0].Content)
	}
	if !strings.Contains(allMessages[1].Content, "继续循环中") || !strings.Contains(allMessages[1].Content, "bbb") {
		t.Fatalf("message 2 = %q, want continue message for bbb", allMessages[1].Content)
	}
	if !strings.Contains(allMessages[2].Content, "循环结束") || !strings.Contains(allMessages[2].Content, "1") {
		t.Fatalf("message 3 = %q, want final loop-done message for 1", allMessages[2].Content)
	}
}

func TestRunAgent_AllFixture_IterationFormatsItems(t *testing.T) {
	testDB := setupServiceTestDB(t)
	if err := testDB.AutoMigrate(
		&entity.User{},
		&entity.Tenant{},
		&entity.UserTenant{},
		&entity.UserCanvas{},
		&entity.UserCanvasVersion{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	orig := dao.DB
	dao.DB = testDB
	t.Cleanup(func() { dao.DB = orig })

	raw, err := os.ReadFile(filepath.Join("..", "agent", "dsl", "testdata", "all.json"))
	if err != nil {
		t.Fatalf("read all.json: %v", err)
	}
	var dsl map[string]any
	if err := json.Unmarshal(raw, &dsl); err != nil {
		t.Fatalf("parse all.json: %v", err)
	}
	makeCanvasWithDSL(t, "canvas-all-iteration", "user-1", "tenant-1", "v-all-iteration", dsl)

	tracker, mr := newRunTrackerForTest(t, 30*24*time.Hour)
	cpClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = cpClient.Close() })
	cp := canvas.NewRedisCheckPointStoreWithClient(cpClient, 30*24*time.Hour)
	svc := NewAgentServiceWithOptions(cp, nil, tracker)

	sessionID := "session-all-iteration"

	events1, err := svc.RunAgent(
		context.Background(),
		"user-1",
		"canvas-all-iteration",
		sessionID,
		"",
		"iteration",
	)
	if err != nil {
		t.Fatalf("RunAgent run 1: %v", err)
	}
	messages1, waiting1, errs1, done1 := drainAgentEvents(t, events1)
	if len(errs1) > 0 {
		t.Fatalf("run 1: unexpected error events: %+v", errs1)
	}
	if len(messages1) != 0 {
		t.Fatalf("run 1: expected 0 message events before resume, got %d", len(messages1))
	}
	if len(waiting1) != 1 {
		t.Fatalf("run 1: expected 1 waiting_for_user event, got %+v", waiting1)
	}
	// SSE channel always closes with the `done` terminator,
	// even when the run paused for user input.
	if !done1 {
		t.Error("run 1: expected done terminator after waiting_for_user")
	}

	events2, err := svc.RunAgent(
		context.Background(),
		"user-1",
		"canvas-all-iteration",
		sessionID,
		"",
		"a,b,c,d,e",
	)
	if err != nil {
		t.Fatalf("RunAgent run 2: %v", err)
	}
	messages2, waiting2, errs2, done2 := drainAgentEvents(t, events2)
	if len(errs2) > 0 {
		t.Fatalf("run 2: unexpected error events: %+v", errs2)
	}
	if len(waiting2) != 0 {
		t.Fatalf("run 2: did not expect another waiting_for_user event, got %+v", waiting2)
	}
	if !done2 {
		t.Fatal("run 2: missing done event")
	}
	if len(messages2) != 1 {
		t.Fatalf("run 2: expected 1 message event, got %d", len(messages2))
	}
	content := messages2[0].Content
	// Go renders []any{"a","b","c","d","e"} via fmt.Sprintf("%v", ...)
	// as "[a b c d e]" (space-separated, no commas, no quotes). The
	// DSL template "输入数组: {StringTransform:SplitCSV@result}"
	// therefore produces "输入数组: [a b c d e]" with the leading
	// space the author put in the DSL between ':' and '{'.
	want := "迭代结束。\n输入数组: [a b c d e]\n格式化输出(lines):[0: a 1: b 2: c 3: d 4: e]"
	if content != want {
		t.Fatalf("run 2: Content = %q, want %q", content, want)
	}
}

func TestRunAgent_AllFixture_VarAssigner(t *testing.T) {
	testDB := setupServiceTestDB(t)
	if err := testDB.AutoMigrate(
		&entity.User{},
		&entity.Tenant{},
		&entity.UserTenant{},
		&entity.UserCanvas{},
		&entity.UserCanvasVersion{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	orig := dao.DB
	dao.DB = testDB
	t.Cleanup(func() { dao.DB = orig })

	raw, err := os.ReadFile(filepath.Join("..", "agent", "dsl", "testdata", "all.json"))
	if err != nil {
		t.Fatalf("read all.json: %v", err)
	}
	var dsl map[string]any
	if err := json.Unmarshal(raw, &dsl); err != nil {
		t.Fatalf("parse all.json: %v", err)
	}
	makeCanvasWithDSL(t, "canvas-all-var-assigner", "user-1", "tenant-1", "v-all-var-assigner", dsl)

	svc := NewAgentService()
	events, err := svc.RunAgent(
		context.Background(),
		"user-1",
		"canvas-all-var-assigner",
		"session-all-var-assigner",
		"",
		"var_assigner",
	)
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	messages, waiting, errs, done := drainAgentEvents(t, events)
	if len(errs) > 0 {
		t.Fatalf("unexpected error events: %+v", errs)
	}
	if len(waiting) > 0 {
		t.Fatalf("did not expect waiting_for_user events: %+v", waiting)
	}
	if !done {
		t.Fatal("missing done event")
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message event, got %d", len(messages))
	}
	if messages[0].Content != "env.counter=1" {
		t.Fatalf("Content = %q, want %q", messages[0].Content, "env.counter=1")
	}
}

// TestRunAgent_AllFixture_DataOps drives the data_ops branch of all.json
// (Begin → UserFillUp:Menu → Switch:Route → DataOperations:UpdateSample
// → ListOperations:Top2 → Message:DataListDone). With env.sample_rows
// pre-populated with 3 rows, the message event should list the rows
// after the append_or_update + topN pipeline.
func TestRunAgent_AllFixture_DataOps(t *testing.T) {
	testDB := setupServiceTestDB(t)
	if err := testDB.AutoMigrate(
		&entity.User{},
		&entity.Tenant{},
		&entity.UserTenant{},
		&entity.UserCanvas{},
		&entity.UserCanvasVersion{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	orig := dao.DB
	dao.DB = testDB
	t.Cleanup(func() { dao.DB = orig })

	raw, err := os.ReadFile(filepath.Join("..", "agent", "dsl", "testdata", "all.json"))
	if err != nil {
		t.Fatalf("read all.json: %v", err)
	}
	var dsl map[string]any
	if err := json.Unmarshal(raw, &dsl); err != nil {
		t.Fatalf("parse all.json: %v", err)
	}
	makeCanvasWithDSL(t, "canvas-all-data-ops", "user-1", "tenant-1", "v-all-data-ops", dsl)

	svc := NewAgentService()
	events, err := svc.RunAgent(
		context.Background(),
		"user-1",
		"canvas-all-data-ops",
		"session-all-data-ops",
		"",
		"data_ops",
	)
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	messages, waiting, errs, done := drainAgentEvents(t, events)
	if len(errs) > 0 {
		t.Fatalf("unexpected error events: %+v", errs)
	}
	if len(waiting) > 0 {
		t.Fatalf("did not expect waiting_for_user events: %+v", waiting)
	}
	if !done {
		t.Fatal("missing done event")
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message event, got %d (%v)", len(messages), messages)
	}
	t.Logf("data_ops message content:\n%s", messages[0].Content)
	if !strings.Contains(messages[0].Content, "ListOperations Sort desc") {
		t.Errorf("Content = %q, want substring %q", messages[0].Content, "ListOperations Sort desc")
	}
	if !strings.Contains(messages[0].Content, "ListOperations Head2") {
		t.Errorf("Content = %q, want substring %q", messages[0].Content, "ListOperations Head2")
	}
	// opSort with sort_by="score" sorts by the "score" field (primary,
	// not the legacy hashableKey first-field). desc + score picks
	// Alpha(0.91), Beta(0.88), Gamma(0.76) regardless of input order.
	// Head(2) then takes Alpha, Beta.
	if !strings.Contains(messages[0].Content, "first=map[id:1 score:0.91 tag:demo title:Alpha]") {
		t.Errorf("Content = %q, want first=Alpha (sort_by=score desc top-1)", messages[0].Content)
	}
	if !strings.Contains(messages[0].Content, "last=map[id:2 score:0.88 tag:demo title:Beta]") {
		t.Errorf("Content = %q, want last=Beta (sort_by=score desc top-2)", messages[0].Content)
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
		&entity.User{},
		&entity.Tenant{},
		&entity.UserTenant{},
		&entity.UserCanvas{},
		&entity.UserCanvasVersion{},
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
		"hello",
	)
	if err != nil {
		t.Fatalf("RunAgent returned sync error: %v", err)
	}
	_, _, errs, _ := drainAgentEvents(t, events)
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

// TestRunAgent_AllFixture_CategorizeResume drives the categorize branch
// of all.json through the real checkpoint-backed interrupt/resume path:
//
//  1. First input "categorize" is consumed by UserFillUp:Menu, which routes
//     through Switch:Route into UserFillUp:CateInput and pauses there.
//  2. Second input "hello" must resume CateInput itself, not be re-consumed
//     by the menu as a fresh branch selection.
//
// This specifically covers the buildRunFunc fix that clears sys.query /
// Invoke(query) on resume. Without that fix, the resumed payload can be
// mistaken for a new menu choice and silently drop the paused branch.
func TestRunAgent_AllFixture_CategorizeResume(t *testing.T) {
	testDB := setupServiceTestDB(t)
	if err := testDB.AutoMigrate(
		&entity.User{},
		&entity.Tenant{},
		&entity.UserTenant{},
		&entity.UserCanvas{},
		&entity.UserCanvasVersion{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	orig := dao.DB
	dao.DB = testDB
	t.Cleanup(func() { dao.DB = orig })

	raw, err := os.ReadFile(filepath.Join("..", "agent", "dsl", "testdata", "all.json"))
	if err != nil {
		t.Fatalf("read all.json: %v", err)
	}
	var dsl map[string]any
	if err := json.Unmarshal(raw, &dsl); err != nil {
		t.Fatalf("parse all.json: %v", err)
	}
	makeCanvasWithDSL(t, "canvas-all-categorize", "user-1", "tenant-1", "v-all-categorize", dsl)

	prevInvoker := component.GetDefaultChatInvokerForTest()
	component.SetDefaultChatInvoker(&categorizeResumeInvoker{})
	t.Cleanup(func() { component.SetDefaultChatInvoker(prevInvoker) })

	tracker, mr := newRunTrackerForTest(t, 30*24*time.Hour)
	cpClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = cpClient.Close() })
	cp := canvas.NewRedisCheckPointStoreWithClient(cpClient, 30*24*time.Hour)
	svc := NewAgentServiceWithOptions(cp, nil, tracker)
	events1, err := svc.RunAgent(
		context.Background(),
		"user-1",
		"canvas-all-categorize",
		"session-all-categorize",
		"",
		"categorize",
	)
	if err != nil {
		t.Fatalf("RunAgent run 1: %v", err)
	}
	messages1, waiting1, errs1, done1 := drainAgentEvents(t, events1)
	t.Logf("run1: messages=%d waiting=%d errs=%d done=%v", len(messages1), len(waiting1), len(errs1), done1)
	for i, e := range errs1 {
		t.Logf("  run1 err[%d]: %s", i, e.Message)
	}
	if len(messages1) != 0 {
		t.Fatalf("run 1: expected 0 message events before resume, got %d", len(messages1))
	}
	if len(waiting1) != 1 {
		t.Fatalf("run 1: expected 1 waiting_for_user, got %d (errs=%+v)", len(waiting1), errs1)
	}
	if !strings.Contains(waiting1[0].Tips, "分类") {
		t.Fatalf("run 1: waiting tips = %q, want categorize prompt", waiting1[0].Tips)
	}
	if !done1 {
		t.Error("run 1: expected done terminator after waiting_for_user")
	}

	events2, err := svc.RunAgent(
		context.Background(),
		"user-1",
		"canvas-all-categorize",
		"session-all-categorize",
		"",
		"hello",
	)
	if err != nil {
		t.Fatalf("RunAgent run 2: %v", err)
	}
	messages2, waiting2, errs2, done2 := drainAgentEvents(t, events2)
	t.Logf("run2: messages=%d waiting=%d errs=%d done=%v", len(messages2), len(waiting2), len(errs2), done2)
	for i, e := range errs2 {
		t.Logf("  run2 err[%d]: %s", i, e.Message)
	}
	for i, m := range messages2 {
		t.Logf("  run2 msg[%d]: %q", i, m.Content)
	}
	if len(waiting2) != 0 {
		t.Fatalf("run 2: did not expect another waiting_for_user event, got %+v", waiting2)
	}
	if len(errs2) != 0 {
		t.Fatalf("run 2: expected no downstream errors, got %+v", errs2)
	}
	if !done2 {
		t.Fatal("run 2: expected done event after successful categorize branch completion")
	}
	if len(messages2) != 1 {
		t.Fatalf("run 2: expected 1 message event, got %d", len(messages2))
	}
	if !strings.Contains(messages2[0].Content, "分类结果=Retrieval -> Retrieval") {
		t.Fatalf("run 2: message = %q, want categorize retrieval branch output", messages2[0].Content)
	}
}

type categorizeResumeInvoker struct{}

func (i *categorizeResumeInvoker) Invoke(_ context.Context, req component.ChatInvokeRequest) (*component.ChatInvokeResponse, error) {
	return &component.ChatInvokeResponse{
		Content: "Retrieval",
		Model:   req.ModelName,
		Stopped: true,
	}, nil
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
// mapAgentError must classify the resulting wrapped
// ErrAgentStorageError as CodeServerError (500). The test asserts
// at the service layer; the handler-layer mapping is exercised by
// TestMapAgentError in handler/agent_test.go.
func TestRunAgent_RealCanvas_InvokeFails(t *testing.T) {
	testDB := setupServiceTestDB(t)
	if err := testDB.AutoMigrate(
		&entity.User{},
		&entity.Tenant{},
		&entity.UserTenant{},
		&entity.UserCanvas{},
		&entity.UserCanvasVersion{},
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
		"hello",
	)
	if err != nil {
		t.Fatalf("RunAgent returned sync error: %v", err)
	}
	_, _, errs, _ := drainAgentEvents(t, events)
	if len(errs) == 0 {
		t.Fatal("expected error event from Invoke of DSL with bad component query ref")
	}
	if !strings.Contains(errs[0].Message, "agent storage error") {
		t.Errorf("error message %q does not mention sanitised label", errs[0].Message)
	}
}

// newRunTrackerForTest wires a RunTracker against an in-memory
// miniredis so tests can exercise the production Start /
// AttachCheckpoint / MarkSucceeded sequence without a live Redis.
// The returned miniredis handle is closed via t.Cleanup.
func newRunTrackerForTest(t *testing.T, ttl time.Duration) (*canvas.RunTracker, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return canvas.NewRunTrackerWithClient(client, ttl), mr
}

// TestRunAgent_RunTracker_AttachCheckpoint_CallSequence pins the
// production boot path that v3.6.0 enables: an AgentService
// constructed via NewAgentServiceWithOptions (with a real
// RedisCheckPointStore + CanvasStateSerializer + RunTracker) must
// record the full Start → AttachCheckpoint → MarkSucceeded sequence
// against the run hash during a single successful run.
//
// What this test catches that the nil-tracker tests miss:
//
//   - Start is called before Compile (we verify canvas_id and
//     status="0" survive in the hash, which means Start ran).
//   - AttachCheckpoint is called after Invoke when the store is
//     configured (we verify checkpoint_id is non-empty and matches
//     the canvas+session id).
//   - MarkSucceeded is called when the run completes without error
//     (we verify status="1" and finished_at is set).
//
// If buildRunFunc's "if cpID != "" && s.runTracker != nil" guard
// regresses (e.g. someone drops the AttachCheckpoint call), this
// test fails because checkpoint_id stays empty. The v3.5.2 review
// explicitly flagged this test as deferred; this is the v3.6.1
// closure.
func TestRunAgent_RunTracker_AttachCheckpoint_CallSequence(t *testing.T) {
	testDB := setupServiceTestDB(t)
	if err := testDB.AutoMigrate(
		&entity.User{},
		&entity.Tenant{},
		&entity.UserTenant{},
		&entity.UserCanvas{},
		&entity.UserCanvasVersion{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	orig := dao.DB
	dao.DB = testDB
	t.Cleanup(func() { dao.DB = orig })

	// Shared miniredis: the same client backs the CheckPointStore
	// and the RunTracker so the AgentService's real Compile / Invoke
	// chain can persist + link the checkpoint payload.
	tracker, mr := newRunTrackerForTest(t, 30*24*time.Hour)
	cpClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = cpClient.Close() })
	cp := canvas.NewRedisCheckPointStoreWithClient(cpClient, 30*24*time.Hour)

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
					"params":         map[string]any{"text": "hello {{sys.query}}"},
				},
				"upstream": []any{"begin_0"},
			},
		},
		"path": []any{"begin_0", "message_0"},
	}
	makeCanvasWithDSL(t, "canvas-cp", "user-1", "tenant-1", "v-cp", dsl)

	svc := NewAgentServiceWithOptions(cp, canvas.CanvasStateSerializer{}, tracker)
	events, err := svc.RunAgent(
		context.Background(),
		"user-1",
		"canvas-cp",
		"session-cp",
		"", // latest version
		"world",
	)
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	messages, _, errs, done := drainAgentEvents(t, events)
	if len(errs) > 0 {
		t.Fatalf("unexpected error events: %+v", errs)
	}
	if len(messages) != 1 || !strings.Contains(messages[0].Content, "hello world") {
		t.Fatalf("expected 1 message containing %q, got %+v", "hello world", messages)
	}
	if !done {
		t.Error("missing terminator done event")
	}

	// The run id is canvasID-sessionID per runIDFor; this is also
	// the checkpoint id per the Goal 7 contract.
	runID := "canvas-cp-session-cp"
	got, err := tracker.Get(context.Background(), runID)
	if err != nil {
		t.Fatalf("RunTracker.Get: %v", err)
	}
	if len(got) == 0 {
		t.Fatalf("run hash %q is empty — Start was never called", runID)
	}

	// Start evidence: canvas_id, tenant_id, and a Start-applied TTL
	// (the pipeline inside Start stamps the TTL on the first write;
	// without Start the key would have no TTL).
	if got["canvas_id"] != "canvas-cp" {
		t.Errorf("canvas_id = %q, want %q", got["canvas_id"], "canvas-cp")
	}
	if got["tenant_id"] != "tenant-1" {
		t.Errorf("tenant_id = %q, want %q", got["tenant_id"], "tenant-1")
	}
	if d := mr.TTL("agent:run:" + runID); d <= 0 {
		t.Errorf("TTL on run hash = %v, want > 0 (Start pipeline missing?)", d)
	}

	// AttachCheckpoint evidence: the Goal 7 contract reuses runID
	// as the checkpoint id, so we expect checkpoint_id to equal
	// runID. A missing/empty value means the AttachCheckpoint call
	// in buildRunFunc did not run (or cpID was empty).
	if got["checkpoint_id"] == "" {
		t.Fatalf("checkpoint_id is empty — AttachCheckpoint was never called for run %q (buildRunFunc Goal 7 regressed?)", runID)
	}
	if got["checkpoint_id"] != runID {
		t.Errorf("checkpoint_id = %q, want %q (per Goal 7 runID-as-cpID contract)", got["checkpoint_id"], runID)
	}

	// MarkSucceeded evidence: status must be "1" and finished_at
	// must be set. A "0" status means MarkSucceeded did not run
	// (or the run failed before the success branch).
	if got["status"] != "1" {
		t.Errorf("status = %q, want %q (succeeded); MarkSucceeded missing?", got["status"], "1")
	}
	if got["finished_at"] == "" {
		t.Error("finished_at is empty — MarkSucceeded missing the timestamp?")
	}
}
