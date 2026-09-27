// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package chunker

import (
	"context"
	"strings"
	"testing"
)

// TestTokenChunker_OversizeUnitSplit pins the hard-cap contract (方案 B) for
// the text/markdown path: a single paragraph that exceeds chunk_token_size is
// re-split into <= budget pieces (sentence boundaries first, hard token-split
// fallback), never kept whole. An unbroken input line with no delimiter forces
// the hard-split path while isolating it from the delimiter-splitting logic.
func TestTokenChunker_OversizeUnitSplit(t *testing.T) {
	var longLine = strings.Repeat("word ", 400) // ~400 tokens, far above the 32 budget

	cases := []struct {
		name  string
		conf  map[string]any
		input map[string]any
	}{
		{
			name: "text path",
			conf: map[string]any{"chunk_token_size": 32, "delimiters": []string{"\n"}},
			input: map[string]any{
				"name": "t", "output_format": "text", "text": longLine,
			},
		},
		{
			name: "markdown path",
			conf: map[string]any{"chunk_token_size": 32, "delimiters": []string{"\n"}},
			input: map[string]any{
				"name": "t", "output_format": "markdown", "markdown": longLine,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := NewTokenChunker(tc.conf)
			if err != nil {
				t.Fatalf("NewTokenChunker: %v", err)
			}
			out, err := c.Invoke(context.Background(), nil, tc.input)
			if err != nil {
				t.Fatalf("Invoke: %v", err)
			}
			chunks, ok := out["chunks"].([]map[string]any)
			if !ok {
				t.Fatalf("chunks missing or wrong type: %T", out["chunks"])
			}
			if len(chunks) < 2 {
				t.Fatalf("oversize unit must be split, got %d chunk(s)", len(chunks))
			}
			for i, ck := range chunks {
				text, _ := ck["text"].(string)
				if n := tokenizeStr(text); n > 32 {
					t.Errorf("chunk %d exceeds budget: tokens=%d (cap=32)", i, n)
				}
			}
			// Lossless modulo whitespace normalization: each emitted chunk is
			// trimmed, and hard-split pieces cut at token/word boundaries, so
			// the concatenated words reproduce the original word sequence.
			var joined string
			for _, ck := range chunks {
				joined += strings.TrimSpace(ck["text"].(string))
			}
			if strings.ReplaceAll(joined, " ", "") != strings.ReplaceAll(longLine, " ", "") {
				t.Errorf("split content lost words:\n got %q\nwant %q", joined, longLine)
			}
		})
	}
}

// TestTokenChunker_OversizeUnitSplitAfterInBudgetUnit pins the hard-cap
// contract under the merge: an in-budget unit is followed by an oversized
// unit, which is re-split into <= budget pieces. The in-budget unit stays a
// chunk of its own; the oversized unit becomes multiple chunks that together
// reproduce its text.
func TestTokenChunker_OversizeUnitSplitAfterInBudgetUnit(t *testing.T) {
	var longLine = strings.Repeat("word ", 400) // ~400 tokens, far above the 32 budget
	inBudget := "Hello world."                  // ASCII period is not a sentence delimiter; fits 32

	cases := []struct {
		name  string
		conf  map[string]any
		input map[string]any
	}{
		{
			name: "text path",
			conf: map[string]any{"chunk_token_size": 32, "delimiters": []string{"\n"}},
			input: map[string]any{
				"name": "t", "output_format": "text",
				"text": inBudget + "\n" + longLine,
			},
		},
		{
			name: "markdown path",
			conf: map[string]any{"chunk_token_size": 32, "delimiters": []string{"\n"}},
			input: map[string]any{
				"name": "t", "output_format": "markdown",
				"markdown": inBudget + "\n" + longLine,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := NewTokenChunker(tc.conf)
			if err != nil {
				t.Fatalf("NewTokenChunker: %v", err)
			}
			out, err := c.Invoke(context.Background(), nil, tc.input)
			if err != nil {
				t.Fatalf("Invoke: %v", err)
			}
			chunks, ok := out["chunks"].([]map[string]any)
			if !ok {
				t.Fatalf("chunks missing or wrong type: %T", out["chunks"])
			}
			if len(chunks) < 3 {
				t.Fatalf("want in-budget chunk + split pieces, got %d chunk(s)", len(chunks))
			}
			first, _ := chunks[0]["text"].(string)
			if first != inBudget {
				t.Fatalf("first chunk should be only the in-budget sentence %q, got %q", inBudget, first)
			}
			for i := 1; i < len(chunks); i++ {
				text, _ := chunks[i]["text"].(string)
				if n := tokenizeStr(text); n > 32 {
					t.Errorf("split chunk %d exceeds budget: tokens=%d (cap=32)", i, n)
				}
			}
		})
	}
}
