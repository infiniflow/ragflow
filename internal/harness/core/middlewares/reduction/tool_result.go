package reduction

import (
	"fmt"

	"ragflow/internal/harness/core/schema"
)

// TruncateToolResult truncates a tool result to the given max length.
func TruncateToolResult(result string, maxLen int) string {
	if maxLen <= 0 || len(result) <= maxLen {
		return result
	}
	return fmt.Sprintf("%s\n...(truncated %d bytes)", result[:maxLen], len(result)-maxLen)
}

// LastToolResult finds the last tool result message for a given tool name.
func LastToolResult(msgs []*schema.Message, toolName string) *schema.Message {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == schema.RoleTool && msgs[i].Name == toolName {
			return msgs[i]
		}
	}
	return nil
}
