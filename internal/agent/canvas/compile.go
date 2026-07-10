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
	"strings"

	"go.uber.org/zap"

	"ragflow/internal/common"
	graphpkg "ragflow/internal/harness/graph/graph"
	"ragflow/internal/harness/graph/types"
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
	// Deserialize raw bytes into the original map so that channel values,
	// __completed_tasks__, __last_completed_node__, etc. are directly
	// accessible by the engine's restore code (no __raw__ indirection).
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
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
	Graph        types.CompiledGraph
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
	// SetupOverrides is a run-level override map keyed by cpnID. Each
	// component's `params["setups"]` is merged only with its own entry
	// (an arbitrary string-keyed map); the override wins on top-level key
	// collision (see node_body.go mergeSetups). Components absent from the
	// map are left untouched. Used by the ingestion pipeline so a single
	// Pipeline.Run can override the DSL-baked component setups without
	// mutating the shared *Canvas (see node_body.go applySetupOverrides /
	// mergeSetups).
	SetupOverrides map[string]any
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

// WithCheckPointID sets the stable checkpoint id recorded on the returned
// CompiledCanvas. Thread it to Graph.Invoke with the same id so re-running
// the same task loads the same checkpoint (agent:cp:{id}).
func WithCheckPointID(id string) CompileOption {
	return func(o *CompileOptions) { o.CheckPointID = id }
}

// WithInterruptAfterNonTerminalCpn registers an after-node interrupt on
// every non-terminal component (out-degree > 0) automatically.
// UserFillUp nodes are excluded (§4.2.b). See computeNonTerminalCpnIDs.
func WithInterruptAfterNonTerminalCpn() CompileOption {
	return func(o *CompileOptions) { o.InterruptAfterNonTerminal = true }
}

// WithSetupOverrides attaches a run-level setups override map (keyed by
// cpnID) to the compile. Each component's `params["setups"]` is merged
// with its own entry (run-level wins on key collision).
func WithSetupOverrides(m map[string]any) CompileOption {
	return func(o *CompileOptions) { o.SetupOverrides = m }
}

// foldLegacyComponents mutates c in place, folding LoopItem/IterationItem
// nodes out of the component topology before BuildWorkflow sees them.
//
//  1. Find its parent (NodeParents first, then topology scan via downstream edges).
//  2. Append the child's Downstream to the parent's Downstream (body nodes
//     remain reachable inside the parent's sub-graph).
//  3. Rewrite every remaining component's Upstream list: replace the child ID
//     with the parent ID (or remove it if the parent ID is already present).
//  4. Remove the child from c.Components.
//
// If no parent is found, the child is deleted and a warning is logged.
// Orphan children are still removed from all other components' upstream
// lists so no dangling references survive.
func foldLegacyComponents(c *Canvas) {
	if c == nil || len(c.Components) == 0 {
		return
	}

	// Build child→parent map. Priority: NodeParents (from graph.nodes),
	// then topology scan (downstream edges).
	parentOf := make(map[string]string, len(c.NodeParents))
	for child, parent := range c.NodeParents {
		parentOf[child] = parent
	}
	for id, comp := range c.Components {
		for _, down := range comp.Downstream {
			if _, exists := parentOf[down]; !exists {
				parentOf[down] = id
			}
		}
	}

	// Phase 1: collect all legacy children with their replacement parent.
	type foldTarget struct {
		childID  string
		parentID string // "" for orphans
	}
	var targets []foldTarget

	for childID, comp := range c.Components {
		switch strings.ToLower(comp.Obj.ComponentName) {
		case "loopitem", "iterationitem":
		default:
			continue
		}
		parentID, ok := parentOf[childID]
		if !ok {
			common.Warn("canvas: dropping orphan legacy node",
				zap.String("child_id", childID),
				zap.String("component_name", comp.Obj.ComponentName))
			targets = append(targets, foldTarget{childID: childID})
			continue
		}
		if _, exists := c.Components[parentID]; !exists {
			common.Warn("canvas: dropping legacy node — parent not found in components",
				zap.String("child_id", childID),
				zap.String("parent_id", parentID))
			targets = append(targets, foldTarget{childID: childID})
			continue
		}
		targets = append(targets, foldTarget{childID: childID, parentID: parentID})
	}

	if len(targets) == 0 {
		return
	}

	// Build set of child IDs for quick lookup.
	removing := make(map[string]bool, len(targets))
	for _, t := range targets {
		removing[t.childID] = true
	}

	// Phase 2: upstream rewriting — run BEFORE deletion so Components is intact.
	for _, t := range targets {
		if t.parentID == "" {
			// Orphan: remove childID from every component's upstream.
			for id, comp := range c.Components {
				if removing[id] {
					continue
				}
				up := removeFromStrSlice(comp.Upstream, t.childID)
				if !strSliceEqual(up, comp.Upstream) {
					entry := c.Components[id]
					entry.Upstream = up
					c.Components[id] = entry
				}
			}
		} else {
			// Has parent: replace childID with parentID.
			pComp := c.Components[t.parentID]
			for id, comp := range c.Components {
				if removing[id] {
					continue
				}
				up := replaceInStrSlice(comp.Upstream, t.childID, t.parentID)
				if !strSliceEqual(up, comp.Upstream) {
					entry := c.Components[id]
					entry.Upstream = up
					c.Components[id] = entry
				}
			}

			// Filter childID out of parent's Downstream and append child's Downstream.
			childComp := c.Components[t.childID]
			filtered := make([]string, 0, len(pComp.Downstream))
			childDown := childComp.Downstream
			for _, d := range pComp.Downstream {
				if d != t.childID {
					filtered = append(filtered, d)
				}
			}
			seen := make(map[string]bool, len(filtered))
			for _, d := range filtered {
				seen[d] = true
			}
			for _, d := range childDown {
				if d != t.childID && !seen[d] {
					filtered = append(filtered, d)
					seen[d] = true
				}
			}
			pComp.Downstream = filtered
			c.Components[t.parentID] = pComp
		}
	}

	// Phase 3: delete all folded children.
	for _, t := range targets {
		delete(c.Components, t.childID)
	}
}

// removeFromStrSlice returns a copy of s without all occurrences of drop.
// Returns the original slice if drop is not found.
func removeFromStrSlice(s []string, drop string) []string {
	var out []string
	for _, x := range s {
		if x != drop {
			out = append(out, x)
		}
	}
	if len(out) == len(s) {
		return s
	}
	return out
}

// replaceInStrSlice replaces the first occurrence of oldID with newID in s.
// If newID is already present in s, oldID is simply removed instead.
// Returns the original slice if neither oldID nor newID appear.
// strSliceEqual reports whether a and b have the same elements in the same order.
func strSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func replaceInStrSlice(s []string, oldID, newID string) []string {
	hasNew := false
	for _, x := range s {
		if x == newID {
			hasNew = true
			break
		}
	}
	var out []string
	replaced := false
	for _, x := range s {
		if x == oldID && !replaced {
			if !hasNew {
				out = append(out, newID)
			}
			replaced = true
		} else {
			out = append(out, x)
		}
	}
	if !replaced {
		return s
	}
	return out
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

	// Deep-copy the Canvas before any mutation so the caller's
	// original is not modified by foldLegacyComponents or BuildWorkflow.
	work := c
	if c != nil {
		if data, err := json.Marshal(c); err == nil {
			var copy Canvas
			if json.Unmarshal(data, &copy) == nil {
				work = &copy
			}
		}
		var n int
		for _, comp := range work.Components {
			switch strings.ToLower(comp.Obj.ComponentName) {
			case "loopitem", "iterationitem":
				n++
			}
		}
		if n > 0 {
			common.Info("canvas: Compile received Canvas with legacy LoopItem/IterationItem/Iteration nodes; this path bypassed dsl.NormalizeForCanvas — the fold step is not applied", zap.Int("n", n))
		}
		foldLegacyComponents(work)
	}

	// Ingestion safety guard: when InterruptAfterNonTerminal is set,
	// forbid UserFillUp and legacy no-op nodes. See plan §8 for rationale.
	if cfg.InterruptAfterNonTerminal && work != nil {
		var bad []string
		bad = append(bad, AutoDiscoverUserFillUpIDs(work)...)
		for cpnID, comp := range work.Components {
			if isLegacyNoOp(comp.Obj.ComponentName) {
				bad = append(bad, cpnID)
			}
		}
		if len(bad) > 0 {
			return nil, fmt.Errorf("canvas: Compile: WithInterruptAfterNonTerminalCpn forbids UserFillUp/legacy-no-op nodes %v (plan §4.2.b)", bad)
		}
	}

	// Thread the run-level setups override (if any) into ctx.
	if cfg.SetupOverrides != nil {
		ctx = withSetupOverrides(ctx, cfg.SetupOverrides)
	}

	sg, err := BuildWorkflow(ctx, work)
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
	after := append([]string{}, cfg.InterruptAfter...)
	if cfg.InterruptAfterNonTerminal {
		after = append(after, computeNonTerminalCpnIDs(work)...)
	}
	after = dedupeStrings(after)
	if len(after) > 0 {
		compileOpts = append(compileOpts, graphpkg.WithInterruptsAfter(after...))
	}

	var args []interface{}
	for _, o := range compileOpts {
		args = append(args, o)
	}
	cg, err := sg.Compile(args...)
	if err != nil {
		return nil, fmt.Errorf("canvas: harness compile: %w", err)
	}
	return &CompiledCanvas{Graph: cg, CheckPointID: cfg.CheckPointID}, nil
}

// computeNonTerminalCpnIDs returns cpnIDs of components with out-degree > 0.
// UserFillUp nodes are excluded (§4.2.b); terminal nodes are excluded too.
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
