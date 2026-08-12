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

// Package canvas — compile entry.
//
// Compile turns a Canvas (DSL) into a CompiledCanvas: a compiled
// compose.Runnable. The compile-time wiring (state pre- / post-handlers,
// run-level override params) is configured here; the actual run path lives in
// runner.go and the HTTP handler / SSE are wired in internal/service and
// internal/handler.
package canvas

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/compose"
	"go.uber.org/zap"

	"ragflow/internal/common"
)

// CompiledCanvas is the compiled runtime representation of a Canvas DSL.
// Workflow is the eino Runnable.
type CompiledCanvas struct {
	Workflow compose.Runnable[map[string]any, map[string]any]
}

// CompileOptions bundles the optional collaborators the compile entry needs.
// All fields are optional; nil/zero means "skip that wire".
type CompileOptions struct {
	// OverrideParams is a run-level override map keyed by cpnID. Each
	// component's `params` is merged only with its own entry
	// (an arbitrary string-keyed map); the override wins on top-level key
	// collision. Components absent from the
	// map are left untouched. Used by the ingestion pipeline so a single
	// Pipeline.Run can override the DSL-baked component params without
	// mutating the shared *Canvas (see node_body.go applyOverrideParams).
	OverrideParams map[string]any
}

// CompileOption mutates a CompileOptions before the compile runs.
type CompileOption func(*CompileOptions)

// WithOverrideParams attaches a run-level override map (keyed by
// cpnID) to compile. Each component's params are merged with
// its own entry at compile time (run-level wins on key collision, see
// node_body.go applyOverrideParams). Passing nil is a no-op.
func WithOverrideParams(m map[string]any) CompileOption {
	return func(o *CompileOptions) { o.OverrideParams = m }
}

// Compile builds the eino Workflow from the Canvas and returns the
// compiled Runnable. State pre- / post-handlers are wired inside BuildWorkflow
// (see scheduler.go).
func Compile(ctx context.Context, c *Canvas, opts ...CompileOption) (*CompiledCanvas, error) {
	cfg := CompileOptions{}
	for _, o := range opts {
		o(&cfg)
	}

	// Decoder-boundary guard: if the caller handed us a Canvas
	// whose `components` still contains LoopItem or IterationItem
	// entries, they bypassed dsl.NormalizeForCanvas (the only
	// supported decoder path). The fold step never ran, so the
	// runtime will see legacy child names and the workflow below
	// will misbehave. Surface a visible stderr warning so the
	// regression is observable — this is intentionally a log
	// rather than a panic, because internal drivers (tests,
	// fixtures) may exercise the path with raw components.
	if c != nil {
		var n int
		for _, comp := range c.Components {
			switch strings.ToLower(comp.Obj.ComponentName) {
			case "loopitem", "iterationitem", "iteration":
				n++
			}
		}
		if n > 0 {
			common.Info("canvas: Compile received Canvas with legacy LoopItem/IterationItem/Iteration nodes; this path bypassed dsl.NormalizeForCanvas — the fold step is not applied", zap.Int("n", n))
		}
	}

	// Thread the run-level override (if any) into ctx so each
	// component's params is merged with its own entry inside
	// buildNodeBody. The override is keyed by cpnID; the canvas package
	// never imports ingestion.
	if cfg.OverrideParams != nil {
		ctx = withOverrideParams(ctx, cfg.OverrideParams)
	}

	wf, err := BuildWorkflow(ctx, c)
	if err != nil {
		return nil, fmt.Errorf("canvas: build workflow: %w", err)
	}

	runnable, err := wf.Compile(ctx)
	if err != nil {
		return nil, fmt.Errorf("canvas: eino compile: %w", err)
	}
	return &CompiledCanvas{Workflow: runnable}, nil
}
