package telemetry

import (
	"context"
	"testing"

	"ragflow/internal/harness/core"
	"ragflow/internal/harness/core/schema"
)

func TestNew(t *testing.T) {
	mw := New()
	if mw == nil {
		t.Fatal("expected non-nil middleware")
	}
	if !mw.cfg.EnableTracing {
		t.Error("expected tracing enabled by default")
	}
}

func TestNewWithOptions(t *testing.T) {
	mw := New(WithTracing(false))
	if mw == nil {
		t.Fatal("expected non-nil middleware")
	}
	if mw.cfg.EnableTracing {
		t.Error("expected tracing disabled")
	}
}

func TestMiddlewareImplementsInterface(t *testing.T) {
	mw := New()
	var _ core.ReActMiddleware = mw
	_ = mw
}

func TestWrapModelNoTracer(t *testing.T) {
	mw := New(WithTracing(false))

	generated := false
	model := &mockModel{
		generateFn: func(ctx context.Context, msgs []*schema.Message, opts ...core.ModelOption) (*schema.Message, error) {
			generated = true
			return &schema.Message{Role: "assistant", Content: "ok"}, nil
		},
	}

	wrapped, err := mw.WrapModel(context.Background(), model, &core.ModelContext{})
	if err != nil {
		t.Fatalf("WrapModel failed: %v", err)
	}

	result, err := wrapped.Generate(context.Background(), []*schema.Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if !generated {
		t.Error("expected inner model to be called")
	}
	if result.Content != "ok" {
		t.Errorf("expected 'ok', got '%s'", result.Content)
	}
}

// mockModel is a minimal Model implementation for testing.
type mockModel struct {
	generateFn func(ctx context.Context, msgs []*schema.Message, opts ...core.ModelOption) (*schema.Message, error)
	streamFn   func(ctx context.Context, msgs []*schema.Message, opts ...core.ModelOption) (*schema.StreamReader[*schema.Message], error)
}

func (m *mockModel) Generate(ctx context.Context, msgs []*schema.Message, opts ...core.ModelOption) (*schema.Message, error) {
	if m.generateFn != nil {
		return m.generateFn(ctx, msgs, opts...)
	}
	return &schema.Message{Role: "assistant", Content: "mock"}, nil
}

func (m *mockModel) Stream(ctx context.Context, msgs []*schema.Message, opts ...core.ModelOption) (*schema.StreamReader[*schema.Message], error) {
	if m.streamFn != nil {
		return m.streamFn(ctx, msgs, opts...)
	}
	return schema.NewStreamReader[*schema.Message](), nil
}

func (m *mockModel) BindTools(tools []*schema.ToolInfo) error { return nil }
