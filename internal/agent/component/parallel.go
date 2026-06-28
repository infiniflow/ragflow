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

// Package component — Parallel component (T3, plan §2.11.3 row 9).
//
// Parallel is the parent node for an array-iteration subgraph. The Go port
// implements a single-node parallel fan-out driven by workflowx.AddParallelNode:
// when BuildWorkflow sees a Parallel cpn, it collects the Parallel's
// downstream descendants into a sub-graph, installs a workflowx.AddParallelNode
// in place of the Parallel subtree, and skips Parallel in the main
// node-registration pass.
//
// As a result, ParallelComponent itself does NOT do any per-item
// work at runtime. ParallelComponent.Invoke is a no-op marker that
// returns an empty map; the actual iteration is driven by the
// sub-graph run once per input item via AddParallelNode. Items are
// processed with bounded concurrency; the output list order strictly
// corresponds to the input list order (see
// .claude/plans/eino-workflow-parallel.md §5 Order preservation).
//
// The component still exists in the registry so:
//   - tooling / introspection (component.New, RegisteredNames) work;
//   - factory-style wiring can still construct a ParallelComponent from
//     a params map (useful for tests and direct API callers);
//
// ParallelParam and its Update/Check/AsDict methods stay because they
// describe the canonical Parallel DSL shape, even though the runtime path
// bypasses them (canvas.buildParallelExpansion parses the raw params
// map directly). Keep them as a single source of truth for what a
// Parallel params block looks like.
//
// The former Iteration/IterationItem component pair is subsumed into
// this single Parallel component: per-item execution is handled by the
// sub-graph body nodes, and the fan-out orchestration (counter, _done
// signalling, output collation) is handled by workflowx.AddParallelNode.
package component

import (
	"context"
)

const componentNameParallel = "Parallel"

// ParallelComponent is the canvas-level parallel parent. The runtime
// parallel driver lives in workflowx.AddParallelNode, not in this type.
// The component exists for registry / factory / introspection only —
// Invoke is a no-op that returns an empty map.
type ParallelComponent struct {
	param ParallelParam
}

// ParallelParam captures the (resolved) DSL parameters for a Parallel
// node. Only `items_ref` and `max_concurrency` are meaningful for the
// runtime path; the canvas layer (buildParallelExpansion) resolves the
// array and passes it as input to workflowx.AddParallelNode.
type ParallelParam struct {
	// ItemsRef is a variable reference (e.g. "sys.arr", "parallel_0@result")
	// pointing to the list to iterate over.
	ItemsRef string

	// MaxConcurrency caps the number of per-item sub-workflow invocations
	// that run concurrently. 0 (default) means sequential execution.
	// Maps to workflowx.WithParallelMaxConcurrency in the macro expansion.
	MaxConcurrency int
}

// Update copies conf into p. Used by the editor / API to hand-craft a
// params map; type validation is intentionally minimal in P2.
func (p *ParallelParam) Update(conf map[string]any) error {
	if conf == nil {
		return nil
	}
	if v, ok := stringFrom(conf, "items_ref"); ok {
		p.ItemsRef = v
	}
	if v, ok := intFrom(conf, "max_concurrency"); ok {
		p.MaxConcurrency = v
	}
	return nil
}

// Check performs shallow validation.
func (p *ParallelParam) Check() error {
	return nil
}

// AsDict returns the params as a plain map for serialization / debug.
func (p *ParallelParam) AsDict() map[string]any {
	out := map[string]any{}
	if p.ItemsRef != "" {
		out["items_ref"] = p.ItemsRef
	}
	if p.MaxConcurrency > 0 {
		out["max_concurrency"] = p.MaxConcurrency
	}
	return out
}

// NewParallelComponent builds a ParallelComponent from the supplied
// param struct.
func NewParallelComponent(p ParallelParam) *ParallelComponent {
	return &ParallelComponent{param: p}
}

// Name returns the registered component name.
func (c *ParallelComponent) Name() string { return componentNameParallel }

// Inputs returns parameter metadata for tooling.
func (c *ParallelComponent) Inputs() map[string]string {
	return map[string]string{
		"cpn_id":          "Stable component identifier — BuildWorkflow uses this to detect Parallel and apply the workflowx.AddParallelNode macro expansion.",
		"items_ref":       "Variable reference to the list to iterate (e.g. \"sys.arr\").",
		"max_concurrency": "Maximum concurrent per-item sub-workflow invocations. 0 = sequential.",
	}
}

// Outputs returns the Parallel's public outputs. In the new architecture,
// the actual output is a []O slice produced by
// workflowx.AddParallelNode, not by ParallelComponent.Invoke.
// ParallelComponent itself emits no outputs; this map documents the
// contract for downstream consumers reading the parallel node's result.
func (c *ParallelComponent) Outputs() map[string]string {
	return map[string]string{
		"_result": "Output list ([]O) — order strictly corresponds to input list order.",
	}
}

// Invoke is a no-op marker. The real per-item work runs inside the
// sub-graph (AddParallelNode fans out the sub-workflow to run once per
// input item). ParallelComponent.Invoke is kept on the Component
// interface for callers that construct a ParallelComponent directly
// outside the canvas engine (e.g. unit tests that want to verify
// registration); under the canvas engine, this method is never called.
//
// The returned map is empty. State writes from this method would be
// silently dropped by the eino graph, because ParallelComponent is not
// registered as an eino node when the macro expansion fires.
func (c *ParallelComponent) Invoke(_ context.Context, _ map[string]any) (map[string]any, error) {
	return map[string]any{}, nil
}

// Stream mirrors Invoke and emits an empty map as a single chunk.
func (c *ParallelComponent) Stream(ctx context.Context, inputs map[string]any) (<-chan map[string]any, error) {
	out, err := c.Invoke(ctx, inputs)
	if err != nil {
		return nil, err
	}
	ch := make(chan map[string]any, 1)
	ch <- out
	close(ch)
	return ch, nil
}

// init registers ParallelComponent with the orchestrator-owned registry.
//
// ParallelComponent.Invoke is a no-op; the runtime parallel driver lives
// in workflowx.AddParallelNode and is installed by canvas.BuildWorkflow
// when it sees a Parallel cpn in the DSL.
//
// The former IterationItem component is NOT registered — the per-item
// execution formerly handled by that component is now subsumed by the
// sub-graph body nodes inside AddParallelNode, and the fan-out
// orchestration (counter, _done signalling, output collation) is handled
// by the parallel extension.
func init() {
	Register(componentNameParallel, func(params map[string]any) (Component, error) {
		var p ParallelParam
		if err := p.Update(params); err != nil {
			return nil, err
		}
		return NewParallelComponent(p), nil
	})
}
