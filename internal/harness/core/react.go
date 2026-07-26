package core

import "ragflow/internal/harness/core/schema"

// TypedReActAgentState is the exported state type for ReActAgent middlewares.
type TypedReActAgentState[M MessageType] struct {
	Messages            []M
	ToolInfos           []*schema.ToolInfo
	DeferredToolInfos   []*schema.ToolInfo
	Extra               map[string]any
	RemainingIterations int
}

type ReActAgentState = TypedReActAgentState[*schema.Message]

func NewReActAgentState[M MessageType](msgs []M, tools []*schema.ToolInfo, maxIter int) *TypedReActAgentState[M] {
	return &TypedReActAgentState[M]{
		Messages: msgs, ToolInfos: tools,
		RemainingIterations: maxIter, Extra: make(map[string]any),
	}
}

// ReActAgentContext is passed to BeforeAgent middlewares.
type ReActAgentContext struct {
	Instruction    string
	Tools          []Tool
	ReturnDirectly map[string]bool
	ToolSearchTool *schema.ToolInfo
}

// ToolContext provides metadata about a tool being wrapped.
type ToolContext struct {
	Name   string
	CallID string
}

// ToolCallsContext contains metadata about completed tool calls.
type ToolCallsContext struct {
	ToolCalls []ToolContext
}
