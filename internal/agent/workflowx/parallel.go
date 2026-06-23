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
// once per input item, with bounded concurrency, and supports
// per-item interrupt / resume. The shape mirrors AddLoopNode:
// the outer workflow sees a single node; the fan-out is entirely
// inside the lambda body.
//
// The first release is invoke-only on the outer lambda; the inner
// per-item sub-workflow is invoked via runner.Invoke.
//
// See .claude/plans/eino-workflow-parallel.md (and
// .omc/autopilot/spec.md) for the design rationale.
package workflowx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// ParallelAddressSegment is the per-item address segment used when
// addressing interrupts. It mirrors the batch node's
// AddressSegmentBatchProcess.
const ParallelAddressSegment compose.AddressSegmentType = "workflowx-parallel"

// Sentinel errors for the parallel extension. Tests use errors.Is
// to assert these.
var (
	// ErrParallelCompileFailed wraps a compile-time failure of the
	// inner sub-workflow. The original error from sub.Compile is
	// reachable via errors.Unwrap.
	ErrParallelCompileFailed = errors.New("workflowx: parallel sub-workflow compile failed")

	// ErrParallelResumeStateInvalid is returned when a resume is
	// requested but the persisted state is missing, malformed, or
	// has an empty Inputs slice.
	ErrParallelResumeStateInvalid = errors.New("workflowx: parallel resume state invalid")
)

// ParallelOption configures AddParallelNode. Follows the
// functional-options pattern.
type ParallelOption func(*parallelOptions)

type parallelOptions struct {
	maxConcurrency      int
	compileOpts         []compose.GraphCompileOption
	runOpts             []compose.Option
	checkpointBuilder   func(nodeKey string, index int) string
	enableSubCheckpoint bool
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
// sub-workflow's Compile call. Useful for wiring a Serializer or a
// caller-managed CheckPointStore on the inner sub-graph.
//
// Note: the parallel extension always passes its own bridge
// CheckPointStore first (when sub-checkpoint is enabled), so any
// store set via this option will not collide with the bridge store.
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

// WithParallelCheckpointIDBuilder supplies a deterministic checkpoint
// ID for each per-item sub-workflow invocation. eino does not expose
// the active outer checkpoint ID through ctx, so the extension
// cannot derive child IDs by itself.
//
// The default is a reserved-namespace builder
// (workflowx-parallel:<nodeKey>:<index>) which is deterministic and
// stable across resumes. Callers that need a shared prefix can
// capture it in the closure.
//
// The builder is invoked on the first run AND on resume, with stable
// (nodeKey, index) arguments. An empty return is treated as "skip
// the per-item WithCheckPointID" so the inner task does not get a
// bad namespace.
func WithParallelCheckpointIDBuilder(b func(nodeKey string, index int) string) ParallelOption {
	return func(o *parallelOptions) {
		if b != nil {
			o.checkpointBuilder = b
		}
	}
}

// WithParallelEnableSubCheckpoint opts the parallel node into
// passing compose.WithCheckPointID(...) and an internal bridge
// store to the sub-workflow on every per-item Invoke call.
//
// The default is true. Disabling it is only useful when the caller
// explicitly wants the smaller/no-sub-checkpoint behavior.
func WithParallelEnableSubCheckpoint(enable bool) ParallelOption {
	return func(o *parallelOptions) {
		o.enableSubCheckpoint = enable
	}
}

// defaultParallelCheckpointBuilder returns a deterministic per-item
// checkpoint ID. Unlike the loop extension, the parallel extension
// does not need a UUID in the default because the same item index
// is naturally re-derived from the persisted InterruptedIndices on
// resume — so the same ID is reused.
func defaultParallelCheckpointBuilder(nodeKey string, index int) string {
	return fmt.Sprintf("workflowx-parallel:%s:%d", nodeKey, index)
}

func getParallelOptions(opts []ParallelOption) *parallelOptions {
	o := &parallelOptions{
		checkpointBuilder:   defaultParallelCheckpointBuilder,
		enableSubCheckpoint: true,
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// ParallelInterruptState is the parallel-local checkpoint payload.
// It is persisted as the state argument of
// compose.CompositeInterrupt so a resumed run can continue from the
// interrupted items rather than restart.
//
// The struct mirrors the reference batch node's NodeInterruptState,
// with one important adaptation: OriginalInputs is stored as a
// JSON byte slice (not []any) so the parallel extension can
// re-decode it with the original Go types on resume. JSON's
// default behaviour of decoding numbers into float64 would
// otherwise break integer and other typed inputs.
type ParallelInterruptState struct {
	// OriginalInputsJSON is the JSON encoding of the input slice
	// as seen by the parallel lambda on first run. On resume
	// the lambda input is replaced by a zero value by eino's
	// rerun mechanism; this byte slice is the source of truth.
	OriginalInputsJSON []byte `json:"original_inputs_json"`

	// CompletedResults carries every index that already produced
	// a value on a previous (interrupted) run.
	CompletedResults map[int]any `json:"completed_results"`

	// InterruptedIndices is the list of indices that were not
	// durably confirmed completed at the interrupt boundary.
	// In the common case this equals "the items whose sub-workflow
	// Invoke returned an interrupt". Under concurrent execution,
	// however, any item that is not present in CompletedResults is
	// treated conservatively as needing replay / resume, because its
	// precise execution state may be unknown when the outer node
	// returns a CompositeInterrupt.
	InterruptedIndices []int `json:"interrupted_indices"`

	// TotalCount is the size of the input slice. It is the source
	// of truth for the output slice length on resume.
	TotalCount int `json:"total_count"`

	// ItemCheckpoints is the per-item bridge-store payload captured
	// at interrupt time. Keys are the per-item child checkpoint
	// IDs (whatever the configured builder produced).
	ItemCheckpoints map[string][]byte `json:"item_checkpoints,omitempty"`
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

	// Build a fresh per-node bridge store. It is captured in the
	// lambda's closure and rehydrated from ItemCheckpoints on
	// resume.
	bridgeState := newParallelBridgeState(nil)

	compileOpts := append([]compose.GraphCompileOption{}, options.compileOpts...)
	if options.enableSubCheckpoint {
		compileOpts = append(compileOpts, compose.WithCheckPointStore(bridgeState.store()))
	}
	compiled, err := sub.Compile(ctx, compileOpts...)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrParallelCompileFailed, key, err)
	}

	lambda, err := compose.AnyLambda[[]I, []O, struct{}](
		func(ctx context.Context, items []I, _ ...struct{}) ([]O, error) {
			return runParallelInvoke(ctx, key, compiled, items, options, bridgeState)
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
// handler. It implements the documented state machine:
//
//   - On a fresh run: process every item 0..len(items)-1 with an
//     empty CompletedResults map.
//   - On a resume: process exactly prev.InterruptedIndices, with
//     prev.CompletedResults pre-populated into the output slice.
//     On resume the lambda's items input is replaced by a zero
//     value by eino's rerun mechanism; the canonical inputs come
//     from prev.OriginalInputs.
//   - If any items interrupt, return a single CompositeInterrupt
//     carrying all per-item interrupt errors and a state that lets
//     a resumed run re-enter deterministically.
//   - On a non-interrupt error, return the first one (wrapped per
//     "item %d: %w") and discard the other items' results.
func runParallelInvoke[I, O any](
	ctx context.Context,
	nodeKey string,
	sub compose.Runnable[I, O],
	items []I,
	options *parallelOptions,
	defaultBridge *parallelBridgeState,
) ([]O, error) {
	prev, isResume, resumeErr := loadParallelSnapshot(ctx)
	if resumeErr != nil {
		return nil, resumeErr
	}
	// On a resume, eino's rerun mechanism passes a zero-value
	// items slice to the lambda. The canonical inputs come from
	// the persisted state. On a fresh run, items is the user's
	// input and the persisted state is empty.
	effectiveItems := items
	if isResume && prev != nil {
		var restored []I
		if rErr := json.Unmarshal(prev.OriginalInputsJSON, &restored); rErr != nil {
			return nil, fmt.Errorf("%w: decode original_inputs_json: %v", ErrParallelResumeStateInvalid, rErr)
		}
		effectiveItems = restored
	}
	if len(effectiveItems) == 0 {
		return []O{}, nil
	}

	// Allocate output slice. On resume, the total count is the
	// persisted value; on first run, it is the input length.
	totalCount := len(effectiveItems)
	indicesToProcess := make([]int, len(effectiveItems))
	for i := range effectiveItems {
		indicesToProcess[i] = i
	}

	bridgeState := defaultBridge
	outputs := make([]O, totalCount)

	if isResume && prev != nil {
		totalCount = prev.TotalCount
		if totalCount < 0 {
			return nil, fmt.Errorf("%w: negative total_count", ErrParallelResumeStateInvalid)
		}
		outputs = make([]O, totalCount)
		// Replay completed results into the correct output slots.
		// The persisted value came through a JSON round-trip so
		// numeric types are float64; we coerce to O via an
		// intermediate any round-trip.
		for idx, v := range prev.CompletedResults {
			if idx < 0 || idx >= totalCount {
				return nil, fmt.Errorf("%w: completed index %d out of range", ErrParallelResumeStateInvalid, idx)
			}
			typed, ok := coerceAnyToO[O](v)
			if !ok {
				return nil, fmt.Errorf("%w: cannot coerce completed result at index %d (type %T) to target type", ErrParallelResumeStateInvalid, idx, v)
			}
			outputs[idx] = typed
		}
		// Only re-invoke the previously-interrupted indices.
		indicesToProcess = append([]int(nil), prev.InterruptedIndices...)
		// Rehydrate the bridge store from persisted ItemCheckpoints.
		bridgeState = newParallelBridgeState(prev.ItemCheckpoints)
	}

	// Run all items. The sequential / semaphore-bounded fan-out
	// is delegated to runParallelFanout.
	results := runParallelFanout(ctx, nodeKey, sub, effectiveItems, indicesToProcess, options, bridgeState)

	// Drain the result channel, categorising each entry.
	var normalErr error
	var interruptErrs []error
	completedResults := make(map[int]any)
	for r := range results {
		if r.err == nil {
			if r.index >= 0 && r.index < len(outputs) {
				if typed, ok := r.output.(O); ok {
					outputs[r.index] = typed
				}
			}
			completedResults[r.index] = r.output
			continue
		}
		if isInterruptError(r.err) {
			interruptErrs = append(interruptErrs, r.err)
			continue
		}
		// First non-interrupt error wins; we keep draining so
		// goroutines do not leak, but the caller will see this
		// normalErr and discard the rest.
		if normalErr == nil {
			normalErr = fmt.Errorf("item %d: %w", r.index, r.err)
		}
	}

	// Non-interrupt error: discard every other result, return the
	// first one (wrapped). No state is persisted.
	if normalErr != nil {
		return nil, normalErr
	}

	// Interrupt case: persist state and rethrow via CompositeInterrupt.
	// We store every non-completed index, not only the indices that
	// explicitly surfaced an interrupt. This preserves correctness if
	// a future implementation changes the fan-out to short-circuit or
	// cancel in-flight work at the first interrupt boundary.
	if len(interruptErrs) > 0 {
		inputsJSON, jErr := json.Marshal(effectiveItems)
		if jErr != nil {
			return nil, fmt.Errorf("workflowx: marshal original inputs: %w", jErr)
		}
		interruptedIndices := buildPendingIndices(totalCount, completedResults)
		state, sErr := encodeParallelState(ParallelInterruptState{
			OriginalInputsJSON: inputsJSON,
			CompletedResults:   completedResults,
			InterruptedIndices: interruptedIndices,
			TotalCount:         totalCount,
			ItemCheckpoints:    bridgeState.snapshot(),
		})
		if sErr != nil {
			return nil, fmt.Errorf("workflowx: encode parallel interrupt state: %w", sErr)
		}
		return nil, compose.CompositeInterrupt(ctx, nil, state, interruptErrs...)
	}

	return outputs, nil
}

// coerceAnyToO adapts a JSON-roundtripped any to the typed
// output O. JSON decoding maps numeric types to float64 by
// default, so a value that originated as int, int64, float32,
// etc. comes back as float64. This helper covers the common
// numeric conversions; for non-numeric O, the direct assertion
// is used.
func coerceAnyToO[O any](v any) (O, bool) {
	var zero O
	if v == nil {
		return zero, false
	}
	if typed, ok := v.(O); ok {
		return typed, true
	}
	// JSON-decode coercion: float64 -> O when O is one of the
	// common numeric types. We use reflect-free type switches.
	switch any(zero).(type) {
	case int:
		if f, ok := v.(float64); ok {
			return any(int(f)).(O), true
		}
	case int64:
		if f, ok := v.(float64); ok {
			return any(int64(f)).(O), true
		}
	case int32:
		if f, ok := v.(float64); ok {
			return any(int32(f)).(O), true
		}
	case float32:
		if f, ok := v.(float64); ok {
			return any(float32(f)).(O), true
		}
	case float64:
		if f, ok := v.(float64); ok {
			return any(f).(O), true
		}
	case uint:
		if f, ok := v.(float64); ok {
			return any(uint(f)).(O), true
		}
	case uint64:
		if f, ok := v.(float64); ok {
			return any(uint64(f)).(O), true
		}
	case uint32:
		if f, ok := v.(float64); ok {
			return any(uint32(f)).(O), true
		}
	}
	return zero, false
}

// parallelResumeBackdoorKey is a context key used by unit tests
// to drive the resume path without going through eino's
// framework-managed checkpoint store. The production resume
// path uses compose.GetInterruptState; this is a test-only
// backdoor. Set via context.WithValue(ctx,
// parallelResumeBackdoorKey{}, payload) where payload is the
// JSON-encoded ParallelInterruptState.
type parallelResumeBackdoorKey struct{}

// loadParallelSnapshot reads the persisted parallel state from
// ctx if the current run is a resume. On a fresh run it returns
// (nil, false, nil).
//
// The loader first checks for a test-injected payload via
// parallelResumeBackdoorKey (so unit tests can drive the resume
// path directly), then falls back to eino's
// compose.GetInterruptState. The production resume path always
// goes through the second branch.
func loadParallelSnapshot(ctx context.Context) (*ParallelInterruptState, bool, error) {
	// Test backdoor: a hand-injected payload takes priority so
	// unit tests can drive resume without a real checkpoint
	// store. Production code never sets this key.
	if raw, ok := ctx.Value(parallelResumeBackdoorKey{}).([]byte); ok && len(raw) > 0 {
		var st ParallelInterruptState
		if err := json.Unmarshal(raw, &st); err != nil {
			return nil, false, fmt.Errorf("%w: decode state: %v", ErrParallelResumeStateInvalid, err)
		}
		if st.TotalCount < 0 {
			return nil, false, fmt.Errorf("%w: negative total_count", ErrParallelResumeStateInvalid)
		}
		if err := validateParallelSnapshot(&st); err != nil {
			return nil, false, err
		}
		return &st, true, nil
	}
	wasInterrupted, hasState, payload := compose.GetInterruptState[[]byte](ctx)
	if !wasInterrupted || !hasState {
		return nil, false, nil
	}
	var st ParallelInterruptState
	if err := json.Unmarshal(payload, &st); err != nil {
		return nil, false, fmt.Errorf("%w: decode state: %v", ErrParallelResumeStateInvalid, err)
	}
	if st.TotalCount < 0 {
		return nil, false, fmt.Errorf("%w: negative total_count", ErrParallelResumeStateInvalid)
	}
	if err := validateParallelSnapshot(&st); err != nil {
		return nil, false, err
	}
	return &st, true, nil
}

// encodeParallelState marshals the parallel state to the
// persistable form. Go's encoding/json natively encodes
// map[int]any with integer keys as JSON object string keys, so
// the round-trip preserves the type.
func encodeParallelState(s ParallelInterruptState) ([]byte, error) {
	return json.Marshal(s)
}

// buildPendingIndices returns the resume set for an interrupted run:
// every index in [0,totalCount) that is not durably present in
// CompletedResults. The returned slice is intentionally the full
// non-completed complement for safety: under concurrent execution,
// an item whose goroutine was still in-flight at the interrupt
// boundary is treated as needing replay.
func buildPendingIndices(totalCount int, completedResults map[int]any) []int {
	if totalCount <= 0 {
		return nil
	}
	pending := make([]int, 0, totalCount-len(completedResults))
	for idx := 0; idx < totalCount; idx++ {
		if _, ok := completedResults[idx]; ok {
			continue
		}
		pending = append(pending, idx)
	}
	return pending
}

// validateParallelSnapshot enforces the resume invariant:
// CompletedResults and InterruptedIndices must form a partition of
// [0,totalCount). Any hole means the resumed run cannot know whether
// the missing item never started, partially ran, or already caused
// side effects, so the state is rejected as invalid.
func validateParallelSnapshot(st *ParallelInterruptState) error {
	if st == nil {
		return nil
	}
	covered := make([]bool, st.TotalCount)
	for idx := range st.CompletedResults {
		if idx < 0 || idx >= st.TotalCount {
			return fmt.Errorf("%w: completed index %d out of range", ErrParallelResumeStateInvalid, idx)
		}
		if covered[idx] {
			return fmt.Errorf("%w: duplicate index %d across completed/interrupted sets", ErrParallelResumeStateInvalid, idx)
		}
		covered[idx] = true
	}
	for _, idx := range st.InterruptedIndices {
		if idx < 0 || idx >= st.TotalCount {
			return fmt.Errorf("%w: interrupted index %d out of range", ErrParallelResumeStateInvalid, idx)
		}
		if covered[idx] {
			return fmt.Errorf("%w: duplicate index %d across completed/interrupted sets", ErrParallelResumeStateInvalid, idx)
		}
		covered[idx] = true
	}
	for idx, ok := range covered {
		if !ok {
			return fmt.Errorf("%w: missing index %d from completed/interrupted partition", ErrParallelResumeStateInvalid, idx)
		}
	}
	return nil
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
// reported (success, interrupt, or error).
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
	bridgeState *parallelBridgeState,
) <-chan parallelTaskResult {
	resultCh := make(chan parallelTaskResult, len(indices))
	if len(indices) == 0 {
		close(resultCh)
		return resultCh
	}

	runOne := func(idx int) {
		// Derive the per-item checkpoint ID. The builder is
		// invoked on the first run AND on resume. An empty
		// return is treated as "no per-item id"; the option
		// is skipped.
		var cpID string
		if options.enableSubCheckpoint {
			cpID = options.checkpointBuilder(nodeKey, idx)
		}

		// Per-item address segment.
		subCtx := compose.AppendAddressSegment(ctx, ParallelAddressSegment, strconv.Itoa(idx))

		// Bridge store wiring for this item.
		subCtx = withParallelBridgeState(subCtx, bridgeState)

		invokeOpts := make([]compose.Option, 0, len(options.runOpts)+1)
		if options.enableSubCheckpoint && cpID != "" {
			invokeOpts = append(invokeOpts, compose.WithCheckPointID(cpID))
		}
		invokeOpts = append(invokeOpts, options.runOpts...)

		var out O
		var err error
		func() {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("item %d panic: %v", idx, r)
				}
			}()
			out, err = sub.Invoke(subCtx, items[idx], invokeOpts...)
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

// parallelBridgeStoreKey is the context key for the per-run
// parallel bridge state.
type parallelBridgeStoreKey struct{}

// parallelBridgeState is the in-memory map backing the per-item
// child checkpoints. It is owned by AddParallelNode and passed
// through ctx so the parallelBridgeStore can find it.
type parallelBridgeState struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func newParallelBridgeState(data map[string][]byte) *parallelBridgeState {
	cloned := cloneCheckpointMap(data)
	if cloned == nil {
		cloned = make(map[string][]byte)
	}
	return &parallelBridgeState{data: cloned}
}

func (s *parallelBridgeState) get(id string) ([]byte, bool) {
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

func (s *parallelBridgeState) set(id string, payload []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data == nil {
		s.data = make(map[string][]byte)
	}
	buf := make([]byte, len(payload))
	copy(buf, payload)
	s.data[id] = buf
}

func (s *parallelBridgeState) snapshot() map[string][]byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneCheckpointMap(s.data)
}

// parallelBridgeStore is the CheckPointStore implementation that
// reads/writes the parallel bridge state from ctx. It is registered
// on the inner sub-workflow's Compile call (when
// WithParallelEnableSubCheckpoint(true) is in effect).
type parallelBridgeStore struct{}

func newParallelBridgeStore() *parallelBridgeStore {
	return &parallelBridgeStore{}
}

func (s *parallelBridgeStore) Get(ctx context.Context, checkPointID string) ([]byte, bool, error) {
	state, ok := ctx.Value(parallelBridgeStoreKey{}).(*parallelBridgeState)
	if !ok || state == nil {
		return nil, false, nil
	}
	payload, found := state.get(checkPointID)
	return payload, found, nil
}

func (s *parallelBridgeStore) Set(ctx context.Context, checkPointID string, checkPoint []byte) error {
	state, ok := ctx.Value(parallelBridgeStoreKey{}).(*parallelBridgeState)
	if !ok || state == nil {
		return nil
	}
	state.set(checkPointID, checkPoint)
	return nil
}

// withParallelBridgeState wires the per-run bridge state into ctx.
func withParallelBridgeState(ctx context.Context, state *parallelBridgeState) context.Context {
	return context.WithValue(ctx, parallelBridgeStoreKey{}, state)
}

// store returns the per-run CheckPointStore that the inner
// sub-workflow should use. It is captured by closure when the
// inner sub-workflow is compiled.
func (s *parallelBridgeState) store() *parallelBridgeStore {
	return newParallelBridgeStore()
}
