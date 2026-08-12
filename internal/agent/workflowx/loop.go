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

// Package workflowx is a zero-intrusion extension to the eino compose
// package. It does NOT modify eino source. Instead it provides a small
// set of helpers that build on eino's public API to add features the
// core package does not expose directly.
//
// Currently the package exposes one helper, AddLoopNode, which models
// "repeatedly execute a nested workflow until a condition is met" as
// a normal workflow node. See the .claude/plans/eino-workflow-loop.md
// plan for the design rationale.
//
// # Foundation for the canvas Loop component
//
// AddLoopNode is also the runtime driver for the RAGFlow agent canvas's
// "Loop" component (internal/agent/component/loop.go). The canvas engine
// (internal/agent/canvas/scheduler.go) recognises a "Loop" cpn in the
// DSL and uses buildLoopExpansion (canvas/loop_subgraph.go) to:
//
//  1. collect the Loop's downstream descendants into a sub-Workflow;
//  2. prepend a synthetic init lambda that seeds the DSL's
//     loop_variables into the per-run *CanvasState;
//  3. translate the DSL's loop_termination_condition list into a
//     LoopCondition[map[string]any] closure that reads the same state
//     slots via state.GetVar on every iteration; and
//  4. call AddLoopNode here to install a single eino node in place
//     of what would otherwise be a Python-era Loop + LoopItem pair.
//
// The condition operators (string / bool / number / dict / list / nil)
// and the AND/OR combiner implemented by translateLoopCondition are
// the same set that agent/component/loopitem.py:48-122 expresses in
// Python. The DSL's `loop_variables` initial value semantics
// (constant / variable / zero-init-by-type) match
// agent/component/loop.py:60-77.
package workflowx

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// LoopStreamMode controls how the loop node surfaces iteration streams
// to its downstream consumers. The first release supports two modes;
// see the plan §"Stream support" section for the rationale.
type LoopStreamMode string

const (
	// LoopStreamFinalOnly buffers all iterations and exposes ONLY the
	// final iteration's stream to the caller. This is the default and
	// the safest mode because downstream consumers cannot observe
	// intermediate iteration boundaries.
	LoopStreamFinalOnly LoopStreamMode = "final_only"

	// LoopStreamEveryIteration exposes each iteration's stream in
	// sequence.
	LoopStreamEveryIteration LoopStreamMode = "every_iteration"
)

// defaultMaxIterations is a safety guard used only when the caller does not
// configure an explicit limit. The cap is intentionally generous to
// accommodate real workflows (deep research, iterative refinement) but
// is finite so a bug in the quit condition cannot spin forever.
const defaultMaxIterations = 1024

// LoopCondition is the per-iteration exit predicate. It is invoked
// AFTER each completed iteration (i.e. after the sub-workflow has
// returned a value for iteration N), with N starting at 1.
//
// Parameters:
//   - ctx: the loop lambda's context. Cancellation here aborts the
//     loop the same as cancellation anywhere else in the run.
//   - iteration: 1-based index of the iteration that just finished.
//     iteration == 1 on the first call, iteration == 2 after the
//     second sub-workflow run, and so on.
//   - prev: the value that was fed INTO the just-finished iteration
//     as the sub-workflow's input. On iteration 1 this is the outer
//     loop node's input; on iteration N>1 it is the `next` value
//     produced by iteration N-1 (i.e. the previous iteration's
//     output, which becomes this iteration's input).
//   - next: the value the sub-workflow PRODUCED for this iteration.
//     If the predicate returns (true, nil) this is the value that
//     becomes the loop's final output.
//
// Returning (true, nil) ends the loop; the last `next` value becomes
// the loop's final output. Returning (false, nil) advances to the
// next iteration with `next` rewritten as the upcoming `prev`.
// Returning a non-nil error fails the entire loop run.
type LoopCondition[T any] func(ctx context.Context, iteration int, prev, next T) (bool, error)

// Sentinel errors. Tests use errors.Is to assert these.
var (
	// ErrLoopMaxIterationsExceeded is returned when the default safety
	// cap is reached without shouldQuit returning true. Explicit loop
	// limits are normal termination, matching the canvas Python runtime.
	ErrLoopMaxIterationsExceeded = errors.New("workflowx: loop max iterations exceeded")

	// ErrLoopQuitConditionFailed wraps a non-nil error returned by
	// the user-supplied LoopCondition. The loop aborts immediately.
	ErrLoopQuitConditionFailed = errors.New("workflowx: quit condition failed")
)

// LoopOption configures AddLoopNode. LoopOption follows the same
// functional-options pattern as the rest of the eino public API.
type LoopOption func(*loopOptions)

type loopOptions struct {
	maxIterations    int
	maxIterationsSet bool
	compileOpts      []compose.GraphCompileOption
	runOpts          []compose.Option
	streamMode       LoopStreamMode
	onStart          func(context.Context, any)
	onFinish         func(context.Context, error)
}

// WithLoopMaxIterations caps the loop at n iterations. The cap is checked
// AFTER each completed iteration. A value of 0 keeps the default safety cap in
// effect. A positive value is normal loop termination, not an error; this
// matches the canvas Python runtime's maximum_loop_count semantics. A value of
// 1 is legal and yields the single-iteration do-while case.
func WithLoopMaxIterations(n int) LoopOption {
	return func(o *loopOptions) {
		if n > 0 {
			o.maxIterations = n
			o.maxIterationsSet = true
		}
	}
}

// WithLoopCompileOptions appends compile options to the inner sub-
// workflow's Compile call.
func WithLoopCompileOptions(opts ...compose.GraphCompileOption) LoopOption {
	return func(o *loopOptions) {
		o.compileOpts = append(o.compileOpts, opts...)
	}
}

// WithLoopLifecycleHooks installs callbacks around the outer loop node's
// execution. Canvas uses this to emit node lifecycle events for the Loop macro
// without relying on eino state post-processing, which is unsafe for streamed
// multi-iteration output.
func WithLoopLifecycleHooks(onStart func(context.Context, any), onFinish func(context.Context, error)) LoopOption {
	return func(o *loopOptions) {
		o.onStart = onStart
		o.onFinish = onFinish
	}
}

// WithLoopRunOptions appends run options to every nested sub-workflow
// Invoke / Stream call. Use this to forward run-level options such as
// per-iteration callbacks or extra callbacks.
func WithLoopRunOptions(opts ...compose.Option) LoopOption {
	return func(o *loopOptions) {
		o.runOpts = append(o.runOpts, opts...)
	}
}

// WithLoopStream overrides the default LoopStreamFinalOnly mode.
// See the LoopStreamMode documentation for per-mode semantics.
func WithLoopStream(mode LoopStreamMode) LoopOption {
	return func(o *loopOptions) {
		if mode == LoopStreamFinalOnly || mode == LoopStreamEveryIteration {
			o.streamMode = mode
		}
	}
}

func getLoopOptions(opts []LoopOption) *loopOptions {
	o := &loopOptions{
		streamMode: LoopStreamFinalOnly,
	}
	for _, opt := range opts {
		opt(o)
	}
	if o.maxIterations == 0 {
		o.maxIterations = defaultMaxIterations
	}
	return o
}

// AddLoopNode appends a loop node to the outer workflow `wf`. The
// loop is wired as a single normal node: the caller can use the
// returned *WorkflowNode for AddInput / AddDependency just like
// every other node.
//
// The loop is implemented as an AnyLambda that internally invokes
// the supplied sub-workflow `sub` repeatedly until shouldQuit
// returns true. The outer graph remains acyclic because the loop
// lives entirely inside the lambda body — the only public
// contribution to wf is a single node.
//
// The lambda is stream-capable; the chosen LoopStreamMode controls
// how iteration streams are surfaced (see WithLoopStream).
//
// AddLoopNode compiles the sub-workflow immediately. Compile-time
// failures are returned as an error and the outer workflow is not
// modified, so the caller does not need to roll back any state on
// failure.
func AddLoopNode[T any](ctx context.Context, wf *compose.Workflow[T, T], key string, sub *compose.Workflow[T, T], shouldQuit LoopCondition[T], opts ...LoopOption) (*compose.WorkflowNode, error) {
	if wf == nil {
		return nil, errors.New("workflowx: outer workflow is nil")
	}
	if sub == nil {
		return nil, errors.New("workflowx: sub workflow is nil")
	}
	if shouldQuit == nil {
		return nil, errors.New("workflowx: shouldQuit is nil")
	}
	options := getLoopOptions(opts)

	// Compile the sub-workflow up front. Surface compile-time
	// failures directly so the caller never sees a half-built outer
	// workflow.
	compiled, err := sub.Compile(ctx, options.compileOpts...)
	if err != nil {
		return nil, fmt.Errorf("workflowx: compile sub workflow %q: %w", key, err)
	}

	// Build a stream-capable lambda. We use AnyLambda with
	// struct{} as the option payload type because the loop node
	// does not need per-call lambda options; eino passes zero-value
	// struct{} options at run time.
	lambda, err := compose.AnyLambda[T, T, struct{}](
		func(ctx context.Context, input T, _ ...struct{}) (T, error) {
			if options.onStart != nil {
				options.onStart(ctx, input)
			}
			out, err := runLoopInvoke(ctx, compiled, input, shouldQuit, options)
			if options.onFinish != nil {
				options.onFinish(ctx, err)
			}
			return out, err
		},
		func(ctx context.Context, input T, _ ...struct{}) (*schema.StreamReader[T], error) {
			if options.onStart != nil {
				options.onStart(ctx, input)
			}
			reader, err := runLoopStream(ctx, compiled, input, shouldQuit, options)
			if err != nil {
				if options.onFinish != nil {
					options.onFinish(ctx, err)
				}
				return nil, err
			}
			return wrapLoopStreamFinish(ctx, reader, options.onFinish), nil
		},
		nil,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("workflowx: build loop lambda: %w", err)
	}

	return wf.AddLambdaNode(key, lambda), nil
}

// runLoopInvoke executes the loop on the invoke path. It is the
// body of the loop lambda's Invoke handler.
func runLoopInvoke[T any](ctx context.Context, sub compose.Runnable[T, T], input T, shouldQuit LoopCondition[T], options *loopOptions) (T, error) {
	var zero T
	current := input
	iteration := 1

	for {
		next, runErr := sub.Invoke(ctx, current, options.runOpts...)
		if runErr != nil {
			return zero, fmt.Errorf("workflowx: iteration %d: %w", iteration, runErr)
		}

		// Evaluate the quit predicate. A non-nil error from
		// shouldQuit fails the loop.
		quit, qErr := shouldQuit(ctx, iteration, current, next)
		if qErr != nil {
			return zero, fmt.Errorf("%w: iteration %d: %v", ErrLoopQuitConditionFailed, iteration, qErr)
		}
		if quit {
			return next, nil
		}

		// Cap enforcement. The check uses iteration, not a pre-decrement,
		// so WithLoopMaxIterations(1) is the single-iteration do-while
		// case. Explicit caps are normal termination; the default cap is
		// the runaway-loop safety net and remains an error.
		if iteration >= options.maxIterations {
			if options.maxIterationsSet {
				return next, nil
			}
			return zero, fmt.Errorf("%w: %d", ErrLoopMaxIterationsExceeded, options.maxIterations)
		}

		current = next
		iteration++
	}
}

// runLoopStream executes the loop on the stream path. It mirrors
// runLoopInvoke but forwards (or buffers) the per-iteration streams
// to the caller. The implementation differs from the invoke path in
// two ways:
//
//  1. The sub-workflow is invoked via sub.Stream, which yields a
//     *schema.StreamReader per iteration.
//  2. The stream-mode policy decides whether each iteration's reader
//     is concatenated into a single output reader (FinalOnly) or
//     released eagerly (EveryIteration).
func runLoopStream[T any](
	ctx context.Context,
	sub compose.Runnable[T, T],
	input T,
	shouldQuit LoopCondition[T],
	options *loopOptions,
) (*schema.StreamReader[T], error) {
	var zero T
	current := input
	iteration := 1

	streamMode := options.streamMode
	// Pre-allocate the per-iteration readers we will concatenate.
	// The readers are populated lazily by `produce` below and then
	// consumed by the merge step. maxIterations is always set
	// (default or user-supplied), so it is a safe upper bound.
	streamReaders := make([]*schema.StreamReader[T], 0, options.maxIterations)

	pipeErrCh := make(chan error, 1)
	allClosed := make(chan struct{})

	// produce runs the loop body in a goroutine and feeds the
	// per-iteration stream readers into streamReaders. The first
	// error terminates the loop.
	go func() {
		defer close(allClosed)
		for {
			reader, serr := sub.Stream(ctx, current, options.runOpts...)
			if serr != nil {
				pipeErrCh <- fmt.Errorf("workflowx: iteration %d: %w", iteration, serr)
				return
			}

			// Materialize the iteration's stream so we can call
			// shouldQuit on the FINAL value emitted by the
			// sub-workflow. We also need the last value as the
			// `next` for the next iteration.
			collected, cerr := readAllStream(reader)
			if cerr != nil {
				pipeErrCh <- fmt.Errorf("workflowx: iteration %d stream: %w", iteration, cerr)
				return
			}
			if len(collected) == 0 {
				pipeErrCh <- fmt.Errorf("workflowx: iteration %d produced empty stream", iteration)
				return
			}
			next := collected[len(collected)-1]
			streamReaders = append(streamReaders, schema.StreamReaderFromArray(collected))

			quit, qErr := shouldQuit(ctx, iteration, current, next)
			if qErr != nil {
				pipeErrCh <- fmt.Errorf("%w: iteration %d: %v", ErrLoopQuitConditionFailed, iteration, qErr)
				return
			}
			if quit {
				return
			}
			if iteration >= options.maxIterations {
				if options.maxIterationsSet {
					return
				}
				pipeErrCh <- fmt.Errorf("%w: %d", ErrLoopMaxIterationsExceeded, options.maxIterations)
				return
			}
			current = next
			iteration++
		}
	}()

	// Bridge the produce goroutine and the merged output stream.
	outReader, outWriter := schema.Pipe[T](16)

	go func() {
		defer outWriter.Close()
		select {
		case <-allClosed:
			// Produce finished; emit the iteration streams
			// according to streamMode, then signal any error
			// from the goroutine.
			sendIterations(streamReaders, streamMode, outWriter)
			// Drain any pending error from produce.
			select {
			case err := <-pipeErrCh:
				if err != nil {
					outWriter.Send(zero, err)
				}
			default:
			}
		case err := <-pipeErrCh:
			// Produce emitted an error before closing; the
			// readers produced so far are still useful only
			// for LoopStreamEveryIteration. Re-emit them,
			// then surface the error.
			if streamMode == LoopStreamEveryIteration {
				sendIterations(streamReaders, streamMode, outWriter)
			}
			outWriter.Send(zero, err)
		}
	}()

	return outReader, nil
}

func wrapLoopStreamFinish[T any](ctx context.Context, reader *schema.StreamReader[T], onFinish func(context.Context, error)) *schema.StreamReader[T] {
	if onFinish == nil || reader == nil {
		return reader
	}
	outReader, outWriter := schema.Pipe[T](16)
	go func() {
		var zero T
		var finishErr error
		defer func() {
			reader.Close()
			outWriter.Close()
			onFinish(ctx, finishErr)
		}()
		for {
			v, err := reader.Recv()
			if err != nil {
				if errors.Is(err, io.EOF) {
					return
				}
				finishErr = err
				outWriter.Send(zero, err)
				return
			}
			if outWriter.Send(v, nil) {
				return
			}
		}
	}()
	return outReader
}

// sendIterations writes the supplied iteration readers to w. For
// LoopStreamEveryIteration every reader is forwarded in order; for
// LoopStreamFinalOnly only the last reader is forwarded (or none
// if there are no readers).
func sendIterations[T any](readers []*schema.StreamReader[T], mode LoopStreamMode, w *schema.StreamWriter[T]) {
	if len(readers) == 0 {
		return
	}
	emit := readers
	if mode == LoopStreamFinalOnly {
		emit = readers[len(readers)-1:]
	}
	for _, r := range emit {
		for {
			v, err := r.Recv()
			if err != nil {
				r.Close()
				if errors.Is(err, io.EOF) {
					break
				}
				return
			}
			if w.Send(v, nil) {
				r.Close()
				return
			}
		}
	}
}

// readAllStream drains sr and returns every value emitted. The
// reader is closed on return. EOF terminates the read; any other
// non-nil error is propagated.
func readAllStream[T any](sr *schema.StreamReader[T]) ([]T, error) {
	defer sr.Close()
	var out []T
	for {
		v, err := sr.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return out, nil
			}
			return out, err
		}
		out = append(out, v)
	}
}
