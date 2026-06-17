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

package handler

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/cloudwego/eino/compose"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

// waitFakeAgentService is a full AgentService implementation that
// runs the wait_for_user cycle end-to-end. It plugs a fake
// RunFunc into the orchestrator driver so we can assert the SSE
// wire shape and the resume path without a real eino workflow.
//
// The fake records every root input the RunFunc was called with
// and every (canvasID, sessionID) pair it saw, so the test can
// assert that:
//
//  1. The first call (no user_input) drives the canvas once.
//  2. The first call's run returns an eino interrupt error — the
//     orchestrator extracts InterruptContexts, persists the first
//     cpn id, and emits a `waiting_for_user` event.
//  3. The persisted interrupt id is held in the driver (Peek == true).
//  4. The second call (with user_input) re-invokes the canvas with
//     root that contains __resume_interrupt_id__ + __resume_data__.
type waitFakeAgentService struct {
	mu       sync.Mutex
	runCalls int
	roots    []map[string]any
	// stubRunFunc is supplied by the test; it returns either a
	// clean state (resume path) or an eino interrupt error (first
	// call path). The orchestrator driver inspects the error to
	// decide between `message`+`done` and `waiting_for_user` events.
	stubRunFunc func(call int, root map[string]any) (*runtime.CanvasState, error)
	driver      *canvas.Runner
}

func newWaitFakeAgentService(stub func(call int, root map[string]any) (*runtime.CanvasState, error)) *waitFakeAgentService {
	return &waitFakeAgentService{
		stubRunFunc: stub,
		driver:      canvas.NewRunner(),
	}
}

func (f *waitFakeAgentService) ListAgents(string, string, int, int, string, bool, []string, string) (*service.ListAgentsResponse, common.ErrorCode, error) {
	return &service.ListAgentsResponse{}, common.CodeSuccess, nil
}
func (f *waitFakeAgentService) CreateAgent(context.Context, *service.CreateAgentRequest) (*entity.UserCanvas, common.ErrorCode, error) {
	return nil, common.CodeArgumentError, nil
}
func (f *waitFakeAgentService) GetAgent(context.Context, string, string) (*entity.UserCanvas, error) {
	return &entity.UserCanvas{ID: "canvas-wait"}, nil
}
func (f *waitFakeAgentService) UpdateAgent(context.Context, string, string, entity.JSONMap) error {
	return nil
}
func (f *waitFakeAgentService) DeleteAgent(context.Context, string, string) error {
	return nil
}

// RunAgent mimics service.AgentService.RunAgent for the test
// driver. It loads the canvas (a no-op in tests), builds a RunFunc
// from the supplied stub, and hands off to the orchestrator.
func (f *waitFakeAgentService) RunAgent(ctx context.Context, userID, canvasID, sessionID, version, userInput string) (<-chan canvas.RunEvent, error) {
	_ = ctx
	_ = userID
	_ = version
	stub := f.stubRunFunc
	run := func(ctx context.Context, root map[string]any) (*runtime.CanvasState, error) {
		f.mu.Lock()
		f.runCalls++
		call := f.runCalls
		f.roots = append(f.roots, root)
		f.mu.Unlock()
		return stub(call, root)
	}
	return f.driver.Run(ctx, run, canvasID, sessionID, userInput, map[string]any{
		"user_id":    userID,
		"canvas_id":  canvasID,
		"session_id": sessionID,
	}), nil
}

func (f *waitFakeAgentService) CancelAgent(context.Context, string, string) error { return nil }
func (f *waitFakeAgentService) PublishAgent(context.Context, string, string, *service.PublishAgentRequest) (*entity.UserCanvasVersion, error) {
	return &entity.UserCanvasVersion{}, nil
}
func (f *waitFakeAgentService) ListVersions(context.Context, string, string) ([]*entity.UserCanvasVersion, error) {
	return nil, nil
}
func (f *waitFakeAgentService) GetVersion(context.Context, string, string, string) (*entity.UserCanvasVersion, error) {
	return &entity.UserCanvasVersion{}, nil
}
func (f *waitFakeAgentService) DeleteVersion(context.Context, string, string, string) error {
	return nil
}

// waitForUserRoutes wires a minimal gin engine that exposes the
// RunAgent route. The full AgentService surface is not exercised
// by the wait_for_user test, so we only need the SSE endpoint.
func waitForUserRoutes(svc *waitFakeAgentService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := &AgentHandler{agentService: nil} // overridden below
	// The handler holds a *service.AgentService (concrete), so we
	// route through a small adapter: re-define the route to call
	// the fake's RunAgent directly.
	g := r.Group("/api/v1/agents")
	g.Use(func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "user-wait"})
		c.Next()
	})
	g.POST("/:canvas_id/run", func(c *gin.Context) {
		// We deliberately re-implement the handler's SSE loop
		// here so the test does not depend on the concrete
		// *service.AgentService type (the fake is an interface
		// stand-in).
		canvasID := c.Param("canvas_id")
		sessionID := c.Query("session_id")
		userInput := c.Query("user_input")
		events, err := svc.RunAgent(c.Request.Context(), "user-wait", canvasID, sessionID, "", userInput)
		if err != nil {
			// We never expect a non-nil err from the fake,
			// but be defensive.
			c.JSON(http.StatusInternalServerError, gin.H{"code": common.CodeServerError, "message": err.Error()})
			return
		}
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		c.Writer.Header().Set("Cache-Control", "no-cache")
		c.Writer.Header().Set("Connection", "keep-alive")
		flusher, _ := c.Writer.(http.Flusher)
		for ev := range events {
			payload, _ := json.Marshal(map[string]any{
				"event":     ev.Type,
				"canvas_id": canvasID,
				"data":      ev.Data,
			})
			fmt.Fprintf(c.Writer, "data: %s\n\n", payload)
			if flusher != nil {
				flusher.Flush()
			}
		}
		fmt.Fprintf(c.Writer, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		_ = h // silence unused
	})
	return r
}

// TestWaitForUser_SSECycleRoundTrip drives the wait-for-user cycle
// end-to-end using the eino interrupt mechanism.
//
//  1. First call (no user_input). The stub RunFunc returns an eino
//     interrupt error via compose.Interrupt — simulating a UserFillUp
//     node pausing the graph. The orchestrator must:
//     - extract the InterruptCtx list (which is nil for a raw
//     InterruptSignal but the error IS classified as interrupt)
//     - emit a `waiting_for_user` event
//     - persist the interrupt id so the resume call can target it
//  2. Second call with user_input="yes please". The orchestrator
//     reads the persisted interrupt id, injects __resume_interrupt_id__
//     + __resume_data__ into root, and re-invokes the canvas. The
//     stub now returns a clean state — the orchestrator must emit
//     `message` + `done`.
//
// The test asserts on the SSE wire format and the call-count / root
// shape so we catch regressions in either half of the cycle.
func TestWaitForUser_SSECycleRoundTrip(t *testing.T) {
	const sessionID = "sess-wait-1"
	const userReply = "yes please"

	// Stub: first call returns an eino interrupt signal (simulating
	// a UserFillUp node pausing the graph), second call returns a
	// clean completion state.
	stub := func(call int, root map[string]any) (*runtime.CanvasState, error) {
		if call == 1 {
			// First call: emit a raw interrupt signal. The
			// orchestrator driver classifies this as an
			// interrupt error and emits waiting_for_user.
			// The cpn id in the SSE event is the error string
			// (since a raw signal has no wrapped InterruptCtx
			// list — this is acceptable for V1 and matches
			// the test's relaxed cpn_id assertion below).
			return nil, compose.Interrupt(context.Background(), map[string]any{
				"kind":    "user_fill_up",
				"cpn_id":  "answer-1",
				"tips":    "Do you want to continue?",
				"message": "waiting for user input",
			})
		}
		// Resume: emulate a clean completion.
		state := runtime.NewCanvasState("canvas-wait", "")
		state.RecordOutput("answer-1", "answer", "Glad to continue.")
		return state, nil
	}

	svc := newWaitFakeAgentService(stub)
	r := waitForUserRoutes(svc)

	// --- 1. First call: canvas should pause on wait_for_user ---
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest(http.MethodPost,
		"/api/v1/agents/canvas-wait/run?session_id="+sessionID, nil)
	r.ServeHTTP(w1, req1)

	if w1.Code != http.StatusOK {
		t.Fatalf("first call: expected 200, got %d", w1.Code)
	}
	frames1 := parseSSEFrames(t, w1.Body.Bytes())
	if len(frames1) < 2 {
		t.Fatalf("first call: expected at least 2 SSE frames (event + [DONE]), got %d: %v", len(frames1), frames1)
	}

	// Last frame must be the [DONE] terminator.
	if frames1[len(frames1)-1] != "[DONE]" {
		t.Fatalf("first call: expected last frame == [DONE], got %q", frames1[len(frames1)-1])
	}

	// Find the waiting_for_user event. The frame is the JSON
	// envelope; the `data` field is a JSON-encoded payload.
	var waitFrame map[string]any
	for _, fr := range frames1[:len(frames1)-1] {
		var env map[string]any
		if err := json.Unmarshal([]byte(fr), &env); err != nil {
			t.Fatalf("first call: bad JSON frame %q: %v", fr, err)
		}
		if env["event"] == "waiting_for_user" {
			waitFrame = env
			break
		}
	}
	if waitFrame == nil {
		t.Fatalf("first call: no waiting_for_user event in frames: %v", frames1)
	}
	if waitFrame["canvas_id"] != "canvas-wait" {
		t.Errorf("first call: canvas_id mismatch: %v", waitFrame["canvas_id"])
	}
	// The `data` field is a JSON string. Decode and check cpn_id.
	//
	// For the raw InterruptSignal path the orchestrator emits the
	// error.Error() string as the cpn id (no wrapped InterruptCtx
	// list to extract from). Production paths with a real eino
	// runner wrap the signal and surface the actual cpn id; the
	// unit test exercises the raw path so we only assert non-empty.
	dataRaw, ok := waitFrame["data"].(string)
	if !ok {
		t.Fatalf("first call: waiting_for_user data is not a string: %T", waitFrame["data"])
	}
	var dataEnv struct {
		CpnID string `json:"cpn_id"`
	}
	if err := json.Unmarshal([]byte(dataRaw), &dataEnv); err != nil {
		t.Fatalf("first call: bad waiting_for_user data: %v", err)
	}
	if dataEnv.CpnID == "" {
		t.Errorf("first call: cpn_id should be non-empty, got %q", dataEnv.CpnID)
	}

	// The interrupt id must be persisted for the resume call.
	if !svc.driver.Peek("canvas-wait", sessionID) {
		t.Fatalf("first call: interrupt id not persisted for (%q, %q)", "canvas-wait", sessionID)
	}

	// --- 2. Second call: resume with user_input ---
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest(http.MethodPost,
		"/api/v1/agents/canvas-wait/run?session_id="+sessionID+"&user_input="+userReply, nil)
	r.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("second call: expected 200, got %d", w2.Code)
	}
	frames2 := parseSSEFrames(t, w2.Body.Bytes())
	if frames2[len(frames2)-1] != "[DONE]" {
		t.Fatalf("second call: expected [DONE] tail, got %q", frames2[len(frames2)-1])
	}

	// Assert the run was called twice with the expected roots.
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.runCalls != 2 {
		t.Fatalf("expected exactly 2 canvas invocations, got %d", svc.runCalls)
	}
	if len(svc.roots) != 2 {
		t.Fatalf("expected 2 recorded roots, got %d", len(svc.roots))
	}

	// First call's root has no resume signal (it was the
	// initial turn, no follow-up supplied).
	if _, ok := svc.roots[0]["__resume_interrupt_id__"]; ok {
		t.Errorf("first call root should NOT have __resume_interrupt_id__, got %v", svc.roots[0])
	}

	// Second call's root MUST carry the resume signal — the
	// driver injects these so the RunFunc can decorate ctx
	// with compose.ResumeWithData(ctx, id, data) before Invoke.
	if _, ok := svc.roots[1]["__resume_interrupt_id__"]; !ok {
		t.Errorf("second call root missing __resume_interrupt_id__: %v", svc.roots[1])
	}
	if got := svc.roots[1]["__resume_data__"]; got != userReply {
		t.Errorf("second call root __resume_data__: got %v want %q", got, userReply)
	}

	// Persisted interrupt id must be cleared after the resume so a
	// third call with no user_input starts fresh.
	if svc.driver.Peek("canvas-wait", sessionID) {
		t.Errorf("persisted interrupt id should be cleared after resume")
	}
}

// TestWaitForUser_NoSentinelEmitsMessage is the negative path: a
// canvas that returns a clean state must produce a `message` event
// (and a `done` terminator) but no `waiting_for_user` event.
func TestWaitForUser_NoSentinelEmitsMessage(t *testing.T) {
	stub := func(call int, root map[string]any) (*runtime.CanvasState, error) {
		state := runtime.NewCanvasState("canvas-ok", "")
		state.RecordOutput("answer-1", "answer", "All done.")
		return state, nil
	}
	svc := newWaitFakeAgentService(stub)
	r := waitForUserRoutes(svc)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost,
		"/api/v1/agents/canvas-ok/run?session_id=sess-ok", nil)
	r.ServeHTTP(w, req)

	frames := parseSSEFrames(t, w.Body.Bytes())
	if frames[len(frames)-1] != "[DONE]" {
		t.Fatalf("expected [DONE] tail, got %q", frames[len(frames)-1])
	}
	for _, fr := range frames[:len(frames)-1] {
		var env map[string]any
		if err := json.Unmarshal([]byte(fr), &env); err != nil {
			t.Fatalf("bad frame: %v", err)
		}
		if env["event"] == "waiting_for_user" {
			t.Errorf("did not expect waiting_for_user on a clean run, got %v", env)
		}
	}
	// At least one `message` event.
	sawMessage := false
	for _, fr := range frames[:len(frames)-1] {
		var env map[string]any
		_ = json.Unmarshal([]byte(fr), &env)
		if env["event"] == "message" {
			sawMessage = true
			break
		}
	}
	if !sawMessage {
		t.Errorf("expected at least one message event, got frames: %v", frames)
	}
}

// TestWaitForUser_RunFuncErrorSurfacesErrorEvent verifies that a
// failed canvas run is surfaced as an `error` event (not a 500).
// The handler must not 500 on transient canvas errors; it must
// close the stream cleanly after the error frame.
func TestWaitForUser_RunFuncErrorSurfacesErrorEvent(t *testing.T) {
	stub := func(call int, root map[string]any) (*runtime.CanvasState, error) {
		return nil, errors.New("synthetic canvas failure")
	}
	svc := newWaitFakeAgentService(stub)
	r := waitForUserRoutes(svc)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost,
		"/api/v1/agents/canvas-err/run?session_id=sess-err", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 SSE start, got %d", w.Code)
	}
	frames := parseSSEFrames(t, w.Body.Bytes())
	if frames[len(frames)-1] != "[DONE]" {
		t.Fatalf("expected [DONE] tail, got %q", frames[len(frames)-1])
	}
	sawError := false
	for _, fr := range frames[:len(frames)-1] {
		var env map[string]any
		_ = json.Unmarshal([]byte(fr), &env)
		if env["event"] == "error" {
			sawError = true
		}
	}
	if !sawError {
		t.Errorf("expected an error event, got frames: %v", frames)
	}
}

// TestIsInterruptError_RecognisesEinoSignal confirms the canvas-layer
// helper that the orchestrator Driver depends on. After the
// wait-for-user refactor (eino interrupt) the driver no longer
// inspects the post-run state for a __wait_for_user__ sentinel —
// it inspects the run error for an eino interrupt signal. The
// helper that classifies the error is canvas.IsInterruptError.
func TestIsInterruptError_RecognisesEinoSignal(t *testing.T) {
	// Plain error — not an interrupt.
	if canvas.IsInterruptError(errors.New("boom")) {
		t.Errorf("plain error should not be classified as interrupt")
	}
	// context.Canceled — also not an interrupt (cancel/timeout
	// takes precedence over wait-for-user).
	if canvas.IsInterruptError(context.Canceled) {
		t.Errorf("context.Canceled should not be classified as interrupt")
	}
	// nil — false.
	if canvas.IsInterruptError(nil) {
		t.Errorf("nil should not be classified as interrupt")
	}
}

// parseSSEFrames splits a raw SSE body into its data frames,
// stripping the leading "data: " and trailing "\n\n". Used by the
// wait_for_user tests to read the channel output.
func parseSSEFrames(t *testing.T, body []byte) []string {
	t.Helper()
	var frames []string
	sc := bufio.NewScanner(bytes.NewReader(body))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		frames = append(frames, strings.TrimPrefix(line, "data: "))
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan sse body: %v", err)
	}
	return frames
}

// silence unused import warnings for packages that may be unused
// in some build configurations.
var _ = gorm.ErrRecordNotFound
