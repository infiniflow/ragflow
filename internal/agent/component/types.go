// Package component — shared types for the canvas component layer.
//
// Replaces schema.Message with a minimal local type so the
// component package has zero external dependencies beyond stdlib
// and the RAGFlow model layer.
package component

// ComponentMessage is a message type used within the component
// package. It replaces eino's schema.Message.
type ComponentMessage struct {
	Role             string
	Content          string
	ReasoningContent string // Thinking/reasoning content from the model
	ToolCalls        []ComponentToolCall
	ToolCallID       string // Required for role="tool" messages — matches the tool call ID
	MultiContent     []ComponentMessagePart
}

// ComponentToolCall represents a tool invocation within a message.
type ComponentToolCall struct {
	ID       string
	Type     string
	Function ComponentFunctionCall
}

// ComponentFunctionCall represents the function details of a tool call.
type ComponentFunctionCall struct {
	Name      string
	Arguments string
}

// ComponentMessagePart represents a single multi-modal content part.
type ComponentMessagePart struct {
	Type     string
	Text     string
	ImageURL string
}

// Standard role constants (mirror schema.RoleType values).
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// NewSystemMessage creates a system message.
func NewSystemMessage(content string) ComponentMessage {
	return ComponentMessage{Role: RoleSystem, Content: content}
}

// NewUserMessage creates a user message.
func NewUserMessage(content string) ComponentMessage {
	return ComponentMessage{Role: RoleUser, Content: content}
}
