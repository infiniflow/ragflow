package reduction

import (
	"context"
	"strings"

	"ragflow/internal/harness/core"
	"ragflow/internal/harness/core/schema"
)

// ClearConfig configures the clear functionality.
type ClearConfig struct {
	// ExcludeTools lists tool names whose results should NOT be cleared.
	ExcludeTools []string
}

// ClearOldToolResults removes old tool call messages from state before model rewrite.
// This prevents the context window from being filled with stale tool results.
func ClearOldToolResults[M core.MessageType](ctx context.Context, state *core.TypedReActAgentState[M], exclude []string) *core.TypedReActAgentState[M] {
	if state == nil || len(state.Messages) == 0 {
		return state
	}
	cleaned := make([]M, 0, len(state.Messages))
	keepCount := 0
	for _, msg := range state.Messages {
		switch v := any(msg).(type) {
		case *schema.Message:
			if v.Role == schema.RoleTool && !isExcluded(v.Name, exclude) {
				if keepToolCall(cleaned, v) {
					cleaned = append(cleaned, msg)
				}
				continue
			}
		}
		cleaned = append(cleaned, msg)
		keepCount++
	}
	state.Messages = cleaned
	return state
}

func isExcluded(name string, exclude []string) bool {
	if name == "" {
		return false
	}
	for _, e := range exclude {
		if strings.EqualFold(name, e) {
			return true
		}
	}
	return false
}

// keepToolCall returns true if this tool result should be kept (most recent tool result per name).
func keepToolCall[M core.MessageType](existing []M, newMsg *schema.Message) bool {
	for i := len(existing) - 1; i >= 0; i-- {
		switch v := any(existing[i]).(type) {
		case *schema.Message:
			if v.Role == schema.RoleTool && v.Name == newMsg.Name {
				return false
			}
		}
	}
	return true
}
