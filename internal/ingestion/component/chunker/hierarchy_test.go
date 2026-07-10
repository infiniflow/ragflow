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
	"strings"
	"testing"

	"ragflow/internal/agent/runtime"
)

func TestHierarchyTitleChunker_Registered(t *testing.T) {
	factory, cat, meta, ok := runtime.DefaultRegistry.Lookup("HierarchyTitleChunker")
	if !ok {
		t.Fatal("HierarchyTitleChunker: registry miss")
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

func TestHierarchyTitleChunker_Parallelism(t *testing.T) {
	c, err := NewHierarchyTitleChunker(map[string]any{
		"hierarchy": 1,
		"levels":    [][]string{{`^# `}, {`^## `}},
	})
	if err != nil {
		t.Fatalf("NewHierarchyTitleChunker: %v", err)
	}
	hc, ok := c.(*HierarchyTitleChunkerComponent)
	if !ok {
		t.Fatalf("NewHierarchyTitleChunker returned %T", c)
	}
	if got := hc.Parallelism(); got != 2 {
		t.Errorf("Parallelism() = %d, want 2", got)
	}
}

func TestHierarchyTitleChunker_InputsOutputs_NonEmpty(t *testing.T) {
	_, _, meta, ok := runtime.DefaultRegistry.Lookup("HierarchyTitleChunker")
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

func TestHierarchyTitleChunker_NewRejectsMissingHierarchy(t *testing.T) {
	if _, err := NewHierarchyTitleChunker(map[string]any{
		"levels": [][]string{{`^# `}},
	}); err == nil {
		t.Fatal("expected error for missing hierarchy, got nil")
	}
}

func TestHierarchyTitleChunker_NewRejectsBadHierarchy(t *testing.T) {
	if _, err := NewHierarchyTitleChunker(map[string]any{
		"hierarchy": 0,
		"levels":    [][]string{{`^# `}},
	}); err == nil {
		t.Fatal("expected error for hierarchy=0, got nil")
	}
}

func TestHierarchyTitleChunker_InvokeEmptyInput(t *testing.T) {
	c, err := NewHierarchyTitleChunker(map[string]any{
		"hierarchy": 1,
		"levels":    [][]string{{`^# `}},
	})
	if err != nil {
		t.Fatalf("NewHierarchyTitleChunker: %v", err)
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

func TestHierarchyTitleChunker_NestedHeadings(t *testing.T) {
	c, err := NewHierarchyTitleChunker(map[string]any{
		"hierarchy":               2,
		"levels":                  [][]string{{`^# `}, {`^## `}, {`^### `}},
		"include_heading_content": true,
	})
	if err != nil {
		t.Fatalf("NewHierarchyTitleChunker: %v", err)
	}
	input := "# H1\nbody1a\nbody1b\n## H2a\nbody2a1\nbody2a2\n## H2b\nbody2b1\n# H1-2\nbody-last"
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
	for i, ck := range chunks {
		if text, _ := ck["text"].(string); text == "" {
			t.Errorf("chunk[%d] text is empty", i)
		}
	}
}

func TestHierarchyTitleChunker_LeafOnlyDefault(t *testing.T) {
	// include_heading_content = false (default): every emitted
	// chunk path should be a leaf-only path (no body content under
	// non-leaf headings surfaces).
	c, err := NewHierarchyTitleChunker(map[string]any{
		"hierarchy": 2,
		"levels":    [][]string{{`^# `}, {`^## `}},
	})
	if err != nil {
		t.Fatalf("NewHierarchyTitleChunker: %v", err)
	}
	_, err = c.Invoke(context.Background(), map[string]any{
		"name": "doc.md",
		"text": "# A\nbody a\n## A1\nbody a1\n# B\nbody b",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
}

// TestHierarchyChunker_StructuredMetadata pins Gap E for the hierarchy
// strategy: a structured (output_format=chunks) image record keeps its
// doc_type_kwd and img_id on the emitted chunk.
func TestHierarchyChunker_StructuredMetadata(t *testing.T) {
	c, err := NewHierarchyTitleChunker(map[string]any{
		"hierarchy": 1,
		"levels":    [][]string{{`^# `}},
	})
	if err != nil {
		t.Fatalf("NewHierarchyTitleChunker: %v", err)
	}
	items := []map[string]any{
		{"text": "# H1", "doc_type_kwd": "text"},
		{"text": "body one", "doc_type_kwd": "text"},
		{"text": "imgA caption", "doc_type_kwd": "image", "img_id": "a"},
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
			if ck["img_id"] != "a" {
				t.Errorf("image chunk img_id = %v, want a", ck["img_id"])
			}
		}
	}
	if !found {
		t.Fatal("no image chunk emitted")
	}
}

func TestHierarchyTitleChunker_InvokeDeterministic(t *testing.T) {
	c, err := NewHierarchyTitleChunker(map[string]any{
		"hierarchy": 2,
		"levels":    [][]string{{`^# `}, {`^## `}},
	})
	if err != nil {
		t.Fatalf("NewHierarchyTitleChunker: %v", err)
	}
	inputs := map[string]any{
		"name": "doc.md",
		"text": "# A\nbody a\n## A1\nbody a1\n## A2\nbody a2\n# B\nbody b",
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

// TestHierarchyTitleChunker_ConsecutiveNonTextOrder pins Gap G: two
// consecutive non-text records must stay in input order and ahead of
// the following text run. Python's flush_text_records flushes the
// preceding text run on the first non-text and is a no-op for the next
// non-text, yielding [..., N1, N2, run, ...]; the old loop flushed the
// trailing run early, yielding [..., N1, run, N2].
func TestHierarchyTitleChunker_ConsecutiveNonTextOrder(t *testing.T) {
	c, err := NewHierarchyTitleChunker(map[string]any{
		"hierarchy": 1,
		"levels":    [][]string{{`^# `}},
	})
	if err != nil {
		t.Fatalf("NewHierarchyTitleChunker: %v", err)
	}
	items := []map[string]any{
		{"text": "# H1", "doc_type_kwd": "text"},
		{"text": "body one", "doc_type_kwd": "text"},
		{"text": "imgA caption", "doc_type_kwd": "image", "img_id": "a"},
		{"text": "imgB caption", "doc_type_kwd": "image", "img_id": "b"},
		{"text": "# H2", "doc_type_kwd": "text"},
		{"text": "body two", "doc_type_kwd": "text"},
	}
	out, err := c.Invoke(context.Background(), map[string]any{
		"name":   "doc",
		"chunks": items,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	idx := func(sub string) int {
		for i, ck := range chunks {
			if text, _ := ck["text"].(string); strings.Contains(text, sub) {
				return i
			}
		}
		return -1
	}
	iA := idx("imgA caption")
	iB := idx("imgB caption")
	iBody := idx("body two")
	if iA < 0 || iB < 0 || iBody < 0 {
		t.Fatalf("missing expected chunks: imgA=%d imgB=%d body=%d (chunks=%d)", iA, iB, iBody, len(chunks))
	}
	if !(iA < iB && iB < iBody) {
		t.Errorf("non-text order wrong: imgA=%d imgB=%d body=%d, want imgA<imgB<body", iA, iB, iBody)
	}
}
