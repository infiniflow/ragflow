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

// Slice 3 tests for port-rag-flow-pipeline-to-go.md Phase 5.
//
// Pins the Tokenizer content_with_weight fallback and the
// Extractor prompt placeholder substitution.

package component

import (
	"context"
	"strings"
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

// TestSubstitutePromptPlaceholders_ReplacesAtChunks pins the
// resume-template pattern `{TitleChunker:FlatMiceFix@chunks}`.
// The substitute is the joined chunk text.
func TestSubstitutePromptPlaceholders_ReplacesAtChunks(t *testing.T) {
	prompt := "Extract metadata from: {TitleChunker:FlatMiceFix@chunks}"
	chunks := []map[string]any{
		{"text": "First chunk."},
		{"text": "Second chunk."},
	}
	got := substitutePromptPlaceholders(prompt, chunks)
	if strings.Contains(got, "{TitleChunker:FlatMiceFix@chunks}") {
		t.Errorf("placeholder not substituted: %q", got)
	}
	if !strings.Contains(got, "First chunk.") || !strings.Contains(got, "Second chunk.") {
		t.Errorf("substitute missing chunk content: %q", got)
	}
}

// TestSubstitutePromptPlaceholders_LeavesPatternWhenNoChunks pins
// the opt-in substitution rule. When chunks is empty the
// placeholder is left intact so a misconfigured template surfaces
// as a clear pattern rather than silently disappearing.
func TestSubstitutePromptPlaceholders_LeavesPatternWhenNoChunks(t *testing.T) {
	prompt := "Extract metadata from: {TitleChunker:FlatMiceFix@chunks}"
	got := substitutePromptPlaceholders(prompt, nil)
	if got != prompt {
		t.Errorf("empty chunks: placeholder should be preserved\n  got: %q\n want: %q", got, prompt)
	}
}

// TestSubstitutePromptPlaceholders_NoPlaceholderInPrompt pins the
// no-op behaviour when the prompt carries no @chunks pattern.
func TestSubstitutePromptPlaceholders_NoPlaceholderInPrompt(t *testing.T) {
	prompt := "Plain prompt with no substitution."
	chunks := []map[string]any{{"text": "x"}}
	got := substitutePromptPlaceholders(prompt, chunks)
	if got != prompt {
		t.Errorf("no-placeholder prompt should be unchanged\n  got: %q\n want: %q", got, prompt)
	}
}

// TestSubstitutePromptPlaceholders_SkipsEmptyChunkText pins the
// per-chunk text trim. A chunk with no text field does not
// contribute a trailing blank line.
func TestSubstitutePromptPlaceholders_SkipsEmptyChunkText(t *testing.T) {
	prompt := "p {TitleChunker:FlatMiceFix@chunks} q"
	chunks := []map[string]any{
		{"text": ""},
		{"text": "actual"},
		{},
	}
	got := substitutePromptPlaceholders(prompt, chunks)
	if strings.Contains(got, "{TitleChunker:FlatMiceFix@chunks}") {
		t.Errorf("placeholder not substituted: %q", got)
	}
	if !strings.Contains(got, "actual") {
		t.Errorf("chunk text missing: %q", got)
	}
}
