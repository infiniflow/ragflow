package backend

import "fmt"

// LargeToolResult handles cases where tool results exceed context window limits.
type LargeToolResult struct {
	Size    int
	Content string
}

func NewLargeToolResult(content string, maxSize int) *LargeToolResult {
	if len(content) > maxSize {
		return &LargeToolResult{Size: len(content), Content: content[:maxSize] + "\n...(truncated)"}
	}
	return &LargeToolResult{Size: len(content), Content: content}
}

func (r *LargeToolResult) String() string {
	return fmt.Sprintf("[Tool Result: %d bytes]\n%s", r.Size, r.Content)
}
