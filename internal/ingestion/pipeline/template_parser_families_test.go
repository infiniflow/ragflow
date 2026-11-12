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
//

package pipeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/ingestion/component/schema"
)

// TestBuiltinTemplatesKeepParserFamilyDeclarationsInSync pins that each
// builtin template declares the same Parser file families in both copies:
// components.params (the defaults BuildParserConfig persists) and
// graph.nodes[].data.form.setups (the canvas form the frontend renders).
//
// A family may carry no parameters and still be declared as an empty object.
// That is deliberate: the key is what tells the dataset Parser form that the
// template accepts the file type.
func TestBuiltinTemplatesKeepParserFamilyDeclarationsInSync(t *testing.T) {
	registry, err := DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}
	refs := registry.Refs()
	if len(refs) == 0 {
		t.Fatal("no builtin templates registered")
	}

	for _, ref := range refs {
		t.Run(ref, func(t *testing.T) {
			dsl, err := LoadBuiltinDSL(ref)
			if err != nil {
				t.Fatalf("LoadBuiltinDSL(%q): %v", ref, err)
			}
			var root map[string]any
			if err := json.Unmarshal([]byte(dsl), &root); err != nil {
				t.Fatalf("decode %s DSL: %v", ref, err)
			}
			assertParserFamilyDeclarationsInSync(t, ref, root)
		})
	}
}

// TestAgentTemplatesKeepParserFamilyDeclarationsInSync extends the pin to the
// seeded canvas templates under internal/agent/templates. Only templates with a
// Parser component are checked; the rest have no parser families to compare.
func TestAgentTemplatesKeepParserFamilyDeclarationsInSync(t *testing.T) {
	dir := filepath.Join("..", "..", "agent", "templates")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read agent templates: %v", err)
	}
	checked := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		var wrapper map[string]any
		if err := json.Unmarshal(raw, &wrapper); err != nil {
			t.Fatalf("decode %s: %v", name, err)
		}
		root, ok := wrapper["dsl"].(map[string]any)
		if !ok {
			root = wrapper
		}
		if _, found := parserComponentFamilies(root); !found {
			continue
		}
		checked++
		t.Run(name, func(t *testing.T) {
			assertParserFamilyDeclarationsInSync(t, name, root)
		})
	}
	if checked == 0 {
		t.Fatal("no agent templates with a Parser component found")
	}
}

func assertParserFamilyDeclarationsInSync(t *testing.T, ref string, root map[string]any) {
	t.Helper()
	componentFamilies, foundComponent := parserComponentFamilies(root)
	if !foundComponent {
		return
	}
	graphFamilies, foundGraph := parserGraphFamilies(root)
	if !foundGraph {
		return
	}
	for family := range graphFamilies {
		if !componentFamilies[family] {
			t.Errorf("%s: Parser graph form declares family %q, but components.params does not", ref, family)
		}
	}
	for family := range componentFamilies {
		if !graphFamilies[family] {
			t.Errorf("%s: Parser components.params declares family %q, but the graph form does not", ref, family)
		}
	}
}

func parserComponentFamilies(root map[string]any) (map[string]bool, bool) {
	components, _ := root["components"].(map[string]any)
	for cpnID, rawComponent := range components {
		component, _ := rawComponent.(map[string]any)
		obj, _ := component["obj"].(map[string]any)
		name, _ := obj["component_name"].(string)
		if !common.IsParserComponent(cpnID, name) {
			continue
		}
		params, _ := obj["params"].(map[string]any)
		params = schema.FlattenLegacyParserSetups(params)
		families := map[string]bool{}
		for key := range params {
			if parserSetupParamsKeys[key] {
				continue
			}
			families[key] = true
		}
		return families, true
	}
	return nil, false
}

func parserGraphFamilies(root map[string]any) (map[string]bool, bool) {
	graph, _ := root["graph"].(map[string]any)
	nodes, _ := graph["nodes"].([]any)
	for _, rawNode := range nodes {
		node, _ := rawNode.(map[string]any)
		id, _ := node["id"].(string)
		if !common.IsParserComponentID(id) {
			continue
		}
		data, _ := node["data"].(map[string]any)
		form, _ := data["form"].(map[string]any)
		setups, _ := form["setups"].([]any)
		families := map[string]bool{}
		for _, rawSetup := range setups {
			setup, _ := rawSetup.(map[string]any)
			family, _ := setup["fileFormat"].(string)
			if family != "" {
				families[family] = true
			}
		}
		return families, true
	}
	return nil, false
}
