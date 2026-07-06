// Package graph — Loop macro support.
//
// NewLoopNodeFunc wraps a compiled sub-graph in a NodeFunc closure that
// repeatedly invokes the sub-graph until a LoopCondition returns true.
// The loop lives entirely inside the closure — the outer StateGraph sees
// a single node.
//
// Design (independent sub-graph pattern):
//
//   - Fresh run: iterate from iteration 1 using the outer input.
//   - Sub-graph interrupt: the closure captures its own state
//     (iteration, current input, sub-checkpoint ID), serialises it to
//     JSON, and re-throws via interrupt.Interrupt(ctx, encodedState).
//     The outer Pregel engine saves a checkpoint and surfaces the
//     GraphInterrupt error.
//   - Resume: the closure is re-invoked. It reads the encoded state
//     from interrupt.GetResumeValues(ctx), decodes iteration/input/
//     sub-checkpoint ID, and continues from where it left off.
//     The sub-graph is invoked with a RunnableConfig whose ThreadID
//     incorporates the sub-checkpoint namespace, so the sub-graph's
//     own checkpointer can resume the interrupted sub-iteration.
//   - Termination: when shouldQuit returns true or max iterations is
//     reached, the last iteration's output is returned as the node's
//     output.
package graph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"ragflow/internal/common"
	"ragflow/internal/harness/graph/interrupt"
	"ragflow/internal/harness/graph/types"
)

// LoopCondition is the per-iteration exit predicate invoked AFTER each
// completed iteration. Return (true, nil) to terminate the loop; the
// `next` value becomes the loop's final output.
// Return (false, nil) to continue to the next iteration.
type LoopCondition func(ctx context.Context, iteration int, prev, next interface{}) (bool, error)

// LoopOption configures NewLoopNodeFunc.
type LoopOption func(*loopOptions)

type loopOptions struct {
	maxIterations      int
	checkpointIDPrefix string
}

// WithLoopMaxIterations caps the loop at n iterations. Default 1024.
func WithLoopMaxIterations(maxIterations int) LoopOption {
	return func(o *loopOptions) {
		if maxIterations >= 0 {
			o.maxIterations = maxIterations
		}
	}
}

// WithLoopCheckpointIDPrefix sets a stable prefix for per-iteration
// sub-graph checkpoint IDs. Defaults to the node key.
func WithLoopCheckpointIDPrefix(prefix string) LoopOption {
	return func(o *loopOptions) {
		if prefix != "" {
			o.checkpointIDPrefix = prefix
		}
	}
}

func getLoopOptions(opts []LoopOption) *loopOptions {
	o := &loopOptions{
		maxIterations:      1024,
		checkpointIDPrefix: "",
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// Sentinel errors.
var (
	ErrLoopMaxIterationsExceeded = errors.New("graph: loop max iterations exceeded")
	ErrLoopSubgraphInterrupted   = errors.New("graph: sub-graph interrupted")
	ErrLoopResumeStateInvalid    = errors.New("graph: loop resume state invalid")
)

// loopStateCtxKey is the context key for storing loop sub-graph state
// during checkpoint restore. It is separate from the interrupt resume
// values so that UserFillUp's consumeNextResumeValue doesn't accidentally
// consume the loop state instead of the user's follow-up input.
type loopStateCtxKeyType struct{}

var loopStateCtxKey = loopStateCtxKeyType{}

// loopInterruptState is the JSON-serialised loop state saved when the
// sub-graph emits an interrupt.
type loopInterruptState struct {
	Iteration       int             `json:"iteration"`
	CurrentInput    json.RawMessage `json:"current_input,omitempty"`
	UserFillUpValue json.RawMessage `json:"user_fill_up_value,omitempty"`
}

// NewLoopNodeFunc wraps a compiled sub-graph into a NodeFunc that loops.
//
// The returned NodeFunc can be added directly to a StateGraph via
// sg.AddNode(key, nodeFunc). The outer graph sees one node; the loop
// lives entirely inside the closure.
//
// key is the node name (used for deterministic sub-checkpoint IDs).
// sub is the already-compiled sub-graph to invoke each iteration.
// shouldQuit is the termination predicate (called after each iteration).
func NewLoopNodeFunc(
	key string,
	sub *compiledGraph,
	shouldQuit LoopCondition,
	opts ...LoopOption,
) (types.NodeFunc, error) {
	if key == "" {
		return nil, fmt.Errorf("graph: loop key is empty")
	}
	if sub == nil {
		return nil, fmt.Errorf("graph: loop sub-graph is nil")
	}
	if shouldQuit == nil {
		return nil, fmt.Errorf("graph: loop shouldQuit is nil")
	}
	options := getLoopOptions(opts)
	if options.checkpointIDPrefix == "" {
		options.checkpointIDPrefix = key
	}

	nodeFunc := func(ctx context.Context, state interface{}) (interface{}, error) {
		return runLoop(ctx, key, sub, state, shouldQuit, options)
	}
	return nodeFunc, nil
}

// runLoop implements the loop node body.
func runLoop(
	ctx context.Context,
	key string,
	sub *compiledGraph,
	input interface{},
	shouldQuit LoopCondition,
	options *loopOptions,
) (interface{}, error) {
	// Check for resume values (set by interrupt.Interrupt on a previous run).
	snap, isResume := loadLoopSnapshot(ctx)

	var current interface{}
	startIteration := 1

	if isResume {
		startIteration = snap.Iteration
		if snap.CurrentInput != nil {
			var decoded interface{}
			if err := json.Unmarshal(snap.CurrentInput, &decoded); err != nil {
				return nil, fmt.Errorf("%w: decode current input: %v", ErrLoopResumeStateInvalid, err)
			}
			current = decoded
		} else {
			current = input
		}
	} else {
		current = input
	}
	// Guard against nil input. The sub-graph's inlineApplyInput
	// (inlineToMap) fails on nil with "nil value". An empty map
	// is safe and harmless for the first iteration.
	if current == nil {
		current = make(map[string]any)
	}

	iteration := startIteration

	for {
		// Build sub-checkpoint thread ID for this iteration.
		subCheckpointID := fmt.Sprintf("%s:loop:%s:iter:%d", options.checkpointIDPrefix, key, iteration)

		// Set up config for sub-graph invocation with per-iteration checkpoint.
		subCfg := &types.RunnableConfig{Configurable: make(map[string]interface{})}
		subCfg.Configurable["thread_id"] = subCheckpointID

		// Invoke sub-graph.
		next, invokeErr := sub.Invoke(ctx, current, subCfg)
		if invokeErr != nil {
			// Check if it's an interrupt from the sub-graph.
			if interrupt.IsInterrupt(invokeErr) {
				// Encode loop state and re-throw via interrupt.Interrupt
				// so the outer engine receives a GraphInterrupt with a
				// non-nil Interrupts list (required for the engine's
				// IsGraphInterrupt→interrupt detection chain).  Preserve
				// the original UserFillUp value inside the state so
				// MustExtractInterruptContexts can extract tips/cpn_id.
				currentJSON, mErr := json.Marshal(current)
				if mErr != nil {
					return nil, fmt.Errorf("graph: loop marshal state: %w", mErr)
				}
				originalVal, _ := interrupt.GetInterruptValue(invokeErr)
				var fillUpJSON json.RawMessage
				if originalVal != nil {
					if b, e := json.Marshal(originalVal); e == nil {
						fillUpJSON = b
					}
				}
				loopState := loopInterruptState{
					Iteration:       iteration,
					CurrentInput:    currentJSON,
					UserFillUpValue: fillUpJSON,
				}
				// Pass loopState directly — the checkpoint engine serializes
				// interrupt values when persisting __sub_state__. Avoid
				// double-serialization by not marshalling here.
				_, interruptErr := interrupt.Interrupt(ctx, loopState)
				common.Debug("runLoop interruptErr",
					zap.String("type", fmt.Sprintf("%T", interruptErr)),
					zap.Any("error", interruptErr))
				return nil, interruptErr
			}
			return nil, fmt.Errorf("graph: loop iteration %d: %w", iteration, invokeErr)
		}

		// Evaluate the quit predicate.
		quit, qErr := shouldQuit(ctx, iteration, current, next)
		if qErr != nil {
			return nil, fmt.Errorf("graph: loop condition iteration %d: %w", iteration, qErr)
		}
		if quit {
			return next, nil
		}

		if iteration >= options.maxIterations {
			return nil, fmt.Errorf("%w: %d", ErrLoopMaxIterationsExceeded, options.maxIterations)
		}

		iteration++
		current = next
	}
}

// loadLoopSnapshot reads the loop state from context (set by the engine
// during checkpoint restore). Falls back to interrupt resume values for
// backward compatibility.
func loadLoopSnapshot(ctx context.Context) (loopInterruptState, bool) {
	// Check context key first (set by engine's checkpoint restore).
	if ctx != nil {
		if v := ctx.Value(interrupt.SubGraphStateCtxKey); v != nil {
			switch tv := v.(type) {
			case []byte:
				var st loopInterruptState
				if err := json.Unmarshal(tv, &st); err == nil && st.Iteration > 0 {
					return st, true
				}
			}
		}
	}
	// Fallback: interrupt resume values (deprecated, remove when
	// engine.go and graph.go are migrated to loopStateCtxKey).
	values := interrupt.GetResumeValues(ctx)
	for _, v := range values {
		switch tv := v.(type) {
		case []byte:
			var st loopInterruptState
			if err := json.Unmarshal(tv, &st); err == nil && st.Iteration > 0 {
				return st, true
			}
		case string:
			var st loopInterruptState
			if err := json.Unmarshal([]byte(tv), &st); err == nil && st.Iteration > 0 {
				return st, true
			}
		}
	}
	return loopInterruptState{}, false
}
