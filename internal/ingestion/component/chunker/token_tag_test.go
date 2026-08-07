//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
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
	"strings"
	"testing"
)

// TestTokenChunker_TextPath_StripsParserTags pins residual B: the
// TokenChunker text path must not leak parser position tags (@@...##)
// into the final chunk text. The input mimics a PDF parser payload where
// the body text still carries coordinate markers.
func TestTokenChunker_TextPath_StripsParserTags(t *testing.T) {
	c, err := NewTokenChunker(map[string]any{
		"delimiter_mode":   "token_size",
		"chunk_token_size": 1000,
		"delimiters":       []string{"\n"},
	})
	if err != nil {
		t.Fatalf("NewTokenChunker: %v", err)
	}
	out, err := c.Invoke(context.Background(), nil, map[string]any{
		"name":          "doc.txt",
		"output_format": "text",
		"text":          "First line with tag@@123## inside.\nSecond line with tag@@456## inside.",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) == 0 {
		t.Fatal("chunks: want >=1, got 0")
	}
	for i, ck := range chunks {
		txt, _ := ck["text"].(string)
		if strings.Contains(txt, "@@") || strings.Contains(txt, "##") {
			t.Errorf("chunk %d text leaks parser tag: %q", i, txt)
		}
	}
}

// TestTokenChunker_JSONPath_StripsParserTags pins residual B for the
// structured (output_format == "json") path.
func TestTokenChunker_JSONPath_StripsParserTags(t *testing.T) {
	c, err := NewTokenChunker(map[string]any{
		"delimiter_mode": "delimiter",
		"delimiters":     []string{"\n"},
	})
	if err != nil {
		t.Fatalf("NewTokenChunker: %v", err)
	}
	items := []map[string]any{
		{"text": "Alpha@@1## text\nBeta@@2## text", "doc_type_kwd": "text"},
		{"text": "Gamma@@3## text\nDelta@@4## text", "doc_type_kwd": "text"},
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
	for i, ck := range chunks {
		txt, _ := ck["text"].(string)
		if strings.Contains(txt, "@@") || strings.Contains(txt, "##") {
			t.Errorf("chunk %d text leaks parser tag: %q", i, txt)
		}
	}
}
