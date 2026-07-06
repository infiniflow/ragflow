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

// ProgressCallback receives progress notifications from TrackProgress.
// The numeric progress values follow the convention used by the Python
// pipeline canvas callback:
//
//	progress=0  before fn runs       (component just started)
//	progress=1  on success           (component finished cleanly)
//	progress=-1 on failure           (component errored; message is the error)
//
// Concrete sinks (Redis log writer, in-memory test recorder) implement
// this signature. nil is a valid value: TrackProgress treats a nil cb as
// "no observer" and simply runs fn.
type ProgressCallback func(progress int, message string)

// TrackProgress wraps fn with progress notifications. The callback is
// invoked at most twice per call (once at start, once at end).
//
// On success: cb(1, "<compName> Done") and nil error.
// On failure: cb(-1, "<compName>: <err>") and the original error.
//
// A nil callback is permitted: fn runs to completion and its return
// value (including error) is passed through untouched.
func TrackProgress(compName string, cb ProgressCallback, fn func() error) error {
	if cb != nil {
		cb(0, compName+" Started")
	}
	err := fn()
	if cb == nil {
		return err
	}
	if err != nil {
		cb(-1, fmt.Sprintf("%s: %s", compName, err.Error()))
		return err
	}
	cb(1, compName+" Done")
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
