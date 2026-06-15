// Package toolsearch provides dynamic tool search middleware.
// Instead of passing all tools to the model, agents can search for tools
// by keyword using a meta-tool, suitable for large tool libraries.
package toolsearch

import (
	"context"
	"strings"

	"ragflow/internal/harness/agentcore"
	"ragflow/internal/harness/agentcore/schema"
)

// TypedConfig configures the toolsearch middleware.
type TypedConfig[M agentcore.MessageType] struct {
	AllTools        []agentcore.Tool
	MaxResults      int
	SearchThreshold int // Pass all directly if <= threshold; otherwise use search
	UseDeferred     bool // Use DeferredToolInfos for model-native search
}

type middleware[M agentcore.MessageType] struct {
	agentcore.BaseMiddleware[M]
	cfg *TypedConfig[M]
	initialized bool
}

func NewTyped[M agentcore.MessageType](cfg *TypedConfig[M]) agentcore.TypedReActMiddleware[M] {
	if cfg == nil { cfg = &TypedConfig[M]{MaxResults: 5, SearchThreshold: 10} }
	if cfg.MaxResults <= 0 { cfg.MaxResults = 5 }
	if cfg.SearchThreshold <= 0 { cfg.SearchThreshold = 10 }
	return &middleware[M]{cfg: cfg}
}

func New(cfg *TypedConfig[*schema.Message]) agentcore.TypedReActMiddleware[*schema.Message] {
	return NewTyped[*schema.Message](cfg)
}

func (m *middleware[M]) BeforeAgent(ctx context.Context, rc *agentcore.ReActAgentContext) (context.Context, *agentcore.ReActAgentContext, error) {
	if m.initialized { return ctx, rc, nil }
	m.initialized = true

	if len(m.cfg.AllTools) <= m.cfg.SearchThreshold {
		// Small toolset: pass all directly
		rc.Tools = append(rc.Tools, m.cfg.AllTools...)
		return ctx, rc, nil
	}

	// Large toolset: search or deferred mode
	if m.cfg.UseDeferred {
		// Model-native search mode
		rc.ToolSearchTool = &schema.ToolInfo{
			Name:        "search_tools",
			Description: "Search for available tools by keyword",
		}
		return ctx, rc, nil
	}

	// Client-side search mode: add search meta-tool + pass some directly
	rc.Tools = append(rc.Tools, m.newSearchTool())

	// Pass the first threshold/2 tools directly (commonly needed)
	passDirect := m.cfg.SearchThreshold / 2
	if passDirect > len(m.cfg.AllTools) { passDirect = len(m.cfg.AllTools) }
	rc.Tools = append(rc.Tools, m.cfg.AllTools[:passDirect]...)

	return ctx, rc, nil
}

func (m *middleware[M]) BeforeModelRewrite(ctx context.Context, state *agentcore.TypedReActAgentState[M], mc *agentcore.TypedModelContext[M]) (context.Context, *agentcore.TypedReActAgentState[M], error) {
	if !m.cfg.UseDeferred { return ctx, state, nil }

	// Deferred mode: build tool info list
	infos := make([]*schema.ToolInfo, 0, len(m.cfg.AllTools))
	for _, t := range m.cfg.AllTools {
		infos = append(infos, &schema.ToolInfo{Name: t.Name(), Description: t.Description()})
	}
	state.DeferredToolInfos = infos
	return ctx, state, nil
}

func (m *middleware[M]) newSearchTool() agentcore.Tool {
	return agentcore.NewBaseTool("tool_search",
		"Search for available tools by keyword. Supports: keywords, select:name1,name2, +required.",
		func(ctx context.Context, args string) (string, error) {
			args = strings.TrimSpace(args)

			// Direct selection syntax
			if strings.HasPrefix(args, "select:") {
				selected := strings.Split(args[7:], ",")
				for i := range selected { selected[i] = strings.TrimSpace(selected[i]) }
				var results []string
				for _, t := range m.cfg.AllTools {
					for _, s := range selected {
						if strings.EqualFold(t.Name(), s) {
							results = append(results, t.Name()+": "+t.Description())
						}
					}
				}
				if len(results) == 0 { return "No selected tools found.", nil }
				return "Selected tools:\n" + strings.Join(results, "\n"), nil
			}

			// Keyword search
			keywords := strings.Fields(args)
			if len(keywords) == 0 { return "Please provide keywords to search.", nil }

			// Separate required (+prefix) and optional keywords
			var required, optional []string
			for _, kw := range keywords {
				if strings.HasPrefix(kw, "+") {
					required = append(required, strings.ToLower(kw[1:]))
				} else {
					optional = append(optional, strings.ToLower(kw))
				}
			}

			// Score each tool
			type scoredTool struct {
				name  string
				desc  string
				score int
			}
			var scored []scoredTool
			for _, t := range m.cfg.AllTools {
				name := strings.ToLower(t.Name())
				desc := strings.ToLower(t.Description())
				score := 0

				// Check required keywords
				allMatched := true
				for _, r := range required {
					if !strings.Contains(name, r) && !strings.Contains(desc, r) { allMatched = false; break }
				}
				if !allMatched { continue }

				// Score optional keywords
				for _, kw := range optional {
					nameParts := splitToolName(t.Name())
					for _, part := range nameParts {
						if strings.EqualFold(part, kw) { score += 10; continue }
						if strings.Contains(strings.ToLower(part), kw) { score += 5 }
					}
					if strings.EqualFold(t.Name(), kw) { score += 10 }
					if strings.Contains(name, kw) { score += 3 }
					if strings.Contains(desc, kw) { score += 2 }
				}
				if score > 0 || len(optional) == 0 {
					scored = append(scored, scoredTool{name: t.Name(), desc: t.Description(), score: score})
				}
			}

			// Sort by score (simple bubble sort)
			for i := 0; i < len(scored); i++ {
				for j := i + 1; j < len(scored); j++ {
					if scored[j].score > scored[i].score {
						scored[i], scored[j] = scored[j], scored[i]
					}
				}
			}

			if len(scored) == 0 { return "No tools found for: " + args, nil }

			// Limit results
			if len(scored) > m.cfg.MaxResults { scored = scored[:m.cfg.MaxResults] }
			var results []string
			for _, s := range scored {
				results = append(results, s.name+": "+s.desc)
			}
			return "Available tools:\n" + strings.Join(results, "\n"), nil
		})
}

// splitToolName splits tool names by separators (__ or _ or camelCase).
func splitToolName(name string) []string {
	// Handle __ (MCP separator), _ (underscore), and camelCase
	name = strings.ReplaceAll(name, "__", "|")
	name = strings.ReplaceAll(name, "_", "|")
	parts := strings.Split(name, "|")

	// Further split camelCase
	var result []string
	for _, part := range parts {
		if part == "" { continue }
		var current strings.Builder
		for i, r := range part {
			if i > 0 && r >= 'A' && r <= 'Z' {
				if current.Len() > 0 {
					result = append(result, strings.ToLower(current.String()))
				}
				current.Reset()
			}
			current.WriteRune(r)
		}
		if current.Len() > 0 {
			result = append(result, strings.ToLower(current.String()))
		}
	}
	return result
}

