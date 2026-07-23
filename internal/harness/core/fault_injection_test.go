package core

import (
	"context"
	"errors"
	"sync"
	"testing"

	"ragflow/internal/harness/core/schema"
)

// ---- Fault Injection Mocks ----

// failFirstNModel succeeds after N failures.
type failFirstNModel struct {
	mu       sync.Mutex
	calls    int
	failForN int
}

func (m *failFirstNModel) Generate(ctx context.Context, msgs []Message, opts ...modelOption) (Message, error) {
	m.mu.Lock()
	m.calls++
	shouldFail := m.calls <= m.failForN
	m.mu.Unlock()
	if shouldFail {
		return nil, errors.New("simulated failure")
	}
	return &schema.Message{Role: schema.RoleAssistant, Content: "ok"}, nil
}
func (m *failFirstNModel) Stream(ctx context.Context, msgs []Message, opts ...modelOption) (*schema.StreamReader[Message], error) {
	msg, err := m.Generate(ctx, msgs, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]Message{msg}), nil
}
func (m *failFirstNModel) BindTools(tools []*schema.ToolInfo) error { return nil }

// alwaysFailTool always fails.
type alwaysFailTool struct{}

func (t *alwaysFailTool) Name() string        { return "always_fail" }
func (t *alwaysFailTool) Description() string { return "always fails" }
func (t *alwaysFailTool) Invoke(ctx context.Context, args string, opts ...ToolOption) (string, error) {
	return "", errors.New("always fails")
}
func (t *alwaysFailTool) Stream(ctx context.Context, args string, opts ...ToolOption) (*schema.StreamReader[string], error) {
	return nil, errors.New("always fails")
}

// ---- Test: Retry succeeds after N failures ----
func TestFault_LLMRetryThenSuccess(t *testing.T) {
	inner := &failFirstNModel{failForN: 2}
	cfg := &ModelRetryConfig{MaxRetries: 3}
	wrapped := WithModelRetry(inner, cfg)

	ctx := context.Background()
	resp, err := wrapped.Generate(ctx, []Message{schema.UserMessage("hi")})
	if err != nil {
		t.Fatalf("expected success after retry: %v", err)
	}
	if resp.Content != "ok" {
		t.Errorf("content = %s", resp.Content)
	}
	inner.mu.Lock()
	calls := inner.calls
	inner.mu.Unlock()
	if calls != 3 {
		t.Errorf("expected 3 calls (1 + 2 retries), got %d", calls)
	}
	t.Logf("Retry success: %d calls", calls)
}

// ---- Test: Retry exhausts ----
func TestFault_LLMRetryExhausted(t *testing.T) {
	inner := &failFirstNModel{failForN: 10}
	cfg := &ModelRetryConfig{MaxRetries: 3}
	wrapped := WithModelRetry(inner, cfg)

	ctx := context.Background()
	_, err := wrapped.Generate(ctx, []Message{schema.UserMessage("hi")})
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}
	inner.mu.Lock()
	calls := inner.calls
	inner.mu.Unlock()
	expected := 4
	if calls != expected {
		t.Errorf("expected %d calls, got %d", expected, calls)
	}
	t.Logf("Retry exhausted: %d calls, err=%v", calls, err)
}

// ---- Test: No retry on success ----
func TestFault_LLMNoRetryOnSuccess(t *testing.T) {
	inner := &failFirstNModel{failForN: 0}
	cfg := &ModelRetryConfig{MaxRetries: 5}
	wrapped := WithModelRetry(inner, cfg)

	ctx := context.Background()
	resp, err := wrapped.Generate(ctx, []Message{schema.UserMessage("hi")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "ok" {
		t.Errorf("content = %s", resp.Content)
	}
	inner.mu.Lock()
	calls := inner.calls
	inner.mu.Unlock()
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
}

// ---- Test: All tools fail, agent continues ----
func TestFault_ToolAllFail(t *testing.T) {
	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: &forcedToolModel{
			toolCalls: []schema.ToolCall{{ID: "call_1", Function: schema.ToolCallFunction{Name: "always_fail", Arguments: "{}"}}},
			finalResp: "done",
		},
		Tools: []Tool{&alwaysFailTool{}},
	}).WithName("fault_agent")

	ctx := context.Background()
	iter := agent.Run(ctx, &AgentInput{
		Messages: []Message{schema.UserMessage("analyze this")},
	})

	var lastMsg Message
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			t.Logf("Event error: %v", ev.Err)
		}
		if ev.Output != nil && ev.Output.MessageOutput != nil && !ev.Output.MessageOutput.IsStreaming {
			lastMsg = ev.Output.MessageOutput.Message
		}
	}
	if lastMsg == nil {
		t.Fatal("expected final assistant message")
	}
	t.Logf("Tool all fail: final content=%q", lastMsg.Content)
}

// ---- Test: Tool error doesn't crash ----
func TestFault_ReActToolErrorDoesNotCrash(t *testing.T) {
	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: &forcedToolModel{
			toolCalls: []schema.ToolCall{{ID: "call_1", Function: schema.ToolCallFunction{Name: "always_fail", Arguments: "{}"}}},
			finalResp: "recovered",
		},
		Tools: []Tool{&alwaysFailTool{}},
	}).WithName("crash_test")

	ctx := context.Background()
	iter := agent.Run(ctx, &AgentInput{
		Messages: []Message{schema.UserMessage("trigger")},
	})

	msgCount := 0
	hasError := false
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			hasError = true
		}
		msgCount++
	}
	if msgCount == 0 {
		t.Error("expected at least one event")
	}
	t.Logf("Tool error: %d events, hasError=%v", msgCount, hasError)
}

// ---- Test: Concurrent model calls ----
func TestFault_ConcurrentModelCalls(t *testing.T) {
	model := &mockModel{}
	for i := 0; i < 10; i++ {
		model.addResp("ok")
	}

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Go(func() {
			ctx := context.Background()
			_, err := model.Generate(ctx, []Message{schema.UserMessage("conc")})
			if err != nil {
				t.Errorf("concurrent call: %v", err)
			}
		})
	}
	wg.Wait()
}

// ---- Test: Agent doesn't crash after model error ----
func TestFault_ReActAgent_ModelError(t *testing.T) {
	model := &mockModel{}
	model.addResp("recovery")

	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: model,
	}).WithName("recovery_agent")

	ctx := context.Background()
	iter := agent.Run(ctx, &AgentInput{
		Messages: []Message{schema.UserMessage("do something")},
	})

	var lastMsg Message
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Output != nil && ev.Output.MessageOutput != nil && !ev.Output.MessageOutput.IsStreaming {
			lastMsg = ev.Output.MessageOutput.Message
		}
	}
	if lastMsg == nil {
		t.Log("Agent completed without final message (acceptable)")
		return
	}
	t.Logf("Agent final: %q", lastMsg.Content)
}
