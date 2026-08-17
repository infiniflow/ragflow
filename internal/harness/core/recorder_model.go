package core

import (
	"context"
	"time"

	"ragflow/internal/harness/core/schema"
	"ragflow/internal/harness/events"
)

// ---- Model Wrapper: records LLM calls via EventRecorder from context ----

// eventRecorderModelWrapper wraps a Model and records each invocation to the
// EventRecorder found in the context (via events.RecorderFromContext).
type eventRecorderModelWrapper[M MessageType] struct {
	inner Model[M]
}

func wrapModelWithEventRecorder[M MessageType](inner Model[M]) Model[M] {
	return &eventRecorderModelWrapper[M]{inner: inner}
}

func (w *eventRecorderModelWrapper[M]) Generate(ctx context.Context, msgs []M, opts ...ModelOption) (M, error) {
	start := time.Now()
	resp, err := w.inner.Generate(ctx, msgs, opts...)
	durMs := time.Since(start).Milliseconds()
	rec := events.RecorderFromContext(ctx)
	if rec != nil && err == nil {
		var msgsAny []any
		for _, m := range msgs {
			msgsAny = append(msgsAny, any(m))
		}
		// We record the model as "unknown" when the name isn't accessible here.
		// The agent sets model name via BindTools / config; that info can be
		// added by providing it through the context in a future iteration.
		rec.RecordModelCall(ctx, "unknown", "", msgsAny, contentOf(resp), events.TokenUsage{}, durMs, 0)
	}
	return resp, err
}

func (w *eventRecorderModelWrapper[M]) Stream(ctx context.Context, msgs []M, opts ...ModelOption) (*schema.StreamReader[M], error) {
	return w.inner.Stream(ctx, msgs, opts...)
}

func (w *eventRecorderModelWrapper[M]) BindTools(tools []*schema.ToolInfo) error {
	return w.inner.BindTools(tools)
}

// ---- Handler that injects the wrapper via TypedReActMiddleware.WrapModel ----

type eventRecorderModelHandler[M MessageType] struct{}

// NewEventRecorderModelWrapper creates a middleware handler that wraps the model
// to record LLM invocations to the EventRecorder stored in context.
// Usage:
//
//	recorder := events.NewEventRecorder(store)
//	ctx := events.ContextWithRecorder(ctx, recorder)
//	cfg := &ReActConfig[*schema.Message]{
//	    Model: model,
//	    Handlers: []TypedReActMiddleware[*schema.Message]{
//	        NewEventRecorderModelWrapper[*schema.Message](),
//	    },
//	}
func NewEventRecorderModelWrapper[M MessageType]() *eventRecorderModelHandler[M] {
	return &eventRecorderModelHandler[M]{}
}

func (h *eventRecorderModelHandler[M]) WrapModel(ctx context.Context, m Model[M], mc *TypedModelContext[M]) (Model[M], error) {
	rec := events.RecorderFromContext(ctx)
	if rec == nil {
		return m, nil // no recorder in context — pass through
	}
	return wrapModelWithEventRecorder(m), nil
}

func (h *eventRecorderModelHandler[M]) BeforeAgent(ctx context.Context, rc *ReActAgentContext) (context.Context, *ReActAgentContext, error) {
	return ctx, rc, nil
}
func (h *eventRecorderModelHandler[M]) AfterAgent(ctx context.Context, state *TypedReActAgentState[M]) (context.Context, error) {
	return ctx, nil
}
func (h *eventRecorderModelHandler[M]) BeforeModelRewrite(ctx context.Context, st *TypedReActAgentState[M], mc *TypedModelContext[M]) (context.Context, *TypedReActAgentState[M], error) {
	return ctx, st, nil
}
func (h *eventRecorderModelHandler[M]) AfterModelRewrite(ctx context.Context, st *TypedReActAgentState[M], mc *TypedModelContext[M]) (context.Context, *TypedReActAgentState[M], error) {
	return ctx, st, nil
}

// contentOf extracts the text content from a response message.
func contentOf[M MessageType](resp M) string {
	if msg, ok := any(resp).(*schema.Message); ok && msg != nil {
		return msg.Content
	}
	if am, ok := any(resp).(*schema.AgenticMessage); ok && am != nil {
		return am.Content
	}
	return ""
}
