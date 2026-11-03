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

package agentic_rag

import (
	"sort"
	"testing"
)

// TestCompiledKnowledgeSurfaceIsReachable pins the two ways a compiled-knowledge
// tool can reach the model: declared by a template's tool list, or injected by
// Run when the bound datasets hold compiled products. A tool that is neither is
// registered but dead - and a review that only reads the registry, or only reads
// the template, cannot tell that apart from "the model chose not to use it".
func TestCompiledKnowledgeSurfaceIsReachable(t *testing.T) {
	reg := toolRegistry()
	for _, name := range compiledKnowledgeToolNames {
		if _, ok := reg[name]; !ok {
			t.Errorf("%s is injected but not registered in the tool registry", name)
		}
	}

	// The navigation tools stay REGISTERED (a template may declare them) but are
	// deliberately not injected any more: compiled evidence reaches the model
	// with the ordinary search results, so navigating the structure is a second,
	// costlier path to the same facts. A review that expects them injected would
	// be reading the old contract.
	injected := map[string]bool{}
	for _, name := range compiledKnowledgeToolNames {
		injected[name] = true
	}
	for _, name := range []string{navigateTreeToolName, navigateStructureToolName} {
		if injected[name] {
			t.Errorf("%s must not be injected: compiled evidence travels with search results", name)
		}
		if _, ok := reg[name]; !ok {
			t.Errorf("%s must stay registered so a template can declare it", name)
		}
	}
	for _, name := range []string{graphExploreToolName, graphDocOutlineToolName} {
		if !injected[name] {
			t.Errorf("%s is never injected: a template that omits it leaves it unreachable", name)
		}
	}

	// Build must yield one tool per name, in the same order, with nothing
	// silently dropped by a missing registry entry.
	built := compiledKnowledgeTools("tenant", []string{"kb1"})
	if len(built) != len(compiledKnowledgeToolNames) {
		t.Errorf("compiledKnowledgeTools built %d tools, want %d", len(built), len(compiledKnowledgeToolNames))
	}
}

// TestCompiledKnowledgeNamesAreStable guards the injection list itself: the set
// is part of the runtime contract (what the model is offered on a compiled
// corpus), so losing an entry is a behaviour change, not a refactor.
func TestCompiledKnowledgeNamesAreStable(t *testing.T) {
	got := append([]string(nil), compiledKnowledgeToolNames...)
	sort.Strings(got)
	want := []string{
		graphDocOutlineToolName,
		graphExploreToolName,
	}
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("compiledKnowledgeToolNames = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("compiledKnowledgeToolNames = %v, want %v", got, want)
		}
	}
}
