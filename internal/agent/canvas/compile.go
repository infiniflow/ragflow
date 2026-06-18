// Package canvas — compile entry.
//
// Compile turns a Canvas (DSL) into a CompiledCanvas: a compiled
// graph.StateGraph plus the CheckPointID used at this compile. The
// compile-time wiring (state pre/post handlers, checkpointer) is
// configured here; the actual run path lives in runner.go and the
// HTTP handler / SSE / RunTracker are wired in internal/service and
// internal/handler.
package canvas

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	graphpkg "ragflow/internal/harness/graph/graph"
)

// CheckPointStore ...

// CheckPointStore is the minimal interface Compile needs at compile time.
// Matches the harness checkpoint.BaseCheckpointer shape (Get/Put/Delete).
type CheckPointStore interface {
	Get(ctx context.Context, id string) ([]byte, bool, error)
	Set(ctx context.Context, id string, payload []byte) error
	Delete(ctx context.Context, id string) error
}

// checkpointerAdapter adapts canvas.CheckPointStore (with key-based Get/Put/Delete)
// to the harness checkpointer interface (config-based Get/Put/List).
type checkpointerAdapter struct{ inner CheckPointStore }

func (a checkpointerAdapter) Get(ctx context.Context, config map[string]interface{}) (map[string]interface{}, error) {
	if config == nil {
		return nil, nil
	}
	id, ok := config["thread_id"].(string)
	if !ok || id == "" {
		return nil, nil
	}
	data, found, err := a.inner.Get(ctx, id)
	if err != nil || !found {
		return nil, err
	}
	// Deserialize raw bytes into map
	result := make(map[string]interface{})
	result["__raw__"] = data
	result["thread_id"] = id
	return result, nil
}

func (a checkpointerAdapter) Put(ctx context.Context, config map[string]interface{}, checkpoint map[string]interface{}) error {
	if config == nil {
		return nil
	}
	id, ok := config["thread_id"].(string)
	if !ok || id == "" {
		return nil
	}
	// Serialize checkpoint map to bytes and persist via inner store.
	data, err := json.Marshal(checkpoint)
	if err != nil {
		return fmt.Errorf("checkpoint marshal: %w", err)
	}
	return a.inner.Set(ctx, id, data)
}

func (a checkpointerAdapter) List(ctx context.Context, config map[string]interface{}, limit int) ([]map[string]interface{}, error) {
	return nil, nil
}

// CompiledCanvas is the compiled runtime representation of a Canvas DSL.
// Graph is the compiled harness graph; CheckPointID is the checkpoint
// identifier for this compile.
type CompiledCanvas struct {
	Graph        *graphpkg.CompiledGraph
	CheckPointID string
}

// CompileOptions bundles the optional collaborators the compile entry needs.
type CompileOptions struct {
	Store      CheckPointStore
	Serializer interface{} // kept for compatibility, not used by harness
	// InterruptBefore / InterruptAfter are passed through to
	// graph.WithInterrupts / graph.WithInterruptsAfter.
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
func WithStateSerializer(s interface{}) CompileOption {
	return func(o *CompileOptions) { o.Serializer = s }
}

// WithInterruptBefore configures graph.WithInterrupts.
func WithInterruptBefore(nodes []string) CompileOption {
	return func(o *CompileOptions) { o.InterruptBefore = nodes }
}

// WithInterruptAfter configures graph.WithInterruptsAfter.
func WithInterruptAfter(nodes []string) CompileOption {
	return func(o *CompileOptions) { o.InterruptAfter = nodes }
}

// Compile builds the harness StateGraph from the Canvas and returns the
// compiled graph. State pre/post handlers are wired inside BuildWorkflow
// (see scheduler.go). Checkpointer is wired here as a compile option.
//
// IMPORTANT: harness compile options map as follows:
//
//	WithInterrupts (before) → graph.WithInterrupts(nodes...)
//	WithInterruptsAfter     → graph.WithInterruptsAfter(nodes...)
//	WithCheckpointer        → graph.WithCheckpointer(adapter)
func Compile(ctx context.Context, c *Canvas, opts ...CompileOption) (*CompiledCanvas, error) {
	cfg := CompileOptions{}
	for _, o := range opts {
		o(&cfg)
	}

	// Decoder-boundary guard
	if c != nil {
		var n int
		for _, comp := range c.Components {
			switch strings.ToLower(comp.Obj.ComponentName) {
			case "loopitem", "iterationitem", "iteration":
				n++
			}
		}
		if n > 0 {
			log.Printf("canvas: Compile received Canvas with %d legacy LoopItem/IterationItem/Iteration nodes; this path bypassed dsl.NormalizeForCanvas — the fold step is not applied", n)
		}
	}

	sg, err := BuildWorkflow(ctx, c)
	if err != nil {
		return nil, fmt.Errorf("canvas: build workflow: %w", err)
	}

	compileOpts := make([]graphpkg.CompileOption, 0, 4)
	if cfg.Store != nil {
		compileOpts = append(compileOpts, graphpkg.WithCheckpointer(checkpointerAdapter{cfg.Store}))
	}
	if len(cfg.InterruptBefore) > 0 {
		compileOpts = append(compileOpts, graphpkg.WithInterrupts(cfg.InterruptBefore...))
	}
	if len(cfg.InterruptAfter) > 0 {
		compileOpts = append(compileOpts, graphpkg.WithInterruptsAfter(cfg.InterruptAfter...))
	}

	cg, err := sg.Compile(compileOpts...)
	if err != nil {
		return nil, fmt.Errorf("canvas: harness compile: %w", err)
	}
	return &CompiledCanvas{Graph: cg}, nil
}
