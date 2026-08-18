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

// TestRenderExtractorSystemPrompt_ReplacesAtChunks pins the
// `{ComponentName:ParamName@chunks}` placeholder substitution in system prompt.
func TestRenderExtractorSystemPrompt_ReplacesAtChunks(t *testing.T) {
	prompt := "Extract metadata from: {TitleChunker:FlatMiceFix@chunks}"
	ck := map[string]any{"text": "First chunk."}
	got := renderExtractorSystemPrompt(prompt, ck, "First chunk.")
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

// TestRenderExtractorSystemPrompt_LeavesUnknownPattern pins that unrecognized
// placeholders are preserved verbatim.
func TestRenderExtractorSystemPrompt_LeavesUnknownPattern(t *testing.T) {
	prompt := "Extract metadata from: {unknown_placeholder}"
	got := renderExtractorSystemPrompt(prompt, nil, "")
	if got != prompt {
		t.Errorf("unknown placeholder: pattern should be preserved\n  got: %q\n want: %q", got, prompt)
	}
}
