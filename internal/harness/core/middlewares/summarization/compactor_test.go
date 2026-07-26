package summarization

import (
	"strings"
	"testing"

	"ragflow/internal/harness/core/schema"
)

// ======================== Compactor Tests ========================

func TestCompactor_ShouldCompact_BelowThreshold(t *testing.T) {
	c := &Compactor{}
	msgs := []*schema.Message{
		{Role: schema.RoleUser, Content: "hi"},
		{Role: schema.RoleAssistant, Content: "hello"},
	}
	cfg := &CompactionConfig{TriggerTokens: 10000}
	if c.ShouldCompact(msgs, cfg) {
		t.Error("should not compact below threshold")
	}
}

func TestCompactor_ShouldCompact_AboveThreshold(t *testing.T) {
	c := &Compactor{}
	msgs := make([]*schema.Message, 20)
	for i := range msgs {
		msgs[i] = &schema.Message{Role: schema.RoleUser, Content: "a message that is long enough to trigger compaction when there are many of them"}
	}
	cfg := &CompactionConfig{TriggerTokens: 10, PreserveRecent: 4}
	if !c.ShouldCompact(msgs, cfg) {
		t.Error("should compact above threshold")
	}
}

func TestFindSafeSplit_NoToolPairs(t *testing.T) {
	msgs := []*schema.Message{
		{Role: schema.RoleUser, Content: "a"},
		{Role: schema.RoleAssistant, Content: "b"},
		{Role: schema.RoleUser, Content: "c"},
	}
	idx := findSafeSplit(msgs, 2)
	if idx != 2 {
		t.Errorf("expected split at 2, got %d", idx)
	}
}

func TestFindSafeSplit_SkipsToolResult(t *testing.T) {
	msgs := []*schema.Message{
		{Role: schema.RoleUser, Content: "a"},
		{Role: schema.RoleAssistant, Content: "tool call", ToolCalls: []schema.ToolCall{{ID: "tc1"}}},
		{Role: schema.RoleTool, Name: "tc1", Content: "tool result"},
		{Role: schema.RoleUser, Content: "b"},
	}
	idx := findSafeSplit(msgs, 3)
	if idx != 1 {
		t.Errorf("expected split at 1 (skip ToolUse+ToolResult), got %d", idx)
	}
}

func TestCompactor_Compact_Basic(t *testing.T) {
	c := &Compactor{}
	msgs := make([]*schema.Message, 15)
	for i := range msgs {
		role := schema.RoleUser
		if i%2 == 1 {
			role = schema.RoleAssistant
		}
		msgs[i] = &schema.Message{Role: role, Content: "msg"}
	}

	cfg := &CompactionConfig{TriggerTokens: 10, PreserveRecent: 4}
	result, err := c.Compact(msgs, cfg, nil)
	if err != nil {
		t.Fatalf("Compact: %v", err)
	}
	if len(result) < 2 {
		t.Errorf("expected at least 2 messages (summary + preserved), got %d", len(result))
	}
	t.Logf("compact: %d msgs → %d msgs", len(msgs), len(result))
}

// ======================== Summary Compression Tests ========================

func TestCompressSummary_Budget(t *testing.T) {
	text := `Summary: test conversation
Scope: testing
This is a very long line that should be compressed to fit within the budget
- list item 1
- list item 2
extra detail line`

	budget := &SummaryBudget{MaxChars: 200, MaxLines: 5, MaxLineLen: 100}
	result := CompressSummary(text, budget)
	if result == "" {
		t.Error("expected non-empty compressed summary")
	}
	t.Logf("compressed: %d chars", len(result))
}

func TestCompressSummary_PriorityPreserved(t *testing.T) {
	text := `- low priority item
Summary: core structural line
- another low item`

	result := CompressSummary(text, &SummaryBudget{MaxChars: 500, MaxLines: 10, MaxLineLen: 200})
	if !strings.Contains(result, "Summary:") {
		t.Error("expected 'Summary:' line to be preserved (highest priority)")
	}
	t.Logf("compressed: %s", result)
}

func TestClassifyLinePriority(t *testing.T) {
	tests := []struct {
		line     string
		expected int
	}{
		{"Summary: core", 0},
		{"conversation summary: main", 0},
		{"scope: project", 0},
		{"current work: fixing", 0},
		{"Section Header:", 1},
		{"- list item", 2},
		{"  - nested item", 2},
		{"random detail line", 3},
	}
	for _, tc := range tests {
		got := classifyLinePriority(tc.line)
		if got != tc.expected {
			t.Errorf("classifyLinePriority(%q) = %d, want %d", tc.line, got, tc.expected)
		}
	}
}

// ======================== Supersede Tests ========================

func TestAnalyzeFileOps_NoSupersede(t *testing.T) {
	msgs := []*schema.Message{
		{Role: schema.RoleAssistant, Content: "I'll read the file"},
		{Role: schema.RoleTool, Name: "read_file", Content: "/src/main.go: content"},
	}
	result := AnalyzeFileOps(msgs)
	if result.RemovedCount != 0 {
		t.Errorf("expected 0 removed, got %d", result.RemovedCount)
	}
}

func TestAnalyzeFileOps_ReadSupersededByWrite(t *testing.T) {
	msgs := []*schema.Message{
		{Role: schema.RoleAssistant, Content: "read file"},
		{Role: schema.RoleTool, Name: "read_file", Content: "/src/main.go: old content"},
		{Role: schema.RoleAssistant, Content: "write file"},
		{Role: schema.RoleTool, Name: "write_file", Content: "/src/main.go: new content"},
	}
	result := AnalyzeFileOps(msgs)
	if result.RemovedCount != 1 {
		t.Errorf("expected 1 removed (read superseded by write), got %d", result.RemovedCount)
	}
}

func TestApplySupersede(t *testing.T) {
	msgs := []*schema.Message{
		{Role: schema.RoleAssistant, Content: "read"},
		{Role: schema.RoleTool, Name: "read_file", Content: "/a.go: old"},
		{Role: schema.RoleAssistant, Content: "write"},
		{Role: schema.RoleTool, Name: "write_file", Content: "/a.go: new"},
	}
	filtered := ApplySupersede(msgs)
	if len(filtered) != 3 {
		t.Errorf("expected 3 messages after supersede (removed 1), got %d", len(filtered))
	}
}

func TestClassifyOpType(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		expected string
	}{
		{"write", "write_file", "write"},
		{"edit", "edit_file", "edit"},
		{"read", "read_file", "read"},
		{"search", "grep_search", "search"},
		{"glob", "glob_search", "search"},
	}
	for _, tc := range tests {
		got := classifyOpType("", tc.toolName)
		if got != tc.expected {
			t.Errorf("classifyOpType(%q) = %s, want %s", tc.toolName, got, tc.expected)
		}
	}
}
