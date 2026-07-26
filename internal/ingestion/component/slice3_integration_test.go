//go:build integration
// +build integration

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

// Slice 3 integration tests (require tokenizer pool).
package component

import (
	"context"
	"testing"
)

// TestTokenizer_FallsBackToContentWithWeight pins the python
// rag/flow/tokenizer.py:111 fallback. A chunk with only
// content_with_weight (no text) must tokenize the fallback text.
func TestTokenizer_FallsBackToContentWithWeight(t *testing.T) {
	requireTokenizerPool(t)
	c := &TokenizerComponent{}
	c.param.SearchMethod = []string{"full_text"}
	c.param.Fields = []string{"text"}
	c.param.FilenameEmbdWeight = 0

	out, err := c.Invoke(context.Background(), map[string]any{
		"name":          "doc",
		"output_format": "chunks",
		"chunks": []map[string]any{
			{"content_with_weight": "fallback text", "doc_type_kwd": "text"},
		},
	})
	if err != nil {
		t.Fatalf("Tokenizer.Invoke: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) == 0 {
		t.Fatal("no chunks emitted")
	}
	if got := chunks[0]["content_ltks"]; got == nil {
		t.Errorf("content_ltks missing; content_with_weight fallback did not run")
	} else if s, ok := got.(string); !ok || s == "" {
		t.Errorf("content_ltks = %v (type %T), want non-empty string", got, got)
	}
}

// TestTokenizer_DoesNotChangeChunkText pins the regression guard
// for the fallback. When both "text" and "content_with_weight"
// are present, "text" wins.
func TestTokenizer_DoesNotChangeChunkText(t *testing.T) {
	requireTokenizerPool(t)
	c := &TokenizerComponent{}
	c.param.SearchMethod = []string{"full_text"}
	c.param.Fields = []string{"text"}
	c.param.FilenameEmbdWeight = 0

	out, err := c.Invoke(context.Background(), map[string]any{
		"name":          "doc",
		"output_format": "chunks",
		"chunks": []map[string]any{
			{"text": "primary text", "content_with_weight": "fallback", "doc_type_kwd": "text"},
		},
	})
	if err != nil {
		t.Fatalf("Tokenizer.Invoke: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) == 0 {
		t.Fatal("no chunks emitted")
	}
	if got, want := chunks[0]["text"], "primary text"; got != want {
		t.Errorf("text = %v, want %v (text should win over content_with_weight)", got, want)
	}
}
