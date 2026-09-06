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
//  distributed under the License is an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package pipeline

import (
	"encoding/json"
	"testing"
)

func TestAugmentBuiltinDSLWithCompiler_InsertsBeforeTokenizer(t *testing.T) {
	dsl, err := LoadBuiltinDSL("general")
	if err != nil {
		t.Fatalf("LoadBuiltinDSL: %v", err)
	}
	parserConfig := map[string]any{
		"Compiler:BuiltinWiki": map[string]any{
			"compilation_template_group_id": "wiki-group",
			"llm_id":                      "chat-model",
		},
	}
	augmented, err := AugmentBuiltinDSLWithCompiler(dsl, parserConfig)
	if err != nil {
		t.Fatalf("AugmentBuiltinDSLWithCompiler: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(augmented), &parsed); err != nil {
		t.Fatalf("unmarshal augmented dsl: %v", err)
	}
	components, ok := parsed["components"].(map[string]any)
	if !ok {
		t.Fatal("missing components")
	}
	compiler, ok := components["Compiler:BuiltinWiki"].(map[string]any)
	if !ok {
		t.Fatal("compiler component not injected")
	}
	upstream := compiler["upstream"].([]any)
	if len(upstream) != 1 {
		t.Fatalf("compiler upstream = %#v", upstream)
	}
	downstream := compiler["downstream"].([]any)
	if len(downstream) != 1 {
		t.Fatalf("compiler downstream = %#v", downstream)
	}
}

func TestAugmentBuiltinDSLWithCompiler_NoOpWithoutCompilerConfig(t *testing.T) {
	dsl, err := LoadBuiltinDSL("general")
	if err != nil {
		t.Fatalf("LoadBuiltinDSL: %v", err)
	}
	augmented, err := AugmentBuiltinDSLWithCompiler(dsl, map[string]any{})
	if err != nil {
		t.Fatalf("AugmentBuiltinDSLWithCompiler: %v", err)
	}
	if augmented != dsl {
		t.Fatal("expected unchanged dsl when no compiler config is present")
	}
}
