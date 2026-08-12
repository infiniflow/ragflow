package core

import (
	"context"
	"errors"
	"fmt"
)

// ToolInterruptError is returned by tools to signal an interrupt during execution.
// When ToolsNode receives this error, it saves the interrupt state to the
// ToolExecutedCache and propagates the interrupt up to the graph engine for
// checkpointing. On resume, the cached result is used and the tool is not re-invoked.
type ToolInterruptError struct {
	// Info is user-facing information about the interrupt.
	Info any
	// State is internal state saved in the checkpoint (restored on resume).
	State any
}

func (e *ToolInterruptError) Error() string {
	return fmt.Sprintf("tool interrupt: %v", e.Info)
}

// ToolInterrupt creates an interrupt error for use in tool Invoke/EnhancedInvoke.
// The tool should return this error from Invoke to pause execution and trigger
// a checkpoint. The interrupt info is saved and can be inspected on resume.
//
// Example:
//
//	func (t *MyTool) Invoke(ctx, args string, opts ...) (string, error) {
//	    if needsApproval(args) {
//	        return "", ToolInterrupt(ctx, "needs user approval")
//	    }
//	    return doWork(args), nil
//	}
func ToolInterrupt(ctx context.Context, info any) error {
	return &ToolInterruptError{Info: info}
}

// ToolStatefulInterrupt creates a stateful interrupt error with persisted state.
// The state is restored via GetToolInterruptState on resume.
func ToolStatefulInterrupt(ctx context.Context, info, state any) error {
	return &ToolInterruptError{Info: info, State: state}
}

// IsToolInterrupt checks if an error is a tool interrupt and returns the
// parsed ToolInterruptError if so.
func IsToolInterrupt(err error) (*ToolInterruptError, bool) {
	var tie *ToolInterruptError
	if errors.As(err, &tie) {
		return tie, true
	}
	return nil, false
}

// toolInterruptContextKey stores ToolInterruptError state across resume.
type toolInterruptContextKey struct{}

// setToolInterruptState stores interrupt state in the context for resume.
func setToolInterruptState(ctx context.Context, tie *ToolInterruptError) context.Context {
	return context.WithValue(ctx, toolInterruptContextKey{}, tie.State)
}

// getToolInterruptState retrieves interrupt state from context on resume.
// Returns the saved state (nil if none) and true if this is a resume from interrupt.
func getToolInterruptState(ctx context.Context) (state any, wasInterrupted bool) {
	s := ctx.Value(toolInterruptContextKey{})
	return s, s != nil
}

// GetToolInterruptState retrieves the typed interrupt state from context.
// Useful for tools to detect if they are being resumed after an interrupt.
//
// Example:
//
//	func (t *MyTool) Invoke(ctx, args string, opts ...) (string, error) {
//	    state, wasInterrupted := GetToolInterruptState[MyState](ctx)
//	    if wasInterrupted {
//	        return continueFrom(state), nil  // resume from saved state
//	    }
//	    return "", ToolStatefulInterrupt(ctx, "paused", MyState{Step: 1})
//	}
func GetToolInterruptState[T any](ctx context.Context) (state T, wasInterrupted bool) {
	s, ok := ctx.Value(toolInterruptContextKey{}).(T)
	if ok {
		return s, true
	}
	var zero T
	return zero, false
}
