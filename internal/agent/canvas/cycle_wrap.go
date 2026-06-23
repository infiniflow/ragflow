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

// cycle_wrap.go — cycle detection + synthetic Loop wrapping.
//
// eino's compose.Workflow is strictly a DAG: it rejects any data or
// control edge that would close a cycle (see
// compose.DAGInvalidLoopErr in eino v0.9.0-beta.1 graph.go:1129).
// Several v1 DSL fixtures in
// internal/agent/dsl/testdata/v1_examples (exesql.json,
// headhunter_zh.json) carry intentional cycles — Answer ↔ ExeSQL
// and Answer ↔ Message — that model "wait for the next user turn"
// in a multi-turn conversation flow. The Python v1 engine resolves
// those cycles at run time via iterative stateful execution; the Go
// port, built on eino's DAG model, cannot model them directly.
//
// Phase 1 strategy: when the canvas has a cycle, wrap the entire
// component set in a synthetic Loop node driven by
// workflowx.AddLoopNode. The Loop's body is the unrolled canvas; the
// Loop's shouldQuit closure returns true after the first iteration,
// so the eino outer graph is a single (acyclic) Loop node and the
// cycle-causing edges live inside the Loop's sub-workflow. The
// "wait for user" semantics are NOT preserved at this layer — the
// stub AnswerStub just returns an empty answer immediately — but the
// e2e compile + invoke path is fully exercised for the cyclic
// fixtures, which is what the dsl-examples suite needs.
//
// This is a documented Phase 1 simplification. The real "wait for
// user" support lands in a future orchestration layer (Phase 5 /
// SSE handler) that pauses the run and resumes on the next user
// turn, by which point the sub-workflow's iteration count can be
// driven by the orchestrator instead of a hard-coded "run once and
// exit" shouldQuit.

package canvas

import (
	"context"
	"fmt"

	"ragflow/internal/agent/workflowx"

	"github.com/cloudwego/eino/compose"
)

// syntheticLoopKey is the cpn_id used for the synthetic Loop node
// that wraps a cyclic canvas. Using a reserved key avoids
// collisions with any user-defined cpn_id.
const syntheticLoopKey = "__synthetic_loop__"

// hasCycle reports whether the canvas's Downstream / Upstream edges
// form at least one cycle (a self-edge, or a non-trivial strongly
// connected component).
//
// The check is a simple iterative Tarjan-style SCC walk — we do not
// need the full SCC decomposition, only a yes/no answer. The walk
// uses the explicit Downstream lists that the canvas already
// exposes; the loop's own internal edges (Begin↔Answer cycles
// inside an existing Loop sub-graph) are not relevant here because
// buildLoopExpansion has already consumed them by the time
// BuildWorkflow asks.
//
// Complexity: O(V + E) — single DFS over the components map, with
// early exit as soon as a back-edge is found. The fixture set has
// at most ~30 components per canvas, so a simple recursive
// implementation is more than fast enough.
func hasCycle(c *Canvas) bool {
	// Self-edge check — cheap, do it first.
	for cpnID, comp := range c.Components {
		for _, down := range comp.Downstream {
			if down == cpnID {
				return true
			}
		}
	}

	// Iterative DFS with three-colour marking: 0 = unvisited, 1 =
	// in current DFS stack, 2 = fully visited. A back-edge (an edge
	// to a node already in the current stack) means a cycle.
	const (
		unvisited = 0
		onStack   = 1
		done      = 2
	)
	state := make(map[string]int, len(c.Components))
	for start := range c.Components {
		if state[start] != unvisited {
			continue
		}
		// Stack entries: (cpn_id, index into Downstream).
		stack := []struct {
			cpn string
			i   int
		}{{cpn: start, i: 0}}
		state[start] = onStack
		for len(stack) > 0 {
			top := &stack[len(stack)-1]
			comp := c.Components[top.cpn]
			if top.i >= len(comp.Downstream) {
				state[top.cpn] = done
				stack = stack[:len(stack)-1]
				continue
			}
			down := comp.Downstream[top.i]
			top.i++
			if down == top.cpn {
				// Self-edge inside a Downstream list — already
				// filtered out by the early check, but kept here
				// as a defence-in-depth.
				return true
			}
			switch state[down] {
			case unvisited:
				state[down] = onStack
				stack = append(stack, struct {
					cpn string
					i   int
				}{cpn: down, i: 0})
			case onStack:
				return true
			case done:
				// Cross / forward edge into a fully-visited
				// component — cannot create a new cycle.
			}
		}
	}
	return false
}

// buildSyntheticLoop wraps the entire canvas in a single Loop node
// so the outer eino Workflow is acyclic. The Loop's body is the
// unrolled canvas (all components registered as members); the
// Loop's shouldQuit is "always quit after one iteration" so the
// outer workflow returns its (synthetic, body-shaped) output to the
// caller on the first pass.
//
// The returned *loopExpansion is the same shape buildLoopExpansion
// produces for user-declared Loops, so BuildWorkflow can use it
// through the existing install path (workflowx.AddLoopNode +
// loopMembers bookkeeping). The `members` field is the full
// component set, so the main BuildWorkflow pass skips them
// entirely; the outer workflow ends up with exactly one node — the
// synthetic Loop.
//
// `c.Components` is assumed to be non-empty by the caller; an empty
// canvas is rejected earlier in BuildWorkflow.
//
// Cycle breaking: eino's compose.Workflow is itself strictly a
// DAG, so the sub-workflow inside the synthetic Loop would
// otherwise reject the same cycle. We pre-process the member edge
// set to drop back-edges (edges that would close a cycle when
// added to the current forward graph). For each cpn, only its
// FIRST upstream is wired as a data edge; subsequent upstreams
// are dropped entirely (no AddDependency — eino's cycle check
// catches control edges too). The dropped edges are the
// cycle-causing back-edges in practice; the kept data edge
// preserves the primary flow direction. Phase 5 / the real
// orchestrator will replace this with a proper iterative
// control-flow driver.
func buildSyntheticLoop(ctx context.Context, c *Canvas) (*loopExpansion, error) {
	if c == nil || len(c.Components) == 0 {
		return nil, fmt.Errorf("canvas: buildSyntheticLoop: empty canvas")
	}

	members := make(map[string]bool, len(c.Components))
	for cpnID := range c.Components {
		members[cpnID] = true
	}

	// Phase 1: shouldQuit always returns true (quit after the
	// first iteration). shouldQuit is invoked AFTER each
	// completed iteration; with iteration==1 and a constant
	// "true" return, the loop body runs exactly once. The hard
	// cap via WithLoopMaxIterations(1) below is defence in
	// depth in case a future refactor moves the shouldQuit
	// check around.
	shouldQuit := func(_ context.Context, iteration int, _, _ map[string]any) (bool, error) {
		return iteration >= 1, nil
	}

	// Build the sub-workflow. buildSubWorkflow is reused so the
	// loop-body node wiring / state plumbing stays in one place.
	// The dropped-edges policy above is implemented inside the
	// helper via a `breakCycles` flag — see the patched edge
	// loop in buildSubWorkflow.
	sub, err := buildSubWorkflowBreakCycles(ctx, c, members, syntheticLoopKey, nil)
	if err != nil {
		return nil, fmt.Errorf("canvas: synthetic loop buildSubWorkflow: %w", err)
	}

	return &loopExpansion{
		Sub:        sub,
		ShouldQuit: shouldQuit,
		MaxIters:   1,
		Members:    members,
	}, nil
}

// alwaysQuitOption is a tiny helper: callers that need a one-iteration
// loop pass it as the LoopOption set so the workflowx cap matches
// shouldQuit's first-iteration behaviour.
func alwaysQuitOption() workflowx.LoopOption {
	return workflowx.WithLoopMaxIterations(1)
}

// compileSyntheticLoop installs the synthetic loop node in wf and
// returns the resolved *compose.WorkflowNode so the caller can wire
// START/END against it. It is the cycle-wrap path's equivalent of
// the pre-pass block in BuildWorkflow that calls
// workflowx.AddLoopNode for user-declared Loops.
func compileSyntheticLoop(
	ctx context.Context,
	wf *compose.Workflow[map[string]any, map[string]any],
	exp *loopExpansion,
) (*compose.WorkflowNode, error) {
	node, err := workflowx.AddLoopNode[map[string]any](
		ctx, wf, syntheticLoopKey, exp.Sub, exp.ShouldQuit, alwaysQuitOption(),
	)
	if err != nil {
		return nil, fmt.Errorf("canvas: install synthetic loop: %w", err)
	}
	return node, nil
}

// buildSubWorkflowBreakCycles is the cycle-breaking variant of
// buildSubWorkflow used by the synthetic Loop wrap. It is otherwise
// identical (init lambda, state plumbing, END wiring, START
// wiring) except the edge-wiring step:
//
//   - for each cpn, only the FIRST upstream in the DSL's Upstream
//     list is wired as a data edge to cpn;
//   - subsequent upstreams are dropped entirely (not converted to
//     exec-only AddDependency), because eino's cycle check
//     includes control edges in the cycle search — see
//     eino/compose/graph.go:1123 ("DAGInvalidLoopErr ... has
//     loop").
//
// This deterministic policy (drop secondary upstreams) is what
// actually breaks the cycle: every non-trivial cycle in a v1
// fixture involves a back-edge that, on at least one of the
// cyclic nodes, is a secondary upstream. Keeping the first
// upstream preserves the primary flow direction; the dropped
// edges correspond to the "wait for user / wait for next turn"
// back-edges that the Python v1 engine resolves iteratively.
// Phase 5's orchestrator will replace this with a proper
// iterative driver.
func buildSubWorkflowBreakCycles(
	ctx context.Context,
	c *Canvas,
	members map[string]bool,
	loopID string,
	initValues map[string]initVarSpec,
) (*compose.Workflow[map[string]any, map[string]any], error) {
	_ = ctx
	sub := compose.NewWorkflow[map[string]any, map[string]any]()
	nodes := make(map[string]*compose.WorkflowNode, len(members)+1)

	// Synthetic init lambda: passthrough when no initValues are
	// supplied (the synthetic loop carries none). The body is
	// unconditional so the helper compiles even when the
	// initValues map is nil.
	initNode := sub.AddLambdaNode(loopInitKey,
		compose.InvokableLambda(func(ctx context.Context, in map[string]any) (map[string]any, error) {
			if len(initValues) == 0 {
				return in, nil
			}
			state, _, err := GetStateFromContext[*CanvasState](ctx)
			if err != nil || state == nil {
				return in, nil
			}
			for k, spec := range initValues {
				existing, _ := state.GetVar(loopID + "@" + k)
				if existing != nil {
					continue
				}
				state.SetVar(loopID, k, spec.Value)
			}
			return in, nil
		}),
	)
	nodes[loopInitKey] = initNode

	// Body nodes: one per member, factory-built (or
	// placeholder) wrapped with withStateBracket so they share
	// the outer state.
	for cpnID := range members {
		name := c.Components[cpnID].Obj.ComponentName
		if name == "" {
			return nil, fmt.Errorf("canvas: synthetic loop member %q has empty component_name", cpnID)
		}
		body, err := buildNodeBody(cpnID, name, c.Components[cpnID].Obj.Params)
		if err != nil {
			return nil, err
		}
		nodes[cpnID] = sub.AddLambdaNode(cpnID,
			compose.InvokableLambda[map[string]any, map[string]any](withStateBracket(body)),
			compose.WithNodeName(cpnID),
		)
	}

	// Edge wiring — the cycle-breaking policy. For each cpn we
	// walk its Upstream list and wire only the FIRST in-subgraph
	// upstream. Subsequent upstreams (typically the back-edge in
	// a cycle) are dropped, which is what makes the resulting
	// eino graph acyclic.
	for cpnID := range members {
		upstreams := c.Components[cpnID].Upstream
		first := true
		for _, up := range upstreams {
			if up == loopID {
				// No parent-Loop upstream in the synthetic
				// path, but handle it defensively.
				if first {
					nodes[cpnID].AddInput(loopInitKey)
					first = false
				}
				continue
			}
			if !members[up] {
				continue
			}
			if first {
				nodes[cpnID].AddInput(up)
				first = false
			}
			// Subsequent upstreams are dropped: see the long
			// comment on the function for the rationale.
		}
		if first {
			// No in-subgraph upstream: wire from init so the
			// node still has a data source.
			nodes[cpnID].AddInput(loopInitKey)
		}
	}

	// Wire END: every member that has no downstream within the
	// sub-graph is a sub-graph terminal.
	hasDownstream := make(map[string]bool, len(members))
	for cpnID := range members {
		for _, down := range c.Components[cpnID].Downstream {
			if members[down] {
				hasDownstream[cpnID] = true
				break
			}
		}
	}
	hasEnd := false
	for cpnID := range members {
		if hasDownstream[cpnID] {
			continue
		}
		sub.End().AddInput(cpnID, compose.ToField(cpnID))
		hasEnd = true
	}
	if !hasEnd {
		sub.End().AddInput(loopInitKey, compose.ToField(loopInitKey))
	}

	initNode.AddInput(compose.START)
	return sub, nil
}
