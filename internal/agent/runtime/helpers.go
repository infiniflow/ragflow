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

// Cross-cutting helpers that replace Python's `rag/flow/base.py:ProcessBase`
// wrapper (lines 33-63). Three call-site concerns are extracted into plain
// higher-order functions:
//
//	(a) timeout enforcement         -> WithTimeout
//	(b) progress callback fan-out   -> TrackProgress
//	(c) elapsed-time accounting     -> TrackElapsed
//
// These live in `runtime` (rather than as a `Component` interface method or
// a base type) because they are call-site concerns, not extension points.
// Both `internal/ingestion/pipeline` and `internal/agent/canvas` compose
// them at the DAG-node / goroutine boundary.
//
// LOSSY MAPPING (plan §8 R1):
//
// Python `ProcessBase._invoke` is wrapped by BOTH `asyncio.wait_for` AND
// the `@timeout` decorator — a dual-layer timeout to catch different
// failure modes. Go's `context.WithTimeout` collapses this into a single
// layer; `WithTimeout` covers the outer one (asyncio.wait_for equivalent).
// The inner `@timeout` decorator has no Go equivalent and is not
// replicated here. If a future requirement needs the inner layer,
// `WithTimeout` can be nested at the call site.
package runtime

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ProgressPhase classifies a component lifecycle event emitted by
// TrackProgress. The integer values are stable and persisted in the
// ingestion_task_log.phase column, so they are part of the data contract
// (see internal/ingestion/pipeline PROGRESS_LOG_RESUME_PLAN §5.1):
//
//	PhaseEnter = 0  component just started
//	PhaseExit  = 1  component finished cleanly
//	PhaseError = 2  component errored (Err carries the error)
type ProgressPhase int

const (
	PhaseEnter ProgressPhase = iota
	PhaseExit
	PhaseError
)

// ProgressEvent is a structured progress notification emitted by
// TrackProgress for every component lifecycle event.
//
// Component is the node id (cpnID) — the unique identifier of the node in
// the DSL graph, NOT the component class name. Class names cannot
// disambiguate multiple instances of the same class, so sinks must key on
// Component for attribution, ordering, and GROUP BY (plan §5.1).
//
// Err is non-nil only when Phase == PhaseError.
//
// ProgressEvent deliberately does NOT carry the component's output:
// resume is owned by the framework's eino checkpoint, so progress is
// purely observational (plan §5.1 / §5.3). Keeping the event free of
// output also avoids serializing large payloads on every event.
//
// Concrete sinks (ingestion task-log writer, in-memory test recorder)
// implement ProgressCallback. nil is a valid value: TrackProgress treats a
// nil cb as "no observer" and simply runs fn.
type ProgressEvent struct {
	Phase     ProgressPhase
	Component string
	Err       error
}

// ProgressCallback receives progress notifications from TrackProgress.
type ProgressCallback func(event ProgressEvent)

// TrackProgress wraps fn with progress notifications. The callback is
// invoked at most twice per call (once at start, once at end):
//
//	enter: cb(ProgressEvent{Phase: PhaseEnter, Component: cpnID})
//	exit:  cb(ProgressEvent{Phase: PhaseExit,  Component: cpnID})
//	error: cb(ProgressEvent{Phase: PhaseError, Component: cpnID, Err: err})
//
// A nil callback is permitted: fn runs to completion and its return value
// (including error) is passed through untouched.
//
// cpnID is the node id from the DSL graph. The canvas framework
// (internal/agent/canvas realComponentBody) is the single chokepoint that
// calls TrackProgress, so individual components must NOT call it
// themselves — that keeps the observer injection point in one place.
// realComponentBody pulls the callback from ctx via
// ProgressCallbackFromContext.
func TrackProgress(cpnID string, cb ProgressCallback, fn func() error) error {
	if cb != nil {
		cb(ProgressEvent{Phase: PhaseEnter, Component: cpnID})
	}
	err := fn()
	if cb == nil {
		return err
	}
	if err != nil {
		cb(ProgressEvent{Phase: PhaseError, Component: cpnID, Err: err})
		return err
	}
	cb(ProgressEvent{Phase: PhaseExit, Component: cpnID})
	return nil
}

// WithTimeout runs fn under a derived context that cancels either when
// d elapses or when the parent ctx is cancelled (whichever happens
// first). fn receives the child context so it can honor cancellation at
// its own yield points.
//
// On timeout: returns context.DeadlineExceeded (matching Python's
// asyncio.TimeoutError semantics).
// On parent cancellation: returns the parent ctx's error (typically
// context.Canceled).
// On fn completion within d: returns fn's error (may be nil).
//
// NOTES:
//
//   - This function implements ONLY the outer timeout layer that
//     Python `ProcessBase` enforces via `asyncio.wait_for`. The inner
//     `@timeout` decorator is not replicated in Go (see plan §8 R1).
//   - fn MUST NOT retain or use the ctx past return; once fn returns
//     the child context's cancel func is invoked by WithTimeout.
func WithTimeout(ctx context.Context, d time.Duration, fn func(ctx context.Context) error) error {
	childCtx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	if err := fn(childCtx); err != nil {
		// If fn honored cancellation, prefer the ctx error so callers
		// see a uniform "timed out" / "canceled" signal regardless of
		// whether fn propagated the error or replaced it.
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return err
		}
		if cerr := childCtx.Err(); cerr != nil {
			return cerr
		}
		return err
	}
	// fn returned nil — but the deadline may have elapsed between
	// fn's last yield point and return. Surface that as
	// DeadlineExceeded so the caller sees a consistent timeout
	// signal rather than a false "success".
	if cerr := childCtx.Err(); cerr != nil {
		return cerr
	}
	return nil
}

// TrackElapsed records the wall-clock duration of fn and stamps the
// output map with two synthetic keys mirroring Python `ProcessBase`
// (base.py:42, 58):
//
//	"_created_time"  RFC3339Nano-formatted timestamp taken BEFORE fn runs.
//	"_elapsed_time"  float64 seconds (with sub-second precision) that
//	                 fn took to complete, in [0, +∞).
//
// Any keys already present in fn's result map are preserved verbatim;
// the two synthetic keys are added only if absent (fn-supplied values
// win on conflict — fn is the authoritative source of business data).
// This matches the Python ProcessBase convention: a component that
// computes its own elapsed time is trusted over the helper's stopwatch.
//
// On error: the returned map is nil and the error is propagated
// untouched. The "name" parameter is recorded in the error message
// when err is non-nil so log readers can attribute the elapsed
// accounting failure to a specific component.
func TrackElapsed(name string, fn func() (map[string]any, error)) (map[string]any, error) {
	start := time.Now()
	out, err := fn()
	elapsed := time.Since(start)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if out == nil {
		out = make(map[string]any)
	}
	if _, ok := out["_created_time"]; !ok {
		out["_created_time"] = start.UTC().Format(time.RFC3339Nano)
	}
	if _, ok := out["_elapsed_time"]; !ok {
		out["_elapsed_time"] = elapsed.Seconds()
	}
	return out, nil
}

// progressCBKey is the context key under which a ProgressCallback is
// carried so the canvas framework can fan progress out to an observer
// without every component knowing about it. The framework owns the
// callback; components only see their own work.
type progressCBKey struct{}

// WithProgressCallback attaches a ProgressCallback to ctx. The canvas
// framework reads it inside realComponentBody and forwards it to
// TrackProgress when a component runs, so progress reporting is a
// framework-level concern. A run that wants progress fan-out (e.g. the
// ingestion pipeline's task log writer) injects one; when none is set,
// ProgressCallbackFromContext returns nil and TrackProgress is a no-op.
func WithProgressCallback(ctx context.Context, cb ProgressCallback) context.Context {
	return context.WithValue(ctx, progressCBKey{}, cb)
}

// ProgressCallbackFromContext returns the ProgressCallback attached to
// ctx, or nil if none was set. TrackProgress treats a nil callback as
// "no observer" and simply runs fn.
func ProgressCallbackFromContext(ctx context.Context) ProgressCallback {
	if ctx == nil {
		return nil
	}
	if cb, ok := ctx.Value(progressCBKey{}).(ProgressCallback); ok {
		return cb
	}
	return nil
}
