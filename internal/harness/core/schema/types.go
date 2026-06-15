// Package schema provides shared message and stream types for the agent harness.
package schema

import (
	"fmt"
	"io"
)

// RoleType represents the role of a message in a conversation.
type RoleType string

const (
	RoleUser      RoleType = "user"
	RoleAssistant RoleType = "assistant"
	RoleSystem    RoleType = "system"
	RoleTool      RoleType = "tool"
	RoleFunction  RoleType = "function"
)

// AgenticRoleType represents the role of an agentic message.
type AgenticRoleType string

const (
	AgenticRoleAssistant AgenticRoleType = "assistant"
	AgenticRoleUser      AgenticRoleType = "user"
	AgenticRoleSystem    AgenticRoleType = "system"
)

// ToolCallFunction represents a function call in a tool call.
type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolCall represents a call to a tool by the model.
type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type,omitempty"`
	Function ToolCallFunction `json:"function"`
}

// Message represents a conversation message with typed role and content.
type Message struct {
	Role      RoleType           `json:"role"`
	Content   string             `json:"content"`
	Name      string             `json:"name,omitempty"`
	ToolCalls []ToolCall         `json:"tool_calls,omitempty"`
	ToolName  string             `json:"tool_name,omitempty"`
	Extra     map[string]any     `json:"extra,omitempty"`
}

// ToolCallInfo represents information about a tool call for agentic messages.
type ToolCallInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolResult represents the result of a tool execution.
// Used by both standard tools and enhanced tools.
type ToolResult struct {
	ToolCallID string         `json:"tool_call_id"`
	Name       string         `json:"name"`
	Content    string         `json:"content"`
	Error      string         `json:"error,omitempty"`
	Extra      map[string]any `json:"extra,omitempty"`
}

// ContentBlock represents a structured content element within an AgenticMessage.
type ContentBlock struct {
	Type       string       `json:"type"`
	Text       string       `json:"text,omitempty"`
	ToolCall   *ToolCallInfo `json:"tool_call,omitempty"`
	ToolResult *ToolResult   `json:"tool_result,omitempty"`
}

// AgenticMessage represents an agent-oriented message with structured content blocks.
type AgenticMessage struct {
	Role          AgenticRoleType `json:"role"`
	Content       string          `json:"content"`
	ContentBlocks []ContentBlock  `json:"content_blocks,omitempty"`
}

// ToolInfo provides information about a tool to the model.
type ToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"input_schema,omitempty"`
}

// ToolChoice controls how the model uses the tools provided to it.
type ToolChoice string

const (
	// ToolChoiceForbidden instructs the model not to call any tools.
	ToolChoiceForbidden ToolChoice = "forbidden"
	// ToolChoiceAllowed lets the model decide whether to call tools.
	ToolChoiceAllowed   ToolChoice = "allowed"
	// ToolChoiceForced requires the model to call at least one tool.
	ToolChoiceForced    ToolChoice = "forced"
)

// AllowedTool specifies a tool that the model is permitted or required to call.
type AllowedTool struct {
	// FunctionName specifies a function tool by name.
	FunctionName string `json:"function_name,omitempty"`
}

// AgenticToolChoice provides fine-grained control over which tools the model may call.
type AgenticToolChoice struct {
	// Type is the tool choice mode (forbidden, allowed, forced).
	Type ToolChoice `json:"type"`
	// Allowed optionally specifies the list of tools the model may call.
	Allowed *struct {
		Tools []*AllowedTool `json:"tools,omitempty"`
	} `json:"allowed,omitempty"`
	// Forced optionally specifies the list of tools the model must call.
	Forced *struct {
		Tools []*AllowedTool `json:"tools,omitempty"`
	} `json:"forced,omitempty"`
}

// ToolArgument represents structured arguments passed to an enhanced tool invocation.
type ToolArgument struct {
	// Name is the name of the tool being invoked.
	Name string `json:"name"`

	// Arguments is the raw JSON string of arguments.
	Arguments string `json:"arguments"`

	// CallID is the unique identifier for this tool call.
	CallID string `json:"call_id,omitempty"`
}

// ---- Gob registration helpers for checkpoint/resume ----

var registeredTypes = make(map[string]func() any)

func RegisterType(name string, factory func() any) {
	registeredTypes[name] = factory
}

// RegisterName registers a concrete type for gob serialization under the given name.
// This must be called in init() for custom types stored via SetRunLocalValue,
// so they survive interrupt/resume checkpoint cycles.
func RegisterName[T any](name string) {
	RegisterType(name, func() any { var t T; return &t })
}

// StreamReader is a generic buffered stream reader.
type StreamReader[M any] struct {
	ch     chan streamFrame[M]
	closed bool
}

type streamFrame[M any] struct {
	Data M
	Err  error
}

// NewStreamReader creates a new StreamReader.
func NewStreamReader[M any]() *StreamReader[M] {
	return &StreamReader[M]{ch: make(chan streamFrame[M], 64)}
}

// Recv reads the next item, blocking until available.
func (sr *StreamReader[M]) Recv() (M, error) {
	frame, ok := <-sr.ch
	if !ok {
		var zero M
		return zero, io.EOF
	}
	return frame.Data, frame.Err
}

// Send pushes an item to the stream.
func (sr *StreamReader[M]) Send(data M, err error) {
	if sr.closed {
		return
	}
	sr.ch <- streamFrame[M]{Data: data, Err: err}
}

// Close closes the stream.
func (sr *StreamReader[M]) Close() {
	if !sr.closed {
		sr.closed = true
		close(sr.ch)
	}
}

// StreamReaderFromArray creates a stream pre-populated with items.
func StreamReaderFromArray[M any](items []M) *StreamReader[M] {
	sr := NewStreamReader[M]()
	for _, item := range items {
		sr.Send(item, nil)
	}
	sr.Close()
	return sr
}

// ConcatMessages concatenates multiple messages into one.
func ConcatMessages(msgs []*Message) (*Message, error) {
	if len(msgs) == 0 {
		return nil, fmt.Errorf("no messages to concatenate")
	}
	result := &Message{
		Role:    msgs[0].Role,
		Content: "",
		Extra:   make(map[string]any),
	}
	for _, m := range msgs {
		result.Content += m.Content
		if m.Extra != nil {
			for k, v := range m.Extra {
				result.Extra[k] = v
			}
		}
		if len(m.ToolCalls) > 0 {
			result.ToolCalls = m.ToolCalls
		}
		if m.ToolName != "" {
			result.ToolName = m.ToolName
		}
	}
	return result, nil
}

// ConcatAgenticMessages concatenates multiple agentic messages into one.
func ConcatAgenticMessages(msgs []*AgenticMessage) (*AgenticMessage, error) {
	if len(msgs) == 0 {
		return nil, fmt.Errorf("no messages to concatenate")
	}
	result := &AgenticMessage{
		Role:          msgs[0].Role,
		Content:       "",
		ContentBlocks: nil,
	}
	for _, m := range msgs {
		result.Content += m.Content
		if m.ContentBlocks != nil {
			result.ContentBlocks = append(result.ContentBlocks, m.ContentBlocks...)
		}
	}
	return result, nil
}

// ConcatMessageStream reads all items from a stream and concatenates them.
func ConcatMessageStream(sr *StreamReader[*Message]) (*Message, error) {
	defer sr.Close()
	var msgs []*Message
	for {
		m, err := sr.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return ConcatMessages(msgs)
}

// ---- Message constructors ----

func UserMessage(content string) *Message {
	return &Message{Role: RoleUser, Content: content, Extra: make(map[string]any)}
}
func AssistantMessage(content string) *Message {
	return &Message{Role: RoleAssistant, Content: content, Extra: make(map[string]any)}
}
func SystemMessage(content string) *Message {
	return &Message{Role: RoleSystem, Content: content, Extra: make(map[string]any)}
}
func ToolMessage(content, toolCallID string) *Message {
	return &Message{Role: RoleTool, Content: content, Name: toolCallID, Extra: make(map[string]any)}
}
func FunctionMessage(content, name string) *Message {
	return &Message{Role: RoleFunction, Content: content, Name: name, Extra: make(map[string]any)}
}
func UserAgenticMessage(content string) *AgenticMessage {
	return &AgenticMessage{
		Role: AgenticRoleUser, Content: content,
		ContentBlocks: []ContentBlock{{Type: "text", Text: content}},
	}
}
