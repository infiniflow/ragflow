package pregel

import (
	"context"
	"fmt"
	"io"
	"sync"

	"ragflow/internal/harness/graph/types"
)

// StreamMessagesHandler handles streaming of messages from LLM nodes.
// It supports token-by-token streaming and message chunk aggregation.
type StreamMessagesHandler struct {
	streams      map[string]*MessageStream
	mu           sync.RWMutex
	aggregator   *MessageAggregator
	flushTrigger FlushTrigger
}

// MessageStream represents a stream of message chunks from a single node.
type MessageStream struct {
	node    string
	chunks  []*MessageChunk
	current *MessageChunk
	closed  bool
	mu      sync.RWMutex
}

// MessageChunk represents a chunk of a message.
type MessageChunk struct {
	Index      int
	Content    string
	Metadata   map[string]any
	IsComplete bool
	Role       string
	ToolCalls  []*ToolCall
}

// ToolCall represents a tool call within a message.
type ToolCall struct {
	ID        string
	Name      string
	Arguments map[string]any
}

// FlushTrigger determines when to flush aggregated messages.
type FlushTrigger int

const (
	FlushOnComplete FlushTrigger = iota // Flush when message is complete
	FlushImmediate                      // Flush immediately
	FlushOnEnd                          // Flush on stream end
)

// NewStreamMessagesHandler creates a new stream messages handler.
func NewStreamMessagesHandler(opts ...StreamMessagesOption) *StreamMessagesHandler {
	h := &StreamMessagesHandler{
		streams:      make(map[string]*MessageStream),
		aggregator:   NewMessageAggregator(),
		flushTrigger: FlushOnComplete,
	}

	for _, opt := range opts {
		opt(h)
	}

	return h
}

// StreamMessagesOption configures a StreamMessagesHandler.
type StreamMessagesOption func(*StreamMessagesHandler)

// WithFlushTrigger sets the flush trigger.
func WithFlushTrigger(trigger FlushTrigger) StreamMessagesOption {
	return func(h *StreamMessagesHandler) {
		h.flushTrigger = trigger
	}
}

// OnChunk handles a message chunk from a node.
func (h *StreamMessagesHandler) OnChunk(ctx context.Context, node string, chunk *MessageChunk) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Get or create stream for this node
	ms, ok := h.streams[node]
	if !ok {
		ms = &MessageStream{
			node:   node,
			chunks: make([]*MessageChunk, 0),
		}
		h.streams[node] = ms
	}

	ms.mu.Lock()
	defer ms.mu.Unlock()

	if ms.closed {
		return fmt.Errorf("stream for node %s is closed", node)
	}

	// Add chunk to current message or create new one
	if ms.current == nil || ms.current.IsComplete {
		ms.current = &MessageChunk{
			Index:    len(ms.chunks),
			Metadata: make(map[string]any),
		}
	}

	// Merge chunk content
	ms.current.Content += chunk.Content

	// Update metadata
	if chunk.Metadata != nil {
		if ms.current.Metadata == nil {
			ms.current.Metadata = make(map[string]any)
		}
		for k, v := range chunk.Metadata {
			ms.current.Metadata[k] = v
		}
	}

	// Update role
	if chunk.Role != "" {
		ms.current.Role = chunk.Role
	}

	// Merge tool calls
	if len(chunk.ToolCalls) > 0 {
		if ms.current.ToolCalls == nil {
			ms.current.ToolCalls = make([]*ToolCall, 0)
		}
		ms.current.ToolCalls = append(ms.current.ToolCalls, chunk.ToolCalls...)
	}

	// Mark as complete if needed
	if chunk.IsComplete {
		ms.current.IsComplete = true
		ms.chunks = append(ms.chunks, ms.current)
		ms.current = nil

		// Add to aggregator
		h.aggregator.AddMessage(node, ms.chunks[len(ms.chunks)-1])

		// Flush if needed
		if h.flushTrigger == FlushOnComplete {
			return h.aggregator.Flush(ctx)
		}
	} else if h.flushTrigger == FlushImmediate {
		return h.aggregator.Flush(ctx)
	}

	return nil
}

// OnComplete marks a node's stream as complete.
func (h *StreamMessagesHandler) OnComplete(ctx context.Context, node string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if ms, ok := h.streams[node]; ok {
		ms.mu.Lock()
		defer ms.mu.Unlock()

		if !ms.closed {
			ms.closed = true

			// Complete any pending chunk
			if ms.current != nil {
				ms.current.IsComplete = true
				ms.chunks = append(ms.chunks, ms.current)
				ms.current = nil
				h.aggregator.AddMessage(node, ms.chunks[len(ms.chunks)-1])
			}

			if h.flushTrigger == FlushOnEnd {
				return h.aggregator.Flush(ctx)
			}
		}
	}

	return nil
}

// Flush flushes all pending messages.
func (h *StreamMessagesHandler) Flush(ctx context.Context) error {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.aggregator.Flush(ctx)
}

// AddEmitter adds a message emitter to the handler.
func (h *StreamMessagesHandler) AddEmitter(emitter MessageEmitter) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.aggregator.AddEmitter(emitter)
}

// GetMessages returns all messages from a node.
func (h *StreamMessagesHandler) GetMessages(node string) []*MessageChunk {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if ms, ok := h.streams[node]; ok {
		ms.mu.RLock()
		defer ms.mu.RUnlock()
		return ms.chunks
	}
	return nil
}

// GetAllMessages returns all messages from all nodes.
func (h *StreamMessagesHandler) GetAllMessages() map[string][]*MessageChunk {
	h.mu.RLock()
	defer h.mu.RUnlock()

	all := make(map[string][]*MessageChunk)
	for node, ms := range h.streams {
		ms.mu.RLock()
		chunks := make([]*MessageChunk, len(ms.chunks))
		copy(chunks, ms.chunks)
		ms.mu.RUnlock()
		all[node] = chunks
	}
	return all
}

// Close closes all streams and flushes any pending messages.
func (h *StreamMessagesHandler) Close(ctx context.Context) error {
	h.mu.Lock()
	nodes := make([]string, 0, len(h.streams))
	for node := range h.streams {
		nodes = append(nodes, node)
	}
	h.mu.Unlock()

	for _, node := range nodes {
		_ = h.OnComplete(ctx, node)
	}

	return h.aggregator.Flush(ctx)
}

// MessageAggregator aggregates and emits messages.
type MessageAggregator struct {
	messages map[string][]*MessageChunk
	emitters []MessageEmitter
	mu       sync.RWMutex
}

// MessageEmitter is a function that emits aggregated messages.
type MessageEmitter func(ctx context.Context, node string, chunk *MessageChunk) error

// NewMessageAggregator creates a new message aggregator.
func NewMessageAggregator() *MessageAggregator {
	return &MessageAggregator{
		messages: make(map[string][]*MessageChunk),
		emitters: make([]MessageEmitter, 0),
	}
}

// AddMessage adds a message to the aggregator.
func (a *MessageAggregator) AddMessage(node string, chunk *MessageChunk) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.messages[node] = append(a.messages[node], chunk)
}

// Flush emits all pending messages to registered emitters.
func (a *MessageAggregator) Flush(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	for node, chunks := range a.messages {
		for _, chunk := range chunks {
			for _, emitter := range a.emitters {
				if err := emitter(ctx, node, chunk); err != nil {
					return err
				}
			}
		}
		delete(a.messages, node)
	}

	return nil
}

// AddEmitter adds a message emitter.
func (a *MessageAggregator) AddEmitter(emitter MessageEmitter) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.emitters = append(a.emitters, emitter)
}

// StreamToChannel emits messages to a stream channel.
func StreamToChannel(ch *types.ChannelStream) MessageEmitter {
	return func(ctx context.Context, node string, chunk *MessageChunk) error {
		chunkData := map[string]any{
			"node":     node,
			"role":     chunk.Role,
			"content":  chunk.Content,
			"index":    chunk.Index,
			"complete": chunk.IsComplete,
		}
		if len(chunk.ToolCalls) > 0 {
			chunkData["tool_calls"] = chunk.ToolCalls
		}
		if len(chunk.Metadata) > 0 {
			chunkData["metadata"] = chunk.Metadata
		}
		return ch.Emit(ctx, &types.StreamChunk{
			Mode: types.StreamModeMessages,
			Data: chunkData,
			Node: node,
		})
	}
}

// StreamToWriter emits messages to an io.Writer.
func StreamToWriter(w io.Writer, format string) MessageEmitter {
	return func(ctx context.Context, node string, chunk *MessageChunk) error {
		var output string
		switch format {
		case "json":
			output = fmt.Sprintf(`{"node":"%s","role":"%s","content":%q}`,
				node, chunk.Role, chunk.Content)
		case "compact":
			output = fmt.Sprintf("[%s:%s] %s", node, chunk.Role, chunk.Content)
		default:
			output = fmt.Sprintf("%s: %s", chunk.Role, chunk.Content)
		}
		_, err := fmt.Fprintln(w, output)
		return err
	}
}

// CollectToMap collects messages to a map.
func CollectToMap(target map[string][]*MessageChunk) MessageEmitter {
	return func(ctx context.Context, node string, chunk *MessageChunk) error {
		target[node] = append(target[node], chunk)
		return nil
	}
}

// StreamModeMessages integrates with the Pregel engine.
// This is a helper function to set up message streaming.
func StreamModeMessages(opts ...StreamMessagesOption) *StreamMessagesHandler {
	return NewStreamMessagesHandler(opts...)
}

// ExtractMessagesFromOutput extracts messages from node output.
func ExtractMessagesFromOutput(output any) ([]*MessageChunk, error) {
	// Check if output is already a MessageChunk
	if chunk, ok := output.(*MessageChunk); ok {
		return []*MessageChunk{chunk}, nil
	}

	// Check if output is a slice of MessageChunks
	if chunks, ok := output.([]*MessageChunk); ok {
		return chunks, nil
	}

	// Try to extract from map
	if m, ok := output.(map[string]any); ok {
		if content, ok := m["content"].(string); ok {
			role := "assistant"
			if r, ok := m["role"].(string); ok {
				role = r
			}
			chunk := &MessageChunk{
				Content:  content,
				Role:     role,
				Metadata: make(map[string]any),
			}
			return []*MessageChunk{chunk}, nil
		}
	}

	return nil, fmt.Errorf("cannot extract messages from output of type %T", output)
}

// ConvertToTaskResult converts message chunks to a task result.
func ConvertToTaskResult(node string, chunks []*MessageChunk) *TaskResult {
	if len(chunks) == 0 {
		return &TaskResult{
			Name:   node,
			Output: nil,
			Err:    nil,
		}
	}

	// Merge chunks
	merged := &MessageChunk{
		Content:   "",
		Metadata:  make(map[string]any),
		ToolCalls: make([]*ToolCall, 0),
	}

	for _, chunk := range chunks {
		merged.Content += chunk.Content
		if chunk.Role != "" {
			merged.Role = chunk.Role
		}
		for k, v := range chunk.Metadata {
			merged.Metadata[k] = v
		}
		if len(chunk.ToolCalls) > 0 {
			merged.ToolCalls = append(merged.ToolCalls, chunk.ToolCalls...)
		}
	}

	// Convert to map for output
	output := map[string]any{
		"messages": []*MessageChunk{merged},
		"content":  merged.Content,
		"role":     merged.Role,
	}

	if len(merged.ToolCalls) > 0 {
		output["tool_calls"] = merged.ToolCalls
	}

	return &TaskResult{
		Name:   node,
		Output: output,
		Err:    nil,
	}
}

// MessageStreamWrapper wraps a stream protocol to handle messages.
type MessageStreamWrapper struct {
	handler *StreamMessagesHandler
	stream  *types.ChannelStream
	ctx     context.Context
}

// NewMessageStreamWrapper creates a new message stream wrapper.
func NewMessageStreamWrapper(ctx context.Context, stream *types.ChannelStream, opts ...StreamMessagesOption) *MessageStreamWrapper {
	handler := NewStreamMessagesHandler(opts...)
	handler.AddEmitter(StreamToChannel(stream))

	return &MessageStreamWrapper{
		handler: handler,
		stream:  stream,
		ctx:     ctx,
	}
}

// HandleChunk handles a message chunk.
func (w *MessageStreamWrapper) HandleChunk(node string, chunk *MessageChunk) error {
	return w.handler.OnChunk(w.ctx, node, chunk)
}

// HandleComplete marks a node as complete.
func (w *MessageStreamWrapper) HandleComplete(node string) error {
	return w.handler.OnComplete(w.ctx, node)
}

// Close closes the wrapper and flushes all messages.
func (w *MessageStreamWrapper) Close() error {
	return w.handler.Close(w.ctx)
}

// GetHandler returns the underlying handler.
func (w *MessageStreamWrapper) GetHandler() *StreamMessagesHandler {
	return w.handler
}
