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
	"strings"
	"testing"
	"time"

	"ragflow/internal/agent/canvas"
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

// TestRunAgent_RealCanvas_BeginMessage is the load-bearing happy-path
// test for Phase 4.4 V2. It publishes a 2-component DSL (Begin →
// Message where Message.text = "hello {{sys.query}}"), invokes
// RunAgent with user_input="world", and asserts the SSE surface
// emits one message whose Answer is "hello world".
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
	if !strings.Contains(messages[0].Answer, "hello world") {
		t.Errorf("Answer = %q, want substring %q", messages[0].Answer, "hello world")
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
	if !strings.Contains(messages2[0].Answer, "got: my follow-up") {
		t.Errorf("run 2: Answer = %q, want substring %q", messages2[0].Answer, "got: my follow-up")
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

// TestRunAgent_RealCanvas_InvokeFails pins the runtime-failure
// branch: a DSL that compiles cleanly (registry is happy) but
// fails at runtime — using a Message component with an
// unresolvable reference ({{nonexistent@var}}) so the template
// resolver errors out during Message.Invoke.
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
				"downstream": []any{"message_0"},
			},
			"message_0": map[string]any{
				"obj": map[string]any{
					"component_name": "Message",
					// Deliberately unresolvable; Message.Invoke's
					// ResolveTemplate will fail on this and the
					// component body returns an error.
					"params": map[string]any{"text": "boom: {{nonexistent@nonexistent}}"},
				},
				"upstream": []any{"begin_0"},
			},
		},
		"path": []any{"begin_0", "message_0"},
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
		t.Fatal("expected error event from Invoke of DSL with unresolvable template ref")
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
	if len(messages) != 1 || !strings.Contains(messages[0].Answer, "hello world") {
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
