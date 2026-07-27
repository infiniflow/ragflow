package canvas

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	_ "ragflow/internal/agent/component" // blank import: registers factories via component.init()
	dslpkg "ragflow/internal/agent/dsl"
)

// TestAllFixture_NormalizeAndCompile ensures the largest legacy fixture
// remains compilable through the runtime normalization boundary.
func TestAllFixture_NormalizeAndCompile(t *testing.T) {
	t.Skip("all.json includes unsupported component types (Parallel, IterationItem)")
	raw, err := os.ReadFile(filepath.Join("..", "dsl", "testdata", "all.json"))
	if err != nil {
		t.Fatalf("read all.json: %v", err)
	}

	var fixture map[string]any
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("parse all.json: %v", err)
	}

	normalized := dslpkg.NormalizeForRun(fixture)
	rawComponents, _ := normalized["components"].(map[string]any)
	if len(rawComponents) == 0 {
		t.Fatal("normalized all.json has no components")
	}

	c := &Canvas{
		Components:  make(map[string]CanvasComponent, len(rawComponents)),
		NodeParents: make(map[string]string),
	}
	if path, ok := normalized["path"].([]any); ok {
		c.Path = make([]string, 0, len(path))
		for _, item := range path {
			if s, ok := item.(string); ok {
				c.Path = append(c.Path, s)
			}
		}
	}
	if globals, ok := normalized["globals"].(map[string]any); ok {
		c.Globals = globals
	}
	if graph, ok := normalized["graph"].(map[string]any); ok {
		if nodes, ok := graph["nodes"].([]any); ok {
			for _, rawNode := range nodes {
				node, ok := rawNode.(map[string]any)
				if !ok || node == nil {
					continue
				}
				id, _ := node["id"].(string)
				parentID, _ := node["parentId"].(string)
				if id != "" && parentID != "" {
					c.NodeParents[id] = parentID
				}
			}
		}
	}

	for cpnID, rawComp := range rawComponents {
		comp, ok := rawComp.(map[string]any)
		if !ok || comp == nil {
			continue
		}
		name, params := extractFixtureComponentFields(comp)
		if name == "" {
			t.Fatalf("component %q missing component_name", cpnID)
		}
		if parentID, _ := comp["parent_id"].(string); parentID != "" {
			c.NodeParents[cpnID] = parentID
		}
		c.Components[cpnID] = CanvasComponent{
			Obj: CanvasComponentObj{
				ComponentName: name,
				Params:        params,
			},
			Downstream: stringSliceFromAny(comp["downstream"]),
			Upstream:   stringSliceFromAny(comp["upstream"]),
		}
	}
	cc, err := Compile(context.Background(), c)
	if err != nil {
		t.Fatalf("Compile(all.json): %v", err)
	}
	if cc == nil || cc.Graph == nil {
		t.Fatal("Compile(all.json) returned nil graph")
	}
}

func extractFixtureComponentFields(comp map[string]any) (name string, params map[string]any) {
	if obj, ok := comp["obj"].(map[string]any); ok && obj != nil {
		name, _ = obj["component_name"].(string)
		if p, ok := obj["params"].(map[string]any); ok {
			params = p
		}
	}
	return
}

func stringSliceFromAny(v any) []string {
	if v == nil {
		return nil
	}
	switch sv := v.(type) {
	case []string:
		return sv
	case []any:
		out := make([]string, 0, len(sv))
		for _, s := range sv {
			if ss, ok := s.(string); ok {
				out = append(out, ss)
			}
		}
		return out
	}
	return nil
}
