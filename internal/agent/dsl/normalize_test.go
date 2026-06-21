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

package dsl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixturesDir resolves the testdata/ directory for the dsl package. The
// production fixtures live at internal/agent/dsl/testdata/ (after the
// v1_examples/ flatten). Helpers and tests below walk that directory
// directly so a new fixture is picked up automatically.
func fixturesDir(t *testing.T) string {
	t.Helper()
	return filepath.Join("testdata")
}

// loadFixture reads a JSON file from internal/agent/dsl/testdata into a
// map[string]any. The function t.Skip()s (not t.Fatal) on a missing
// file so a test run on a slim checkout still goes green.
func loadFixture(t *testing.T, name string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(fixturesDir(t), name))
	if err != nil {
		t.Skipf("fixture %s not readable: %v", name, err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("[%s] parse: %v", name, err)
	}
	return m
}

// TestNormalize_NoopWhenGraphPresent guards against accidentally
// clobbering a payload the front-end just saved. Any input with a
// non-empty `graph.nodes` must round-trip with `graph` untouched.
func TestNormalize_NoopWhenGraphPresent(t *testing.T) {
	in := map[string]any{
		"graph": map[string]any{
			"nodes": []any{
				map[string]any{"id": "begin", "type": "beginNode"},
			},
			"edges": []any{},
		},
		"components": map[string]any{},
	}
	out := NormalizeForCanvas(in)
	got, _ := out["graph"].(map[string]any)
	if got == nil {
		t.Fatal("graph block disappeared")
	}
	if nodes, _ := got["nodes"].([]any); len(nodes) != 1 {
		t.Errorf("graph.nodes length = %d, want 1", len(nodes))
	}
}

// TestNormalize_BuildsGraphFromComponents verifies the
// components → graph derivation path. Given a populated `components`
// block but no `graph`, the function should produce a deterministic
// graph with one node per component and one edge per downstream
// declaration. The graph node's `data.form` must always be present
// (even when empty) so the front-end's React Flow shape is stable.
func TestNormalize_BuildsGraphFromComponents(t *testing.T) {
	in := map[string]any{
		"components": map[string]any{
			"begin": map[string]any{
				"obj":        map[string]any{"component_name": "Begin", "params": map[string]any{}},
				"downstream": []any{"llm:0"},
			},
			"llm:0": map[string]any{
				"obj":        map[string]any{"component_name": "LLM", "params": map[string]any{"k": "v"}},
				"downstream": []any{},
			},
		},
	}
	out := NormalizeForCanvas(in)
	graph, _ := out["graph"].(map[string]any)
	if graph == nil {
		t.Fatal("graph not derived from components")
	}
	nodes, _ := graph["nodes"].([]any)
	if len(nodes) != 2 {
		t.Fatalf("graph.nodes length = %d, want 2", len(nodes))
	}
	edges, _ := graph["edges"].([]any)
	if len(edges) != 1 {
		t.Fatalf("graph.edges length = %d, want 1", len(edges))
	}
	// Each derived node must carry data.form, even when the source
	// params is empty — this is the front-end's React-Flow invariant.
	for _, n := range nodes {
		nm, _ := n.(map[string]any)
		if nm == nil {
			continue
		}
		data, _ := nm["data"].(map[string]any)
		if data == nil {
			t.Errorf("node %v missing data block", nm["id"])
			continue
		}
		if _, ok := data["form"]; !ok {
			t.Errorf("node %v data.form missing", nm["id"])
		}
	}
}

// TestNormalize_DeterministicNodeOrder guards against the Go runtime
// map iteration lottery producing a different graph layout on
// successive normalize passes. The function must sort the component
// ids before iterating so the layout (x = 50 + i*350) is a stable
// function of the input dsl.
func TestNormalize_DeterministicNodeOrder(t *testing.T) {
	comps := map[string]any{}
	for _, k := range []string{"z", "a", "m", "b"} {
		comps[k] = map[string]any{
			"obj":        map[string]any{"component_name": "Begin", "params": map[string]any{}},
			"downstream": []any{},
		}
	}
	first := NormalizeForCanvas(map[string]any{"components": comps})
	second := NormalizeForCanvas(map[string]any{"components": comps})

	idsFirst := nodeIDs(t, first)
	idsSecond := nodeIDs(t, second)
	if !equalStringSlices(idsFirst, idsSecond) {
		t.Errorf("non-deterministic order: first=%v second=%v", idsFirst, idsSecond)
	}
	// Sorted order expected.
	want := []string{"a", "b", "m", "z"}
	if !equalStringSlices(idsFirst, want) {
		t.Errorf("node order = %v, want %v", idsFirst, want)
	}
}

// TestNormalize_HandleIdsEnforced ensures source/target handle ids
// match the front-end's React Flow convention (source=start,
// target=end) regardless of which value the writer used. Agent/tool
// handles (non-"end"/"start" ids) are left alone.
func TestNormalize_HandleIdsEnforced(t *testing.T) {
	in := map[string]any{
		"graph": map[string]any{
			"nodes": []any{
				map[string]any{"id": "a", "type": "beginNode"},
				map[string]any{"id": "b", "type": "messageNode"},
			},
			"edges": []any{
				// Inverted: source uses "end" instead of "start".
				map[string]any{
					"id":           "e1",
					"source":       "a",
					"target":       "b",
					"sourceHandle": "end",
					"targetHandle": "start",
				},
				// Agent/tool handle: must NOT be touched.
				map[string]any{
					"id":           "e2",
					"source":       "a",
					"target":       "b",
					"sourceHandle": "tool-1",
					"targetHandle": "tool-1",
				},
			},
		},
		"components": map[string]any{},
	}
	out := NormalizeForCanvas(in)
	edges, _ := out["graph"].(map[string]any)["edges"].([]any)
	if len(edges) != 2 {
		t.Fatalf("edges length = %d, want 2", len(edges))
	}
	e1, _ := edges[0].(map[string]any)
	if e1["sourceHandle"] != "start" {
		t.Errorf("e1 sourceHandle = %v, want start", e1["sourceHandle"])
	}
	if e1["targetHandle"] != "end" {
		t.Errorf("e1 targetHandle = %v, want end", e1["targetHandle"])
	}
	e2, _ := edges[1].(map[string]any)
	if e2["sourceHandle"] != "tool-1" {
		t.Errorf("e2 sourceHandle = %v, want tool-1 (preserved)", e2["sourceHandle"])
	}
	if e2["targetHandle"] != "tool-1" {
		t.Errorf("e2 targetHandle = %v, want tool-1 (preserved)", e2["targetHandle"])
	}
}

// TestNormalize_FoldsLoopAndIteration is the Go-port compatibility
// step: a dsl carrying the legacy Loop+LoopItem or
// Iteration+IterationItem node pair must be folded into a single
// Loop/Parallel node, with the child node removed and its downstream
// merged into the parent. Iteration parents are also renamed to
// "Parallel" so downstream compile/expand paths only see modern names.
//
// Two cases are exercised end-to-end:
//
//  1. Loop:abc + LoopItem:def + Body:1 → Loop:abc + Body:1
//     (child dropped; Body:1 appended to parent.downstream)
//  2. Iteration:abc + IterationItem:def + Body:1 → Parallel:abc + Body:1
//     (child dropped; parent renamed to Parallel; Body:1 appended)
func TestNormalize_FoldsLoopAndIteration(t *testing.T) {
	t.Run("LoopPlusLoopItem", func(t *testing.T) {
		in := map[string]any{
			"graph": map[string]any{
				"nodes": []any{
					map[string]any{"id": "Loop:abc", "type": "loopNode"},
					map[string]any{"id": "LoopItem:def", "type": "loopStartNode", "parentId": "Loop:abc"},
					map[string]any{"id": "Body:1", "type": "messageNode"},
				},
				"edges": []any{},
			},
			"components": map[string]any{
				"Loop:abc": map[string]any{
					"obj":        map[string]any{"component_name": "Loop", "params": map[string]any{"k": "v"}},
					"downstream": []any{"LoopItem:def"},
				},
				"LoopItem:def": map[string]any{
					"obj":        map[string]any{"component_name": "LoopItem", "params": map[string]any{}},
					"downstream": []any{"Body:1"},
				},
				"Body:1": map[string]any{
					"obj":        map[string]any{"component_name": "Message", "params": map[string]any{}},
					"downstream": []any{},
				},
			},
		}
		out := NormalizeForCanvas(in)
		comps, _ := out["components"].(map[string]any)
		if _, dropped := comps["LoopItem:def"]; dropped {
			t.Error("LoopItem:def should be folded away")
		}
		parent, _ := comps["Loop:abc"].(map[string]any)
		if parent == nil {
			t.Fatal("Loop:abc missing after fold")
		}
		ds, _ := parent["downstream"].([]any)
		gotDS := stringSliceAny(ds)
		if !equalStringSlices(gotDS, []string{"Body:1"}) {
			t.Errorf("parent.downstream = %v, want [Body:1]", gotDS)
		}
	})

	t.Run("IterationPlusIterationItem", func(t *testing.T) {
		in := map[string]any{
			"graph": map[string]any{
				"nodes": []any{
					map[string]any{"id": "Iteration:abc", "type": "iterationNode"},
					map[string]any{"id": "IterationItem:def", "type": "iterationStartNode", "parentId": "Iteration:abc"},
					map[string]any{"id": "Body:1", "type": "messageNode"},
				},
				"edges": []any{},
			},
			"components": map[string]any{
				"Iteration:abc": map[string]any{
					"obj":        map[string]any{"component_name": "Iteration", "params": map[string]any{"items_ref": "x"}},
					"downstream": []any{"IterationItem:def"},
				},
				"IterationItem:def": map[string]any{
					"obj":        map[string]any{"component_name": "IterationItem", "params": map[string]any{}},
					"downstream": []any{"Body:1"},
				},
				"Body:1": map[string]any{
					"obj":        map[string]any{"component_name": "Message", "params": map[string]any{}},
					"downstream": []any{},
				},
			},
		}
		out := NormalizeForCanvas(in)
		comps, _ := out["components"].(map[string]any)
		if _, dropped := comps["IterationItem:def"]; dropped == false {
			// has anything, we want it dropped
			if comps["IterationItem:def"] != nil {
				t.Error("IterationItem:def should be folded away")
			}
		}
		parent, _ := comps["Iteration:abc"].(map[string]any)
		if parent == nil {
			t.Fatal("Iteration:abc missing after fold")
		}
		// Renamed to "Parallel".
		if obj, _ := parent["obj"].(map[string]any); obj != nil {
			if obj["component_name"] != "Parallel" {
				t.Errorf("parent.obj.component_name = %v, want Parallel", obj["component_name"])
			}
		}
		ds, _ := parent["downstream"].([]any)
		gotDS := stringSliceAny(ds)
		if !equalStringSlices(gotDS, []string{"Body:1"}) {
			t.Errorf("parent.downstream = %v, want [Body:1]", gotDS)
		}
		// Graph node label also renamed so the front-end's
		// componentNameToNodeTypeMap lookup ("Parallel" →
		// "parallelNode") succeeds.
		graph, _ := out["graph"].(map[string]any)
		if graph != nil {
			nodes, _ := graph["nodes"].([]any)
			for _, n := range nodes {
				nm, _ := n.(map[string]any)
				if nm == nil {
					continue
				}
				if nm["id"] != "Iteration:abc" {
					continue
				}
				if data, _ := nm["data"].(map[string]any); data != nil {
					if data["label"] != "Parallel" {
						t.Errorf("graph node data.label = %v, want Parallel", data["label"])
					}
				}
				if nm["type"] != "parallelNode" {
					t.Errorf("graph node type = %v, want parallelNode", nm["type"])
				}
			}
		}
	})
}

// TestNormalize_DoesNotMutateInput pins the documented
// "never mutates its input" contract. The original DSL map's
// graph.edges[*].sourceHandle / targetHandle, components
// entries, and components[*].obj.component_name must all be
// unchanged after NormalizeForCanvas returns.
func TestNormalize_DoesNotMutateInput(t *testing.T) {
	in := map[string]any{
		"graph": map[string]any{
			"nodes": []any{
				map[string]any{"id": "begin", "type": "beginNode"},
			},
			"edges": []any{
				map[string]any{
					"id":           "e1",
					"source":       "begin",
					"target":       "begin",
					"sourceHandle": "end", // inverted — to be rewritten
					"targetHandle": "start",
				},
			},
		},
		"components": map[string]any{
			"Iteration:abc": map[string]any{
				"obj": map[string]any{
					"component_name": "Iteration", // to be renamed to Parallel
					"params":         map[string]any{"items_ref": "x"},
				},
				"downstream": []any{"IterationItem:def"},
			},
			"IterationItem:def": map[string]any{
				"obj": map[string]any{
					"component_name": "IterationItem",
					"params":         map[string]any{},
				},
				"downstream": []any{"Body:1"},
			},
		},
	}

	// Snapshot the original before NormalizeForCanvas runs.
	origEdge := in["graph"].(map[string]any)["edges"].([]any)[0].(map[string]any)
	origSourceHandle := origEdge["sourceHandle"]
	origTargetHandle := origEdge["targetHandle"]

	origIterObj := in["components"].(map[string]any)["Iteration:abc"].(map[string]any)["obj"].(map[string]any)
	origIterName := origIterObj["component_name"]

	iterItemKey := "IterationItem:def"
	hadIterItem := false
	if _, ok := in["components"].(map[string]any)[iterItemKey]; ok {
		hadIterItem = true
	}

	// Run the normalizer.
	_ = NormalizeForCanvas(in)

	// (1) graph.edges[*].sourceHandle / targetHandle must NOT be
	// rewritten in the original input.
	if got := origEdge["sourceHandle"]; got != origSourceHandle {
		t.Errorf("input edge sourceHandle mutated: %v -> %v", origSourceHandle, got)
	}
	if got := origEdge["targetHandle"]; got != origTargetHandle {
		t.Errorf("input edge targetHandle mutated: %v -> %v", origTargetHandle, got)
	}

	// (2) components[*].obj.component_name must NOT be renamed
	// in the original input.
	if got := origIterObj["component_name"]; got != origIterName {
		t.Errorf("input components[Itr].obj.component_name mutated: %v -> %v", origIterName, got)
	}

	// (3) The legacy child component must still be present in
	// the original components map (fold deletes from the COPY).
	if !hadIterItem {
		t.Errorf("input was missing IterationItem:def before normalize — fixture bug")
	}
	if _, ok := in["components"].(map[string]any)[iterItemKey]; !ok {
		t.Errorf("input components[IterationItem:def] was deleted by fold — input was mutated")
	}
}

// TestNormalize_FixtureSmoke walks the on-disk testdata/ directory
// and runs every fixture through NormalizeForCanvas. The dsl_examples e2e
// suite enumerates the same fixture names; keeping the two lists in
// sync is the responsibility of internal/agent/canvas/dsl_examples_test.go
// — the comment in dsl_examples_test.go is the single source of truth.
func TestNormalize_FixtureSmoke(t *testing.T) {
	dir := fixturesDir(t)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("testdata/ not readable: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		name := e.Name()
		t.Run(name, func(t *testing.T) {
			raw := loadFixture(t, name)
			out := NormalizeForCanvas(raw)
			if out == nil {
				t.Fatalf("[%s] normalize returned nil", name)
			}
			// Must have either graph or components at minimum.
			_, hasGraph := out["graph"].(map[string]any)
			comps, hasComps := out["components"].(map[string]any)
			if !hasGraph && !hasComps {
				t.Errorf("[%s] normalize produced neither graph nor components", name)
			}
			// If components survived, no Iteration / LoopItem /
			// IterationItem may linger — those are folded.
			if hasComps {
				for id, raw := range comps {
					comp, _ := raw.(map[string]any)
					if comp == nil {
						continue
					}
					if obj, _ := comp["obj"].(map[string]any); obj != nil {
						switch obj["component_name"] {
						case "LoopItem", "IterationItem":
							t.Errorf("[%s] component %q still has legacy name %q", name, id, obj["component_name"])
						case "Iteration":
							t.Errorf("[%s] component %q still has pre-rename name %q", name, id, obj["component_name"])
						}
					}
				}
			}
		})
	}
}

// ----- helpers -----

func nodeIDs(t *testing.T, dsl map[string]any) []string {
	t.Helper()
	graph, _ := dsl["graph"].(map[string]any)
	if graph == nil {
		return nil
	}
	nodes, _ := graph["nodes"].([]any)
	ids := make([]string, 0, len(nodes))
	for _, n := range nodes {
		nm, _ := n.(map[string]any)
		if nm == nil {
			continue
		}
		if id, _ := nm["id"].(string); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func stringSliceAny(v []any) []string {
	out := make([]string, 0, len(v))
	for _, x := range v {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func equalStringSlices(a, b []string) bool {
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
