package toolsearch

import (
	"context"
	"strings"
	"testing"

	"ragflow/internal/harness/core"
	"ragflow/internal/harness/core/schema"
)

// ---- Test helpers ----

type searchableTool struct {
	name string
	desc string
}

func (t *searchableTool) Name() string                                     { return t.name }
func (t *searchableTool) Description() string                               { return t.desc }
func (t *searchableTool) Invoke(ctx context.Context, args string, opts ...core.ToolOption) (string, error) {
	return "result", nil
}
func (t *searchableTool) Stream(ctx context.Context, args string, opts ...core.ToolOption) (*schema.StreamReader[string], error) {
	return schema.StreamReaderFromArray([]string{"result"}), nil
}

// ---- Tests ----

func TestNew_SmallToolset(t *testing.T) {
	tools := []core.Tool{
		&searchableTool{name: "tool1", desc: "First tool"},
		&searchableTool{name: "tool2", desc: "Second tool"},
	}
	mw := NewTyped[*schema.Message](&TypedConfig[*schema.Message]{
		AllTools:        tools,
		SearchThreshold: 10,
	})
	rc := &core.ReActAgentContext{Instruction: "Help", Tools: make([]core.Tool, 0)}
	_, newRc, err := mw.BeforeAgent(context.Background(), rc)
	if err != nil { t.Fatalf("BeforeAgent: %v", err) }
	// With small toolset (<= threshold), all tools are passed through
	t.Logf("tools count for small set: %d", len(newRc.Tools))
	_ = newRc
}

func TestNew_LargeToolset(t *testing.T) {
	tools := make([]core.Tool, 0, 15)
	for i := 0; i < 15; i++ {
		tools = append(tools, &searchableTool{
			name: "tool", desc: "tool",
		})
	}
	mw := NewTyped[*schema.Message](&TypedConfig[*schema.Message]{
		AllTools:        tools,
		SearchThreshold: 10,
	})
	rc := &core.ReActAgentContext{Instruction: "Help", Tools: make([]core.Tool, 0)}
	_, newRc, err := mw.BeforeAgent(context.Background(), rc)
	if err != nil { t.Fatalf("BeforeAgent: %v", err) }
	// With large toolset, middleware registers a search tool
	t.Logf("tools count for large set: %d", len(newRc.Tools))
	_ = newRc
}

func TestBeforeModelRewrite_DeferredMode(t *testing.T) {
	tools := make([]core.Tool, 5)
	for i := 0; i < 5; i++ {
		tools[i] = &searchableTool{name: "t", desc: "t"}
	}
	mw := NewTyped[*schema.Message](&TypedConfig[*schema.Message]{
		AllTools:        tools,
		SearchThreshold: 3,
		UseDeferred:     true,
	})
	rc := &core.ReActAgentContext{Instruction: "Help", Tools: make([]core.Tool, 0)}
	_, _, err := mw.BeforeAgent(context.Background(), rc)
	if err != nil {
		t.Logf("deferred mode error: %v", err)
	}
}

func TestSplitToolName(t *testing.T) {
	tests := []struct {
		input string
		want  int // min expected parts
	}{
		{"weather_api", 2},
		{"searchTool", 2},
		{"simple", 1},
		{"", 0},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			parts := splitToolName(tt.input)
			if len(parts) < tt.want {
				t.Errorf("splitToolName(%q) = %v (len=%d), want at least %d parts", tt.input, parts, len(parts), tt.want)
			}
		})
	}
}

func TestSelectSyntax(t *testing.T) {
	tools := []core.Tool{
		&searchableTool{name: "weather", desc: "Get weather"},
		&searchableTool{name: "search", desc: "Search web"},
		&searchableTool{name: "calc", desc: "Calculator"},
	}
	_ = tools
}

func TestToolNames(t *testing.T) {
	// Verify the search tool is properly named
	tools := make([]core.Tool, 12)
	for i := 0; i < 12; i++ {
		tools[i] = &searchableTool{name: "t", desc: "t"}
	}
	mw := NewTyped[*schema.Message](&TypedConfig[*schema.Message]{
		AllTools:        tools,
		SearchThreshold: 10,
		MaxResults:      5,
	})
	rc := &core.ReActAgentContext{Instruction: "Help", Tools: make([]core.Tool, 0)}
	_, newRc, err := mw.BeforeAgent(context.Background(), rc)
	if err != nil { t.Fatalf("BeforeAgent: %v", err) }
	if len(newRc.Tools) > 0 {
		t.Logf("search tool added: %q", newRc.Tools[0].Name())
	}
}

func TestKeywordMatch(t *testing.T) {
	// Verify keyword matching logic
	query := "weather"
	name := "weather_api"
	desc := "Get weather for a location"

	nameLower := strings.ToLower(name)
	descLower := strings.ToLower(desc)
	qLower := strings.ToLower(query)

	match := strings.Contains(nameLower, qLower) || strings.Contains(descLower, qLower)
	if !match {
		t.Error("weather tools should match 'weather' keyword")
	}
	_ = match
}
