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

// node_body_timeout_test.go — pins the cancellation-only execution context
// contract of realComponentBody. The framework imposes no wall-clock deadline
// on component execution (see realComponentBodyWithOptions), so a component
// only stops when its parent context is cancelled; there is no timeout wrap to
// test any more. These tests assert cancellation propagation and the
// pass-through of fast components.

package canvas

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"ragflow/internal/agent/runtime"

	"gorm.io/gorm"
)

// blockingComponent is a runtime.Component whose Invoke blocks until ctx is
// cancelled.
type blockingComponent struct{}

type failingComponent struct {
	output map[string]any
}

func (f *failingComponent) Name() string { return "failing" }

func (f *failingComponent) Invoke(_ context.Context, _ *gorm.DB, _ map[string]any) (map[string]any, error) {
	return f.output, errors.New("boom")
}

func (f *failingComponent) Stream(_ context.Context, _ map[string]any) (<-chan map[string]any, error) {
	return nil, nil
}

func (f *failingComponent) Inputs() map[string]string  { return nil }
func (f *failingComponent) Outputs() map[string]string { return nil }

func (b *blockingComponent) Name() string { return "blocking" }

func (b *blockingComponent) Invoke(ctx context.Context, _ *gorm.DB, _ map[string]any) (map[string]any, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (b *blockingComponent) Stream(_ context.Context, _ map[string]any) (<-chan map[string]any, error) {
	return nil, nil
}

func (b *blockingComponent) Inputs() map[string]string  { return nil }
func (b *blockingComponent) Outputs() map[string]string { return nil }

// TestRealComponentBody_RespectsParentCancellation verifies that when the
// parent context is already cancelled, the body surfaces a wrapped
// context.Canceled error. With no framework deadline, parent cancellation is
// the only mechanism that stops a long-running component.
func TestRealComponentBody_RespectsParentCancellation(t *testing.T) {
	comp := &blockingComponent{}
	body := realComponentBody("test-cpn", "TestBlocking", "", comp)

	parentCtx, cancel := context.WithCancel(t.Context())
	cancel() // pre-cancel

	_, err := body(parentCtx, map[string]any{"x": 1})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled wrapped error, got: %v", err)
	}
	if !strings.Contains(err.Error(), `component "test-cpn" invoke`) {
		t.Errorf("error = %q, want component ID fallback", err)
	}
}

func TestRealComponentBody_UsesDisplayNameInError(t *testing.T) {
	body := realComponentBody("CodeExec:SweetMooseTalk", "CodeExec", "Analyze sales", &failingComponent{})

	_, err := body(t.Context(), nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), `component "Analyze sales" invoke`) {
		t.Errorf("error = %q, want display name", err)
	}
	if strings.Contains(err.Error(), "CodeExec:SweetMooseTalk") {
		t.Errorf("error = %q, should not contain component ID when display name is set", err)
	}
}

func TestWithStateBracket_PersistsToolInvocationsOnComponentError(t *testing.T) {
	invocations := []map[string]any{{"tool_name": "search"}}
	state := runtime.NewCanvasState("run-agent", "session-agent")
	events := make(chan RunEvent, 2)
	ctx := WithRunMeta(WithState(t.Context(), state), &RunMeta{Events: events})
	component := &failingComponent{output: map[string]any{"tool_invocations": invocations}}
	body := withStateBracket("agent_0", "Agent", realComponentBody("agent_0", "Agent", "Agent", component))

	if _, err := body(ctx, nil); err == nil {
		t.Fatal("expected component error")
	}
	<-events // node_started
	finished := <-events
	if finished.Type != "node_finished" {
		t.Fatalf("event type = %q, want node_finished", finished.Type)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(finished.Data), &payload); err != nil {
		t.Fatalf("unmarshal node_finished: %v", err)
	}
	outputs, ok := payload["outputs"].(map[string]any)
	if !ok {
		t.Fatalf("node_finished outputs = %T, want map", payload["outputs"])
	}
	got, ok := outputs["tool_invocations"].([]any)
	if !ok || len(got) != 1 {
		t.Fatalf("node_finished tool_invocations = %#v, want one invocation", outputs["tool_invocations"])
	}
	if got[0].(map[string]any)["tool_name"] != "search" {
		t.Fatalf("node_finished invocation = %#v, want search", got[0])
	}
}

func TestBuildWorkflow_PersistsOutputsAndFinishesNodeOnComponentError(t *testing.T) {
	previousFactory := runtime.DefaultFactory()
	runtime.SetDefaultFactory(func(string, map[string]any) (runtime.Component, error) {
		return &failingComponent{output: map[string]any{
			"tool_invocations": []map[string]any{{"tool_name": "search"}},
		}}, nil
	})
	t.Cleanup(func() { runtime.SetDefaultFactory(previousFactory) })

	compiled, err := Compile(t.Context(), &Canvas{Components: map[string]CanvasComponent{
		"agent_0": {Obj: CanvasComponentObj{ComponentName: "Agent", Params: map[string]any{}}},
	}})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	state := runtime.NewCanvasState("run-agent-error", "session-agent-error")
	events := make(chan RunEvent, 4)
	ctx := WithRunMeta(WithState(t.Context(), state), &RunMeta{Events: events})
	if _, err := compiled.Workflow.Invoke(ctx, map[string]any{}); err == nil {
		t.Fatal("expected workflow error")
	}

	got, stateErr := state.GetVar("agent_0@tool_invocations")
	invocations, ok := got.([]map[string]any)
	if stateErr != nil || !ok || len(invocations) != 1 {
		t.Fatalf("tool_invocations = %#v, state error = %v; want one recorded invocation", got, stateErr)
	}
	finishedCount := 0
	persisted := false
	for len(events) > 0 {
		event := <-events
		if event.Type != "node_finished" {
			continue
		}
		finishedCount++
		var payload map[string]any
		if err := json.Unmarshal([]byte(event.Data), &payload); err != nil {
			t.Fatalf("unmarshal node_finished: %v", err)
		}
		outputs, _ := payload["outputs"].(map[string]any)
		if invocations, ok := outputs["tool_invocations"].([]any); ok && len(invocations) == 1 {
			persisted = true
			if nodeErr, ok := payload["error"].(string); !ok || !strings.Contains(nodeErr, "boom") {
				t.Errorf("node_finished error = %v, want component failure", payload["error"])
			}
		}
	}
	if finishedCount != 1 {
		t.Errorf("node_finished events = %d, want 1", finishedCount)
	}
	if !persisted {
		t.Fatal("node_finished event did not include tool_invocations")
	}
}

// TestRealComponentBody_NoTimeoutWhenFast verifies that a component returning
// immediately does not incur any timeout-induced latency or error wrapping.
func TestRealComponentBody_NoTimeoutWhenFast(t *testing.T) {
	comp := &echoComponent{}
	body := realComponentBody("test-cpn", "TestEcho", "", comp)

	out, err := body(t.Context(), map[string]any{"x": 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["__cpn_id__"] != "test-cpn" {
		t.Errorf("expected __cpn_id__=test-cpn, got %v", out["__cpn_id__"])
	}
	if out["x"] != 1 {
		t.Errorf("expected input to pass through, got x=%v", out["x"])
	}
}

// echoComponent is a minimal runtime.Component used by the no-timeout test. It
// returns the input map unchanged plus a __cpn_id__ tag.
type echoComponent struct{}

func (e *echoComponent) Name() string { return "echo" }

func (e *echoComponent) Invoke(_ context.Context, _ *gorm.DB, in map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out, nil
}

func (e *echoComponent) Stream(_ context.Context, _ map[string]any) (<-chan map[string]any, error) {
	return nil, nil
}

func (e *echoComponent) Inputs() map[string]string  { return nil }
func (e *echoComponent) Outputs() map[string]string { return nil }

// Compile-time check that the stubs satisfy the interface.
var (
	_ runtime.Component = (*blockingComponent)(nil)
	_ runtime.Component = (*failingComponent)(nil)
	_ runtime.Component = (*echoComponent)(nil)
)
