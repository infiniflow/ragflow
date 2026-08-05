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
// rag/nlp/__init__.py) never atom-splits an oversize unit; the previous Go
// behaviour called splitOversizedUnit and emitted Go-only sub-chunks, which
// diverged from the Python reference (parity cases token__text_long_paragraph
// and token__markdown_long). An unbroken input line with no delimiter forces
// the oversize path while isolating it from the delimiter-splitting logic.
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
		})
	}
}
