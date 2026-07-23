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
	// CheckPointID is the stable eino checkpoint identifier. Unlike
	// eino's compose.WithCheckPointID (a run-time Option applied at
	// Workflow.Invoke), this is a compile-time descriptor: Compile cannot
	// call compose.WithCheckPointID (the option type is wrong for a
	// GraphCompileOption), so it only records the id on the returned
	// CompiledCanvas — the caller threads it to Invoke. Use a stable,
	// per-task value (e.g. taskID) so re-running the same task hits the
	// same Redis checkpoint (agent:cp:{id}). When empty,
	// CompiledCanvas.CheckPointID stays empty and the caller must supply
	// its own id (or omit it for a fresh per-run checkpoint).
	CheckPointID string
	// InterruptAfterNonTerminal, when true, makes Compile compute the
	// non-terminal node ids internally (components with out-degree > 0)
	// and register compose.WithInterruptAfterNodes on them — the caller
	// does not enumerate them. UserFillUp nodes are excluded (see §4.2.b)
	// because they already emit their own compose.Interrupt;
	// double-registering the same node for two interrupt sources would
	// break resume. Terminal nodes (no downstream) are excluded so the
	// graph does not pause on completion and force an extra, needless
	// ResumeWithData round.
	InterruptAfterNonTerminal bool
	// OverrideParams is a run-level override map keyed by cpnID. Each
	// component's `params` is merged only with its own entry
	// (an arbitrary string-keyed map); the override wins on top-level key
	// collision. Components absent from the
	// map are left untouched. Used by the ingestion pipeline so a single
	// Pipeline.Run can override the DSL-baked component params without
	// mutating the shared *Canvas (see node_body.go applyOverrideParams).
	OverrideParams map[string]any
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

// WithCheckPointID sets the stable checkpoint id recorded on the returned
// CompiledCanvas. Unlike eino's compose.WithCheckPointID (a run-time
// Option), this is a compile-time descriptor: Compile stores the id so the
// caller can pass it to Workflow.Invoke. Pass a stable, per-task value
// (e.g. taskID) so re-running the same task loads the same Redis
// checkpoint (agent:cp:{id}).
func WithCheckPointID(id string) CompileOption {
	return func(o *CompileOptions) { o.CheckPointID = id }
}

// WithInterruptAfterNonTerminalCpn registers an after-node interrupt on
// every non-terminal component (out-degree > 0) automatically. The set is
// computed inside Compile from the Canvas topology, so callers can't pass
// the wrong list (e.g. all cpnIDs, which would also interrupt terminal
// nodes and force an extra needless ResumeWithData round). UserFillUp
// nodes are excluded (§4.2.b). See computeNonTerminalCpnIDs for the exact
// selection rules.
func WithInterruptAfterNonTerminalCpn() CompileOption {
	return func(o *CompileOptions) { o.InterruptAfterNonTerminal = true }
}

// WithOverrideParams attaches a run-level override map (keyed by
// cpnID) to the compile. Each component's params are merged with
// its own entry at compile time (run-level wins on key collision, see
// node_body.go applyOverrideParams). Passing nil is a no-op.
func WithOverrideParams(m map[string]any) CompileOption {
	return func(o *CompileOptions) { o.OverrideParams = m }
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

	// S3 (plan §4.2.b 方案 A): ingestion resume mode forbids UserFillUp
	// nodes. A UserFillUp node emits its own compose.Interrupt
	// (wait-for-user); the pipeline resume loop (pipeline.go runResumable)
	// classifies every interrupt via IsInterruptError and auto-resumes with
	// nil data — so a UserFillUp pause would be silently skipped instead of
	// waiting for a human. Reject at compile time so the mis-classification
	// can never occur. The non-terminal-after filter (computeNonTerminalCpnIDs)
	// already keeps UserFillUp out of the after-node set; this is a hard
	// guard layered on top (plan §8 step 5). Checked before BuildWorkflow so
	// the guard fires on DSL content regardless of whether the graph builds.
	//
	// The same guard also forbids legacy no-op nodes (e.g. "ExitLoop", see
	// legacyNoOpNames / isLegacyNoOp). A no-op node routes to an echo body
	// that never emits TrackProgress, so it would still be counted in
	// ingestion_task.component_total yet never report progress — leaving the
	// aggregate percent permanently below 100% (plan §8 "known
	// inconsistency"). Forbidding it keeps component_total == "components
	// that report progress", so percent can reach 100% and the
	// resume/percent invariant holds. Ingestion DSLs must consist solely of
	// progress-reporting components.
	if cfg.InterruptAfterNonTerminal && c != nil {
		var bad []string
		bad = append(bad, AutoDiscoverUserFillUpIDs(c)...)
		for cpnID, comp := range c.Components {
			if isLegacyNoOp(comp.Obj.ComponentName) {
				bad = append(bad, cpnID)
			}
		}
		if len(bad) > 0 {
			return nil, fmt.Errorf("canvas: Compile: WithInterruptAfterNonTerminalCpn forbids UserFillUp/legacy-no-op nodes %v (plan §4.2.b): ingestion has no user to fill up and no-op nodes do not report progress, breaking the resume/percent invariant", bad)
		}
	}

	// Thread the run-level override (if any) into ctx so each
	// component's params is merged with its own entry inside
	// buildNodeBody. The override is keyed by cpnID; the canvas package
	// never imports ingestion.
	if cfg.OverrideParams != nil {
		ctx = withOverrideParams(ctx, cfg.OverrideParams)
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
	// Merge the caller-supplied InterruptAfter list with the
	// internally-computed non-terminal set (when requested). The
	// computed set excludes UserFillUp nodes (§4.2.b); the caller list is
	// trusted verbatim. Dedupe so a node isn't registered twice in one
	// WithInterruptAfterNodes call.
	after := append([]string{}, cfg.InterruptAfter...)
	if cfg.InterruptAfterNonTerminal {
		after = append(after, computeNonTerminalCpnIDs(c)...)
	}
	after = dedupeStrings(after)
	if len(after) > 0 {
		compileOpts = append(compileOpts, compose.WithInterruptAfterNodes(after))
	}

	runnable, err := wf.Compile(ctx, compileOpts...)
	if err != nil {
		return nil, fmt.Errorf("canvas: eino compile: %w", err)
	}
	return &CompiledCanvas{Workflow: runnable, CheckPointID: cfg.CheckPointID}, nil
}

// computeNonTerminalCpnIDs returns the cpnIDs of every component with at
// least one downstream edge (out-degree > 0). These are the nodes the
// "interrupt-after-node" resume strategy must pause on: any node that has
// work after it.
//
// Terminal nodes (no downstream) are intentionally excluded — interrupting
// them would make Invoke return an interrupt error instead of a completion,
// and force an extra needless ResumeWithData round before the graph truly
// finishes.
//
// UserFillUp nodes are excluded (§4.2.b): they already emit their own
// compose.Interrupt and must not be registered for a second, conflicting
// interrupt source. Double-registering the same node breaks resume.
func computeNonTerminalCpnIDs(c *Canvas) []string {
	if c == nil {
		return nil
	}
	exclude := make(map[string]bool, len(c.Components))
	for _, id := range AutoDiscoverUserFillUpIDs(c) {
		exclude[id] = true
	}
	var ids []string
	for cpnID, comp := range c.Components {
		if exclude[cpnID] {
			continue
		}
		if len(comp.Downstream) > 0 {
			ids = append(ids, cpnID)
		}
	}
	return ids
}

// dedupeStrings returns in with duplicate entries removed, preserving
// first-seen order. Used to merge the computed non-terminal set with the
// caller-supplied InterruptAfter list without registering a node twice in
// the same WithInterruptAfterNodes call.
func dedupeStrings(in []string) []string {
	if len(in) == 0 {
		return in
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
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
