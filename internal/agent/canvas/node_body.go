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

// node_body.go — per-node lambda body construction.
//
// Both the outer graph (scheduler.go) and the Loop sub-graph
// (loop_subgraph.go) install lambda nodes that:
//
//  1. tag their output with __cpn_id__ so statePost can persist the
//     result into Outputs[cpnID]["result"];
//  2. either invoke a real factory-built component or fall back to a
//     no-op echo body.
//
// Centralising the construction here keeps both call sites consistent
// and makes the legacy-no-op / factory / placeholder routing logic the
// single source of truth.
package canvas

import (
	"context"
	"errors"
	"fmt"
	"ragflow/internal/common"
	"strconv"
	"strings"
	"time"

	"ragflow/internal/agent/runtime"
)

// nodeBodyFn is the plain function shape compose.InvokableLambda accepts.
// We avoid a named type alias because compose.InvokableLambda's generic
// inference only accepts the underlying func literal type, not a named
// alias on top of it.
type nodeBodyFn = func(ctx context.Context, in map[string]any) (map[string]any, error)

// buildNodeBody returns the lambda body for a single canvas node.
//
// Routing rules:
//
//  1. isLegacyNoOp(name) → legacyNoOpBody (echo + __legacy_noop__ tag).
//     DSL v1 sentinels like "ExitLoop" land here.
//  2. name is "UserFillUp" (case-insensitive) → UserFillUpNodeBody.
//     This route takes precedence over the regular factory path so
//     the eino interrupt semantics replace the legacy
//     UserFillUpComponent.Invoke body. UserFillUpNodeBody calls
//     compose.Interrupt on first execution and reads the resume
//     payload via compose.GetResumeContext on subsequent runs.
//  3. runtime.DefaultFactory() is non-nil → call the factory once to
//     construct a runtime.Component, then return a body that delegates
//     to that component's Invoke. A factory error surfaces here with
//     the cpn_id wrapped for diagnostics.
//  4. otherwise → placeholderBody. This is the canvas-package-only
//     fallback used when no factory has been registered (most commonly
//     in canvas-only unit tests that do not import the component
//     package). Production runs always have a factory installed via
//     component.init() → runtime.SetDefaultFactory(component.New).
//
// The returned body always tags the output map with __cpn_id__ so the
// shared statePost handler can persist the result into the per-cpn
// Outputs bucket. UserFillUpNodeBody tags its output itself so the
// interrupt-driven branch still attributes the resume payload to the
// right cpn.
// ctxKeyOverrideParams carries the run-level override map into
// BuildWorkflow so a component's params can be merged with it
// at compile time. The map is keyed by cpnID; each component only sees the
// entry for its own id (an arbitrary string-keyed map). It mirrors the ctx
// plumbing used for the per-run component factory
// (componentFactoryFromContext): the override is threaded through
// canvas.Compile → BuildWorkflow → buildNodeBody without the canvas
// package ever importing the ingestion layer.
const ctxKeyOverrideParams ctxKey = "canvas_override_params"

// withOverrideParams attaches a run-level override map to ctx. It is
// a no-op when m is nil so callers can pass a possibly-nil run parameter
// straight through.
func withOverrideParams(ctx context.Context, m map[string]any) context.Context {
	if m == nil {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyOverrideParams, m)
}

func overrideParamsFromContext(ctx context.Context) map[string]any {
	m, _ := ctx.Value(ctxKeyOverrideParams).(map[string]any)
	return m
}

// applyOverrideParams returns a clone of params with the per-component
// override (already resolved for this cpnID by the caller) merged into
// params. The override wins on top-level key collisions. The original
// params map is never mutated — the merge result is a fresh map —
// because the params come from the shared *Canvas and a per-run override
// must not leak into the next Run on the same Pipeline.
func applyOverrideParams(params, cpnOverride map[string]any) map[string]any {
	if len(cpnOverride) == 0 {
		return params
	}
	out := make(map[string]any, len(params)+len(cpnOverride))
	for k, v := range params {
		out[k] = v
	}
	for k, v := range cpnOverride {
		out[k] = v
	}
	return out
}

func buildNodeBody(ctx context.Context, cpnID, name string, params map[string]any) (nodeBodyFn, error) {
	if overrides := overrideParamsFromContext(ctx); len(overrides) > 0 {
		// overrides is keyed by cpnID; a component only sees its own
		// entry. Components absent from the map are left untouched.
		if cpnOverride, ok := overrides[cpnID].(map[string]any); ok && len(cpnOverride) > 0 {
			params = applyOverrideParams(params, cpnOverride)
		}
	}
	if isLegacyNoOp(name) {
		return legacyNoOpBody(cpnID), nil
	}
	// UserFillUp routes to the eino interrupt-based node body
	// regardless of whether the legacy UserFillUpComponent is
	// registered. The component's Invoke path renders tips / fields
	// but never emits an interrupt signal — it was the missing
	// producer half of the old sentinel chain. With this routing,
	// every UserFillUp node pauses the graph on first execution
	// (compose.Interrupt) and resumes from the orchestrator's
	// compose.ResumeWithData call.
	if strings.EqualFold(name, "UserFillUp") {
		return UserFillUpNodeBody(cpnID, params), nil
	}
	if factory := resolveComponentFactory(ctx); factory != nil {
		comp, err := factory(name, params)
		if err != nil {
			return nil, fmt.Errorf("canvas: component %q (%s): factory: %w", cpnID, name, err)
		}
		if comp == nil {
			return nil, fmt.Errorf("canvas: component %q (%s): factory returned nil component", cpnID, name)
		}
		// Pass the class name through to the body so the per-class
		// timeout resolver (resolveTimeout) can pick the right
		// timeout without the runtime.Component interface needing
		// to expose Name(). The factory returns the class name as
		// the DSL's `component_name` field, which is also what
		// ComponentBase.Name() would have returned.
		return realComponentBody(cpnID, name, comp), nil
	}
	// Fallback: no factory registered. This path is only exercised by
	// canvas-only unit tests; production wiring always installs a
	// factory via component.init().
	if !isKnownPrimitive(name) {
		return nil, fmt.Errorf("canvas: component %q has unknown component_name %q (typo? not in isKnownPrimitive, not in legacyNoOpNames)", cpnID, name)
	}
	return placeholderBody(cpnID), nil
}

// legacyNoOpBody returns the body installed for DSL v1 sentinel
// components (legacyNoOpNames). It echoes the input and tags
// __legacy_noop__ so downstream debuggers can tell the node fired but
// did nothing.

func resolveComponentFactory(ctx context.Context) runtime.ComponentFactory {
	if factory := componentFactoryFromContext(ctx); factory != nil {
		return factory
	}
	return runtime.DefaultFactory()
}

func legacyNoOpBody(cpnID string) nodeBodyFn {
	return func(_ context.Context, in map[string]any) (map[string]any, error) {
		out := make(map[string]any, len(in)+2)
		for k, v := range in {
			out[k] = v
		}
		out["__cpn_id__"] = cpnID
		out["__legacy_noop__"] = true
		return out, nil
	}
}

// componentTimeout returns the per-component Invoke timeout.
//
// Reads the COMPONENT_EXEC_TIMEOUT env var (seconds); defaults to 600s
// (10 min) to match the Python @timeout decorator's default in
// agent/component/base.py. Invalid / non-positive values fall back to
// the default — invalid input must never widen the timeout silently.
func componentTimeout() time.Duration {
	const def = 600 * time.Second
	if v := common.GetEnv(common.EnvComponentExecTimeout); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}
	return def
}

// realComponentBody returns a body that delegates to the supplied
// runtime.Component. The component is constructed once at build time
// (in buildNodeBody) and re-invoked per iteration.
//
// This is the SINGLE chokepoint through which every component Invoke
// passes — both the agent canvas and the ingestion pipeline
// (internal/ingestion/pipeline compiles a canvas and runs its workflow)
// reach components here. Cross-cutting concerns therefore belong here,
// not inside each component's Invoke:
//
//   - per-class timeout: context.WithTimeout from resolveTimeout
//     (4-level: per-class env → per-class defaults table → uniform env
//     → 600s fallback). The lookup is per-invocation (not per-body) so
//     operators can tune COMPONENT_EXEC_TIMEOUT[_<CLASS>] at runtime
//     without rebuilding graphs.
//   - progress: runtime.TrackProgress, with the callback pulled from
//     ctx (nil ⇒ no observer). This makes progress a framework-level
//     concern — components no longer wrap themselves.
//   - elapsed-time accounting: runtime.TrackElapsed stamps
//     _created_time / _elapsed_time into the output map so the
//     dataflow-result UI can show per-node timing without each
//     component repeating the bookkeeping.
//
// Timeout errors are surfaced as `timeout after Xs: <wrapped>`;
// parent-context cancellation as `cancelled: <wrapped>`; all other
// errors wrap the component's own error with the cpn_id for diagnostics.
//
// The output map is tagged with __cpn_id__ before return so statePost
// can attribute the result; if the component already populated that
// key it is overwritten with the canvas-controlled value to keep
// attribution authoritative.
func realComponentBody(cpnID, componentClass string, comp runtime.Component) nodeBodyFn {
	return func(ctx context.Context, in map[string]any) (map[string]any, error) {
		timeout := resolveTimeoutFromContext(ctx, componentClass)
		cctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		var out map[string]any
		invokeErr := runtime.TrackProgress(cpnID, runtime.ProgressCallbackFromContext(ctx), func() error {
			var e error
			out, e = runtime.TrackElapsed(componentClass, func() (map[string]any, error) {
				return comp.Invoke(cctx, in)
			})
			return e
		})
		if invokeErr != nil {
			switch {
			case errors.Is(invokeErr, context.DeadlineExceeded):
				return nil, fmt.Errorf("canvas: component %q invoke: timeout after %s: %w",
					cpnID, timeout, invokeErr)
			case errors.Is(invokeErr, context.Canceled):
				return nil, fmt.Errorf("canvas: component %q invoke: cancelled: %w", cpnID, invokeErr)
			}
			return nil, fmt.Errorf("canvas: component %q invoke: %w", cpnID, invokeErr)
		}
		if out == nil {
			out = make(map[string]any, 1)
		}
		out["__cpn_id__"] = cpnID
		return out, nil
	}
}

// placeholderBody is the canvas-only fallback used when no factory
// has been registered. It echoes the input map untouched (except for
// the __cpn_id__ tag) so canvas unit tests can exercise topology
// wiring without depending on any real component implementation.
func placeholderBody(cpnID string) nodeBodyFn {
	return func(ctx context.Context, in map[string]any) (map[string]any, error) {
		out, err := placeholderLambda(ctx, in)
		if err != nil {
			return nil, err
		}
		out["__cpn_id__"] = cpnID
		return out, nil
	}
}

// withStateBracket wraps body so that it performs the same pre/post
// state work as the outer-graph's eino StatePreHandler / StatePostHandler
// pair, but reads the state from the request context (attached via
// runtime.WithState) instead of an eino-managed graph-local state.
//
// This is the path used by the Loop sub-graph: its nodes do not have
// access to the outer graph's WithGenLocalState, but they do inherit
// the context-attached *CanvasState that the outer graph (or the
// invoking caller) installed. Wrapping the body lets sub-graph nodes
// participate in the same state snapshot / result-persistence
// contract as outer nodes.
//
// If no state is attached to ctx (e.g. a sub-graph test that runs
// the body directly), the wrapper degrades to a plain invocation:
// the body still runs, its output is still tagged with __cpn_id__,
// but no state snapshot is injected and no result is persisted.
func withStateBracket(cpnID, componentName string, body nodeBodyFn) nodeBodyFn {
	return func(ctx context.Context, in map[string]any) (map[string]any, error) {
		originalIn := in
		state, _, _ := runtime.GetStateFromContext[*runtime.CanvasState](ctx)
		if state != nil {
			nodeStartedAt(ctx, state, cpnID, componentName, componentName, originalIn)
			if in == nil {
				in = map[string]any{}
			}
			snapshot := state.Snapshot()
			wrapped := make(map[string]any, len(in)+1)
			for k, v := range in {
				wrapped[k] = v
			}
			wrapped["state"] = snapshot
			in = wrapped
		}
		out, err := body(ctx, in)
		if err != nil {
			if state != nil {
				nodeFinishedNow(ctx, state, cpnID, componentName, componentName, err)
			}
			return nil, err
		}
		if state == nil {
			return out, nil
		}
		if out == nil {
			nodeFinishedNow(ctx, state, cpnID, componentName, componentName, nil)
			return out, nil
		}
		outputCpnID, _ := out["__cpn_id__"].(string)
		if outputCpnID == "" {
			nodeFinishedNow(ctx, state, cpnID, componentName, componentName, nil)
			return out, nil
		}
		for k, v := range out {
			if k == "__cpn_id__" || k == "state" || k == "__legacy_noop__" {
				continue
			}
			state.SetVar(outputCpnID, k, v)
		}
		nodeFinishedNow(ctx, state, cpnID, componentName, componentName, nil)
		return out, nil
	}
}
