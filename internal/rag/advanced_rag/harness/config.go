//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package harness

import "strings"

// Thinking-mode configuration: the single authority for mode behaviour.
//
// Mirrors Python harness/config.py. Every mode-dependent decision in the
// harness reads from this file instead of re-deriving it from a thinking_mode
// string, so tuning a mode is a one-table edit.
//
// Unknown labels fall back to NAIVE rather than erroring: the label comes from
// user input at request time, and failing here would fail the whole request.

// allTools are the tools the action session can bind, in declaration order.
// Kept here so a mode's tool set is data, not an if-chain in the runtime.
var allTools = []string{
	"retrieve",
	"search_chunks",
	"list_chunks",
	"navigate_tree",
	"navigate_structure",
	"calculate",
	"web_search",
}

// GraphExploreTool is the relational exploration tool reserved for ultra.
const GraphExploreTool = "graph_explore"

// ModeSpec mirrors Python config.ModeSpec (a frozen dataclass). Everything that
// varies between thinking modes lives here.
//
// Tools is the set of tool names visible to the model in this mode. An empty
// set means the model gets no tool loop at all.
type ModeSpec struct {
	Label          string
	Agentic        bool
	EnableSCA      bool
	SCAMaxRounds   int
	UseFanout      bool
	ActionMaxTurns int
	Tools          map[string]bool
}

// toolsOf builds the tool set from names (mirrors Python _tools()).
func toolsOf(names ...string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

// allToolSet returns the default tool surface (mirrors Python _ALL_TOOLS).
func allToolSet() map[string]bool {
	return toolsOf(allTools...)
}

// THINKING_MODES mirrors Python config.THINKING_MODES.
//
//   - low: one hybrid-search pass through direct_search. No action session,
//     so no tool loop — the model never sees tools in this mode.
//   - medium: agentic, SCA review on, no planner decomposition.
//   - high: adds planner + prefetch fan-out over the same tool surface.
//   - ultra: deeper sessions, more SCA rounds, and the relational tool.
var THINKING_MODES = map[string]ModeSpec{
	"low": {
		Label: "low", Agentic: false, EnableSCA: false,
		SCAMaxRounds: 0, UseFanout: false, ActionMaxTurns: 4,
		Tools: map[string]bool{},
	},
	"medium": {
		Label: "medium", Agentic: true, EnableSCA: true,
		SCAMaxRounds: 3, UseFanout: false, ActionMaxTurns: 4,
		Tools: allToolSet(),
	},
	"high": {
		Label: "high", Agentic: true, EnableSCA: true,
		SCAMaxRounds: 3, UseFanout: true, ActionMaxTurns: 4,
		Tools: allToolSet(),
	},
	"ultra": {
		Label: "ultra", Agentic: true, EnableSCA: true,
		SCAMaxRounds: 5, UseFanout: true, ActionMaxTurns: 6,
		Tools: toolsOf(append(append([]string{}, allTools...), GraphExploreTool)...),
	},
}

// NAIVE is the fallback for an unrecognised mode label. It is not agentic — the
// caller answers with plain retrieval rather than failing the request.
var NAIVE = ModeSpec{
	Label: "naive", Agentic: false, EnableSCA: false,
	SCAMaxRounds: 0, UseFanout: false, ActionMaxTurns: 4,
	Tools: map[string]bool{},
}

// HasTool reports whether the mode exposes the named tool.
func (m ModeSpec) HasTool(name string) bool { return m.Tools[name] }

// ToolNames returns the mode's visible tool names in declaration order, so
// schema generation is deterministic (Go map iteration is randomised).
func (m ModeSpec) ToolNames() []string {
	out := make([]string, 0, len(m.Tools))
	for _, n := range allTools {
		if m.Tools[n] {
			out = append(out, n)
		}
	}
	if m.Tools[GraphExploreTool] {
		out = append(out, GraphExploreTool)
	}
	return out
}

// GetMode mirrors Python get_mode(label): unknown labels fall back to NAIVE.
// The label arrives from user input, so erroring here would fail the request.
func GetMode(label string) ModeSpec {
	m, ok := THINKING_MODES[strings.ToLower(strings.TrimSpace(label))]
	if !ok {
		return NAIVE
	}
	return m
}

// ThinkingModeCarrier is implemented by the RAGTools-like object that owns the
// request-scoped retrieval context (Python RAGTools.thinking_mode).
type ThinkingModeCarrier interface {
	GetThinkingMode() string
}

// ResolveMode mirrors Python resolve_mode(tools): reads a RAGTools-like
// object's thinking mode into its spec. Values that do not implement
// ThinkingModeCarrier fall back to NAIVE, matching Python's getattr default.
func ResolveMode(tools any) ModeSpec {
	if c, ok := tools.(ThinkingModeCarrier); ok {
		return GetMode(c.GetThinkingMode())
	}
	return NAIVE
}
