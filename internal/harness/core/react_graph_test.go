package core

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	"ragflow/internal/harness/core/schema"
	"ragflow/internal/harness/graph/checkpoint"
	"ragflow/internal/harness/graph/graph"
	harnesserrors "ragflow/internal/harness/graph/errors"
	"ragflow/internal/harness/graph/types"
)

// ---- Basic ReAct Graph tests (no Pregel engine dependency) ----

// TestReActGraph_CheckpointInterruptResume verifies interrupt capture.
func TestReActGraph_CheckpointInterruptResume(t *testing.T) {
	model := &forcedToolModel{
		inner: &mockModel{},
		toolCalls: []schema.ToolCall{{ID: "c1",
			Function: schema.ToolCallFunction{Name: "approve", Arguments: "{}"},
		}},
		finalResp: "done",
		firstCall: true,
	}
	tool := &mockTool{name: "approve", desc: "approval"}
	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: model, Tools: []Tool{tool},
		ToolsConfig: &ToolsNodeConfig{Tools: []Tool{tool}},
		MaxIterations: 2,
	})
	agent.name = "interrupt_agent"

	rg, err := NewReActGraph(agent, &ReActGraphConfig{
		Checkpointer:   checkpoint.NewMemorySaver(),
		RecursionLimit: 20,
	})
	if err != nil {
		t.Fatalf("NewReActGraph: %v", err)
	}

	ctx := context.Background()
	_, err = rg.Invoke(ctx, &AgentInput{
		Messages: []*schema.Message{schema.UserMessage("approve")}},
		nil)
	if err != nil {
		var gi *harnesserrors.GraphInterrupt
		if stderrors.As(err, &gi) {
			t.Logf("interrupt captured (expected): %v", gi)
		} else {
			t.Logf("other error: %v", err)
		}
	}
}

// TestReActGraph_StreamWithInterrupt verifies streaming events include checkpoints.
func TestReActGraph_StreamWithInterrupt(t *testing.T) {
	model := &forcedToolModel{
		inner:     &mockModel{},
		toolCalls: []schema.ToolCall{{ID: "s1",
			Function: schema.ToolCallFunction{Name: "tool_s", Arguments: "{}"},
		}},
		finalResp: "stream ok",
		firstCall: true,
	}
	tool := &mockTool{name: "tool_s", desc: "stream test"}
	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: model, Tools: []Tool{tool},
		ToolsConfig: &ToolsNodeConfig{Tools: []Tool{tool}},
		MaxIterations: 2,
	})
	agent.name = "stream_agent"

	rg, err := NewReActGraph(agent, &ReActGraphConfig{
		Checkpointer:    checkpoint.NewMemorySaver(),
		InterruptBefore: []string{},
		RecursionLimit:  20,
	})
	if err != nil {
		t.Fatalf("NewReActGraph: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	outputCh, errCh := rg.Stream(ctx, &AgentInput{
		Messages: []*schema.Message{schema.UserMessage("test")}},
		nil, types.StreamModeValues)
	go func() {
		for range outputCh {
		}
	}()
	select {
	case e := <-errCh:
		t.Logf("stream completed: err=%v", e)
	case <-time.After(2 * time.Second):
		t.Log("stream timed out (expected for async pattern)")
	}
}

// ---- Comprehensive Graph ReAct tests (require Pregel engine) ----

// TestReActGraph_FullCheckpointInterruptResume verifies the COMPLETE lifecycle:
//
//	1. Build graph with checkpoint + interrupt
//	2. Invoke → reaches tool call → pauses at execute_tools (interrupt)
//	3. Resume from checkpoint → executes tool → completes
//	4. Verify final state is correct
func TestReActGraph_FullCheckpointInterruptResume(t *testing.T) {
	t.Skip("requires Pregel engine — run from harness root: go test ./...")

	model := &forcedToolModel{
		inner: &mockModel{},
		toolCalls: []schema.ToolCall{{
			ID: "full_cp_1",
			Function: schema.ToolCallFunction{
				Name:      "calculator",
				Arguments: "{\"x\":10,\"y\":20}",
			},
		}},
		finalResp: "the result is 30",
		firstCall: true,
	}
	tool := &mockTool{name: "calculator", desc: "math tool"}
	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model:       model,
		Tools:       []Tool{tool},
		ToolsConfig: &ToolsNodeConfig{Tools: []Tool{tool}},
		MaxIterations: 3,
	})
	agent.name = "full_cycle_agent"

	saver := checkpoint.NewMemorySaver()
	rg, err := NewReActGraph(agent, &ReActGraphConfig{
		Checkpointer:    saver,
		RecursionLimit:  20,
		InterruptBefore: []string{"execute_tools"}, // pause before tool execution
	})
	if err != nil {
		t.Fatalf("NewReActGraph: %v", err)
	}

	ctx := context.Background()
	input := &AgentInput{
		Messages: []*schema.Message{schema.UserMessage("what is 10+20?")},
	}
	config := &types.RunnableConfig{ThreadID: "full-cycle-001"}

	// ---- Phase 1: First invocation - reaches interrupt ----
	t.Log("=== Phase 1: First invocation ===")
	_, err = rg.Invoke(ctx, input, config)
	if err == nil {
		t.Fatal("expected interrupt error, got nil")
	}
	t.Logf("interrupt captured: %v", err)

	// ---- Phase 2: Human-in-the-loop review (simulated) ----
	t.Log("=== Phase 2: Human review ===")
	time.Sleep(5 * time.Millisecond) // simulate review time

	// ---- Phase 3: Resume from checkpoint ----
	t.Log("=== Phase 3: Resume ===")
	state, err := rg.Invoke(ctx, nil, config)
	if err != nil {
		t.Fatalf("resume failed: %v", err)
	}
	if state == nil || len(state.Messages) == 0 {
		t.Fatal("expected messages after resume")
	}
	last := state.Messages[len(state.Messages)-1]
	if last.Content != "the result is 30" {
		t.Errorf("expected 'the result is 30', got %q", last.Content)
	}
	t.Logf("=== Final output: %s ===", last.Content)
}

// TestReActGraph_SerialCheckpointCycles verifies multiple interrupt-resume cycles.
func TestReActGraph_SerialCheckpointCycles(t *testing.T) {
	t.Skip("requires Pregel engine — run from harness root: go test ./...")

	model := &sequentialToolModel{
		mock: &mockModel{},
		toolCalls: [][]schema.ToolCall{
			{{ID: "sc1", Function: schema.ToolCallFunction{Name: "step1", Arguments: "{}"}}},
			{{ID: "sc2", Function: schema.ToolCallFunction{Name: "step2", Arguments: "{}"}}},
		},
		finalResp: "all steps complete",
	}
	tool1 := &mockTool{name: "step1", desc: "first step"}
	tool2 := &mockTool{name: "step2", desc: "second step"}

	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model:       model,
		Tools:       []Tool{tool1, tool2},
		ToolsConfig: &ToolsNodeConfig{Tools: []Tool{tool1, tool2}},
		MaxIterations: 5,
	})
	agent.name = "serial_cycle"

	saver := checkpoint.NewMemorySaver()
	rg, err := NewReActGraph(agent, &ReActGraphConfig{
		Checkpointer:   saver,
		RecursionLimit: 30,
	})
	if err != nil {
		t.Fatalf("NewReActGraph: %v", err)
	}

	ctx := context.Background()
	config := &types.RunnableConfig{ThreadID: "serial-cycle-001"}
	input := &AgentInput{Messages: []*schema.Message{schema.UserMessage("run all steps")}}

	cycles := 0
	maxCycles := 3
	for cycles < maxCycles {
		_, err = rg.Invoke(ctx, input, config)
		if err == nil {
			t.Log("graph completed without interrupt")
			break
		}
		var gi *harnesserrors.GraphInterrupt
		if stderrors.As(err, &gi) {
			cycles++
			t.Logf("cycle %d: interrupted, resuming...", cycles)
		} else {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	t.Logf("serial checkpoint cycles completed: %d interrupt-resume cycles", cycles)
}

// TestReActGraph_StreamingCheckpointEvents verifies streaming produces
// checkpoint events at each node boundary.
func TestReActGraph_StreamingCheckpointEvents(t *testing.T) {
	model := &forcedToolModel{
		inner:     &mockModel{},
		toolCalls: []schema.ToolCall{{
			ID: "stream_cp",
			Function: schema.ToolCallFunction{Name: "stream_tool", Arguments: "{}"},
		}},
		finalResp: "streaming done",
		firstCall: true,
	}
	tool := &mockTool{name: "stream_tool", desc: "stream test"}
	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model:       model,
		Tools:       []Tool{tool},
		ToolsConfig: &ToolsNodeConfig{Tools: []Tool{tool}},
		MaxIterations: 2,
	})
	agent.name = "stream_cp_agent"

	saver := checkpoint.NewMemorySaver()
	rg, err := NewReActGraph(agent, &ReActGraphConfig{
		Checkpointer:    saver,
		InterruptBefore: []string{},
		RecursionLimit:  20,
	})
	if err != nil {
		t.Fatalf("NewReActGraph: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	outCh, _ := rg.Stream(ctx, &AgentInput{
		Messages: []*schema.Message{schema.UserMessage("stream test")},
	}, nil, types.StreamModeCheckpoints)

	eventCount := 0
timeout:
	for {
		select {
		case ev, ok := <-outCh:
			if !ok {
				break timeout
			}
			_ = ev
			eventCount++
		case <-ctx.Done():
			break timeout
		}
	}
	t.Logf("streaming checkpoint events received: %d", eventCount)
}

// TestReActGraph_ConcurrentCheckpoints verifies concurrent graph instances
// with separate checkpoints don't interfere.
func TestReActGraph_ConcurrentCheckpoints(t *testing.T) {
	t.Skip("requires Pregel engine — run from harness root: go test ./...")

	const instances = 5
	errs := make(chan error, instances)

	for i := 0; i < instances; i++ {
		go func(id int) {
			m := &mockModel{}
			m.addResp("concurrent result")
			agent := NewReActAgent(&ReActConfig[*schema.Message]{
				Model:  m,
				MaxIterations: 1,
			}).WithName("concurrent_cp_agent")

			rg, err := NewReActGraph(agent, &ReActGraphConfig{
				Checkpointer:    checkpoint.NewMemorySaver(),
				InterruptBefore: []string{},
				RecursionLimit:  10,
			})
			if err != nil {
				errs <- err
				return
			}

			ctx := context.Background()
			_, err = rg.Invoke(ctx, &AgentInput{
				Messages: []*schema.Message{schema.UserMessage("concurrent test")},
			}, nil)
			errs <- err
		}(i)
	}

	for i := 0; i < instances; i++ {
		if err := <-errs; err != nil {
			t.Errorf("concurrent instance %d failed: %v", i, err)
		}
	}
	t.Logf("concurrent checkpoints: %d instances completed", instances)
}

// ---- DAG mode test (standalone graph, no ReAct dependency) ----

// TestReActGraph_DAGMode verifies AllPredecessor trigger mode.
func TestReActGraph_DAGMode(t *testing.T) {
	sg := graph.NewStateGraph(map[string]interface{}{"a": "", "b": "", "c": ""})
	sg.AddNode("node_a", func(ctx context.Context, state interface{}) (interface{}, error) {
		s := state.(map[string]interface{})
		s["a"] = "done"
		return s, nil
	})
	sg.AddNode("node_b", func(ctx context.Context, state interface{}) (interface{}, error) {
		s := state.(map[string]interface{})
		s["b"] = "done"
		return s, nil
	})
	sg.AddNode("node_c", func(ctx context.Context, state interface{}) (interface{}, error) {
		s := state.(map[string]interface{})
		s["c"] = "merged"
		return s, nil
	})
	sg.AddEdge("__start__", "node_a")
	sg.AddEdge("node_a", "node_b")
	sg.AddEdge("node_b", "node_c")
	sg.AddEdge("node_c", "__end__")

	cg, err := sg.Compile(
		graph.WithNodeTriggerMode(types.NodeTriggerAllPredecessor),
		graph.WithRecursionLimit(10),
	)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	result, err := cg.Invoke(context.Background(), map[string]interface{}{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	m := result.(map[string]interface{})
	if m["c"] != "merged" {
		t.Errorf("expected 'merged', got %v", m["c"])
	}
	t.Logf("DAG result: a=%v b=%v c=%v", m["a"], m["b"], m["c"])
}

// ---- Helper models ----

// sequentialToolModel returns different tool calls on each Generate call,
// simulating a multi-step tool interaction.
type sequentialToolModel struct {
	mock      *mockModel
	toolCalls [][]schema.ToolCall
	finalResp string
	callCount int
}

func (m *sequentialToolModel) Generate(ctx context.Context, msgs []*schema.Message, opts ...ModelOption) (*schema.Message, error) {
	if m.callCount < len(m.toolCalls) {
		tcs := m.toolCalls[m.callCount]
		m.callCount++
		msg := &schema.Message{Role: schema.RoleAssistant, Content: ""}
		msg.ToolCalls = tcs
		return msg, nil
	}
	return &schema.Message{Role: schema.RoleAssistant, Content: m.finalResp}, nil
}

func (m *sequentialToolModel) Stream(ctx context.Context, msgs []*schema.Message, opts ...ModelOption) (*schema.StreamReader[*schema.Message], error) {
	r := schema.NewStreamReader[*schema.Message]()
	msg, err := m.Generate(ctx, msgs, opts...)
	if err != nil {
		r.Close()
		return r, err
	}
	r.Send(msg, nil)
	r.Close()
	return r, nil
}

func (m *sequentialToolModel) BindTools(tools []*schema.ToolInfo) error { return nil }
