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
