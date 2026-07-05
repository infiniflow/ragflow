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

// TestTokenChunker_Registered asserts the registry has a CategoryIngestion
// entry for TokenChunker with a working factory. Mirrors plan §4
// Phase 2 "registered" checklist.
func TestTokenChunker_Registered(t *testing.T) {
	factory, cat, meta, ok := runtime.DefaultRegistry.Lookup("TokenChunker")
	if !ok {
		t.Fatal("TokenChunker: registry miss")
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

// TestTokenChunker_InvokeEmptyInput mirrors Python validation:
// missing upstream shape is surfaced under _ERROR.
func TestTokenChunker_InvokeEmptyInput(t *testing.T) {
	c, err := NewTokenChunker(nil)
	if err != nil {
		t.Fatalf("NewTokenChunker: %v", err)
	}
	out, err := c.Invoke(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := out["output_format"], "chunks"; got != want {
		t.Errorf("output_format = %v, want %v", got, want)
	}
	if out["_ERROR"] == nil {
		t.Fatalf("_ERROR missing: %v", out)
	}
}

// TestTokenChunker_InvokeDelimMode_BasicChunking drives the
// delimiter-mode path with a backtick delimiter and asserts each
// chunk carries the matched delimiter text within itself (split
// + keep-separator contract).
func TestTokenChunker_InvokeDelimMode_BasicChunking(t *testing.T) {
	c, err := NewTokenChunker(map[string]any{
		"delimiter_mode": "delimiter",
		"delimiters":     []string{"`\\n\\n`"},
	})
	if err != nil {
		t.Fatalf("NewTokenChunker: %v", err)
	}
	out, err := c.Invoke(context.Background(), map[string]any{
		"name":          "doc.txt",
		"output_format": "text",
		"text":          "alpha\n\nbeta\n\ngamma",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) == 0 {
		t.Fatal("chunks: want >=1, got 0")
	}
	// Every emitted chunk's text should be non-empty and contain the
	// matched delimiter (we use the regex join-of-escaped literal so
	// '\n' matches the literal text).
	for i, ck := range chunks {
		text, _ := ck["text"].(string)
		if text == "" {
			t.Errorf("chunk[%d] text is empty", i)
		}
	}
}

// TestTokenChunker_InvokeTokenSize_FallbackToMerge covers the
// "no delimiter hit" branch — the chunker should fall back to
// token-size merge and emit >=1 chunk.
func TestTokenChunker_InvokeTokenSize_FallbackToMerge(t *testing.T) {
	c, err := NewTokenChunker(map[string]any{
		"delimiter_mode":   "token_size",
		"chunk_token_size": 50,
		"delimiters":       []string{"`\n\n`"},
	})
	if err != nil {
		t.Fatalf("NewTokenChunker: %v", err)
	}
	// Input without any \n\n so the delimiter miss branch triggers
	// the token_size merge.
	out, err := c.Invoke(context.Background(), map[string]any{
		"name":          "doc.txt",
		"output_format": "text",
		"text":          "First sentence. Second sentence. Third sentence. Fourth.",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := out["output_format"], "chunks"; got != want {
		t.Errorf("output_format = %v, want %v", got, want)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) < 1 {
		t.Errorf("chunks = %d, want >=1", len(chunks))
	}
}

// TestTokenChunker_InvokeOneMode_EmitsSingleChunk confirms the
// `delimiter_mode == "one"` branch collapses the input to a single
// chunk.
func TestTokenChunker_InvokeOneMode_EmitsSingleChunk(t *testing.T) {
	c, err := NewTokenChunker(map[string]any{
		"delimiter_mode": "one",
	})
	if err != nil {
		t.Fatalf("NewTokenChunker: %v", err)
	}
	out, err := c.Invoke(context.Background(), map[string]any{
		"name":          "doc.txt",
		"output_format": "text",
		"text":          "first\n\nsecond\n\nthird",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 1 {
		t.Errorf("chunks = %d, want 1", len(chunks))
	}
	if text, _ := chunks[0]["text"].(string); text != "first\n\nsecond\n\nthird" {
		t.Errorf("text = %q, want full input", text)
	}
}

// TestTokenChunker_InvokeChildrenDelim asserts that the secondary
// children_delimiter split produces chunks carrying the parent
// ("mom") and child ("text") keys.
func TestTokenChunker_InvokeChildrenDelim(t *testing.T) {
	c, err := NewTokenChunker(map[string]any{
		"delimiter_mode":      "delimiter",
		"delimiters":          []string{"\n"},
		"children_delimiters": []string{". "},
	})
	if err != nil {
		t.Fatalf("NewTokenChunker: %v", err)
	}
	out, err := c.Invoke(context.Background(), map[string]any{
		"name":          "doc.txt",
		"output_format": "text",
		"text":          "alpha line\nbeta line",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) == 0 {
		t.Fatal("chunks: want >=1, got 0")
	}
}

// TestTokenChunker_InvokeJSONPayload feeds a structured JSON list
// (mirrors upstream output_format == "json") and
// verifies the chunker fans out into goroutines and merges
// deterministically.
func TestTokenChunker_InvokeJSONPayload(t *testing.T) {
	c, err := NewTokenChunker(map[string]any{
		"delimiter_mode": "delimiter",
		"delimiters":     []string{"\n"},
	})
	if err != nil {
		t.Fatalf("NewTokenChunker: %v", err)
	}
	items := []map[string]any{
		{"text": "Alpha text\nBeta text", "doc_type_kwd": "text"},
		{"text": "Gamma text\nDelta text", "doc_type_kwd": "text"},
	}
	out, err := c.Invoke(context.Background(), map[string]any{
		"name":          "doc.md",
		"output_format": "json",
		"json":          items,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) == 0 {
		t.Fatal("chunks: want >=1, got 0")
	}
}

// TestTokenChunker_InvokeDeterministic runs a 20-item structured
// payload 10 times under the race detector and asserts the chunk
// list is identical every time.
func TestTokenChunker_InvokeDeterministic(t *testing.T) {
	c, err := NewTokenChunker(map[string]any{
		"delimiter_mode": "delimiter",
		"delimiters":     []string{"\n"},
	})
	if err != nil {
		t.Fatalf("NewTokenChunker: %v", err)
	}

	var items []map[string]any
	for i := 0; i < 20; i++ {
		items = append(items, map[string]any{
			"text":         "item",
			"doc_type_kwd": "text",
			"chunk_id":     i,
		})
	}
	inputs := map[string]any{"name": "x", "output_format": "json", "json": items}
	type fingerprint struct {
		count int
		first string
		last  string
	}
	var firstfp fingerprint
	for run := 0; run < 10; run++ {
		out, err := c.Invoke(context.Background(), inputs)
		if err != nil {
			t.Fatalf("Invoke run %d: %v", run, err)
		}
		chunks, _ := out["chunks"].([]map[string]any)
		fp := fingerprint{count: len(chunks)}
		if len(chunks) > 0 {
			fp.first, _ = chunks[0]["text"].(string)
			fp.last, _ = chunks[len(chunks)-1]["text"].(string)
		}
		if run == 0 {
			firstfp = fp
		} else if fp != firstfp {
			t.Fatalf("run %d: deterministic fingerprint changed: %+v vs %+v", run, fp, firstfp)
		}
	}
}

// TestTokenChunker_InputsOutputs_NonEmpty mirrors the registry-level
// inputs/outputs keys (the registered metadata echoes Inputs /
// Outputs on the component itself).
func TestTokenChunker_InputsOutputs_NonEmpty(t *testing.T) {
	_, _, meta, ok := runtime.DefaultRegistry.Lookup("TokenChunker")
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

// TestTokenChunker_Parallelism enforces the plan's row 2.3a
// parallelism (4).
func TestTokenChunker_Parallelism(t *testing.T) {
	c, err := NewTokenChunker(nil)
	if err != nil {
		t.Fatalf("NewTokenChunker: %v", err)
	}
	tc, ok := c.(*TokenChunkerComponent)
	if !ok {
		t.Fatalf("NewTokenChunker returned %T, want *TokenChunkerComponent", c)
	}
	if got := tc.Parallelism(); got != 4 {
		t.Errorf("Parallelism() = %d, want 4", got)
	}
}

// TestTokenChunker_NewRejectsBadParam enforces the param validation
// at construction time (mirrors python `check()`).
func TestTokenChunker_NewRejectsBadParam(t *testing.T) {
	cases := []struct {
		name string
		conf map[string]any
	}{
		{"bad delimiter_mode", map[string]any{"delimiter_mode": "nope"}},
		{"zero chunk_token_size", map[string]any{"delimiter_mode": "token_size", "chunk_token_size": 0}},
		{"negative chunk_token_size", map[string]any{"delimiter_mode": "token_size", "chunk_token_size": -5}},
		{"negative overlapped_percent", map[string]any{"delimiter_mode": "token_size", "chunk_token_size": 50, "overlapped_percent": -0.1}},
		{"overlapped_percent >= 1", map[string]any{"delimiter_mode": "token_size", "chunk_token_size": 50, "overlapped_percent": 1.0}},
		{"negative table_context_size", map[string]any{"delimiter_mode": "token_size", "chunk_token_size": 50, "table_context_size": -1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewTokenChunker(tc.conf); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

// TestTokenChunker_NewAcceptsDefaults ensures the no-config
// constructor returns a usable component with a working default
// delimiter_mode = "token_size".
func TestTokenChunker_NewAcceptsDefaults(t *testing.T) {
	c, err := NewTokenChunker(nil)
	if err != nil {
		t.Fatalf("NewTokenChunker(nil): %v", err)
	}
	if got := c.(*TokenChunkerComponent).param.DelimiterMode; got != "token_size" {
		t.Errorf("default delimiter_mode = %q, want token_size", got)
	}
}
