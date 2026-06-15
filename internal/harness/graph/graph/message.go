package graph

import (
	"context"
	"fmt"

	"ragflow/internal/harness/graph/channels"
	"ragflow/internal/harness/graph/types"
)

// Message represents a message in the conversation.
type Message struct {
	ID      string                 // Unique identifier for deduplication
	Role    string                 // e.g., "user", "assistant", "system"
	Content string                 // The message content
	Extra   map[string]interface{} // Additional metadata
}

// NewMessage creates a new message without an ID.
func NewMessage(role, content string) *Message {
	return &Message{
		ID:      "",
		Role:    role,
		Content: content,
		Extra:   make(map[string]interface{}),
	}
}

// NewMessageWithID creates a new message with an ID.
func NewMessageWithID(id, role, content string) *Message {
	return &Message{
		ID:      id,
		Role:    role,
		Content: content,
		Extra:   make(map[string]interface{}),
	}
}

// MessagesState represents the state for message-based graphs.
// It contains a list of messages and optional additional fields.
type MessagesState struct {
	Messages []*Message
	Extra    map[string]interface{}
}

// AddMessages adds messages to the state.
func (s *MessagesState) AddMessages(msgs ...*Message) {
	s.Messages = append(s.Messages, msgs...)
}

// GetMessages returns all messages.
func (s *MessagesState) GetMessages() []*Message {
	return s.Messages
}

// GetLastMessage returns the last message.
func (s *MessagesState) GetLastMessage() *Message {
	if len(s.Messages) == 0 {
		return nil
	}
	return s.Messages[len(s.Messages)-1]
}

// GetMessagesByRole returns messages of a specific role.
func (s *MessagesState) GetMessagesByRole(role string) []*Message {
	filtered := make([]*Message, 0)
	for _, msg := range s.Messages {
		if msg.Role == role {
			filtered = append(filtered, msg)
		}
	}
	return filtered
}

// AddMessagesReducer is a reducer function that adds messages to the state.
// It performs deduplication based on message ID.
func AddMessagesReducer(existing interface{}, updates interface{}) (interface{}, error) {
	msgs, ok := updates.([]*Message)
	if !ok {
		// Try single message
		if msg, ok := updates.(*Message); ok {
			msgs = []*Message{msg}
		} else {
			return nil, &GraphError{Message: fmt.Sprintf("cannot add messages of type %T", updates)}
		}
	}

	if existing == nil {
		return msgs, nil
	}

	existingMsgs, ok := existing.([]*Message)
	if !ok {
		return nil, &GraphError{Message: fmt.Sprintf("existing messages is not []*Message, got %T", existing)}
	}

	// Create a map of existing messages by ID for quick lookup
	existingMap := make(map[string]*Message)
	for _, msg := range existingMsgs {
		if msg.ID != "" {
			existingMap[msg.ID] = msg
		}
	}

	// Process updates: update existing messages with same ID, append new ones
	result := make([]*Message, 0, len(existingMsgs)+len(msgs))
	// Keep track of which IDs have been processed
	processedIDs := make(map[string]bool)
	
	// First, add all existing messages, updating those that have updates
	for _, msg := range existingMsgs {
		if msg.ID == "" {
			// Messages without ID are always kept as-is
			result = append(result, msg)
			continue
		}
		// Check if there's an update for this ID
		var updated *Message
		for _, update := range msgs {
			if update.ID == msg.ID {
				updated = update
				break
			}
		}
		if updated != nil {
			result = append(result, updated)
			processedIDs[msg.ID] = true
		} else {
			result = append(result, msg)
		}
	}
	
	// Then, append new messages that don't have matching IDs in existing
	for _, msg := range msgs {
		if msg.ID == "" {
			// Messages without ID are always appended
			result = append(result, msg)
		} else if !processedIDs[msg.ID] && existingMap[msg.ID] == nil {
			// This is a new message with an ID not in existing
			result = append(result, msg)
		}
	}

	return result, nil
}

// MessageGraph is a graph specialized for message-based workflows.
// It automatically manages a messages channel with the AddMessages reducer.
type MessageGraph struct {
	graph            *StateGraph
	messagesChannel  string
}

// NewMessageGraph creates a new message-based graph.
func NewMessageGraph() *MessageGraph {
	// Create a simple state schema with messages field
	stateSchema := map[string]interface{}{
		"messages": []any{},
	}

	g := NewStateGraph(stateSchema)

	// Register the messages channel so GetMessages works
	messagesChannel := "messages"
	g.AddChannel(messagesChannel, channels.NewLastValue([]*Message{}))

	return &MessageGraph{
		graph:           g,
		messagesChannel: messagesChannel,
	}
}

// AddNode adds a node to the message graph.
func (g *MessageGraph) AddNode(name string, action types.NodeFunc) *Node {
	return g.graph.AddNode(name, action)
}

// AddEdge adds a directed edge between nodes.
func (g *MessageGraph) AddEdge(startKey, endKey string) error {
	return g.graph.AddEdge(startKey, endKey)
}

// AddConditionalEdge adds a conditional edge.
func (g *MessageGraph) AddConditionalEdge(source string, condition types.EdgeFunc, edgeMap map[string]string) error {
	return g.graph.AddConditionalEdges(source, condition, edgeMap)
}

// SetEntryPoint sets the entry point node.
func (g *MessageGraph) SetEntryPoint(node string) error {
	return g.graph.SetEntryPoint(node)
}

// Build returns a compiled message graph.
func (g *MessageGraph) Build() (*CompiledGraph, error) {
	return g.graph.Compile()
}

// GetState returns the current state of the graph.
func (g *MessageGraph) GetState() map[string]interface{} {
	return map[string]interface{}{
		"messages": []any{},
	}
}

// GetMessages returns the messages channel value.
func (g *MessageGraph) GetMessages(ctx context.Context, channelRegistry *channels.Registry) ([]*Message, error) {
	if ch, ok := channelRegistry.Get(g.messagesChannel); ok {
		data, err := ch.Get()
		if err == nil && data != nil {
			if msgs, ok := data.([]*Message); ok {
				return msgs, nil
			}
		}
	}
	return []*Message{}, nil
}

// GetMessagesFromState extracts messages from state.
func GetMessagesFromState(state map[string]interface{}) ([]*Message, error) {
	messages, ok := state["messages"]
	if !ok {
		return []*Message{}, nil
	}

	switch msgs := messages.(type) {
	case []*Message:
		return msgs, nil
	case []interface{}:
		result := make([]*Message, len(msgs))
		for i, m := range msgs {
			if msg, ok := m.(*Message); ok {
				result[i] = msg
			} else {
				return nil, &GraphError{Message: fmt.Sprintf("message at index %d is not *Message, got %T", i, m)}
			}
		}
		return result, nil
	default:
		return nil, &GraphError{Message: fmt.Sprintf("messages is not []*Message, got %T", messages)}
	}
}

// AddMessagesToState adds messages to the state.
func AddMessagesToState(state map[string]interface{}, msgs ...*Message) error {
	existing, err := GetMessagesFromState(state)
	if err != nil {
		return err
	}

	result := make([]*Message, len(existing)+len(msgs))
	copy(result, existing)
	copy(result[len(existing):], msgs)
	state["messages"] = result
	return nil
}

// MessageRole constants
const (
	MessageRoleUser      = "user"
	MessageRoleAssistant = "assistant"
	MessageRoleSystem    = "system"
	MessageRoleTool      = "tool"
	MessageRoleFunction  = "function"
)

// HumanMessage creates a user message.
func HumanMessage(content string) *Message {
	return NewMessage(MessageRoleUser, content)
}

// AIMessage creates an assistant message.
func AIMessage(content string) *Message {
	return NewMessage(MessageRoleAssistant, content)
}

// SystemMessage creates a system message.
func SystemMessage(content string) *Message {
	return NewMessage(MessageRoleSystem, content)
}

// ToolMessage creates a tool message.
func ToolMessage(content string, toolCallID string) *Message {
	msg := NewMessage(MessageRoleTool, content)
	if msg.Extra == nil {
		msg.Extra = make(map[string]interface{})
	}
	msg.Extra["tool_call_id"] = toolCallID
	return msg
}

// FunctionMessage creates a function message.
func FunctionMessage(content string, name string) *Message {
	msg := NewMessage(MessageRoleFunction, content)
	if msg.Extra == nil {
		msg.Extra = make(map[string]interface{})
	}
	msg.Extra["name"] = name
	return msg
}

// MessageHelper provides utility functions for working with messages.
type MessageHelper struct {
}

// FormatMessages formats messages for display or logging.
func FormatMessages(msgs []*Message) []string {
	formatted := make([]string, len(msgs))
	for i, msg := range msgs {
		formatted[i] = fmt.Sprintf("%s: %s", msg.Role, msg.Content)
	}
	return formatted
}

// GetLastUserMessage returns the last user message.
func GetLastUserMessage(msgs []*Message) *Message {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == MessageRoleUser {
			return msgs[i]
		}
	}
	return nil
}

// GetLastAIMessage returns the last assistant message.
func GetLastAIMessage(msgs []*Message) *Message {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == MessageRoleAssistant {
			return msgs[i]
		}
	}
	return nil
}

// FilterMessagesByRole returns messages filtered by role.
func FilterMessagesByRole(msgs []*Message, roles ...string) []*Message {
	roleSet := make(map[string]bool)
	for _, role := range roles {
		roleSet[role] = true
	}

	filtered := make([]*Message, 0)
	for _, msg := range msgs {
		if roleSet[msg.Role] {
			filtered = append(filtered, msg)
		}
	}
	return filtered
}

// MessagesFilter provides message filtering capabilities.
type MessagesFilter struct {
	roles    []string
	limit    int
	offset   int
	reverse  bool
	predicate func(*Message) bool
}

// NewMessagesFilter creates a new messages filter.
func NewMessagesFilter() *MessagesFilter {
	return &MessagesFilter{}
}

// WithRole filters by message roles.
func (f *MessagesFilter) WithRole(roles ...string) *MessagesFilter {
	f.roles = roles
	return f
}

// WithLimit limits the number of messages.
func (f *MessagesFilter) WithLimit(limit int) *MessagesFilter {
	f.limit = limit
	return f
}

// WithOffset skips the first offset messages.
func (f *MessagesFilter) WithOffset(offset int) *MessagesFilter {
	f.offset = offset
	return f
}

// WithReverse reverses the message order.
func (f *MessagesFilter) WithReverse() *MessagesFilter {
	f.reverse = true
	return f
}

// WithPredicate adds a custom predicate function.
func (f *MessagesFilter) WithPredicate(predicate func(*Message) bool) *MessagesFilter {
	f.predicate = predicate
	return f
}

// Filter applies the filter to the messages.
func (f *MessagesFilter) Filter(msgs []*Message) []*Message {
	result := make([]*Message, 0)

	for _, msg := range msgs {
		// Check role filter
		if len(f.roles) > 0 {
			match := false
			for _, role := range f.roles {
				if msg.Role == role {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}

		// Check predicate
		if f.predicate != nil && !f.predicate(msg) {
			continue
		}

		result = append(result, msg)
	}

	// Apply offset
	if f.offset > 0 && f.offset < len(result) {
		result = result[f.offset:]
	}

	// Apply limit
	if f.limit > 0 && f.limit < len(result) {
		result = result[:f.limit]
	}

	// Apply reverse
	if f.reverse {
		for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
			result[i], result[j] = result[j], result[i]
		}
	}

	return result
}

// GraphError represents a graph-related error.
type GraphError struct {
	Message string
	Code    string
}

func (e *GraphError) Error() string {
	if e.Code != "" {
		return e.Code + ": " + e.Message
	}
	return e.Message
}

// OpenAI format conversion utilities

// OpenAIChatMessage represents a message in OpenAI's chat completion API format.
type OpenAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Name    string `json:"name,omitempty"`
	// Additional fields like function_call, tool_calls can be added as needed
}

// ToOpenAIChatMessage converts a Message to OpenAI chat message format.
func (m *Message) ToOpenAIChatMessage() *OpenAIChatMessage {
	return &OpenAIChatMessage{
		Role:    m.Role,
		Content: m.Content,
		// Name can be extracted from Extra if needed
	}
}

// MessagesToOpenAIFormat converts a slice of Messages to OpenAI chat completion format.
func MessagesToOpenAIFormat(messages []*Message) []OpenAIChatMessage {
	result := make([]OpenAIChatMessage, len(messages))
	for i, msg := range messages {
		result[i] = OpenAIChatMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}
	return result
}

// OpenAIFormatToMessages converts OpenAI format messages back to Messages.
func OpenAIFormatToMessages(openaiMessages []OpenAIChatMessage) []*Message {
	result := make([]*Message, len(openaiMessages))
	for i, msg := range openaiMessages {
		result[i] = &Message{
			ID:      "", // ID will need to be generated or preserved separately
			Role:    msg.Role,
			Content: msg.Content,
			Extra:   make(map[string]interface{}),
		}
		if msg.Name != "" {
			result[i].Extra["name"] = msg.Name
		}
	}
	return result
}
