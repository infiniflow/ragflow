// Package reduction provides tool output reduction middleware.
// Two-phase design: Truncation (immediate) -> Clear (before model rewrite).
package reduction

import (
	"context"
	"sync"

	"ragflow/internal/harness/core"
	"ragflow/internal/harness/core/schema"
)

// Backend persists overflow content.
type Backend interface {
	Store(key string, content string) error
	Load(key string) (string, error)
}

// ToolConfig provides per-tool reduction configuration.
type ToolConfig struct {
	SkipTruncation bool
	SkipClear      bool
}

// TypedConfig configures the reduction middleware.
type TypedConfig[M core.MessageType] struct {
	Backend           Backend
	MaxToolOutputLen  int // Truncate tool output beyond this (0 = no truncation)
	MaxToolCalls      int // Clear tool calls beyond this (0 = no clear)
	MaxTokensForClear int // Trigger clear when total tokens exceed this
	ClearAtLeast      int // Ensure at least this many tokens are freed per clear
	ToolConfigs       map[string]*ToolConfig
	ExcludeTools      map[string]bool
}

type Config = TypedConfig[*schema.Message]

type middleware[M core.MessageType] struct {
	core.BaseMiddleware[M]
	cfg *TypedConfig[M]
	mu  sync.Mutex
	keyCounter int
}

func NewTyped[M core.MessageType](cfg *TypedConfig[M]) core.TypedReActMiddleware[M] {
	if cfg == nil { cfg = &TypedConfig[M]{} }
	if cfg.MaxToolOutputLen <= 0 { cfg.MaxToolOutputLen = 2000 }
	if cfg.MaxToolCalls <= 0 { cfg.MaxToolCalls = 20 }
	if cfg.MaxTokensForClear <= 0 { cfg.MaxTokensForClear = 100000 }
	return &middleware[M]{cfg: cfg}
}

func New(cfg *Config) core.TypedReActMiddleware[*schema.Message] {
	return NewTyped[*schema.Message](cfg)
}


// ---- Clear Phase (BeforeModelRewrite) ----

func (mw *middleware[M]) BeforeModelRewrite(ctx context.Context, state *core.TypedReActAgentState[M], mc *core.TypedModelContext[M]) (context.Context, *core.TypedReActAgentState[M], error) {
	// Phase 1: Truncate oversized outputs
	mw.truncateToolOutputs(state)

	// Phase 2: Clear old tool calls if total tokens exceed threshold
	if mw.cfg.MaxTokensForClear > 0 {
		totalTokens := mw.estimateTokens(state.Messages)
		if totalTokens > mw.cfg.MaxTokensForClear {
			mw.clearOldToolCalls(state, totalTokens)
		}
	}

	return ctx, state, nil
}

func (mw *middleware[M]) truncateToolOutputs(state *core.TypedReActAgentState[M]) {
	toolCount := 0
	for i, msg := range state.Messages {
		m, ok := any(msg).(*schema.Message)
		if !ok || m == nil || m.Role != schema.RoleTool { continue }
		toolCount++
		if mw.cfg.MaxToolCalls > 0 && toolCount > mw.cfg.MaxToolCalls {
			m.Content = "..."
			m.Extra = nil
			state.Messages[i] = any(m).(M)
			continue
		}
		if mw.cfg.MaxToolOutputLen > 0 && len(m.Content) > mw.cfg.MaxToolOutputLen {
			if !mw.isExcluded(m.ToolName) {
				m.Content = m.Content[:mw.cfg.MaxToolOutputLen] + "\n...(truncated)"
				state.Messages[i] = any(m).(M)
			}
		}
	}
}

func (mw *middleware[M]) clearOldToolCalls(state *core.TypedReActAgentState[M], totalTokens int) {
	if mw.cfg.ClearAtLeast <= 0 { return }
	targetTokens := mw.cfg.MaxTokensForClear - mw.cfg.ClearAtLeast
	if totalTokens <= targetTokens { return }

	freed := 0
	toolCount := 0
	for i, msg := range state.Messages {
		m, ok := any(msg).(*schema.Message)
		if !ok || m == nil || m.Role != schema.RoleTool { continue }
		toolCount++
		if mw.cfg.MaxToolCalls > 0 && toolCount > mw.cfg.MaxToolCalls {
			before := len([]rune(m.Content))
			m.Content = "..."
			freed += before - 3
			state.Messages[i] = any(m).(M)
			if totalTokens-freed <= targetTokens { break }
		}
	}
}

func (mw *middleware[M]) estimateTokens(msgs []M) int {
	total := 0
	for _, msg := range msgs {
		switch v := any(msg).(type) {
		case *schema.Message:
			total += len([]rune(v.Content)) * 4 / 3
			for _, tc := range v.ToolCalls {
				total += len([]rune(tc.Function.Arguments)) * 4 / 3
			}
		case *schema.AgenticMessage:
			total += len([]rune(v.Content)) * 4 / 3
		}
	}
	return total
}

func (mw *middleware[M]) isExcluded(name string) bool {
	if mw.cfg.ExcludeTools == nil { return false }
	return mw.cfg.ExcludeTools[name]
}

func (mw *middleware[M]) nextKey() int {
	mw.mu.Lock()
	defer mw.mu.Unlock()
	mw.keyCounter++
	return mw.keyCounter
}

func truncateText(s string, maxLen int) string {
	if len(s) <= maxLen { return s }
	return s[:maxLen]
}
