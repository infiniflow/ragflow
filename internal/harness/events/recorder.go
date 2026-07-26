package events

import (
	"context"
	"encoding/json"

	"ragflow/internal/harness/graph/pregel"
)

// ---- Context helpers for passing EventRecorder through context ----

type recorderContextKey struct{}

// ContextWithRecorder stores an EventRecorder in context for use by
// model wrappers and tool middlewares in the agent core.
func ContextWithRecorder(ctx context.Context, r *EventRecorder) context.Context {
	return context.WithValue(ctx, recorderContextKey{}, r)
}

// RecorderFromContext retrieves an EventRecorder from context.
// Returns nil when no recorder is present.
func RecorderFromContext(ctx context.Context) *EventRecorder {
	r, _ := ctx.Value(recorderContextKey{}).(*EventRecorder)
	return r
}

// RecorderOption configures an EventRecorder.
type RecorderOption func(*recorderOptions)

type recorderOptions struct {
	traceID  string
	threadID string
}

// WithTraceID sets the trace ID for the recorder.
func WithTraceID(traceID string) RecorderOption {
	return func(o *recorderOptions) {
		o.traceID = traceID
	}
}

// WithThreadID sets the thread ID for the recorder.
func WithThreadID(threadID string) RecorderOption {
	return func(o *recorderOptions) {
		o.threadID = threadID
	}
}

// EventRecorder records graph execution events as append-only Events.
// It implements pregel.GraphCallback and can be added to a CallbackManager.
// Additionally, RecordModelCall / RecordToolCall / etc. provide fine-grained
// event recording for LLM invocations and tool executions.
//
// Usage:
//
//	store := events.NewMemoryEventStore()
//	recorder := events.NewEventRecorder(store, events.WithTraceID("trace-001"))
//	cb := pregel.NewCallbackManager()
//	cb.AddCallback(recorder)
type EventRecorder struct {
	store    EventLog
	clock    *LogicalClock
	traceID  string
	threadID string
}

// NewEventRecorder creates a new EventRecorder.
func NewEventRecorder(store EventLog, opts ...RecorderOption) *EventRecorder {
	o := &recorderOptions{}
	for _, opt := range opts {
		opt(o)
	}
	return &EventRecorder{
		store:    store,
		clock:    NewLogicalClock(),
		traceID:  o.traceID,
		threadID: o.threadID,
	}
}

// record creates and appends an event.
func (r *EventRecorder) record(ctx context.Context, typ EventType, opts ...func(*Event)) {
	ev := NewEvent(typ, r.clock.Tick())
	ev.TraceID = r.traceID
	ev.ThreadID = r.threadID
	for _, fn := range opts {
		fn(ev)
	}
	ev.Seal()
	_ = r.store.Append(ctx, ev)
}

// ---- Context-based recording (used by model/tool wrappers) ----

// RecordModelCall records an LLM model invocation with its result.
func (r *EventRecorder) RecordModelCall(ctx context.Context, model, provider string, messages []any, content string, tokens TokenUsage, durationMs int64, cost float64) {
	r.record(ctx, EventLLMCallStart, func(ev *Event) {
		ev.Deterministic = false
		ev.Metadata["model"] = model
		ev.Metadata["provider"] = provider
	})
	r.record(ctx, EventLLMCallEnd, func(ev *Event) {
		ev.Deterministic = false
		pl := LLMCallPayload{
			Model:      model,
			Provider:   provider,
			Messages:   messages,
			Tokens:     tokens,
			Content:    content,
			DurationMs: durationMs,
			Cost:       cost,
		}
		ev.Payload, _ = json.Marshal(pl)
	})
}

// RecordLLMChunk records a single streaming chunk from an LLM call.
func (r *EventRecorder) RecordLLMChunk(ctx context.Context, model string, chunk string) {
	r.record(ctx, EventLLMCallChunk, func(ev *Event) {
		ev.Deterministic = false
		ev.Metadata["model"] = model
		ev.Metadata["chunk"] = chunk
	})
}

// RecordToolCall records a tool invocation with its result.
func (r *EventRecorder) RecordToolCall(ctx context.Context, toolName string, arguments map[string]any, result any, durationMs int64, retryCount int, errStr string) {
	r.record(ctx, EventToolCallStart, func(ev *Event) {
		ev.Metadata["tool"] = toolName
	})
	r.record(ctx, EventToolCallResult, func(ev *Event) {
		pl := ToolCallPayload{
			ToolName:   toolName,
			Arguments:  arguments,
			Result:     result,
			DurationMs: durationMs,
			RetryCount: retryCount,
			Error:      errStr,
		}
		ev.Payload, _ = json.Marshal(pl)
		if errStr != "" {
			ev.Deterministic = false
		}
	})
}

// RecordSubAgentCall records a sub-agent invocation with its result.
func (r *EventRecorder) RecordSubAgentCall(ctx context.Context, subAgentName string, input, output any, depth int, durationMs int64, errStr string) {
	r.record(ctx, EventSubAgentCallStart, func(ev *Event) {
		pl := SubAgentCallPayload{
			SubAgentName: subAgentName,
			Input:        input,
			Depth:        depth,
		}
		ev.Payload, _ = json.Marshal(pl)
	})
	r.record(ctx, EventSubAgentCallEnd, func(ev *Event) {
		pl := SubAgentCallPayload{
			SubAgentName: subAgentName,
			Output:       output,
			Depth:        depth,
			DurationMs:   durationMs,
			Error:        errStr,
		}
		ev.Payload, _ = json.Marshal(pl)
		if errStr != "" {
			ev.Deterministic = false
		}
	})
}

// RecordSessionValue records a session value change.
func (r *EventRecorder) RecordSessionValue(ctx context.Context, key string, value any) {
	r.record(ctx, EventSessionValueSet, func(ev *Event) {
		pl := SessionValuePayload{Key: key, Value: value}
		ev.Payload, _ = json.Marshal(pl)
	})
}

// RecordSessionTransfer records an agent transfer event.
func (r *EventRecorder) RecordSessionTransfer(ctx context.Context, fromAgent, toAgent, reason string, input any) {
	r.record(ctx, EventSessionTransfer, func(ev *Event) {
		pl := SessionTransferPayload{
			FromAgent: fromAgent,
			ToAgent:   toAgent,
			Reason:    reason,
			Input:     input,
		}
		ev.Payload, _ = json.Marshal(pl)
	})
}

// RecordStateWrite records a state transition.
func (r *EventRecorder) RecordStateWrite(ctx context.Context, channel string, oldValue, newValue any, reducer string) {
	r.record(ctx, EventStateWrite, func(ev *Event) {
		pl := StateTransitionPayload{
			Channel:  channel,
			OldValue: oldValue,
			NewValue: newValue,
			Reducer:  reducer,
		}
		ev.Payload, _ = json.Marshal(pl)
	})
}

// RecordMemoryWrite records a memory operation.
func (r *EventRecorder) RecordMemoryWrite(ctx context.Context, store, operation, key string, value any, score float64) {
	r.record(ctx, EventMemoryWrite, func(ev *Event) {
		pl := MemoryWritePayload{
			Store:     store,
			Operation: operation,
			Key:       key,
			Value:     value,
			Score:     score,
		}
		ev.Payload, _ = json.Marshal(pl)
	})
}

// RecordMemoryRead records a memory read operation.
func (r *EventRecorder) RecordMemoryRead(ctx context.Context, store, key string, score float64) {
	r.record(ctx, EventMemoryRead, func(ev *Event) {
		pl := MemoryWritePayload{
			Store: store,
			Key:   key,
			Score: score,
		}
		ev.Payload, _ = json.Marshal(pl)
	})
}

// RecordApproval records a human-in-the-loop approval event.
func (r *EventRecorder) RecordApproval(ctx context.Context, requestID, action string, context any, decision string, latencyMs int64) {
	r.record(ctx, EventApprovalRequest, func(ev *Event) {
		pl := ApprovalPayload{
			RequestID: requestID,
			Action:    action,
			Context:   context,
			Decision:  decision,
			LatencyMs: latencyMs,
		}
		ev.Payload, _ = json.Marshal(pl)
	})
}

// RecordError records an execution error.
func (r *EventRecorder) RecordError(ctx context.Context, errMsg string) {
	r.record(ctx, EventError, func(ev *Event) {
		ev.Metadata["error"] = errMsg
	})
}

// RecordRetry records a retry event.
func (r *EventRecorder) RecordRetry(ctx context.Context, detail string) {
	r.record(ctx, EventRetry, func(ev *Event) {
		ev.Metadata["detail"] = detail
	})
}

// ---- GraphCallback implementation ----

// OnRunStart implements pregel.RunCallback.
func (r *EventRecorder) OnRunStart(ctx context.Context, graphName, threadID string) {
	r.record(ctx, EventGraphStart, func(ev *Event) {
		ev.Metadata["graph_name"] = graphName
	})
}

// OnRunEnd implements pregel.RunCallback.
func (r *EventRecorder) OnRunEnd(ctx context.Context, graphName, threadID string, err error) {
	r.record(ctx, EventGraphEnd, func(ev *Event) {
		ev.Metadata["graph_name"] = graphName
		if err != nil {
			ev.Metadata["error"] = err.Error()
		}
	})
}

// OnStepStart implements pregel.StepCallback.
func (r *EventRecorder) OnStepStart(ctx context.Context, step, taskCount int) {
	r.record(ctx, EventStepStart, func(ev *Event) {
		ev.Step = step
		ev.Metadata["task_count"] = taskCount
	})
}

// OnStepEnd implements pregel.StepCallback.
func (r *EventRecorder) OnStepEnd(ctx context.Context, step int, err error) {
	r.record(ctx, EventStepEnd, func(ev *Event) {
		ev.Step = step
		if err != nil {
			ev.Metadata["error"] = err.Error()
		}
	})
}

// OnNodeStart implements pregel.NodeCallback.
func (r *EventRecorder) OnNodeStart(ctx context.Context, nodeName string, step int) {
	r.record(ctx, EventNodeStart, func(ev *Event) {
		ev.Node = nodeName
		ev.Step = step
	})
}

// OnNodeEnd implements pregel.NodeCallback.
func (r *EventRecorder) OnNodeEnd(ctx context.Context, nodeName string, step int, output interface{}, err error) {
	r.record(ctx, EventNodeEnd, func(ev *Event) {
		ev.Node = nodeName
		ev.Step = step
		if err != nil {
			ev.Metadata["error"] = err.Error()
		}
	})
}

// OnCheckpointSave implements pregel.CheckpointCallback.
func (r *EventRecorder) OnCheckpointSave(ctx context.Context, threadID, checkpointID string, step int) {
	r.record(ctx, EventCheckpointCreated, func(ev *Event) {
		ev.ThreadID = threadID
		ev.Step = step
		ev.Metadata["checkpoint_id"] = checkpointID
	})
}

// OnCheckpointLoad implements pregel.CheckpointCallback.
func (r *EventRecorder) OnCheckpointLoad(ctx context.Context, threadID, checkpointID string, step int) {
	r.record(ctx, EventCheckpointRestored, func(ev *Event) {
		ev.ThreadID = threadID
		ev.Step = step
		ev.Metadata["checkpoint_id"] = checkpointID
	})
}

// OnCheckpointUpdate implements pregel.CheckpointCallback.
func (r *EventRecorder) OnCheckpointUpdate(ctx context.Context, threadID, asNode string) {
	r.record(ctx, EventStateWrite, func(ev *Event) {
		ev.ThreadID = threadID
		ev.Node = asNode
	})
}

// OnInterrupt implements pregel.InterruptCallback.
func (r *EventRecorder) OnInterrupt(ctx context.Context, nodeNames []string, step int) {
	r.record(ctx, EventInterrupt, func(ev *Event) {
		ev.Step = step
		ev.Metadata["interrupt_nodes"] = nodeNames
	})
}

// OnResume implements pregel.InterruptCallback.
func (r *EventRecorder) OnResume(ctx context.Context, threadID string) {
	r.record(ctx, EventResume, func(ev *Event) {
		ev.ThreadID = threadID
	})
}

// compile-time interface checks
var (
	_ pregel.GraphCallback = (*EventRecorder)(nil)
)
