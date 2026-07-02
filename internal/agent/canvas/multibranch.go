// multibranch.go — runtime branch wiring for Switch / Categorize.
//
// Switch and Categorize are control-flow components that produce a
// `_next` output identifying which downstream child should run at
// runtime. The static AddEdge edges from a parent to every declared
// child carry the data path; this file adds harness AddBranch wiring
// that gates control so only the chosen child executes.
//
// harness StateGraph branches (AddBranch)
// the condition function receives the current state and returns node
// names to execute next. Unlike GraphBranch which works at the
// edge level on per-node output, harness Branch operates at the graph
// level on the shared state.
package canvas

import (
	"context"
	"fmt"
	"strings"

	graphpkg "ragflow/internal/harness/graph/graph"
)

// branchableControlNames is the case-insensitive set of component
// names that produce a runtime `_next` field.
var branchableControlNames = map[string]bool{
	"switch":     true,
	"categorize": true,
}

// isBranchableControl reports whether the component name is one of
// the runtime-control components that should get a Branch edge.
func isBranchableControl(name string) bool {
	return branchableControlNames[strings.ToLower(name)]
}

// wireMultiBranches registers AddConditionalEdges on every branchable
// parent (Switch, Categorize) that has >= 2 downstream children.
//
// The condition reads the Switch/Categorize output (_next) from
// CanvasState (attached to ctx) — the Pregel engine's graph state does
// not carry per-node outputs because those live in CanvasState.Outputs.
// When a branchable node has conditional edges, the engine's getNextNodes
// does NOT fall back to AddEdge (regular edges), so only the routed
// child executes.
func wireMultiBranches(
	sg *graphpkg.StateGraph,
	c *Canvas,
	loopMembers map[string]bool,
) error {
	if sg == nil || c == nil {
		return nil
	}
	for cpnID, comp := range c.Components {
		if loopMembers[cpnID] {
			continue
		}
		if !isBranchableControl(comp.Obj.ComponentName) {
			continue
		}
		// Filter downstreams: only non-loop-member children.
		endNodes := make(map[string]bool, len(comp.Downstream))
		for _, child := range comp.Downstream {
			if loopMembers[child] {
				continue
			}
			if _, ok := c.Components[child]; !ok {
				continue
			}
			endNodes[child] = true
		}
		if len(endNodes) < 2 {
			continue
		}

		// Build mapping: every child maps to itself so the reachability
		// check passes and the condition can return any child ID.
		mapping := make(map[string]string, len(endNodes))
		for n := range endNodes {
			mapping[n] = n
		}

		// Condition: read _next from the graph state (merged output of
		// the last completed node — Switch/Categorize writes _next as a
		// top-level key).  The graph state is available via the `state`
		// parameter; CanvasState is NOT directly accessible because the
		// engine's goroutine may wrap the context.
		condition := func(ctx context.Context, state any) (any, error) {
			m, ok := state.(map[string]any)
			if !ok {
				return "", nil
			}
			raw, has := m["_next"]
			if !has || raw == nil {
				return "", nil
			}
			switch tv := raw.(type) {
			case string:
				if endNodes[tv] {
					return tv, nil
				}
			case []any:
				if len(tv) > 0 {
					if s, ok := tv[0].(string); ok && endNodes[s] {
						return s, nil
					}
				}
			}
			return "", nil
		}

		if err := sg.AddConditionalEdges(cpnID, condition, mapping); err != nil {
			return fmt.Errorf("canvas: add conditional edges for %q: %w", cpnID, err)
		}
	}
	return nil
}
