package core

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"

	"ragflow/internal/harness/core/schema"
)

type workflowMode int

const (
	workflowModeUnknown workflowMode = iota
	workflowModeSequential
	workflowModeLoop
	workflowModeParallel
)

type workflowState struct {
	InterruptIdx int
}
type workflowParallelState struct {
	SubEvents map[int][]*agentEventWrap
}
type workflowLoopState struct {
	Iter int
	Idx  int
}

type agentEventWrap struct{ Event any }

type WorkflowInterruptInfo struct {
	OrigInput      *AgentInput
	SequentialIdx  int
	SequentialInfo *InterruptInfo
	LoopIter       int
	ParallelInfo   map[int]*InterruptInfo
}

type workflowAgent struct {
	name      string
	desc      string
	subAgents []*flowAgent
	mode      workflowMode
	maxIter   int
}

func (a *workflowAgent) Name(_ context.Context) string        { return a.name }
func (a *workflowAgent) Description(_ context.Context) string { return a.desc }
func (a *workflowAgent) GetType() string {
	switch a.mode {
	case workflowModeSequential:
		return "Sequential"
	case workflowModeParallel:
		return "Parallel"
	case workflowModeLoop:
		return "Loop"
	default:
		return "WorkflowAgent"
	}
}

func (a *workflowAgent) Run(ctx context.Context, _ *AgentInput, opts ...RunOption) *AsyncIterator[*AgentEvent] {
	it, gen := NewAsyncIteratorPair[*AgentEvent]()
	cc := getCommonOptions(nil, opts...).cancelCtx
	ctx = withCancelContext(ctx, cc)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				gen.Send(&AgentEvent{Err: fmt.Errorf("panic: %v\n%s", r, debug.Stack())})
			}
			gen.Close()
		}()
		switch a.mode {
		case workflowModeSequential:
			a.runSeq(ctx, gen, nil, nil, opts...)
		case workflowModeParallel:
			a.runPar(ctx, gen, nil, nil, opts...)
		case workflowModeLoop:
			a.runLoop(ctx, gen, nil, nil, opts...)
		default:
			gen.Send(&AgentEvent{Err: fmt.Errorf("unsupported mode %d", a.mode)})
		}
	}()
	return it
}

func (a *workflowAgent) Resume(ctx context.Context, info *ResumeInfo, opts ...RunOption) *AsyncIterator[*AgentEvent] {
	it, gen := NewAsyncIteratorPair[*AgentEvent]()
	cc := getCommonOptions(nil, opts...).cancelCtx
	ctx = withCancelContext(ctx, cc)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				gen.Send(&AgentEvent{Err: fmt.Errorf("panic: %v\n%s", r, debug.Stack())})
			}
			gen.Close()
		}()
		st := info.InterruptState
		if st == nil {
			gen.Send(&AgentEvent{Err: fmt.Errorf("no state for resume")})
			return
		}
		switch s := st.(type) {
		case *workflowState:
			a.runSeq(ctx, gen, s, info, opts...)
		case *workflowParallelState:
			a.runPar(ctx, gen, s, info, opts...)
		case *workflowLoopState:
			a.runLoop(ctx, gen, s, info, opts...)
		default:
			gen.Send(&AgentEvent{Err: fmt.Errorf("unknown state %T", s)})
		}
	}()
	return it
}

// ---- Sequential ----

func (a *workflowAgent) runSeq(ctx context.Context, gen *AsyncGenerator[*AgentEvent], st *workflowState, info *ResumeInfo, opts ...RunOption) error {
	start := 0
	wfCtx := ctx
	if st != nil {
		start = st.InterruptIdx
		wfCtx = buildPath(ctx, a.subAgents, start, 0)
	}

	for i := start; i < len(a.subAgents); i++ {
		sa := a.subAgents[i]
		if cc := getCancelContext(ctx); cc != nil && cc.shouldCancel() {
			if cerr, ok := cc.createAndMarkHandled(); ok {
				gen.Send(&AgentEvent{Err: cerr})
				return nil
			}
			// createAndMarkHandled returned ok=false — a sibling
			// wrapIterWithCancelCtx has already marked the context stDone.
			// Emit the CancelError directly so the consumer sees the signal
			// rather than a transition event with no Err field.
			gen.Send(&AgentEvent{Err: cc.createError()})
			return nil
		}
		var si *AsyncIterator[*AgentEvent]
		if st != nil {
			if wfInfo, _ := info.Data.(*WorkflowInterruptInfo); wfInfo != nil && wfInfo.SequentialInfo != nil {
				si = sa.Resume(wfCtx, &ResumeInfo{EnableStreaming: info.EnableStreaming, InterruptInfo: wfInfo.SequentialInfo}, opts...)
			} else {
				si = sa.Run(wfCtx, nil, opts...)
			}
			st = nil
		} else {
			si = sa.Run(wfCtx, nil, opts...)
		}

		wfCtx = updateRunPathOnly(wfCtx, sa.Name(wfCtx))
		last := drainEvents(si, gen)
		if cc := getCancelContext(ctx); cc != nil && cc.shouldCancel() {
			// If a sibling wrapIterWithCancelCtx already transitioned the
			// cancel context to stDone (via markDone), createAndMarkHandled
			// returns ok=false. In that case, still surface the CancelError
			// so the test consumer sees the cancellation signal — the cancel
			// already happened, we just need to deliver it.
			if cerr, ok := cc.createAndMarkHandled(); ok {
				gen.Send(&AgentEvent{Err: cerr})
				return nil
			}
			gen.Send(&AgentEvent{Err: cc.createError()})
			return nil
		}
		if last != nil {
			if last.Err != nil {
				gen.Send(last)
				return nil
			}
			if last.Action.internalInterrupted != nil {
				s := &workflowState{InterruptIdx: i}
				ev := CompositeInterrupt(ctx, "Seq interrupted", s, last.Action.internalInterrupted)
				ev.Action.Interrupted.Data = &WorkflowInterruptInfo{OrigInput: inputFromCtx(ctx), SequentialIdx: i, SequentialInfo: last.Action.Interrupted}
				ev.AgentName, ev.RunPath = last.AgentName, last.RunPath
				gen.Send(ev)
				return nil
			}
			if last.Action.Exit {
				gen.Send(last)
				return nil
			}
			gen.Send(last)
		}
	}
	return nil
}

// ---- Loop ----

func (a *workflowAgent) runLoop(ctx context.Context, gen *AsyncGenerator[*AgentEvent], ls *workflowLoopState, info *ResumeInfo, opts ...RunOption) error {
	if len(a.subAgents) == 0 {
		return nil
	}
	startIter, startIdx := 0, 0
	wfCtx := ctx
	if ls != nil {
		startIter, startIdx = ls.Iter, ls.Idx
		wfCtx = buildPath(ctx, a.subAgents, startIdx, startIter)
	}

	for i := startIter; i < a.maxIter || a.maxIter == 0; i++ {
		for j := startIdx; j < len(a.subAgents); j++ {
			sa := a.subAgents[j]
			if cc := getCancelContext(ctx); cc != nil && cc.shouldCancel() {
				if cerr, ok := cc.createAndMarkHandled(); ok {
					gen.Send(&AgentEvent{Err: cerr})
					return nil
				}
				// createAndMarkHandled returned ok=false — see runSeq above.
				gen.Send(&AgentEvent{Err: cc.createError()})
				return nil
			}
			var si *AsyncIterator[*AgentEvent]
			if ls != nil {
				if wfInfo, _ := info.Data.(*WorkflowInterruptInfo); wfInfo != nil && wfInfo.SequentialInfo != nil {
					si = sa.Resume(wfCtx, &ResumeInfo{EnableStreaming: info.EnableStreaming, InterruptInfo: wfInfo.SequentialInfo}, opts...)
				} else {
					si = sa.Run(wfCtx, nil, opts...)
				}
				ls = nil
			} else {
				si = sa.Run(wfCtx, nil, opts...)
			}

			wfCtx = updateRunPathOnly(wfCtx, sa.Name(wfCtx))
			var breakEv *AgentEvent
			_ = breakEv
			last := drainEvents(si, gen)
			if cc := getCancelContext(ctx); cc != nil && cc.shouldCancel() {
				// If a sibling wrapIterWithCancelCtx already transitioned the
				// cancel context to stDone, createAndMarkHandled returns ok=false.
				// Still surface a CancelError so the consumer observes the signal.
				if cerr, ok := cc.createAndMarkHandled(); ok {
					gen.Send(&AgentEvent{Err: cerr})
					return nil
				}
				gen.Send(&AgentEvent{Err: cc.createError()})
				return nil
			}
			if last != nil {
				if last.Err != nil {
					gen.Send(last)
					return nil
				}
				if last.Action.BreakLoop != nil && !last.Action.BreakLoop.Done {
					last.Action.BreakLoop.Done = true
					last.Action.BreakLoop.CurrentIterations = i
					gen.Send(last)
					return nil
				}
				if last.Action.internalInterrupted != nil {
					s := &workflowLoopState{Iter: i, Idx: j}
					ev := CompositeInterrupt(ctx, "Loop interrupted", s, last.Action.internalInterrupted)
					ev.Action.Interrupted.Data = &WorkflowInterruptInfo{OrigInput: inputFromCtx(ctx), LoopIter: i, SequentialIdx: j, SequentialInfo: last.Action.Interrupted}
					ev.AgentName, ev.RunPath = last.AgentName, last.RunPath
					gen.Send(ev)
					return nil
				}
				if last.Action.Exit {
					gen.Send(last)
					return nil
				}
				gen.Send(last)
			}
		}
		startIdx = 0
	}
	return nil
}

// ---- Parallel ----

func (a *workflowAgent) runPar(ctx context.Context, gen *AsyncGenerator[*AgentEvent], ps *workflowParallelState, info *ResumeInfo, opts ...RunOption) error {
	if len(a.subAgents) == 0 {
		return nil
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var signals []*InterruptSignal
	dataMap := make(map[int]*InterruptInfo)
	var names map[string]bool

	if ps != nil {
		n, err := getNextResumeAgents(ctx, info)
		if err != nil {
			return err
		}
		names = n
	}
	childCtxs := make([]context.Context, len(a.subAgents))
	for i := range a.subAgents {
		childCtxs[i] = forkRunCtx(ctx)
		if ps != nil && ps.SubEvents != nil {
			if evts, ok := ps.SubEvents[i]; ok {
				if rc := getRunCtx(childCtxs[i]); rc != nil && rc.Session != nil {
					for _, e := range evts {
						rc.Session.addEvent(e)
					}
				}
			}
		}
	}
	if cc := getCancelContext(ctx); cc != nil && cc.shouldCancel() {
		if cerr, ok := cc.createAndMarkHandled(); ok {
			gen.Send(&AgentEvent{Err: cerr})
			return nil
		}
		// createAndMarkHandled returned ok=false — see runSeq above.
		gen.Send(&AgentEvent{Err: cc.createError()})
		return nil
	}
	for i := range a.subAgents {
		wg.Add(1)
		go func(idx int, ag *flowAgent) {
			defer wg.Done()
			var it *AsyncIterator[*AgentEvent]
			if names != nil {
				if _, ok := names[ag.Name(ctx)]; ok {
					ri := &ResumeInfo{EnableStreaming: info.EnableStreaming}
					if wf, _ := info.Data.(*WorkflowInterruptInfo); wf != nil {
						ri.InterruptInfo = wf.ParallelInfo[idx]
					}
					it = ag.Resume(childCtxs[idx], ri, opts...)
				} else if ps != nil {
					return
				} else {
					it = ag.Run(childCtxs[idx], nil, opts...)
				}
			} else {
				it = ag.Run(childCtxs[idx], nil, opts...)
			}

			for {
				ev, ok := it.Next()
				if !ok {
					break
				}
				if ev.Action != nil && ev.Action.internalInterrupted != nil {
					mu.Lock()
					signals = append(signals, ev.Action.internalInterrupted)
					dataMap[idx] = ev.Action.Interrupted
					mu.Unlock()
					break
				}
				gen.Send(ev)
			}
		}(i, a.subAgents[i])
	}
	wg.Wait()
	if cc := getCancelContext(ctx); cc != nil && cc.shouldCancel() {
		if cerr, ok := cc.createAndMarkHandled(); ok {
			gen.Send(&AgentEvent{Err: cerr})
			return nil
		}
		gen.Send(&AgentEvent{Err: cc.createError()})
		return nil
	}
	if len(signals) > 0 {
		subEvts := make(map[int][]*agentEventWrap)
		for i, cc := range childCtxs {
			if rc := getRunCtx(cc); rc != nil && rc.Session != nil {
				var ws []*agentEventWrap
				for _, e := range rc.Session.getEvents() {
					ws = append(ws, &agentEventWrap{Event: e})
				}
				subEvts[i] = ws
			}
		}
		st := &workflowParallelState{SubEvents: subEvts}
		ev := CompositeInterrupt(ctx, "Parallel interrupted", st, signals...)
		ev.Action.Interrupted.Data = &WorkflowInterruptInfo{OrigInput: inputFromCtx(ctx), ParallelInfo: dataMap}
		ev.AgentName = a.Name(ctx)
		ev.RunPath = getRunCtx(ctx).getRunPath()
		gen.Send(ev)
	}
	return nil
}

// ---- Helpers ----

func buildPath(ctx context.Context, subs []*flowAgent, idx, iter int) context.Context {
	var steps []string
	for k := 0; k < iter; k++ {
		for _, s := range subs {
			steps = append(steps, s.Name(ctx))
		}
	}
	for k := 0; k < idx; k++ {
		steps = append(steps, subs[k].Name(ctx))
	}
	return updateRunPathOnly(ctx, steps...)
}

func drainEvents(ai *AsyncIterator[*AgentEvent], gen *AsyncGenerator[*AgentEvent]) *AgentEvent {
	var last *AgentEvent
	for {
		ev, ok := ai.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			// Return error event instead of sending it to gen — caller handles propagation.
			return ev
		}
		if ev.Action != nil {
			last = ev
			continue
		}
		gen.Send(ev)
	}
	return last
}

func inputFromCtx(ctx context.Context) *AgentInput {
	if rc := getRunCtx(ctx); rc != nil {
		if in, ok := rc.RootInput.(*AgentInput); ok {
			return in
		}
	}
	return nil
}

// ---- Constructors ----

type SequentialConfig struct {
	Name, Description string
	SubAgents         []Agent
}
type ParallelConfig struct {
	Name, Description string
	SubAgents         []Agent
}
type LoopConfig struct {
	Name, Description string
	SubAgents         []Agent
	MaxIterations     int
}

func newWf(ctx context.Context, name, desc string, subs []Agent, mode workflowMode, maxIter int) (*flowAgent, error) {
	wa := &workflowAgent{name: name, desc: desc, mode: mode, maxIter: maxIter}
	fas := make([]Agent, len(subs))
	for i, s := range subs {
		fas[i] = toFlowAgent(ctx, s, WithDisallowTransferToParent())
	}
	fa, err := SetSubAgents(ctx, wa, fas)
	if err != nil {
		return nil, err
	}
	// Set sub-agents directly on the workflowAgent so its Run() has access
	wa.subAgents = make([]*flowAgent, len(fas))
	for i, s := range fas {
		wa.subAgents[i] = toFlowAgent(ctx, s, WithDisallowTransferToParent())
	}
	return fa.(*flowAgent), nil
}

func NewSequential(ctx context.Context, cfg *SequentialConfig) (ResumableAgent, error) {
	if cfg == nil {
		return nil, fmt.Errorf("SequentialConfig is nil")
	}
	return newWf(ctx, cfg.Name, cfg.Description, cfg.SubAgents, workflowModeSequential, 0)
}
func NewParallel(ctx context.Context, cfg *ParallelConfig) (ResumableAgent, error) {
	if cfg == nil {
		return nil, fmt.Errorf("ParallelConfig is nil")
	}
	return newWf(ctx, cfg.Name, cfg.Description, cfg.SubAgents, workflowModeParallel, 0)
}
func NewLoop(ctx context.Context, cfg *LoopConfig) (ResumableAgent, error) {
	if cfg == nil {
		return nil, fmt.Errorf("LoopConfig is nil")
	}
	if cfg.MaxIterations <= 0 {
		cfg.MaxIterations = 10
	}
	return newWf(ctx, cfg.Name, cfg.Description, cfg.SubAgents, workflowModeLoop, cfg.MaxIterations)
}

func init() {
	schema.RegisterType("_harness_wf_interrupt_info", func() any { return &WorkflowInterruptInfo{} })
	schema.RegisterType("_harness_wf_state", func() any { return &workflowState{} })
	schema.RegisterType("_harness_wf_parallel_state", func() any { return &workflowParallelState{} })
	schema.RegisterType("_harness_wf_loop_state", func() any { return &workflowLoopState{} })
}
