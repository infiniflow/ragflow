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
				Message: &schema.Message{
					Role: schema.RoleAssistant,
					ToolCalls: []schema.ToolCall{
						{ID: "call_1", Function: schema.ToolCallFunction{Name: "lookup", Arguments: "{}"}},
					},
				},
			},
		},
	})
	sess.addEvent(&AgentEvent{
		Output: &AgentOutput{
			MessageOutput: &MessageVariant{
				Message: schema.ToolMessage("lookup result", "call_1"),
			},
		},
	})
	sess.addEvent(&AgentEvent{
		Output: &AgentOutput{
			MessageOutput: &MessageVariant{
				Message: schema.AssistantMessage("Turn 1 assistant reply"),
			},
		},
	})
	// Parent assistant message with pending tool call currently executing inner agent:
	sess.addEvent(&AgentEvent{
		Output: &AgentOutput{
			MessageOutput: &MessageVariant{
				Message: &schema.Message{
					Role: schema.RoleAssistant,
					ToolCalls: []schema.ToolCall{
						{ID: "call_pending", Function: schema.ToolCallFunction{Name: "history_inner", Arguments: "Turn 2 sub-agent query"}},
					},
				},
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
	// Expected messages:
	// 0: User: "Turn 1 user query"
	// 1: Assistant with completed call_1
	// 2: Tool: "lookup result" (call_1)
	// 3: Assistant: "Turn 1 assistant reply"
	// (Pending call_pending is stripped/omitted)
	// 4: User: "Turn 2 sub-agent query"
	if len(msgs) != 5 {
		t.Fatalf("expected 5 dialogue messages, got %d: %v", len(msgs), msgs)
	}
	if msgs[0].Content != "Turn 1 user query" {
		t.Errorf("expected first message to be 'Turn 1 user query', got %q", msgs[0].Content)
	}
	if len(msgs[1].ToolCalls) != 1 || msgs[1].ToolCalls[0].ID != "call_1" {
		t.Errorf("expected completed tool call 'call_1', got %v", msgs[1].ToolCalls)
	}
	if msgs[2].Role != schema.RoleTool || msgs[2].Name != "call_1" {
		t.Errorf("expected tool result for call_1, got %v", msgs[2])
	}
	if msgs[3].Content != "Turn 1 assistant reply" {
		t.Errorf("expected fourth message to be 'Turn 1 assistant reply', got %q", msgs[3].Content)
	}
	if msgs[4].Content != "Turn 2 sub-agent query" {
		t.Errorf("expected fifth message to be 'Turn 2 sub-agent query', got %q", msgs[4].Content)
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

// TestAgentTool_FullChatHistory_DeduplicateRestoredSession verifies that in resume
// or restored session flows where identical turns exist in both RootInput.Messages
// and Session historical events, turns are not duplicated in the sub-agent input.
func TestAgentTool_FullChatHistory_DeduplicateRestoredSession(t *testing.T) {
	captureM := &capturingHistoryModel{resp: "inner captured"}
	innerAgent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: captureM,
	}).WithName("dedupe_inner").WithDescription("inner agent")

	toolWithHistory := NewAgentTool(t.Context(), innerAgent, WithFullChatHistoryAsInput())

	sess := newRunSession()
	// Session was restored with prior events that match the turns in RootInput:
	sess.addEvent(&AgentEvent{
		Output: &AgentOutput{
			MessageOutput: &MessageVariant{
				Message: &schema.Message{
					Role:    schema.RoleAssistant,
					Content: "Prior assistant reply",
				},
			},
		},
	})
	// Plus an event generated in the current run before the sub-agent call:
	sess.addEvent(&AgentEvent{
		Output: &AgentOutput{
			MessageOutput: &MessageVariant{
				Message: schema.ToolMessage("calc result: 42", "calc_1"),
			},
		},
	})

	rc := &runContext{
		RootInput: &AgentInput{
			Messages: []Message{
				schema.UserMessage("Prior user query"),
				schema.AssistantMessage("Prior assistant reply"),
			},
		},
		Session: sess,
	}
	ctx := context.WithValue(t.Context(), runContextKey{}, rc)

	res, err := toolWithHistory.Invoke(ctx, "Next turn query")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != "inner captured" {
		t.Fatalf("unexpected result: %q", res)
	}

	msgs := captureM.getCapturedMsgs()
	if len(msgs) > 0 && msgs[0].Role == schema.RoleSystem {
		msgs = msgs[1:]
	}

	// Expected:
	// 0: User: "Prior user query"
	// 1: Assistant: "Prior assistant reply" (only once, NOT duplicated from session)
	// (calc_1 is an unmatched tool result without a preceding assistant tool call, so it is omitted)
	// 2: User: "Next turn query"
	if len(msgs) != 3 {
		t.Fatalf("expected 3 dialogue messages after deduplication and omitting unmatched tool result, got %d: %v", len(msgs), msgs)
	}
	if msgs[0].Content != "Prior user query" {
		t.Errorf("expected msgs[0] = 'Prior user query', got %q", msgs[0].Content)
	}
	if msgs[1].Content != "Prior assistant reply" {
		t.Errorf("expected msgs[1] = 'Prior assistant reply', got %q", msgs[1].Content)
	}
	if msgs[2].Content != "Next turn query" {
		t.Errorf("expected msgs[2] = 'Next turn query', got %q", msgs[2].Content)
	}
}

// TestAgentTool_FullChatHistory_RepeatedOrdinaryTurnsPreserved verifies that sequential
// prefix matching deduplicates only the restored turns matching RootInput, while preserving
// later ordinary conversation turns even if they have identical content (e.g. repeated "OK").
func TestAgentTool_FullChatHistory_RepeatedOrdinaryTurnsPreserved(t *testing.T) {
	captureM := &capturingHistoryModel{resp: "inner captured"}
	innerAgent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: captureM,
	}).WithName("repeated_inner").WithDescription("inner agent")

	toolWithHistory := NewAgentTool(t.Context(), innerAgent, WithFullChatHistoryAsInput())

	sess := newRunSession()
	// Turn 1 reply restored in session:
	sess.addEvent(&AgentEvent{
		Output: &AgentOutput{
			MessageOutput: &MessageVariant{
				Message: schema.AssistantMessage("OK"),
			},
		},
	})
	// Turn 2 in normal conversation flow, where assistant again replies "OK":
	sess.addEvent(&AgentEvent{
		Output: &AgentOutput{
			MessageOutput: &MessageVariant{
				Message: schema.UserMessage("Are you sure?"),
			},
		},
	})
	sess.addEvent(&AgentEvent{
		Output: &AgentOutput{
			MessageOutput: &MessageVariant{
				Message: schema.AssistantMessage("OK"),
			},
		},
	})

	rc := &runContext{
		RootInput: &AgentInput{
			Messages: []Message{
				schema.UserMessage("Hello"),
				schema.AssistantMessage("OK"),
			},
		},
		Session: sess,
	}
	ctx := context.WithValue(t.Context(), runContextKey{}, rc)

	res, err := toolWithHistory.Invoke(ctx, "Proceed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != "inner captured" {
		t.Fatalf("unexpected result: %q", res)
	}

	msgs := captureM.getCapturedMsgs()
	if len(msgs) > 0 && msgs[0].Role == schema.RoleSystem {
		msgs = msgs[1:]
	}

	// Expected:
	// 0: User: "Hello"
	// 1: Assistant: "OK" (restored prefix, deduplicated)
	// 2: User: "Are you sure?"
	// 3: Assistant: "OK" (later ordinary turn preserved)
	// 4: User: "Proceed"
	if len(msgs) != 5 {
		t.Fatalf("expected 5 dialogue messages, got %d: %v", len(msgs), msgs)
	}
	if msgs[0].Content != "Hello" {
		t.Errorf("expected msgs[0] = 'Hello', got %q", msgs[0].Content)
	}
	if msgs[1].Content != "OK" {
		t.Errorf("expected msgs[1] = 'OK', got %q", msgs[1].Content)
	}
	if msgs[2].Content != "Are you sure?" {
		t.Errorf("expected msgs[2] = 'Are you sure?', got %q", msgs[2].Content)
	}
	if msgs[3].Content != "OK" {
		t.Errorf("expected msgs[3] = 'OK', got %q", msgs[3].Content)
	}
	if msgs[4].Content != "Proceed" {
		t.Errorf("expected msgs[4] = 'Proceed', got %q", msgs[4].Content)
	}
}

// TestAgentTool_FullChatHistory_ReusedToolCallID verifies occurrence-based matching
// ensures a completed tool call is retained while a subsequent pending tool call reusing
// the same ID is stripped.
func TestAgentTool_FullChatHistory_ReusedToolCallID(t *testing.T) {
	captureM := &capturingHistoryModel{resp: "inner captured"}
	innerAgent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: captureM,
	}).WithName("reused_id_inner").WithDescription("inner agent")

	toolWithHistory := NewAgentTool(t.Context(), innerAgent, WithFullChatHistoryAsInput())

	sess := newRunSession()
	// Turn 1: assistant calls fetch with ID "call_shared"
	sess.addEvent(&AgentEvent{
		Output: &AgentOutput{
			MessageOutput: &MessageVariant{
				Message: &schema.Message{
					Role: schema.RoleAssistant,
					ToolCalls: []schema.ToolCall{
						{ID: "call_shared", Function: schema.ToolCallFunction{Name: "fetch", Arguments: "{}"}},
					},
				},
			},
		},
	})
	// Turn 1: tool returns result for "call_shared"
	sess.addEvent(&AgentEvent{
		Output: &AgentOutput{
			MessageOutput: &MessageVariant{
				Message: schema.ToolMessage("data 1", "call_shared"),
			},
		},
	})
	// Turn 2: assistant makes another call that reuses ID "call_shared" but is still pending (no tool result)
	sess.addEvent(&AgentEvent{
		Output: &AgentOutput{
			MessageOutput: &MessageVariant{
				Message: &schema.Message{
					Role: schema.RoleAssistant,
					ToolCalls: []schema.ToolCall{
						{ID: "call_shared", Function: schema.ToolCallFunction{Name: "fetch", Arguments: "{}"}},
					},
				},
			},
		},
	})

	rc := &runContext{
		RootInput: &AgentInput{
			Messages: []Message{
				schema.UserMessage("Run twice"),
			},
		},
		Session: sess,
	}
	ctx := context.WithValue(t.Context(), runContextKey{}, rc)

	_, err := toolWithHistory.Invoke(ctx, "sub-agent task")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msgs := captureM.getCapturedMsgs()
	if len(msgs) > 0 && msgs[0].Role == schema.RoleSystem {
		msgs = msgs[1:]
	}

	// Expected:
	// 0: User: "Run twice"
	// 1: Assistant with completed call_shared
	// 2: Tool: "data 1" (call_shared)
	// (Pending second call_shared stripped and omitted)
	// 3: User: "sub-agent task"
	if len(msgs) != 4 {
		t.Fatalf("expected 4 dialogue messages, got %d: %v", len(msgs), msgs)
	}
	if len(msgs[1].ToolCalls) != 1 || msgs[1].ToolCalls[0].ID != "call_shared" {
		t.Errorf("expected first tool call retained, got %v", msgs[1].ToolCalls)
	}
	if msgs[2].Role != schema.RoleTool || msgs[2].Name != "call_shared" {
		t.Errorf("expected tool result retained, got %v", msgs[2])
	}
	if msgs[3].Content != "sub-agent task" {
		t.Errorf("expected msgs[3] = 'sub-agent task', got %q", msgs[3].Content)
	}
}

// TestAgentTool_FullChatHistory_ExcludesParentSystemPrompt verifies that any RoleSystem
// messages in RootInput (parent agent's instructions) are excluded from sub-agent history
// so they do not conflict with the sub-agent's own system prompt.
func TestAgentTool_FullChatHistory_ExcludesParentSystemPrompt(t *testing.T) {
	captureM := &capturingHistoryModel{resp: "inner captured"}
	innerAgent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: captureM,
	}).WithName("nosys_inner").WithDescription("inner agent")

	toolWithHistory := NewAgentTool(t.Context(), innerAgent, WithFullChatHistoryAsInput())

	rc := &runContext{
		RootInput: &AgentInput{
			Messages: []Message{
				schema.SystemMessage("You are the parent coordinator"),
				schema.UserMessage("Parent user query"),
			},
		},
	}
	ctx := context.WithValue(t.Context(), runContextKey{}, rc)

	res, err := toolWithHistory.Invoke(ctx, "Sub-agent query")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != "inner captured" {
		t.Fatalf("unexpected result: %q", res)
	}

	msgs := captureM.getCapturedMsgs()
	// Strip the sub-agent's own system prompt inserted by ReActAgent
	if len(msgs) > 0 && msgs[0].Role == schema.RoleSystem {
		msgs = msgs[1:]
	}

	// Expected: only parent user message + current sub-agent query (no parent system prompt!)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 dialogue messages, got %d: %v", len(msgs), msgs)
	}
	if msgs[0].Role != schema.RoleUser || msgs[0].Content != "Parent user query" {
		t.Errorf("expected msgs[0] = User('Parent user query'), got %v", msgs[0])
	}
	if msgs[1].Role != schema.RoleUser || msgs[1].Content != "Sub-agent query" {
		t.Errorf("expected msgs[1] = User('Sub-agent query'), got %v", msgs[1])
	}
}

func init() {
	schema.RegisterType("_test_agent_tool", func() any { return &AgentToolOptions{} })
}
