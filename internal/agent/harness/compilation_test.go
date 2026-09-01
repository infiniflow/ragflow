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

package harness

import (
	"testing"

	"ragflow/internal/entity"
)

// TestNormalizeCompilationKind asserts pageindex/page_index collapse to timeline
// and '-'→'_' normalization.
func TestNormalizeCompilationKind(t *testing.T) {
	cases := map[string]string{
		"pageindex":       "timeline",
		"page_index":      "timeline",
		"PAGE_INDEX":      "timeline",
		"mind_map":        "mind_map",
		"Mind-Map":        "mind_map",
		"knowledge_graph": "knowledge_graph",
		"tree":            "tree",
	}
	for in, want := range cases {
		if got := normalizeCompilationKind(in); got != want {
			t.Errorf("normalizeCompilationKind(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestAddCompilationKind asserts each template kind maps onto the canonical token.
func TestAddCompilationKind(t *testing.T) {
	comps := map[string]bool{}
	addCompilationKind(comps, "knowledge_graph")
	addCompilationKind(comps, "mind_map")
	addCompilationKind(comps, "timeline")
	addCompilationKind(comps, "wiki")
	addCompilationKind(comps, "tree")
	for _, want := range []string{"knowledge_graph", "mindmap", "page_index", "wiki", "tree"} {
		if !comps[want] {
			t.Errorf("missing %q in comps %v", want, comps)
		}
	}
	if comps["mind_map"] {
		t.Error("mind_map must be normalized to mindmap, not kept verbatim")
	}
}

// TestParserConfigTemplateGroupIDs asserts single-string, list, and ext-nested
// forms all resolve, with dedup and empty-drop.
func TestParserConfigTemplateGroupIDs(t *testing.T) {
	// top-level single string
	pc := entity.JSONMap{"compilation_template_group_id": "g1"}
	if got := parserConfigTemplateGroupIDs(pc); len(got) != 1 || got[0] != "g1" {
		t.Errorf("single string: got %v", got)
	}
	// top-level list with dup + empty
	pc = entity.JSONMap{"compilation_template_group_id": []interface{}{"g1", "g1", "", "g2"}}
	if got := parserConfigTemplateGroupIDs(pc); len(got) != 2 {
		t.Errorf("list dedup: got %v", got)
	}
	// ext-nested
	pc = entity.JSONMap{"ext": map[string]interface{}{"compilation_template_group_id": []interface{}{"g3"}}}
	if got := parserConfigTemplateGroupIDs(pc); len(got) != 1 || got[0] != "g3" {
		t.Errorf("ext-nested: got %v", got)
	}
	// absent → nil
	if got := parserConfigTemplateGroupIDs(entity.JSONMap{}); got != nil {
		t.Errorf("absent: got %v, want nil", got)
	}
}

// TestBoolVal asserts bool/string/float forms.
func TestBoolVal(t *testing.T) {
	if !boolVal(entity.JSONMap{"x": true}, "x") {
		t.Error("bool true must be true")
	}
	if !boolVal(entity.JSONMap{"x": "true"}, "x") {
		t.Error(`string "true" must be true`)
	}
	if boolVal(entity.JSONMap{"x": "false"}, "x") {
		t.Error(`string "false" must be false`)
	}
	if boolVal(entity.JSONMap{"x": 1.0}, "x") != true {
		t.Error("float 1.0 must be true")
	}
	if boolVal(entity.JSONMap{}, "x") {
		t.Error("absent key must be false")
	}
}
