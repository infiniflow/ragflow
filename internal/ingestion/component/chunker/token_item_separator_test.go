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

package chunker

import (
	"context"
	"ragflow/internal/ingestion/component/schema"
	"testing"
)

func TestTokenChunkerSeparatesAdjacentJSONItems(t *testing.T) {
	items := []string{
		"The installer requires Python 3.10 or later.",
		"## Configuration",
		"Edit config.yaml before the first start.",
	}
	if got, want := invokeTokenChunkerJSON(t, items...), "The installer requires Python 3.10 or later.\n## Configuration\nEdit config.yaml before the first start."; got != want {
		t.Errorf("chunk text = %q, want %q", got, want)
	}
}

func TestTokenChunkerPreservesExistingJSONItemWhitespace(t *testing.T) {
	tests := []struct {
		name  string
		items []string
		want  string
	}{
		{name: "previous item ends in whitespace", items: []string{"first\n", "second"}, want: "first\nsecond"},
		{name: "next item starts in whitespace", items: []string{"first", " second"}, want: "first second"},
		{name: "delimiter inside one item", items: []string{"first\nsecond"}, want: "first\nsecond"},
		{name: "empty item between text items", items: []string{"first", "", "second"}, want: "first\nsecond"},
		{name: "leading empty item", items: []string{"", "second"}, want: "second"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := invokeTokenChunkerJSON(t, tt.items...); got != tt.want {
				t.Errorf("chunk text = %q, want %q", got, tt.want)
			}
		})
	}
}

func invokeTokenChunkerJSON(t *testing.T, texts ...string) string {
	t.Helper()
	comp, err := NewTokenChunker(map[string]any{"chunk_token_size": float64(512)})
	if err != nil {
		t.Fatalf("construct TokenChunker: %v", err)
	}
	items := make([]map[string]any, len(texts))
	for i, text := range texts {
		items[i] = map[string]any{"text": text, "doc_type_kwd": "text"}
	}
	out, err := comp.Invoke(context.Background(), nil, map[string]any{
		"name":          "readme.md",
		"output_format": "json",
		"json":          items,
	})
	if err != nil {
		t.Fatalf("invoke TokenChunker: %v", err)
	}
	if msg, _ := out["_ERROR"].(string); msg != "" {
		t.Fatalf("TokenChunker returned _ERROR: %s", msg)
	}

	chunks, ok := schema.ChunkCarrierToMaps(out["chunks"])
	if !ok || len(chunks) != 1 {
		t.Fatalf("chunks = %#v, want one merged chunk", out["chunks"])
	}
	text, ok := chunks[0]["text"].(string)
	if !ok {
		t.Fatalf("chunk text = %T, want string", chunks[0]["text"])
	}
	return text
}
