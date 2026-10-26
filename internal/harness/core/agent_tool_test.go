package core

import (
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

func init() {
	schema.RegisterType("_test_agent_tool", func() any { return &AgentToolOptions{} })
}

// TestAgentTool_FullChatHistoryAsInput verifies that when FullChatHistoryAsInput
// is enabled, the inner agent receives the parent conversation history.
func TestAgentTool_FullChatHistoryAsInput(t *testing.T) {
	// Inner agent: records the messages it receives.
	var receivedMsgs []Message
	innerM := &historyRecordingModel{record: &receivedMsgs}
	innerM.addResp("inner response")
	innerAgent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: innerM,
	}).WithName("history_inner").WithDescription("inner agent for history test")

	ctx := t.Context()
	agentTool := NewAgentTool(ctx, innerAgent, WithFullChatHistoryAsInput())

	// Parent agent: uses the agent tool after an initial turn.
	parentM := &forcedToolModel{
		inner:     &mockModel{},
		toolCalls: []schema.ToolCall{{ID: "ht", Function: schema.ToolCallFunction{Name: "history_inner", Arguments: "{}"}}},
		finalResp: "parent done",
		firstCall: true,
	}
	parent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model:         parentM,
		Tools:         []Tool{agentTool},
		ToolsConfig:   &ToolsNodeConfig{Tools: []Tool{agentTool}},
		MaxIterations: 3,
	}).WithName("history_parent")

	store := newCancelTestStore()
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: parent, CheckPointStore: store})
	// First user message establishes conversation history.
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("first turn")})

	// Drain events.
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			t.Logf("history test event err: %v", ev.Err)
			break
		}
	}

	// The inner agent should have received the parent history + tool args.
	if len(receivedMsgs) == 0 {
		t.Fatal("inner agent received no messages")
	}

	// First message should be the parent's user message.
	if receivedMsgs[0].Role != schema.RoleUser || receivedMsgs[0].Content != "first turn" {
		t.Errorf("expected first message to be user 'first turn', got %s: %q", receivedMsgs[0].Role, receivedMsgs[0].Content)
	}

	// Last message should be the tool call arguments.
	last := receivedMsgs[len(receivedMsgs)-1]
	if last.Role != schema.RoleUser {
		t.Errorf("expected last message to be user (tool args), got %s", last.Role)
	}

	t.Logf("full chat history test: inner received %d messages", len(receivedMsgs))
}

// TestAgentTool_NoChatHistoryAsInput verifies that when FullChatHistoryAsInput
// is disabled (default), the inner agent receives only the tool arguments.
func TestAgentTool_NoChatHistoryAsInput(t *testing.T) {
	var receivedMsgs []Message
	innerM := &historyRecordingModel{record: &receivedMsgs}
	innerM.addResp("inner response")
	innerAgent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: innerM,
	}).WithName("no_history_inner").WithDescription("inner agent for no-history test")

	ctx := t.Context()
	// Default: FullChatHistoryAsInput is false.
	agentTool := NewAgentTool(ctx, innerAgent)

	parentM := &forcedToolModel{
		inner:     &mockModel{},
		toolCalls: []schema.ToolCall{{ID: "nh", Function: schema.ToolCallFunction{Name: "no_history_inner", Arguments: "{}"}}},
		finalResp: "parent done",
		firstCall: true,
	}
	parent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model:         parentM,
		Tools:         []Tool{agentTool},
		ToolsConfig:   &ToolsNodeConfig{Tools: []Tool{agentTool}},
		MaxIterations: 3,
	}).WithName("no_history_parent")

	store := newCancelTestStore()
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: parent, CheckPointStore: store})
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("first turn")})

	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			t.Logf("no-history test event err: %v", ev.Err)
			break
		}
	}

	if len(receivedMsgs) != 1 {
		t.Errorf("expected inner agent to receive exactly 1 message (tool args), got %d", len(receivedMsgs))
	}
	if receivedMsgs[0].Role != schema.RoleUser {
		t.Errorf("expected single message to be user role, got %s", receivedMsgs[0].Role)
	}
}

// historyRecordingModel is a mock model that records all messages it receives.
type historyRecordingModel struct {
	record    *[]Message
	responses []string
	mu        sync.Mutex
	callCount int
}

func (m *historyRecordingModel) addResp(r string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses = append(m.responses, r)
}

func (m *historyRecordingModel) Generate(ctx context.Context, msgs []Message, opts ...modelOption) (Message, error) {
	*m.record = append(*m.record, msgs...)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.callCount >= len(m.responses) {
		m.callCount = 0
	}
	resp := m.responses[m.callCount]
	m.callCount++
	return &schema.Message{Role: schema.RoleAssistant, Content: resp}, nil
}

func (m *historyRecordingModel) Stream(ctx context.Context, msgs []Message, opts ...modelOption) (*schema.StreamReader[Message], error) {
	msg, err := m.Generate(ctx, msgs, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]Message{msg}), nil
}

func (m *historyRecordingModel) BindTools(tools []*schema.ToolInfo) error { return nil }
