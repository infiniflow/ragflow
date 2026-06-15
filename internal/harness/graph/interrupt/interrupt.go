// Package interrupt provides interrupt functionality for LangGraph Go.
package interrupt

import (
	"context"
	"fmt"
	"sync"

	"ragflow/internal/harness/graph/errors"
	"ragflow/internal/harness/graph/types"
)

// contextKey is the key for interrupt context in context.Context.
type contextKey struct{}

// WithInterruptContext creates a new context with interrupt support.
func WithInterruptContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, contextKey{}, &interruptContext{
		resumeValues: make([]interface{}, 0),
		index:        0,
	})
}

// GetInterruptContext retrieves the interrupt context from the context.
func GetInterruptContext(ctx context.Context) *interruptContext {
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
// A client resuming the graph must use the Command primitive to specify a value
// for the interrupt and continue execution.
// The graph resumes from the start of the node, re-executing all logic.
//
// If a node contains multiple interrupt calls, LangGraph matches resume values
// to interrupts based on their order in the node.
//
// To use an interrupt, you must enable a checkpointer, as the feature relies
// on persisting the graph state.
func Interrupt(ctx context.Context, value interface{}) (interface{}, error) {
	ic := GetInterruptContext(ctx)
	if ic == nil {
		// Fall back to global context for backward compatibility
		ic = globalContext
	}

	// Check for resume values
	resumeValues := ic.getResumeValues()
	idx := ic.getInterruptIndex()

	if idx < len(resumeValues) {
		// Return the resume value
		ic.incrementIndex()
		return resumeValues[idx], nil
	}

	// Check for current resume value
	v := ic.getNullResume()
	if v != nil {
		ic.appendResumeValue(v)
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
	nullResume   interface{}
}

// Global context for backward compatibility
var globalContext = &interruptContext{
	resumeValues: make([]interface{}, 0),
	index:        0,
}

// getResumeValues returns the current resume values.
func (ic *interruptContext) getResumeValues() []interface{} {
	if ic == nil {
		return nil
	}
	ic.mu.Lock()
	defer ic.mu.Unlock()
	return ic.resumeValues
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

// getNullResume checks for a null resume value.
func (ic *interruptContext) getNullResume() interface{} {
	if ic == nil {
		return nil
	}
	ic.mu.Lock()
	defer ic.mu.Unlock()
	return ic.nullResume
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

// setNullResume sets the null resume value.
func (ic *interruptContext) setNullResume(v interface{}) {
	if ic == nil {
		return
	}
	ic.mu.Lock()
	defer ic.mu.Unlock()
	ic.nullResume = v
}

// incrementIndex increments the interrupt index by 1 and returns the new value.
func (ic *interruptContext) incrementIndex() int {
	if ic == nil {
		return 0
	}
	ic.mu.Lock()
	defer ic.mu.Unlock()
	ic.index++
	return ic.index
}

// setResumeValues replaces the resume values.
func (ic *interruptContext) setResumeValues(values []interface{}) {
	if ic == nil {
		return
	}
	ic.mu.Lock()
	defer ic.mu.Unlock()
	ic.resumeValues = values
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
	ic.nullResume = nil
}

// GetResumeValues returns the current resume values from context.
func GetResumeValues(ctx context.Context) []interface{} {
	var ic *interruptContext
	if ctx != nil {
		ic = GetInterruptContext(ctx)
	}
	if ic == nil {
		ic = globalContext
	}
	return ic.getResumeValues()
}

// GetInterruptIndex returns the current interrupt index from context.
func GetInterruptIndex(ctx context.Context) int {
	ic := GetInterruptContext(ctx)
	if ic == nil {
		ic = globalContext
	}
	return ic.getInterruptIndex()
}

// AppendResumeValue appends a resume value to the context.
func AppendResumeValue(ctx context.Context, v interface{}) {
	var ic *interruptContext
	if ctx != nil {
		ic = GetInterruptContext(ctx)
	}
	if ic == nil {
		ic = globalContext
	}
	ic.appendResumeValue(v)
}

// GetNullResume gets the null resume value from context.
// If consume is true, the value is cleared after retrieval.
func GetNullResume(ctx context.Context, consume bool) interface{} {
	ic := GetInterruptContext(ctx)
	if ic == nil {
		ic = globalContext
	}
	v := ic.getNullResume()
	if consume {
		ic.setNullResume(nil)
	}
	return v
}

// Reset clears the interrupt context.
func Reset(ctx context.Context) {
	ic := GetInterruptContext(ctx)
	if ic != nil {
		ic.reset()
	}
	// Also reset global context
	globalContext.reset()
}

// generateInterruptID generates a unique ID for an interrupt.
func generateInterruptID(value interface{}) string {
	// In actual implementation, this would use a hash of the namespace
	return fmt.Sprintf("%v", value)
}

// IsInterrupt checks if an error is a GraphInterrupt.
func IsInterrupt(err error) bool {
	return errors.IsGraphInterrupt(err)
}

// GetInterruptValue extracts the interrupt value from a GraphInterrupt error.
func GetInterruptValue(err error) (interface{}, bool) {
	if !errors.IsGraphInterrupt(err) {
		return nil, false
	}

	if gi, ok := err.(*errors.GraphInterrupt); ok && len(gi.Interrupts) > 0 {
		return gi.Interrupts[0], true
	}

	return nil, false
}

// SetResumeValues sets the resume values for testing.
func SetResumeValues(ctx context.Context, values []interface{}) {
	ic := GetInterruptContext(ctx)
	if ic == nil {
		ic = globalContext
	}
	ic.setResumeValues(values)
}
