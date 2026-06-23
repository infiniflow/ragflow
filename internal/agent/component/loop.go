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

// Package component — Loop component (T3, plan §2.11.3 row 11).
//
// Loop is the parent node for a conditional loop subgraph. The Go port
// implements a single-node loop driven by workflowx.AddLoopNode: when
// BuildWorkflow sees a Loop cpn, it collects the Loop's downstream
// descendants into a sub-graph (see canvas/loop_subgraph.go), installs
// a workflowx.AddLoopNode in place of the Loop subtree, and skips
// Loop in the main node-registration pass.
//
// As a result, LoopComponent itself does NOT do any per-iteration
// work at runtime. LoopComponent.Invoke is a no-op marker that
// returns an empty map; the actual loop iteration is driven by
// the sub-graph's init lambda (which seeds loop_variables into
// CanvasState) and the sub-workflow's per-iteration body. Loop
// termination is driven by the workflowx.LoopCondition produced by
// translateLoopCondition from the DSL's loop_termination_condition
// list.
//
// The component still exists in the registry so:
//   - tooling / introspection (component.New, RegisteredNames) work;
//   - factory-style wiring can still construct a LoopComponent from
//     a params map (useful for tests and direct API callers);
//
// loopParam and its Update/Check/AsDict methods stay because they
// describe the canonical Loop DSL shape, even though runtime path
// bypasses them (canvas.buildLoopExpansion parses the raw params
// map directly). Keep them as a single source of truth for what a
// Loop params block looks like.
package component

import (
	"context"
)

const componentNameLoop = "Loop"

// LoopComponent is the canvas-level loop parent. The runtime loop
// driver lives in workflowx.AddLoopNode, not in this type. The
// component exists for registry / factory / introspection only —
// Invoke is a no-op that returns an empty map.
type LoopComponent struct {
	param loopParam
}

// loopParam captures the (resolved) DSL parameters for a Loop node.
// Only `loop_variables` and `loop_termination_condition` are
// meaningful; the parent.get_start() walk that the Python version
// performs (loop.py:46-51) is an engine concern handled by
// canvas.buildLoopExpansion at BuildWorkflow time.
type loopParam struct {
	// LoopVariables is the list of variable initializers. Each entry is
	// a map with keys {variable, input_mode, value, type}. The slice
	// pointer is shared with the DSL loader — callers should treat it
	// as read-only.
	LoopVariables []map[string]any

	// LoopTerminationCondition is the list of termination conditions.
	// Each entry is a map with keys {variable, operator, value,
	// input_mode}. The condition list is translated to a
	// workflowx.LoopCondition closure by canvas.translateLoopCondition.
	LoopTerminationCondition []map[string]any

	// LogicalOperator combines per-condition results: "and" (default)
	// or "or".
	LogicalOperator string

	// MaximumLoopCount caps the iteration count. 0 = infinite.
	MaximumLoopCount int
}

// Update copies conf into p. Used by the editor / API to hand-craft a
// params map; type validation is intentionally minimal in P2.
func (p *loopParam) Update(conf map[string]any) error {
	if conf == nil {
		return nil
	}
	if raw, ok := conf["loop_variables"]; ok {
		p.LoopVariables = toAnyMapSlice(raw)
	}
	if raw, ok := conf["loop_termination_condition"]; ok {
		p.LoopTerminationCondition = toAnyMapSlice(raw)
	}
	if v, ok := stringFrom(conf, "logical_operator"); ok {
		p.LogicalOperator = v
	}
	if v, ok := intFrom(conf, "maximum_loop_count"); ok {
		p.MaximumLoopCount = v
	}
	return nil
}

// Check performs shallow validation. The Python check() at loop.py:39
// always returns True; we mirror that.
func (p *loopParam) Check() error {
	return nil
}

// AsDict returns the params as a plain map for serialization / debug.
func (p *loopParam) AsDict() map[string]any {
	out := map[string]any{}
	if p.LoopVariables != nil {
		out["loop_variables"] = p.LoopVariables
	}
	if p.LoopTerminationCondition != nil {
		out["loop_termination_condition"] = p.LoopTerminationCondition
	}
	if p.LogicalOperator != "" {
		out["logical_operator"] = p.LogicalOperator
	}
	if p.MaximumLoopCount > 0 {
		out["maximum_loop_count"] = p.MaximumLoopCount
	}
	return out
}

// NewLoopComponent builds a LoopComponent from the supplied param struct.
func NewLoopComponent(p loopParam) *LoopComponent {
	return &LoopComponent{param: p}
}

// Name returns the registered component name.
func (c *LoopComponent) Name() string { return componentNameLoop }

// Inputs returns parameter metadata for tooling.
func (c *LoopComponent) Inputs() map[string]string {
	return map[string]string{
		"cpn_id": "Stable component identifier — BuildWorkflow uses this to detect Loop and apply the workflowx.AddLoopNode macro expansion.",
	}
}

// Outputs returns the Loop's public outputs. In the new architecture,
// the actual loop output is the last iteration's body output, which
// flows through the eino sub-graph node. LoopComponent itself emits
// no outputs; this map documents the contract for downstream
// consumers reading the sub-graph's result via FieldMapping.
func (c *LoopComponent) Outputs() map[string]string {
	return map[string]string{
		"_result": "Final iteration output (set by the sub-graph, not by LoopComponent.Invoke).",
	}
}

// Invoke is a no-op marker. The real per-iteration work runs inside
// the sub-graph (init lambda seeds loop_variables into state; the
// sub-workflow runs the body; the LoopCondition closure evaluates
// termination on every iteration). LoopComponent.Invoke is kept on
// the Component interface for callers that construct a LoopComponent
// directly outside the canvas engine (e.g. unit tests that want to
// verify registration); under the canvas engine, this method is
// never called.
//
// The returned map is empty. State writes from this method would be
// silently dropped by the eino graph, because LoopComponent is not
// registered as an eino node when the macro expansion fires.
func (c *LoopComponent) Invoke(_ context.Context, _ map[string]any) (map[string]any, error) {
	return map[string]any{}, nil
}

// Stream mirrors Invoke and emits an empty map as a single chunk.
func (c *LoopComponent) Stream(ctx context.Context, inputs map[string]any) (<-chan map[string]any, error) {
	out, err := c.Invoke(ctx, inputs)
	if err != nil {
		return nil, err
	}
	ch := make(chan map[string]any, 1)
	ch <- out
	close(ch)
	return ch, nil
}

// toAnyMapSlice accepts either []map[string]any or []any and returns
// the canonical []map[string]any view. Unknown element types are
// skipped silently — the per-item check in the canvas layer will
// surface the malformed entry.
func toAnyMapSlice(raw any) []map[string]any {
	switch v := raw.(type) {
	case []map[string]any:
		return v
	case []any:
		out := make([]map[string]any, 0, len(v))
		for _, e := range v {
			if m, ok := e.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	}
	return nil
}

// init registers LoopComponent with the orchestrator-owned registry.
//
// LoopComponent.Invoke is a no-op; the runtime loop driver lives in
// workflowx.AddLoopNode and is installed by canvas.BuildWorkflow
// when it sees a Loop cpn in the DSL.
func init() {
	Register(componentNameLoop, func(params map[string]any) (Component, error) {
		var p loopParam
		if err := p.Update(params); err != nil {
			return nil, err
		}
		return NewLoopComponent(p), nil
	})
}
