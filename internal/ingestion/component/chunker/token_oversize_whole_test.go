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

// TestTokenChunker_OversizeUnitKeptWhole pins the Python OVER_CAP contract for
// the text/markdown path: a single paragraph that exceeds chunk_token_size is
// kept as one standalone chunk. Python's naive_merge (_merge_paragraph_groups,
// rag/nlp/__init__.py) never atom-splits an oversize unit; it is kept whole and
// the model layer truncates it later. An unbroken input line with no delimiter
// forces the oversize path while isolating it from the delimiter-splitting logic.
func TestTokenChunker_OversizeUnitKeptWhole(t *testing.T) {
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
			if len(chunks) != 1 {
				t.Fatalf("oversize unit: want 1 standalone chunk, got %d", len(chunks))
			}
			if got, _ := chunks[0]["text"].(string); strings.TrimSpace(got) != strings.TrimSpace(longLine) {
				t.Fatalf("oversize unit content not preserved: got %q", got)
			}
		})
	}
}

// TestTokenChunker_OversizeUnitStandsAloneAfterInBudgetUnit exercises the
// mergeDecision oversize branch (incomingTokens > target -> startNewChunk),
// which TestTokenChunker_OversizeUnitKeptWhole never reaches because its lone
// oversize unit goes through the len(cks)==0 path of addChunk. A short
// in-budget sentence precedes the oversize paragraph; after the sentence
// delimiter split the oversize unit must stand alone as its own chunk
// (matching Python OVER_CAP), not be merged into or atom-split across the
// previous chunk.
func TestTokenChunker_OversizeUnitStandsAloneAfterInBudgetUnit(t *testing.T) {
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
			if len(chunks) != 2 {
				t.Fatalf("oversize after in-budget: want 2 chunks (in-budget + standalone oversize), got %d", len(chunks))
			}
			first, _ := chunks[0]["text"].(string)
			if first != inBudget {
				t.Fatalf("first chunk should be only the in-budget sentence %q, got %q", inBudget, first)
			}
			second, _ := chunks[1]["text"].(string)
			if second != strings.TrimSpace(longLine) {
				t.Fatalf("oversize chunk should be the whole long line kept whole, got %q", second)
			}
		})
	}
}
