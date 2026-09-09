package core

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"strings"

	"ragflow/internal/harness/core/schema"
)

// HistoryEntry represents a message in conversation history.
type HistoryEntry struct {
	IsUserInput bool
	AgentName   string
	Message     Message
}

// HistoryRewriter transforms conversation history during agent transfers.
type HistoryRewriter func(ctx context.Context, entries []*HistoryEntry) ([]Message, error)

// flowAgent wraps an Agent with orchestration (sub-agents, history, transfer, callbacks).
//
// TODO: flowAgent and workflowAgent share sub-agent management. workflowAgent
// creates sub-agents AND injects them into flowAgent via SetSubAgents(),
// causing double bookkeeping (workflowAgent.subAgents + flowAgent.subAgents).
// Consider a single source of truth for sub-agent ownership.
type flowAgent struct {
	Agent
	subAgents                []*flowAgent
	parentAgent              *flowAgent
	disallowTransferToParent bool
	historyRewriter          HistoryRewriter
	checkPointStore          CheckPointStore
}

func (a *flowAgent) deepCopy() *flowAgent {
	cp := &flowAgent{Agent: a.Agent, parentAgent: a.parentAgent,
		disallowTransferToParent: a.disallowTransferToParent, historyRewriter: a.historyRewriter,
		checkPointStore: a.checkPointStore}
	for _, sa := range a.subAgents {
		cp.subAgents = append(cp.subAgents, sa.deepCopy())
	}
	return cp
}

func SetSubAgents(ctx context.Context, agent Agent, subs []Agent) (ResumableAgent, error) {
	var fa *flowAgent
	var ok bool
	if fa, ok = agent.(*flowAgent); !ok {
		fa = &flowAgent{Agent: agent}
	}
	if fa.historyRewriter == nil {
		fa.historyRewriter = defaultHistoryRewriter(agent.Name(ctx))
	}
	if len(fa.subAgents) > 0 {
		return nil, errors.New("sub-agents already set")
	}
	for _, s := range subs {
		fa.subAgents = append(fa.subAgents, toFlowAgent(ctx, s, WithDisallowTransferToParent()))
	}
	return fa, nil
}

func AgentWithOptions(ctx context.Context, agent Agent, opts ...AgentOption) Agent {
	return toFlowAgent(ctx, agent, opts...)
}

type AgentOption func(*flowAgent)

func WithDisallowTransferToParent() AgentOption {
	return func(fa *flowAgent) { fa.disallowTransferToParent = true }
}
func WithHistoryRewriter(h HistoryRewriter) AgentOption {
	return func(fa *flowAgent) { fa.historyRewriter = h }
}

func toFlowAgent(ctx context.Context, agent Agent, opts ...AgentOption) *flowAgent {
	var fa *flowAgent
	var ok bool
	if fa, ok = agent.(*flowAgent); !ok {
		fa = &flowAgent{Agent: agent}
	} else {
		fa = fa.deepCopy()
	}
	for _, o := range opts {
		o(fa)
	}
	if fa.historyRewriter == nil {
		fa.historyRewriter = defaultHistoryRewriter(agent.Name(ctx))
	}
	return fa
}

func (a *flowAgent) getAgent(ctx context.Context, name string) *flowAgent {
	for _, sa := range a.subAgents {
		if sa.Name(ctx) == name {
			return sa
		}
	}
	if a.parentAgent != nil && a.parentAgent.Name(ctx) == name {
		return a.parentAgent
	}
	return nil
}

func defaultHistoryRewriter(name string) HistoryRewriter {
	return func(ctx context.Context, entries []*HistoryEntry) ([]Message, error) {
		msgs := make([]Message, 0, len(entries))
		for _, e := range entries {
			m := e.Message
			if !e.IsUserInput && e.AgentName != name {
				m = rewriteMsg(m, e.AgentName)
			}
			if m != nil {
				msgs = append(msgs, m)
			}
		}
		return msgs, nil
	}
}

func rewriteMsg(msg Message, agentName string) Message {
	if msg.Role == schema.RoleAssistant && msg.Content == "" && len(msg.ToolCalls) == 0 {
		return nil
	}
	if msg.Role == schema.RoleTool && msg.Content == "" && msg.ToolName == "" {
		return nil
	}
	var sb strings.Builder
	sb.WriteString("For context:")
	if msg.Role == schema.RoleAssistant {
		if msg.Content != "" {
			sb.WriteString(fmt.Sprintf(" [%s] said: %s.", agentName, msg.Content))
		}
		for _, tc := range msg.ToolCalls {
			sb.WriteString(fmt.Sprintf(" [%s] called tool `%s` args: %s.", agentName, tc.Function.Name, tc.Function.Arguments))
		}
	} else if msg.Role == schema.RoleTool && msg.Content != "" {
		sb.WriteString(fmt.Sprintf(" [%s] `%s` returned: %s.", agentName, msg.ToolName, msg.Content))
	}
	r := schema.UserMessage(sb.String())
	if msg.Extra != nil {
		r.Extra = copyMap(msg.Extra)
	}
	return r
}

func deepCopyInput(ai *AgentInput) *AgentInput {
	return &AgentInput{Messages: append([]Message(nil), ai.Messages...), EnableStreaming: ai.EnableStreaming}
}

// TODO: On every transfer, genInput replays ALL historical events to rebuild
// conversation history. This is O(n) per transfer, where n grows with conversation
// length. Consider caching the reconstructed history per agent and invalidating
// on state changes rather than re-scanning the full event list.
func (a *flowAgent) genInput(ctx context.Context, rc *runContext, skipTransfer bool) (*AgentInput, error) {
	input := deepCopyInput(rc.RootInput.(*AgentInput))
	entries := make([]*HistoryEntry, 0)
	for _, m := range input.Messages {
		entries = append(entries, &HistoryEntry{IsUserInput: true, Message: m})
	}
	for _, ev := range rc.Session.getEvents() {
		ae, ok := ev.(*AgentEvent)
		if !ok {
			continue
		}
		if skipTransfer && ae.Action != nil && ae.Action.TransferToAgent != nil {
			if ae.Output != nil && ae.Output.MessageOutput != nil && ae.Output.MessageOutput.Role == schema.RoleTool && len(entries) > 0 {
				entries = entries[:len(entries)-1]
			}
			continue
		}
		msg := msgFromEvent(ae)
		if msg == nil {
			continue
		}
		entries = append(entries, &HistoryEntry{AgentName: ae.AgentName, Message: msg})
	}
	msgs, err := a.historyRewriter(ctx, entries)
	if err != nil {
		return nil, err
	}
	input.Messages = msgs
	return input, nil
}

func msgFromEvent(ev *AgentEvent) Message {
	if ev == nil || ev.Output == nil || ev.Output.MessageOutput == nil {
		return nil
	}
	mv := ev.Output.MessageOutput
	if mv.IsStreaming {
		return nil
	}
	return mv.Message
}

func (a *flowAgent) Run(ctx context.Context, input *AgentInput, opts ...RunOption) *AsyncIterator[*AgentEvent] {
	name := a.Name(ctx)
	ctx, rc := initRunCtx(ctx, name, input)
	ctx = AppendAddressSegment(ctx, AddressSegmentAgent, name)
	o := getCommonOptions(nil, opts...)
	cc := o.cancelCtx

	pi, err := a.genInput(ctx, rc, o.skipTransferMessages)
	if err != nil {
		if cc != nil {
			cc.markDone()
		}
		_ = &AgentCallbackInput{Input: input}
		return wrapIterEnd(ctx, errorIterMsg(err))
	}
	ctx = initAgentCallbacks(ctx, name, getAgentType(a.Agent), filterOptions(name, opts)...)

	cancelCtx := withCancelContext(ctx, cc)
	ai := a.Agent.Run(cancelCtx, pi, filterOptions(name, opts)...)
	it, gen := NewAsyncIteratorPair[*AgentEvent]()
	go a.runLoop(cancelCtx, cancelCtx, rc, ai, gen, filterCancelOption(opts)...)
	return wrapIterWithCancelCtx(it, cc)
}

func (a *flowAgent) runLoop(ctx, subCtx context.Context, rc *runContext, ai *AsyncIterator[*AgentEvent], gen *AsyncGenerator[*AgentEvent], opts ...RunOption) {
	defer func() {
		if r := recover(); r != nil {
			gen.Send(&AgentEvent{Err: fmt.Errorf("panic: %v\n%s", r, debug.Stack())})
		}
		gen.Close()
	}()
	var lastAction *AgentAction
	for {
		ev, ok := ai.Next()
		if !ok {
			break
		}
		curRunPath := rc.getRunPath()
		if len(ev.RunPath) == 0 {
			ev.AgentName = a.Name(ctx)
			ev.RunPath = curRunPath
		}
		if (ev.Action == nil || ev.Action.Interrupted == nil) && pathMatch(curRunPath, ev.RunPath) {
			cp := copyTypedAgentEvent(ev)
			setAutomaticClose(cp)
			setAutomaticClose(ev)
			rc.Session.addEvent(cp)
		}
		if pathMatch(curRunPath, ev.RunPath) {
			lastAction = ev.Action
		}
		cp := copyTypedAgentEvent(ev)
		setAutomaticClose(cp)
		setAutomaticClose(ev)
		gen.Send(cp)
	}
	var dest string
	if lastAction != nil {
		if lastAction.Interrupted != nil || lastAction.Exit {
			return
		}
		if lastAction.TransferToAgent != nil {
			dest = lastAction.TransferToAgent.DestAgentName
		}
	}
	if dest != "" {
		if cc := getCancelContext(subCtx); cc != nil && cc.shouldCancel() {
			return
		}
		next := a.getAgent(subCtx, dest)
		if next == nil {
			gen.Send(&AgentEvent{Err: fmt.Errorf("transfer: agent '%s' not found from '%s'", dest, a.Name(subCtx))})
			return
		}
		for {
			se, ok := next.Run(subCtx, nil, opts...).Next()
			if !ok {
				break
			}
			setAutomaticClose(se)
			if se.Action == nil || se.Action.Interrupted == nil {
				rc.Session.addEvent(copyTypedAgentEvent(se))
			}
			gen.Send(se)
		}
	}
}

func (a *flowAgent) Resume(ctx context.Context, info *ResumeInfo, opts ...RunOption) *AsyncIterator[*AgentEvent] {
	name := a.Name(ctx)
	ctx, info = buildResumeInfo(ctx, name, info)
	o := getCommonOptions(nil, opts...)
	cc := o.cancelCtx
	ctx = initAgentCallbacks(ctx, name, getAgentType(a.Agent), filterOptions(name, opts)...)

	if info.WasInterrupted {
		if ra, ok := a.Agent.(ResumableAgent); ok {
			ai := ra.Resume(withCancelContext(ctx, cc), info, opts...)
			it, gen := NewAsyncIteratorPair[*AgentEvent]()
			go a.runLoop(withCancelContext(ctx, cc), withCancelContext(ctx, cc), getRunCtx(ctx), ai, gen, filterCancelOption(opts)...)
			return wrapIterWithCancelCtx(it, cc)
		}
		if cc != nil {
			cc.markDone()
		}
		return wrapIterEnd(ctx, errorIterMsg(fmt.Errorf("agent '%s' not ResumableAgent", name)))
	}
	next, err := getNextResumeAgent(ctx, info)
	if err != nil {
		if cc != nil {
			cc.markDone()
		}
		return wrapIterEnd(ctx, errorIterMsg(err))
	}
	sa := a.getAgent(ctx, next)
	if sa == nil {
		if len(a.subAgents) == 0 {
			if ra, ok := a.Agent.(ResumableAgent); ok {
				inner := ra.Resume(withCancelContext(ctx, cc), info, filterCancelOption(opts)...)
				return wrapIterWithCancelCtx(wrapIterEnd(ctx, inner), cc)
			}
			return wrapIterEnd(ctx, errorIterMsg(fmt.Errorf("agent '%s' has no sub-agents and not ResumableAgent", name)))
		}
		if cc != nil {
			cc.markDone()
		}
		return wrapIterEnd(ctx, errorIterMsg(fmt.Errorf("sub-agent '%s' not found", next)))
	}
	inner := sa.Resume(withCancelContext(ctx, cc), info, filterCancelOption(opts)...)
	return wrapIterWithCancelCtx(wrapIterEnd(ctx, inner), cc)
}

func pathMatch(a, b []RunStep) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].Equals(b[i]) {
			return false
		}
	}
	return true
}

func wrapIterEnd(ctx context.Context, iter *AsyncIterator[*AgentEvent]) *AsyncIterator[*AgentEvent] {
	it, gen := NewAsyncIteratorPair[*AgentEvent]()
	go func() {
		defer gen.Close()
		for {
			ev, ok := iter.Next()
			if !ok {
				break
			}
			if !gen.SendCtx(ctx, ev) {
				return
			}
		}
	}()
	return it
}

func errorIterMsg(err error) *AsyncIterator[*AgentEvent] {
	it, gen := NewAsyncIteratorPair[*AgentEvent]()
	gen.Send(&AgentEvent{Err: err})
	gen.Close()
	return it
}

// ---- Typed flow agent (AgenticMessage path) ----

type typedFlowAgent[M MessageType] struct {
	TypedAgent[M]
	checkPointStore CheckPointStore
}

func toTypedFlowAgent[M MessageType](a TypedAgent[M]) *typedFlowAgent[M] {
	if fa, ok := a.(*typedFlowAgent[M]); ok {
		return fa
	}
	return &typedFlowAgent[M]{TypedAgent: a}
}

func (a *typedFlowAgent[M]) Run(ctx context.Context, input *TypedAgentInput[M], opts ...RunOption) *AsyncIterator[*TypedAgentEvent[M]] {
	name := a.Name(ctx)
	ctx, rc := initTypedRunCtx(ctx, name, input)
	ctx = AppendAddressSegment(ctx, AddressSegmentAgent, name)
	o := getCommonOptions(nil, opts...)
	cc := o.cancelCtx
	ctx = initAgenticCallbacks(ctx, name, "", filterOptions(name, opts)...)
	ai := a.TypedAgent.Run(withCancelContext(ctx, cc), input, filterOptions(name, opts)...)
	it, gen := NewAsyncIteratorPair[*TypedAgentEvent[M]]()
	go a.runLoop(withCancelContext(ctx, cc), rc, ai, gen)
	return wrapIterWithCancelCtx(it, cc)
}

// runLoop for typedFlowAgent drains events only. Unlike flowAgent.runLoop,
// it does NOT handle TransferToAgent actions or route to sub-agents. This is
// a design choice: the typed agent path currently does not support agent-to-agent
// transfers. If transfer support is needed, add sub-agent routing logic here.
func (a *typedFlowAgent[M]) runLoop(ctx context.Context, rc *runContext, ai *AsyncIterator[*TypedAgentEvent[M]], gen *AsyncGenerator[*TypedAgentEvent[M]]) {
	defer func() {
		if r := recover(); r != nil {
			gen.Send(&TypedAgentEvent[M]{Err: fmt.Errorf("panic: %v\n%s", r, debug.Stack())})
		}
		gen.Close()
	}()
	for {
		ev, ok := ai.Next()
		if !ok {
			break
		}
		curRunPath := rc.getRunPath()
		if len(ev.RunPath) == 0 {
			ev.AgentName = a.Name(ctx)
			ev.RunPath = curRunPath
		}
		if (ev.Action == nil || ev.Action.Interrupted == nil) && pathMatch(curRunPath, ev.RunPath) {
			cp := copyTypedAgentEvent(ev)
			typedSetAutomaticClose(cp)
			typedSetAutomaticClose(ev)
			addTypedEvent(rc.Session, cp)
		}
		gen.Send(ev)
	}
}

func (a *typedFlowAgent[M]) Resume(ctx context.Context, info *ResumeInfo, opts ...RunOption) *AsyncIterator[*TypedAgentEvent[M]] {
	name := a.Name(ctx)
	ctx, info = buildResumeInfo(ctx, name, info)
	o := getCommonOptions(nil, opts...)
	cc := o.cancelCtx
	if info.WasInterrupted {
		if ra, ok := a.TypedAgent.(TypedResumableAgent[M]); ok {
			ai := ra.Resume(withCancelContext(ctx, cc), info, opts...)
			it, gen := NewAsyncIteratorPair[*TypedAgentEvent[M]]()
			go a.runLoop(withCancelContext(ctx, cc), getRunCtx(ctx), ai, gen)
			return wrapIterWithCancelCtx(it, cc)
		}
		if cc != nil {
			cc.markDone()
		}
		return typedErrorIterEnd[M](ctx, fmt.Errorf("agent '%s' not ResumableAgent", name))
	}
	if ra, ok := a.TypedAgent.(TypedResumableAgent[M]); ok {
		inner := ra.Resume(withCancelContext(ctx, cc), info, filterCancelOption(opts)...)
		return wrapIterWithCancelCtx(typedWrapIterEnd(ctx, inner), cc)
	}
	return typedErrorIterEnd[M](ctx, fmt.Errorf("agent '%s' not ResumableAgent", name))
}

func initTypedRunCtx[M MessageType](ctx context.Context, name string, input *TypedAgentInput[M]) (context.Context, *runContext) {
	rc := getRunCtx(ctx)
	if rc == nil {
		rc = &runContext{RootInput: input, RunPath: make([]RunStep, 0), Session: newRunSession()}
		ctx = context.WithValue(ctx, runContextKey{}, rc)
	}
	rc.appendRunPath(RunStep{agentName: name})
	return ctx, rc
}

func typedWrapIterEnd[M MessageType](ctx context.Context, iter *AsyncIterator[*TypedAgentEvent[M]]) *AsyncIterator[*TypedAgentEvent[M]] {
	it, gen := NewAsyncIteratorPair[*TypedAgentEvent[M]]()
	go func() {
		defer gen.Close()
		for {
			ev, ok := iter.Next()
			if !ok {
				break
			}
			if !gen.SendCtx(ctx, ev) {
				return
			}
		}
	}()
	return it
}
func typedErrorIterEnd[M MessageType](ctx context.Context, err error) *AsyncIterator[*TypedAgentEvent[M]] {
	return errorIter[M](err)
}
