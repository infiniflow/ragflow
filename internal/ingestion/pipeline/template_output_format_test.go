//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pipeline

import (
	"encoding/json"
	"testing"
)

// parserSetupParamsKeys are the Parser params keys that are NOT per-family
// setups: the wire-format declaration, the global vision settings and the
// legacy allow-list. Every other object-valued params key is a file family
// whose setup map may carry output_format.
var parserSetupParamsKeys = map[string]bool{
	"outputs":                   true,
	"allowed_output_format":     true,
	"enable_vision_enhancement": true,
	"vlm":                       true,
}

// TestBuiltinTemplatesCarryNoParserOutputFormat pins that no builtin
// ingestion template declares a Parser family's output_format.
//
// The Go Parser component exposes JSON only: the Parser runtime overwrites
// every family's output_format with "json" when the component is built, so a
// template-baked value never reaches the parser that would read it. The key
// is therefore dead configuration in the templates: it states an intent the
// runtime discards, and leaving it in place invites a reader to believe the
// value is honoured.
//
// Both copies are checked. dsl.components holds the params the runtime
// builds the Parser component from; dsl.graph.nodes holds the per-family
// setup list the frontend renders as the Parser form, which is a separate
// serialization of the same configuration and must not drift back.
func TestBuiltinTemplatesCarryNoParserOutputFormat(t *testing.T) {
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

			assertNoParserOutputFormatInParams(t, ref, root)
			assertNoParserOutputFormatInGraph(t, ref, root)
		})
	}
}

// assertNoParserOutputFormatInParams checks the dsl.components copy: every
// Parser component's per-family setup map must not declare output_format.
func assertNoParserOutputFormatInParams(t *testing.T, ref string, root map[string]any) {
	t.Helper()
	components, _ := root["components"].(map[string]any)
	for cpnID, rawComponent := range components {
		component, _ := rawComponent.(map[string]any)
		obj, _ := component["obj"].(map[string]any)
		if obj["component_name"] != "Parser" {
			continue
		}
		params, _ := obj["params"].(map[string]any)
		for family, rawSetup := range params {
			if parserSetupParamsKeys[family] {
				continue
			}
			setup, ok := rawSetup.(map[string]any)
			if !ok {
				continue
			}
			if value, found := setup["output_format"]; found {
				t.Errorf("%s: Parser %s family %q declares output_format = %v; the Parser runtime overwrites it with \"json\", so the key is dead configuration", ref, cpnID, family, value)
			}
		}
	}
}

// assertNoParserOutputFormatInGraph checks the dsl.graph.nodes copy: every
// Parser node's form setup list must not declare output_format. The frontend
// renders these entries as the Parser form, so a value here reaches the UI
// even though the runtime would discard it.
func assertNoParserOutputFormatInGraph(t *testing.T, ref string, root map[string]any) {
	t.Helper()
	graph, _ := root["graph"].(map[string]any)
	nodes, _ := graph["nodes"].([]any)
	for _, rawNode := range nodes {
		node, _ := rawNode.(map[string]any)
		id, _ := node["id"].(string)
		if !IsParserComponentID(id) {
			continue
		}
		data, _ := node["data"].(map[string]any)
		form, _ := data["form"].(map[string]any)
		setups, _ := form["setups"].([]any)
		for _, rawSetup := range setups {
			setup, _ := rawSetup.(map[string]any)
			fileFormat, _ := setup["fileFormat"].(string)
			if value, found := setup["output_format"]; found {
				t.Errorf("%s: Parser node %s family %q declares output_format = %v; the Parser runtime overwrites it with \"json\", so the key is dead configuration", ref, id, fileFormat, value)
			}
		}
	}
}
