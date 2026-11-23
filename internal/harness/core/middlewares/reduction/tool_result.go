package reduction

import (
	"fmt"
	"unicode/utf8"

	"ragflow/internal/harness/core/schema"
	"ragflow/internal/utility"
)

// TruncateToolResult truncates a tool result to the given max length.
// The truncation is applied at a rune boundary to avoid splitting UTF-8.
func TruncateToolResult(result string, maxLen int) string {
	if maxLen <= 0 || len(result) <= maxLen {
		return result
	}
	// Truncate at a rune boundary ([:maxLen] may split multi-byte chars).
	if maxLen > utf8.RuneCountInString(result) {
		return result
	}
	truncated := utility.TruncateRunes(result, maxLen)
	return fmt.Sprintf("%s\n...(truncated %d bytes)", truncated, len(result)-len(truncated))
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
