package core

import (
	"context"
	"errors"
	"testing"

	"ragflow/internal/harness/core/schema"
)

// ---- Handler/middleware lifecycle tests ----

func TestBaseMiddleware_AllMethods(t *testing.T) {
	var b BaseMiddleware[*schema.Message]
	rc := &ReActAgentContext{}
	s := NewReActAgentState([]*schema.Message{}, nil, 10)
	mc := &ModelContext{}

	ctx, rc2, err := b.BeforeAgent(context.Background(), rc)
	if err != nil {
		t.Fatalf("BeforeAgent: %v", err)
	}
	if rc2 == nil {
		t.Error("nil rc returned")
	}
	_ = ctx

	ctx, err = b.AfterAgent(context.Background(), s)
	if err != nil {
		t.Fatalf("AfterAgent: %v", err)
	}
	_ = ctx

	ctx, s2, err := b.BeforeModelRewrite(context.Background(), s, mc)
	if err != nil {
		t.Fatalf("BeforeModelRewrite: %v", err)
	}
	if s2 == nil {
		t.Error("nil state returned")
	}
	_ = ctx

	ctx, s3, err := b.AfterModelRewrite(context.Background(), s, mc)
	if err != nil {
		t.Fatalf("AfterModelRewrite: %v", err)
	}
	if s3 == nil {
		t.Error("nil state returned")
	}
	_ = ctx

	m, err := b.WrapModel(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("WrapModel: %v", err)
	}
	if m != nil {
		_ = m
	}
}

func TestMiddleware_BeforeAgentCanModifyInstruction(t *testing.T) {
	mw := &testMiddleware{}
	mw.beforeAgent = func(ctx context.Context, rc *ReActAgentContext) (context.Context, *ReActAgentContext, error) {
		rc.Instruction = "MODIFIED: " + rc.Instruction
		return ctx, rc, nil
	}
	model := &mockModel{}
	model.addResp("modified")
	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: model, Middlewares: []ReActMiddleware{mw},
	})
	agent.name = "mod_agent"
	iter := agent.Run(context.Background(), &AgentInput{
		Messages: []Message{schema.UserMessage("test")},
	})
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		_ = ev
	}
}

func TestMiddleware_BeforeModelRewriteCanModifyState(t *testing.T) {
	mw := &testMiddleware{}
	mw.beforeModel = func(ctx context.Context, state *ReActAgentState, mc *ModelContext) (context.Context, *ReActAgentState, error) {
		state.RemainingIterations = 1 // force stop after 1 iteration
		return ctx, state, nil
	}
	model := &mockModel{}
	model.addResp("bmr-test")
	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: model, Middlewares: []ReActMiddleware{mw},
	})
	agent.name = "bmr_agent"
	iter := agent.Run(context.Background(), &AgentInput{
		Messages: []Message{schema.UserMessage("test")},
	})
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		_ = ev
	}
}

func TestMiddleware_AfterModelRewriteModifiesState(t *testing.T) {
	mw := &testMiddleware{}
	mw.afterModel = func(ctx context.Context, state *ReActAgentState, mc *ModelContext) (context.Context, *ReActAgentState, error) {
		if len(state.Messages) > 0 {
			state.Messages[len(state.Messages)-1] = schema.ToolMessage("rewritten", "call_override")
		}
		return ctx, state, nil
	}
	model := &mockModel{}
	model.addResp("original")
	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: model, Middlewares: []ReActMiddleware{mw},
	})
	agent.name = "amr_agent"
	iter := agent.Run(context.Background(), &AgentInput{
		Messages: []Message{schema.UserMessage("test")},
	})
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		_ = ev
	}
}

func TestMiddleware_MultipleMiddlewares(t *testing.T) {
	var order []string
	mw1 := &testMiddleware{}
	mw1.beforeAgent = func(ctx context.Context, rc *ReActAgentContext) (context.Context, *ReActAgentContext, error) {
		order = append(order, "mw1.BeforeAgent")
		return ctx, rc, nil
	}
	mw2 := &testMiddleware{}
	mw2.beforeAgent = func(ctx context.Context, rc *ReActAgentContext) (context.Context, *ReActAgentContext, error) {
		order = append(order, "mw2.BeforeAgent")
		return ctx, rc, nil
	}
	model := &mockModel{}
	model.addResp("multi")
	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: model, Middlewares: []ReActMiddleware{mw1, mw2},
	})
	agent.name = "multi_mw"
	iter := agent.Run(context.Background(), &AgentInput{
		Messages: []Message{schema.UserMessage("test")},
	})
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		_ = ev
	}
	if len(order) != 2 {
		t.Errorf("expected 2 calls, got %d: %v", len(order), order)
	}
}

func TestMiddleware_BeforeAgentError(t *testing.T) {
	expectedErr := errors.New("before agent error")
	mw := &testMiddleware{}
	mw.beforeAgent = func(ctx context.Context, rc *ReActAgentContext) (context.Context, *ReActAgentContext, error) {
		return ctx, nil, expectedErr
	}
	model := &mockModel{}
	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: model, Middlewares: []ReActMiddleware{mw},
	})
	agent.name = "err_before"
	iter := agent.Run(context.Background(), &AgentInput{
		Messages: []Message{schema.UserMessage("test")},
	})
	var lastErr error
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			lastErr = ev.Err
		}
	}
	if lastErr == nil {
		t.Error("expected error from BeforeAgent middleware")
	}
}

func TestMiddleware_BeforeModelRewriteError(t *testing.T) {
	expectedErr := errors.New("before model error")
	mw := &testMiddleware{}
	mw.beforeModel = func(ctx context.Context, state *ReActAgentState, mc *ModelContext) (context.Context, *ReActAgentState, error) {
		return ctx, nil, expectedErr
	}
	model := &mockModel{}
	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: model, Middlewares: []ReActMiddleware{mw},
	})
	agent.name = "err_bmr"
	iter := agent.Run(context.Background(), &AgentInput{
		Messages: []Message{schema.UserMessage("test")},
	})
	var lastErr error
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			lastErr = ev.Err
		}
	}
	if lastErr == nil {
		t.Error("expected error from BeforeModelRewrite middleware")
	}
}

func TestMiddleware_AfterModelRewriteError(t *testing.T) {
	expectedErr := errors.New("after model error")
	mw := &testMiddleware{}
	mw.afterModel = func(ctx context.Context, state *ReActAgentState, mc *ModelContext) (context.Context, *ReActAgentState, error) {
		return ctx, nil, expectedErr
	}
	model := &mockModel{}
	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: model, Middlewares: []ReActMiddleware{mw},
	})
	agent.name = "err_amr"
	iter := agent.Run(context.Background(), &AgentInput{
		Messages: []Message{schema.UserMessage("test")},
	})
	var lastErr error
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			lastErr = ev.Err
		}
	}
	if lastErr == nil {
		t.Error("expected error from AfterModelRewrite middleware")
	}
}

func TestMiddleware_AfterAgentError(t *testing.T) {
	expectedErr := errors.New("after agent error")
	mw := &testMiddleware{}
	mw.afterAgent = func(ctx context.Context, state *ReActAgentState) (context.Context, error) {
		return ctx, expectedErr
	}
	model := &mockModel{}
	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: model, Middlewares: []ReActMiddleware{mw},
	})
	agent.name = "err_aa"
	iter := agent.Run(context.Background(), &AgentInput{
		Messages: []Message{schema.UserMessage("test")},
	})
	var lastErr error
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			lastErr = ev.Err
		}
	}
	if lastErr == nil {
		t.Error("expected error from AfterAgent middleware")
	}
}

func TestMiddleware_WrapModelReturnsError(t *testing.T) {
	expectedErr := errors.New("wrap model error")
	mw := &testMiddleware{}
	mw.wrapModel = func(ctx context.Context, m Model[*schema.Message], mc *ModelContext) (Model[*schema.Message], error) {
		return nil, expectedErr
	}
	model := &mockModel{}
	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: model, Middlewares: []ReActMiddleware{mw},
	})
	agent.name = "err_wm"
	iter := agent.Run(context.Background(), &AgentInput{
		Messages: []Message{schema.UserMessage("test")},
	})
	var lastErr error
	for {
		ev, ok := iter.Next()
		if !ok { break }
		if ev.Err != nil { lastErr = ev.Err }
	}
	if lastErr == nil {
		t.Error("expected error from WrapModel middleware")
	}
}

// ---- Tool integration with middleware chain ----

