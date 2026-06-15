package core

import (
	"context"
	"testing"
	"time"

	"ragflow/internal/harness/core/schema"
)

// ======================== Tests: Sequential Workflow ========================

func TestWorkflow_SequentialAgent(t *testing.T) {
	m1 := &mockModel{}; m1.addResp("A1")
	m2 := &mockModel{}; m2.addResp("A2")
	a1 := reActAgentSetup(m1, nil); a1.name = "seq_a1"
	a2 := reActAgentSetup(m2, nil); a2.name = "seq_a2"

	ctx := context.Background()
	wf, err := NewSequential(ctx, &SequentialConfig{
		Name: "seq", Description: "test", SubAgents: []Agent{a1, a2},
	})
	if err != nil {
		t.Fatalf("NewSequential: %v", err)
	}

	iter := wf.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("run")}})
	events := drainAgentEvents(t, iter)
	if len(events) == 0 {
		t.Error("expected events from sequential workflow")
	}
}

func TestWorkflow_ParallelAgent(t *testing.T) {
	m1 := &mockModel{}; m1.addResp("P1")
	m2 := &mockModel{}; m2.addResp("P2")
	a1 := reActAgentSetup(m1, nil); a1.name = "par_a1"
	a2 := reActAgentSetup(m2, nil); a2.name = "par_a2"

	ctx := context.Background()
	wf, err := NewParallel(ctx, &ParallelConfig{
		Name: "par", Description: "test", SubAgents: []Agent{a1, a2},
	})
	if err != nil {
		t.Fatalf("NewParallel: %v", err)
	}

	iter := wf.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("run")}})
	events := drainAgentEvents(t, iter)
	if len(events) == 0 {
		t.Error("expected events from parallel workflow")
	}
}

func TestWorkflow_NestedParallel(t *testing.T) {
	m1 := &mockModel{}; m1.addResp("inner1")
	m2 := &mockModel{}; m2.addResp("inner2")
	m3 := &mockModel{}; m3.addResp("outer")

	a1 := reActAgentSetup(m1, nil); a1.name = "inner_a"
	a2 := reActAgentSetup(m2, nil); a2.name = "inner_b"

	innerPar, err := NewParallel(context.Background(), &ParallelConfig{
		Name: "inner-par", Description: "inner parallel", SubAgents: []Agent{a1, a2},
	})
	if err != nil {
		t.Fatalf("NewParallel: %v", err)
	}

	a3 := reActAgentSetup(m3, nil); a3.name = "outer"
	wf, err := NewSequential(context.Background(), &SequentialConfig{
		Name: "nested", Description: "nested parallel", SubAgents: []Agent{innerPar, a3},
	})
	if err != nil {
		t.Fatalf("NewSequential: %v", err)
	}

	iter := wf.Run(context.Background(), &AgentInput{Messages: []Message{schema.UserMessage("nested")}})
	events := drainAgentEvents(t, iter)
	if len(events) == 0 {
		t.Error("expected events from nested workflow")
	}
	t.Logf("nested workflow: %d events", len(events))
}

func TestWorkflow_LoopAgent(t *testing.T) {
	m := &mockModel{}; m.addResp("loop body")
	body := reActAgentSetup(m, nil); body.name = "loop_body"

	ctx := context.Background()
	wf, err := NewLoop(ctx, &LoopConfig{
		Name: "loop", Description: "test", SubAgents: []Agent{body}, MaxIterations: 3,
	})
	if err != nil {
		t.Fatalf("NewLoop: %v", err)
	}

	iter := wf.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("iterate")}})
	events := drainAgentEvents(t, iter)
	if len(events) == 0 {
		t.Error("expected events from loop workflow")
	}
}

func TestWorkflow_UnsupportedMode(t *testing.T) {
	wf := &workflowAgent{name: "bad", mode: workflowModeUnknown}
	iter := wf.Run(context.Background(), &AgentInput{})
	ev, ok := iter.Next()
	if !ok {
		t.Fatal("expected an event")
	}
	if ev.Err == nil {
		t.Error("expected error for unsupported mode")
	} else 	if ev.Err.Error() != "unsupported mode 0" {
		t.Errorf("expected 'unsupported mode 0', got %v", ev.Err)
	}
}

// ======================== Tests: Agentic Integration ========================

func TestAgenticIntegration_BasicGenerate(t *testing.T) {
	model := &mockModel{}; model.addResp("Hello!")
	agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: model}).WithName("e2e")

	iter := agent.Run(context.Background(), &AgentInput{Messages: []Message{schema.UserMessage("Hi")}})
	events := drainAgentEvents(t, iter)
	if len(events) == 0 {
		t.Fatal("expected events")
	}
}

func TestAgenticIntegration_ToolInvocation(t *testing.T) {
	model := &mockModel{}
	model.addResp("I'll use a tool")
	model.addResp("Here are results")
	tool := &mockTool{name: "search", desc: "search tool"}

	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: model, Tools: []Tool{tool},
	}).WithName("tool_e2e")

	iter := agent.Run(context.Background(), &AgentInput{Messages: []Message{schema.UserMessage("search something")}})
	events := drainAgentEvents(t, iter)
	if len(events) == 0 {
		t.Error("expected events")
	}
	t.Logf("tool integration: %d events", len(events))
}

func TestAgenticIntegration_StreamingOutput(t *testing.T) {
	model := &mockModel{}
	model.addResp("streaming response")

	agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: model}).WithName("stream_e2e")

	ctx := context.Background()
	iter := agent.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("stream test")}})
	events := drainAgentEvents(t, iter)
	if len(events) == 0 {
		t.Error("expected events")
	}
}

func TestAgenticIntegration_EmptyInput(t *testing.T) {
	model := &mockModel{}; model.addResp("response")
	agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: model}).WithName("empty")

	ctx := context.Background()
	iter := agent.Run(ctx, &AgentInput{})
	events := drainAgentEvents(t, iter)
	if len(events) == 0 {
		t.Error("expected events even with empty input")
	}
}

// ======================== Tool Calling Integration Tests ========================

func TestAgenticIntegration_ToolInvokeMiddlewareChain(t *testing.T) {
	model := &mockModel{}
	model.addResp("I'll call a tool")
	model.addResp("Done")
	tool := &mockTool{name: "search", desc: "search"}

	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: model,
		Tools: []Tool{tool},
		ToolsConfig: &ToolsNodeConfig{
			Tools: []Tool{tool},
			ToolInvokeMiddlewares: []ToolInvokeMiddleware{
				NewTimeoutToolMiddleware(5 * time.Second),
			},
		},
	}).WithName("mw_chain_e2e")

	iter := agent.Run(context.Background(), &AgentInput{Messages: []Message{schema.UserMessage("search")}})
	events := drainAgentEvents(t, iter)
	if len(events) == 0 {
		t.Error("expected events")
	}
	t.Logf("mw chain integration: %d events", len(events))
}

func TestAgenticIntegration_ReflectToolAgent(t *testing.T) {
	weatherTool, err := ReflectTool("get_weather", "Get weather",
		func(ctx context.Context, args *weatherArgs) (string, error) {
			return "Weather in " + args.City + ": 22°C", nil
		})
	if err != nil {
		t.Fatalf("ReflectTool: %v", err)
	}

	model := &mockModel{}
	model.addResp("Let me check weather")
	model.addResp("Done")

	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: model,
		Tools: []Tool{weatherTool},
	}).WithName("reflect_e2e")

	iter := agent.Run(context.Background(), &AgentInput{Messages: []Message{schema.UserMessage("weather in Tokyo")}})
	events := drainAgentEvents(t, iter)
	if len(events) == 0 {
		t.Error("expected events")
	}
	t.Logf("reflect tool e2e: %d events", len(events))
}

func TestAgenticIntegration_ToolRegistryAgent(t *testing.T) {
	r := NewToolRegistry()
	searchTool := MustReflectTool("web_search", "Search web",
		func(ctx context.Context, args *weatherArgs) (string, error) {
			return "Search results for " + args.City, nil
		})
	r.Register(searchTool, WithAlias("search"), WithCategory("web"))

	model := &mockModel{}
	model.addResp("I'll search")
	model.addResp("Results ready")

	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: model,
		Tools: r.ToSlice(),
	}).WithName("registry_e2e")

	iter := agent.Run(context.Background(), &AgentInput{Messages: []Message{schema.UserMessage("search for London")}})
	events := drainAgentEvents(t, iter)
	if len(events) == 0 {
		t.Error("expected events")
	}
	t.Logf("registry e2e: %d events", len(events))
}

func TestAgenticIntegration_RetryToolMiddleware(t *testing.T) {
	model := &mockModel{}
	model.addResp("Calling tool")
	model.addResp("Finally done")
	tool := &mockTool{name: "flakey_tool", desc: "might fail"}

	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: model,
		Tools: []Tool{tool},
		ToolsConfig: &ToolsNodeConfig{
			Tools: []Tool{tool},
			ToolInvokeMiddlewares: []ToolInvokeMiddleware{
				NewRetryToolMiddleware(&ToolRetryConfig{
					MaxAttempts: 2, Backoff: time.Millisecond,
					IsRetryable: func(err error) bool { return true },
				}),
			},
		},
	}).WithName("retry_tool_e2e")

	iter := agent.Run(context.Background(), &AgentInput{Messages: []Message{schema.UserMessage("run")}})
	events := drainAgentEvents(t, iter)
	if len(events) == 0 {
		t.Error("expected events")
	}
	t.Logf("retry tool e2e: %d events", len(events))
}

func TestAgenticIntegration_ToolFallback(t *testing.T) {
	model := &mockModel{}
	model.addResp("Using primary tool")
	model.addResp("Fallback complete")
	primary := &mockTool{name: "primary", desc: "primary"}

	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: model,
		Tools: []Tool{primary},
		ToolsConfig: &ToolsNodeConfig{
			Tools: []Tool{primary},
			ToolInvokeMiddlewares: []ToolInvokeMiddleware{
				NewFallbackToolMiddleware(func(ctx context.Context, args *schema.ToolArgument) (*schema.ToolResult, error) {
					return &schema.ToolResult{Content: "fallback result", ToolCallID: args.CallID}, nil
				}),
			},
		},
	}).WithName("fallback_e2e")

	iter := agent.Run(context.Background(), &AgentInput{Messages: []Message{schema.UserMessage("run")}})
	events := drainAgentEvents(t, iter)
	if len(events) == 0 {
		t.Error("expected events")
	}
	t.Logf("fallback e2e: %d events", len(events))
}

func TestAgenticIntegration_ModelErrorRecovery(t *testing.T) {
	model := &countingModelForRetry{failTimes: 2}
	cfg := &ModelRetryConfig{MaxRetries: 5, IsRetryAble: func(_ context.Context, err error) bool { return true }}
	wrapped := WithModelRetry(model, cfg)

	agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: wrapped}).WithName("retry_e2e")

	iter := agent.Run(context.Background(), &AgentInput{Messages: []Message{schema.UserMessage("retry test")}})
	_ = drainAgentEvents(t, iter)
}
