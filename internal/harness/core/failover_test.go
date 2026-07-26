package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"ragflow/internal/harness/core/schema"
)

func TestWithModelFailover_SingleModel(t *testing.T) {
	model := &mockModel{}
	wrapped := WithModelFailover(model)
	// WithModelFailover always wraps in failoverModel, even with single model
	if wrapped == nil {
		t.Fatal("nil wrapped model")
	}
	// Verify it still works (delegates to underlying model)
	model.addResp("ok")
	ctx := context.Background()
	msgs := []Message{schema.UserMessage("hi")}
	resp, err := wrapped.Generate(ctx, msgs)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Content != "ok" {
		t.Errorf("content = %s, want ok", resp.Content)
	}
}

func TestWithModelFailover_PrimarySucceeds(t *testing.T) {
	primary := &mockModel{}
	primary.addResp("from primary")

	fallback := &mockModel{}
	fallback.addResp("from fallback")

	wrapped := WithModelFailover(primary, fallback)
	ctx := context.Background()
	msgs := []Message{schema.UserMessage("hi")}

	resp, err := wrapped.Generate(ctx, msgs)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Content != "from primary" {
		t.Errorf("content = %s, want from primary", resp.Content)
	}
}

func TestWithModelFailover_FallsBack(t *testing.T) {
	primary := &failOnceModel{}
	fallback := &mockModel{}
	fallback.addResp("fallback result")

	wrapped := WithModelFailover(primary, fallback)
	ctx := context.Background()
	msgs := []Message{schema.UserMessage("failover test")}

	resp, err := wrapped.Generate(ctx, msgs)
	if err != nil {
		t.Fatalf("Generate after failover: %v", err)
	}
	if resp.Content != "fallback result" {
		t.Errorf("content = %s, want fallback result", resp.Content)
	}
}

func TestWithModelFailover_AllFail(t *testing.T) {
	primary := &alwaysFailModel{}
	secondary := &alwaysFailModel{}

	wrapped := WithModelFailover(primary, secondary)
	ctx := context.Background()
	_, err := wrapped.Generate(ctx, []Message{schema.UserMessage("")})
	if err == nil {
		t.Error("expected error when all models fail")
	}
}

type failOnceModel struct {
	failed bool
}

func (m *failOnceModel) Generate(_ context.Context, _ []Message, _ ...modelOption) (Message, error) {
	if !m.failed {
		m.failed = true
		return nil, errors.New("primary failure")
	}
	return &schema.Message{Content: "recovery"}, nil
}
func (m *failOnceModel) Stream(ctx context.Context, msgs []Message, opts ...modelOption) (*schema.StreamReader[Message], error) {
	msg, err := m.Generate(ctx, msgs, opts...)
	return schema.StreamReaderFromArray([]Message{msg}), err
}
func (m *failOnceModel) BindTools(tools []*schema.ToolInfo) error { return nil }

func TestFailover_WithShouldFailoverCallback(t *testing.T) {
	primary := &mockModel{}
	primary.addResp("ok")
	secondary := &mockModel{}
	secondary.addResp("fallback")

	cfg := &FailoverConfig[*schema.Message]{
		Models: []Model[*schema.Message]{secondary},
		ShouldFailover: func(ctx context.Context, err error) bool {
			return false // Skip failover
		},
	}
	model := newFailoverModel([]Model[*schema.Message]{primary, secondary}, cfg)
	resp, err := model.Generate(context.Background(), []*schema.Message{{Content: "hi"}})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Content != "ok" {
		t.Errorf("expected 'ok', got %q", resp.Content)
	}
}

func TestFailover_ShouldFailoverSkipsSecondary(t *testing.T) {
	primary := &mockModel{shouldFail: true}

	secondary := &mockModel{}
	secondary.addResp("fallback")

	cfg := &FailoverConfig[*schema.Message]{
		Models: []Model[*schema.Message]{secondary},
		ShouldFailover: func(ctx context.Context, err error) bool {
			return false // Skip failover
		},
	}
	model := newFailoverModel([]Model[*schema.Message]{primary, secondary}, cfg)

	// Primary fails, shouldFailover returns false, so we expect an error, not fallback
	_, err := model.Generate(context.Background(), []*schema.Message{{Content: "hi"}})
	if err == nil {
		t.Error("expected error since ShouldFailover returns false")
	}
	if !strings.Contains(err.Error(), "failover skipped") {
		t.Errorf("expected 'failover skipped' error, got: %v", err)
	}
	_ = fmt.Sprintf("%v", err)
}
