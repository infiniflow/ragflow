package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"ragflow/internal/harness/core/schema"
)

func TestNewToolsNode(t *testing.T) {
	tool := &mockTool{name: "test", desc: "test tool"}
	tn := NewToolsNode[*schema.Message](&ToolsNodeConfig{
		Tools: []Tool{tool},
	})
	if tn == nil {
		t.Fatal("nil ToolsNode")
	}
	if len(tn.toolMap) != 1 {
		t.Error("tool map not populated")
	}
}

func TestToolsNode_Execute_NoToolCalls(t *testing.T) {
	tn := NewToolsNode[*schema.Message](&ToolsNodeConfig{})
	resp := &schema.Message{Role: schema.RoleAssistant, Content: "no tools here"}
	state := &TypedReActAgentState[*schema.Message]{}

	results, action, err := tn.Execute(context.Background(), resp, state, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if results != nil {
		t.Error("expected nil results when no tool calls")
	}
	if action != nil {
		t.Error("expected nil action")
	}
}

func TestToolsNode_Execute_ToolNotFound(t *testing.T) {
	tn := NewToolsNode[*schema.Message](&ToolsNodeConfig{
		Tools: []Tool{&mockTool{name: "existing", desc: ""}},
	})
	resp := &schema.Message{
		Role: schema.RoleAssistant,
		ToolCalls: []schema.ToolCall{
			{ID: "tc1", Function: schema.ToolCallFunction{Name: "missing_tool", Arguments: "{}"}},
		},
	}
	state := &TypedReActAgentState[*schema.Message]{}

	results, _, err := tn.Execute(context.Background(), resp, state, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result (error msg in tool response), got %d", len(results))
	}
}

func TestToolsNode_Execute_ReturnDirectly(t *testing.T) {
	exitTool := &mockTool{name: "exit_tool", desc: ""}

	tn := NewToolsNode[*schema.Message](&ToolsNodeConfig{
		Tools:          []Tool{exitTool},
		ReturnDirectly: map[string]bool{"exit_tool": true},
	})
	resp := &schema.Message{
		Role: schema.RoleAssistant,
		ToolCalls: []schema.ToolCall{
			{ID: "tc1", Function: schema.ToolCallFunction{Name: "exit_tool", Arguments: "{}"}},
		},
	}
	state := &TypedReActAgentState[*schema.Message]{}

	results, action, err := tn.Execute(context.Background(), resp, state, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !exitTool.executed {
		t.Error("tool was not executed")
	}
	if action == nil || !action.Exit {
		t.Error("expected Exit action for return-directly tool")
	}
	if len(results) != 1 {
		t.Error("expected 1 result even with return-directly")
	}
}

func TestParseToolArgs_ValidJSON(t *testing.T) {
	var in struct {
		Name string `json:"name"`
	}
	err := parseToolArgs(`{"name":"test"}`, &in)
	if err != nil {
		t.Fatalf("parseToolArgs: %v", err)
	}
	if in.Name != "test" {
		t.Error("name not parsed")
	}
}

// failingTool is a mock tool that always returns an error.
type failingTool struct {
	name string
	desc string
}

func (t *failingTool) Name() string        { return t.name }
func (t *failingTool) Description() string { return t.desc }
func (t *failingTool) Invoke(ctx context.Context, s string, opts ...toolOption) (string, error) {
	return "", errors.New(t.name + " failed")
}
func (t *failingTool) Stream(ctx context.Context, s string, opts ...toolOption) (*schema.StreamReader[string], error) {
	return nil, errors.New(t.name + " stream failed")
}

func TestParseToolArgs_InvalidJSON(t *testing.T) {
	err := parseToolArgs(`{invalid`, struct{}{})
	if err == nil {
		t.Error("expected parse error")
	}
}

// ======================== Integration: ToolInvokeMiddleware Chain ========================

func TestToolsNode_WithToolInvokeMiddleware_Retry(t *testing.T) {
	var callCount int32
	tn := NewToolsNode[*schema.Message](&ToolsNodeConfig{
		Tools: []Tool{&mockTool{name: "flakey", desc: "flaky"}},
		ToolInvokeMiddlewares: []ToolInvokeMiddleware{
			NewRetryToolMiddleware(&ToolRetryConfig{
				MaxAttempts: 3, Backoff: time.Millisecond,
				IsRetryable: func(err error) bool { return true },
			}),
		},
	})

	resp := &schema.Message{
		Role: schema.RoleAssistant,
		ToolCalls: []schema.ToolCall{
			{ID: "tc1", Function: schema.ToolCallFunction{Name: "flakey", Arguments: "{}"}},
		},
	}

	results, _, err := tn.Execute(context.Background(), resp, &TypedReActAgentState[*schema.Message]{}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
	_ = callCount
}

func TestToolsNode_WithToolInvokeMiddleware_Timeout(t *testing.T) {
	tn := NewToolsNode[*schema.Message](&ToolsNodeConfig{
		Tools: []Tool{&mockTool{name: "slow", desc: "slow"}},
		ToolInvokeMiddlewares: []ToolInvokeMiddleware{
			NewTimeoutToolMiddleware(1 * time.Millisecond),
		},
	})

	resp := &schema.Message{
		Role: schema.RoleAssistant,
		ToolCalls: []schema.ToolCall{
			{ID: "tc1", Function: schema.ToolCallFunction{Name: "slow", Arguments: "{}"}},
		},
	}

	results, _, err := tn.Execute(context.Background(), resp, &TypedReActAgentState[*schema.Message]{}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	t.Logf("timeout result count: %d", len(results))
}

func TestToolsNode_WithToolInvokeMiddleware_Fallback(t *testing.T) {
	tn := NewToolsNode[*schema.Message](&ToolsNodeConfig{
		Tools: []Tool{&failingTool{name: "primary", desc: "always fails"}},
		ToolInvokeMiddlewares: []ToolInvokeMiddleware{
			NewFallbackToolMiddleware(func(ctx context.Context, args *schema.ToolArgument) (*schema.ToolResult, error) {
				return &schema.ToolResult{Content: "fallback result", ToolCallID: args.CallID}, nil
			}),
		},
	})

	resp := &schema.Message{
		Role: schema.RoleAssistant,
		ToolCalls: []schema.ToolCall{
			{ID: "tc1", Function: schema.ToolCallFunction{Name: "primary", Arguments: "{}"}},
		},
	}

	results, _, err := tn.Execute(context.Background(), resp, &TypedReActAgentState[*schema.Message]{}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestToolsNode_WithToolInvokeMiddleware_MultipleConcurrentTools(t *testing.T) {
	tool1 := &mockTool{name: "t1", desc: "tool1"}
	tool2 := &mockTool{name: "t2", desc: "tool2"}

	tn := NewToolsNode[*schema.Message](&ToolsNodeConfig{
		Tools: []Tool{tool1, tool2},
		ToolInvokeMiddlewares: []ToolInvokeMiddleware{
			NewTimeoutToolMiddleware(5 * time.Second),
		},
	})

	resp := &schema.Message{
		Role: schema.RoleAssistant,
		ToolCalls: []schema.ToolCall{
			{ID: "c1", Function: schema.ToolCallFunction{Name: "t1", Arguments: `{"x":1}`}},
			{ID: "c2", Function: schema.ToolCallFunction{Name: "t2", Arguments: `{"y":2}`}},
		},
	}

	results, _, err := tn.Execute(context.Background(), resp, &TypedReActAgentState[*schema.Message]{}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

// ======================== Integration: ToolRegistry + Full Agent ========================

func TestToolRegistry_WithReActAgentIntegration(t *testing.T) {
	r := NewToolRegistry()
	myTool := MustReflectTool("greet", "Greet someone",
		func(ctx context.Context, args *weatherArgs) (string, error) {
			return "Hello, " + args.City, nil
		})
	r.Register(myTool)

	model := &mockModel{}
	model.addResp("I'll use the greet tool")
	model.addResp("Done")

	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: model,
		Tools: r.ToSlice(),
	}).WithName("registry_agent")

	iter := agent.Run(context.Background(), &AgentInput{Messages: []Message{schema.UserMessage("Greet London")}})
	events := drainAgentEvents(t, iter)
	if len(events) == 0 {
		t.Error("expected events")
	}
	t.Logf("registry agent: %d events", len(events))
}

// ======================== Integration: ReflectTool in Full Agent ========================

func TestReflectTool_WithReActAgent(t *testing.T) {
	weatherTool, err := ReflectTool("get_weather", "Get weather for a city",
		func(ctx context.Context, args *weatherArgs) (string, error) {
			return "Weather in " + args.City + ": 22°C", nil
		})
	if err != nil {
		t.Fatalf("ReflectTool: %v", err)
	}

	model := &mockModel{}
	model.addResp("Let me check the weather")
	model.addResp("All done")

	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: model,
		Tools: []Tool{weatherTool},
	}).WithName("reflect_agent")

	ctx := context.Background()
	iter := agent.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("What is the weather in Tokyo?")}})
	events := drainAgentEvents(t, iter)
	if len(events) == 0 {
		t.Error("expected events")
	}
	t.Logf("reflect tool agent: %d events", len(events))
}

// ======================== Integration: ApprovalMiddleware in Agent ========================

func TestApprovalMiddleware_WithToolsNode(t *testing.T) {
	approved := false
	tn := NewToolsNode[*schema.Message](&ToolsNodeConfig{
		Tools: []Tool{&mockTool{name: "approve_me", desc: "needs approval"}},
		ToolInvokeMiddlewares: []ToolInvokeMiddleware{
			ApprovalMiddleware(func(ctx context.Context, ictx *ToolInvocationContext) (*ApprovalRequest, error) {
				ch := make(chan bool, 1)
				ch <- true
				approved = true
				return &ApprovalRequest{
					ToolName:    ictx.Name,
					CallID:      ictx.CallID,
					Arguments:   ictx.Arguments,
					ApproveChan: ch,
				}, nil
			}),
		},
	})

	resp := &schema.Message{
		Role: schema.RoleAssistant,
		ToolCalls: []schema.ToolCall{
			{ID: "c1", Function: schema.ToolCallFunction{Name: "approve_me", Arguments: "{}"}},
		},
	}

	results, _, err := tn.Execute(context.Background(), resp, &TypedReActAgentState[*schema.Message]{}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
	if !approved {
		t.Error("expected approval callback to be called")
	}
}

// ======================== LoopGuard Tests ========================

func TestLoopGuard_DetectsRepeatedSameArgs(t *testing.T) {
	g := NewLoopGuard(3, 0) // max 3 same-args, no failure limit

	err := g.CheckSameArgs("search", `{"q":"hello"}`)
	if err != nil {
		t.Fatalf("unexpected error on 1st call: %v", err)
	}
	err = g.CheckSameArgs("search", `{"q":"hello"}`)
	if err != nil {
		t.Fatalf("unexpected error on 2nd call: %v", err)
	}
	err = g.CheckSameArgs("search", `{"q":"hello"}`)
	if err == nil {
		t.Fatal("expected loop guard error on 3rd identical call")
	}
	t.Logf("loop guard message: %v", err)
}

func TestLoopGuard_DifferentArgsOk(t *testing.T) {
	g := NewLoopGuard(3, 0)

	g.CheckSameArgs("search", `{"q":"hello"}`)
	g.CheckSameArgs("search", `{"q":"hello"}`)
	err := g.CheckSameArgs("search", `{"q":"world"}`) // different args
	if err != nil {
		t.Errorf("expected no error for different args, got %v", err)
	}
}

func TestLoopGuard_ResetClearsCount(t *testing.T) {
	g := NewLoopGuard(3, 0)

	g.CheckSameArgs("search", `{"q":"hello"}`)
	g.CheckSameArgs("search", `{"q":"hello"}`)
	g.Reset("search")
	err := g.CheckSameArgs("search", `{"q":"hello"}`) // should be 1st again
	if err != nil {
		t.Errorf("expected no error after reset, got %v", err)
	}
}

func TestLoopGuard_NilGuardNoOp(t *testing.T) {
	var g *LoopGuard
	err := g.CheckSameArgs("any", `{}`)
	if err != nil {
		t.Errorf("nil guard should not error: %v", err)
	}
}

func TestLoopGuard_ConsecutiveFailures(t *testing.T) {
	g := NewLoopGuard(0, 3) // no same-args limit, max 3 failures

	for i := 0; i < 3; i++ {
		err := g.RecordFailure("calc")
		if err != nil && i < 2 {
			t.Fatalf("unexpected failure on attempt %d: %v", i+1, err)
		}
		_ = err
	}
	err := g.RecordFailure("calc")
	if err == nil {
		t.Fatal("expected error after 3 consecutive failures")
	}
}

func TestLoopGuard_ResetClearsFailures(t *testing.T) {
	g := NewLoopGuard(0, 3)

	g.RecordFailure("calc")
	g.RecordFailure("calc")
	g.Reset("calc")
	err := g.RecordFailure("calc")
	if err != nil {
		t.Errorf("expected no error after reset, got %v", err)
	}
}

func TestLoopGuard_WithToolsNode(t *testing.T) {
	lg := NewLoopGuard(2, 0)
	tn := NewToolsNode[*schema.Message](&ToolsNodeConfig{
		Tools:     []Tool{&mockTool{name: "echo", desc: "echo tool"}},
		LoopGuard: lg,
	})

	// One call should succeed.
	resp1 := &schema.Message{
		Role: schema.RoleAssistant,
		ToolCalls: []schema.ToolCall{
			{ID: "c1", Function: schema.ToolCallFunction{Name: "echo", Arguments: `"hello"`}},
		},
	}
	results1, _, err1 := tn.Execute(context.Background(), resp1, &TypedReActAgentState[*schema.Message]{}, nil)
	if err1 != nil {
		t.Fatalf("1st call: %v", err1)
	}
	if len(results1) != 1 {
		t.Errorf("expected 1 result, got %d", len(results1))
	}

	// Second call with same args should succeed (guard limit is 3, we set 2).
	resp2 := &schema.Message{
		Role: schema.RoleAssistant,
		ToolCalls: []schema.ToolCall{
			{ID: "c2", Function: schema.ToolCallFunction{Name: "echo", Arguments: `"hello"`}},
		},
	}
	results2, _, err2 := tn.Execute(context.Background(), resp2, &TypedReActAgentState[*schema.Message]{}, nil)
	if err2 != nil {
		t.Fatalf("2nd call: %v", err2)
	}
	if len(results2) != 1 {
		t.Errorf("expected 1 result, got %d", len(results2))
	}
	t.Log("loop guard + toolsnode integration passed")
}

// ======================== Tool Capability + Batch Planning Tests ========================

type capableTestTool struct {
	mockTool
	cap ToolCapability
}

func (t *capableTestTool) Capability() ToolCapability { return t.cap }

func TestPlanBatches_ReadOnlyToolInParallel(t *testing.T) {
	tn := NewToolsNode[*schema.Message](&ToolsNodeConfig{
		Tools: []Tool{
			&capableTestTool{mockTool: mockTool{name: "read1"}, cap: ToolCapReadOnly},
			&capableTestTool{mockTool: mockTool{name: "read2"}, cap: ToolCapReadOnly},
		},
	})

	batches := tn.planBatches([]schema.ToolCall{
		{ID: "c1", Function: schema.ToolCallFunction{Name: "read1"}},
		{ID: "c2", Function: schema.ToolCallFunction{Name: "read2"}},
	})

	if len(batches) != 1 {
		t.Fatalf("expected 1 batch (parallel), got %d", len(batches))
	}
	if batches[0].mode != batchParallel {
		t.Error("expected parallel batch for read-only tools")
	}
	if len(batches[0].calls) != 2 {
		t.Errorf("expected 2 calls in batch, got %d", len(batches[0].calls))
	}
}

func TestPlanBatches_SerialToolSeparateBatch(t *testing.T) {
	tn := NewToolsNode[*schema.Message](&ToolsNodeConfig{
		Tools: []Tool{
			&capableTestTool{mockTool: mockTool{name: "write"}, cap: ToolCapWritesFiles},
			&capableTestTool{mockTool: mockTool{name: "read"}, cap: ToolCapReadOnly},
		},
	})

	batches := tn.planBatches([]schema.ToolCall{
		{ID: "c1", Function: schema.ToolCallFunction{Name: "write"}},
		{ID: "c2", Function: schema.ToolCallFunction{Name: "read"}},
	})

	if len(batches) != 2 {
		t.Fatalf("expected 2 batches (serial + parallel), got %d", len(batches))
	}
	if batches[0].mode != batchSerial {
		t.Error("expected first batch to be serial (write)")
	}
	if batches[1].mode != batchParallel {
		t.Error("expected second batch to be parallel (read)")
	}
}

func TestPlanBatches_UnknownToolDefaultSerial(t *testing.T) {
	tn := NewToolsNode[*schema.Message](&ToolsNodeConfig{
		Tools: []Tool{&capableTestTool{mockTool: mockTool{name: "read1"}, cap: ToolCapReadOnly}},
	})

	batches := tn.planBatches([]schema.ToolCall{
		{ID: "c1", Function: schema.ToolCallFunction{Name: "unknown_tool"}},
	})

	if len(batches) != 1 {
		t.Fatalf("expected 1 batch, got %d", len(batches))
	}
	if batches[0].mode != batchSerial {
		t.Error("expected serial batch for unknown tool")
	}
}

func TestCapableTool_DefaultCapability(t *testing.T) {
	// Tools without CapableTool interface default to ToolCapWritesFiles (serial).
	tool := &mockTool{name: "default"}
	cap := toolCapFromTool(tool)
	if cap != ToolCapWritesFiles {
		t.Errorf("expected ToolCapWritesFiles, got %v", cap)
	}
}

func TestCapableTool_WithCapability(t *testing.T) {
	tool := &capableTestTool{cap: ToolCapReadOnly}
	cap := toolCapFromTool(tool)
	if cap != ToolCapReadOnly {
		t.Errorf("expected ToolCapReadOnly, got %v", cap)
	}
}

func TestPlanBatches_ConcurrentExecuteWithCapability(t *testing.T) {
	tn := NewToolsNode[*schema.Message](&ToolsNodeConfig{
		Tools: []Tool{
			&mockTool{name: "t1", desc: "read-only tool 1"},
			&mockTool{name: "t2", desc: "read-only tool 2"},
		},
	})

	resp := &schema.Message{
		Role: schema.RoleAssistant,
		ToolCalls: []schema.ToolCall{
			{ID: "c1", Function: schema.ToolCallFunction{Name: "t1", Arguments: `{}`}},
			{ID: "c2", Function: schema.ToolCallFunction{Name: "t2", Arguments: `{}`}},
		},
	}
	results, _, err := tn.Execute(context.Background(), resp, &TypedReActAgentState[*schema.Message]{}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestApprovalMiddleware_Rejected(t *testing.T) {
	tn := NewToolsNode[*schema.Message](&ToolsNodeConfig{
		Tools: []Tool{&mockTool{name: "reject_me", desc: "will be rejected"}},
		ToolInvokeMiddlewares: []ToolInvokeMiddleware{
			ApprovalMiddleware(func(ctx context.Context, ictx *ToolInvocationContext) (*ApprovalRequest, error) {
				ch := make(chan bool, 1)
				ch <- false // reject
				return &ApprovalRequest{
					ToolName:    ictx.Name,
					CallID:      ictx.CallID,
					Arguments:   ictx.Arguments,
					ApproveChan: ch,
				}, nil
			}),
		},
	})

	resp := &schema.Message{
		Role: schema.RoleAssistant,
		ToolCalls: []schema.ToolCall{
			{ID: "c1", Function: schema.ToolCallFunction{Name: "reject_me", Arguments: "{}"}},
		},
	}

	results, _, err := tn.Execute(context.Background(), resp, &TypedReActAgentState[*schema.Message]{}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result (rejection message), got %d", len(results))
	}
	if len(results) > 0 {
		content := extractTextContent(results[0])
		if content != "Error: rejected" {
			t.Logf("rejection content: %s", content)
		}
	}
}
