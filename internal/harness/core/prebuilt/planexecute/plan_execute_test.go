package planexecute

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"ragflow/internal/harness/core"
	"ragflow/internal/harness/core/schema"
)

// ============================================================
// Mock model
// ============================================================

type mockPlanModel struct {
	responses []mockResponse
	idx       int
}

type mockResponse struct {
	content   string
	toolCalls []schema.ToolCall
}

func (m *mockPlanModel) Generate(ctx context.Context, msgs []*schema.Message, opts ...core.ModelOption) (*schema.Message, error) {
	if m.idx >= len(m.responses) {
		return &schema.Message{Role: schema.RoleAssistant, Content: "done"}, nil
	}
	r := m.responses[m.idx]
	m.idx++
	return &schema.Message{
		Role:      schema.RoleAssistant,
		Content:   r.content,
		ToolCalls: r.toolCalls,
	}, nil
}

func (m *mockPlanModel) Stream(ctx context.Context, msgs []*schema.Message, opts ...core.ModelOption) (*schema.StreamReader[*schema.Message], error) {
	return schema.StreamReaderFromArray([]*schema.Message{{Role: schema.RoleAssistant, Content: "mock stream"}}), nil
}

func (m *mockPlanModel) BindTools(tools []*schema.ToolInfo) error { return nil }

// ============================================================
// Test Plan interface
// ============================================================

func TestDefaultPlan_MarshalUnmarshal(t *testing.T) {
	p := &defaultPlan{StepList: []string{"Step 1", "Step 2", "Step 3"}}
	data, err := p.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	var p2 defaultPlan
	if err := p2.UnmarshalJSON(data); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if len(p2.StepList) != 3 {
		t.Errorf("expected 3 steps, got %d", len(p2.StepList))
	}
	if p2.StepList[0] != "Step 1" {
		t.Errorf("expected 'Step 1', got %q", p2.StepList[0])
	}
}

func TestDefaultPlan_Steps(t *testing.T) {
	p := &defaultPlan{StepList: []string{"A", "B"}}
	steps := p.Steps()
	if len(steps) != 2 || steps[0] != "A" || steps[1] != "B" {
		t.Errorf("unexpected steps: %v", steps)
	}
}

func TestDefaultPlan_JSONRoundtrip(t *testing.T) {
	p := &defaultPlan{StepList: []string{"Research", "Write", "Review"}}
	data, _ := json.Marshal(p)
	var restored defaultPlan
	json.Unmarshal(data, &restored)
	if len(restored.StepList) != 3 {
		t.Errorf("expected 3 steps after roundtrip, got %d", len(restored.StepList))
	}
}

func TestNewPlan(t *testing.T) {
	p := &defaultPlan{StepList: []string{}}
	if p == nil {
		t.Fatal("nil plan")
	}
	if len(p.Steps()) != 0 {
		t.Errorf("expected empty plan, got %d steps", len(p.Steps()))
	}
}

// ============================================================
// Test Config validation
// ============================================================

func TestNew_NilConfig(t *testing.T) {
	ctx := context.Background()
	_, err := New(ctx, nil)
	if err == nil {
		t.Error("expected error for nil config")
	}
}

func TestNew_MissingPlanner(t *testing.T) {
	ctx := context.Background()
	_, err := New(ctx, &Config{})
	if err == nil {
		t.Error("expected error for missing Planner")
	}
}

func TestNew_MissingExecutor(t *testing.T) {
	ctx := context.Background()
	_, err := New(ctx, &Config{
		Planner: &PlannerConfig{Model: &mockPlanModel{}},
	})
	if err == nil {
		t.Error("expected error for missing Executor")
	}
}

func TestNew_MissingReplanner(t *testing.T) {
	ctx := context.Background()
	_, err := New(ctx, &Config{
		Planner:  &PlannerConfig{Model: &mockPlanModel{}},
		Executor: &ExecutorConfig{Model: &mockPlanModel{}},
	})
	if err == nil {
		t.Error("expected error for missing Replanner")
	}
}

// ============================================================
// Test New with valid config
// ============================================================

func TestNew_DefaultConfig(t *testing.T) {
	ctx := context.Background()
	model := &mockPlanModel{}

	flow, err := New(ctx, &Config{
		Planner:           &PlannerConfig{Model: model},
		Executor:          &ExecutorConfig{Model: model},
		Replanner:         &ReplannerConfig{Model: model},
		Name:              "test_plan_execute",
		MaxLoopIterations: 5,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if flow == nil {
		t.Fatal("nil flow agent")
	}
	if flow.Name(ctx) != "test_plan_execute" {
		t.Errorf("expected name 'test_plan_execute', got %q", flow.Name(ctx))
	}
}

func TestNew_DefaultName(t *testing.T) {
	ctx := context.Background()
	model := &mockPlanModel{}

	flow, err := New(ctx, &Config{
		Planner:   &PlannerConfig{Model: model},
		Executor:  &ExecutorConfig{Model: model},
		Replanner: &ReplannerConfig{Model: model},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if flow.Name(ctx) != "plan_execute_agent" {
		t.Errorf("expected default name, got %q", flow.Name(ctx))
	}
}

func TestNew_DefaultMaxLoopIterations(t *testing.T) {
	ctx := context.Background()
	model := &mockPlanModel{}

	// This should use default MaxLoopIterations (10) — just verify no error
	flow, err := New(ctx, &Config{
		Planner:   &PlannerConfig{Model: model},
		Executor:  &ExecutorConfig{Model: model},
		Replanner: &ReplannerConfig{Model: model},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_ = flow
}

// ============================================================
// Test custom prompts
// ============================================================

func TestNew_CustomPlannerPrompt(t *testing.T) {
	ctx := context.Background()
	model := &mockPlanModel{}

	flow, err := New(ctx, &Config{
		Planner: &PlannerConfig{
			Model:       model,
			Instruction: "Custom planner instruction",
		},
		Executor:  &ExecutorConfig{Model: model},
		Replanner: &ReplannerConfig{Model: model},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_ = flow
}

func TestNew_CustomExecutorPrompt(t *testing.T) {
	ctx := context.Background()
	model := &mockPlanModel{}

	flow, err := New(ctx, &Config{
		Planner: &PlannerConfig{Model: model},
		Executor: &ExecutorConfig{
			Model:       model,
			Instruction: "Custom executor instruction",
		},
		Replanner: &ReplannerConfig{Model: model},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_ = flow
}

func TestNew_CustomReplannerPrompt(t *testing.T) {
	ctx := context.Background()
	model := &mockPlanModel{}

	flow, err := New(ctx, &Config{
		Planner:  &PlannerConfig{Model: model},
		Executor: &ExecutorConfig{Model: model},
		Replanner: &ReplannerConfig{
			Model:       model,
			Instruction: "Custom replanner instruction",
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_ = flow
}

// ============================================================
// Test custom NewPlan factory
// ============================================================



// ============================================================
// Test Executor with tools
// ============================================================

func TestNew_ExecutorWithTools(t *testing.T) {
	ctx := context.Background()
	model := &mockPlanModel{}

	tool := core.NewBaseTool(
		"test_tool",
		"A test tool",
		func(ctx context.Context, args string) (string, error) {
			return "tool result", nil
		},
	)

	flow, err := New(ctx, &Config{
		Planner:  &PlannerConfig{Model: model},
		Executor: &ExecutorConfig{Model: model, Tools: []core.Tool{tool}},
		Replanner: &ReplannerConfig{Model: model},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_ = flow
}

// ============================================================
// Integration test — full Planner→Executor→Replanner pipeline
// ============================================================

func TestPlanExecute_Integration(t *testing.T) {
	ctx := context.Background()

	// Planner: plan_tool → creates a plan
	plannerModel := &mockPlanModel{responses: []mockResponse{
		{toolCalls: []schema.ToolCall{{
			ID: "pl_1", Type: "function",
			Function: schema.ToolCallFunction{Name: toolPlan, Arguments: `{"steps":["Step 1"]}`},
		}}},
	}}

	// Executor: returns text (no tool call)
	executorModel := &mockPlanModel{responses: []mockResponse{
		{content: "Step 1 executed"},
	}}

	// Replanner: respond_tool → signals completion
	replannerModel := &mockPlanModel{responses: []mockResponse{
		{toolCalls: []schema.ToolCall{{
			ID: "rp_1", Type: "function",
			Function: schema.ToolCallFunction{Name: toolRespond, Arguments: `{"response":"Task complete"}`},
		}}},
	}}

	agent, err := New(ctx, &Config{
		Planner:           &PlannerConfig{Model: plannerModel},
		Executor:          &ExecutorConfig{Model: executorModel},
		Replanner:         &ReplannerConfig{Model: replannerModel},
		MaxLoopIterations: 5,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	runner := core.NewTypedRunner(core.RunnerConfig[*schema.Message]{Agent: agent})
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("do something")})

	var lastContent string
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			t.Fatalf("unexpected error: %v", ev.Err)
		}
		if ev.Output != nil && ev.Output.MessageOutput != nil && !ev.Output.MessageOutput.IsStreaming {
			if msg := ev.Output.MessageOutput.Message; msg != nil {
				lastContent = msg.Content
			}
		}
	}
	if lastContent == "" {
		t.Error("expected some output content")
	}
	t.Logf("integration test: final content=%q", lastContent)
}
// ============================================================

func TestPlanJSON_Marshal(t *testing.T) {
	p := &defaultPlan{StepList: []string{"A", "B"}}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	steps, ok := result["steps"].([]interface{})
	if !ok {
		t.Fatal("expected steps array")
	}
	if len(steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(steps))
	}
}

func TestPlanJSON_UnmarshalInvalid(t *testing.T) {
	p := &defaultPlan{}
	err := json.Unmarshal([]byte(`{"steps": "not_an_array"}`), p)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// ============================================================
// Test helper functions
// ============================================================

func TestGetObjective(t *testing.T) {
	msgs := []*schema.Message{
		schema.SystemMessage("be helpful"),
		schema.UserMessage("write a poem"),
	}
	obj := getObjective(msgs)
	if obj != "write a poem" {
		t.Errorf("expected 'write a poem', got %q", obj)
	}
}

func TestGetObjective_NoUser(t *testing.T) {
	msgs := []*schema.Message{
		schema.SystemMessage("be helpful"),
		schema.AssistantMessage("hello"),
	}
	obj := getObjective(msgs)
	if obj != "" {
		t.Errorf("expected empty, got %q", obj)
	}
}

// ============================================================
// Test plan_execute tool definitions
// ============================================================

func TestPlanToolDef(t *testing.T) {
	if planToolDef.Name() != toolPlan {
		t.Errorf("expected name %q, got %q", toolPlan, planToolDef.Name())
	}
	if planToolDef.Description() == "" {
		t.Error("empty description")
	}
}

func TestRespondToolDef(t *testing.T) {
	if respondToolDef.Name() != toolRespond {
		t.Errorf("expected name %q, got %q", toolRespond, respondToolDef.Name())
	}
	if respondToolDef.Description() == "" {
		t.Error("empty description")
	}
}

// ============================================================
// Test constant values
// ============================================================

func TestConstants(t *testing.T) {
	if agentNamePlanner != "planner" {
		t.Errorf("agentNamePlanner = %q", agentNamePlanner)
	}
	if agentNameExecutor != "executor" {
		t.Errorf("agentNameExecutor = %q", agentNameExecutor)
	}
	if agentNameReplanner != "replanner" {
		t.Errorf("agentNameReplanner = %q", agentNameReplanner)
	}
	if agentNameLoop != "planexecute_loop" {
		t.Errorf("agentNameLoop = %q", agentNameLoop)
	}
	if sessionKeyPlan != "__planexecute_plan" {
		t.Errorf("sessionKeyPlan = %q", sessionKeyPlan)
	}
	if sessionKeyStepsDone != "__planexecute_steps_done" {
		t.Errorf("sessionKeyStepsDone = %q", sessionKeyStepsDone)
	}
}

// ============================================================
// Test prompt constants
// ============================================================

func TestPlannerPrompt(t *testing.T) {
	if PlannerPrompt == "" {
		t.Error("PlannerPrompt is empty")
	}
	if !strings.Contains(PlannerPrompt, "plan_tool") {
		t.Error("PlannerPrompt should mention plan_tool")
	}
}

func TestExecutorPrompt(t *testing.T) {
	if ExecutorPrompt == "" {
		t.Error("ExecutorPrompt is empty")
	}
	if !strings.Contains(ExecutorPrompt, "{objective}") {
		t.Error("ExecutorPrompt should contain {objective}")
	}
	if !strings.Contains(ExecutorPrompt, "{plan}") {
		t.Error("ExecutorPrompt should contain {plan}")
	}
	if !strings.Contains(ExecutorPrompt, "{completed_steps}") {
		t.Error("ExecutorPrompt should contain {completed_steps}")
	}
}

func TestReplannerPrompt(t *testing.T) {
	if ReplannerPrompt == "" {
		t.Error("ReplannerPrompt is empty")
	}
	if !strings.Contains(ReplannerPrompt, "plan_tool") {
		t.Error("ReplannerPrompt should mention plan_tool")
	}
	if !strings.Contains(ReplannerPrompt, "respond_tool") {
		t.Error("ReplannerPrompt should mention respond_tool")
	}
}


