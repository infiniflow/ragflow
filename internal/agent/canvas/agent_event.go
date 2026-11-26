package canvas

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
)

type agentEventSinkKey struct{}
type agentEventRunKey struct{}
type agentEventParentKey struct{}
type agentNodeKey struct {
	id         string
	state      *CanvasState
	invocation string
}

func agentNodeIdentity(ctx context.Context, state *CanvasState, nodeID string) agentNodeKey {
	if canonical, _ := runtime.GetStateFromContext(ctx); canonical != nil {
		state = canonical
	}
	return agentNodeKey{id: nodeID, state: state}
}

// WithAgentEventSink enables execution events for Agent runs. Ingestion does not
// install a sink. The sink must return promptly; persistence is best effort.
func WithAgentEventSink(ctx context.Context, sink func(common.AgentEvent)) context.Context {
	return context.WithValue(ctx, agentEventSinkKey{}, sink)
}

type agentNodeExecution struct {
	event   common.AgentEvent
	started time.Time
}

type agentEventRun struct {
	mu       sync.Mutex
	sink     func(common.AgentEvent)
	base     common.AgentEvent
	sequence uint64
	started  time.Time
	active   map[agentNodeKey]*agentNodeExecution
	parents  map[string]string
	names    map[string]string
	closed   bool
}

func agentEvents(ctx context.Context) *agentEventRun {
	run, _ := ctx.Value(agentEventRunKey{}).(*agentEventRun)
	return run
}

func startAgentEvents(ctx context.Context, agentID, sessionID, messageID string, root map[string]any) (context.Context, *agentEventRun) {
	sink, _ := ctx.Value(agentEventSinkKey{}).(func(common.AgentEvent))
	if sink == nil {
		return ctx, nil
	}
	version, _ := root["version_id"].(string)
	run := &agentEventRun{sink: sink, base: common.AgentEvent{AgentID: agentID, SessionID: sessionID, MessageID: messageID, RunID: common.GenerateToken(), AgentVersion: version}, started: time.Now(), active: make(map[agentNodeKey]*agentNodeExecution), parents: make(map[string]string), names: make(map[string]string)}
	eventType := "run_started"
	if id, _ := root["__resume_interrupt_id__"].(string); id != "" {
		eventType = "run_resumed"
	}
	inputs, truncated := common.AgentEventSnapshot(root)
	run.emit(common.AgentEvent{EventType: eventType, Status: "running", InputsJSON: inputs, PayloadTruncated: truncated})
	ctx = context.WithValue(ctx, agentEventRunKey{}, run)
	return runtime.WithAgentNodeObserver(ctx, run.observeNode), run
}

// emit is called under mu after the run has started; the first event is emitted
// before the context is published. Serializing sink calls preserves sequence.
func (run *agentEventRun) emit(event common.AgentEvent) {
	run.sequence++
	event.EventID = common.GenerateToken()
	event.EventAt = time.Now().UTC()
	event.Sequence = run.sequence
	event.AgentID = run.base.AgentID
	event.AgentVersion = run.base.AgentVersion
	event.SessionID = run.base.SessionID
	event.MessageID = run.base.MessageID
	event.RunID = run.base.RunID
	// Observability must not change a workflow's result if its sink fails.
	func() { defer func() { _ = recover() }(); run.sink(event) }()
}

func setAgentNodeParent(ctx context.Context, nodeID, parentID string) {
	if run := agentEvents(ctx); run != nil {
		run.mu.Lock()
		defer run.mu.Unlock()
		if _, exists := run.parents[nodeID]; !exists {
			run.parents[nodeID] = parentID
		}
	}
}

func startAgentNode(ctx context.Context, state *CanvasState, nodeID, name, kind string, inputs map[string]any) {
	if run := agentEvents(ctx); run != nil {
		run.startNode(ctx, agentNodeIdentity(ctx, state, nodeID), name, kind, 1, inputs)
	}
}

func (run *agentEventRun) startNode(ctx context.Context, key agentNodeKey, name, kind string, attempt uint32, inputs any) string {
	snapshot, truncated := common.AgentEventSnapshot(inputs)
	run.mu.Lock()
	defer run.mu.Unlock()
	if run.closed {
		return ""
	}
	if label := run.names[key.id]; label != "" {
		name = label
	}
	event := common.AgentEvent{EventType: "node_started", NodeID: key.id, NodeName: name, NodeType: kind, ExecutionID: common.GenerateToken(), Attempt: attempt, Status: "running", InputsJSON: snapshot, PayloadTruncated: truncated}
	event.ParentExecutionID, _ = ctx.Value(agentEventParentKey{}).(string)
	if parent := run.active[agentNodeIdentity(ctx, key.state, run.parents[key.id])]; parent != nil {
		event.ParentExecutionID = parent.event.ExecutionID
	}
	run.active[key] = &agentNodeExecution{event: event, started: time.Now()}
	run.emit(event)
	return event.ExecutionID
}

func (run *agentEventRun) observeNode(ctx context.Context, nodeID, name, kind string, attempt uint32, inputs any) (context.Context, func(any, error)) {
	key := agentNodeIdentity(ctx, nil, nodeID)
	key.invocation = common.GenerateToken()
	executionID := run.startNode(ctx, key, name, kind, attempt, inputs)
	ctx = context.WithValue(ctx, agentEventParentKey{}, executionID)
	return ctx, func(outputs any, err error) { run.mu.Lock(); defer run.mu.Unlock(); run.finishNode(key, outputs, err) }
}

func agentEventStatus(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case IsInterruptError(err), len(ExtractInterruptContexts(err)) > 0:
		return "paused"
	case err != nil:
		return "failed"
	default:
		return "completed"
	}
}

func (run *agentEventRun) finishNode(key agentNodeKey, outputs any, err error) {
	execution := run.active[key]
	if execution == nil {
		return
	}
	delete(run.active, key)
	event := execution.event
	event.Status = agentEventStatus(err)
	event.EventType = "node_" + event.Status
	event.DurationMS = time.Since(execution.started).Milliseconds()
	event.OutputsJSON, event.PayloadTruncated = common.AgentEventSnapshot(outputs)
	event.PayloadTruncated = event.PayloadTruncated || execution.event.PayloadTruncated
	setAgentEventError(&event, err)
	run.emit(event)
}

func setAgentEventError(event *common.AgentEvent, err error) {
	if err == nil {
		return
	}
	event.ErrorCode = event.Status
	// Public error classification already hides internal storage details.
	message := runErrorEvent(err).Message
	if len(message) > 4096 {
		message = strings.ToValidUTF8(message[:4096], "")
		event.PayloadTruncated = true
	}
	message = common.RedactAgentEventText(message)
	if len(message) > 4096 {
		message = strings.ToValidUTF8(message[:4096], "")
		event.PayloadTruncated = true
	}
	event.ErrorMessage = message
}

func finishAgentNode(ctx context.Context, state *CanvasState, nodeID string, err error, current ...map[string]any) {
	run := agentEvents(ctx)
	if run == nil {
		return
	}
	var outputs map[string]any
	if ctxState, _ := runtime.GetStateFromContext(ctx); ctxState != nil {
		outputs = ctxState.Snapshot()[nodeID]
	}
	if len(outputs) == 0 && state != nil {
		outputs = state.Snapshot()[nodeID]
	}
	if len(current) > 0 {
		outputs = current[0]
		if err == nil {
			if message, _ := outputs["_ERROR"].(string); message != "" {
				err = errors.New(message)
			}
		}
	}
	run.mu.Lock()
	defer run.mu.Unlock()
	run.finishNode(agentNodeIdentity(ctx, state, nodeID), outputs, err)
}

func (run *agentEventRun) finish(state *CanvasState, err error) {
	if run == nil {
		return
	}
	run.mu.Lock()
	defer run.mu.Unlock()
	if run.closed {
		return
	}
	run.closed = true
	ids := make([]agentNodeKey, 0, len(run.active))
	for id := range run.active {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(a, b agentNodeKey) int { return run.active[a].started.Compare(run.active[b].started) })
	for _, id := range ids {
		var outputs map[string]any
		if id.state != nil {
			outputs = id.state.Snapshot()[id.id]
		} else if state != nil {
			outputs = state.Snapshot()[id.id]
		}
		run.finishNode(id, outputs, err)
	}
	event := common.AgentEvent{Status: agentEventStatus(err), DurationMS: time.Since(run.started).Milliseconds()}
	event.EventType = "run_" + event.Status
	if state != nil {
		event.OutputsJSON, event.PayloadTruncated = common.AgentEventSnapshot(state.Snapshot())
	}
	setAgentEventError(&event, err)
	run.emit(event)
}

// Each Parallel item receives a distinct state and inherits the execution of
// its enclosing macro before that state is cloned.
func withAgentNodeParent(ctx context.Context, nodeID string) context.Context {
	run := agentEvents(ctx)
	if run == nil {
		return ctx
	}
	run.mu.Lock()
	defer run.mu.Unlock()
	if parent := run.active[agentNodeIdentity(ctx, nil, nodeID)]; parent != nil {
		return context.WithValue(ctx, agentEventParentKey{}, parent.event.ExecutionID)
	}
	return ctx
}

func registerAgentNodes(ctx context.Context, c *Canvas) {
	run := agentEvents(ctx)
	if run == nil {
		return
	}
	run.mu.Lock()
	defer run.mu.Unlock()
	for id, node := range c.Components {
		run.names[id] = node.DisplayName
	}
	for id, parent := range c.NodeParents {
		run.parents[id] = parent
	}
}
