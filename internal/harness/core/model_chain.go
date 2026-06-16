package core

import (
	"context"
	"io"

	"ragflow/internal/harness/core/schema"
)

// ---- EventSenderModelWrapper ----

type eventSenderModelWrapper[M MessageType] struct {
	inner   Model[M]
	execCtx *reActExecCtx
}

func wrapModelWithEventSender[M MessageType](inner Model[M], ec *reActExecCtx) Model[M] {
	return &eventSenderModelWrapper[M]{inner: inner, execCtx: ec}
}

func (w *eventSenderModelWrapper[M]) Generate(ctx context.Context, msgs []M, opts ...ModelOption) (M, error) {
	if w.execCtx != nil && w.execCtx.suppressEventSend {
		return w.inner.Generate(ctx, msgs, opts...)
	}
	resp, err := w.inner.Generate(ctx, msgs, opts...)
	if err != nil { return resp, err }
	if w.execCtx != nil && w.execCtx.generator != nil && !isNilMessage(resp) {
		w.execCtx.send(typedModelOutputEvent(resp, nil))
	}
	return resp, nil
}

func (w *eventSenderModelWrapper[M]) Stream(ctx context.Context, msgs []M, opts ...ModelOption) (*schema.StreamReader[M], error) {
	s, err := w.inner.Stream(ctx, msgs, opts...)
	if err != nil { return nil, err }
	if w.execCtx != nil && w.execCtx.suppressEventSend {
		return s, nil
	}
	r := schema.NewStreamReader[M]()
	go func() {
		defer r.Close()
		defer s.Close()
		var chunks []M
		for {
			c, err := s.Recv()
			if err == io.EOF { break }
			if err != nil { r.Send(c, err); return }
			chunks = append(chunks, c)
			r.Send(c, nil)
		}
		if len(chunks) > 0 && w.execCtx != nil {
			if merged, e := mergeChunks(chunks); e == nil {
				w.execCtx.send(typedModelOutputEvent(merged, nil))
			}
		}
	}()
	return r, nil
}

func (w *eventSenderModelWrapper[M]) BindTools(tools []*schema.ToolInfo) error { return w.inner.BindTools(tools) }

// ---- CallbackInjectionModelWrapper (for tracing/monitoring) ----

type callbackModelWrapper[M MessageType] struct {
	inner Model[M]
}

func (w *callbackModelWrapper[M]) Generate(ctx context.Context, msgs []M, opts ...ModelOption) (M, error) {
	msgs = injectMessageID(msgs)
	cbs := getCallbacks(ctx)
	if len(cbs) > 0 {
		input := &AgentCallbackInput{}
		if len(msgs) > 0 {
			switch any(msgs[0]).(type) {
			case *schema.Message:
				msgSlice := make([]Message, len(msgs))
				for i, m := range msgs { msgSlice[i] = any(m).(*schema.Message) }
				input.Input = &AgentInput{Messages: msgSlice}
			}
		}
		for _, cb := range cbs { cb.onStart(ctx, input) }
	}
	resp, err := w.inner.Generate(ctx, msgs, opts...)
	if len(cbs) > 0 {
		if err != nil {
			for _, cb := range cbs {
				if cb.onError != nil { cb.onError(ctx, err) }
			}
		}
		evIter, evGen := NewAsyncIteratorPair[*AgentEvent]()
		if err == nil {
			evGen.Send(&AgentEvent{
				Output: &AgentOutput{MessageOutput: &MessageVariant{Message: any(resp).(*schema.Message)}},
			})
		} else {
			evGen.Send(&AgentEvent{Err: err})
		}
		evGen.Close()
		output := &AgentCallbackOutput{Events: evIter}
		for _, cb := range cbs { cb.onEnd(ctx, output) }
	}
	return resp, err
}
func (w *callbackModelWrapper[M]) Stream(ctx context.Context, msgs []M, opts ...ModelOption) (*schema.StreamReader[M], error) {
	cbs := getCallbacks(ctx)
	if len(cbs) > 0 {
		input := &AgentCallbackInput{}
		if len(msgs) > 0 {
			switch any(msgs[0]).(type) {
			case *schema.Message:
				msgSlice := make([]Message, len(msgs))
				for i, m := range msgs { msgSlice[i] = any(m).(*schema.Message) }
				input.Input = &AgentInput{Messages: msgSlice}
			}
		}
		for _, cb := range cbs { cb.onStart(ctx, input) }
	}
	s, err := w.inner.Stream(ctx, msgs, opts...)
	if err != nil {
		if len(cbs) > 0 {
			for _, cb := range cbs {
				if cb.onError != nil { cb.onError(ctx, err) }
			}
			evIter, evGen := NewAsyncIteratorPair[*AgentEvent]()
			evGen.Send(&AgentEvent{Err: err})
			evGen.Close()
			output := &AgentCallbackOutput{Events: evIter}
			for _, cb := range cbs { cb.onEnd(ctx, output) }
		}
		return nil, err
	}
	// Wrap stream to fire OnEnd on completion
	r := schema.NewStreamReader[M]()
	go func() {
		defer r.Close()
		defer s.Close()
		var allChunks []M
		for {
			c, e := s.Recv()
			if e == io.EOF { break }
			if e != nil { r.Send(c, e); return }
			allChunks = append(allChunks, c)
			r.Send(c, nil)
		}
		if len(cbs) > 0 && len(allChunks) > 0 {
			merged, _ := mergeChunks(allChunks)
			evIter, evGen := NewAsyncIteratorPair[*AgentEvent]()
			evGen.Send(&AgentEvent{
				Output: &AgentOutput{MessageOutput: &MessageVariant{Message: any(merged).(*schema.Message)}},
			})
			evGen.Close()
			output := &AgentCallbackOutput{Events: evIter}
			for _, cb := range cbs { cb.onEnd(ctx, output) }
		}
	}()
	return r, nil
}
func (w *callbackModelWrapper[M]) BindTools(tools []*schema.ToolInfo) error { return w.inner.BindTools(tools) }

// ---- Model Wrapper Chain Builder ----

// BuildModelWrapperChain builds the complete wrapper chain:
//
//	base → failover → retry → eventSender → user wrappers → callback
//
// The chain is built from innermost (closest to model) to outermost.
func BuildModelWrapperChain[M MessageType](base Model[M], ec *reActExecCtx, cfg *ReActConfig[M]) Model[M] {
	model := base

	// 1. Event sender (skip if user middlewares provide their own to avoid duplicates)
	if !HasUserEventSenderModelWrapper(cfg.Middlewares) {
		model = wrapModelWithEventSender(model, ec)
	}

	// 2. Retry (wraps event sender so retries replay the entire inner chain)
	if cfg.RetryConfig != nil {
		model = newTypedRetryModelWrapper(model, cfg.RetryConfig)
	}

	// 3. Failover (wraps retry so each failover attempt gets retry behavior)
	if cfg.FailoverConfig != nil && len(cfg.FailoverConfig.Models) > 0 {
		allModels := append([]Model[M]{base}, cfg.FailoverConfig.Models...)
		model = newFailoverModel(allModels, cfg.FailoverConfig)
	}

	// 4. User middleware wrappers (outermost)
	for _, mw := range cfg.Middlewares {
		if mw == nil { continue }
		mc := &TypedModelContext[M]{
			Tools: toolsToInfosTyped[M](cfg.Tools),
			ModelRetryConfig:    cfg.RetryConfig,
			ModelFailoverConfig: cfg.FailoverConfig,
		}
		wrapped, err := mw.WrapModel(context.Background(), model, mc)
		if err == nil && wrapped != nil { model = wrapped }
	}

	// 5. State wrapper: message deep copy + ID injection + cancel check (guards against middleware side-effects)
	var cancelCtx *cancelContext
	if ec != nil { cancelCtx = ec.cancelCtx }
	model = newTypedStateModelWrapper(model, cancelCtx)

	// 6. Callback injection (outermost — wraps everything)
	model = &callbackModelWrapper[M]{inner: model}

	return model
}

// injectMessageID assigns a unique message ID to each message if not already present.
// Operates on copies to avoid data races when messages are shared across parallel goroutines.
func injectMessageID[M MessageType](msgs []M) []M {
	for i, msg := range msgs {
		switch v := any(msg).(type) {
		case *schema.Message:
			if v.Extra != nil && GetMessageID(v.Extra) != "" {
				continue // already has ID, skip
			}
			// Deep-copy so concurrent access is safe for shared messages.
			cp := copyMessage(msg)
			copied := any(cp).(*schema.Message)
			copied.Extra = EnsureMessageID(copied.Extra)
			if c2, ok := any(copied).(M); ok {
				msgs[i] = c2
			}
		}
	}
	return msgs
}
