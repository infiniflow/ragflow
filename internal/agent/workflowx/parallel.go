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

// Package workflowx parallel extension.
//
// AddParallelNode is a zero-intrusion helper that runs a sub-workflow
// once per input item, with bounded concurrency. The shape mirrors
// AddLoopNode: the outer workflow sees a single node; the fan-out is
// entirely inside the lambda body.
//
// The first release is invoke-only on the outer lambda; the inner
// per-item sub-workflow is invoked via runner.Invoke.
//
// See .claude/plans/eino-workflow-parallel.md (and
// .omc/autopilot/spec.md) for the design rationale.
package workflowx

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// Sentinel errors for the parallel extension. Tests use errors.Is
// to assert these.
var (
	// ErrParallelCompileFailed wraps a compile-time failure of the
	// inner sub-workflow. The original error from sub.Compile is
	// reachable via errors.Unwrap.
	ErrParallelCompileFailed = errors.New("workflowx: parallel sub-workflow compile failed")
)

// ParallelOption configures AddParallelNode. Follows the
// functional-options pattern.
type ParallelOption func(*parallelOptions)

type parallelOptions struct {
	maxConcurrency int
	compileOpts    []compose.GraphCompileOption
	runOpts        []compose.Option
	contextBuilder func(ctx context.Context, item any, index int) context.Context
}

// WithParallelMaxConcurrency caps the number of per-item sub-workflow
// invocations that run concurrently.
//
// n <= 1  — sequential execution on the calling goroutine (no
// goroutines are spawned for any input length).
// n  > 1  — bounded fan-out using a semaphore of size n; the first
// item still runs on the main goroutine.
//
// The default is 0 (sequential).
func WithParallelMaxConcurrency(n int) ParallelOption {
	return func(o *parallelOptions) {
		if n >= 0 {
			o.maxConcurrency = n
		}
	}
}

// WithParallelCompileOptions appends compile options to the inner
// sub-workflow's Compile call.
func WithParallelCompileOptions(opts ...compose.GraphCompileOption) ParallelOption {
	return func(o *parallelOptions) {
		o.compileOpts = append(o.compileOpts, opts...)
	}
}

// WithParallelRunOptions appends run options to every per-item
// sub-workflow Invoke call. Use this to forward run-level options
// such as per-item callbacks.
func WithParallelRunOptions(opts ...compose.Option) ParallelOption {
	return func(o *parallelOptions) {
		o.runOpts = append(o.runOpts, opts...)
	}
}

// WithParallelContextBuilder decorates the per-item sub-workflow
// context before Invoke. This lets callers attach item-scoped runtime
// state without changing the outer []I -> []O parallel API.
func WithParallelContextBuilder(
	b func(ctx context.Context, item any, index int) context.Context,
) ParallelOption {
	return func(o *parallelOptions) {
		if b != nil {
			o.contextBuilder = b
		}
	}
}

func getParallelOptions(opts []ParallelOption) *parallelOptions {
	o := &parallelOptions{}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// Compilable is the input type accepted by AddParallelNode. Both
// *compose.Graph[I, O] and *compose.Workflow[I, O] satisfy it.
type Compilable[I, O any] interface {
	Compile(ctx context.Context, opts ...compose.GraphCompileOption) (compose.Runnable[I, O], error)
}

// AddParallelNode appends a parallel-fanout node to the outer
// workflow. The fan-out is inside the lambda body; the outer graph
// sees one node.
//
// The lambda is invoke-only in v1; its Stream handler returns a
// documented error. Callers that need outer-stream parallelism
// should treat that as a future v2 plan.
//
// AddParallelNode compiles the sub-workflow immediately. Compile-
// time failures are returned as an error and the outer workflow
// is not modified.
func AddParallelNode[I, O any](
	ctx context.Context,
	wf *compose.Workflow[[]I, []O],
	key string,
	sub Compilable[I, O],
	opts ...ParallelOption,
) (*compose.WorkflowNode, error) {
	if wf == nil {
		return nil, errors.New("workflowx: outer workflow is nil")
	}
	if sub == nil {
		return nil, errors.New("workflowx: sub workflow is nil")
	}
	options := getParallelOptions(opts)

	compiled, err := sub.Compile(ctx, options.compileOpts...)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrParallelCompileFailed, key, err)
	}

	lambda, err := compose.AnyLambda[[]I, []O, struct{}](
		func(ctx context.Context, items []I, _ ...struct{}) ([]O, error) {
			return runParallelInvoke(ctx, key, compiled, items, options)
		},
		func(ctx context.Context, items []I, _ ...struct{}) (*schema.StreamReader[[]O], error) {
			return nil, errParallelOuterStreamUnsupported
		},
		nil,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("workflowx: build parallel lambda: %w", err)
	}

	return wf.AddLambdaNode(key, lambda), nil
}

// errParallelOuterStreamUnsupported is the documented v1 error
// returned from the outer Stream handler. Surfaced as a sentinel
// for tests to assert against via errors.Is.
var errParallelOuterStreamUnsupported = errors.New("workflowx: parallel node does not support outer stream in v1")

// ErrParallelOuterStreamUnsupported is exported so external tests
// can assert on it. The lambda's Stream handler returns this
// (wrapped) error.
var ErrParallelOuterStreamUnsupported = errParallelOuterStreamUnsupported

// runParallelInvoke is the body of the parallel lambda's Invoke
// handler. It processes every item in order (or with bounded
// concurrency) and returns the per-item outputs in input order.
// The first non-nil item error is returned; the other items'
// results are discarded.
func runParallelInvoke[I, O any](
	ctx context.Context,
	nodeKey string,
	sub compose.Runnable[I, O],
	items []I,
	options *parallelOptions,
) ([]O, error) {
	if len(items) == 0 {
		return []O{}, nil
	}

	indicesToProcess := make([]int, len(items))
	for i := range items {
		indicesToProcess[i] = i
	}
	outputs := make([]O, len(items))

	// Run all items. The sequential / semaphore-bounded fan-out
	// is delegated to runParallelFanout.
	results := runParallelFanout(ctx, nodeKey, sub, items, indicesToProcess, options)

	// Drain the result channel, categorising each entry.
	var normalErr error
	for r := range results {
		if r.err == nil {
			if r.index >= 0 && r.index < len(outputs) {
				if typed, ok := r.output.(O); ok {
					outputs[r.index] = typed
				}
			}
			continue
		}
		// First error wins; we keep draining so goroutines do not
		// leak, but the caller will see this normalErr and discard
		// the rest.
		if normalErr == nil {
			normalErr = fmt.Errorf("item %d: %w", r.index, r.err)
		}
	}

	if normalErr != nil {
		return nil, normalErr
	}
	return outputs, nil
}

// parallelTaskResult is the per-item outcome that the fan-out
// goroutines send back to the main loop. `output` is any so the
// fan-out helper can be shared by runParallelInvoke callers of
// arbitrary I, O; the consumer type-asserts back to O when filling
// the output slice.
type parallelTaskResult struct {
	index  int
	output any
	err    error
}

// runParallelFanout executes the per-item sub-workflow calls
// according to the configured concurrency policy and returns a
// channel of results. The channel is closed once every item has
// reported.
//
// Concurrency policy:
//   - maxConcurrency <= 1: strictly sequential, no goroutines
//     spawned (matches plan §"Concurrency policy" and the P0
//     acceptance criterion "no goroutine spawns for 0 or 1").
//   - maxConcurrency > 1: bounded fan-out via a buffered channel
//     semaphore of size maxConcurrency. The first item runs on
//     the main goroutine; subsequent items run in worker
//     goroutines that acquire the semaphore before invoking.
//
// Per-item panics are recovered and surfaced as a normal error
// wrapped with "item %d:" so the outer lambda never crashes.
func runParallelFanout[I, O any](
	ctx context.Context,
	nodeKey string,
	sub compose.Runnable[I, O],
	items []I,
	indices []int,
	options *parallelOptions,
) <-chan parallelTaskResult {
	resultCh := make(chan parallelTaskResult, len(indices))
	if len(indices) == 0 {
		close(resultCh)
		return resultCh
	}

	runOne := func(idx int) {
		subCtx := ctx
		if options.contextBuilder != nil {
			subCtx = options.contextBuilder(subCtx, items[idx], idx)
		}

		var out O
		var err error
		func() {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("item %d panic: %v", idx, r)
				}
			}()
			out, err = sub.Invoke(subCtx, items[idx], options.runOpts...)
		}()
		resultCh <- parallelTaskResult{index: idx, output: out, err: err}
	}

	// Strictly sequential path: no goroutines, regardless of
	// input length.
	if options.maxConcurrency <= 1 {
		for _, idx := range indices {
			runOne(idx)
		}
		close(resultCh)
		return resultCh
	}

	// Concurrent path. Use a buffered channel semaphore.
	sem := make(chan struct{}, options.maxConcurrency)
	var wg sync.WaitGroup
	for i, idx := range indices {
		wg.Add(1)
		idx := idx
		if i == 0 {
			// First task runs on the main goroutine.
			runOne(idx)
			wg.Done()
			continue
		}
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			runOne(idx)
		}()
	}
	go func() {
		wg.Wait()
		close(resultCh)
	}()
	return resultCh
}
