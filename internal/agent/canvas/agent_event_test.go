package canvas

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"fmt"
	"github.com/cloudwego/eino/compose"
	"gorm.io/gorm"
	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	"time"
)

func TestAgentEventsRunAndRepeatedNodesWithoutSSE(t *testing.T) {
	var events []common.AgentEvent
	ctx := WithAgentEventSink(t.Context(), func(event common.AgentEvent) { events = append(events, event) })
	run := func(ctx context.Context, _ map[string]any) (*CanvasState, error) {
		state := NewCanvasState("lease", "")
		for range 2 {
			nodeStartedAt(ctx, state, "node", "LLM", "LLM", map[string]any{"password": "hidden", "nested": map[string]any{"api_key": "secret"}, "value": "hello"})
			state.SetVar("node", "answer", "done")
			nodeFinishedNow(ctx, state, "node", "LLM", "LLM", nil)
		}
		return state, nil
	}
	for range NewRunner().Run(ctx, run, "agent", "session", nil, map[string]any{"__message_id__": "message"}) {
	}
	if len(events) != 6 {
		t.Fatalf("got %d events, want start, two node pairs, completed: %+v", len(events), events)
	}
	if events[0].EventType != "run_started" || events[5].EventType != "run_completed" {
		t.Fatalf("run boundaries: %+v", events)
	}
	if events[1].ExecutionID == events[3].ExecutionID || events[1].ExecutionID == "" {
		t.Fatal("repeated node invocations share execution identity")
	}
	if events[1].ExecutionID != events[2].ExecutionID {
		t.Fatal("node start and completion do not correlate")
	}
	for i, event := range events {
		if event.Sequence != uint64(i+1) || event.RunID == "" || event.RunID == "lease" || event.AgentID != "agent" || event.MessageID != "message" {
			t.Fatalf("bad identity/order: %+v", event)
		}
	}
	if strings.Contains(events[1].InputsJSON, "hidden") || strings.Contains(events[1].InputsJSON, "secret") || !strings.Contains(events[1].InputsJSON, "hello") {
		t.Fatalf("unsafe/lost inputs: %s", events[1].InputsJSON)
	}
	if !strings.Contains(events[2].OutputsJSON, "done") {
		t.Fatalf("missing results: %+v", events[2])
	}
}

func TestAgentEventsClosePendingNodesOnFailure(t *testing.T) {
	for _, tc := range []struct {
		name     string
		err      error
		panicRun bool
		status   string
	}{
		{"error", errors.New("failure"), false, "failed"},
		{"cancel", context.Canceled, false, "cancelled"},
		{"provider timeout", context.DeadlineExceeded, false, "failed"},
		{"panic", nil, true, "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var events []common.AgentEvent
			ctx := WithAgentEventSink(t.Context(), func(event common.AgentEvent) { events = append(events, event) })
			run := func(ctx context.Context, _ map[string]any) (*CanvasState, error) {
				nodeStartedAt(ctx, NewCanvasState("lease", ""), "node", "LLM", "LLM", nil)
				if tc.panicRun {
					panic("broken")
				}
				return nil, tc.err
			}
			for range NewRunner().Run(ctx, run, "agent", "session", nil, map[string]any{}) {
			}
			if len(events) != 4 || events[2].Status != tc.status || events[3].Status != tc.status {
				t.Fatalf("pending node not finished coherently: %+v", events)
			}
		})
	}
}

func TestAgentEventsParallelAndIsolatedRuns(t *testing.T) {
	var events []common.AgentEvent
	var mu sync.Mutex
	ctx := WithAgentEventSink(t.Context(), func(event common.AgentEvent) { mu.Lock(); defer mu.Unlock(); events = append(events, event) })
	run := func(ctx context.Context, _ map[string]any) (*CanvasState, error) {
		var wg sync.WaitGroup
		for _, id := range []string{"a", "b", "c"} {
			wg.Go(func() {
				state := NewCanvasState("lease", "")
				nodeStartedAt(ctx, state, id, "LLM", "LLM", nil)
				nodeFinishedNow(ctx, state, id, "LLM", "LLM", nil)
			})
		}
		wg.Wait()
		return nil, nil
	}
	runner := NewRunner()
	for range runner.Run(ctx, run, "agent", "session", nil, map[string]any{}) {
	}
	firstRun := events[0].RunID
	for range runner.Run(ctx, run, "agent", "session", nil, map[string]any{}) {
	}
	if len(events) != 16 || events[8].RunID == firstRun {
		t.Fatalf("runs not isolated: %+v", events)
	}
	for i, event := range events {
		if event.Sequence != uint64(i%8+1) {
			t.Fatalf("parallel events unordered: %+v", events)
		}
	}
}

// The same DSL node runs concurrently for each Parallel item, each with its own state.
func TestAgentEventsParallelItemsKeepDistinctInvocations(t *testing.T) {
	var events []common.AgentEvent
	ctx := WithAgentEventSink(t.Context(), func(event common.AgentEvent) { events = append(events, event) })
	run := func(ctx context.Context, _ map[string]any) (*CanvasState, error) {
		a, b := NewCanvasState("lease", ""), NewCanvasState("lease", "")
		nodeStartedAt(ctx, a, "same", "LLM", "LLM", nil)
		nodeStartedAt(ctx, b, "same", "LLM", "LLM", nil)
		nodeFinishedNow(ctx, b, "same", "LLM", "LLM", nil)
		nodeFinishedNow(ctx, a, "same", "LLM", "LLM", nil)
		return nil, nil
	}
	for range NewRunner().Run(ctx, run, "agent", "session", nil, map[string]any{}) {
	}
	if len(events) != 6 {
		t.Fatalf("parallel invocation lost: %+v", events)
	}
	if events[1].ExecutionID != events[4].ExecutionID || events[2].ExecutionID != events[3].ExecutionID {
		t.Fatalf("parallel node completions crossed: %+v", events)
	}
}

func TestAgentEventsCompiledLoopIncludesIterationsAndParent(t *testing.T) {
	var events []common.AgentEvent
	ctx := WithAgentEventSink(t.Context(), func(event common.AgentEvent) { events = append(events, event) })
	run := func(ctx context.Context, _ map[string]any) (*CanvasState, error) {
		state := NewCanvasState("lease", "")
		ctx = WithState(ctx, state)
		c := counterLoopDSL(1, 3, 10)
		bump := c.Components["bump"]
		bump.DisplayName = "Increase counter"
		c.Components["bump"] = bump
		compiled, err := Compile(ctx, c)
		if err != nil {
			return state, err
		}
		_, err = compiled.Workflow.Invoke(ctx, map[string]any{})
		return state, err
	}
	for range NewRunner().Run(ctx, run, "agent", "session", nil, map[string]any{}) {
	}
	starts := 0
	ids := map[string]bool{}
	parent := ""
	for _, event := range events {
		if event.NodeID == "loop" && event.EventType == "node_started" {
			parent = event.ExecutionID
		}
		if event.NodeID == "bump" && event.EventType == "node_started" {
			starts++
			ids[event.ExecutionID] = true
			if event.ParentExecutionID != parent || parent == "" || event.NodeName != "Increase counter" {
				t.Fatalf("bad loop child metadata: %+v", event)
			}
		}
	}
	if starts != 3 || len(ids) != 3 || events[len(events)-1].Status != "completed" {
		t.Fatalf("missing loop iterations: %+v", events)
	}
}

func TestAgentEventsCompiledParallelLogsEveryItem(t *testing.T) {
	var events []common.AgentEvent
	ctx := WithAgentEventSink(t.Context(), func(event common.AgentEvent) { events = append(events, event) })
	run := func(ctx context.Context, _ map[string]any) (*CanvasState, error) {
		state := NewCanvasState("lease", "")
		state.SetSys("items", []any{"a", "b", "c"})
		ctx = WithState(ctx, state)
		c := &Canvas{Components: map[string]CanvasComponent{
			"parallel": {Obj: CanvasComponentObj{ComponentName: "Parallel", Params: map[string]any{"items_ref": "sys.items", "max_concurrency": 3}}},
			"child":    {Obj: CanvasComponentObj{ComponentName: "VariableAssigner", Params: map[string]any{"variables": []any{}}}, Upstream: []string{"parallel"}},
		}, NodeParents: map[string]string{"child": "parallel"}}
		compiled, err := Compile(ctx, c)
		if err != nil {
			return state, err
		}
		_, err = compiled.Workflow.Invoke(ctx, map[string]any{})
		return state, err
	}
	for range NewRunner().Run(ctx, run, "agent", "session", nil, map[string]any{}) {
	}
	started, finished := map[string]bool{}, map[string]bool{}
	parent := ""
	for _, event := range events {
		if event.NodeID == "parallel" && event.EventType == "node_started" {
			parent = event.ExecutionID
		}
		if event.NodeID != "child" {
			continue
		}
		if event.ParentExecutionID != parent || parent == "" {
			t.Fatalf("missing parallel parent: %+v", event)
		}
		if event.EventType == "node_started" {
			started[event.ExecutionID] = true
		} else if event.EventType == "node_completed" {
			finished[event.ExecutionID] = true
		}
	}
	if len(started) != 3 || len(finished) != 3 || events[len(events)-1].Status != "completed" {
		t.Fatalf("parallel item events incomplete: %+v", events)
	}
	for id := range started {
		if !finished[id] {
			t.Fatalf("unfinished parallel execution %s", id)
		}
	}
}

func TestAgentEventsCompiledUserFillUpPauseAndResume(t *testing.T) {
	var events []common.AgentEvent
	ctx := WithAgentEventSink(t.Context(), func(event common.AgentEvent) { events = append(events, event) })
	store, _ := newTestStore(t, time.Minute)
	c := &Canvas{Components: map[string]CanvasComponent{"input": {Obj: CanvasComponentObj{ComponentName: "UserFillUp", Params: map[string]any{"inputs": map[string]any{"value": map[string]any{"type": "line"}}}}}}}
	run := func(ctx context.Context, root map[string]any) (*CanvasState, error) {
		state := NewCanvasState("lease", "")
		ctx = WithState(ctx, state)
		compiled, err := Compile(ctx, c, WithCheckPointStore(store))
		if err != nil {
			return state, err
		}
		if id, _ := root["__resume_interrupt_id__"].(string); id != "" {
			ctx = compose.ResumeWithData(ctx, id, root["__resume_data__"])
		}
		_, err = compiled.Workflow.Invoke(ctx, map[string]any{}, compose.WithCheckPointID("telemetry-user-input"))
		return state, err
	}
	runner := NewRunner()
	for range runner.Run(ctx, run, "agent", "session", nil, map[string]any{}) {
	}
	if len(events) < 4 || events[len(events)-2].Status != "paused" || events[len(events)-1].EventType != "run_paused" {
		t.Fatalf("UserFillUp pause not logged: %+v", events)
	}
	pausedRun := events[0].RunID
	firstCount := len(events)
	for range runner.Run(ctx, run, "agent", "session", "answer", map[string]any{}) {
	}
	if events[firstCount].EventType != "run_resumed" || events[firstCount].RunID == pausedRun || events[len(events)-1].EventType != "run_completed" {
		t.Fatalf("resume not logged as a fresh invocation: %+v", events)
	}
}

func TestAgentEventsDeferredNodeCompletesAfterConsumption(t *testing.T) {
	var events []common.AgentEvent
	ctx := WithAgentEventSink(t.Context(), func(event common.AgentEvent) { events = append(events, event) })
	run := func(ctx context.Context, _ map[string]any) (*CanvasState, error) {
		state := NewCanvasState("lease", "")
		ctx = runtime.WithDeferredNodeRegistry(WithState(ctx, state))
		body := withStateBracket("producer", "Agent", func(context.Context, map[string]any) (map[string]any, error) {
			return map[string]any{"__cpn_id__": "producer", "content": &runtime.DeferredStream{Open: func(context.Context, runtime.AgentDeltaSink) (map[string]any, error) { return nil, nil }}}, nil
		})
		if _, err := body(ctx, nil); err != nil {
			return state, err
		}
		if len(events) != 2 {
			t.Fatalf("producer completed before stream consumption: %+v", events)
		}
		state.SetVar("producer", "content", "final answer")
		runtime.CompleteDeferredNode(ctx, "producer", nil)
		runtime.CompleteDeferredNode(ctx, "producer", nil)
		return state, nil
	}
	for range NewRunner().Run(ctx, run, "agent", "session", nil, map[string]any{}) {
	}
	if len(events) != 4 || events[2].EventType != "node_completed" || !strings.Contains(events[2].OutputsJSON, "final answer") {
		t.Fatalf("deferred result not logged once: %+v", events)
	}
}

func TestAgentEventsSinkFailureDoesNotFailWorkflow(t *testing.T) {
	for _, ctx := range []context.Context{t.Context(), WithAgentEventSink(t.Context(), func(common.AgentEvent) { panic("collector unavailable") })} {
		completed := false
		run := func(ctx context.Context, _ map[string]any) (*CanvasState, error) {
			state := NewCanvasState("lease", "")
			nodeStartedAt(ctx, state, "node", "Begin", "Begin", nil)
			nodeFinishedNow(ctx, state, "node", "Begin", "Begin", nil)
			completed = true
			return state, nil
		}
		for range NewRunner().Run(ctx, run, "agent", "session", nil, nil) {
		}
		if !completed {
			t.Fatal("optional observability changed workflow execution")
		}
	}
}

func TestAgentEventsRecoveredNodeFailureRemainsVisible(t *testing.T) {
	var events []common.AgentEvent
	ctx := WithAgentEventSink(t.Context(), func(event common.AgentEvent) { events = append(events, event) })
	run := func(ctx context.Context, _ map[string]any) (*CanvasState, error) {
		state := NewCanvasState("lease", "")
		ctx = WithState(ctx, state)
		ctx = WithComponentFactory(ctx, func(string, map[string]any) (runtime.Component, error) { return &failingComponent{}, nil })
		c := &Canvas{Components: map[string]CanvasComponent{"node": {Obj: CanvasComponentObj{ComponentName: "Failing", Params: map[string]any{"exception_method": "comment", "exception_default_value": "fallback"}}}}}
		compiled, err := Compile(ctx, c)
		if err != nil {
			return state, err
		}
		_, err = compiled.Workflow.Invoke(ctx, map[string]any{})
		return state, err
	}
	for range NewRunner().Run(ctx, run, "agent", "session", nil, map[string]any{}) {
	}
	if len(events) != 4 || events[2].EventType != "node_failed" || !strings.Contains(events[2].ErrorMessage, "boom") || events[3].Status != "completed" {
		t.Fatalf("recovered failure disappeared: %+v", events)
	}
}

func TestAgentEventsCancellationRetainsPendingPartialOutputs(t *testing.T) {
	var events []common.AgentEvent
	ctx := WithAgentEventSink(t.Context(), func(event common.AgentEvent) { events = append(events, event) })
	run := func(ctx context.Context, _ map[string]any) (*CanvasState, error) {
		state := NewCanvasState("lease", "")
		ctx = WithState(ctx, state)
		nodeStartedAt(ctx, state, "node", "Agent", "Agent", nil)
		state.SetVar("node", "content", "partial answer")
		return nil, context.Canceled
	}
	for range NewRunner().Run(ctx, run, "agent", "session", nil, map[string]any{}) {
	}
	if len(events) != 4 || !strings.Contains(events[2].OutputsJSON, "partial answer") {
		t.Fatalf("cancelled partial result lost: %+v", events)
	}
}

type deferredAgentEventComponent struct {
	firstOpened chan struct{}
	opened      chan string
	release     map[string]chan struct{}
}

func (c *deferredAgentEventComponent) Invoke(ctx context.Context, _ *gorm.DB, _ map[string]any) (map[string]any, error) {
	state, _ := runtime.GetStateFromContext(ctx)
	item, _ := state.GetVar("item")
	label := fmt.Sprint(item)
	if label == "second" {
		<-c.firstOpened
	}
	return map[string]any{"content": &runtime.DeferredStream{Open: func(ctx context.Context, _ runtime.AgentDeltaSink) (map[string]any, error) {
		if label == "first" {
			close(c.firstOpened)
		}
		c.opened <- label
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-c.release[label]:
		}
		return map[string]any{"content": label + "-result"}, nil
	}}}, nil
}

func TestAgentEventsParallelDeferredRegistryKeepsItemLifecycle(t *testing.T) {
	var events []common.AgentEvent
	secondCompleted := make(chan struct{})
	ctx := WithAgentEventSink(t.Context(), func(event common.AgentEvent) {
		events = append(events, event)
		if event.NodeID == "producer" && event.EventType == "node_completed" && strings.Contains(event.OutputsJSON, "second-result") {
			close(secondCompleted)
		}
	})
	component := &deferredAgentEventComponent{firstOpened: make(chan struct{}), opened: make(chan string, 2), release: map[string]chan struct{}{"first": make(chan struct{}), "second": make(chan struct{})}}
	run := func(ctx context.Context, _ map[string]any) (*CanvasState, error) {
		state := NewCanvasState("lease", "")
		state.SetSys("items", []any{"first", "second"})
		ctx = runtime.WithDeferredNodeRegistry(WithState(ctx, state))
		ctx = WithComponentFactory(ctx, func(name string, params map[string]any) (runtime.Component, error) {
			if name == "Agent" {
				return component, nil
			}
			return runtime.DefaultFactory()(name, params)
		})
		c := &Canvas{Components: map[string]CanvasComponent{
			"parallel": {Obj: CanvasComponentObj{ComponentName: "Parallel", Params: map[string]any{"items_ref": "sys.items", "max_concurrency": 2}}},
			"producer": {Obj: CanvasComponentObj{ComponentName: "Agent"}, Upstream: []string{"parallel"}, Downstream: []string{"message"}},
			"message":  {Obj: CanvasComponentObj{ComponentName: "Message", Params: map[string]any{"text": "{producer@content}"}}, Upstream: []string{"producer"}},
		}, NodeParents: map[string]string{"producer": "parallel", "message": "parallel"}}
		compiled, err := Compile(ctx, c)
		if err != nil {
			return state, err
		}
		_, err = compiled.Workflow.Invoke(ctx, map[string]any{})
		return state, err
	}
	stream := NewRunner().Run(ctx, run, "agent", "session", nil, map[string]any{})
	for range 2 {
		select {
		case <-component.opened:
		case <-time.After(5 * time.Second):
			t.Fatal("parallel streams did not open")
		}
	}
	close(component.release["second"])
	select {
	case <-secondCompleted:
	case <-time.After(5 * time.Second):
		t.Fatal("second producer did not finish")
	}
	close(component.release["first"])
	for range stream {
	}
	for _, label := range []string{"first-result", "second-result"} {
		producer, message := -1, -1
		for i, event := range events {
			if event.EventType != "node_completed" || !strings.Contains(event.OutputsJSON, label) {
				continue
			}
			if event.NodeID == "producer" {
				producer = i
			}
			if event.NodeID == "message" {
				message = i
			}
		}
		if producer < 0 || message < 0 || producer > message {
			t.Errorf("%s producer must finish before its Message: producer=%d message=%d", label, producer, message)
		}
	}
}

func TestAgentEventsLoopSuccessDoesNotReusePreviousHandledError(t *testing.T) {
	var events []common.AgentEvent
	ctx := WithAgentEventSink(t.Context(), func(event common.AgentEvent) { events = append(events, event) })
	run := func(ctx context.Context, _ map[string]any) (*CanvasState, error) {
		state := NewCanvasState("lease", "")
		nodeStartedAt(ctx, state, "node", "LLM", "LLM", nil)
		state.SetVar("node", "_ERROR", "first attempt error")
		nodeFinishedNow(ctx, state, "node", "LLM", "LLM", errors.New("first attempt error"))
		nodeStartedAt(ctx, state, "node", "LLM", "LLM", nil)
		nodeFinishedNow(ctx, state, "node", "LLM", "LLM", nil)
		return state, nil
	}
	for range NewRunner().Run(ctx, run, "agent", "session", nil, map[string]any{}) {
	}
	if len(events) != 6 || events[2].Status != "failed" || events[4].Status != "completed" {
		t.Fatalf("previous handled error poisoned later success: %+v", events)
	}
}

func TestAgentEventsNestedOperationsKeepParentsAndRetryAttempts(t *testing.T) {
	var events []common.AgentEvent
	ctx := WithAgentEventSink(t.Context(), func(event common.AgentEvent) { events = append(events, event) })
	run := func(ctx context.Context, _ map[string]any) (*CanvasState, error) {
		state := NewCanvasState("lease", "")
		ctx = WithState(ctx, state)
		nodeStartedAt(ctx, state, "agent_node", "Agent", "Agent", nil)
		ctx = withAgentNodeParent(ctx, "agent_node")
		childCtx, finishTool := runtime.ObserveAgentNode(ctx, "tool:search", "Search", "Tool", 1, map[string]any{"query": "hello"})
		for attempt := uint32(1); attempt <= 2; attempt++ {
			_, finishModel := runtime.ObserveAgentNode(childCtx, "model", "Chat", "Model", attempt, map[string]any{"prompt": "hello"})
			if attempt == 1 {
				finishModel(nil, errors.New("retryable"))
			} else {
				finishModel(map[string]any{"content": "answer"}, nil)
			}
		}
		finishTool(map[string]any{"result": "found"}, nil)
		nodeFinishedNow(ctx, state, "agent_node", "Agent", "Agent", nil)
		return state, nil
	}
	for range NewRunner().Run(ctx, run, "agent", "session", nil, map[string]any{}) {
	}
	if len(events) != 10 {
		t.Fatalf("nested events missing: %+v", events)
	}
	parent := events[1].ExecutionID
	tool := events[2].ExecutionID
	if events[2].ParentExecutionID != parent || events[3].ParentExecutionID != tool || events[5].ParentExecutionID != tool || events[3].Attempt != 1 || events[5].Attempt != 2 || events[3].ExecutionID == events[5].ExecutionID {
		t.Fatalf("nested parent/attempt identity incorrect: %+v", events)
	}
}
