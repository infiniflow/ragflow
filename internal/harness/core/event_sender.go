package core

import (
	"context"

	"ragflow/internal/harness/core/schema"
)

// ---- NewEventSenderModelWrapper creates a handler that sends model output events.
// Place this in the Handlers chain to control WHERE events are emitted:
// - Innermost position (last in Handlers list): events contain original (unmodified) model output
// - Outermost position (first in Handlers list): events contain fully processed output
//
// When detected in Handlers, the framework skips its built-in event sender to avoid duplicates.
func NewEventSenderModelWrapper[M MessageType]() *eventSenderModelHandler[M] {
	return &eventSenderModelHandler[M]{}
}

type eventSenderModelHandler[M MessageType] struct{}

func (h *eventSenderModelHandler[M]) WrapModel(ctx context.Context, m Model[M], mc *TypedModelContext[M]) (Model[M], error) {
	ec := getReActExecCtx[M](ctx)
	if ec == nil { return m, nil }
	return wrapModelWithEventSender(m, ec), nil
}

// All other middleware methods are no-op
func (h *eventSenderModelHandler[M]) BeforeAgent(ctx context.Context, rc *ReActAgentContext) (context.Context, *ReActAgentContext, error) { return ctx, rc, nil }
func (h *eventSenderModelHandler[M]) AfterAgent(ctx context.Context, state *TypedReActAgentState[M]) (context.Context, error) { return ctx, nil }
func (h *eventSenderModelHandler[M]) BeforeModelRewrite(ctx context.Context, state *TypedReActAgentState[M], mc *TypedModelContext[M]) (context.Context, *TypedReActAgentState[M], error) { return ctx, state, nil }
func (h *eventSenderModelHandler[M]) AfterModelRewrite(ctx context.Context, state *TypedReActAgentState[M], mc *TypedModelContext[M]) (context.Context, *TypedReActAgentState[M], error) { return ctx, state, nil }

// HasUserEventSenderModelWrapper checks if the handlers list contains a user-provided
// NewEventSenderModelWrapper. When present, the framework skips its internal default
// model event sender to avoid duplicate events.
func HasUserEventSenderModelWrapper[M MessageType](handlers []TypedReActMiddleware[M]) bool {
	for _, h := range handlers {
		if _, ok := h.(*eventSenderModelHandler[M]); ok {
			return true
		}
	}
	return false
}

// ---- Tool event constructors ----

// TypedToolInvokeEvent creates an event for a synchronous tool result.
func TypedToolInvokeEvent(result string, tc *ToolContext) *TypedAgentEvent[*schema.Message] {
	msg := schema.ToolMessage(result, tc.CallID)
	return typedEventFromMessage(msg, nil, schema.RoleTool, tc.Name)
}

// TypedToolStreamEvent creates an event for a streaming tool result.
func TypedToolStreamEvent(resultChunks []string, tc *ToolContext) *TypedAgentEvent[*schema.Message] {
	content := ""
	for _, ch := range resultChunks {
		content += ch
	}
	msg := schema.ToolMessage(content, tc.CallID)
	return typedEventFromMessage(msg, nil, schema.RoleTool, tc.Name)
}

// TypedEnhancedToolInvokeEvent creates an event for an enhanced tool result.
// Propagates Extra metadata for multimodal support.
func TypedEnhancedToolInvokeEvent(result *schema.ToolResult, tc *ToolContext) *TypedAgentEvent[*schema.Message] {
	content := result.Content
	if content == "" {
		content = result.Error
	}
	msg := schema.ToolMessage(content, tc.CallID)
	msg.Name = tc.Name
	if result.Extra != nil {
		if msg.Extra == nil {
			msg.Extra = make(map[string]any, len(result.Extra))
		}
		for k, v := range result.Extra {
			msg.Extra[k] = v
		}
	}
	return typedEventFromMessage(msg, nil, schema.RoleTool, tc.Name)
}

// TypedEnhancedToolStreamEvent creates an event for a streaming enhanced tool result.
// Propagates the last result's Extra metadata.
func TypedEnhancedToolStreamEvent(results []*schema.ToolResult, tc *ToolContext) *TypedAgentEvent[*schema.Message] {
	if len(results) == 0 {
		return nil
	}
	last := results[len(results)-1]
	content := last.Content
	if content == "" {
		content = last.Error
	}
	msg := schema.ToolMessage(content, tc.CallID)
	msg.Name = tc.Name
	if last.Extra != nil {
		if msg.Extra == nil {
			msg.Extra = make(map[string]any, len(last.Extra))
		}
		for k, v := range last.Extra {
			msg.Extra[k] = v
		}
	}
	return typedEventFromMessage(msg, nil, schema.RoleTool, tc.Name)
}
