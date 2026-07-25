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

// Cross-domain smoke test for runtime.DefaultRegistry.
//
// This file lives in internal/registry/ — a dedicated package
// that imports BOTH internal/agent/component AND
// internal/ingestion/component, triggering their init() side
// effects, then verifies that runtime.DefaultRegistry.Lookup
// resolves names from each category (plan §9 #6).
//
// A unit test inside internal/agent/component cannot trigger
// internal/ingestion/component's init() without importing it
// (Go has no import-by-side-effect-only mechanism); putting
// the smoke test here keeps the import graph acyclic and
// proves the cross-domain registration works.
package registry

import (
	"testing"

	"ragflow/internal/agent/component"
	_ "ragflow/internal/agent/component" // blank import: registers agent factories
	"ragflow/internal/agent/runtime"
	_ "ragflow/internal/ingestion/component"         // blank import: registers ingestion factories
	_ "ragflow/internal/ingestion/component/chunker" // blank import: registers 4 chunker variants
)

// TestRegistry_CrossDomain verifies the runtime registry sees
// both agent and ingestion components from this single test
// binary. If either category is empty, the cross-domain
// wiring is broken.
func TestRegistry_CrossDomain(t *testing.T) {
	agentNames := runtime.DefaultRegistry.NamesByCategory(runtime.CategoryAgent)
	ingestionNames := runtime.DefaultRegistry.NamesByCategory(runtime.CategoryIngestion)

	if len(agentNames) == 0 {
		t.Errorf("expected at least one agent component; got 0")
	}
	if len(ingestionNames) == 0 {
		t.Errorf("expected at least one ingestion component; got 0")
	}

	// Spot-check a few known names.
	wantIngestion := []string{"File", "Parser", "Tokenizer", "Extractor"}
	for _, name := range wantIngestion {
		if _, _, _, ok := runtime.DefaultRegistry.Lookup(name); !ok {
			t.Errorf("expected ingestion %q registered; Names=%v", name, ingestionNames)
		}
	}

	// Agent components: at least Begin, LLM, Message, Agent
	// are common — assert Begin as a known constant.
	if _, _, _, ok := runtime.DefaultRegistry.Lookup("Begin"); !ok {
		t.Errorf("expected agent Begin registered; Names=%v", agentNames)
	}

	// Use a couple of component symbols so the imports are
	// non-vacuous; the compiler enforces referenceability.
	_ = component.NewBeginComponent
}
