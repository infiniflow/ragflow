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

package chunker

import (
	"context"
	"testing"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/schema"
)

func TestGroupTitleChunker_Registered(t *testing.T) {
	factory, cat, meta, ok := runtime.DefaultRegistry.Lookup("GroupTitleChunker")
	if !ok {
		t.Fatal("GroupTitleChunker: registry miss")
	}
	if cat != runtime.CategoryIngestion {
		t.Errorf("category = %q, want %q", cat, runtime.CategoryIngestion)
	}
	if factory == nil {
		t.Error("factory is nil")
	}
	if len(meta.Inputs) == 0 {
		t.Errorf("inputs metadata is empty")
	}
	if len(meta.Outputs) == 0 {
		t.Errorf("outputs metadata is empty")
	}
}

func TestGroupTitleChunker_Parallelism(t *testing.T) {
	c, err := NewGroupTitleChunker(map[string]any{
		"levels": [][]string{{`^# `}},
	})
	if err != nil {
		t.Fatalf("NewGroupTitleChunker: %v", err)
	}
	gc, ok := c.(*GroupTitleChunkerComponent)
	if !ok {
		t.Fatalf("NewGroupTitleChunker returned %T", c)
	}
	if got := gc.Parallelism(); got != 2 {
		t.Errorf("Parallelism() = %d, want 2", got)
	}
}

func TestGroupTitleChunker_InputsOutputs_NonEmpty(t *testing.T) {
	_, _, meta, ok := runtime.DefaultRegistry.Lookup("GroupTitleChunker")
	if !ok {
		t.Fatal("registry miss")
	}
	if len(meta.Inputs) == 0 {
		t.Error("inputs metadata is empty")
	}
	if len(meta.Outputs) == 0 {
		t.Error("outputs metadata is empty")
	}
}

func TestGroupTitleChunker_NewRejectsHierarchyWithoutHierarchyParam(t *testing.T) {
	// Although method defaults to "group", the levels branch still
	// fires and rejects an empty levels config.
	if _, err := NewGroupTitleChunker(map[string]any{}); err == nil {
		t.Fatal("expected error for empty levels, got nil")
	}
}

func TestGroupTitleChunker_InvokeEmptyInput(t *testing.T) {
	c, err := NewGroupTitleChunker(map[string]any{
		"levels": [][]string{{`^# `}},
	})
	if err != nil {
		t.Fatalf("NewGroupTitleChunker: %v", err)
	}
	out, err := c.Invoke(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := out["output_format"], "chunks"; got != want {
		t.Errorf("output_format = %v, want %v", got, want)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 0 {
		t.Errorf("chunks = %d, want 0", len(chunks))
	}
}

func TestGroupTitleChunker_Headings_ASCII(t *testing.T) {
	c, err := NewGroupTitleChunker(map[string]any{
		"levels": [][]string{
			{`^# `},
			{`^## `},
		},
	})
	if err != nil {
		t.Fatalf("NewGroupTitleChunker: %v", err)
	}
	input := "# Heading One\nBody line under H1.\nAnother body line.\n# Heading Two\nBody under second H1."
	out, err := c.Invoke(context.Background(), map[string]any{
		"name": "doc.md",
		"text": input,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) == 0 {
		t.Fatal("chunks: want >=1, got 0")
	}
}

func TestGroupTitleChunker_RootChunkAsHeading_StillSingleGroup(t *testing.T) {
	// When the input is single-group, the root-as-heading branch
	// doesn't reduce the count below 1.
	c, err := NewGroupTitleChunker(map[string]any{
		"levels":                [][]string{{`^# `}},
		"root_chunk_as_heading": true,
	})
	if err != nil {
		t.Fatalf("NewGroupTitleChunker: %v", err)
	}
	out, err := c.Invoke(context.Background(), map[string]any{
		"name": "doc.md",
		"text": "Body without any heading here.\nMore body.",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) == 0 {
		t.Fatal("chunks: want >=1, got 0")
	}
}

// TestResolveGroupTargetLevel_UsesMostLevelDirectly pins Gap F: when
// `hierarchy` is unset the group target level is `most_level` DIRECTLY,
// not resolve_target_level (which would re-rank the distinct heading
// levels and pick the wrong depth when levels are not contiguous from
// 1). With levels {2,2,3} most_level is 2, but resolve_target_level
// would return 3.
func TestResolveGroupTargetLevel_UsesMostLevelDirectly(t *testing.T) {
	levels := []int{2, 2, 3, bodyLevel, bodyLevel}
	pUnset := &titleChunkerParam{TitleChunkerParam: schema.TitleChunkerParam{Method: "group"}}
	if got := resolveGroupTargetLevel(levels, pUnset, 2); got != 2 {
		t.Errorf("unset hierarchy: got %d, want 2 (most_level directly)", got)
	}
	h := 2
	pSet := &titleChunkerParam{TitleChunkerParam: schema.TitleChunkerParam{Method: "group", Hierarchy: &h}}
	if got := resolveGroupTargetLevel(levels, pSet, 2); got != 3 {
		t.Errorf("hierarchy=2: got %d, want 3 (resolve_target_level)", got)
	}
}

// TestGroupChunker_StructuredMetadata pins Gap E: for a structured
// (output_format=chunks) payload, non-text records keep their
// doc_type_kwd and img_id on the emitted chunk.
func TestGroupChunker_StructuredMetadata(t *testing.T) {
	c, err := NewGroupTitleChunker(map[string]any{
		"levels": [][]string{{`^# `}},
	})
	if err != nil {
		t.Fatalf("NewGroupTitleChunker: %v", err)
	}
	items := []map[string]any{
		{"text": "# Heading", "doc_type_kwd": "text"},
		{"text": "body line", "doc_type_kwd": "text"},
		{"text": "an image caption", "doc_type_kwd": "image", "img_id": "img-9"},
	}
	out, err := c.Invoke(context.Background(), map[string]any{
		"name":          "doc",
		"output_format": "chunks",
		"chunks":        items,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	found := false
	for _, ck := range chunks {
		if dt, _ := ck["doc_type_kwd"].(string); dt == "image" {
			found = true
			if ck["img_id"] != "img-9" {
				t.Errorf("image chunk img_id = %v, want img-9", ck["img_id"])
			}
		}
	}
	if !found {
		t.Fatal("no image chunk emitted")
	}
}

func TestGroupTitleChunker_InvokeDeterministic(t *testing.T) {
	c, err := NewGroupTitleChunker(map[string]any{
		"levels": [][]string{
			{`^# `},
			{`^## `},
		},
	})
	if err != nil {
		t.Fatalf("NewGroupTitleChunker: %v", err)
	}
	inputs := map[string]any{
		"name": "doc.md",
		"text": "# A\nbody a1\nbody a2\n# B\nbody b1\nbody b2",
	}
	var firstLen int
	var firstTexts []string
	for run := 0; run < 10; run++ {
		out, err := c.Invoke(context.Background(), inputs)
		if err != nil {
			t.Fatalf("Invoke run %d: %v", run, err)
		}
		chunks, _ := out["chunks"].([]map[string]any)
		texts := make([]string, len(chunks))
		for i, ck := range chunks {
			texts[i], _ = ck["text"].(string)
		}
		if run == 0 {
			firstLen = len(chunks)
			firstTexts = texts
			continue
		}
		if firstLen != len(chunks) {
			t.Fatalf("run %d: chunk count changed (%d vs %d)", run, len(chunks), firstLen)
		}
		for i := range chunks {
			if firstTexts[i] != texts[i] {
				t.Fatalf("run %d: chunk[%d] text changed", run, i)
			}
		}
	}
}
