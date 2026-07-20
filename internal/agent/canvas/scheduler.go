// Package canvas — StateGraph topology builder.
//
// BuildWorkflow turns a Canvas (DSL) into a *graph.StateGraph. The
// routing rules per cpn are centralised in buildNodeBody
// (node_body.go): legacy no-op names go to a dedicated echo node;
// UserFillUp goes to the harness interrupt-based body; every
// other name delegates to the runtime factory.
//
// State pre/post handlers are wired here as NodeOptions, NOT compile
// options. They read/write CanvasState from/to the request context
// (runtime.WithState) so the shared data bag works independently of
// the graph engine's channel-based state.
//
// Cycle policy: the frontend (`hasCanvasCycle`) prevents cycle-creating
// edges in user-facing canvases at the React Flow layer, so production
// graphs arriving at BuildWorkflow are guaranteed acyclic. No defensive
// cycle detection is needed here — let harness's Compile error surface
// naturally.
package canvas

import (
	"context"
	"fmt"
	"strings"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/harness/graph/channels"
	"ragflow/internal/harness/graph/constants"
	graphpkg "ragflow/internal/harness/graph/graph"
	"ragflow/internal/harness/graph/types"
)

// placeholderLambda is the canvas-package-only fallback for component
// bodies when no factory is registered. It copies the input map into
// the output map untouched.
func placeholderLambda(_ context.Context, in map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out, nil
}

// isLegacyNoOp reports whether name is in legacyNoOpNames (defined
// in canvas.go). The set names the DSL v1 sentinel components that
// the Go port accepts but does not implement.
func isLegacyNoOp(name string) bool {
	return legacyNoOpNames[strings.ToLower(name)]
}

// isKnownPrimitive reports whether name is a real component the Go
// port can route to a body.
func isKnownPrimitive(name string) bool {
	if name == "" {
		return false
	}
	if isLegacyNoOp(name) {
		return true
	}
	switch strings.ToLower(name) {
	case "begin", "message", "llm", "categorize", "switch",
		"agent", "invoke", "dataoperations", "listoperations",
		"stringtransform", "variableaggregator", "variableassigner",
		"loop":
		return true
	}
	return false
}

// statePre injects the current CanvasState snapshot into the input map
// under the "state" key so the lambda body can read its inputs. It also
// synces the context-attached CanvasState from upstream outputs.
func statePre(ctx context.Context, in any) (any, error) {
	inMap, _ := in.(map[string]any)
	if inMap == nil {
		inMap = map[string]any{}
	}
	// Read CanvasState from context (set via runtime.WithState).
	ctxState, _, _ := runtime.GetStateFromContext[*runtime.CanvasState](ctx)
	if ctxState != nil {
		snapshot := ctxState.Snapshot()
		inMap["state"] = snapshot
	}
	return inMap, nil
}

// statePost flattens the node's output keys into CanvasState.Outputs
// keyed by the cpn_id from the output map.
func statePost(ctx context.Context, out any) (any, error) {
	outMap, ok := out.(map[string]any)
	if !ok || outMap == nil {
		return out, nil
	}
	cpnID, _ := outMap["__cpn_id__"].(string)
	if cpnID == "" {
		return outMap, nil
	}
	ctxState, _, _ := runtime.GetStateFromContext[*runtime.CanvasState](ctx)
	for k, v := range outMap {
		if k == "__cpn_id__" || k == "state" || k == "__legacy_noop__" {
			continue
		}
		if ctxState != nil {
			ctxState.SetVar(cpnID, k, v)
		}
	}
	return outMap, nil
}

// BuildWorkflow assembles a types.StateGraph from a Canvas DSL.
//
// Topology rules:
//   - For every cpn_id in c.Components: add a node via sg.AddNodeWithOptions.
//   - For every (cpn_id, upstream) edge: sg.AddEdge(upstream, cpn_id).
//   - For components with no upstream (Begin nodes): sg.AddEdge(Start, cpn).
//   - For components with no downstream (terminals): sg.AddEdge(cpn, End).
//   - Switch/Categorize with >= 2 children get an AddBranch for runtime routing.
func BuildWorkflow(ctx context.Context, c *Canvas) (types.StateGraph, error) {
	if c == nil {
		return nil, fmt.Errorf("canvas: nil canvas")
	}
	if len(c.Components) == 0 {
		return nil, fmt.Errorf("canvas: no components")
	}

	sg := graphpkg.NewStateGraph(map[string]any{})
	// Register the "query" channel so the Pregel engine can apply
	// input {"query": userInput} from buildRunFunc. The canvas uses
	// CanvasState (attached to ctx) for data flow, not harness
	// channels, but the engine expects all input keys to exist.
	sg.AddChannel("query", channels.NewAnyValue(nil))

	// Pre-pass 0: derive Upstream from Downstream so tests and
	// inline canvas constructions that only set Downstream also
	// produce correct edge wiring. The DSL normalizer already
	// populates both; this is a safety net.
	for cpnID, comp := range c.Components {
		for _, down := range comp.Downstream {
			if _, ok := c.Components[down]; ok {
				existing := c.Components[down].Upstream
				found := false
				for _, u := range existing {
					if u == cpnID {
						found = true
						break
					}
				}
				if !found {
					entry := c.Components[down]
					entry.Upstream = append(existing, cpnID)
					c.Components[down] = entry
				}
			}
		}
	}

	// Pre-pass 1: Loop macro expansion. For each Loop cpn, build a
	// sub-graph and create a NodeFunc closure via graph.NewLoopNodeFunc.
	// Track loop members so the main pass skips them.
	loopMembers := make(map[string]bool)
	loopNodeFuncs := make(map[string]types.NodeFunc)
	for cpnID, comp := range c.Components {
		if !strings.EqualFold(comp.Obj.ComponentName, "Loop") {
			continue
		}
		exp, err := buildLoopExpansion(ctx, c, cpnID)
		if err != nil {
			return nil, err
		}
		var opts []graphpkg.LoopOption
		if exp.MaxIters > 0 {
			opts = append(opts, graphpkg.WithLoopMaxIterations(exp.MaxIters))
		}
		loopFn, err := graphpkg.NewLoopNodeFunc(cpnID, exp.Sub, exp.ShouldQuit, opts...)
		if err != nil {
			return nil, fmt.Errorf("canvas: install loop %q: %w", cpnID, err)
		}
		loopNodeFuncs[cpnID] = loopFn
		for m := range exp.Members {
			loopMembers[m] = true
		}
	}

	// Pass 1: register every non-loop node.
	// NodeOptions with StatePre/StatePost wrappers installed.
	nodes := make(map[string]bool, len(c.Components))
	for cpnID := range c.Components {
		if _, isLoop := loopNodeFuncs[cpnID]; isLoop {
			continue
		}
		if loopMembers[cpnID] {
			continue
		}
		name := c.Components[cpnID].Obj.ComponentName
		if name == "" {
			return nil, fmt.Errorf("canvas: component %q has empty component_name", cpnID)
		}
		body, err := buildNodeBody(ctx, cpnID, name, c.Components[cpnID].Obj.Params)
		if err != nil {
			return nil, err
		}
		sg.AddNodeWithOptions(cpnID, body, types.NodeOptions{
			StatePre:  statePre,
			StatePost: statePost,
		})
		nodes[cpnID] = true
	}

	// Register loop nodes.
	for cpnID, loopFn := range loopNodeFuncs {
		sg.AddNode(cpnID, loopFn)
		nodes[cpnID] = true
	}

	// Pass 2: wire edges. For multi-upstream nodes, ALL upstreams get
	// an AddEdge (control flow triggers). Data flows through CanvasState
	// attached to ctx, not through harness channels.
	for cpnID, comp := range c.Components {
		if loopMembers[cpnID] {
			continue
		}
		for _, up := range comp.Upstream {
			if up == cpnID {
				return nil, fmt.Errorf("canvas: self-edge on %q", cpnID)
			}
			if !nodes[up] {
				if _, isLoop := loopNodeFuncs[up]; !isLoop {
					return nil, fmt.Errorf("canvas: component %q has unknown upstream %q", cpnID, up)
				}
			}
			if err := sg.AddEdge(up, cpnID); err != nil {
				return nil, fmt.Errorf("canvas: edge %s -> %s: %w", up, cpnID, err)
			}
		}
	}

	// Pass 2.5: install Branch edges for Switch/Categorize parents.
	if err := wireMultiBranches(sg, c, loopMembers); err != nil {
		return nil, fmt.Errorf("canvas: wire branches: %w", err)
	}

	// Pass 3: wire start/end nodes.
	for cpnID := range c.Components {
		if loopMembers[cpnID] {
			continue
		}
		comp := c.Components[cpnID]
		// Loop node: check outer-graph downstream separately from
		// its body members (which live inside the sub-graph).
		if _, isLoop := loopNodeFuncs[cpnID]; isLoop {
			hasOuterDownstream := false
			for _, down := range comp.Downstream {
				if !loopMembers[down] {
					hasOuterDownstream = true
					break
				}
			}
			if !hasOuterDownstream {
				if err := sg.AddEdge(cpnID, constants.End); err != nil {
					return nil, fmt.Errorf("canvas: end edge %s: %w", cpnID, err)
				}
			}
			continue
		}
		if len(comp.Upstream) == 0 {
			// No upstream → start node
			if err := sg.AddEdge(constants.Start, cpnID); err != nil {
				return nil, fmt.Errorf("canvas: start edge %s: %w", cpnID, err)
			}
		}
		if len(comp.Downstream) == 0 {
			// No downstream → terminal node
			if err := sg.AddEdge(cpnID, constants.End); err != nil {
				return nil, fmt.Errorf("canvas: end edge %s: %w", cpnID, err)
			}
		}
	}

	return sg, nil
}

// snapshotOutputs is retained as a thin wrapper around state.Snapshot()
// for any leftover callers in test/bench files.
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
