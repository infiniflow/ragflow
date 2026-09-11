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

package models

import (
	"slices"
	"testing"
)

// ParseListModel feeds the provider-model list endpoint whose merge treats
// remote entries as authoritative: every returned entry must carry non-empty
// model types. Catalog hits are normalized; catalog misses fall back to
// name-based inference (defaulting to chat), mirroring Python's
// OpenAIAPICompatible._format_model_list.
func TestParseListModelAssignsModelTypes(t *testing.T) {
	dir, restore := setupProviderTestDir(t, "aliyun.json")
	defer restore()

	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	got := ParseListModel(ModelList{Models: []ModelListItem{
		{ID: "qwen3-vl-plus"},             // catalog hit: chat+vision+ocr -> chat+vision
		{ID: "qwen-vl-plus"},              // catalog hit: chat+vision
		{ID: "qwen3-vl-flash-2099-01-01"}, // miss: future dated variant -> inferred
		{ID: "gpt-4o-2099-01-01"},         // miss -> inferred
		{ID: "text-embedding-v4"},         // miss -> inferred
		{ID: "qwen3-reranker-2099"},       // miss -> inferred
		{ID: "some-unknown-model"},        // miss -> default chat
		{ID: " qwen-vl-max-2099-01-01 "},  // trimmed, miss -> inferred
		{ID: " "},                         // skipped
	}})

	want := map[string][]string{
		"qwen3-vl-plus":             {"chat", "vision"},
		"qwen-vl-plus":              {"chat", "vision"},
		"qwen3-vl-flash-2099-01-01": {"chat", "vision"},
		"gpt-4o-2099-01-01":         {"chat", "vision"},
		"text-embedding-v4":         {"embedding"},
		"qwen3-reranker-2099":       {"rerank"},
		"some-unknown-model":        {"chat"},
		"qwen-vl-max-2099-01-01":    {"chat", "vision"},
	}

	if len(got) != len(want) {
		t.Fatalf("ParseListModel returned %d entries, want %d (blank IDs skipped)", len(got), len(want))
	}

	for _, m := range got {
		wantTypes, ok := want[m.Name]
		if !ok {
			t.Fatalf("unexpected entry %q in result", m.Name)
		}
		if !slices.Equal(m.ModelTypes, wantTypes) {
			t.Errorf("model %q: ModelTypes = %v, want %v", m.Name, m.ModelTypes, wantTypes)
		}
	}

	// Catalog metadata still flows through on a hit (qwen3-vl-plus declares
	// max_output 8192 in conf/all_models.json).
	for _, m := range got {
		if m.Name == "qwen3-vl-plus" {
			if m.MaxOutput == nil || *m.MaxOutput != 8192 {
				t.Errorf("qwen3-vl-plus: MaxOutput = %v, want 8192", m.MaxOutput)
			}
		}
	}
}

func TestParseListModelPrefersRemoteContextLength(t *testing.T) {
	dir, restore := setupProviderTestDir(t, "moonshot.json")
	defer restore()

	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	remoteContextLength := 123456
	got := ParseListModel(ModelList{Models: []ModelListItem{
		{ID: "kimi-k2.6", ContextLength: &remoteContextLength},
	}})
	if len(got) != 1 {
		t.Fatalf("ParseListModel returned %d entries, want 1", len(got))
	}
	if got[0].ContextLength == nil || *got[0].ContextLength != remoteContextLength {
		t.Fatalf("ContextLength = %v, want remote %d", got[0].ContextLength, remoteContextLength)
	}
}
