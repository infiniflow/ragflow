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
	// builtinHistory is set when historyRewriter is the built-in per-entry
	// rewriter. Only then can rewritten messages be appended incrementally.
	builtinHistory  bool
	historyName     string
	checkPointStore CheckPointStore
}

func (a *flowAgent) deepCopy() *flowAgent {
	cp := &flowAgent{Agent: a.Agent, parentAgent: a.parentAgent,
		disallowTransferToParent: a.disallowTransferToParent, historyRewriter: a.historyRewriter,
		builtinHistory: a.builtinHistory, historyName: a.historyName,
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
		fa.useDefaultHistoryRewriter(agent.Name(ctx))
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
	return func(fa *flowAgent) {
		fa.historyRewriter = h
		fa.builtinHistory = false
		fa.historyName = ""
	}
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
		fa.useDefaultHistoryRewriter(agent.Name(ctx))
	}
	return fa
}

func (a *flowAgent) useDefaultHistoryRewriter(name string) {
	a.historyRewriter = defaultHistoryRewriter(name)
	a.builtinHistory = true
	a.historyName = name
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
		msgs, _ := defaultRewriteEntries(name, entries)
		return msgs, nil
	}
}

// defaultRewriteEntries rewrites entries with the built-in per-entry rule.
// The second result reports whether the message was freshly rewritten; the
// incremental cache uses it to hand out independent copies on every transfer,
// matching the allocation behavior of a full replay.
func defaultRewriteEntries(name string, entries []*HistoryEntry) ([]Message, []bool) {
	msgs := make([]Message, 0, len(entries))
	fresh := make([]bool, 0, len(entries))
	for _, e := range entries {
		m := e.Message
		rewritten := false
		if !e.IsUserInput && e.AgentName != name {
			m = rewriteMsg(m, e.AgentName)
			rewritten = true
		}
		if m != nil {
			msgs = append(msgs, m)
			fresh = append(fresh, rewritten)
		}
	}
	return msgs, fresh
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

// historyCache incrementally reconstructs the history entries for one variant
// (with or without skipped transfer messages) of a run.
type historyCache struct {
	sessionEpoch   uint64
	branchMode     bool
	entriesVersion uint64
	lastEventIndex int
	entries        []*HistoryEntry
	rewritten      map[*flowAgent]*rewrittenHistory
	// scannedEvents counts events examined by genInput. Tests use it to verify
	// that transfers only process newly appended events.
	scannedEvents int
}

// rewrittenHistory caches the built-in rewriter output for a single agent.
type rewrittenHistory struct {
	entriesVersion uint64
	lastEntryIndex int
	msgs           []Message
	fresh          []bool
}

func (rc *runContext) historyCacheFor(skipTransfer bool) *historyCache {
	idx := 0
	if skipTransfer {
		idx = 1
	}
	if rc.history[idx] == nil {
		rc.history[idx] = &historyCache{}
	}
	return rc.history[idx]
}

// syncHistoryEntriesLocked brings the cached entries up to date with the
// session event log and returns them. The caller must hold rc.historyMu.
//
// Events only append in the common (non-branch) case, so the cache extends
// from lastEventIndex instead of replaying the whole log. Any state mutation
// that can reorder or truncate the log bumps the session epoch and forces a
// rebuild.
func (rc *runContext) syncHistoryEntriesLocked(root *AgentInput, skipTransfer bool) []*HistoryEntry {
	c := rc.historyCacheFor(skipTransfer)
	events, total, epoch, branchMode := rc.Session.eventSnapshotSince(c.lastEventIndex)

	rebuild := c.entries == nil ||
		c.sessionEpoch != epoch ||
		c.branchMode != branchMode ||
		c.lastEventIndex > total
	if branchMode {
		// getEvents merges and sorts in-flight lane events, so the cached
		// prefix is not necessarily a prefix of the next snapshot.
		rebuild = true
	}
	if rebuild {
		if !branchMode && c.lastEventIndex != 0 {
			// The log changed underneath the cached prefix; rewind and rebuild
			// from the beginning.
			events, total, epoch, branchMode = rc.Session.eventSnapshotSince(0)
		}
		c.entries = make([]*HistoryEntry, 0, len(root.Messages)+len(events))
		for _, m := range root.Messages {
			c.entries = append(c.entries, &HistoryEntry{IsUserInput: true, Message: m})
		}
		c.lastEventIndex = 0
		c.entriesVersion++
		c.rewritten = nil
	}
	c.sessionEpoch = epoch
	c.branchMode = branchMode

	for _, ev := range events {
		c.scannedEvents++
		ae, ok := ev.(*AgentEvent)
		if !ok {
			continue
		}
		if skipTransfer && ae.Action != nil && ae.Action.TransferToAgent != nil {
			if ae.Output != nil && ae.Output.MessageOutput != nil && ae.Output.MessageOutput.Role == schema.RoleTool && len(c.entries) > 0 {
				c.entries = c.entries[:len(c.entries)-1]
				c.entriesVersion++
			}
			continue
		}
		msg := msgFromEvent(ae)
		if msg == nil {
			continue
		}
		c.entries = append(c.entries, &HistoryEntry{AgentName: ae.AgentName, Message: msg})
	}
	c.lastEventIndex = total
	return c.entries
}

// syncRewrittenLocked returns the built-in rewriter output for a. Existing
// messages are reused; only entries appended since the agent last ran are
// rewritten. Freshly rewritten messages are copied on return so a caller
// cannot mutate the cache, matching full-replay allocation behavior.
func (rc *runContext) syncRewrittenLocked(a *flowAgent, entries []*HistoryEntry, skipTransfer bool) []Message {
	c := rc.historyCacheFor(skipTransfer)
	if c.rewritten == nil {
		c.rewritten = make(map[*flowAgent]*rewrittenHistory)
	}
	r := c.rewritten[a]
	if r == nil || r.entriesVersion != c.entriesVersion || r.lastEntryIndex > len(entries) {
		r = &rewrittenHistory{entriesVersion: c.entriesVersion}
		c.rewritten[a] = r
	}
	if r.lastEntryIndex < len(entries) {
		msgs, fresh := defaultRewriteEntries(a.historyName, entries[r.lastEntryIndex:])
		r.msgs = append(r.msgs, msgs...)
		r.fresh = append(r.fresh, fresh...)
		r.lastEntryIndex = len(entries)
	}
	out := make([]Message, len(r.msgs))
	for i, m := range r.msgs {
		if !r.fresh[i] || m == nil {
			out[i] = m
			continue
		}
		cp := *m
		if cp.Extra != nil {
			cp.Extra = copyMap(cp.Extra)
		}
		out[i] = &cp
	}
	return out
}

func (a *flowAgent) genInput(ctx context.Context, rc *runContext, skipTransfer bool) (*AgentInput, error) {
	root := rc.RootInput.(*AgentInput)

	rc.historyMu.Lock()
	entries := rc.syncHistoryEntriesLocked(root, skipTransfer)
	if a.builtinHistory {
		msgs := rc.syncRewrittenLocked(a, entries, skipTransfer)
		rc.historyMu.Unlock()
		return &AgentInput{Messages: msgs, EnableStreaming: root.EnableStreaming}, nil
	}
	// A custom rewriter sees fresh HistoryEntry values, as it did before the
	// cache existed, so it cannot corrupt the cached entries.
	snapshot := make([]*HistoryEntry, len(entries))
	for i, e := range entries {
		cp := *e
		snapshot[i] = &cp
	}
	rc.historyMu.Unlock()

	msgs, err := a.historyRewriter(ctx, snapshot)
	if err != nil {
		return nil, err
	}
	return &AgentInput{Messages: msgs, EnableStreaming: root.EnableStreaming}, nil
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
