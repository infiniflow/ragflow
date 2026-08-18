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
// Pins the Extractor prompt placeholder substitution.

package component

import (
	"strings"
	"testing"
)

// TestRenderExtractorPrompts_ReplacesAtChunks pins the
// `{ComponentName:ParamName@chunks}` placeholder substitution.
func TestRenderExtractorPrompts_ReplacesAtChunks(t *testing.T) {
	prompt := "Extract metadata from: {TitleChunker:FlatMiceFix@chunks}"
	ck := map[string]any{"text": "First chunk."}
	_, got := renderExtractorPrompts("", prompt, ck, "First chunk.")
	if strings.Contains(got, "{TitleChunker:FlatMiceFix@chunks}") {
		t.Errorf("placeholder not substituted: %q", got)
	}
	if !strings.Contains(got, "First chunk.") {
		t.Errorf("substitute missing chunk content: %q", got)
	}
	if n := strings.Count(got, "First chunk."); n != 1 {
		t.Errorf("chunk text appears %d times, want 1: %q", n, got)
	}
}

// TestRenderExtractorPrompts_LeavesUnknownPattern pins that unrecognized
// placeholders are preserved verbatim.
func TestRenderExtractorPrompts_LeavesUnknownPattern(t *testing.T) {
	prompt := "Extract metadata from: {unknown_placeholder}"
	_, got := renderExtractorPrompts("", prompt, nil, "")
	if got != prompt {
		t.Errorf("unknown placeholder: pattern should be preserved\n  got: %q\n want: %q", got, prompt)
	}
}

// TestRenderExtractorPrompts_NoPlaceholderAppendsChunkText pins the
// fallback append behavior when the prompt carries no body placeholder.
func TestRenderExtractorPrompts_NoPlaceholderAppendsChunkText(t *testing.T) {
	prompt := "Plain prompt with no substitution."
	ck := map[string]any{"text": "my chunk text"}
	_, got := renderExtractorPrompts("", prompt, ck, "my chunk text")
	want := "Plain prompt with no substitution.\n\nmy chunk text"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestRenderExtractorPrompts_SkipsEmptyChunkText pins that an empty
// chunkText does not append a trailing blank line.
func TestRenderExtractorPrompts_SkipsEmptyChunkText(t *testing.T) {
	prompt := "Plain prompt"
	_, got := renderExtractorPrompts("", prompt, map[string]any{}, "")
	if got != prompt {
		t.Errorf("empty chunkText should not alter prompt\n  got: %q\n want: %q", got, prompt)
	}
}
