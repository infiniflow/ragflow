// Package component — LLM unit tests.
//
// Tests use a stub ChatInvoker to avoid the network. The production path
// flows through einoChatInvoker + models.NewEinoChatModel + the real
// provider driver; here we focus on the component contract:
//   - inputs → outputs map shape
//   - json_output parsing
//   - Stream variant emits the same payload + closes
//   - error path surfaces invoker errors
//   - variable reference substitution is the canvas engine's job, not
//     this component's — we only verify the raw user_prompt is passed
//     through to the invoker.
package component

import (
	"context"
	"errors"
	"testing"

	"github.com/cloudwego/eino/schema"
)

// stubInvoker is a programmable ChatInvoker used by these tests.
type stubInvoker struct {
	resp     *ChatInvokeResponse
	err      error
	captured *ChatInvokeRequest
	calls    int
}

func (s *stubInvoker) Invoke(_ context.Context, req ChatInvokeRequest) (*ChatInvokeResponse, error) {
	s.calls++
	cp := req
	s.captured = &cp
	if s.err != nil {
		return nil, s.err
	}
	return s.resp, nil
}

// withStubInvoker swaps the package-level ChatInvoker for the duration of t.
func withStubInvoker(t *testing.T, s ChatInvoker) {
	t.Helper()
	prev := getDefaultChatInvoker()
	SetDefaultChatInvoker(s)
	t.Cleanup(func() { SetDefaultChatInvoker(prev) })
}

func TestLLM_Invoke_HappyPath(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "hello", Model: "echo-model", Stopped: true, Tokens: 7}}
	withStubInvoker(t, stub)

	c := NewLLMComponent(LLMParam{ModelID: "echo-model"})
	out, err := c.Invoke(context.Background(), map[string]any{
		"user_prompt": "hi",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := out["content"], "hello"; got != want {
		t.Errorf("content=%v, want %v", got, want)
	}
	if got, want := out["model"], "echo-model"; got != want {
		t.Errorf("model=%v, want %v", got, want)
	}
	if got, want := out["stopped"], true; got != want {
		t.Errorf("stopped=%v, want %v", got, want)
	}
	if stub.calls != 1 {
		t.Errorf("invoker calls=%d, want 1", stub.calls)
	}
	if stub.captured == nil || stub.captured.ModelName != "echo-model" {
		t.Errorf("ModelName not propagated: %+v", stub.captured)
	}
	if len(stub.captured.Messages) != 1 || stub.captured.Messages[0].Role != schema.User || stub.captured.Messages[0].Content != "hi" {
		t.Errorf("messages not built correctly: %+v", stub.captured.Messages)
	}
}

func TestLLM_Invoke_JSONOutput(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: `{"k":"v"}`, Model: "echo", Stopped: true}}
	withStubInvoker(t, stub)

	c := NewLLMComponent(LLMParam{ModelID: "echo"})
	out, err := c.Invoke(context.Background(), map[string]any{
		"user_prompt": "give me json",
		"json_output": true,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := out["content"], `{"k":"v"}`; got != want {
		t.Errorf("content=%v, want %v", got, want)
	}
	parsed, ok := out["json"].(map[string]any)
	if !ok {
		t.Fatalf("json output missing or wrong type: %T", out["json"])
	}
	if parsed["k"] != "v" {
		t.Errorf("json[k]=%v, want v", parsed["k"])
	}
}

func TestLLM_Invoke_SystemAndUser(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "ok", Model: "echo"}}
	withStubInvoker(t, stub)

	c := NewLLMComponent(LLMParam{ModelID: "echo"})
	_, err := c.Invoke(context.Background(), map[string]any{
		"system_prompt": "you are helpful",
		"user_prompt":   "say hi",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got := len(stub.captured.Messages); got != 2 {
		t.Fatalf("messages=%d, want 2", got)
	}
	if stub.captured.Messages[0].Role != schema.System || stub.captured.Messages[0].Content != "you are helpful" {
		t.Errorf("system msg wrong: %+v", stub.captured.Messages[0])
	}
	if stub.captured.Messages[1].Role != schema.User || stub.captured.Messages[1].Content != "say hi" {
		t.Errorf("user msg wrong: %+v", stub.captured.Messages[1])
	}
}

func TestLLM_Stream(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "streamed", Model: "echo", Stopped: true}}
	withStubInvoker(t, stub)

	c := NewLLMComponent(LLMParam{ModelID: "echo"})
	ch, err := c.Stream(context.Background(), map[string]any{"user_prompt": "go"})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	// Drain all chunks; the implementation emits content + done
	// over the goroutine-streaming pattern.
	var got []map[string]any
	for chunk := range ch {
		got = append(got, chunk)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 chunks (content + done), got %d", len(got))
	}
	if got[0]["content"] != "streamed" {
		t.Errorf("chunk[0].content=%v, want 'streamed'", got[0]["content"])
	}
	if got[1]["done"] != true {
		t.Errorf("chunk[1].done=%v, want true", got[1]["done"])
	}
}

func TestLLM_Invoke_MissingModelID(t *testing.T) {
	withStubInvoker(t, &stubInvoker{resp: &ChatInvokeResponse{Content: "should not be called"}})
	c := NewLLMComponent(LLMParam{}) // no model_id
	_, err := c.Invoke(context.Background(), map[string]any{"user_prompt": "x"})
	if err == nil {
		t.Fatal("expected ParamError for missing model_id")
	}
	var pe *ParamError
	if !errors.As(err, &pe) {
		t.Errorf("err type=%T, want *ParamError", err)
	}
}

func TestLLM_Invoke_InvokerError(t *testing.T) {
	stub := &stubInvoker{err: errors.New("upstream blew up")}
	withStubInvoker(t, stub)
	c := NewLLMComponent(LLMParam{ModelID: "echo"})
	_, err := c.Invoke(context.Background(), map[string]any{"user_prompt": "x"})
	if err == nil {
		t.Fatal("expected error to propagate")
	}
	if stub.calls != 1 {
		t.Errorf("calls=%d, want 1", stub.calls)
	}
}

func TestLLM_Registered(t *testing.T) {
	names := RegisteredNames()
	found := false
	for _, n := range names {
		if n == "llm" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("LLM not registered; names=%v", names)
	}
	// And a factory round-trip.
	c, err := New("LLM", map[string]any{"model_id": "echo"})
	if err != nil {
		t.Fatalf("New(LLM): %v", err)
	}
	if c.Name() != "LLM" {
		t.Errorf("Name()=%q, want LLM", c.Name())
	}
}
