package core

import (
	"context"
	"io"
	"testing"

	"ragflow/internal/harness/core/schema"
)

// ---- BuildModelWrapperChain tests ----

func TestBuildModelWrapperChain_NoConfig(t *testing.T) {
	base := &mockModel{}
	base.addResp("raw")
	model := BuildModelWrapperChain(base, nil, DefaultReActConfig[*schema.Message](), nil)
	if model == nil {
		t.Fatal("nil model")
	}
	resp, err := model.Generate(context.Background(), []Message{schema.UserMessage("hi")})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Content != "raw" {
		t.Errorf("content = %s", resp.Content)
	}
}

func TestBuildModelWrapperChain_WithRetry(t *testing.T) {
	base := &mockModel{}
	base.addResp("retry-ok")
	cfg := DefaultReActConfig[*schema.Message]()
	cfg.RetryConfig = &ModelRetryConfig{MaxRetries: 2}
	model := BuildModelWrapperChain(base, nil, cfg, nil)
	if model == nil {
		t.Fatal("nil model")
	}
	resp, err := model.Generate(context.Background(), []Message{schema.UserMessage("hi")})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Content != "retry-ok" {
		t.Errorf("content = %s", resp.Content)
	}
}

func TestBuildModelWrapperChain_WithFailover(t *testing.T) {
	primary := &mockModel{}
	primary.addResp("primary-ok")
	fallback := &mockModel{}
	fallback.addResp("fallback")

	cfg := DefaultReActConfig[*schema.Message]()
	cfg.FailoverConfig = &FailoverConfigMsg{Models: []Model[*schema.Message]{fallback}}
	model := BuildModelWrapperChain(primary, nil, cfg, nil)
	resp, err := model.Generate(context.Background(), []Message{schema.UserMessage("hi")})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Content != "primary-ok" {
		t.Errorf("content = %s", resp.Content)
	}
}

func TestBuildModelWrapperChain_WithMiddleware(t *testing.T) {
	var wrapCalled bool
	mw := &testMiddleware{}
	mw.wrapModel = func(ctx context.Context, m Model[*schema.Message], mc *ModelContext) (Model[*schema.Message], error) {
		wrapCalled = true
		return m, nil
	}
	base := &mockModel{}
	base.addResp("mw-ok")
	cfg := DefaultReActConfig[*schema.Message]()
	cfg.Middlewares = []ReActMiddleware{mw}
	model := BuildModelWrapperChain(base, nil, cfg, nil)
	resp, err := model.Generate(context.Background(), []Message{schema.UserMessage("hi")})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !wrapCalled {
		t.Error("middleware WrapModel not called")
	}
	_ = resp
}

func TestBuildModelWrapperChain_WithFullChain(t *testing.T) {
	var wrapCalled bool
	mw := &testMiddleware{}
	mw.wrapModel = func(ctx context.Context, m Model[*schema.Message], mc *ModelContext) (Model[*schema.Message], error) {
		wrapCalled = true
		return m, nil
	}

	primary := &mockModel{}
	primary.addResp("chain-ok")
	fallback := &mockModel{}
	fallback.addResp("fallback")

	cfg := DefaultReActConfig[*schema.Message]()
	cfg.RetryConfig = &ModelRetryConfig{MaxRetries: 2}
	cfg.FailoverConfig = &FailoverConfigMsg{Models: []Model[*schema.Message]{fallback}}
	cfg.Middlewares = []ReActMiddleware{mw}

	model := BuildModelWrapperChain(primary, nil, cfg, nil)
	resp, err := model.Generate(context.Background(), []Message{schema.UserMessage("hi")})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !wrapCalled {
		t.Error("middleware WrapModel not called in chain")
	}
	_ = resp
}

func TestBuildModelWrapperChain_NilMiddleware(t *testing.T) {
	base := &mockModel{}
	base.addResp("nil-mw")
	cfg := DefaultReActConfig[*schema.Message]()
	cfg.Middlewares = []ReActMiddleware{nil, nil}
	model := BuildModelWrapperChain(base, nil, cfg, nil)
	_, err := model.Generate(context.Background(), []Message{schema.UserMessage("hi")})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
}

// ---- eventSenderModelWrapper tests ----

func TestEventSenderModelWrapper_GenerateSendsEvent(t *testing.T) {
	base := &mockModel{}
	base.addResp("event-test")
	it, gen := NewAsyncIteratorPair[*TypedAgentEvent[*schema.Message]]()
	ec := &reActExecCtx{generator: gen}
	wrapped := wrapModelWithEventSender(base, ec)

	go func() {
		defer gen.Close()
		resp, err := wrapped.Generate(context.Background(), []Message{schema.UserMessage("hi")})
		if err != nil {
			t.Errorf("Generate: %v", err)
		}
		if resp.Content != "event-test" {
			t.Errorf("content = %s", resp.Content)
		}
	}()

	ev, ok := it.Next()
	if !ok {
		t.Fatal("expected event from wrapper")
	}
	if ev.Output == nil || ev.Output.MessageOutput == nil {
		t.Error("expected message output event")
	}
}

func TestEventSenderModelWrapper_StreamSendsEvent(t *testing.T) {
	base := &mockModel{}
	base.addResp("stream-event")
	it, gen := NewAsyncIteratorPair[*TypedAgentEvent[*schema.Message]]()
	ec := &reActExecCtx{generator: gen}
	wrapped := wrapModelWithEventSender(base, ec)

	go func() {
		defer gen.Close()
		s, err := wrapped.Stream(context.Background(), []Message{schema.UserMessage("hi")})
		if err != nil {
			t.Errorf("Stream: %v", err)
		}
		for {
			_, err := s.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Errorf("Recv: %v", err)
				return
			}
		}
	}()

	ev, ok := it.Next()
	if !ok {
		t.Fatal("expected event from stream wrapper")
	}
	if ev.Output == nil || ev.Output.MessageOutput == nil {
		t.Error("expected message output event from stream")
	}
}

func TestEventSenderModelWrapper_SuppressEventSend(t *testing.T) {
	base := &mockModel{}
	base.addResp("suppressed")
	it, gen := NewAsyncIteratorPair[*TypedAgentEvent[*schema.Message]]()
	ec := &reActExecCtx{generator: gen, suppressEventSend: true}
	wrapped := wrapModelWithEventSender(base, ec)

	done := make(chan struct{})
	go func() {
		defer close(done)
		defer gen.Close()
		_, err := wrapped.Generate(context.Background(), []Message{schema.UserMessage("hi")})
		if err != nil {
			t.Errorf("Generate: %v", err)
		}
	}()

	// After Generate returns, check that the channel has no events
	<-done
	// The generator is closed; try reading one item. If suppressEventSend works,
	// the closed empty channel returns a zero value immediately.
	_, ok := it.Next()
	// When the channel is closed and empty, Next() returns (zero, false)
	if ok {
		t.Error("event should be suppressed")
	}
}

func TestEventSenderModelWrapper_NilExecCtx(t *testing.T) {
	base := &mockModel{}
	base.addResp("nil-ec")
	wrapped := wrapModelWithEventSender(base, nil)
	resp, err := wrapped.Generate(context.Background(), []Message{schema.UserMessage("hi")})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Content != "nil-ec" {
		t.Errorf("content = %s", resp.Content)
	}
}

func TestEventSenderModelWrapper_NilGenerator(t *testing.T) {
	base := &mockModel{}
	base.addResp("nil-gen")
	ec := &reActExecCtx{generator: nil}
	wrapped := wrapModelWithEventSender(base, ec)
	resp, err := wrapped.Generate(context.Background(), []Message{schema.UserMessage("hi")})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Content != "nil-gen" {
		t.Errorf("content = %s", resp.Content)
	}
}

func TestEventSenderModelWrapper_BindTools(t *testing.T) {
	base := &mockModel{}
	wrapped := wrapModelWithEventSender(base, nil)
	err := wrapped.BindTools([]*schema.ToolInfo{{Name: "test"}})
	if err != nil {
		t.Fatalf("BindTools: %v", err)
	}
}

func TestEventSenderModelWrapper_IsNilMessage(t *testing.T) {
	base := &mockModel{}
	base.addResp("")
	it, gen := NewAsyncIteratorPair[*TypedAgentEvent[*schema.Message]]()
	ec := &reActExecCtx{generator: gen}
	wrapped := wrapModelWithEventSender(base, ec)

	done := make(chan struct{})
	go func() {
		defer gen.Close()
		_, err := wrapped.Generate(context.Background(), []Message{schema.UserMessage("hi")})
		if err != nil {
			t.Errorf("Generate: %v", err)
		}
		close(done)
	}()
	// Empty content might still send the event because it's not nil
	var hasEvent bool
	select {
	case <-done:
	case <-it.ch:
		hasEvent = true
	}
	_ = hasEvent
}

// ---- callbackModelWrapper tests ----

func TestCallbackModelWrapper_Basic(t *testing.T) {
	inner := &mockModel{}
	inner.addResp("cb-ok")
	wrapped := &callbackModelWrapper[*schema.Message]{inner: inner}
	resp, err := wrapped.Generate(context.Background(), []Message{schema.UserMessage("hi")})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Content != "cb-ok" {
		t.Errorf("content = %s", resp.Content)
	}
}

func TestCallbackModelWrapper_Stream(t *testing.T) {
	inner := &mockModel{}
	inner.addResp("cb-stream")
	wrapped := &callbackModelWrapper[*schema.Message]{inner: inner}
	s, err := wrapped.Stream(context.Background(), []Message{schema.UserMessage("hi")})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	chunk, err := s.Recv()
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	if chunk.Content != "cb-stream" {
		t.Errorf("content = %s", chunk.Content)
	}
}

func TestCallbackModelWrapper_BindTools(t *testing.T) {
	inner := &mockModel{}
	wrapped := &callbackModelWrapper[*schema.Message]{inner: inner}
	err := wrapped.BindTools([]*schema.ToolInfo{{Name: "test"}})
	if err != nil {
		t.Fatalf("BindTools: %v", err)
	}
}

// ---- HasUserEventSenderModelWrapper tests ----

func TestHasUserEventSenderModelWrapper_NilSlice(t *testing.T) {
	if HasUserEventSenderModelWrapper[*schema.Message](nil) {
		t.Error("nil should be false")
	}
}

func TestHasUserEventSenderModelWrapper_WithWrapper(t *testing.T) {
	w := NewEventSenderModelWrapper[*schema.Message]()
	handlers := []TypedReActMiddleware[*schema.Message]{w}
	if !HasUserEventSenderModelWrapper(handlers) {
		t.Error("should detect wrapper")
	}
}

func TestHasUserEventSenderModelWrapper_WithoutWrapper(t *testing.T) {
	mw := &testMiddleware{}
	handlers := []TypedReActMiddleware[*schema.Message]{mw}
	if HasUserEventSenderModelWrapper(handlers) {
		t.Error("should not detect non-wrapper")
	}
}

// ---- Wrapper behavior with ExecCtx integration ----

func TestEventSenderModelWrapper_WithExecCtxGenerator(t *testing.T) {
	base := &mockModel{}
	base.addResp("gen-event")
	it, gen := NewAsyncIteratorPair[*TypedAgentEvent[*schema.Message]]()
	ec := &reActExecCtx{generator: gen}
	wrapped := wrapModelWithEventSender(base, ec)

	go func() {
		defer gen.Close()
		_, err := wrapped.Generate(context.Background(), []Message{schema.UserMessage("hi")})
		if err != nil {
			t.Errorf("Generate: %v", err)
		}
	}()

	// Read event from iterator to verify generator integration works
	ev, ok := it.Next()
	if !ok {
		t.Fatal("expected event via generator")
	}
	if ev.Output == nil || ev.Output.MessageOutput == nil {
		t.Error("expected message output event")
	}
}
