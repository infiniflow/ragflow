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

// wireMultiBranches registers an harness AddBranch on every branchable
// parent that has at least two declared downstream children.
//
// For harness, the branch condition receives the graph state (not the
// node output map). The state carries the parent output through
// CanvasState.Outputs[parentID], so the condition reads
// state["state"]["_next"] (injected by statePre).
func wireMultiBranches(
	sg *graphpkg.StateGraph,
	c *Canvas,
	loopMembers map[string]bool,
) {
	if sg == nil || c == nil {
		return
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
		endNodesList := make([]string, 0, len(endNodes))
		for n := range endNodes {
			endNodesList = append(endNodesList, n)
		}

		// Build condition: read _next from the state snapshot.
		condition := func(ctx context.Context, state any) (any, error) {
			st, ok := state.(map[string]any)
			if !ok {
				return "", nil
			}
			stateVal, _ := st["state"].(map[string]map[string]any)
			if stateVal == nil {
				return "", nil
			}
			parentOut, _ := stateVal[cpnID]
			if parentOut == nil {
				return "", nil
			}
			next, ok := parentOut["_next"].(string)
			if !ok || next == "" || !endNodes[next] {
				return "", nil
			}
			return next, nil
		}

		then := func(v interface{}) []string {
			s, _ := v.(string)
			if s == "" {
				return nil
			}
			return []string{s}
		}

		if err := sg.AddBranch(cpnID, condition, then); err != nil {
			// Log and continue — a misconfigured branch does not
			// prevent the graph from running (it just won't branch).
			_ = err
		}
	}
}
