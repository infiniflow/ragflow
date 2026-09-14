package core

import (
	"context"
	"errors"
	"fmt"

	"ragflow/internal/harness/core/schema"
)

// TypedRunner is the primary entry point for agent execution.
type TypedRunner[M MessageType] struct {
	a               TypedAgent[M]
	enableStreaming bool
	store           CheckPointStore
}

type Runner = TypedRunner[*schema.Message]

type RunnerConfig[M MessageType] struct {
	Agent           TypedAgent[M]
	EnableStreaming bool
	CheckPointStore CheckPointStore
}

type ResumeParams struct{ Targets map[string]any }

func NewRunner(ctx context.Context, conf RunnerConfig[*schema.Message]) *Runner {
	return NewTypedRunner[*schema.Message](conf)
}

func NewTypedRunner[M MessageType](conf RunnerConfig[M]) *TypedRunner[M] {
	return &TypedRunner[M]{a: conf.Agent, enableStreaming: conf.EnableStreaming, store: conf.CheckPointStore}
}

func (r *TypedRunner[M]) Run(ctx context.Context, msgs []M, opts ...RunOption) *AsyncIterator[*TypedAgentEvent[M]] {
	return runImpl(r.a, r.enableStreaming, r.store, ctx, msgs, opts...)
}

func (r *TypedRunner[M]) Query(ctx context.Context, query string, opts ...RunOption) *AsyncIterator[*TypedAgentEvent[M]] {
	msgs, err := newUserMsg[M](query)
	if err != nil {
		return errorIter[M](err)
	}
	return r.Run(ctx, []M{msgs}, opts...)
}

func (r *TypedRunner[M]) Resume(ctx context.Context, cid string, opts ...RunOption) (*AsyncIterator[*TypedAgentEvent[M]], error) {
	return resumeInternal(r.a, r.store, ctx, cid, nil, opts...)
}

func (r *TypedRunner[M]) ResumeWithParams(ctx context.Context, cid string, params *ResumeParams, opts ...RunOption) (*AsyncIterator[*TypedAgentEvent[M]], error) {
	return resumeInternal(r.a, r.store, ctx, cid, params.Targets, opts...)
}

// ---- Internal ----

func errorIter[M MessageType](err error) *AsyncIterator[*TypedAgentEvent[M]] {
	it, gen := NewAsyncIteratorPair[*TypedAgentEvent[M]]()
	gen.Send(&TypedAgentEvent[M]{Err: err})
	gen.Close()
	return it
}

func newUserMsg[M MessageType](query string) (M, error) {
	var zero M
	switch any(zero).(type) {
	case *schema.Message:
		return any(schema.UserMessage(query)).(M), nil
	case *schema.AgenticMessage:
		return any(schema.UserAgenticMessage(query)).(M), nil
	default:
		return zero, fmt.Errorf("unsupported message type %T", zero)
	}
}

func runImpl[M MessageType](a TypedAgent[M], streaming bool, store CheckPointStore, ctx context.Context, msgs []M, opts ...RunOption) *AsyncIterator[*TypedAgentEvent[M]] {
	o := getCommonOptions(nil, opts...)
	input := &TypedAgentInput[M]{Messages: msgs, EnableStreaming: streaming}

	var zero M
	if _, ok := any(zero).(*schema.Message); ok {
		ca, ok := any(a).(Agent)
		if !ok || ca == nil {
			return errorIter[M](fmt.Errorf("agent does not implement Agent interface"))
		}
		fa := toFlowAgent(ctx, ca)
		if store != nil {
			fa.checkPointStore = store
		}
		ci, ok := any(input).(*AgentInput)
		if !ok {
			return errorIter[M](fmt.Errorf("input type assertion failed: expected *AgentInput, got %T", input))
		}
		ctx = setupRunContext(ctx, input, o)
		return wrapIterForStore(streaming, store, ctx, any(fa.Run(ctx, ci, opts...)).(*AsyncIterator[*TypedAgentEvent[M]]), o)
	}

	tfa := toTypedFlowAgent(a)
	if store != nil {
		tfa.checkPointStore = store
	}
	ctx = setupRunContext(ctx, input, o)
	return wrapIterForStore(streaming, store, ctx, tfa.Run(ctx, input, opts...), o)
}

func resumeInternal[M MessageType](a TypedAgent[M], store CheckPointStore, ctx context.Context, cid string, data map[string]any, opts ...RunOption) (*AsyncIterator[*TypedAgentEvent[M]], error) {
	if store == nil {
		return nil, fmt.Errorf("resume requires a checkpoint store")
	}
	ctx, rc, info, err := loadCheckpoint(store, ctx, cid)
	if err != nil {
		return nil, err
	}
	streaming := info.EnableStreaming
	o := getCommonOptions(nil, opts...)
	if o.sharedParentSession {
		if ps := getSession(ctx); ps != nil {
			rc.Session.Values = ps.Values
		}
	}
	if rc.Session.Values == nil {
		rc.Session.Values = make(map[string]any)
	}
	ctx = setRunCtx(ctx, rc)
	AddSessionValues(ctx, o.sessionValues)

	var zero M
	if _, ok := any(zero).(*schema.Message); ok {
		ca, _ := any(a).(Agent)
		fa := toFlowAgent(ctx, ca)
		ra, ok := Agent(fa).(ResumableAgent)
		if !ok {
			return nil, fmt.Errorf("agent %T does not support resume", a)
		}
		return newIterForStore(streaming, store, ctx, any(ra.Resume(ctx, info, opts...)).(*AsyncIterator[*TypedAgentEvent[M]]), &cid, o.cancelCtx), nil
	}

	tfa := toTypedFlowAgent(a)
	ra, ok := TypedAgent[M](tfa).(TypedResumableAgent[M])
	if !ok {
		return nil, fmt.Errorf("agent %T does not support resume", a)
	}
	return newIterForStore(streaming, store, ctx, ra.Resume(ctx, info, opts...), &cid, o.cancelCtx), nil
}

// setupRunContext initializes the run context and applies session values for a new Run.
func setupRunContext[M MessageType](ctx context.Context, input *TypedAgentInput[M], o *runOptions) context.Context {
	ctx = ctxWithNewTypedRunCtx(ctx, input, o.sharedParentSession)
	AddSessionValues(ctx, o.sessionValues)
	return ctx
}

// wrapIterForStore conditionally wraps an event iterator with handleIter when a checkpoint
// store or cancel context is active. Returns the original iterator unchanged otherwise.
func wrapIterForStore[M MessageType](streaming bool, store CheckPointStore, ctx context.Context, iter *AsyncIterator[*TypedAgentEvent[M]], o *runOptions) *AsyncIterator[*TypedAgentEvent[M]] {
	if store == nil && o.cancelCtx == nil {
		return iter
	}
	return newIterForStore(streaming, store, ctx, iter, o.checkPointID, o.cancelCtx)
}

// newIterForStore creates a new iterator pair backed by handleIter for checkpoint store
// and cancel handling.
func newIterForStore[M MessageType](streaming bool, store CheckPointStore, ctx context.Context, iter *AsyncIterator[*TypedAgentEvent[M]], cid *string, cc *cancelContext) *AsyncIterator[*TypedAgentEvent[M]] {
	nit, gen := NewAsyncIteratorPair[*TypedAgentEvent[M]]()
	go handleIter(streaming, store, ctx, iter, gen, cid, cc)
	return nit
}

func handleIter[M MessageType](streaming bool, store CheckPointStore, ctx context.Context, ai *AsyncIterator[*TypedAgentEvent[M]], gen *AsyncGenerator[*TypedAgentEvent[M]], cid *string, cc *cancelContext) {
	defer func() {
		if r := recover(); r != nil {
			gen.Send(&TypedAgentEvent[M]{Err: fmt.Errorf("panic: %v", r)})
		}
		gen.Close()
	}()
	var sig *InterruptSignal
	for {
		ev, ok := ai.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			var ce *CancelError
			if errors.As(ev.Err, &ce) {
				if cc != nil && cc.isRoot() && cc.shouldCancel() {
					cc.markHandled()
				}
				if ce.interruptSignal != nil && cid != nil {
					ce.InterruptContexts = nil
					saveCheckpoint(store, ctx, *cid, streaming, &InterruptInfo{}, ce.interruptSignal)
				}
				gen.Send(ev)
				break
			}
		}
		if ev.Action != nil && ev.Action.internalInterrupted != nil {
			if sig != nil {
				panic("multiple interrupt actions")
			}
			sig = ev.Action.internalInterrupted
			ev = &TypedAgentEvent[M]{
				AgentName: ev.AgentName, RunPath: ev.RunPath, Output: ev.Output,
				Action: &AgentAction{Interrupted: &InterruptInfo{Data: ev.Action.Interrupted.Data}, internalInterrupted: sig},
			}
			if cid != nil {
				saveCheckpoint(store, ctx, *cid, streaming, &InterruptInfo{Data: ev.Action.Interrupted.Data}, sig)
			}
		}
		gen.Send(ev)
	}
}

// ResumeWithData creates a ResumeInfo with custom resume data.
// Use this to pass ReActAgentResumeData (e.g., HistoryModifier)
// when resuming an interrupted agent.
func ResumeWithData(data any) *ResumeInfo {
	return &ResumeInfo{ResumeData: data}
}
