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

package models

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// TestHandleStreamingResponseBareRootFinishReason verifies the shared handler
// terminates the stream when an event carries only a root-level finish_reason
// with no choices and no trailing [DONE]. Before the fix this event was
// skipped by the choices guard and the handler returned "stream ended before
// [DONE] or finish_reason".
func TestHandleStreamingResponseBareRootFinishReason(t *testing.T) {
	sse := "data: {\"finish_reason\":\"stop\"}\n"

	var contentChunks, reasonChunks []string
	err := HandleStreamingResponse(
		io.NopCloser(bytes.NewBufferString(sse)),
		nil,
		nil,
		OpenAIParserConfig,
		func(content *string, reason *string) error {
			if content != nil && *content != "" {
				contentChunks = append(contentChunks, *content)
			}
			if reason != nil && *reason != "" {
				reasonChunks = append(reasonChunks, *reason)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("HandleStreamingResponse: %v", err)
	}

	// The terminal [DONE] marker is the only content the handler emits for a
	// finish_reason-only event: no tokens arrived and no reasoning was sent.
	if got := strings.Join(contentChunks, ""); got != "[DONE]" {
		t.Errorf("content=%q, want only the [DONE] marker", got)
	}
	if len(reasonChunks) != 0 {
		t.Errorf("reasoning=%q, want none for a finish_reason-only event", strings.Join(reasonChunks, ""))
	}
}

// TestHandleStreamingResponseTruncatedStream verifies a stream that ends
// without [DONE], a root-level finish_reason, or a per-choice finish_reason
// is rejected as truncated. This is the counterpart of the bare
// finish_reason case: only a genuine truncation must fail.
func TestHandleStreamingResponseTruncatedStream(t *testing.T) {
	sse := "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n"

	err := HandleStreamingResponse(
		io.NopCloser(bytes.NewBufferString(sse)),
		nil,
		nil,
		OpenAIParserConfig,
		func(*string, *string) error { return nil },
	)
	if err == nil || !strings.Contains(err.Error(), "stream ended before [DONE] or finish_reason") {
		t.Fatalf("expected truncation error, got %v", err)
	}
}

// TestExtractDefaultStreamReasoning covers the reasoning field names accepted
// by the default OpenAI-compatible stream extractor.
func TestExtractDefaultStreamReasoning(t *testing.T) {
	cases := []struct {
		name  string
		delta map[string]any
		want  string
	}{
		{"reasoning_content only", map[string]any{"reasoning_content": "a"}, "a"},
		{"reasoning only", map[string]any{"reasoning": "b"}, "b"},
		{"both prefers reasoning_content", map[string]any{"reasoning_content": "a", "reasoning": "b"}, "a"},
		{"empty reasoning_content falls back", map[string]any{"reasoning_content": "", "reasoning": "b"}, "b"},
		{"non-string reasoning ignored", map[string]any{"reasoning": map[string]any{"effort": "high"}}, ""},
		{"neither", map[string]any{"content": "c"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractDefaultStreamReasoning(tc.delta); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestHandleStreamingResponseReasoningField verifies reasoning streamed under
// delta.reasoning (instead of delta.reasoning_content) reaches the sender.
func TestHandleStreamingResponseReasoningField(t *testing.T) {
	sse := "data: {\"choices\":[{\"delta\":{\"reasoning\":\"think\"}}]}\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"answer\"},\"finish_reason\":\"stop\"}]}\n"

	var contentChunks, reasonChunks []string
	err := HandleStreamingResponse(
		io.NopCloser(bytes.NewBufferString(sse)),
		nil,
		nil,
		OpenAIParserConfig,
		func(content *string, reason *string) error {
			if content != nil {
				contentChunks = append(contentChunks, *content)
			}
			if reason != nil {
				reasonChunks = append(reasonChunks, *reason)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.Join(reasonChunks, ""); got != "think" {
		t.Errorf("reasoning = %q, want %q", got, "think")
	}
	if got := strings.Join(contentChunks, ""); !strings.Contains(got, "answer") {
		t.Errorf("content = %q, want it to contain %q", got, "answer")
	}
}
