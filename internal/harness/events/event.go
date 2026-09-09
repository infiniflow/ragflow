// Package events provides append-only event sourcing for agent execution.
//
// Every tool call, state transition, memory write, approval, LLM invocation,
// and checkpoint operation is recorded as an immutable Event. Events are
// causally ordered via a monotonic logical clock, enabling deterministic
// replay, fork/diff, and postmortem analysis.
package events

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"
)

// EventID is a globally unique event identifier (UUID v7, time-ordered).
type EventID string

// EventType enumerates every recordable action during agent execution.
type EventType string

const (
	// Graph execution lifecycle.
	EventGraphStart EventType = "graph.start"
	EventGraphEnd   EventType = "graph.end"
	EventStepStart  EventType = "step.start"
	EventStepEnd    EventType = "step.end"

	// Node execution.
	EventNodeStart EventType = "node.start"
	EventNodeEnd   EventType = "node.end"

	// State transitions.
	EventStateRead  EventType = "state.read"
	EventStateWrite EventType = "state.write"

	// Tool calls.
	EventToolCallStart  EventType = "tool.call.start"
	EventToolCallResult EventType = "tool.call.result"
	EventToolCallError  EventType = "tool.call.error"

	// LLM invocations.
	EventLLMCallStart EventType = "llm.call.start"
	EventLLMCallChunk EventType = "llm.call.chunk"
	EventLLMCallEnd   EventType = "llm.call.end"

	// Memory operations.
	EventMemoryRead  EventType = "memory.read"
	EventMemoryWrite EventType = "memory.write"

	// Human-in-the-loop.
	EventApprovalRequest EventType = "approval.request"
	EventApprovalGranted EventType = "approval.granted"
	EventApprovalDenied  EventType = "approval.denied"

	// Checkpoint.
	EventCheckpointCreated  EventType = "checkpoint.created"
	EventCheckpointRestored EventType = "checkpoint.restored"

	// Interrupt / Resume.
	EventInterrupt EventType = "interrupt"
	EventResume    EventType = "resume"

	// Error & retry.
	EventError EventType = "error"
	EventRetry EventType = "retry"

	// Fork — branch from an existing event.
	EventFork EventType = "fork"

	// Sub-agent execution.
	EventSubAgentCallStart EventType = "subagent.call.start"
	EventSubAgentCallEnd   EventType = "subagent.call.end"

	// Session / Transfer.
	EventSessionValueSet EventType = "session.value.set"
	EventSessionTransfer EventType = "session.transfer"
)

// Event is an immutable append-only event.
type Event struct {
	// ID is the globally unique event identifier.
	ID EventID `json:"id"`
	// Type describes what happened.
	Type EventType `json:"type"`
	// Timestamp is the wall-clock time when this event was recorded.
	Timestamp time.Time `json:"timestamp"`
	// Clock is the monotonic logical clock value (global total order).
	Clock uint64 `json:"clock"`

	// TraceID identifies one complete execution trace.
	TraceID string `json:"trace_id"`
	// ParentID is the immediate predecessor event in the same trace.
	ParentID EventID `json:"parent_id,omitempty"`
	// CausedBy lists predecessor events (multiple for fork/join scenarios).
	CausedBy []EventID `json:"caused_by,omitempty"`

	// ThreadID identifies the execution thread.
	ThreadID string `json:"thread_id,omitempty"`
	// Step is the Pregel superstep number.
	Step int `json:"step,omitempty"`
	// Node is the graph node name.
	Node string `json:"node,omitempty"`
	// TaskID identifies the execution task.
	TaskID string `json:"task_id,omitempty"`

	// Payload is the type-specific event payload (JSON).
	Payload json.RawMessage `json:"payload,omitempty"`
	// Metadata holds arbitrary key-value metadata.
	Metadata map[string]any `json:"metadata,omitempty"`

	// Deterministic is false when the event involves non-deterministic
	// operations (LLM output, random, wall-clock time).
	Deterministic bool `json:"deterministic"`
	// Hash is the SHA-256 of Payload+Metadata (for integrity verification).
	Hash string `json:"hash,omitempty"`
}

// NewEvent creates a new Event with auto-generated ID and current timestamp.
func NewEvent(typ EventType, clock uint64) *Event {
	return &Event{
		ID:        EventID(fmt.Sprintf("evt-%d-%x", clock, time.Now().UnixNano())),
		Type:      typ,
		Timestamp: time.Now(),
		Clock:     clock,
		Metadata:  make(map[string]any),
	}
}

// computeHash computes the SHA-256 hash of the event payload and metadata.
func (e *Event) computeHash() string {
	h := sha256.New()
	if e.Payload != nil {
		h.Write(e.Payload)
	}
	if e.Metadata != nil {
		meta, _ := json.Marshal(e.Metadata)
		h.Write(meta)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// Seal finalises the event by computing its hash and marking it immutable.
func (e *Event) Seal() {
	e.Hash = e.computeHash()
}

// ---- typed payloads ----

// ToolCallPayload is the payload for tool call events.
type ToolCallPayload struct {
	ToolName   string         `json:"tool_name"`
	Arguments  map[string]any `json:"arguments,omitempty"`
	Result     any            `json:"result,omitempty"`
	DurationMs int64          `json:"duration_ms,omitempty"`
	Error      string         `json:"error,omitempty"`
	RetryCount int            `json:"retry_count,omitempty"`
}

// LLMCallPayload is the payload for LLM invocation events.
type LLMCallPayload struct {
	Model      string     `json:"model"`
	Provider   string     `json:"provider,omitempty"`
	Messages   []any      `json:"messages,omitempty"`
	Tokens     TokenUsage `json:"tokens,omitempty"`
	Content    string     `json:"content,omitempty"`
	Chunks     int        `json:"chunks,omitempty"`
	DurationMs int64      `json:"duration_ms,omitempty"`
	Cost       float64    `json:"cost,omitempty"`
}

// TokenUsage tracks token consumption for an LLM call.
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// StateTransitionPayload is the payload for state change events.
type StateTransitionPayload struct {
	Channel  string `json:"channel"`
	OldValue any    `json:"old_value,omitempty"`
	NewValue any    `json:"new_value"`
	Reducer  string `json:"reducer,omitempty"`
}

// MemoryWritePayload is the payload for memory operation events.
type MemoryWritePayload struct {
	Store     string  `json:"store"`
	Operation string  `json:"operation"`
	Key       string  `json:"key,omitempty"`
	Value     any     `json:"value,omitempty"`
	Score     float64 `json:"score,omitempty"`
}

// ApprovalPayload is the payload for approval events.
type ApprovalPayload struct {
	RequestID string `json:"request_id"`
	Action    string `json:"action"`
	Context   any    `json:"context,omitempty"`
	Decision  string `json:"decision,omitempty"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
}

// SubAgentCallPayload is the payload for sub-agent call events.
type SubAgentCallPayload struct {
	SubAgentName string `json:"sub_agent_name"`
	Input        any    `json:"input,omitempty"`
	Output       any    `json:"output,omitempty"`
	Depth        int    `json:"depth,omitempty"`
	DurationMs   int64  `json:"duration_ms,omitempty"`
	Error        string `json:"error,omitempty"`
}

// SessionValuePayload is the payload for session value events.
type SessionValuePayload struct {
	Key   string `json:"key"`
	Value any    `json:"value,omitempty"`
}

// SessionTransferPayload is the payload for session transfer events.
type SessionTransferPayload struct {
	FromAgent string `json:"from_agent"`
	ToAgent   string `json:"to_agent"`
	Reason    string `json:"reason,omitempty"`
	Input     any    `json:"input,omitempty"`
}
