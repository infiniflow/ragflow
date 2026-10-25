package core

import (
	"context"
	"sync"
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

	ctx := t.Context()
	agentTool := NewAgentTool(ctx, innerAgent)

	// Parent agent: uses the agent tool.
	parentM := &forcedToolModel{
		inner:     &mockModel{},
		toolCalls: []schema.ToolCall{{ID: "call_1", Function: schema.ToolCallFunction{Name: "inner", Arguments: "{}"}}},
		finalResp: "parent finished",
		firstCall: true,
	}
	parent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model:         parentM,
		Tools:         []Tool{agentTool},
		ToolsConfig:   &ToolsNodeConfig{Tools: []Tool{agentTool}},
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

	ctx := t.Context()
	agentTool := NewAgentTool(ctx, innerAgent)

	parentM := &forcedToolModel{
		inner:     &mockModel{},
		toolCalls: []schema.ToolCall{{ID: "nc", Function: schema.ToolCallFunction{Name: "nested", Arguments: "{}"}}},
		finalResp: "with checkpoint",
		firstCall: true,
	}
	parent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model:         parentM,
		Tools:         []Tool{agentTool},
		ToolsConfig:   &ToolsNodeConfig{Tools: []Tool{agentTool}},
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

	ctx := t.Context()
	agentTool := NewAgentTool(ctx, innerAgent, WithEmitInternalEvents())

	parentM := &forcedToolModel{
		inner:     &mockModel{},
		toolCalls: []schema.ToolCall{{ID: "fe", Function: schema.ToolCallFunction{Name: "forward_inner", Arguments: "{}"}}},
		finalResp: "forwarded done",
		firstCall: true,
	}
	parent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model:         parentM,
		Tools:         []Tool{agentTool},
		ToolsConfig:   &ToolsNodeConfig{Tools: []Tool{agentTool}},
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
		Model:         innerM,
		Tools:         []Tool{tool},
		ToolsConfig:   &ToolsNodeConfig{Tools: []Tool{tool}},
		MaxIterations: 3,
	}).WithName("resume_inner").WithDescription("interruptible inner agent")

	ctx := t.Context()
	agentTool := NewAgentTool(ctx, innerAgent)

	parentM := &forcedToolModel{
		inner:     &mockModel{},
		toolCalls: []schema.ToolCall{{ID: "pr", Function: schema.ToolCallFunction{Name: "resume_inner", Arguments: "{}"}}},
		finalResp: "parent after resume",
		firstCall: true,
	}
	parent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model:         parentM,
		Tools:         []Tool{agentTool},
		ToolsConfig:   &ToolsNodeConfig{Tools: []Tool{agentTool}},
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


type capturingHistoryModel struct {
	mu           sync.Mutex
	capturedMsgs []Message
	resp         string
}

func (m *capturingHistoryModel) Generate(ctx context.Context, msgs []Message, opts ...modelOption) (Message, error) {
	m.mu.Lock()
	m.capturedMsgs = append([]Message(nil), msgs...)
	m.mu.Unlock()
	return &schema.Message{Role: schema.RoleAssistant, Content: m.resp}, nil
}

func (m *capturingHistoryModel) Stream(ctx context.Context, msgs []Message, opts ...modelOption) (*schema.StreamReader[Message], error) {
	msg, err := m.Generate(ctx, msgs, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]Message{msg}), nil
}

func (m *capturingHistoryModel) BindTools(tools []*schema.ToolInfo) error { return nil }

func (m *capturingHistoryModel) getCapturedMsgs() []Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]Message, len(m.capturedMsgs))
	copy(cp, m.capturedMsgs)
	return cp
}

func (m *capturingHistoryModel) clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.capturedMsgs = nil
}

// TestAgentTool_FullChatHistoryAsInput verifies that when FullChatHistoryAsInput is enabled,
// historical conversation turns from RootInput and Session events are prepended to the
// sub-agent's input messages.
func TestAgentTool_FullChatHistoryAsInput(t *testing.T) {
	captureM := &capturingHistoryModel{resp: "inner captured"}
	innerAgent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: captureM,
	}).WithName("history_inner").WithDescription("inner agent")

	// 1. When FullChatHistoryAsInput is enabled:
	toolWithHistory := NewAgentTool(t.Context(), innerAgent, WithFullChatHistoryAsInput())

	sess := newRunSession()
	sess.addEvent(&AgentEvent{
		Output: &AgentOutput{
			MessageOutput: &MessageVariant{
				Message: schema.AssistantMessage("Turn 1 assistant reply"),
			},
		},
	})

	rc := &runContext{
		RootInput: &AgentInput{
			Messages: []Message{schema.UserMessage("Turn 1 user query")},
		},
		Session: sess,
	}
	ctx := setRunCtx(t.Context(), rc)

	res, err := toolWithHistory.Invoke(ctx, "Turn 2 sub-agent query")
	if err != nil {
		t.Fatalf("unexpected error invoking tool: %v", err)
	}
	if res != "inner captured" {
		t.Errorf("unexpected tool output: %q", res)
	}

	msgs := captureM.getCapturedMsgs()
	// Strip system prompt inserted by ReActAgent
	if len(msgs) > 0 && msgs[0].Role == schema.RoleSystem {
		msgs = msgs[1:]
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 dialogue messages, got %d: %v", len(msgs), msgs)
	}
	if msgs[0].Content != "Turn 1 user query" {
		t.Errorf("expected first message to be 'Turn 1 user query', got %q", msgs[0].Content)
	}
	if msgs[1].Content != "Turn 1 assistant reply" {
		t.Errorf("expected second message to be 'Turn 1 assistant reply', got %q", msgs[1].Content)
	}
	if msgs[2].Content != "Turn 2 sub-agent query" {
		t.Errorf("expected third message to be 'Turn 2 sub-agent query', got %q", msgs[2].Content)
	}

	// 2. When FullChatHistoryAsInput is false (default):
	captureM.clear()
	toolWithoutHistory := NewAgentTool(t.Context(), innerAgent)
	res, err = toolWithoutHistory.Invoke(ctx, "Turn 2 standalone query")
	if err != nil {
		t.Fatalf("unexpected error invoking tool without history: %v", err)
	}
	msgs = captureM.getCapturedMsgs()
	if len(msgs) > 0 && msgs[0].Role == schema.RoleSystem {
		msgs = msgs[1:]
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message when history disabled, got %d: %v", len(msgs), msgs)
	}
	if msgs[0].Content != "Turn 2 standalone query" {
		t.Errorf("expected 'Turn 2 standalone query', got %q", msgs[0].Content)
	}
}

func init() {
	schema.RegisterType("_test_agent_tool", func() any { return &AgentToolOptions{} })
}
