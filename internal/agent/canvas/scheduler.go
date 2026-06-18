// Package canvas — eino Workflow topology builder.
//
// BuildWorkflow turns a Canvas (DSL) into a *compose.Workflow. The
// routing rules per cpn are centralised in buildNodeBody
// (node_body.go): legacy no-op names go to a dedicated echo
// lambda; UserFillUp goes to the eino interrupt-based body; every
// other name delegates to the runtime factory.
//
// State pre/post handlers are wired here as NODE options
// (GraphAddNodeOpt), NOT compile options.
//
// Cycle policy: eino's compose.Workflow is strictly a DAG and
// rejects any cycle at Compile() time. The frontend
// (`hasCanvasCycle` in web/src/pages/agent/hooks.tsx) prevents
// cycle-creating edges in user-facing canvases at the React Flow
// layer, so production graphs arriving at BuildWorkflow are
// guaranteed acyclic. No defensive cycle detection is needed
// here — let eino's Compile error surface naturally.
package canvas

import (
	"context"
	"fmt"
	"strings"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/agent/workflowx"

	"github.com/cloudwego/eino/compose"
)

// placeholderLambda is the canvas-package-only fallback for component
// bodies when no factory is registered. It copies the input map into
// the output map untouched, which lets BuildWorkflow validate the
// topology (compile + edge wiring) without depending on any real
// component implementation. Production runs always have a factory
// installed via component.init() → runtime.SetDefaultFactory(component.New);
// this fallback is exercised by canvas-only unit tests that do not
// import the component package.
func placeholderLambda(_ context.Context, in map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out, nil
}

// isLegacyNoOp reports whether name is in legacyNoOpNames (defined
// in canvas.go). The set names the DSL v1 sentinel components that
// the Go port accepts but does not implement — e.g. "ExitLoop".
// Encountering one routes the node to a no-op echo body so the
// workflow still compiles.
//
// The lookup is case-insensitive: legacyNoOpNames stores keys
// lowercase, but the DSL preserves user case (see canvas.go
// "matches agent/component/<name>.py's class name
// (case-insensitive)"). All callers go through this predicate so
// the case-normalization is in exactly one place.
//
// Note: the canvas package cannot import internal/agent/component
// (foundation layer must not depend on its callers), so the
// component-name check is intentionally NOT performed here. The
// unknown-component error path is exercised by the explicit
// TestBuildWorkflow_UnknownComponentErrors test using a name that
// is neither in the legacy set nor any of the known DSL primitives.
func isLegacyNoOp(name string) bool {
	return legacyNoOpNames[strings.ToLower(name)]
}

// isKnownPrimitive reports whether name is a real component the Go
// port can route to a body. The allowlist is a mirror of the names
// referenced in the test fixtures so that an unknown component
// name surfaces a clear error from BuildWorkflow instead of
// silently producing a no-op node. The component-name check is
// intentionally a separate path from the runtime factory
// lookup — the factory is the source of truth in production, and
// this allowlist only matters for canvas-only unit tests that
// don't import the component package.
func isKnownPrimitive(name string) bool {
	if name == "" {
		return false
	}
	// Legacy names ARE known — they route to a dedicated no-op echo
	// body installed by Pass 1 below. The "known" predicate is the
	// union of the legacy set and the real-component allowlist.
	if isLegacyNoOp(name) {
		return true
	}
	switch strings.ToLower(name) {
	case "begin", "message", "llm", "categorize", "switch",
		"agent", "invoke", "dataoperations", "listoperations",
		"stringtransform", "variableaggregator", "variableassigner",
		"loop": // Loop is a macro in BuildWorkflow; the pre-pass absorbs it.
		return true
	}
	return false
}

// statePre is the StatePreHandler wired onto every node. It injects the
// current per-cpn Outputs into the input map under the "state" key so the
// lambda body can read its inputs without re-fetching from ctx. We don't
// mutate the user's input map — we shallow-copy.
//
// The context-attached *CanvasState is the canonical store for
// components (Begin / Message / LLM all read it via
// runtime.GetStateFromContext). When the caller attached one to the
// context (orchestrator path or test setup), we sync the eino
// per-run state's outputs into it so downstream nodes see the
// upstream outputs. The eino state is still useful as a fallback
// when no context state is attached.
func statePre(ctx context.Context, in map[string]any, state *CanvasState) (map[string]any, error) {
	if in == nil {
		in = map[string]any{}
	}
	// Sync the eino state → context state when both exist so
	// downstream components reading via GetStateFromContext see
	// the upstream outputs the state post handler already wrote.
	if state != nil {
		if ctxState, _, _ := runtime.GetStateFromContext[*runtime.CanvasState](ctx); ctxState != nil && ctxState != state {
			for cpnID, bucket := range state.Outputs {
				for k, v := range bucket {
					ctxState.SetVar(cpnID, k, v)
				}
			}
		}
	}
	snapshot := state.Snapshot()
	out := make(map[string]any, len(in)+1)
	for k, v := range in {
		out[k] = v
	}
	out["state"] = snapshot
	return out, nil
}

// statePost is the StatePostHandler — it flattens the lambda's output
// keys into the per-cpn Outputs bucket keyed by the cpn_id passed
// through the input map ("cpn_id" key, injected by BuildWorkflow's
// per-node wrapper).
//
// Storage convention: each top-level key in the component's output
// map lands as Outputs[cpnID][key]. v1 templates reference these as
// {{cpnID@key}} (e.g. {{generate:0@content}}). Nesting the entire
// payload under Outputs[cpnID]["result"] would force every template
// to use {{cpnID@result.content}} which the v1 DSL never writes.
//
// The write is mirrored into the context-attached *CanvasState when
// one is present, so downstream components that read state via
// runtime.GetStateFromContext (Begin / Message / LLM) see the
// upstream output. The eino per-run state stays the source of truth
// for the snapshot exposed via statePre.
func statePost(ctx context.Context, out map[string]any, state *CanvasState) (map[string]any, error) {
	cpnID, _ := out["__cpn_id__"].(string)
	if cpnID == "" {
		return out, nil
	}
	ctxState, _, _ := runtime.GetStateFromContext[*runtime.CanvasState](ctx)
	for k, v := range out {
		if k == "__cpn_id__" || k == "state" || k == "__legacy_noop__" {
			continue
		}
		if state != nil {
			state.SetVar(cpnID, k, v)
		}
		if ctxState != nil {
			ctxState.SetVar(cpnID, k, v)
		}
	}
	return out, nil
}

// BuildWorkflow assembles a *compose.Workflow from a Canvas DSL.
//
// Topology rules (per plan §1.1, §2.4):
//
//   - For every cpn_id in c.Components: add a Lambda node.
//   - For every (cpn_id, upstream) edge: cpn.AddInput(upstream).
//   - For components with no upstream (Begin nodes): wire an empty input
//     from compose.START so eino knows they are start candidates.
//   - For components with no downstream (terminals): wire them to the
//     implicit END via wf.End().AddInput(cpnID, ...).
//
// State pre/post handlers are added to every node as NODE options
// (GraphAddNodeOpt). The handlers carry the per-run *CanvasState which eino
// extracts from context for us (via WithGenLocalState — wired in compile.go).
func BuildWorkflow(ctx context.Context, c *Canvas) (*compose.Workflow[map[string]any, map[string]any], error) {
	if c == nil {
		return nil, fmt.Errorf("canvas: nil canvas")
	}
	if len(c.Components) == 0 {
		return nil, fmt.Errorf("canvas: no components")
	}

	// GenLocalState seeds each run with a fresh *CanvasState. eino calls
	// this once per run and threads the result through StatePre/Post
	// handlers via context.
	genState := func(_ context.Context) *CanvasState {
		return NewCanvasState("", "")
	}

	wf := compose.NewWorkflow[map[string]any, map[string]any](
		compose.WithGenLocalState(genState),
	)

	// Pre-pass: Loop macro expansion. For each Loop cpn, build a
	// sub-workflow from its downstream descendants and install a
	// workflowx.AddLoopNode in the outer graph in place of the Loop
	// subtree. The sub-graph members are tracked in `loopMembers` so
	// the main pass skips them.
	loopMembers := make(map[string]bool)
	loopNodes := make(map[string]*compose.WorkflowNode)
	for cpnID, comp := range c.Components {
		if !strings.EqualFold(comp.Obj.ComponentName, "Loop") {
			continue
		}
		exp, err := buildLoopExpansion(ctx, c, cpnID)
		if err != nil {
			return nil, err
		}
		var opts []workflowx.LoopOption
		if exp.MaxIters > 0 {
			opts = append(opts, workflowx.WithLoopMaxIterations(exp.MaxIters))
		}
		node, err := workflowx.AddLoopNode[map[string]any](
			ctx, wf, cpnID, exp.Sub, exp.ShouldQuit, opts...,
		)
		if err != nil {
			return nil, fmt.Errorf("canvas: install loop %q: %w", cpnID, err)
		}
		loopNodes[cpnID] = node
		for m := range exp.Members {
			loopMembers[m] = true
		}
	}

	// Pass 1: register every node and remember its upstream list so we can
	// wire edges in a second pass (Compose disallows AddInput before the
	// upstream exists). Skip Loop cpns and their sub-graph members —
	// they live in `loopNodes` and inside the sub-workflow respectively.
	//
	// Component-routing rules per cpn (centralised in buildNodeBody):
	//
	//   1. component_name is in legacyNoOpNames (e.g. "ExitLoop") →
	//      dedicated no-op echo lambda with __legacy_noop__ tag.
	//   2. runtime.DefaultFactory() registered → factory-built real
	//      component invoked per iteration.
	//   3. no factory registered → placeholder body (canvas-only test
	//      fallback; production wiring always registers a factory via
	//      component.init()).
	type pendingEdge struct {
		cpn string
		up  string
	}
	pending := make([]pendingEdge, 0, 4*len(c.Components))
	nodes := make(map[string]*compose.WorkflowNode, len(c.Components))
	for cpnID := range c.Components {
		// Loop cpns are already registered as workflowx nodes in
		// loopNodes (pre-pass). We still need to record their
		// upstream edges so Pass 2 can wire `upstream → loop`.
		if _, isLoop := loopNodes[cpnID]; isLoop {
			for _, up := range c.Components[cpnID].Upstream {
				pending = append(pending, pendingEdge{cpn: cpnID, up: up})
			}
			continue
		}
		if loopMembers[cpnID] {
			continue
		}
		name := c.Components[cpnID].Obj.ComponentName
		if name == "" {
			return nil, fmt.Errorf("canvas: component %q has empty component_name", cpnID)
		}
		body, err := buildNodeBody(cpnID, name, c.Components[cpnID].Obj.Params)
		if err != nil {
			return nil, err
		}
		lambda := compose.InvokableLambda[map[string]any, map[string]any](body)
		node := wf.AddLambdaNode(cpnID, lambda,
			compose.WithStatePreHandler[map[string]any, *CanvasState](statePre),
			compose.WithStatePostHandler[map[string]any, *CanvasState](statePost),
			compose.WithNodeName(cpnID),
		)
		nodes[cpnID] = node
		for _, up := range c.Components[cpnID].Upstream {
			pending = append(pending, pendingEdge{cpn: cpnID, up: up})
		}
	}

	// Pass 2: wire edges. Skip self-edges and edges to unknown upstreams —
	// those would be a DSL bug; BuildWorkflow returns an error so the
	// orchestrator can surface a clear failure (better than a silent
	// non-trigger).
	//
	// Multi-upstream handling: eino's Workflow only allows ONE actual data
	// input per node (subsequent AddInput without FieldMapping triggers
	// "entire output has already been mapped"). For diamond / merge
	// topologies, the first upstream carries data; the rest register as
	// exec-only dependencies via AddDependency so the node waits for
	// them but doesn't try to consume a second data source. Component
	// bodies that need to merge multi-source inputs switch to explicit
	// FieldMapping via the StatePreHandler (see scheduler.go's
	// statePre implementation).
	//
	// An upstream may be a regular node OR a Loop node (registered in
	// the pre-pass). Both are valid edge sources. Symmetrically, the
	// downstream may itself be a Loop node — in that case we resolve
	// the *compose.WorkflowNode via loopNodes rather than nodes.
	resolveNode := func(id string) *compose.WorkflowNode {
		if n, ok := nodes[id]; ok {
			return n
		}
		if n, ok := loopNodes[id]; ok {
			return n
		}
		return nil
	}
	first := make(map[string]bool, len(c.Components))
	for _, e := range pending {
		if e.cpn == e.up {
			return nil, fmt.Errorf("canvas: self-edge on %q", e.cpn)
		}
		if resolveNode(e.up) == nil {
			return nil, fmt.Errorf("canvas: component %q has unknown upstream %q", e.cpn, e.up)
		}
		cpnNode := resolveNode(e.cpn)
		if cpnNode == nil {
			return nil, fmt.Errorf("canvas: pending edge references unknown cpn %q", e.cpn)
		}
		if !first[e.cpn] {
			cpnNode.AddInput(e.up)
			first[e.cpn] = true
		} else {
			cpnNode.AddDependency(e.up)
		}
	}

	// Pass 2.5: install MultiBranch edges for runtime-control parents.
	// Switch / Categorize produce a `_next` output identifying which
	// downstream child should run at runtime. Without this pass every
	// declared child fires unconditionally (Pass 2 wired AddInput from
	// parent to every child); the branch adds a control-only gate so
	// only the chosen child is executed. The AddInput edges stay in
	// place — they carry the data path; the branch carries the control
	// path. See multibranch.go for the full rationale.
	wireMultiBranches(wf, c, loopMembers)

	// Pass 3: wire start nodes (no upstream) from compose.START, and wire
	// terminal nodes (no downstream) to compose.END via wf.End(). eino
	// tracks start/end membership by these explicit wirings — without
	// them, Compile() returns "start node not set" / "end node not set".
	//
	// Multi-terminal case: when two or more components have empty
	// Downstream, eino's END node complains "entire output has already
	// been mapped for node: end" unless each terminal is wired with a
	// distinct compose.ToField(cpnID) mapping. We always include the
	// FieldMapping argument (per terminal) so the count of inputs
	// matters only to eino's bookkeeping, not to our wire code.
	//
	// A "start" node with no upstream gets an empty input from START so
	// eino registers it as a workflow entry point. FieldMapping is nil
	// because the placeholder lambdas just echo whatever they receive.
	//
	// Loop nodes are wired here too: a Loop is START if it has no
	// upstream; it is END if it has no downstream in the outer graph
	// (a downstream that's also a sub-graph member doesn't count — that
	// node is part of the loop's body, not the outer graph's edge).
	for cpnID, comp := range c.Components {
		if node, isLoop := loopNodes[cpnID]; isLoop {
			// Loops with no upstream are START nodes. Loops WITH
			// upstream had their AddInput wired in Pass 2 already.
			if len(comp.Upstream) == 0 && !first[cpnID] {
				node.AddInput(compose.START)
			}
			hasOuterDownstream := false
			for _, down := range comp.Downstream {
				if loopMembers[down] {
					continue
				}
				hasOuterDownstream = true
				break
			}
			if !hasOuterDownstream {
				wf.End().AddInput(cpnID, compose.ToField(cpnID))
			}
			continue
		}
		if loopMembers[cpnID] {
			continue
		}
		if len(comp.Upstream) == 0 {
			nodes[cpnID].AddInput(compose.START)
		}
		if len(comp.Downstream) == 0 {
			wf.End().AddInput(cpnID, compose.ToField(cpnID))
		}
	}

	return wf, nil
}

// snapshotOutputs is retained as a thin wrapper around state.Snapshot()
// for any leftover callers in test/bench files. New code should call
// state.Snapshot() directly.
func snapshotOutputs(src map[string]map[string]any) map[string]map[string]any {
	out := make(map[string]map[string]any, len(src))
	for k, v := range src {
		cp := make(map[string]any, len(v))
		for kk, vv := range v {
			cp[kk] = vv
		}
		out[k] = cp
	}
	return out
}
