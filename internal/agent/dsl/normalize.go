//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

// Package dsl — single-shape canvas normalizer.
//
// The RAGFlow agent DSL has exactly one canonical wire shape:
//
//	{
//	  "globals":   { ... },
//	  "graph":     { "nodes": [...], "edges": [...] },   // React-Flow layout
//	  "variables": { ... },
//	  "components": { "<Name>:<UUID>": {                 // execution topology
//	    "downstream": [...], "upstream": [...],
//	    "obj": { "component_name": "Name", "params": {...} }
//	  }},
//	  "path": [...], "retrieval": {...}, "history": [...]
//	}
//
// Go server code (handler/service/Compile) reads the `components` block —
// the Python server (agent/canvas.py) does the same. The `graph` block is
// consumed by the React-Flow front-end to render the canvas. Either side
// can be missing on a given row (e.g. a hand-imported v1 export from a
// Python server has `components` but no `graph`; a v1 fixture from the
// Go port test suite has `graph` and `components` but a slightly
// different internal layout convention).
//
// NormalizeForCanvas is the decoder-boundary entry point for every
// front-end-facing Go path (handler.AgentHandler, service.AgentService
// create/update/publish/reset, version reads). The function:
//
//  1. Repairs React-Flow handle ids on whatever `graph.edges` are present
//     (source/target handle ids: source=start, target=end).
//  2. If `graph.nodes` is missing but `components` is non-empty, builds
//     a default-layout graph from the components (deterministic order,
//     50/200/350 px column layout).
//  3. Repairs any historically leaked runtime-only Parallel /
//     parallelNode canvas shape back to the front-end's Iteration /
//     iterationNode protocol.
//  4. Returns a defensive copy of the input with all transforms
//     applied. Never mutates its input.
//
// The function never panics on malformed input; unparseable entries are
// skipped and a best-effort graph is returned.
//
// IMPORTANT: this function preserves the front-end canvas protocol. It
// must not leak runtime-only node kinds (for example "Parallel" /
// "parallelNode") into `graph.nodes` or rewrite user-authored DSL
// semantics. Runtime-only folding lives in NormalizeForRun.

package dsl

import (
	"regexp"
	"sort"
)

// componentNameIteration / componentNameIterationItem are the legacy
// v1 names that the front-end may still emit. The Go port's runtime
// uses "Parallel" for the same concept; this constant is the
// pre-rename label.
const (
	componentNameLoop          = "Loop"
	componentNameLoopItem      = "LoopItem"
	componentNameIteration     = "Iteration"
	componentNameIterationItem = "IterationItem"
	componentNameParallel      = "Parallel"
)

var legacyIterationAliasPattern = regexp.MustCompile(`IterationItem:[A-Za-z0-9_:-]+@(item|index)\b`)

// NormalizeForCanvas returns a defensive copy of dsl with a derived
// `graph.nodes`/`graph.edges` block when missing.
//
// Behaviour:
//   - nil in, nil out.
//   - graph.nodes already non-empty: handle ids are still repaired in
//     place (idempotent). Otherwise graph is derived from components.
//   - empty / components:{}: no-op, returns dsl as-is.
//   - any components: builds graph only.
//   - any historically leaked Parallel / parallelNode canvas state is
//     repaired back to Iteration / iterationNode.
//
// The function never panics on malformed input; unparseable entries are
// skipped and a best-effort graph is returned.
func NormalizeForCanvas(dsl map[string]any) map[string]any {
	return normalize(dsl, false)
}

// NormalizeForRun prepares a DSL for the runtime/compiler path. Unlike
// NormalizeForCanvas, it is allowed to fold legacy LoopItem /
// IterationItem children away and rename Iteration to Parallel because
// the returned map never goes back to the front-end.
func NormalizeForRun(dsl map[string]any) map[string]any {
	return normalize(dsl, true)
}

func normalize(dsl map[string]any, foldLegacy bool) map[string]any {
	if dsl == nil {
		return nil
	}
	// Defensive deep copy: the normalize pipeline rewrites
	// graph.edges[*].sourceHandle / targetHandle, deletes
	// components entries, and mutates components[*].obj.component_name
	// — all in place. Without the deep copy, callers that reuse
	// the original decoded DSL map would observe side effects.
	out := deepCopyDSL(dsl)

	// (1) Repair React-Flow handle ids on whatever edges exist.
	enforceHandleIds(out)

	// (2) Build a default-layout graph if missing.
	if !graphHasNodes(out) {
		rawComps, _ := out["components"].(map[string]any)
		if len(rawComps) > 0 {
			nodes, edges, normComps := buildGraphFromComponents(rawComps)
			if len(nodes) > 0 {
				out["graph"] = map[string]any{
					"nodes": nodes,
					"edges": edges,
				}
				out["components"] = normComps
			}
		}
	}

	// (3) Repair any historically leaked runtime-only Parallel /
	// parallelNode view back to the front-end's Iteration /
	// iterationNode protocol. This keeps response payloads
	// renderable without exposing backend implementation details.
	repairParallelLeaksForCanvas(out)

	if foldLegacy {
		// (4) Runtime-only compatibility: fold legacy Loop+LoopItem and
		// Iteration+IterationItem pairs in place. This step uses
		// graph.nodes[*].parentId to discover parent/child
		// relationships; if `graph` is still missing the fold degrades
		// to a pure rename (component_name: "Iteration" → "Parallel";
		// LoopItem/IterationItem names stay in components but
		// downstream compile/expand paths must tolerate them).
		foldLegacyLoopVariants(out)

		rewriteLegacyIterationAliases(out)
	}

	return out
}

// rewriteLegacyIterationAliases rewrites runtime-only references to the
// legacy IterationItem child's synthetic outputs back to the modern
// item/index aliases that CanvasState exposes. This runs only on the
// runtime-normalized copy, never on the front-end-facing canvas view.
func rewriteLegacyIterationAliases(dsl map[string]any) {
	for k, v := range dsl {
		switch x := v.(type) {
		case string:
			dsl[k] = replaceLegacyIterationAliasRefs(x)
		case map[string]any:
			rewriteLegacyIterationAliases(x)
		case []any:
			rewriteLegacyIterationAliasesInSlice(x)
		}
	}
}

func rewriteLegacyIterationAliasesInSlice(items []any) {
	for i, v := range items {
		switch x := v.(type) {
		case string:
			items[i] = replaceLegacyIterationAliasRefs(x)
		case map[string]any:
			rewriteLegacyIterationAliases(x)
		case []any:
			rewriteLegacyIterationAliasesInSlice(x)
		}
	}
}

func replaceLegacyIterationAliasRefs(s string) string {
	return legacyIterationAliasPattern.ReplaceAllStringFunc(s, func(match string) string {
		sub := legacyIterationAliasPattern.FindStringSubmatch(match)
		if len(sub) != 2 {
			return match
		}
		alias := sub[1]
		switch alias {
		case "item", "index":
			return alias
		default:
			return match
		}
	})
}

// repairParallelLeaksForCanvas rewrites any historically leaked
// runtime-only Parallel / parallelNode view back to the front-end's
// Iteration / iterationNode protocol. This is a response-shape repair
// only; it does not perform parent/child folding.
func repairParallelLeaksForCanvas(dsl map[string]any) {
	rawComps, _ := dsl["components"].(map[string]any)
	for _, raw := range rawComps {
		comp, _ := raw.(map[string]any)
		if comp == nil {
			continue
		}
		if obj, ok := comp["obj"].(map[string]any); ok {
			if obj["component_name"] == componentNameParallel {
				obj["component_name"] = componentNameIteration
			}
		}
		if comp["name"] == componentNameParallel {
			comp["name"] = componentNameIteration
		}
	}

	graph, _ := dsl["graph"].(map[string]any)
	if graph == nil {
		return
	}
	nodes, _ := graph["nodes"].([]any)
	for _, raw := range nodes {
		node, _ := raw.(map[string]any)
		if node == nil {
			continue
		}
		if node["type"] == "parallelNode" {
			node["type"] = "iterationNode"
		}
		data, _ := node["data"].(map[string]any)
		if data == nil {
			continue
		}
		if data["label"] == componentNameParallel {
			data["label"] = componentNameIteration
		}
		if data["name"] == componentNameParallel {
			data["name"] = componentNameIteration
		}
	}
}

// enforceHandleIds rewrites graph.edges[*].sourceHandle / targetHandle
// to the front-end's React Flow convention. Tool/agent handles (id !=
// "end" on source / != "start" on target) are left alone because they
// are not produced by the basic component DAG.
func enforceHandleIds(dsl map[string]any) {
	graph, _ := dsl["graph"].(map[string]any)
	if graph == nil {
		return
	}
	edges, _ := graph["edges"].([]any)
	if len(edges) == 0 {
		return
	}
	for _, e := range edges {
		m, _ := e.(map[string]any)
		if m == nil {
			continue
		}
		// Only rewrite plain "start"/"end" conventions. Agent/tool
		// handles carry semantic info we must not stomp.
		if src, _ := m["sourceHandle"].(string); src == "end" || src == "start" {
			m["sourceHandle"] = "start"
		}
		if dst, _ := m["targetHandle"].(string); dst == "start" || dst == "end" {
			m["targetHandle"] = "end"
		}
	}
}

// graphHasNodes reports whether the input already carries a non-empty
// React-Flow-shaped graph. Any missing / wrong-typed sub-key counts as
// "no graph", which is the conservative default.
func graphHasNodes(dsl map[string]any) bool {
	graph, ok := dsl["graph"].(map[string]any)
	if !ok {
		return false
	}
	nodes, ok := graph["nodes"].([]any)
	if !ok {
		return false
	}
	return len(nodes) > 0
}

// buildGraphFromComponents converts the `components` block into
// React-Flow-shaped nodes + edges and a normalised (flat) components
// map keyed the same way the input was.
//
// Layout strategy: simple left-to-right single row, x = 50 + i*350,
// y = 200. Cycles are not detected — every component gets its own slot
// in iteration order. The user is expected to re-arrange via the
// front-end, which is consistent with how legacy data used to be
// surfaced to the editor before the bug-fix.
func buildGraphFromComponents(components map[string]any) (nodes []any, edges []any, normalized map[string]any) {
	nodes = make([]any, 0, len(components))
	edges = make([]any, 0)
	normalized = make(map[string]any, len(components))

	// Go's map iteration is randomised. Sort the component ids before
	// iterating so the layout (x = 50 + i*350) is a stable function of
	// the input dsl, not of Go's runtime iteration lottery. Two
	// normalize passes over the same dsl also used to produce
	// `components` and `graph.nodes` in different orders, which broke
	// the dslToGraph equality invariant; sorting here closes that gap.
	keys := make([]string, 0, len(components))
	for k := range components {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	const xStep = 350.0
	const yBase = 200.0
	i := 0
	for _, key := range keys {
		raw := components[key]
		comp, _ := raw.(map[string]any)
		if comp == nil {
			continue
		}
		name, params, downstream := extractComponent(comp)
		if name == "" {
			name = key
		}
		node := map[string]any{
			"id":       key,
			"type":     componentNameToNodeType(name),
			"position": map[string]any{"x": 50.0 + float64(i)*xStep, "y": yBase},
			// Always emit `data.form` (even when empty) so the React
			// Flow node shape is byte-equal between the Python v1
			// fallback (which reads `obj.params` and may be `{}`) and
			// the Go v2 path. The same invariant applies to the
			// normalised components map above.
			"data":           map[string]any{"label": name, "name": name, "form": params},
			"sourcePosition": "right",
			"targetPosition": "left",
		}
		nodes = append(nodes, node)

		for _, dst := range downstream {
			edges = append(edges, map[string]any{
				"id":     "xy-edge__" + key + "-" + dst,
				"source": key,
				"target": dst,
				// Source/target handle ids match the front-end's React Flow
				// convention (web/src/pages/agent/hooks/use-add-node.ts:114):
				//   source node renders its OUTPUT handle with id = "start"
				//   target node renders its INPUT  handle with id = "end"
				"sourceHandle": "start",
				"targetHandle": "end",
			})
		}

		flat := map[string]any{
			"id":         key,
			"name":       name,
			"downstream": toStringSlice(comp["downstream"]),
			"upstream":   toStringSlice(comp["upstream"]),
			// Always emit `params` (even when empty) so the normalised
			// component shape matches the Python v1 server byte-for-byte.
			"params": params,
		}
		normalized[key] = flat
		i++
	}
	return nodes, edges, normalized
}

// extractComponent pulls (name, params, downstream) out of a component
// block. The Go port stores them flat (`name` / `params` at top
// level). Returns empty values for missing fields.
func extractComponent(comp map[string]any) (name string, params map[string]any, downstream []string) {
	if obj, ok := comp["obj"].(map[string]any); ok {
		name, _ = obj["component_name"].(string)
		if p, ok := obj["params"].(map[string]any); ok {
			params = p
		}
		// Read obj.downstream first; the trailing outer-downstream
		// append below handles the case where the v1 writer put the
		// topology on the outer field. Use a local var so the nil
		// check stays unambiguous to the nilness analyser.
		var ds []string
		ds = toStringSlice(obj["downstream"])
		if len(ds) > 0 {
			downstream = ds
		}
	}
	if name == "" {
		name, _ = comp["name"].(string)
	}
	if params == nil {
		if p, ok := comp["params"].(map[string]any); ok {
			params = p
		}
	}
	downstream = append(downstream, toStringSlice(comp["downstream"])...)
	return name, params, downstream
}

func toStringSlice(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, x := range arr {
		if s, ok := x.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// deepCopyDSL returns a deep copy of the parts of `dsl` that
// NormalizeForCanvas mutates: the top-level keys "graph" and
// "components", and within `graph` the "nodes" and "edges" slices.
// All other top-level keys (`globals`, `variables`, `path`,
// `retrieval`, `history`, `*`) are shallow-copied by reference —
// they are read-only and never modified by the normalize pipeline.
//
// The deep copy is required because:
//   - enforceHandleIds rewrites graph.edges[*].sourceHandle /
//     targetHandle in place.
//   - foldLegacyLoopVariants deletes entries from components,
//     rewrites components[*].obj.component_name, and rewrites
//     graph.nodes[*].data.label / type.
//
// Without the deep copy, a caller that reuses the original
// decoded DSL map (e.g. for re-validation or diffing) would
// observe side effects that contradict the documented
// "never mutates its input" contract.
//
// Primitives and non-mutable values (string, number, bool) are
// shared by reference; only the maps and slices that the
// normalize pipeline touches are duplicated.
func deepCopyDSL(dsl map[string]any) map[string]any {
	out := make(map[string]any, len(dsl)+1)
	for k, v := range dsl {
		switch k {
		case "graph":
			if g, ok := v.(map[string]any); ok {
				out["graph"] = deepCopyGraph(g)
			} else {
				out["graph"] = v
			}
		case "components":
			if c, ok := v.(map[string]any); ok {
				out["components"] = deepCopyComponents(c)
			} else {
				out["components"] = v
			}
		default:
			// Shallow: globals, variables, path, retrieval,
			// history, and any other top-level key are not
			// mutated by the normalize pipeline. Sharing the
			// reference is safe.
			out[k] = v
		}
	}
	return out
}

// deepCopyAny returns a recursive deep copy of v. Maps and slices
// are duplicated recursively; primitives and nil are passed through.
// This ensures mutations to nested fields (e.g. data.label, data.name)
// never alias into the caller's original input.
func deepCopyAny(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[k] = deepCopyAny(val)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = deepCopyAny(val)
		}
		return out
	default:
		return v
	}
}

// deepCopyGraph copies a graph block. Nodes and edges are deep-copied
// element-by-element so that later mutations (e.g. data.label rewrite
// in fixComponentNames) target the copy, not the caller's input.
func deepCopyGraph(g map[string]any) map[string]any {
	out := make(map[string]any, len(g))
	for k, v := range g {
		switch k {
		case "nodes":
			if nodes, ok := v.([]any); ok {
				copied := make([]any, len(nodes))
				for i, n := range nodes {
					copied[i] = deepCopyAny(n)
				}
				out["nodes"] = copied
			} else {
				out["nodes"] = v
			}
		case "edges":
			if edges, ok := v.([]any); ok {
				copied := make([]any, len(edges))
				for i, e := range edges {
					copied[i] = deepCopyAny(e)
				}
				out["edges"] = copied
			} else {
				out["edges"] = v
			}
		default:
			out[k] = v
		}
	}
	return out
}

// deepCopyComponents copies a components block. Each component
// entry is a new map; the `obj` sub-map (when present) is also
// deep-copied so rewrites to component_name / params land on
// the copy.
func deepCopyComponents(c map[string]any) map[string]any {
	out := make(map[string]any, len(c))
	for k, v := range c {
		if cm, ok := v.(map[string]any); ok {
			entry := deepCopyAny(cm).(map[string]any)
			if obj, ok := cm["obj"].(map[string]any); ok {
				entry["obj"] = deepCopyAny(obj)
			}
			out[k] = entry
		} else {
			out[k] = v
		}
	}
	return out
}

// copyMapStringAny returns a shallow copy of m. The new map
// aliases the original values; callers that need a deeper copy
// recurse on their own (e.g. deepCopyGraph / deepCopyComponents
// recurse on `obj` and on each node / edge).
func copyMapStringAny(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// stringsToAny is the inverse of toStringSlice: it packs a []string
// back into a []any so a downstream `.([]any)` type assertion
// succeeds. The fold step needs this because it computes a
// []string and the parent component's downstream slot is consumed
// elsewhere as []any.
func stringsToAny(s []string) []any {
	out := make([]any, 0, len(s))
	for _, x := range s {
		if x == "" {
			continue
		}
		out = append(out, x)
	}
	if len(out) == 0 {
		return []any{}
	}
	return out
}

// componentNameToNodeType maps a component_name to the front-end React
// Flow node type. Unknown names fall back to "agentNode" — the
// front-end re-derives the operator from `data.label`, so an unknown
// type is still rendered (just possibly in a generic shape). The user
// can re-pick a type from the operator palette to refine.
var componentNameToNodeTypeMap = map[string]string{
	"Begin":              "beginNode",
	"Retrieval":          "ragNode",
	"Categorize":         "categorizeNode",
	"Message":            "messageNode",
	"Answer":             "messageNode",
	"RewriteQuestion":    "rewriteNode",
	"ExeSQL":             "toolNode",
	"Switch":             "switchNode",
	"Agent":              "agentNode",
	"Tool":               "toolNode",
	"File":               "fileNode",
	"Parser":             "parserNode",
	"Tokenizer":          "tokenizerNode",
	"TokenChunker":       "chunkerNode",
	"TitleChunker":       "chunkerNode",
	"Extractor":          "contextNode",
	"Loop":               "loopNode",
	"LoopStart":          "loopStartNode",
	"ExitLoop":           "exitLoopNode",
	"Iteration":          "iterationNode",
	"IterationStart":     "iterationStartNode",
	"Parallel":           "parallelNode",
	"DataOperations":     "dataOperationsNode",
	"ListOperations":     "listOperationsNode",
	"VariableAssigner":   "variableAssignerNode",
	"VariableAggregator": "variableAggregatorNode",
	"Keyword":            "keywordNode",
	"Note":               "noteNode",
	"Placeholder":        "placeholderNode",
	"Code":               "toolNode",
}

func componentNameToNodeType(name string) string {
	if t, ok := componentNameToNodeTypeMap[name]; ok {
		return t
	}
	return "agentNode"
}

// foldLegacyLoopVariants collapses Loop+LoopItem and Iteration+IterationItem
// node pairs into single Loop / Parallel nodes. The fold runs at the
// decoder boundary so every Go path (handler, service, future Compile)
// inherits the compatibility for free.
//
// The algorithm:
//
//  1. Build a childOf map from graph.nodes[*].parentId. If graph is
//     missing, childOf is empty and the fold degrades to a pure rename.
//  2. For each component whose component_name is "LoopItem" or
//     "IterationItem": drop it from components and, if its parent is
//     known, append the child's downstream to the parent's downstream
//     (preserving the React-Flow edge topology).
//  3. Rename remaining "Iteration" parents to "Parallel" so the
//     downstream compile / expand paths only need to know about the
//     modern names. Loop parents keep their canonical "Loop" name.
//
// Notes:
//   - Child params are not merged into the parent. The control surface
//     (loop_variables / loop_termination_condition / items_ref) lives
//     on the parent; child params typically carry only `outputs` schema
//     declarations that are derived at runtime, not stored in the dsl.
//   - This function mutates `dsl` in place (the caller already gave us
//     a defensive copy at the top of NormalizeForCanvas).
func foldLegacyLoopVariants(dsl map[string]any) {
	rawComps, _ := dsl["components"].(map[string]any)
	if len(rawComps) == 0 {
		return
	}

	// Build parent-of map from graph.nodes. parentId is a React-Flow
	// node-level field (verified in testdata/all.json:309).
	childOf := buildParentMap(dsl)

	// (1) Walk every component, drop legacy children, append their
	// downstream to the parent's downstream.
	for childID, raw := range rawComps {
		comp, _ := raw.(map[string]any)
		if comp == nil {
			continue
		}
		childName := componentNameFromComp(comp)
		if !isLegacyChildName(childName) {
			continue
		}
		parentID, ok := childOf[childID]
		if !ok {
			// No parent visible in the graph via parentId
			// mapping. Keep the child — deleting it could
			// leave dangling downstream references in the
			// parent component.
			continue
		}
		parentRaw, ok := rawComps[parentID]
		if !ok {
			delete(rawComps, childID)
			continue
		}
		parentComp, _ := parentRaw.(map[string]any)
		if parentComp == nil {
			delete(rawComps, childID)
			continue
		}
		// Append child downstream to parent downstream, then drop
		// the child id itself from the parent's downstream list —
		// the child is the entry node, not an execution target, so
		// once folded it must not appear in any edge. The result is
		// stored as []any (not []string) so a consumer can do a
		// `parent["downstream"].([]any)` type assertion without
		// losing data.
		childDS := toStringSlice(childCompDownstream(comp))
		merged := mergeDownstream(toStringSlice(parentComp["downstream"]), childDS)
		merged = removeFromSlice(merged, childID)
		parentComp["downstream"] = stringsToAny(merged)
		// Also append to the parent graph node's downstream, if we
		// have a graph. This keeps the React-Flow edges in sync with
		// the topology map.
		if graph, _ := dsl["graph"].(map[string]any); graph != nil {
			if nodes, _ := graph["nodes"].([]any); nodes != nil {
				for _, n := range nodes {
					nm, _ := n.(map[string]any)
					if nm == nil {
						continue
					}
					if id, _ := nm["id"].(string); id == parentID {
						// The graph node's downstream isn't a
						// standard field; the standard React-Flow
						// topology is encoded in `edges`. Leave the
						// graph node alone; the user will re-save
						// to re-derive edges from components.
					}
				}
			}
		}
		// Drop the child from components.
		delete(rawComps, childID)
	}

	// (2) Rename "Iteration" parents to "Parallel". The
	// `component_name` lives under `obj.component_name` (v1 shape) or
	// `name` (Go flat shape); we rewrite both keys for safety.
	for id, raw := range rawComps {
		comp, _ := raw.(map[string]any)
		if comp == nil {
			continue
		}
		if componentNameFromComp(comp) != componentNameIteration {
			continue
		}
		if obj, ok := comp["obj"].(map[string]any); ok {
			obj["component_name"] = componentNameParallel
		}
		comp["name"] = componentNameParallel
		// Also rewrite the graph node label so the React-Flow
		// renderer's componentNameToNodeTypeMap lookup ("Parallel" →
		// "parallelNode") succeeds on the next paint.
		if graph, _ := dsl["graph"].(map[string]any); graph != nil {
			if nodes, _ := graph["nodes"].([]any); nodes != nil {
				for _, n := range nodes {
					nm, _ := n.(map[string]any)
					if nm == nil || nm["id"] != id {
						continue
					}
					if data, _ := nm["data"].(map[string]any); data != nil {
						data["label"] = componentNameParallel
						data["name"] = componentNameParallel
					}
					nm["type"] = componentNameToNodeType(componentNameParallel)
				}
			}
		}
	}
}

// buildParentMap scans graph.nodes for React-Flow's parentId field and
// returns id → parentID. Returns an empty map if graph or nodes is
// missing.
func buildParentMap(dsl map[string]any) map[string]string {
	out := map[string]string{}
	graph, _ := dsl["graph"].(map[string]any)
	if graph == nil {
		return out
	}
	nodes, _ := graph["nodes"].([]any)
	if len(nodes) == 0 {
		return out
	}
	for _, n := range nodes {
		nm, _ := n.(map[string]any)
		if nm == nil {
			continue
		}
		id, _ := nm["id"].(string)
		parent, _ := nm["parentId"].(string)
		if id != "" && parent != "" {
			out[id] = parent
		}
	}
	return out
}

// componentNameFromComp returns the component_name from either the
// nested `obj` (v1) or the flat `name` (Go) shape.
func componentNameFromComp(comp map[string]any) string {
	if obj, ok := comp["obj"].(map[string]any); ok {
		if n, _ := obj["component_name"].(string); n != "" {
			return n
		}
	}
	if n, _ := comp["name"].(string); n != "" {
		return n
	}
	return ""
}

// childCompDownstream returns a child component's downstream list,
// looking at the outer `downstream` (v1) and `obj.downstream` (legacy
// v1 double-write) keys.
func childCompDownstream(comp map[string]any) any {
	if d, ok := comp["downstream"]; ok {
		return d
	}
	if obj, ok := comp["obj"].(map[string]any); ok {
		if d, ok := obj["downstream"]; ok {
			return d
		}
	}
	return nil
}

// mergeDownstream returns parent ∪ child in stable order, with parent
// entries first. Duplicates dropped.
func mergeDownstream(parent, child []string) []string {
	if len(child) == 0 {
		return parent
	}
	seen := make(map[string]bool, len(parent)+len(child))
	out := make([]string, 0, len(parent)+len(child))
	for _, s := range parent {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	for _, s := range child {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	if len(out) == 0 {
		return []string{}
	}
	return out
}

// removeFromSlice returns a copy of s with the first occurrence of
// drop removed (or s unchanged if drop is absent). Used by the loop
// fold to filter the child id out of the parent's downstream list
// once the child has been merged in.
func removeFromSlice(s []string, drop string) []string {
	if drop == "" {
		return s
	}
	out := make([]string, 0, len(s))
	for _, x := range s {
		if x == drop {
			continue
		}
		out = append(out, x)
	}
	return out
}

// isLegacyChildName reports whether name is a legacy parent-child
// control node that should be folded away.
func isLegacyChildName(name string) bool {
	return name == componentNameLoopItem || name == componentNameIterationItem
}
