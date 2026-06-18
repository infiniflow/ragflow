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

// multibranch.go — runtime branch wiring for Switch / Categorize.
//
// Switch and Categorize are control-flow components that produce a
// `_next` output identifying which downstream child should run at
// runtime. The static AddInput edges from a parent to every declared
// child carry the data path; this file adds the eino MultiBranch
// wiring that gates control so only the chosen child executes:
//
//   1. The static AddInput edges stay wired (so the chosen child
//      receives the parent's output as its input data).
//   2. For every Switch / Categorize parent with >= 2 downstream
//      children, we register
//      wf.AddBranch(parent, NewGraphBranch(cond, endNodes)).
//   3. The branch's condition reads in["_next"] from the parent's
//      output map and returns the chosen cpn_id (or "" if no match —
//      which eino interprets as "no branch chosen, fall through").
//
// Per eino v0.9.5 (compose/workflow.go:413-419), Workflow branches
// are control-only: the chosen end-node does NOT auto-receive the
// branch source's output. The static AddInput edges supply the data
// path; the branch supplies the control gate.
//
// Categorize is included for symmetry even though its current
// outputs["_next"] is an empty slice (the chosen category name lives
// at outputs["category"] and the downstream-routing glue between
// "category" and "cpn_id" is tracked at the DSL layer). When the
// glue lands, the existing branch wiring picks it up with no further
// change here.

package canvas

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/compose"
)

// branchableControlNames is the case-insensitive set of component
// names that produce a runtime `_next` field and therefore qualify
// for MultiBranch wiring. Switch emits _next as a single cpn_id
// string; Categorize emits it as a list (see the package comment
// above for the current status). The set is small on purpose: adding
// a new entry requires the component body to emit outputs["_next"]
// in a shape wireMultiBranches can consume.
var branchableControlNames = map[string]bool{
	"switch":     true,
	"categorize": true,
}

// isBranchableControl reports whether the component name is one of
// the runtime-control components that should get a MultiBranch edge
// from BuildWorkflow. The lookup is case-insensitive to match the
// rest of the package's name handling (see canvas.go:92).
func isBranchableControl(name string) bool {
	return branchableControlNames[strings.ToLower(name)]
}

// wireMultiBranches registers an eino MultiBranch on every
// branchable parent that has at least two declared downstream
// children. Pass-2 already wired AddInput edges from parent to each
// child; the branch adds the control-only gating so only the
// chosen child fires at runtime.
//
// The function is a no-op for:
//   - parents with < 2 downstreams (a single-child "switch" is
//     degenerate — no branching needed, AddInput is enough)
//   - parents inside loop subgraphs (their children live in the
//     loop's sub-workflow; the outer graph can't see them)
//   - Loop cpns themselves (their children are inside the loop
//     body; same reason)
//
// Returns the list of registered (parent cpn_id → end-nodes set)
// pairs so tests can assert which branches were installed.
func wireMultiBranches(
	wf *compose.Workflow[map[string]any, map[string]any],
	c *Canvas,
	loopMembers map[string]bool,
) []branchRegistration {
	if wf == nil || c == nil {
		return nil
	}
	var out []branchRegistration
	for cpnID, comp := range c.Components {
		// Skip loop body members — they live in a sub-workflow
		// whose branches must be wired separately by the loop
		// expansion code, not here.
		if loopMembers[cpnID] {
			continue
		}
		if !isBranchableControl(comp.Obj.ComponentName) {
			continue
		}
		// Filter downstreams: keep only nodes that exist in the
		// outer graph (i.e. not loop members). A Switch whose
		// children are all inside a loop body has no
		// outer-graph routing to install.
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
			// Either no outer-graph children, or fewer than
			// two — a MultiBranch with < 2 end-nodes is
			// either meaningless (0/1 end-nodes) or
			// equivalent to plain AddInput. Skip it so we
			// don't pay the branch-evaluation cost when the
			// DSL doesn't actually branch.
			continue
		}
		endNodesList := make([]string, 0, len(endNodes))
		for n := range endNodes {
			endNodesList = append(endNodesList, n)
		}
		cond := makeSwitchBranchCondition(endNodes)
		wf.AddBranch(cpnID, compose.NewGraphBranch(cond, endNodes))
		out = append(out, branchRegistration{
			Parent:   cpnID,
			EndNodes: endNodesList,
		})
	}
	return out
}

// branchRegistration is the public record of a MultiBranch that was
// installed. Returned by wireMultiBranches for test introspection;
// the scheduler does not consume it.
type branchRegistration struct {
	Parent   string
	EndNodes []string
}

// makeSwitchBranchCondition returns a GraphBranchCondition that
// drives eino's MultiBranch from the parent's outputs["_next"]
// field. The condition:
//
//  1. Pulls `_next` out of the parent's output map (which the
//     statePost handler has already written to state.Outputs and
//     the lambda has returned).
//  2. Validates the value against the endNodes whitelist. eino
//     rejects unknown keys at runtime with "branch invocation
//     returns unintended end node: <key>"; clamping here means a
//     misconfigured Switch (e.g. `to: "ghost"` for a downstream
//     that was deleted) degrades to "no branch chosen" instead of
//     crashing the run.
//  3. Falls back to empty string when `_next` is absent, empty, or
//     not in the whitelist. eino treats an empty chosen list as
//     "no successor" — the workflow simply doesn't continue past
//     the parent on this path. This matches the Python semantics
//     for a Switch whose default points to a non-existent node.
func makeSwitchBranchCondition(endNodes map[string]bool) compose.GraphBranchCondition[map[string]any] {
	return func(_ context.Context, in map[string]any) (string, error) {
		raw, ok := in["_next"]
		if !ok {
			return "", nil
		}
		next, ok := raw.(string)
		if !ok || next == "" {
			return "", nil
		}
		if !endNodes[next] {
			// _next resolved to something outside the
			// whitelist. eino would error with "branch
			// invocation returns unintended end node" —
			// suppress that and exit gracefully so a
			// misconfigured DSL doesn't take down the run.
			return "", nil
		}
		return next, nil
	}
}

// fmtBranchRegistrations is a small debug helper kept here so the
// table of installed branches can be dumped from a test or a future
// verbose-logging path without pulling in fmt at the call site.
// Currently unused; lives next to its data type for symmetry.
func fmtBranchRegistrations(regs []branchRegistration) string {
	if len(regs) == 0 {
		return "no multi-branches installed"
	}
	var b strings.Builder
	for _, r := range regs {
		fmt.Fprintf(&b, "%s -> %v\n", r.Parent, r.EndNodes)
	}
	return b.String()
}
