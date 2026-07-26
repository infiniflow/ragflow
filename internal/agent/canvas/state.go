// Package canvas — state engine re-exports.
//
// The actual CanvasState type and its GetVar / SetVar / ReadVars
// methods live in internal/agent/runtime/state.go so the component
// package can depend on them without importing canvas. This file
// keeps the package-internal withState helper used by canvas_test.go
// and the cross-package GetStateFromContext re-export.
package canvas

import (
	"context"
	"sync"

	"ragflow/internal/agent/runtime"
)

// withState attaches *CanvasState to ctx. Production code uses this
// once per run from compile.go; cross-package tests use the exported
// WithState (state_export.go) which delegates to the same runtime
// helper.
func withState(ctx context.Context, s *CanvasState) context.Context {
	return runtime.WithState(ctx, s)
}

// GetStateFromContext re-exports runtime.GetStateFromContext so
// canvas-side callers (and tests that already import canvas) keep
// compiling without an extra import.
func GetStateFromContext[S any](ctx context.Context) (S, *sync.Mutex, error) {
	return runtime.GetStateFromContext[S](ctx)
}
