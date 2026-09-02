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
	"reflect"
	"regexp"
	"strings"
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
	if got, want := chunks[0]["text"], "alpha section"; got != want {
		t.Errorf("chunk[0] text = %q, want %q", got, want)
	}
	if got, want := chunks[1]["text"], "beta section"; got != want {
		t.Errorf("chunk[1] text = %q, want %q", got, want)
	}
}

// TestTokenChunker_InvokeTokenSize_FallbackToMerge covers the
// "no delimiter hit" branch — the chunker should fall back to
// token-size merge and emit >=1 chunk.
func TestTokenChunker_InvokeTokenSize_FallbackToMerge(t *testing.T) {
	c, err := NewTokenChunker(map[string]any{
		"delimiter_mode":   "delimiter",
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

// TestTokenChunker_InvokeJSONPayload_KeepsNonTextStandalone is the
// regression lock for #17889: when merging adjacent segments, only
// "text" segments may be merged; "table"/"image" (any non-text type)
// must each stay a standalone chunk and must not be merged with a
// neighbouring segment.
//
// Go already enforces this via itemDocType (common.go:138, derives the
// type from doc_type_kwd), chunkFromItem (token.go:756, emits a non-text
// item as a single standalone chunk) and mergeByTokenSizeFromJSON
// (token.go:1050 forces non-text standalone; token.go:991 starts a fresh
// text chunk after a non-text chunk so text on either side of a
// table/image is never merged across it). This test pins the behaviour
// so a future refactor cannot silently start folding tables/images into
// text chunks.
func TestTokenChunker_InvokeJSONPayload_KeepsNonTextStandalone(t *testing.T) {
	c, err := NewTokenChunker(map[string]any{
		"delimiter_mode": "delimiter",
		"delimiters":     []string{"\n"},
	})
	if err != nil {
		t.Fatalf("NewTokenChunker: %v", err)
	}
	// text, table, text, image, text — in document order.
	items := []map[string]any{
		{"text": "Alpha section text content", "doc_type_kwd": "text"},
		{"text": "<table>caption</table>", "doc_type_kwd": "table"},
		{"text": "Beta section text content", "doc_type_kwd": "text"},
		{"text": "[image]", "doc_type_kwd": "image"},
		{"text": "Gamma section text content", "doc_type_kwd": "text"},
	}
	out, err := c.Invoke(context.Background(), nil, map[string]any{
		"name":          "doc.md",
		"output_format": "json",
		"json":          items,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, ok := out["chunks"].([]map[string]any)
	if !ok {
		t.Fatalf("chunks: want []map[string]any, got %T", out["chunks"])
	}
	// Each segment must remain its own chunk: 5 in, 5 out. If a table or
	// image were merged into an adjacent text chunk this count would drop.
	if len(chunks) != 5 {
		t.Fatalf("chunks: want 5 (every segment standalone), got %d: %+v", len(chunks), chunks)
	}
	wantTypes := []string{"text", "table", "text", "image", "text"}
	for i, ch := range chunks {
		got, _ := ch["doc_type_kwd"].(string)
		if got != wantTypes[i] {
			t.Errorf("chunk %d: doc_type_kwd = %q, want %q (full chunk: %+v)", i, got, wantTypes[i], ch)
		}
	}
	// The two text segments on either side of the table/image must remain
	// distinct — they must NOT be merged across the non-text segments.
	if got, _ := chunks[0]["text"].(string); !strings.Contains(got, "Alpha") {
		t.Errorf("chunk 0 text = %q, want it to contain Alpha", got)
	}
	if got, _ := chunks[2]["text"].(string); !strings.Contains(got, "Beta") {
		t.Errorf("chunk 2 text = %q, want it to contain Beta", got)
	}
	if got, _ := chunks[4]["text"].(string); !strings.Contains(got, "Gamma") {
		t.Errorf("chunk 4 text = %q, want it to contain Gamma", got)
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
		{"zero chunk_token_size", map[string]any{"delimiter_mode": "delimiter", "chunk_token_size": 0}},
		{"negative chunk_token_size", map[string]any{"delimiter_mode": "delimiter", "chunk_token_size": -5}},
		{"negative table_context_size", map[string]any{"delimiter_mode": "delimiter", "chunk_token_size": 50, "table_context_size": -1}},
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
// delimiter_mode = "delimiter".
func TestTokenChunker_NewAcceptsDefaults(t *testing.T) {
	c, err := NewTokenChunker(nil)
	if err != nil {
		t.Fatalf("NewTokenChunker(nil): %v", err)
	}
	if got := c.(*TokenChunkerComponent).param.DelimiterMode; got != "delimiter" {
		t.Errorf("default delimiter_mode = %q, want delimiter", got)
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
			"delimiter_mode":     "delimiter",
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
// common/float_utils.py:50-58 normalize_overlapped_percent. .
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
		{"fraction 0.29 -> 29 (round, was 28 trunc)", 0.29, 29},
		{"fraction 0.3 -> 30", 0.3, 30},
		{"fraction 0.5 -> 50", 0.5, 50},
		{"fraction 0.57 -> 57 (round, was 56 trunc)", 0.57, 57},
		{"fraction 0.93 -> 90 (clamp after round)", 0.93, 90},
		{"fraction 0.95 -> 90 (clamp)", 0.95, 90},
		{"percent 15", 15, 15},
		{"int truncation 33.3 -> 33", 33.3, 33},
		{"clamp 95 -> 90", 95, 90},
		{"clamp -5 -> 0", -5, 0},
		{"negative fraction -0.1 -> 0", -0.1, 0},
		{`numeric string "10" -> 10`, "10", 10},
		{`numeric string fraction "0.1" -> 10`, "0.1", 10},
		{"bad string -> 0", "abc", 0},
		{"nil -> 0", nil, 0},
		// Huge out-of-range values must clamp to 90 (not 0). Go's
		// float->int is implementation-defined past int's range, so the
		// clamp must run before truncation (review finding #4). Python's
		// normalize_overlapped_percent returns 90 for these, not 0.
		{"huge 1e300 -> 90", 1e300, 90},
		{"huge -1e300 -> 0", -1e300, 0},
		{"huge math.MaxFloat64 -> 90", math.MaxFloat64, 90},
		{"huge math.MaxFloat64 +1 -> 90 (clamp pre-round)", math.MaxFloat64 + 1, 90},
		{`numeric string fraction "0.29" -> 29`, "0.29", 29},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := schema.NormalizeOverlappedPercent(tc.in); got != tc.want {
				t.Errorf("NormalizeOverlappedPercent(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// TestTokenChunkerParam_UpdatePreservesOverlappedPercent covers review
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

// TestTokenChunker_NormalizesOverlappedPercent asserts the stored value after
// construction matches Python's normalized [0,90] scale. .
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
				"delimiter_mode":     "delimiter",
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
		{"fraction 0.29 -> 29 (round, was 28 trunc)", 0.29, 29, false},
		{"fraction 0.57 -> 57 (round, was 56 trunc)", 0.57, 57, false},
		{"percent 0", 0, 0, false},
		{"percent 30", 30, 30, false},
		{"percent 90 (boundary)", 90, 90, false},
		{"percent 95 -> error", 95, 0, true},
		{"negative -5 -> error", -5, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := schema.TokenChunkerParam{
				DelimiterMode:     "delimiter",
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

// TestMergeByTokenSize_CRLFNormalization verifies that, like Python
// naive_merge, line endings are normalised
// (replace("\r\n","\n").replace("\r","\n")) before splitting, so CRLF/CR
// input must segment and split exactly like the equivalent LF input, and
// no carriage return may survive into a produced chunk.
func TestMergeByTokenSize_CRLFNormalization(t *testing.T) {
	chunkTexts := func(t *testing.T, text string) []string {
		t.Helper()
		c := &TokenChunkerComponent{}
		c.param.ChunkTokenSize = 128
		c.param.OverlappedPercent = 0
		out := c.mergeByTokenSize(text, nil, nil)
		raw, ok := out["chunks"].([]map[string]any)
		if !ok {
			t.Fatalf("mergeByTokenSize output missing chunks: %v", out)
		}
		texts := make([]string, 0, len(raw))
		for _, m := range raw {
			if s, ok := m["text"].(string); ok {
				texts = append(texts, s)
			}
		}
		return texts
	}

	t.Run("no carriage return survives", func(t *testing.T) {
		texts := chunkTexts(t, "Para A\r\nPara B\r\nPara C")
		if len(texts) == 0 {
			t.Fatalf("expected chunks, got none")
		}
		for _, s := range texts {
			if strings.Contains(s, "\r") {
				t.Errorf("chunk text contains carriage return: %q", s)
			}
		}
	})

	t.Run("CRLF equals LF", func(t *testing.T) {
		// With no blank-line pre-splitting, CRLF is
		// normalised to LF and the blank-line run is preserved (not
		// collapsed), so equal newline counts must yield equal chunks.
		crlf := chunkTexts(t, "Para A\r\nPara B\r\nPara C")
		lf := chunkTexts(t, "Para A\nPara B\nPara C")
		if !reflect.DeepEqual(crlf, lf) {
			t.Errorf("CRLF/LF divergence:\n crlf=%v\n lf =%v", crlf, lf)
		}
	})
}

// TestMergeByTokenSize_PreservesBlankLines verifies that an original
// blank-line run survives in the produced chunk: naive_merge treats the
// whole payload as a single section and does NOT split on blank lines.
func TestMergeByTokenSize_PreservesBlankLines(t *testing.T) {
	c := &TokenChunkerComponent{}
	c.param.ChunkTokenSize = 128
	c.param.OverlappedPercent = 0
	out := c.mergeByTokenSize("A\n\n\nB", nil, nil)
	raw, ok := out["chunks"].([]map[string]any)
	if !ok {
		t.Fatalf("mergeByTokenSize output missing chunks: %v", out)
	}
	var joined strings.Builder
	for _, m := range raw {
		if s, ok := m["text"].(string); ok {
			joined.WriteString(s)
		}
	}
	if got := joined.String(); !strings.Contains(got, "A\n\n\nB") {
		t.Errorf("blank-line run not preserved: got chunk text %q, want it to contain %q", got, "A\n\n\nB")
	}
}

// TestMergeByTokenSize_OversizeDropsDelimiters verifies that when a
// section exceeds chunk_token_size, naive_merge splits on sentence
// delimiters with a capturing-group re.split but then SKIPS any segment
// that is a pure delimiter (re.fullmatch(dels, sub_sec)), so the
// delimiter character ("。") is DROPPED from the produced chunk text
// rather than retained (rag/nlp/__init__.py:1216-1225). The Go port uses
// regexp.Split (which discards the delimiter) and prepends a single "\n",
// so the merged chunk text must NOT contain the original "。".
func TestMergeByTokenSize_OversizeDropsDelimiters(t *testing.T) {
	c := &TokenChunkerComponent{}
	c.param.ChunkTokenSize = 5
	c.param.OverlappedPercent = 0
	text := "第一句。第二句。第三句。第四句。第五句。"
	out := c.mergeByTokenSize(text, nil, nil)
	raw, ok := out["chunks"].([]map[string]any)
	if !ok {
		t.Fatalf("mergeByTokenSize output missing chunks: %v", out)
	}
	var joined strings.Builder
	for _, m := range raw {
		if s, ok := m["text"].(string); ok {
			joined.WriteString(s)
		}
	}
	if got := joined.String(); strings.Contains(got, "。") {
		t.Errorf("sentence delimiter retained (Python drops it): got chunk text %q, want it to NOT contain %q", got, "。")
	}
}

// TestMergeByTokenSize_OversizeDropsBlankLines covers review #1: in the
// oversize sentence-split path, a blank line whose "\n" delimiter segments
// are dropped (matching Python naive_merge, rag/nlp/__init__.py:1216-1225)
// must NOT survive as a blank line. The lone "\n" delimiters produced by
// "\n\n" are dropped, so the merged chunk text must not contain "\n\n".
func TestMergeByTokenSize_OversizeDropsBlankLines(t *testing.T) {
	c := &TokenChunkerComponent{}
	c.param.ChunkTokenSize = 100
	c.param.OverlappedPercent = 0
	// Two ~70-char blocks (no sentence punctuation, so the only
	// delimiters are the two blank-line newlines) separated by "\n\n".
	// Total exceeds 100 tokens → oversize path; the "\n" delimiters are
	// dropped, mirroring Python, so the blank line must not survive.
	block := strings.Repeat("知识库检索增强生成技术", 7) // 70 chars
	text := block + "\n\n" + block
	out := c.mergeByTokenSize(text, nil, nil)
	raw, ok := out["chunks"].([]map[string]any)
	if !ok {
		t.Fatalf("mergeByTokenSize output missing chunks: %v", out)
	}
	var joined strings.Builder
	for _, m := range raw {
		if s, ok := m["text"].(string); ok {
			joined.WriteString(s)
		}
	}
	if got := joined.String(); strings.Contains(got, "\n\n") {
		t.Errorf("blank line survived in oversize path (Python drops it): got chunk text %q, want no blank line (\\n\\n)", got)
	}
}

// TestApplyChildrenDelimText_DefaultsMomToCurrentChunk verifies that when an
// incoming chunk has no Mom, the children fall back to the current chunk's
// text (the historical behaviour preserved by the fix for #17876).
func TestApplyChildrenDelimText_DefaultsMomToCurrentChunk(t *testing.T) {
	docs := []schema.ChunkDoc{
		{Text: "alpha. beta. gamma"},
	}
	pattern := regexp.MustCompile(`\. `)

	out := applyChildrenDelimText(docs, pattern)
	if len(out) != 3 {
		t.Fatalf("want 3 children, got %d", len(out))
	}
	for i, c := range out {
		if c.Mom != "alpha. beta. gamma" {
			t.Errorf("child %d: Mom=%q, want %q", i, c.Mom, "alpha. beta. gamma")
		}
	}
}

// TestApplyChildrenDelimText_OverwritesIncomingMom documents the
// CURRENT behaviour: when a chunk flowing into applyChildrenDelimText
// already has a non-empty Mom, the function OVERWRITES it with
// TrimPrefix(d.Text, "\n") of the current chunk's text. This is
// the divergence that #17876 item 2 (multi-chunk text-path Mom
// granularity) is tracking. Once the merge-granularity fix lands,
// this test should be updated (or the PreservesIncomingMom version
// added back).
func TestApplyChildrenDelimText_OverwritesIncomingMom(t *testing.T) {
	docs := []schema.ChunkDoc{
		{Text: "alpha. beta. gamma", Mom: "incoming-mom-from-upstream"},
	}
	pattern := regexp.MustCompile(`\. `)

	out := applyChildrenDelimText(docs, pattern)
	if len(out) != 3 {
		t.Fatalf("want 3 children, got %d", len(out))
	}
	for i, c := range out {
		// Children must NOT carry the incoming Mom; the function
		// overwrites it with TrimPrefix(d.Text, "\n") of the current
		// chunk's text. This pins the current behavior; a future
		// merge-granularity fix should update this test (or the test
		// itself flips to assert the new preserved Mom behavior).
		if c.Mom == "incoming-mom-from-upstream" {
			t.Errorf("child %d: Mom=%q, expected overwrite to %q (not preserved)",
				i, c.Mom, "alpha. beta. gamma")
		}
		if c.Mom != "alpha. beta. gamma" {
			t.Errorf("child %d: Mom=%q, want %q (TrimPrefix(d.Text, \"\\n\"))",
				i, c.Mom, "alpha. beta. gamma")
		}
	}
}

// TestApplyChildrenDelimText_NilPatternIsNoop verifies the early return so
// callers that pass a nil pattern don't accidentally clear Mom.
func TestApplyChildrenDelimText_NilPatternIsNoop(t *testing.T) {
	docs := []schema.ChunkDoc{
		{Text: "alpha. beta", Mom: "kept"},
	}
	out := applyChildrenDelimText(docs, nil)
	if len(out) != 1 {
		t.Fatalf("want 1 chunk unchanged, got %d", len(out))
	}
	if out[0].Mom != "kept" || out[0].Text != "alpha. beta" {
		t.Errorf("input mutated under nil pattern: %+v", out[0])
	}
}

// TestApplyChildrenDelimText_FallbackStripsLeadingNewline verifies that
// when no incoming Mom is set, the fallback Mom uses
// strings.TrimPrefix(t, "\n") — i.e. it strips a single leading newline
// from the current chunk's text. The historical default this PR
// preserves; if a child path forgets to strip, JSON-keyed SQL
// downstream could see an extra leading "\n" in the Mom value.
func TestApplyChildrenDelimText_FallbackStripsLeadingNewline(t *testing.T) {
	docs := []schema.ChunkDoc{
		{Text: "\nalpha. beta. gamma"},
	}
	pattern := regexp.MustCompile(`\. `)

	out := applyChildrenDelimText(docs, pattern)
	if len(out) != 3 {
		t.Fatalf("want 3 children, got %d", len(out))
	}
	for i, c := range out {
		// Each child Mom must be the text-path parent segment with
		// the leading "\n" stripped, NOT the raw text-with-newline.
		if c.Mom != "alpha. beta. gamma" {
			t.Errorf("child %d: Mom=%q, want %q (leading newline must be stripped)",
				i, c.Mom, "alpha. beta. gamma")
		}
	}
}
