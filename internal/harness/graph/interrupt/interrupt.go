// Package interrupt provides interrupt functionality for LangGraph Go.
package interrupt

import (
	"context"
	"crypto/sha256"
	stderrors "errors"
	"fmt"
	"sync"
	"sync/atomic"

	"ragflow/internal/harness/graph/errors"
	"ragflow/internal/harness/graph/types"
)

// contextKey is the key for interrupt context in context.Context.
type contextKey struct{}

// ErrMissingInterruptContext is returned when Interrupt is called outside a graph task.
var ErrMissingInterruptContext = stderrors.New("interrupt requires a graph execution context")

// SubGraphStateCtxKey is the context key for sub-graph checkpoint state
// (e.g. Loop iteration, currentInput). Defined here so that both the
// engine (pregel) and the sub-graph node (graph/loop.go) can access it
// without import cycles.
type SubGraphStateCtxKeyType struct{}

var SubGraphStateCtxKey = SubGraphStateCtxKeyType{}

// WithInterruptContext creates a new context with interrupt support.
func WithInterruptContext(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, contextKey{}, &interruptContext{
		resumeValues: make([]interface{}, 0),
		index:        0,
	})
}

// GetInterruptContext retrieves the interrupt context from the context.
func GetInterruptContext(ctx context.Context) *interruptContext {
	if ctx == nil {
		return nil
	}
	if ic, ok := ctx.Value(contextKey{}).(*interruptContext); ok {
		return ic
	}
	return nil
}

// IsInterruptContext checks if the context has interrupt support.
func IsInterruptContext(ctx context.Context) bool {
	return GetInterruptContext(ctx) != nil
}

// Interrupt interrupts the graph with a resumable exception from within a node.
// The value is surfaced to the client and can be used to request input required to resume execution.
//
// In a given node, the first invocation of this function raises a GraphInterrupt
// exception, halting execution. The provided value is included with the exception
// and sent to the client executing the graph.
//
// A caller resumes the graph by supplying task-scoped resume values through
// the graph runtime. The graph resumes from the start of the node, re-executing
// all logic.
//
// If a node contains multiple interrupt calls, LangGraph matches resume values
// to interrupts based on their order in the node.
//
// To use an interrupt, you must enable a checkpointer, as the feature relies
// on persisting the graph state.
func Interrupt(ctx context.Context, value interface{}) (interface{}, error) {
	ic := GetInterruptContext(ctx)
	if ic == nil {
		return nil, ErrMissingInterruptContext
	}

	// Try to consume the next pending resume value under a single lock
	// (avoids TOCTOU races between separate getResumeValues/getInterruptIndex calls).
	if v, ok := ic.consumeNextResumeValue(); ok {
		return v, nil
	}

	// No resume value found, raise interrupt
	return nil, &errors.GraphInterrupt{
		Interrupts: []interface{}{
			&types.Interrupt{
				Value: value,
				ID:    generateInterruptID(value),
			},
		},
	}
}

// interruptContext holds the context for interrupts.
type interruptContext struct {
	mu           sync.Mutex
	resumeValues []interface{}
	index        int
}

// interruptIDCounter provides unique IDs across all interrupt points in the process.
var interruptIDCounter int64

// consumeNextResumeValue atomically reads the next resume value and advances
// the index under a single lock (avoids TOCTOU between separate lock acquisitions).
func (ic *interruptContext) consumeNextResumeValue() (interface{}, bool) {
	if ic == nil {
		return nil, false
	}
	ic.mu.Lock()
	defer ic.mu.Unlock()
	if ic.index < len(ic.resumeValues) {
		v := ic.resumeValues[ic.index]
		ic.index++
		return v, true
	}
	return nil, false
}

// getResumeValues returns a copy of the current resume values.
func (ic *interruptContext) getResumeValues() []interface{} {
	if ic == nil {
		return nil
	}
	ic.mu.Lock()
	defer ic.mu.Unlock()
	result := make([]interface{}, len(ic.resumeValues))
	copy(result, ic.resumeValues)
	return result
}

// getInterruptIndex returns the current interrupt index.
func (ic *interruptContext) getInterruptIndex() int {
	if ic == nil {
		return 0
	}
	ic.mu.Lock()
	defer ic.mu.Unlock()
	return ic.index
}

// appendResumeValue appends a resume value.
func (ic *interruptContext) appendResumeValue(v interface{}) {
	if ic == nil {
		return
	}
	ic.mu.Lock()
	defer ic.mu.Unlock()
	ic.resumeValues = append(ic.resumeValues, v)
}

// reset clears all interrupt context fields.
func (ic *interruptContext) reset() {
	if ic == nil {
		return
	}
	ic.mu.Lock()
	defer ic.mu.Unlock()
	ic.resumeValues = make([]interface{}, 0)
	ic.index = 0
}

// GetResumeValues returns the current resume values from context.
func GetResumeValues(ctx context.Context) []interface{} {
	ic := GetInterruptContext(ctx)
	return ic.getResumeValues()
}

// GetInterruptIndex returns the current interrupt index from context.
func GetInterruptIndex(ctx context.Context) int {
	ic := GetInterruptContext(ctx)
	return ic.getInterruptIndex()
}

// AppendResumeValue appends a resume value to the context.
func AppendResumeValue(ctx context.Context, value interface{}) {
	ic := GetInterruptContext(ctx)
	ic.appendResumeValue(value)
}

// Reset clears the interrupt context attached to ctx.
func Reset(ctx context.Context) {
	ic := GetInterruptContext(ctx)
	ic.reset()
}

// generateInterruptID generates a unique ID for an interrupt.
// The ID combines a hash of the value with a process-unique counter so that
// two interrupts with the same value (e.g. "Please provide input") are still
// distinguishable.
func generateInterruptID(value interface{}) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%v", value)))
	n := atomic.AddInt64(&interruptIDCounter, 1)
	return fmt.Sprintf("%x_%d", h[:8], n)
}

// IsInterrupt checks if an error is a GraphInterrupt.
func IsInterrupt(err error) bool {
	return errors.IsGraphInterrupt(err)
}

// GetInterruptValue extracts the user-supplied interrupt value from a GraphInterrupt error.
// Unlike returning the *types.Interrupt envelope directly, this unwraps to the .Value field
// so callers get the value they originally passed to Interrupt(ctx, value).
func GetInterruptValue(err error) (interface{}, bool) {
	if !errors.IsGraphInterrupt(err) {
		return nil, false
	}

	if gi, ok := err.(*errors.GraphInterrupt); ok && len(gi.Interrupts) > 0 {
		if intr, ok := gi.Interrupts[0].(*types.Interrupt); ok {
			return intr.Value, true
		}
		return gi.Interrupts[0], true
	}

	return nil, false
}
