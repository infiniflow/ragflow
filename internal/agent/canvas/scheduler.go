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
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/agent/workflowx"
	"ragflow/internal/common"

	"github.com/cloudwego/eino/compose"
	"go.uber.org/zap"
)

// ctxKey is the unexported context-key type for per-run metadata
// (events channel, message/task/session ids) so the statePre/statePost
// wrappers can emit node_started/node_finished without depending on
// the service package.
type ctxKey string

const ctxKeyRunMeta ctxKey = "canvas_run_meta"
const terminalMergeNodeID = "__canvas_terminal_merge__"

// RunMeta carries the per-run metadata that node lifecycle hooks need.
type RunMeta struct {
	Events    chan RunEvent
	MessageID string
	TaskID    string
	SessionID string
}

// WithRunMeta attaches run metadata to the context for consumption by
// the per-node statePre/statePost wrappers in BuildWorkflow.
func WithRunMeta(ctx context.Context, m *RunMeta) context.Context {
	return context.WithValue(ctx, ctxKeyRunMeta, m)
}

// GetRunMeta extracts run metadata previously attached with WithRunMeta.
// Returns nil when absent (test paths without a full service harness).
func GetRunMeta(ctx context.Context) *RunMeta {
	m, _ := ctx.Value(ctxKeyRunMeta).(*RunMeta)
	return m
}

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
		"loop", "parallel": // macros in BuildWorkflow; the pre-pass absorbs them.
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
			sysNS, envNS, globalsNS := state.SnapshotNamespaces()
			for k, v := range sysNS {
				ctxState.Sys[k] = v
			}
			for k, v := range envNS {
				ctxState.Env[k] = v
			}
			for k, v := range globalsNS {
				ctxState.Globals[k] = v
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
	if ctxState != nil && state != nil && ctxState != state {
		sysNS, envNS, globalsNS := ctxState.SnapshotNamespaces()
		state.Sys = sysNS
		state.Env = envNS
		state.Globals = globalsNS
	}
	return out, nil
}

// emitEventFromCtx reads the events channel from the RunMeta attached to
// ctx (via WithRunMeta) and pushes the event. No-op when no metadata is
// present (test paths without a full service harness).
func emitEventFromCtx(ctx context.Context, ev RunEvent) {
	meta := GetRunMeta(ctx)
	if meta == nil || meta.Events == nil {
		return
	}
	PushEvent(meta.Events, ev)
}

func sanitizeNodeInputs(inputs map[string]any) map[string]any {
	if len(inputs) == 0 {
		return map[string]any{}
	}

	out := make(map[string]any, len(inputs))
	for k, v := range inputs {
		switch k {
		case "state", "__cpn_id__", "__legacy_noop__":
			continue
		default:
			out[k] = v
		}
	}
	return out
}

// nodeStartedAt records the per-node start time in state.Sys and emits a
// node_started RunEvent. Called from the per-node statePre wrapper.
// Metadata (message/task/session ids) is read from ctx via RunMeta.
func nodeStartedAt(ctx context.Context, state *CanvasState, cpnID, componentName, componentType string, inputs map[string]any) {
	common.Debug("node_started", zap.String("cpnID", cpnID), zap.String("componentName", componentName))
	if state == nil {
		return
	}
	now := float64(time.Now().UnixNano()) / 1e9

	if state.Sys != nil {
		state.Sys["_node_start_"+cpnID] = now
		state.Sys["_node_inputs_"+cpnID] = sanitizeNodeInputs(inputs)
	}
	nsData, _ := json.Marshal(NodeStartedData{
		Inputs:        sanitizeNodeInputs(inputs),
		CreatedAt:     now,
		ComponentID:   cpnID,
		ComponentName: componentName,
		ComponentType: componentType,
		Thoughts:      "",
	})
	meta := GetRunMeta(ctx)
	msgID, taskID, sessionID := "", "", ""
	if meta != nil {
		msgID, taskID, sessionID = meta.MessageID, meta.TaskID, meta.SessionID
	}
	emitEventFromCtx(ctx, RunEvent{
		Type: "node_started", Data: string(nsData),
		MessageID: msgID, CreatedAt: time.Now().Unix(),
		TaskID: taskID, SessionID: sessionID,
	})
}

// nodeFinishedNow emits a node_finished RunEvent. Called from the per-node
// statePost wrapper. The elapsed time is computed from the time recorded
// by nodeStartedAt. Metadata is read from ctx via RunMeta.
func nodeFinishedNow(ctx context.Context, state *CanvasState, cpnID, componentName, componentType string, nodeErr error) {
	if state == nil {
		return
	}
	now := float64(time.Now().UnixNano()) / 1e9
	var elapsed float64
	if state.Sys != nil {
		if start, ok := state.Sys["_node_start_"+cpnID].(float64); ok {
			elapsed = now - start
		}
	}
	if elapsed < 0 {
		elapsed = 0
	}

	// Collect outputs from the state's Outputs bucket for this cpn.
	var outputs map[string]any
	if state.Outputs != nil {
		if bucket, ok := state.Outputs[cpnID]; ok && len(bucket) > 0 {
			outputs = make(map[string]any, len(bucket))
			for k, v := range bucket {
				outputs[k] = v
			}
		}
	}

	inputs := map[string]any{}
	if state.Sys != nil {
		if v, ok := state.Sys["_node_inputs_"+cpnID].(map[string]any); ok {
			inputs = v
		}
	}

	var nfErr interface{}
	if nodeErr != nil {
		nfErr = nodeErr.Error()
	}

	nfData, _ := json.Marshal(NodeFinishedData{
		Inputs:        inputs,
		Outputs:       outputs,
		ComponentID:   cpnID,
		ComponentName: componentName,
		ComponentType: componentType,
		Error:         nfErr,
		ElapsedTime:   elapsed,
		CreatedAt:     now,
	})
	meta := GetRunMeta(ctx)
	msgID, taskID, sessionID := "", "", ""
	if meta != nil {
		msgID, taskID, sessionID = meta.MessageID, meta.TaskID, meta.SessionID
	}
	emitEventFromCtx(ctx, RunEvent{
		Type: "node_finished", Data: string(nfData),
		MessageID: msgID, CreatedAt: time.Now().Unix(),
		TaskID: taskID, SessionID: sessionID,
	})
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
	//
	// The initial env/sys values come from c.Globals (the DSL-level
	// "globals" map) so that env.* references like "env.counter" resolve
	// to their declared defaults rather than nil. The Go port splits
	// "sys.*" and "env.*" dotted keys into separate Sys/Env maps so
	// GetVar("env.counter") can look up Env["counter"] directly;
	// seeding here mirrors the Python canvas.__init__ →
	// self.globals["env.counter"] = 0 path.
	globals := c.Globals
	genState := func(_ context.Context) *CanvasState {
		st := NewCanvasState("", "")
		if globals != nil {
			for k, v := range globals {
				if strings.HasPrefix(k, "sys.") {
					st.Sys[strings.TrimPrefix(k, "sys.")] = v
				} else if strings.HasPrefix(k, "env.") {
					st.Env[strings.TrimPrefix(k, "env.")] = v
				} else {
					st.Globals[k] = v
				}
			}
		}
		return st
	}

	wf := compose.NewWorkflow[map[string]any, map[string]any](
		compose.WithGenLocalState(genState),
	)

	// Pre-pass: runtime-control macro expansion. Loop and Parallel are
	// both compiled as single outer nodes backed by a sub-workflow.
	// Their body members are tracked in `macroMembers` so the main pass
	// skips those nodes in the outer graph.
	macroMembers := make(map[string]bool)
	macroNodes := make(map[string]*compose.WorkflowNode)
	for cpnID, comp := range c.Components {
		switch {
		case strings.EqualFold(comp.Obj.ComponentName, "Loop"):
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
			macroNodes[cpnID] = node
			for m := range exp.Members {
				macroMembers[m] = true
			}
		case strings.EqualFold(comp.Obj.ComponentName, "Parallel"):
			exp, err := buildParallelExpansion(ctx, c, cpnID)
			if err != nil {
				return nil, err
			}
			node := wf.AddGraphNode(cpnID, exp.Graph,
				compose.WithNodeName(cpnID),
				compose.WithStatePreHandler[map[string]any, *CanvasState](func(ctx context.Context, in map[string]any, state *CanvasState) (map[string]any, error) {
					nodeStartedAt(ctx, state, cpnID, comp.Obj.ComponentName, comp.Obj.ComponentName, in)
					return statePre(ctx, in, state)
				}),
				compose.WithStatePostHandler[map[string]any, *CanvasState](func(ctx context.Context, out map[string]any, state *CanvasState) (map[string]any, error) {
					result, postErr := statePost(ctx, out, state)
					nodeFinishedNow(ctx, state, cpnID, comp.Obj.ComponentName, comp.Obj.ComponentName, postErr)
					return result, postErr
				}),
			)
			macroNodes[cpnID] = node
			for m := range exp.Members {
				macroMembers[m] = true
			}
		}
	}

	// Pass 1: register every node and remember its upstream list so we can
	// wire edges in a second pass (Compose disallows AddInput before the
	// upstream exists). Skip macro cpns and their sub-graph members —
	// they live in `macroNodes` and inside the sub-workflow respectively.
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
		// Macro cpns are already registered in the pre-pass. We
		// still need to record their upstream edges so Pass 2 can wire
		// `upstream -> macro`.
		if _, isMacro := macroNodes[cpnID]; isMacro {
			for _, up := range c.Components[cpnID].Upstream {
				pending = append(pending, pendingEdge{cpn: cpnID, up: up})
			}
			continue
		}
		if macroMembers[cpnID] {
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
		// Per-node statePre/statePost wrappers close over cpnID and
		// component metadata so they can emit node_started /
		// node_finished events at the correct per-node lifecycle
		// points. The events channel and run metadata are read from
		// the context via WithRunMeta / GetRunMeta (populated by the
		// service layer before invoke).
		componentName := c.Components[cpnID].Obj.ComponentName
		nodePre := func(ctx context.Context, in map[string]any, state *CanvasState) (map[string]any, error) {
			nodeStartedAt(ctx, state, cpnID, componentName, componentName, in)
			return statePre(ctx, in, state)
		}
		nodePost := func(ctx context.Context, out map[string]any, state *CanvasState) (map[string]any, error) {
			result, postErr := statePost(ctx, out, state)
			nodeFinishedNow(ctx, state, cpnID, componentName, componentName, postErr)
			return result, postErr
		}
		lambda := compose.InvokableLambda[map[string]any, map[string]any](body)
		node := wf.AddLambdaNode(cpnID, lambda,
			compose.WithStatePreHandler[map[string]any, *CanvasState](nodePre),
			compose.WithStatePostHandler[map[string]any, *CanvasState](nodePost),
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
		if n, ok := macroNodes[id]; ok {
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
	wireMultiBranches(wf, c, macroMembers)

	// Pass 3: wire start nodes (no upstream) from compose.START, and wire
	// terminal nodes (no downstream) to compose.END. eino
	// tracks start/end membership by these explicit wirings — without
	// them, Compile() returns "start node not set" / "end node not set".
	//
	// Multi-terminal case: eino's END node is stricter than regular
	// workflow nodes about repeated output mappings. Instead of wiring
	// multiple terminals directly into END, route them through one
	// synthetic merge node. The merge node consumes one terminal as its
	// data input and treats the rest as exec-only dependencies, mirroring
	// the same "first input carries data; the rest are dependencies"
	// policy used in Pass 2.
	//
	// A "start" node with no upstream gets an empty input from START so
	// eino registers it as a workflow entry point. FieldMapping is nil
	// because the placeholder lambdas just echo whatever they receive.
	//
	// Loop nodes are wired here too: a Loop is START if it has no
	// upstream; it is END if it has no downstream in the outer graph
	// (a downstream that's also a sub-graph member doesn't count — that
	// node is part of the loop's body, not the outer graph's edge).
	terminals := make([]string, 0, len(c.Components))
	for cpnID, comp := range c.Components {
		if node, isMacro := macroNodes[cpnID]; isMacro {
			// Macro parents with no upstream are START nodes. Parents
			// with upstream had their AddInput wired in Pass 2 already.
			if len(comp.Upstream) == 0 && !first[cpnID] {
				node.AddInput(compose.START)
			}
			hasOuterDownstream := false
			for _, down := range comp.Downstream {
				if macroMembers[down] {
					continue
				}
				hasOuterDownstream = true
				break
			}
			if !hasOuterDownstream {
				terminals = append(terminals, cpnID)
			}
			continue
		}
		if macroMembers[cpnID] {
			continue
		}
		if len(comp.Upstream) == 0 {
			nodes[cpnID].AddInput(compose.START)
		}
		if len(comp.Downstream) == 0 {
			terminals = append(terminals, cpnID)
		}
	}

	if err := wireWorkflowTerminals(wf, terminals, "", true); err != nil {
		return nil, err
	}

	return wf, nil
}

func wireWorkflowTerminals(
	wf *compose.Workflow[map[string]any, map[string]any],
	terminals []string,
	fallback string,
	useFieldMapping bool,
) error {
	if len(terminals) == 0 {
		if fallback == "" {
			return fmt.Errorf("canvas: end node not set")
		}
		terminals = []string{fallback}
	}

	addEndInput := func(nodeID string) {
		if useFieldMapping {
			wf.End().AddInput(nodeID, compose.ToField(nodeID))
			return
		}
		wf.End().AddInput(nodeID)
	}

	if len(terminals) == 1 {
		addEndInput(terminals[0])
		return nil
	}

	// Sub-workflows wire END without field mappings. These multi-terminal
	// shapes commonly come from mutually exclusive branches (for example a
	// loop body Switch choosing either continue or exit). We therefore
	// create a small field-mapped gather node that forwards whichever
	// branch actually produced output, instead of the outer workflow's
	// dependency-based merge node that would incorrectly wait for every
	// terminal to execute in the same run.
	if !useFieldMapping {
		gatherNode := wf.AddLambdaNode(
			terminalMergeNodeID,
			compose.InvokableLambda[map[string]any, map[string]any](
				func(_ context.Context, in map[string]any) (map[string]any, error) {
					for _, terminalID := range terminals {
						if v, ok := in[terminalID].(map[string]any); ok && v != nil {
							return v, nil
						}
					}
					return in, nil
				},
			),
			compose.WithNodeName(terminalMergeNodeID),
		)
		for _, terminalID := range terminals {
			gatherNode.AddInput(terminalID, compose.ToField(terminalID))
		}
		addEndInput(terminalMergeNodeID)
		return nil
	}

	mergeNode := wf.AddLambdaNode(
		terminalMergeNodeID,
		compose.InvokableLambda[map[string]any, map[string]any](
			func(_ context.Context, in map[string]any) (map[string]any, error) {
				return in, nil
			},
		),
		compose.WithNodeName(terminalMergeNodeID),
	)
	mergeNode.AddInput(terminals[0])
	for _, terminalID := range terminals[1:] {
		mergeNode.AddDependency(terminalID)
	}
	addEndInput(terminalMergeNodeID)
	return nil
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
