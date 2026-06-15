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
// Foundation for the canvas Loop component
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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/google/uuid"

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
	// sequence. Resume is iteration-granular: iterations already
	// fully published are not replayed, while the interrupted
	// iteration may be replayed from its start.
	LoopStreamEveryIteration LoopStreamMode = "every_iteration"
)

// defaultMaxIterations caps the loop when the caller does not
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
	// ErrLoopMaxIterationsExceeded is returned when the configured
	// (or default) iteration cap is reached without shouldQuit
	// returning true. The cap exists purely as a safety net.
	ErrLoopMaxIterationsExceeded = errors.New("workflowx: loop max iterations exceeded")

	// ErrLoopSubGraphInterrupted is wrapped around an interrupt error
	// emitted by the sub-workflow. The original interrupt is still
	// accessible via errors.Unwrap for callers that want to inspect
	// it.
	ErrLoopSubGraphInterrupted = errors.New("workflowx: sub-workflow interrupted")

	// ErrLoopResumeStateInvalid is returned when the loop is being
	// resumed but the saved state is missing, malformed, or refers
	// to a non-positive iteration. This is a hard failure: the loop
	// cannot safely continue from an inconsistent starting point.
	ErrLoopResumeStateInvalid = errors.New("workflowx: resume state invalid")

	// ErrLoopQuitConditionFailed wraps a non-nil error returned by
	// the user-supplied LoopCondition. The loop aborts immediately.
	ErrLoopQuitConditionFailed = errors.New("workflowx: quit condition failed")
)

// LoopOption configures AddLoopNode. LoopOption follows the same
// functional-options pattern as the rest of the eino public API.
type LoopOption func(*loopOptions)

type loopOptions struct {
	maxIterations     int
	compileOpts       []compose.GraphCompileOption
	runOpts           []compose.Option
	streamMode        LoopStreamMode
	checkpointBuilder func(nodeKey string, iteration int) string
	enableSubCheckpoint bool
}

// WithLoopMaxIterations caps the loop at n iterations. The cap is
// checked AFTER each completed iteration. A value of 0 keeps the
// default cap in effect. A value of 1 is legal and yields the
// do-while contract: the sub-workflow executes at least once and the
// loop exits immediately (subject to shouldQuit).
func WithLoopMaxIterations(n int) LoopOption {
	return func(o *loopOptions) {
		if n >= 0 {
			o.maxIterations = n
		}
	}
}

// WithLoopCompileOptions appends compile options to the inner sub-
// workflow's Compile call. Useful for wiring a CheckPointStore or
// Serializer just for the sub-graph.
func WithLoopCompileOptions(opts ...compose.GraphCompileOption) LoopOption {
	return func(o *loopOptions) {
		o.compileOpts = append(o.compileOpts, opts...)
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

// WithLoopCheckpointIDBuilder supplies a deterministic checkpoint ID
// for each sub-workflow invocation. eino does not expose the active
// outer checkpoint ID through ctx, so the loop extension cannot
// derive child IDs by itself.
//
// If the builder is not supplied, a reserved-namespace default is
// used that combines the loop node key and the iteration number with
// a UUID. The default is fine for ad-hoc invocations but does NOT
// guarantee re-entrant resume: a resumed run would derive a fresh
// UUID and the sub-workflow would not find the partial state from
// the interrupted run. Production callers that need checkpoint/
// resume MUST supply a builder that returns stable IDs across
// invocations (e.g. "<parent-id>:<nodeKey>:<iteration>").
func WithLoopCheckpointIDBuilder(b func(nodeKey string, iteration int) string) LoopOption {
	return func(o *loopOptions) {
		if b != nil {
			o.checkpointBuilder = b
		}
	}
}

// WithLoopEnableSubCheckpoint opts the loop into passing
// compose.WithCheckPointID to the sub-workflow on every nested
// Invoke/Stream call and persisting the nested sub-workflow
// checkpoint through an internal bridge store.
//
// The default is true. Disabling it is only useful when the caller
// explicitly wants the smaller/no-sub-checkpoint behavior and
// accepts that resume may replay the in-flight iteration from the
// beginning.
func WithLoopEnableSubCheckpoint(enable bool) LoopOption {
	return func(o *loopOptions) {
		o.enableSubCheckpoint = enable
	}
}

// defaultCheckpointBuilder returns a UUID-based child checkpoint ID.
// It is intentionally non-deterministic so that callers who do not
// configure WithLoopCheckpointIDBuilder get a fresh sub-checkpoint
// on every iteration, which is the safe default for invocations
// that do not need cross-run resume.
func defaultCheckpointBuilder(nodeKey string, iteration int) string {
	return fmt.Sprintf("workflowx-cp:%s:%d:%s", nodeKey, iteration, uuid.NewString())
}

func getLoopOptions(opts []LoopOption) *loopOptions {
	o := &loopOptions{
		streamMode:        LoopStreamFinalOnly,
		checkpointBuilder: defaultCheckpointBuilder,
		enableSubCheckpoint: true,
	}
	for _, opt := range opts {
		opt(o)
	}
	if o.maxIterations == 0 {
		o.maxIterations = defaultMaxIterations
	}
	return o
}

// loopInterruptState is the loop-local checkpoint payload. It is
// marshaled to []byte and stored as the state argument of
// StatefulInterrupt / CompositeInterrupt so a resumed run can
// continue from the interrupted iteration rather than restart.
//
// The struct intentionally avoids a generic field for the input
// value: storing CurrentInput as []byte (the JSON encoding produced
// by the loop itself) sidesteps the need for callers to register
// generic types with the schema package — see plan §"Type shape".
type loopInterruptState struct {
	Iteration       int                `json:"iteration"`
	CurrentInput    []byte             `json:"current_input"`
	StreamMode      LoopStreamMode     `json:"stream_mode"`
	SubCheckpointID string             `json:"sub_checkpoint_id"`
	SubCheckpoints  map[string][]byte  `json:"sub_checkpoints,omitempty"`
	ReplayChunks    [][]byte           `json:"replay_chunks,omitempty"`
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
func AddLoopNode[T any](
	ctx context.Context,
	wf *compose.Workflow[T, T],
	key string,
	sub *compose.Workflow[T, T],
	shouldQuit LoopCondition[T],
	opts ...LoopOption,
) (*compose.WorkflowNode, error) {
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
	compileOpts := append([]compose.GraphCompileOption{}, options.compileOpts...)
	if options.enableSubCheckpoint {
		compileOpts = append(compileOpts, compose.WithCheckPointStore(newLoopBridgeStore()))
	}
	compiled, err := sub.Compile(ctx, compileOpts...)
	if err != nil {
		return nil, fmt.Errorf("workflowx: compile sub workflow %q: %w", key, err)
	}

	// Build a stream-capable lambda. We use AnyLambda with
	// struct{} as the option payload type because the loop node
	// does not need per-call lambda options; eino passes zero-value
	// struct{} options at run time.
	lambda, err := compose.AnyLambda[T, T, struct{}](
		func(ctx context.Context, input T, _ ...struct{}) (T, error) {
			return runLoopInvoke(ctx, key, compiled, input, shouldQuit, options)
		},
		func(ctx context.Context, input T, _ ...struct{}) (*schema.StreamReader[T], error) {
			return runLoopStream(ctx, key, compiled, input, shouldQuit, options)
		},
		nil,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("workflowx: build loop lambda: %w", err)
	}

	return wf.AddLambdaNode(key, lambda), nil
}

// loopSnapshot captures the live state of an in-flight loop run.
// It is used by both invoke and stream paths to share the resume
// detection logic.
type loopSnapshot struct {
	startIteration int
	current        []byte // JSON-encoded current input
	streamMode     LoopStreamMode
	subCheckID     string
	subCheckpoints map[string][]byte
	replayChunks   [][]byte
}

// loadLoopSnapshot reads the loop state from the context if the
// current run is a resume. On a fresh run it returns a zero snapshot
// that drives iteration 1 with the outer input.
func loadLoopSnapshot[T any](ctx context.Context, defaultMode LoopStreamMode) (loopSnapshot, error) {
	wasInterrupted, hasState, payload := compose.GetInterruptState[[]byte](ctx)
	if !wasInterrupted || !hasState {
		return loopSnapshot{
			startIteration: 1,
			streamMode:     defaultMode,
		}, nil
	}
	var st loopInterruptState
	if err := json.Unmarshal(payload, &st); err != nil {
		return loopSnapshot{}, fmt.Errorf("%w: decode state: %v", ErrLoopResumeStateInvalid, err)
	}
	if st.Iteration < 1 {
		return loopSnapshot{}, fmt.Errorf("%w: bad iteration %d", ErrLoopResumeStateInvalid, st.Iteration)
	}
	if st.CurrentInput == nil {
		return loopSnapshot{}, fmt.Errorf("%w: missing current_input", ErrLoopResumeStateInvalid)
	}
	streamMode := st.StreamMode
	if streamMode == "" {
		streamMode = defaultMode
	}
		return loopSnapshot{
			startIteration: st.Iteration,
			current:        st.CurrentInput,
			streamMode:     streamMode,
			subCheckID:     st.SubCheckpointID,
			subCheckpoints: cloneCheckpointMap(st.SubCheckpoints),
			replayChunks:   cloneByteSlices(st.ReplayChunks),
		}, nil
	}

// encodeState marshals a loop snapshot to the persisted form.
func encodeState(s loopSnapshot) ([]byte, error) {
	return json.Marshal(loopInterruptState{
		Iteration:       s.startIteration,
		CurrentInput:    s.current,
		StreamMode:      s.streamMode,
		SubCheckpointID: s.subCheckID,
		SubCheckpoints:  cloneCheckpointMap(s.subCheckpoints),
		ReplayChunks:    cloneByteSlices(s.replayChunks),
	})
}

// runLoopInvoke executes the loop on the invoke path. It is the
// body of the loop lambda's Invoke handler. See the package doc and
// plan §"Invoke path" for the documented state machine.
func runLoopInvoke[T any](
	ctx context.Context,
	nodeKey string,
	sub compose.Runnable[T, T],
	input T,
	shouldQuit LoopCondition[T],
	options *loopOptions,
) (T, error) {
	var zero T

	snap, err := loadLoopSnapshot[T](ctx, options.streamMode)
	if err != nil {
		return zero, err
	}

	// Resolve the starting input. On a fresh run it is the outer
	// lambda input. On a resume it is the JSON blob we persisted.
	var current T
	if snap.startIteration == 1 && snap.current == nil {
		current = input
	} else {
		if err := json.Unmarshal(snap.current, &current); err != nil {
			return zero, fmt.Errorf("%w: decode current input: %v", ErrLoopResumeStateInvalid, err)
		}
	}

	iteration := snap.startIteration
	subCheckID := snap.subCheckID
	bridgeState := newLoopBridgeState(snap.subCheckpoints)

	for {
		// Derive the per-iteration child checkpoint ID. On a fresh
		// run this is the caller's builder output; on a resume the
		// saved ID is reused so the sub-workflow can pick up where
		// it left off.
		if subCheckID == "" {
			subCheckID = options.checkpointBuilder(nodeKey, iteration)
		}

		// Marshal the input we are about to feed the sub-workflow.
		// We persist the JSON so a resume can re-feed the same
		// input without re-running the previous iteration.
		currentJSON, err := json.Marshal(current)
		if err != nil {
			return zero, fmt.Errorf("workflowx: marshal iteration %d input: %w", iteration, err)
		}

		subCtx := withLoopBridgeState(ctx, bridgeState)
		next, runErr := sub.Invoke(subCtx, current, withSubCheckpoint(options.runOpts, subCheckID, options.enableSubCheckpoint)...)
		if runErr != nil {
			if isInterruptError(runErr) {
				// Persist loop state, then propagate the
				// interrupt so the outer graph sees a
				// composite interrupt that the caller can
				// resume via the standard eino
				// ResumeWithData / BatchResumeWithData
				// primitives.
				state, mErr := encodeState(loopSnapshot{
					startIteration: iteration,
					current:        currentJSON,
					streamMode:     snap.streamMode,
					subCheckID:     subCheckID,
					subCheckpoints: bridgeState.snapshot(),
				})
				if mErr != nil {
					return zero, fmt.Errorf("workflowx: encode interrupt state: %w", mErr)
				}
				// errors.Join preserves the sentinel via
				// errors.Is while the framework still
				// sees the composite interrupt error.
				return zero, errors.Join(ErrLoopSubGraphInterrupted,
					compose.CompositeInterrupt(ctx, nil, state, runErr))
			}
			return zero, fmt.Errorf("workflowx: iteration %d: %w", iteration, runErr)
		}

		// Evaluate the quit predicate. A non-nil error from
		// shouldQuit fails the loop.
		quit, qErr := shouldQuit(ctx, iteration, current, next)
		if qErr != nil {
			return zero, fmt.Errorf("%w: iteration %d: %v", ErrLoopQuitConditionFailed, iteration, qErr)
		}
		if quit {
			bridgeState.delete(subCheckID)
			return next, nil
		}

		// Cap enforcement. The check uses iteration, not a
		// pre-decrement, so WithLoopMaxIterations(1) is the
		// single-iteration do-while case.
		if iteration >= options.maxIterations {
			return zero, fmt.Errorf("%w: %d", ErrLoopMaxIterationsExceeded, options.maxIterations)
		}

		bridgeState.delete(subCheckID)
		current = next
		iteration++
		// Reset the per-iteration child ID so the next iteration
		// derives a fresh one.
		subCheckID = ""
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
//
// Interrupt propagation follows the same CompositeInterrupt pattern
// as the invoke path. The persisted state carries the StreamMode
// and the per-iteration sub-checkpoint ID so a resume can re-emit
// from the interrupted iteration.
//
// Resume semantics are mode-specific (per plan §"Checkpoint /
// Resume Design"):
//
//   - FinalOnly: only the in-flight final iteration's stream may be
//     re-emitted. Earlier iterations' streams were never exposed to
//     the caller.
//   - EveryIteration: the resumed run re-emits the full stream from
//     iteration 1. Downstream consumers MUST be replay-tolerant.
func runLoopStream[T any](
	ctx context.Context,
	nodeKey string,
	sub compose.Runnable[T, T],
	input T,
	shouldQuit LoopCondition[T],
	options *loopOptions,
) (*schema.StreamReader[T], error) {
	var zero T

	snap, err := loadLoopSnapshot[T](ctx, options.streamMode)
	if err != nil {
		return nil, err
	}

	var current T
	if snap.startIteration == 1 && snap.current == nil {
		current = input
	} else {
		if err := json.Unmarshal(snap.current, &current); err != nil {
			return nil, fmt.Errorf("%w: decode current input: %v", ErrLoopResumeStateInvalid, err)
		}
	}

	iteration := snap.startIteration
	subCheckID := snap.subCheckID
	bridgeState := newLoopBridgeState(snap.subCheckpoints)

	// Pre-allocate the per-iteration readers we will concatenate.
	// The readers are populated lazily by `produce` below and then
	// consumed by the merge step. We need an upper bound so we can
	// size the slice: maxIterations is always set (default or
	// user-supplied), so a bound of maxIterations - snap.startIteration + 1
	// is safe and tight.
	remaining := options.maxIterations - snap.startIteration + 1
	if remaining < 1 {
		remaining = 1
	}

	streamMode := snap.streamMode
	streamReaders := make([]*schema.StreamReader[T], 0, remaining)
	replayHistory := cloneByteSlices(snap.replayChunks)
	prefilledReplay := false
	if streamMode == LoopStreamEveryIteration && len(replayHistory) > 0 {
		// Resume is iteration-granular, not chunk-granular: we can
		// deterministically replay fully persisted iteration output,
		// but the public Eino APIs do not expose a reliable downstream
		// chunk cursor for "resume from the first un-emitted chunk".
		replayed, derr := decodeReplayChunks[T](replayHistory)
		if derr != nil {
			return nil, fmt.Errorf("%w: decode replay chunks: %v", ErrLoopResumeStateInvalid, derr)
		}
		streamReaders = append(streamReaders, schema.StreamReaderFromArray(replayed))
		prefilledReplay = true
	}
	pipeErrCh := make(chan error, 1)
	allClosed := make(chan struct{})

	// produce runs the loop body in a goroutine and feeds the
	// per-iteration stream readers into streamReaders. The first
	// error terminates the loop. On interrupt the loop persists
	// state and re-throws via CompositeInterrupt, which is
	// propagated to the pipe below.
	go func() {
		defer close(allClosed)
		for {
			if subCheckID == "" {
				subCheckID = options.checkpointBuilder(nodeKey, iteration)
			}
			currentJSON, jerr := json.Marshal(current)
			if jerr != nil {
				pipeErrCh <- fmt.Errorf("workflowx: marshal iteration %d input: %w", iteration, jerr)
				return
			}

			subCtx := withLoopBridgeState(ctx, bridgeState)
			reader, serr := sub.Stream(subCtx, current, withSubCheckpoint(options.runOpts, subCheckID, options.enableSubCheckpoint)...)
			if serr != nil {
				if isInterruptError(serr) {
					state, mErr := encodeState(loopSnapshot{
						startIteration: iteration,
						current:        currentJSON,
						streamMode:     streamMode,
						subCheckID:     subCheckID,
						subCheckpoints: bridgeState.snapshot(),
						replayChunks:   replayHistory,
					})
					if mErr != nil {
						pipeErrCh <- fmt.Errorf("workflowx: encode interrupt state: %w", mErr)
						return
					}
					pipeErrCh <- errors.Join(ErrLoopSubGraphInterrupted,
						compose.CompositeInterrupt(ctx, nil, state, serr))
					return
				}
				pipeErrCh <- fmt.Errorf("workflowx: iteration %d: %w", iteration, serr)
				return
			}

			// Materialize the iteration's stream so we can call
			// shouldQuit on the FINAL value emitted by the
			// sub-workflow. We also need the last value as the
			// `next` for the next iteration.
			collected, cerr := readAllStream(reader)
			if cerr != nil {
				if isInterruptError(cerr) {
					state, mErr := encodeState(loopSnapshot{
						startIteration: iteration,
						current:        currentJSON,
						streamMode:     streamMode,
						subCheckID:     subCheckID,
						subCheckpoints: bridgeState.snapshot(),
						replayChunks:   replayHistory,
					})
					if mErr != nil {
						pipeErrCh <- fmt.Errorf("workflowx: encode interrupt state: %w", mErr)
						return
					}
					pipeErrCh <- errors.Join(ErrLoopSubGraphInterrupted,
						compose.CompositeInterrupt(ctx, nil, state, cerr))
					return
				}
				pipeErrCh <- cerr
				return
			}
			if len(collected) == 0 {
				pipeErrCh <- fmt.Errorf("workflowx: iteration %d produced empty stream", iteration)
				return
			}
			next := collected[len(collected)-1]
			replay := collected
			if streamMode == LoopStreamFinalOnly {
				// Defer the decision: only the LAST
				// iteration's stream is exposed. We
				// accumulate all iterations here and
				// release the last one when the loop
				// ends. The intermediate readers are
				// kept referenced until the loop exits
				// to prevent premature stream close.
				replay = collected
			}

			if !(streamMode == LoopStreamEveryIteration && prefilledReplay && iteration == snap.startIteration) {
				streamReaders = append(streamReaders, schema.StreamReaderFromArray(replay))
			}
			prefilledReplay = false
			if streamMode == LoopStreamEveryIteration {
				encoded, eerr := encodeReplayChunks(collected)
				if eerr != nil {
					pipeErrCh <- fmt.Errorf("workflowx: encode replay chunks: %w", eerr)
					return
				}
				replayHistory = append(replayHistory, encoded...)
			}

			quit, qErr := shouldQuit(ctx, iteration, current, next)
			if qErr != nil {
				pipeErrCh <- fmt.Errorf("%w: iteration %d: %v", ErrLoopQuitConditionFailed, iteration, qErr)
				return
			}
			if quit {
				bridgeState.delete(subCheckID)
				return
			}
			if iteration >= options.maxIterations {
				pipeErrCh <- fmt.Errorf("%w: %d", ErrLoopMaxIterationsExceeded, options.maxIterations)
				return
			}
			bridgeState.delete(subCheckID)
			current = next
			iteration++
			subCheckID = ""
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
			// Drain any pending error. If we get an interrupt
			// or other error from produce, surface it.
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
			// for LoopStreamEveryIteration. We re-emit them
			// so the resume contract is honored, then
			// surface the error.
			if streamMode == LoopStreamEveryIteration {
				sendIterations(streamReaders, streamMode, outWriter)
			}
			outWriter.Send(zero, err)
		}
	}()

	return outReader, nil
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
// non-nil error is propagated so interrupt-like errors are not lost.
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

// isInterruptError reports whether err is an eino interrupt signal
// (Interrupt, StatefulInterrupt, CompositeInterrupt, sub-graph
// interrupt, or the deprecated and-rerun form). Detecting
// interrupts is required so we can persist loop state and re-throw
// via CompositeInterrupt.
func isInterruptError(err error) bool {
	if err == nil {
		return false
	}
	if _, ok := compose.ExtractInterruptInfo(err); ok {
		return true
	}
	if _, ok := compose.IsInterruptRerunError(err); ok {
		return true
	}
	return false
}

// withSubCheckpoint returns opts with a leading WithCheckPointID
// carrying the loop's per-iteration child id. The id is set
// on every nested Invoke/Stream so the sub-workflow's interrupt
// state is persisted under a stable key. The caller-supplied
// opts follow; a user-provided WithCheckPointID would shadow
// the loop's id, which is the intended precedence.
//
// enable gates the option injection. When false, the loop still
// propagates sub-workflow interrupts correctly, but the nested
// sub-workflow does not get a checkpoint namespace and resume of
// an in-flight iteration may replay from the beginning.
func withSubCheckpoint(opts []compose.Option, cpID string, enable bool) []compose.Option {
	if !enable {
		return opts
	}
	out := make([]compose.Option, 0, len(opts)+1)
	out = append(out, compose.WithCheckPointID(cpID))
	out = append(out, opts...)
	return out
}

type loopBridgeStoreKey struct{}

type loopBridgeState struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func newLoopBridgeState(data map[string][]byte) *loopBridgeState {
	cloned := cloneCheckpointMap(data)
	if cloned == nil {
		cloned = make(map[string][]byte)
	}
	return &loopBridgeState{data: cloned}
}

func (s *loopBridgeState) get(id string) ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[id]
	if !ok {
		return nil, false
	}
	buf := make([]byte, len(v))
	copy(buf, v)
	return buf, true
}

func (s *loopBridgeState) set(id string, payload []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data == nil {
		s.data = make(map[string][]byte)
	}
	buf := make([]byte, len(payload))
	copy(buf, payload)
	s.data[id] = buf
}

func (s *loopBridgeState) delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, id)
}

func (s *loopBridgeState) snapshot() map[string][]byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneCheckpointMap(s.data)
}

type loopBridgeStore struct{}

func newLoopBridgeStore() *loopBridgeStore {
	return &loopBridgeStore{}
}

func withLoopBridgeState(ctx context.Context, state *loopBridgeState) context.Context {
	return context.WithValue(ctx, loopBridgeStoreKey{}, state)
}

func (s *loopBridgeStore) Get(ctx context.Context, checkPointID string) ([]byte, bool, error) {
	state, ok := ctx.Value(loopBridgeStoreKey{}).(*loopBridgeState)
	if !ok || state == nil {
		return nil, false, nil
	}
	payload, found := state.get(checkPointID)
	return payload, found, nil
}

func (s *loopBridgeStore) Set(ctx context.Context, checkPointID string, checkPoint []byte) error {
	state, ok := ctx.Value(loopBridgeStoreKey{}).(*loopBridgeState)
	if !ok || state == nil {
		return nil
	}
	state.set(checkPointID, checkPoint)
	return nil
}

func cloneCheckpointMap(src map[string][]byte) map[string][]byte {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string][]byte, len(src))
	for k, v := range src {
		buf := make([]byte, len(v))
		copy(buf, v)
		dst[k] = buf
	}
	return dst
}

func cloneByteSlices(src [][]byte) [][]byte {
	if len(src) == 0 {
		return nil
	}
	dst := make([][]byte, len(src))
	for i, v := range src {
		buf := make([]byte, len(v))
		copy(buf, v)
		dst[i] = buf
	}
	return dst
}

func encodeReplayChunks[T any](chunks []T) ([][]byte, error) {
	if len(chunks) == 0 {
		return nil, nil
	}
	out := make([][]byte, 0, len(chunks))
	for _, chunk := range chunks {
		b, err := json.Marshal(chunk)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}

func decodeReplayChunks[T any](chunks [][]byte) ([]T, error) {
	if len(chunks) == 0 {
		return nil, nil
	}
	out := make([]T, 0, len(chunks))
	for _, chunk := range chunks {
		var v T
		if err := json.Unmarshal(chunk, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}
