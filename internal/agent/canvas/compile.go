// Package canvas — compile entry.
//
// Compile turns a Canvas (DSL) into a CompiledCanvas: a compiled
// compose.Runnable plus the CheckPointID used at this compile. The
// compile-time wiring (state pre/post handlers, checkpoint store,
// serializer) is configured here; the actual run path lives in
// runner.go and the HTTP handler / SSE / RunTracker are wired in
// internal/service and internal/handler.
package canvas

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/compose"
	"go.uber.org/zap"

	"ragflow/internal/common"
)

// CheckPointStore is the minimal interface Compile needs at compile time.
// RedisCheckPointStore satisfies this; tests can pass any in-memory
// implementation. Matches eino's compose.CheckPointStore (an alias for
// core.CheckPointStore) and adds a Delete method.
type CheckPointStore interface {
	Get(ctx context.Context, id string) ([]byte, bool, error)
	Set(ctx context.Context, id string, payload []byte) error
	Delete(ctx context.Context, id string) error
}

// StateSerializer is the minimal interface Compile needs. The
// CanvasStateSerializer in this package satisfies this. Mirrors
// eino's compose.Serializer (Marshal/Unmarshal, no context).
type StateSerializer interface {
	Marshal(v any) ([]byte, error)
	Unmarshal(data []byte, v any) error
}

// CompiledCanvas is the compiled runtime representation of a Canvas DSL.
// Workflow is the eino Runnable; CheckPointID is the eino checkpoint
// identifier for this compile.
type CompiledCanvas struct {
	Workflow     compose.Runnable[map[string]any, map[string]any]
	CheckPointID string
}

// CompileOptions bundles the optional collaborators the compile entry needs.
// All fields are optional; nil/zero means "skip that wire".
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

	// Decoder-boundary guard: if the caller handed us a Canvas
	// whose `components` still contains LoopItem or IterationItem
	// entries, they bypassed dsl.NormalizeForCanvas (the only
	// supported decoder path). The fold step never ran, so the
	// runtime will see legacy child names and the workflow below
	// will misbehave. Surface a visible stderr warning so the
	// regression is observable — this is intentionally a log
	// rather than a panic, because internal drivers (tests,
	// fixtures) may exercise the path with raw components.
	if c != nil {
		var n int
		for _, comp := range c.Components {
			switch strings.ToLower(comp.Obj.ComponentName) {
			case "loopitem", "iterationitem", "iteration":
				n++
			}
		}
		if n > 0 {
			common.Info("canvas: Compile received Canvas with legacy LoopItem/IterationItem/Iteration nodes; this path bypassed dsl.NormalizeForCanvas — the fold step is not applied", zap.Int("n", n))
		}
	}

	wf, err := BuildWorkflow(ctx, c)
	if err != nil {
		return nil, fmt.Errorf("canvas: build workflow: %w", err)
	}

	compileOpts := make([]compose.GraphCompileOption, 0, 4)
	if cfg.Store != nil {
		// eino's compose.WithCheckPointStore expects compose.CheckPointStore
		// (no Delete). Our CheckPointStore adds Delete; pass an adapter
		// that drops it. RunTracker doesn't call Delete on this
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
// does not declare. The RedisCheckPointStore in this package has
// Delete; eino
// doesn't, so the adapter is a thin passthrough.
type checkPointAdapter struct{ inner CheckPointStore }

func (a checkPointAdapter) Get(ctx context.Context, id string) ([]byte, bool, error) {
	return a.inner.Get(ctx, id)
}
func (a checkPointAdapter) Set(ctx context.Context, id string, payload []byte) error {
	return a.inner.Set(ctx, id, payload)
}

// serializerAdapter exposes the eino-shaped Serializer (Marshal/Unmarshal,
// no context). The CanvasStateSerializer in this package matches the
// same shape, so
// the adapter is a passthrough.
type serializerAdapter struct{ inner StateSerializer }

func (a serializerAdapter) Marshal(v any) ([]byte, error)   { return a.inner.Marshal(v) }
func (a serializerAdapter) Unmarshal(b []byte, v any) error { return a.inner.Unmarshal(b, v) }
