// Package patchtoolcalls patches incomplete tool calls in conversation history.
// When the model's tool call was interrupted or cut off, this middleware
// inserts placeholder tool messages so the conversation remains consistent.
package patchtoolcalls

import (
	"context"
	"fmt"

	"ragflow/internal/harness/agentcore"
	"ragflow/internal/harness/agentcore/schema"
)

// PatchedContentGenerator generates the content for a placeholder tool message.
type PatchedContentGenerator func(toolName, toolCallID string) string

// Config configures the patchtoolcalls middleware.
type Config struct {
	// PatchedContent overrides the default patch message content.
	PatchedContent PatchedContentGenerator
	// Language for default messages: "en" or "zh"
	Language string
}

type middleware[M agentcore.MessageType] struct {
	agentcore.BaseMiddleware[M]
	cfg *Config
}

func defaultPatchContent(toolName, toolCallID string) string {
	return fmt.Sprintf("[Tool call was not completed: %s(%s)]", toolName, toolCallID)
}

func zhPatchContent(toolName, toolCallID string) string {
	return fmt.Sprintf("[工具调用未完成: %s(%s)]", toolName, toolCallID)
}

func getPatchContent(cfg *Config, toolName, toolCallID string) string {
	if cfg.PatchedContent != nil {
		return cfg.PatchedContent(toolName, toolCallID)
	}
	if cfg.Language == "zh" {
		return zhPatchContent(toolName, toolCallID)
	}
	return defaultPatchContent(toolName, toolCallID)
}

func New[M agentcore.MessageType](cfg *Config) agentcore.TypedReActMiddleware[M] {
	if cfg == nil {
		cfg = &Config{}
	}
	return &middleware[M]{cfg: cfg}
}

func (m *middleware[M]) BeforeModelRewrite(ctx context.Context, state *agentcore.TypedReActAgentState[M], mc *agentcore.TypedModelContext[M]) (context.Context, *agentcore.TypedReActAgentState[M], error) {
	// Find assistant messages with tool calls that have no corresponding tool result
	for i := 0; i < len(state.Messages)-1; i++ {
		var toolCalls []struct{ ID, Name string }
		switch v := any(state.Messages[i]).(type) {
		case *schema.Message:
			if v.Role != schema.RoleAssistant || len(v.ToolCalls) == 0 {
				continue
			}
			// Check if next message is a tool result
			if i+1 < len(state.Messages) {
				if next, ok := any(state.Messages[i+1]).(*schema.Message); ok && next.Role == schema.RoleTool {
					continue
				}
			}
			for _, tc := range v.ToolCalls {
				toolCalls = append(toolCalls, struct{ ID, Name string }{tc.ID, tc.Function.Name})
			}
		case *schema.AgenticMessage:
			for _, b := range v.ContentBlocks {
				if b.ToolCall != nil && b.ToolCall.ID != "" {
					// Check no matching tool result exists
					if !hasCorrespondingAgenticToolResult(state.Messages, b.ToolCall.ID) {
						toolCalls = append(toolCalls, struct{ ID, Name string }{b.ToolCall.ID, b.ToolCall.Name})
					}
				}
			}
			if len(toolCalls) == 0 {
				continue
			}
		default:
			continue
		}

		if len(toolCalls) == 0 {
			continue
		}

		// Insert placeholder tool results after the assistant message
		insertAt := i + 1
		for _, tc := range toolCalls {
			patchContent := getPatchContent(m.cfg, tc.Name, tc.ID)
			placeholder := schema.ToolMessage(patchContent, tc.ID)
			state.Messages = append(state.Messages[:insertAt], append([]M{any(placeholder).(M)}, state.Messages[insertAt:]...)...)
			insertAt++
			i++
		}
	}
	return ctx, state, nil
}

func hasCorrespondingAgenticToolResult[M agentcore.MessageType](msgs []M, callID string) bool {
	for _, msg := range msgs {
		switch v := any(msg).(type) {
		case *schema.AgenticMessage:
			for _, b := range v.ContentBlocks {
				if b.ToolResult != nil && b.ToolResult.ToolCallID == callID {
					return true
				}
			}
		}
	}
	return false
}
