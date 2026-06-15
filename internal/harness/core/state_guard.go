package core

import (
	"context"
	"encoding/json"
	"io"

	"ragflow/internal/harness/core/schema"
)

// typedStateModelWrapper unifies message deep copy, ID injection, cancel checking,
// and event sending into a single wrapper layer for the model call.
//
// This is the central wrapper (typedStateModelWrapper) that sits between
// middlewares and the retry/failover chain, adding:
//   - Message deep copy (prevent pointer-sharing in middleware chain)
//   - Message ID auto-assignment
//   - Cancel context checking before model call
//   - Model output event emission
//   - BeforeModelRewrite / AfterModelRewrite orchestration (via chatmodel.go loop)
type typedStateModelWrapper[M MessageType] struct {
	inner      Model[M]
	cancelCtx  *cancelContext
}

func newTypedStateModelWrapper[M MessageType](inner Model[M], cc *cancelContext) Model[M] {
	return &typedStateModelWrapper[M]{inner: inner, cancelCtx: cc}
}

// copyMessage performs a deep copy of a Message or AgenticMessage to prevent
// pointer-sharing bugs when the same message flows through multiple wrappers.
//
// The Extra map uses JSON marshal/unmarshal for deep copy (same approach as
// checkpoint.deepCopy) so that nested maps/slices are fully independent.
// If JSON round-trip fails for a value, the original reference is kept as
// a fallback to avoid data loss.
func copyMessage[M MessageType](msg M) M {
	switch v := any(msg).(type) {
	case *schema.Message:
		cp := &schema.Message{
			Role:    v.Role,
			Content: v.Content,
			Name:    v.Name,
		}
		if len(v.ToolCalls) > 0 {
			cp.ToolCalls = make([]schema.ToolCall, len(v.ToolCalls))
			copy(cp.ToolCalls, v.ToolCalls)
		}
		if v.Extra != nil {
			cp.Extra = make(map[string]any, len(v.Extra))
			for k, val := range v.Extra {
				cp.Extra[k] = deepCopyAny(val)
			}
		}
		return any(cp).(M)
	case *schema.AgenticMessage:
		cp := &schema.AgenticMessage{
			Role:    v.Role,
			Content: v.Content,
		}
		if len(v.ContentBlocks) > 0 {
			cp.ContentBlocks = make([]schema.ContentBlock, len(v.ContentBlocks))
			copy(cp.ContentBlocks, v.ContentBlocks)
		}
		return any(cp).(M)
	}
	return msg
}

// deepCopyAny performs a deep copy of an arbitrary value via JSON round-trip.
// Falls back to the original value if JSON marshal/unmarshal fails.
func deepCopyAny(v any) any {
	if v == nil {
		return nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return v // fallback: keep original reference
	}
	var result any
	if err := json.Unmarshal(data, &result); err != nil {
		return v // fallback: keep original reference
	}
	return result
}

// preprocessInput performs cancel check, deep copy, and message ID injection.
// Returns nil if cancelled (caller should return ErrStreamCanceled immediately).
func (w *typedStateModelWrapper[M]) preprocessInput(msgs []M) []M {
	if w.cancelCtx != nil && w.cancelCtx.isImmediate() {
		return nil
	}
	copied := make([]M, len(msgs))
	for i, m := range msgs {
		copied[i] = copyMessage(m)
	}
	for _, m := range copied {
		switch v := any(m).(type) {
		case *schema.Message:
			if v.Extra == nil {
				v.Extra = make(map[string]any)
			}
			v.Extra = EnsureMessageID(v.Extra)
		}
	}
	return copied
}

func (w *typedStateModelWrapper[M]) Generate(ctx context.Context, msgs []M, opts ...ModelOption) (M, error) {
	copied := w.preprocessInput(msgs)
	if copied == nil {
		var zero M
		return zero, ErrStreamCanceled
	}
	resp, err := w.inner.Generate(ctx, copied, opts...)
	if err != nil {
		return resp, err
	}
	return copyMessage(resp), nil
}

func (w *typedStateModelWrapper[M]) Stream(ctx context.Context, msgs []M, opts ...ModelOption) (*schema.StreamReader[M], error) {
	// Cancel check before allocating any resources (returns error-embedded StreamReader)
	if w.cancelCtx != nil && w.cancelCtx.isImmediate() {
		r := schema.NewStreamReader[M]()
		var zero M
		r.Send(zero, ErrStreamCanceled)
		r.Close()
		return r, nil
	}

	copied := w.preprocessInput(msgs)
	if copied == nil {
		return nil, ErrStreamCanceled
	}

	s, err := w.inner.Stream(ctx, copied, opts...)
	if err != nil {
		return nil, err
	}

	r := schema.NewStreamReader[M]()
	go func() {
		defer r.Close()
		defer s.Close()
		for {
			if w.cancelCtx != nil && w.cancelCtx.isImmediate() {
				var zero M
				r.Send(zero, ErrStreamCanceled)
				return
			}
			c, e := s.Recv()
			if e == io.EOF {
				break
			}
			if e != nil {
				r.Send(c, e)
				return
			}
			r.Send(copyMessage(c), nil)
		}
	}()
	return r, nil
}

func (w *typedStateModelWrapper[M]) BindTools(tools []*schema.ToolInfo) error {
	return w.inner.BindTools(tools)
}
