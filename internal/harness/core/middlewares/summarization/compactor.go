package summarization

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"ragflow/internal/harness/core/schema"
)

// ---- Session Compactor ----

// CompactionConfig configures session compaction behavior.
type CompactionConfig struct {
	// TriggerTokens is the estimated token count that triggers compaction.
	TriggerTokens int
	// PreserveRecent is the number of recent messages to keep uncompacted.
	PreserveRecent int
	// TokenEstimator estimates token count for messages. If nil, uses simple heuristic.
	TokenEstimator func(msgs []*schema.Message) int
}

func (c *CompactionConfig) defaults() {
	if c.TriggerTokens <= 0 {
		c.TriggerTokens = 100000
	}
	if c.PreserveRecent <= 0 {
		c.PreserveRecent = 4
	}
	if c.TokenEstimator == nil {
		c.TokenEstimator = defaultTokenEstimate
	}
}

func defaultTokenEstimate(msgs []*schema.Message) int {
	total := 0
	for _, m := range msgs {
		total += len(m.Content) / 4
	}
	return total
}

// Compactor manages session compaction with structured summaries.
type Compactor struct {
	mu sync.Mutex
}

// ShouldCompact checks whether the message list exceeds the budget.
func (c *Compactor) ShouldCompact(msgs []*schema.Message, cfg *CompactionConfig) bool {
	cfg.defaults()
	if len(msgs) <= cfg.PreserveRecent {
		return false
	}
	tokens := cfg.TokenEstimator(msgs)
	return tokens >= cfg.TriggerTokens
}

// Compact compacts the message list: summarizes old messages, preserves recent ones.
func (c *Compactor) Compact(msgs []*schema.Message, cfg *CompactionConfig, summarizer func(context.Context, []*schema.Message) (string, error)) ([]*schema.Message, error) {
	cfg.defaults()
	if len(msgs) <= cfg.PreserveRecent+1 {
		return msgs, nil
	}
	split := findSafeSplit(msgs, len(msgs)-cfg.PreserveRecent)
	summarizeMsgs := msgs[:split]
	keepMsgs := msgs[split:]

	existingSummary := extractExistingSummary(summarizeMsgs)

	var summaryText string
	if summarizer != nil {
		var err error
		summaryText, err = summarizer(context.Background(), summarizeMsgs)
		if err != nil {
			summaryText = fmt.Sprintf("(%d messages compacted)", len(summarizeMsgs))
		}
	} else {
		summaryText = fmt.Sprintf("(%d messages compacted)", len(summarizeMsgs))
	}
	if existingSummary != "" {
		summaryText = mergeSummaries(existingSummary, summaryText)
	}

	content := fmt.Sprintf("<summary>\n%s\n</summary>", summaryText)
	result := make([]*schema.Message, 0, 1+len(keepMsgs))
	result = append(result, &schema.Message{Role: schema.RoleSystem, Content: content})
	result = append(result, keepMsgs...)
	return result, nil
}

// findSafeSplit finds a split index that doesn't break ToolUse/ToolResult pairs.
func findSafeSplit(msgs []*schema.Message, desired int) int {
	if desired >= len(msgs) {
		return len(msgs)
	}
	for i := desired; i > 0; i-- {
		msg := msgs[i-1]
		if msg.Role == schema.RoleTool {
			continue
		}
		if msg.Role == schema.RoleAssistant && len(msg.ToolCalls) > 0 {
			continue
		}
		return i
	}
	return desired
}

func extractExistingSummary(msgs []*schema.Message) string {
	for _, m := range msgs {
		if m.Role == schema.RoleSystem && strings.Contains(m.Content, "<summary>") {
			start := strings.Index(m.Content, "<summary>")
			end := strings.LastIndex(m.Content, "</summary>")
			if start >= 0 && end > start {
				return strings.TrimSpace(m.Content[start+9 : end])
			}
		}
	}
	return ""
}

func mergeSummaries(existing, newSummary string) string {
	return fmt.Sprintf("Previous summary:\n%s\n\nNewly compacted context:\n%s", existing, newSummary)
}

// ---- Priority-based Summary Compression ----

// SummaryBudget defines the budget for compressed summaries.
type SummaryBudget struct {
	MaxChars   int
	MaxLines   int
	MaxLineLen int
}

func (b *SummaryBudget) defaults() {
	if b.MaxChars <= 0 {
		b.MaxChars = 1200
	}
	if b.MaxLines <= 0 {
		b.MaxLines = 24
	}
	if b.MaxLineLen <= 0 {
		b.MaxLineLen = 160
	}
}

// SummaryLine carries a parsed line and its priority.
type SummaryLine struct {
	Text     string
	Priority int
}

// CompressSummary compresses a summary text within the given budget.
func CompressSummary(text string, budget *SummaryBudget) string {
	budget.defaults()
	lines := strings.Split(text, "\n")

	var scored []SummaryLine
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if len(trimmed) > budget.MaxLineLen {
			trimmed = trimAtWord(trimmed[:budget.MaxLineLen]) + "..."
		}
		priority := classifyLinePriority(trimmed)
		scored = append(scored, SummaryLine{Text: trimmed, Priority: priority})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		return scored[i].Priority < scored[j].Priority
	})

	var result []string
	charCount := 0
	for _, sl := range scored {
		if len(result) >= budget.MaxLines {
			break
		}
		if charCount+len(sl.Text)+1 > budget.MaxChars {
			continue
		}
		result = append(result, sl.Text)
		charCount += len(sl.Text) + 1
	}
	return strings.Join(result, "\n")
}

func classifyLinePriority(line string) int {
	lower := strings.ToLower(strings.TrimSpace(line))
	corePrefixes := []string{
		"summary:", "conversation summary:", "scope:", "current work:", "pending work:",
		"key files:", "tools mentioned:", "key timeline:", "newly compacted context:",
	}
	for _, p := range corePrefixes {
		if strings.HasPrefix(lower, p) {
			return 0
		}
	}
	if strings.HasSuffix(line, ":") {
		return 1
	}
	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "  - ") {
		return 2
	}
	return 3
}

func trimAtWord(s string) string {
	lastSpace := strings.LastIndex(s, " ")
	if lastSpace > 0 {
		return s[:lastSpace]
	}
	return s
}

// ---- Supersede ----

// SupersedeResult contains the result of Supersede analysis.
type SupersedeResult struct {
	RemoveIndices []int
	RemovedCount  int
}

// AnalyzeFileOps tracks file operations and finds read ops superseded by later writes.
func AnalyzeFileOps(msgs []*schema.Message) *SupersedeResult {
	type fileOp struct {
		path   string
		opType string
		index  int
	}
	var ops []fileOp
	for i, m := range msgs {
		if m.Role != schema.RoleTool {
			continue
		}
		path := extractFilePath(m.Content)
		if path == "" {
			continue
		}
		opType := classifyOpType(m.Content, m.Name)
		ops = append(ops, fileOp{path: path, opType: opType, index: i})
	}

	byPath := make(map[string][]fileOp)
	for _, op := range ops {
		byPath[op.path] = append(byPath[op.path], op)
	}

	var remove []int
	for _, pathOps := range byPath {
		if len(pathOps) < 2 {
			continue
		}
		lastWrite := -1
		for j := len(pathOps) - 1; j >= 0; j-- {
			if pathOps[j].opType == "write" || pathOps[j].opType == "edit" {
				lastWrite = j
				break
			}
		}
		if lastWrite < 0 {
			continue
		}
		for _, op := range pathOps[:lastWrite] {
			if op.opType == "read" {
				remove = append(remove, op.index)
			}
		}
	}
	sort.Ints(remove)
	return &SupersedeResult{RemoveIndices: remove, RemovedCount: len(remove)}
}

// ApplySupersede removes superseded file operations from the message list.
func ApplySupersede(msgs []*schema.Message) []*schema.Message {
	result := AnalyzeFileOps(msgs)
	if result.RemovedCount == 0 {
		return msgs
	}
	removeSet := make(map[int]bool, len(result.RemoveIndices))
	for _, idx := range result.RemoveIndices {
		removeSet[idx] = true
	}
	filtered := make([]*schema.Message, 0, len(msgs)-result.RemovedCount)
	for i, m := range msgs {
		if !removeSet[i] {
			filtered = append(filtered, m)
		}
	}
	return filtered
}

func extractFilePath(content string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "/") || strings.HasPrefix(line, "./") ||
			strings.HasPrefix(line, "../") || strings.Contains(line, "/") {
			parts := strings.Fields(line)
			for _, p := range parts {
				if strings.Contains(p, ".") || strings.Contains(p, "/") {
					return p
				}
			}
		}
	}
	return ""
}

func classifyOpType(content, toolName string) string {
	lower := strings.ToLower(toolName)
	if strings.Contains(lower, "write") || strings.Contains(lower, "create") {
		return "write"
	}
	if strings.Contains(lower, "edit") || strings.Contains(lower, "patch") {
		return "edit"
	}
	if strings.Contains(lower, "read") || strings.Contains(lower, "view") || strings.Contains(lower, "cat") {
		return "read"
	}
	if strings.Contains(lower, "search") || strings.Contains(lower, "grep") || strings.Contains(lower, "glob") {
		return "search"
	}
	return "read"
}
