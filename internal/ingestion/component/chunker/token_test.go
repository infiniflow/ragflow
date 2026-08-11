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
	"math"
	"testing"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/schema"
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
	out, err := c.Invoke(context.Background(), nil, map[string]any{})
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
	out, err := c.Invoke(context.Background(), nil, map[string]any{
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

// TestTokenChunker_DelimNeverStandaloneChunk is the regression test for
// the "666" bug: the delimiter must be glued to the end of the preceding
// segment (Python _split_text_by_pattern), never emitted as its own chunk.
func TestTokenChunker_DelimNeverStandaloneChunk(t *testing.T) {
	c, err := NewTokenChunker(map[string]any{
		"delimiter_mode": "delimiter",
		"delimiters":     []string{"`666`"},
	})
	if err != nil {
		t.Fatalf("NewTokenChunker: %v", err)
	}
	out, err := c.Invoke(context.Background(), nil, map[string]any{
		"name":          "doc.txt",
		"output_format": "text",
		"text":          "alpha section\n666\nbeta section",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 2 {
		t.Fatalf("chunks = %d, want 2: %v", len(chunks), chunks)
	}
	for i, ck := range chunks {
		text, _ := ck["text"].(string)
		if text == "666" || text == "\n666" || text == "666\n" {
			t.Errorf("chunk[%d] is the bare delimiter %q", i, text)
		}
	}
	if got, want := chunks[0]["text"], "alpha section\n666"; got != want {
		t.Errorf("chunk[0] text = %q, want %q", got, want)
	}
	if got, want := chunks[1]["text"], "\nbeta section"; got != want {
		t.Errorf("chunk[1] text = %q, want %q", got, want)
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
	out, err := c.Invoke(context.Background(), nil, map[string]any{
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
	out, err := c.Invoke(context.Background(), nil, map[string]any{
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
	out, err := c.Invoke(context.Background(), nil, map[string]any{
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
		out, err := c.Invoke(context.Background(), nil, inputs)
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

// TestTokenChunker_NewRejectsBadParam enforces the param validation
// at construction time (mirrors python `check()`).
func TestTokenChunker_NewRejectsBadParam(t *testing.T) {
	cases := []struct {
		name string
		conf map[string]any
	}{
		{"bad delimiter_mode", map[string]any{"delimiter_mode": "nope"}},
		{"one delimiter_mode (use OneChunker)", map[string]any{"delimiter_mode": "one"}},
		{"zero chunk_token_size", map[string]any{"delimiter_mode": "token_size", "chunk_token_size": 0}},
		{"negative chunk_token_size", map[string]any{"delimiter_mode": "token_size", "chunk_token_size": -5}},
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

// TestTokenChunker_PrefersUpstreamChunks is the Go port of the Python
// regression test for #16812 (PR #16825). When a TitleChunker feeds
// this TokenChunker with output_format == "chunks" AND both a "chunks"
// list and a raw "json" list on the wire, the TokenChunker must
// consume the upstream chunks (CHAPTER-AWARE) and must NOT fall through
// to the raw parser json_result (RAW-PARSER-JSON).
func TestTokenChunker_PrefersUpstreamChunks(t *testing.T) {
	c, err := NewTokenChunker(map[string]any{
		"delimiter_mode": "delimiter",
		"delimiters":     []string{"\n"},
	})
	if err != nil {
		t.Fatalf("NewTokenChunker: %v", err)
	}
	out, err := c.Invoke(context.Background(), nil, map[string]any{
		"name":          "doc.md",
		"output_format": "chunks",
		"chunks":        []map[string]any{{"text": "CHAPTER-AWARE", "doc_type_kwd": "text"}},
		"json":          []map[string]any{{"text": "RAW-PARSER-JSON", "doc_type_kwd": "text"}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) == 0 {
		t.Fatal("chunks: want >=1, got 0")
	}
	for i, ck := range chunks {
		text, _ := ck["text"].(string)
		if text == "RAW-PARSER-JSON" {
			t.Fatalf("chunk[%d] consumed the raw parser json_result instead of upstream chunks: %q", i, text)
		}
		if text == "CHAPTER-AWARE" {
			return // happy path: upstream chunk preserved
		}
	}
	t.Fatalf("upstream chunk 'CHAPTER-AWARE' was not found in output: %v", out["chunks"])
}

// TestTokenChunker_NewAcceptsPythonOverlappedRange covers Chunker-2.6:
// overlapped_percent uses Python's [0,90] integer-percentage semantics,
// accepting both a [0,1) fraction and a [0,90] percentage (the latter
// normalized via normalizeOverlappedPercent).
func TestTokenChunker_NewAcceptsPythonOverlappedRange(t *testing.T) {
	// Values that should be valid in the Python range (fractions and
	// percentages, including out-of-range inputs that Python clamps).
	for _, pct := range []float64{0, 0.1, 0.5, 15, 30, 50, 90, 95, -5} {
		conf := map[string]any{
			"delimiter_mode":     "token_size",
			"chunk_token_size":   100,
			"overlapped_percent": pct,
		}
		_, err := NewTokenChunker(conf)
		if err != nil {
			t.Errorf("overlapped_percent=%v: unexpected error: %v", pct, err)
		}
	}
}

// TestNormalizeOverlappedPercent is the Go port of Python
// common/float_utils.py:50-58 normalize_overlapped_percent (diff Chunker-2.6).
// Python's user-facing input is a [0,1) fraction; the helper converts it to
// the [0,90] integer-percentage scale the merge math expects.
func TestNormalizeOverlappedPercent(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want float64
	}{
		{"zero", 0, 0},
		{"fraction 0.1 -> 10", 0.1, 10},
		{"fraction 0.5 -> 50", 0.5, 50},
		{"fraction 0.95 -> 90 (clamp)", 0.95, 90},
		{"percent 15", 15, 15},
		{"int truncation 33.3 -> 33", 33.3, 33},
		{"clamp 95 -> 90", 95, 90},
		{"clamp -5 -> 0", -5, 0},
		{"negative fraction -0.1 -> 0", -0.1, 0},
		// Huge out-of-range values must clamp to 90 (not 0). Go's
		// float->int is implementation-defined past int's range, so the
		// clamp must run before truncation (review finding #4). Python's
		// normalize_overlapped_percent returns 90 for these, not 0.
		{"huge 1e300 -> 90", 1e300, 90},
		{"huge -1e300 -> 0", -1e300, 0},
		{"huge math.MaxFloat64 -> 90", math.MaxFloat64, 90},
		{`numeric string "10" -> 10`, "10", 10},
		{`numeric string fraction "0.1" -> 10`, "0.1", 10},
		{"bad string -> 0", "abc", 0},
		{"nil -> 0", nil, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := schema.NormalizeOverlappedPercent(tc.in); got != tc.want {
				t.Errorf("NormalizeOverlappedPercent(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// TestTokenChunker_NormalizesOverlappedPercent asserts the stored value after
// construction matches Python's normalized [0,90] scale (diff Chunker-2.6).
func TestTokenChunker_NormalizesOverlappedPercent(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want float64
	}{
		{"clamp 95 -> 90", 95, 90},
		{"clamp -5 -> 0", -5, 0},
		{"fraction 0.1 -> 10", 0.1, 10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := NewTokenChunker(map[string]any{
				"delimiter_mode":     "token_size",
				"chunk_token_size":   100,
				"overlapped_percent": tc.in,
			})
			if err != nil {
				t.Fatalf("NewTokenChunker(%v): %v", tc.in, err)
			}
			got := c.(*TokenChunkerComponent).param.OverlappedPercent
			if got != tc.want {
				t.Errorf("overlapped_percent=%v: stored %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// TestSplitIntoSections locks the CRLF-safe blank-line splitting used by
// mergeByTokenSize. It does NOT claim Chunker-2.7/2.10 coverage: that
// section-splitting semantics is still OPEN in
// docs/migration_python_go_diff.md (flow-vs-app Python alignment target
// undecided), so this only guards the existing behaviour.
func TestSplitIntoSections(t *testing.T) {
	tests := []struct {
		name string
		text string
		want int
	}{
		{"two paragraphs", "a\n\nb", 2},
		{"three paragraphs with excess blank lines", "a\n\n\nb\n\nc", 3},
		{"single paragraph", "hello world", 1},
		// A blank line containing only whitespace is still a boundary
		// under \n\s*\n (this is what reverted the stricter \n{2,}).
		{"blank line with space is a boundary", "a\n \nb", 2},
		{"leading blank lines produce empty prefix (caller filters)", "\n\ntext", 2},
		// CRLF regression guard: \r\n\r\n must split the same as \n\n.
		{"CRLF paragraph break splits", "a\r\n\r\nb", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitIntoSections(tt.text)
			if len(got) != tt.want {
				t.Errorf("splitIntoSections(%q) = %d sections, want %d: %v", tt.text, len(got), tt.want, got)
			}
		})
	}
}

// TestTokenChunkerParam_ValidateOverlappedRange covers the strict overlap
// handling in TokenChunkerParam.Validate (review findings #3 + #4):
// a directly-constructed struct with a [0,1) fraction is scaled to its
// [0,90] percent (so 0.3 means 30%, matching the config path), while
// out-of-range values are rejected — the config path (Update) clamps
// instead, so this is the only guard that catches a bad literal.
func TestTokenChunkerParam_ValidateOverlappedRange(t *testing.T) {
	cases := []struct {
		name    string
		in      float64
		want    float64 // expected stored value after Validate when err==nil
		wantErr bool
	}{
		{"fraction 0.3 -> 30", 0.3, 30, false},
		{"percent 0", 0, 0, false},
		{"percent 30", 30, 30, false},
		{"percent 90 (boundary)", 90, 90, false},
		{"percent 95 -> error", 95, 0, true},
		{"negative -5 -> error", -5, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := schema.TokenChunkerParam{
				DelimiterMode:     "token_size",
				ChunkTokenSize:    100,
				OverlappedPercent: tc.in,
			}
			err := p.Validate()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Validate(overlapped_percent=%v): want error, got nil", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate(overlapped_percent=%v): unexpected error: %v", tc.in, err)
			}
			if p.OverlappedPercent != tc.want {
				t.Errorf("after Validate: overlapped_percent=%v, want %v", p.OverlappedPercent, tc.want)
			}
		})
	}
}

// TestTokenChunkerParam_UpdatePreservesOverlappedPercent locks review
// finding #3: tokenChunkerParam.Update must not reset OverlappedPercent to 0
// when the incoming config omits the key. All other fields use a presence
// guard, and a partial Update (e.g. changing only chunk_token_size) must
// preserve the previously configured overlap instead of clobbering it.
func TestTokenChunkerParam_UpdatePreservesOverlappedPercent(t *testing.T) {
	p := defaultsToken(tokenChunkerParam{})
	p.TokenChunkerParam.OverlappedPercent = 30 // pre-existing config

	// Partial update: only chunk_token_size changes.
	p.Update(map[string]any{"chunk_token_size": 100})

	if p.TokenChunkerParam.OverlappedPercent != 30 {
		t.Errorf("after partial Update: overlapped_percent=%v, want 30 (preserved)",
			p.TokenChunkerParam.OverlappedPercent)
	}

	// Explicit key still wins and normalizes.
	p.Update(map[string]any{"overlapped_percent": 0.5})
	if p.TokenChunkerParam.OverlappedPercent != 50 {
		t.Errorf("after explicit Update: overlapped_percent=%v, want 50 (normalized)",
			p.TokenChunkerParam.OverlappedPercent)
	}
}
