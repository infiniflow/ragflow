// Package pregel provides streaming support for Pregel execution.
package pregel

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"ragflow/internal/harness/graph/types"
)

// StreamEventType represents the type of stream event.
type StreamEventType string

const (
	// EventTypeCheckpoint is emitted when a checkpoint is created
	EventTypeCheckpoint StreamEventType = "checkpoint"
	// EventTypeTaskStart is emitted when a task starts
	EventTypeTaskStart StreamEventType = "task_start"
	// EventTypeTaskEnd is emitted when a task completes
	EventTypeTaskEnd StreamEventType = "task_end"
	// EventTypeUpdate is emitted when a node updates state
	EventTypeUpdate StreamEventType = "update"
	// EventTypeValues is emitted when state values are emitted
	EventTypeValues StreamEventType = "values"
	// EventTypeInterrupt is emitted when execution is interrupted
	EventTypeInterrupt StreamEventType = "interrupt"
	// EventTypeError is emitted when an error occurs
	EventTypeError StreamEventType = "error"
	// EventTypeFinal is emitted when execution completes
	EventTypeFinal StreamEventType = "final"
	// EventTypeDebug is emitted for debug information
	EventTypeDebug StreamEventType = "debug"
)

// StreamEvent represents a stream event.
type StreamEvent struct {
	// Type is the event type
	Type StreamEventType
	// Timestamp is when the event occurred
	Timestamp time.Time
	// Step is the current step number
	Step int
	// Node is the node name (for task events)
	Node string
	// TaskID is the task ID (for task events)
	TaskID string
	// Data is the event-specific data
	Data any
	// Error is the error (for error events)
	Error error
}

// NewStreamEvent creates a new stream event.
func NewStreamEvent(eventType StreamEventType, step int) *StreamEvent {
	return &StreamEvent{
		Type:      eventType,
		Timestamp: time.Now(),
		Step:      step,
		Data:      make(map[string]any),
	}
}

// ToJSON converts the event to JSON.
func (e *StreamEvent) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

// StreamManager manages streaming for Pregel execution.
type StreamManager struct {
	mode          types.StreamMode
	eventCh       chan *StreamEvent
	errorCh       chan error
	bufferSize    int
	includeFilter map[StreamEventType]bool
	mu            struct {
		sync.RWMutex
		closed bool
	}
}

// NewStreamManager creates a new stream manager.
func NewStreamManager(mode types.StreamMode, bufferSize int) *StreamManager {
	if bufferSize <= 0 {
		bufferSize = 100
	}

	sm := &StreamManager{
		mode:          mode,
		eventCh:       make(chan *StreamEvent, bufferSize),
		errorCh:       make(chan error, 10),
		bufferSize:    bufferSize,
		includeFilter: make(map[StreamEventType]bool),
	}

	// Set up include filter based on stream mode
	sm.setupIncludeFilter()

	return sm
}

// setupIncludeFilter configures which events to include based on stream mode.
func (sm *StreamManager) setupIncludeFilter() {
	switch sm.mode {
	case types.StreamModeValues:
		sm.includeFilter[EventTypeValues] = true
		sm.includeFilter[EventTypeFinal] = true

	case types.StreamModeUpdates:
		sm.includeFilter[EventTypeUpdate] = true
		sm.includeFilter[EventTypeFinal] = true

	case types.StreamModeTasks:
		sm.includeFilter[EventTypeTaskStart] = true
		sm.includeFilter[EventTypeTaskEnd] = true
		sm.includeFilter[EventTypeError] = true
		sm.includeFilter[EventTypeFinal] = true

	case types.StreamModeCheckpoints:
		sm.includeFilter[EventTypeCheckpoint] = true
		sm.includeFilter[EventTypeFinal] = true

	case types.StreamModeDebug:
		// Include all events in debug mode
		sm.includeFilter[EventTypeCheckpoint] = true
		sm.includeFilter[EventTypeTaskStart] = true
		sm.includeFilter[EventTypeTaskEnd] = true
		sm.includeFilter[EventTypeUpdate] = true
		sm.includeFilter[EventTypeValues] = true
		sm.includeFilter[EventTypeInterrupt] = true
		sm.includeFilter[EventTypeError] = true
		sm.includeFilter[EventTypeDebug] = true
		sm.includeFilter[EventTypeFinal] = true

	default:
		// Default to values mode
		sm.includeFilter[EventTypeValues] = true
		sm.includeFilter[EventTypeFinal] = true
	}
}

// shouldEmit checks if an event should be emitted based on stream mode.
func (sm *StreamManager) shouldEmit(eventType StreamEventType) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.includeFilter[eventType]
}

// EmitCheckpoint emits a checkpoint event.
func (sm *StreamManager) EmitCheckpoint(step int, checkpoint map[string]any) {
	if !sm.shouldEmit(EventTypeCheckpoint) {
		return
	}

	event := NewStreamEvent(EventTypeCheckpoint, step)
	event.Data = map[string]any{
		"checkpoint": checkpoint,
	}

	sm.emit(event)
}

// EmitTaskStart emits a task start event.
func (sm *StreamManager) EmitTaskStart(step int, node string, taskID string) {
	if !sm.shouldEmit(EventTypeTaskStart) {
		return
	}

	event := NewStreamEvent(EventTypeTaskStart, step)
	event.Node = node
	event.TaskID = taskID
	event.Data = map[string]any{
		"node":    node,
		"task_id": taskID,
	}

	sm.emit(event)
}

// EmitTaskEnd emits a task end event.
func (sm *StreamManager) EmitTaskEnd(step int, node string, taskID string, output any, duration time.Duration, err error) {
	if !sm.shouldEmit(EventTypeTaskEnd) {
		return
	}

	event := NewStreamEvent(EventTypeTaskEnd, step)
	event.Node = node
	event.TaskID = taskID
	event.Error = err
	event.Data = map[string]any{
		"node":     node,
		"task_id":  taskID,
		"output":   output,
		"duration": duration.String(),
	}

	sm.emit(event)
}

// EmitUpdate emits a state update event.
func (sm *StreamManager) EmitUpdate(step int, node string, output any) {
	if !sm.shouldEmit(EventTypeUpdate) {
		return
	}

	event := NewStreamEvent(EventTypeUpdate, step)
	event.Node = node
	event.Data = map[string]any{
		"node":   node,
		"output": output,
	}

	sm.emit(event)
}

// EmitValues emits state values event.
func (sm *StreamManager) EmitValues(step int, values map[string]any) {
	if !sm.shouldEmit(EventTypeValues) {
		return
	}

	event := NewStreamEvent(EventTypeValues, step)
	event.Data = map[string]any{
		"values": values,
	}

	sm.emit(event)
}

// EmitInterrupt emits an interrupt event.
func (sm *StreamManager) EmitInterrupt(step int, interrupts []string) {
	if !sm.shouldEmit(EventTypeInterrupt) {
		return
	}

	event := NewStreamEvent(EventTypeInterrupt, step)
	event.Data = map[string]any{
		"interrupts": interrupts,
	}

	sm.emit(event)
}

// EmitError emits an error event.
func (sm *StreamManager) EmitError(step int, err error, node string) {
	if !sm.shouldEmit(EventTypeError) {
		return
	}

	event := NewStreamEvent(EventTypeError, step)
	event.Node = node
	event.Error = err
	event.Data = map[string]any{
		"node":  node,
		"error": err.Error(),
	}

	sm.emit(event)
}

// EmitDebug emits a debug event.
func (sm *StreamManager) EmitDebug(step int, message string, data any) {
	if !sm.shouldEmit(EventTypeDebug) {
		return
	}

	event := NewStreamEvent(EventTypeDebug, step)
	event.Data = map[string]any{
		"message": message,
		"data":    data,
	}

	sm.emit(event)
}

// EmitFinal emits final event with complete state.
// Uses blocking channel send to guarantee delivery — dropping the final event
// would cause RunSync to return nil state (silent data loss).
func (sm *StreamManager) EmitFinal(step int, state any) {
	if !sm.shouldEmit(EventTypeFinal) {
		return
	}

	event := NewStreamEvent(EventTypeFinal, step)
	event.Data = map[string]any{
		"state": state,
	}
	sm.eventCh <- event
}

// emit sends an event to the channel.
func (sm *StreamManager) emit(event *StreamEvent) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if sm.mu.closed {
		return
	}

	select {
	case sm.eventCh <- event:
		// Event sent
	default:
		// Channel full, drop event
	}
}

// Events returns the event channel.
func (sm *StreamManager) Events() <-chan *StreamEvent {
	return sm.eventCh
}

// Errors returns the error channel.
func (sm *StreamManager) Errors() <-chan error {
	return sm.errorCh
}

// Close closes the stream manager.
func (sm *StreamManager) Close() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if !sm.mu.closed {
		sm.mu.closed = true
		close(sm.eventCh)
		close(sm.errorCh)
	}
}

// StreamWriter provides a streaming output for node functions.
type StreamWriter struct {
	streamManager *StreamManager
	step          int
	node          string
}

// NewStreamWriter creates a new stream writer.
func NewStreamWriter(sm *StreamManager, step int, node string) *StreamWriter {
	return &StreamWriter{
		streamManager: sm,
		step:          step,
		node:          node,
	}
}

// Write writes data to the stream.
func (w *StreamWriter) Write(data any) error {
	w.streamManager.EmitDebug(w.step, fmt.Sprintf("custom output from node %s", w.node), data)
	return nil
}

// WriteJSON writes JSON data to the stream.
func (w *StreamWriter) WriteJSON(data any) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return w.Write(jsonData)
}

// EventStream represents a stream of execution events.
type EventStream struct {
	ctx          context.Context
	cancel       context.CancelFunc
	streamEvents chan *StreamEvent
	streamErrors chan error
}

// NewEventStream creates a new event stream.
func NewEventStream(ctx context.Context) *EventStream {
	ctx, cancel := context.WithCancel(ctx)
	return &EventStream{
		ctx:          ctx,
		cancel:       cancel,
		streamEvents: make(chan *StreamEvent, 100),
		streamErrors: make(chan error, 10),
	}
}

// Context returns the stream's context.
func (es *EventStream) Context() context.Context {
	return es.ctx
}

// Cancel cancels the stream.
func (es *EventStream) Cancel() {
	es.cancel()
	close(es.streamEvents)
	close(es.streamErrors)
}

// Emit emits an event to the stream.
func (es *EventStream) Emit(event *StreamEvent) {
	select {
	case es.streamEvents <- event:
	case <-es.ctx.Done():
	}
}

// EmitError emits an error to the stream.
func (es *EventStream) EmitError(err error) {
	select {
	case es.streamErrors <- err:
	case <-es.ctx.Done():
	}
}

// Stream returns the event and error channels.
func (es *EventStream) Stream() (<-chan *StreamEvent, <-chan error) {
	return es.streamEvents, es.streamErrors
}

// StreamProcessor processes stream events.
type StreamProcessor struct {
	filter    func(*StreamEvent) bool
	transform func(*StreamEvent) (*StreamEvent, error)
	handler   func(*StreamEvent)
}

// NewStreamProcessor creates a new stream processor.
func NewStreamProcessor() *StreamProcessor {
	return &StreamProcessor{
		filter:    func(e *StreamEvent) bool { return true },
		transform: func(e *StreamEvent) (*StreamEvent, error) { return e, nil },
		handler:   func(e *StreamEvent) {},
	}
}

// WithFilter sets an event filter.
func (sp *StreamProcessor) WithFilter(filter func(*StreamEvent) bool) *StreamProcessor {
	sp.filter = filter
	return sp
}

// WithTransform sets an event transformer.
func (sp *StreamProcessor) WithTransform(transform func(*StreamEvent) (*StreamEvent, error)) *StreamProcessor {
	sp.transform = transform
	return sp
}

// WithHandler sets an event handler.
func (sp *StreamProcessor) WithHandler(handler func(*StreamEvent)) *StreamProcessor {
	sp.handler = handler
	return sp
}

// Process processes a stream event.
func (sp *StreamProcessor) Process(event *StreamEvent) error {
	if !sp.filter(event) {
		return nil
	}

	transformed, err := sp.transform(event)
	if err != nil {
		return err
	}

	sp.handler(transformed)
	return nil
}

// ProcessStream processes all events from a stream.
func (sp *StreamProcessor) ProcessStream(ctx context.Context, eventCh <-chan *StreamEvent) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-eventCh:
			if !ok {
				return nil
			}
			if err := sp.Process(event); err != nil {
				return err
			}
		}
	}
}
