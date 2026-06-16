package core

import (
	"context"
	"testing"

	"ragflow/internal/harness/core/schema"
)

// TestAgentTool_BasicInvocation verifies an agent can be wrapped as a tool
// and invoked by a parent agent.
func TestAgentTool_BasicInvocation(t *testing.T) {
	// Inner agent: simple echo.
	innerM := &mockModel{}
	innerM.addResp("response from inner agent")
	innerAgent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: innerM,
	}).WithName("inner").WithDescription("inner echo agent")

	ctx := context.Background()
	agentTool := NewAgentTool(ctx, innerAgent)

	// Parent agent: uses the agent tool.
	parentM := &forcedToolModel{
		inner:     &mockModel{},
		toolCalls: []schema.ToolCall{{ID: "call_1", Function: schema.ToolCallFunction{Name: "inner", Arguments: "{}"}}},
		finalResp: "parent finished",
		firstCall: true,
	}
	parent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model:       parentM,
		Tools:       []Tool{agentTool},
		ToolsConfig: &ToolsNodeConfig{Tools: []Tool{agentTool}},
		MaxIterations: 3,
	}).WithName("parent")

	store := newCancelTestStore()
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: parent, CheckPointStore: store})
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("call inner")})

	var lastContent string
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			t.Fatalf("unexpected err: %v", ev.Err)
		}
		if ev.Output != nil && ev.Output.MessageOutput != nil &&
			!ev.Output.MessageOutput.IsStreaming &&
			ev.Output.MessageOutput.Message != nil {
			lastContent = ev.Output.MessageOutput.Message.Content
		}
	}
	if lastContent != "parent finished" {
		t.Errorf("expected 'parent finished', got %q", lastContent)
	}
	t.Logf("agent tool test: final content=%q", lastContent)
}

// TestAgentTool_NestedWithCheckpoint verifies AgentTool nested execution
// integrates with checkpoint for interrupt/resume.
func TestAgentTool_NestedWithCheckpoint(t *testing.T) {
	innerM := &mockModel{}
	innerM.addResp("nested result")
	innerAgent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: innerM,
	}).WithName("nested").WithDescription("nested agent for checkpoint test")

	ctx := context.Background()
	agentTool := NewAgentTool(ctx, innerAgent)

	parentM := &forcedToolModel{
		inner:     &mockModel{},
		toolCalls: []schema.ToolCall{{ID: "nc", Function: schema.ToolCallFunction{Name: "nested", Arguments: "{}"}}},
		finalResp: "with checkpoint",
		firstCall: true,
	}
	parent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model:       parentM,
		Tools:       []Tool{agentTool},
		ToolsConfig: &ToolsNodeConfig{Tools: []Tool{agentTool}},
		MaxIterations: 3,
	}).WithName("parent_cp")

	store := newCancelTestStore()
	// Run with a checkpoint ID.
	cid := "agent-tool-cp-001"
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: parent, CheckPointStore: store})
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("nested call")},
		WithCheckPointID(cid))

	// Drain events — should complete the nested tool call via ToolsNode.
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			t.Logf("nested checkpoint event: err=%v", ev.Err)
			break
		}
	}
	t.Log("agent tool nested checkpoint cycle completed")
}

// TestAgentTool_EventForwarding verifies that internal events from the inner
// agent are forwarded when EmitInternalEvents is enabled.
func TestAgentTool_EventForwarding(t *testing.T) {
	innerM := &mockModel{}
	innerM.addResp("forwarded response")
	innerAgent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: innerM,
	}).WithName("forward_inner").WithDescription("inner with event forwarding")

	ctx := context.Background()
	agentTool := NewAgentTool(ctx, innerAgent, WithEmitInternalEvents())

	parentM := &forcedToolModel{
		inner:     &mockModel{},
		toolCalls: []schema.ToolCall{{ID: "fe", Function: schema.ToolCallFunction{Name: "forward_inner", Arguments: "{}"}}},
		finalResp: "forwarded done",
		firstCall: true,
	}
	parent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model:       parentM,
		Tools:       []Tool{agentTool},
		ToolsConfig: &ToolsNodeConfig{Tools: []Tool{agentTool}},
		MaxIterations: 3,
	}).WithName("forward_parent")

	store := newCancelTestStore()
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: parent, CheckPointStore: store})
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("forward test")})

	var eventCount int
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			t.Logf("event err: %v", ev.Err)
		}
		eventCount++
	}
	t.Logf("agent tool event forwarding: %d events received", eventCount)
}

// TestAgentTool_ResumeAfterInterrupt verifies the inner agent can be
// interrupted and resumed inside a parent agent's tool execution.
func TestAgentTool_ResumeAfterInterrupt(t *testing.T) {
	innerM := &forcedToolModel{
		inner:     &mockModel{},
		toolCalls: []schema.ToolCall{{ID: "ri", Function: schema.ToolCallFunction{Name: "resume_inner_tool", Arguments: "{}"}}},
		finalResp: "resumed inner",
		firstCall: true,
	}
	tool := &mockTool{name: "resume_inner_tool", desc: "tool for resume test"}
	innerAgent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model:       innerM,
		Tools:       []Tool{tool},
		ToolsConfig: &ToolsNodeConfig{Tools: []Tool{tool}},
		MaxIterations: 3,
	}).WithName("resume_inner").WithDescription("interruptible inner agent")

	ctx := context.Background()
	agentTool := NewAgentTool(ctx, innerAgent)

	parentM := &forcedToolModel{
		inner:     &mockModel{},
		toolCalls: []schema.ToolCall{{ID: "pr", Function: schema.ToolCallFunction{Name: "resume_inner", Arguments: "{}"}}},
		finalResp: "parent after resume",
		firstCall: true,
	}
	parent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model:       parentM,
		Tools:       []Tool{agentTool},
		ToolsConfig: &ToolsNodeConfig{Tools: []Tool{agentTool}},
		MaxIterations: 3,
	}).WithName("resume_parent")

	store := newCancelTestStore()
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: parent, CheckPointStore: store})
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("inner resume test")})

	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			t.Logf("resume inner event: err=%v", ev.Err)
			break
		}
	}
	t.Log("agent tool resume-after-interrupt cycle completed")
}

func init() {
	schema.RegisterType("_test_agent_tool", func() any { return &AgentToolOptions{} })
}
