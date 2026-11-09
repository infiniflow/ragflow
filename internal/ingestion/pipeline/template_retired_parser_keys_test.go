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
)

// TestBuiltinTemplatesCarryNoRetiredParserFamilyKeys pins that no builtin
// ingestion template bakes the retired per-family keys:
//
//   - preprocess: no Go reader; chunk preprocessing is a chunker option.
//   - vlm on any family except audio: the vision model is the Parser's
//     top-level vlm block, and audio's family vlm is the ASR model.
//
// Both copies are checked: dsl.components holds the runtime params and
// dsl.graph.nodes holds the Parser form setup list the frontend renders.
func TestBuiltinTemplatesCarryNoRetiredParserFamilyKeys(t *testing.T) {
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
			assertNoRetiredParserFamilyKeysInParams(t, ref, root)
			assertNoRetiredParserFamilyKeysInGraph(t, ref, root)
		})
	}
}

func assertNoRetiredParserFamilyKeysInParams(t *testing.T, ref string, root map[string]any) {
	t.Helper()
	components, _ := root["components"].(map[string]any)
	for cpnID, rawComponent := range components {
		component, _ := rawComponent.(map[string]any)
		obj, _ := component["obj"].(map[string]any)
		if obj["component_name"] != "Parser" {
			continue
		}
		params, _ := obj["params"].(map[string]any)
		// Canvas templates saved by the Python-era frontend nest the families
		// under "setups" (compiler.json uses that shape); the runtime flattens
		// it before merging, so check the families wherever they live.
		if nested, ok := params["setups"].(map[string]any); ok {
			params = nested
		}
		for family, rawSetup := range params {
			if parserSetupParamsKeys[family] {
				continue
			}
			setup, ok := rawSetup.(map[string]any)
			if !ok {
				continue
			}
			assertNoRetiredFamilyKeys(t, ref, cpnID, family, setup)
		}
	}
}

func assertNoRetiredParserFamilyKeysInGraph(t *testing.T, ref string, root map[string]any) {
	t.Helper()
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
		for _, rawSetup := range setups {
			setup, _ := rawSetup.(map[string]any)
			fileFormat, _ := setup["fileFormat"].(string)
			assertNoRetiredFamilyKeys(t, ref, id, fileFormat, setup)
		}
	}
}

func assertNoRetiredFamilyKeys(t *testing.T, ref, cpnID, family string, setup map[string]any) {
	t.Helper()
	if value, found := setup["preprocess"]; found {
		t.Errorf("%s: Parser %s family %q declares preprocess = %v; no parser reads the key", ref, cpnID, family, value)
	}
	if family == "audio" {
		return
	}
	if value, found := setup["vlm"]; found {
		t.Errorf("%s: Parser %s family %q declares vlm = %v; the vision model is the top-level vlm block", ref, cpnID, family, value)
	}
}

// TestAgentTemplatesCarryNoRetiredParserFamilyKeys extends the pin to the
// seeded canvas templates under internal/agent/templates. The dao seeder
// copies their DSL verbatim into the canvas_template table, so a baked key
// there reaches live canvases. compiler.json is a bare DSL fixture without
// the metadata wrapper, so the "dsl" key is optional.
func TestAgentTemplatesCarryNoRetiredParserFamilyKeys(t *testing.T) {
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
		checked++
		t.Run(name, func(t *testing.T) {
			assertNoRetiredParserFamilyKeysInParams(t, name, root)
			assertNoRetiredParserFamilyKeysInGraph(t, name, root)
		})
	}
	if checked == 0 {
		t.Fatal("no agent templates found")
	}
}
