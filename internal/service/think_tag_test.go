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

package service

import (
	"context"
	"strings"
	"testing"
)

func TestNextThinkDelta_NoThinkTag(t *testing.T) {
	state := &ThinkStreamState{}
	deltas := NextThinkDelta(state, "hello world")
	if len(deltas) != 0 {
		t.Fatalf("expected 0 deltas, got %d", len(deltas))
	}
	if state.buffer != "hello world" {
		t.Errorf("buffer = %q", state.buffer)
	}
}

func TestNextThinkDelta_OnlyThinkTag(t *testing.T) {
	state := &ThinkStreamState{}
	deltas := NextThinkDelta(state, "<think>reasoning</think>visible")
	if len(deltas) != 2 {
		t.Fatalf("expected 2 deltas, got %d: %+v", len(deltas), deltas)
	}
	if deltas[0].Kind != ThinkDeltaMarker || deltas[0].Value != "<think>" {
		t.Errorf("first delta should be <think> marker: %+v", deltas[0])
	}
	if deltas[1].Kind != ThinkDeltaMarker || deltas[1].Value != "</think>" {
		t.Errorf("second delta should be </think> marker: %+v", deltas[1])
	}
	if state.buffer != "visible" {
		t.Errorf("buffer = %q, want visible", state.buffer)
	}
}

func TestNextThinkDelta_TextThenThink(t *testing.T) {
	state := &ThinkStreamState{}
	deltas := NextThinkDelta(state, "before <think>inside</think> after")
	// "before " -> buffer (no flush yet)
	// "<think>" -> marker
	// "inside" -> inside think, consumed silently
	// "</think>" -> marker
	// " after" -> buffer
	if len(deltas) != 2 {
		t.Fatalf("expected 2 markers, got %d: %+v", len(deltas), deltas)
	}
	if state.buffer != "before  after" {
		t.Errorf("buffer = %q", state.buffer)
	}
}

func TestNextThinkDelta_MultipleChunks(t *testing.T) {
	state := &ThinkStreamState{}
	NextThinkDelta(state, "hello ")
	NextThinkDelta(state, "<think>")
	NextThinkDelta(state, "reasoning")
	NextThinkDelta(state, "</think>")
	deltas := NextThinkDelta(state, " world")
	if len(deltas) != 0 {
		t.Fatalf("expected 0 deltas from final chunk, got %d", len(deltas))
	}
	if state.buffer != "hello  world" {
		t.Errorf("buffer = %q", state.buffer)
	}
}

func TestNextThinkDelta_UnclosedThink(t *testing.T) {
	state := &ThinkStreamState{}
	deltas := NextThinkDelta(state, "text <think>unclosed")
	if len(deltas) != 1 {
		t.Fatalf("expected 1 marker (think open), got %d", len(deltas))
	}
	if deltas[0].Value != "<think>" {
		t.Errorf("expected <think> marker")
	}
	if state.buffer != "text " {
		t.Errorf("buffer = %q", state.buffer)
	}
	// "unclosed" should be consumed silently inside think
}

func TestNextThinkDelta_EmptyInput(t *testing.T) {
	state := &ThinkStreamState{}
	deltas := NextThinkDelta(state, "")
	if len(deltas) != 0 {
		t.Errorf("expected 0 deltas for empty input")
	}
}

func TestNextThinkDelta_NilState(t *testing.T) {
	deltas := NextThinkDelta(nil, "test")
	if deltas != nil {
		t.Error("expected nil for nil state")
	}
}

func TestFlushThinkBuffer_Empty(t *testing.T) {
	if deltas := FlushThinkBuffer(nil); len(deltas) != 0 {
		t.Error("expected empty for nil state")
	}
	state := &ThinkStreamState{}
	if deltas := FlushThinkBuffer(state); len(deltas) != 0 {
		t.Error("expected empty for zero state")
	}
}

func TestFlushThinkBuffer_WithContent(t *testing.T) {
	state := &ThinkStreamState{buffer: "flushed text"}
	deltas := FlushThinkBuffer(state)
	if len(deltas) != 1 {
		t.Fatalf("expected 1 delta, got %d", len(deltas))
	}
	if deltas[0].Kind != ThinkDeltaText {
		t.Error("expected text delta")
	}
	if deltas[0].Value != "flushed text" {
		t.Errorf("value = %q", deltas[0].Value)
	}
	if state.buffer != "" {
		t.Error("buffer should be cleared after flush")
	}
}

func TestExtractVisibleAnswer_Plain(t *testing.T) {
	if got := ExtractVisibleAnswer("hello"); got != "hello" {
		t.Errorf("expected hello, got %q", got)
	}
}

func TestExtractVisibleAnswer_Empty(t *testing.T) {
	if got := ExtractVisibleAnswer(""); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestExtractVisibleAnswer_WithThink(t *testing.T) {
	raw := "<think>some reasoning</think>the visible answer"
	if got := ExtractVisibleAnswer(raw); got != "the visible answer" {
		t.Errorf("got %q", got)
	}
}

func TestExtractVisibleAnswer_ThinkOnly(t *testing.T) {
	raw := "<think>only reasoning here</think>"
	if got := ExtractVisibleAnswer(raw); got != "" {
		t.Errorf("expected empty for think-only, got %q", got)
	}
}

func TestExtractVisibleAnswer_MultipleThinks(t *testing.T) {
	raw := "<think>first</think>visible1<think>second</think>visible2"
	if got := ExtractVisibleAnswer(raw); got != "visible1visible2" {
		t.Errorf("got %q", got)
	}
}

func TestExtractVisibleAnswer_NestedTags(t *testing.T) {
	raw := "<think><think>nested</think></think>answer"
	if got := ExtractVisibleAnswer(raw); got != "answer" {
		t.Errorf("got %q", got)
	}
}

func TestStreamThinkTagDelta(t *testing.T) {
	chunks := []string{"hello ", "wor", "<think>", "think text", "</think>", "ld", " final"}
	ch := make(chan string, len(chunks))
	for _, c := range chunks {
		ch <- c
	}
	close(ch)

	var texts []string
	var markers []string
	for d := range StreamThinkTagDelta(context.Background(), ch, 16) {
		switch d.Kind {
		case ThinkDeltaText:
			texts = append(texts, d.Value)
		case ThinkDeltaMarker:
			markers = append(markers, d.Value)
		}
	}

	if len(markers) != 2 {
		t.Errorf("expected 2 markers, got %d: %v", len(markers), markers)
	}
	if markers[0] != "<think>" {
		t.Errorf("first marker = %q", markers[0])
	}
	if markers[1] != "</think>" {
		t.Errorf("second marker = %q", markers[1])
	}

	joined := strings.Join(texts, "")
	if !strings.Contains(joined, "hello world") || !strings.Contains(joined, "final") {
		t.Errorf("texts = %q", texts)
	}
}

func TestStreamThinkTagDelta_IncrementalFlush(t *testing.T) {
	// Verify that visible text is streamed incrementally, not all at the end.
	// minTokens=1 → flushSize=4 bytes.  Each chunk triggers a flush.
	chunks := []string{"1234", "5678", "90ab"}
	ch := make(chan string, len(chunks))
	for _, c := range chunks {
		ch <- c
	}
	close(ch)

	var texts []string
	for d := range StreamThinkTagDelta(context.Background(), ch, 1) {
		if d.Kind == ThinkDeltaText {
			texts = append(texts, d.Value)
		}
	}
	// With minTokens=1 (flushSize=4), each chunk triggers a flush.
	// We should get incremental text deltas, not just one final burst.
	if len(texts) < 2 {
		t.Errorf("expected >=2 incremental text deltas, got %d: %q", len(texts), texts)
	}
}

func TestStreamThinkTagDelta_NoThinkTags(t *testing.T) {
	chunks := []string{"just", " plain", " text"}
	ch := make(chan string, len(chunks))
	for _, c := range chunks {
		ch <- c
	}
	close(ch)

	var texts []string
	for d := range StreamThinkTagDelta(context.Background(), ch, 4) {
		texts = append(texts, d.Value)
	}

	joined := strings.Join(texts, "")
	if joined != "just plain text" {
		t.Errorf("got %q, want 'just plain text'", joined)
	}
}