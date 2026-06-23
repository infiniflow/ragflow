// Package canvas — compile entry (Worker A, Phase 1).
//
// Compile turns a Canvas (DSL) into a CompiledCanvas: a compiled
// compose.Runnable plus the CheckPointID used at this compile. The
// compile-time wiring (state pre/post handlers, checkpoint store, serializer)
// is the Phase 1 deliverable; the actual run path (HTTP handler, SSE,
// RunTracker) lands in Phase 5.
package canvas

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/compose"
)

// CheckPointStore is the minimal interface Compile needs at compile time.
// Worker B's RedisCheckPointStore satisfies this; tests can pass any
// in-memory implementation. Matches eino's compose.CheckPointStore (an
// alias for core.CheckPointStore) and adds a Delete method.
type CheckPointStore interface {
	Get(ctx context.Context, id string) ([]byte, bool, error)
	Set(ctx context.Context, id string, payload []byte) error
	Delete(ctx context.Context, id string) error
}

// StateSerializer is the minimal interface Compile needs. Worker B's
// CanvasStateSerializer satisfies this. Mirrors eino's compose.Serializer
// (Marshal/Unmarshal, no context).
type StateSerializer interface {
	Marshal(v any) ([]byte, error)
	Unmarshal(data []byte, v any) error
}

// CompiledCanvas is the compiled runtime representation of a Canvas DSL.
// Workflow is the eino Runnable; CheckPointID is the eino checkpoint
// identifier for this compile (set by the HTTP handler before Invoke in
// Phase 5; Phase 1 leaves it empty).
type CompiledCanvas struct {
	Workflow     compose.Runnable[map[string]any, map[string]any]
	CheckPointID string
}

// CompileOptions bundles the optional collaborators the compile entry needs.
// All fields are optional; nil/zero means "skip that wire". Phase 1 defaults
// to no store, no serializer (in-memory only).
type CompileOptions struct {
	Store      CheckPointStore
	Serializer StateSerializer
	// InterruptBefore / InterruptAfter are passed straight through to
	// compose.WithInterruptBeforeNodes / WithInterruptAfterNodes.
	InterruptBefore []string
	InterruptAfter  []string
}

// CompileOption mutates a CompileOptions before the compile runs.
type CompileOption func(*CompileOptions)

// WithCheckPointStore attaches a CheckPointStore to the compile.
func WithCheckPointStore(s CheckPointStore) CompileOption {
	return func(o *CompileOptions) { o.Store = s }
}

// WithStateSerializer attaches a StateSerializer to the compile.
func WithStateSerializer(s StateSerializer) CompileOption {
	return func(o *CompileOptions) { o.Serializer = s }
}

// WithInterruptBefore configures compose.WithInterruptBeforeNodes.
func WithInterruptBefore(nodes []string) CompileOption {
	return func(o *CompileOptions) { o.InterruptBefore = nodes }
}

// WithInterruptAfter configures compose.WithInterruptAfterNodes.
func WithInterruptAfter(nodes []string) CompileOption {
	return func(o *CompileOptions) { o.InterruptAfter = nodes }
}

// Compile builds the eino Workflow from the Canvas and returns the
// compiled Runnable. State pre/post handlers are wired inside BuildWorkflow
// (see scheduler.go). Checkpoint store + serializer are wired here as
// compile-time options (compose.GraphCompileOption).
//
// IMPORTANT: eino v0.9.2 option split (plan §2.6 fix):
//
//	WithStatePreHandler / WithStatePostHandler  -> GraphAddNodeOpt (NODE option)
//	WithCheckPointStore / WithSerializer        -> GraphCompileOption
//
// Mixing them up makes the call fail to compile. We do not accept
// GraphCompileOption from the caller directly — that would let them pass
// the wrong option type. The CompileOption indirection keeps the
// GraphCompileOption surface inside this file.
func Compile(ctx context.Context, c *Canvas, opts ...CompileOption) (*CompiledCanvas, error) {
	cfg := CompileOptions{}
	for _, o := range opts {
		o(&cfg)
	}

	wf, err := BuildWorkflow(ctx, c)
	if err != nil {
		return nil, fmt.Errorf("canvas: build workflow: %w", err)
	}

	compileOpts := make([]compose.GraphCompileOption, 0, 4)
	if cfg.Store != nil {
		// eino's compose.WithCheckPointStore expects compose.CheckPointStore
		// (no Delete). Our CheckPointStore adds Delete; pass an adapter
		// that drops it. Phase 1's RunTracker doesn't call Delete on this
		// path — it deletes the agent:cp:* key via a separate Redis call.
		compileOpts = append(compileOpts, compose.WithCheckPointStore(checkPointAdapter{cfg.Store}))
	}
	if cfg.Serializer != nil {
		compileOpts = append(compileOpts, compose.WithSerializer(serializerAdapter{cfg.Serializer}))
	}
	if len(cfg.InterruptBefore) > 0 {
		compileOpts = append(compileOpts, compose.WithInterruptBeforeNodes(cfg.InterruptBefore))
	}
	if len(cfg.InterruptAfter) > 0 {
		compileOpts = append(compileOpts, compose.WithInterruptAfterNodes(cfg.InterruptAfter))
	}

	runnable, err := wf.Compile(ctx, compileOpts...)
	if err != nil {
		return nil, fmt.Errorf("canvas: eino compile: %w", err)
	}
	return &CompiledCanvas{Workflow: runnable}, nil
}

// checkPointAdapter drops the Delete method that compose.CheckPointStore
// does not declare. Worker B's RedisCheckPointStore has Delete; eino
// doesn't, so the adapter is a thin passthrough.
type checkPointAdapter struct{ inner CheckPointStore }

func (a checkPointAdapter) Get(ctx context.Context, id string) ([]byte, bool, error) {
	return a.inner.Get(ctx, id)
}
func (a checkPointAdapter) Set(ctx context.Context, id string, payload []byte) error {
	return a.inner.Set(ctx, id, payload)
}

// serializerAdapter exposes the eino-shaped Serializer (Marshal/Unmarshal,
// no context). Worker B's CanvasStateSerializer matches the same shape, so
// the adapter is a passthrough.
type serializerAdapter struct{ inner StateSerializer }

func (a serializerAdapter) Marshal(v any) ([]byte, error)   { return a.inner.Marshal(v) }
func (a serializerAdapter) Unmarshal(b []byte, v any) error { return a.inner.Unmarshal(b, v) }
