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
	deltas := NextThinkDelta(state, "hello world", 0)
	if len(deltas) != 1 {
		t.Fatalf("expected 1 delta, got %d: %+v", len(deltas), deltas)
	}
	if deltas[0].Kind != ThinkDeltaText || deltas[0].Value != "hello world" {
		t.Errorf("expected text delta, got %+v", deltas[0])
	}
}

func TestNextThinkDelta_OnlyThinkTag(t *testing.T) {
	state := &ThinkStreamState{}
	deltas := NextThinkDelta(state, "<think>reasoning</think>visible", 0)
	if len(deltas) < 2 {
		t.Fatalf("expected at least 2 deltas, got %d: %+v", len(deltas), deltas)
	}
	foundOpen, foundClose := false, false
	for _, d := range deltas {
		if d.Kind == ThinkDeltaMarker && d.Value == "<think>" {
			foundOpen = true
		}
		if d.Kind == ThinkDeltaMarker && d.Value == "</think>" {
			foundClose = true
		}
	}
	if !foundOpen || !foundClose {
		t.Errorf("missing markers: open=%v close=%v, deltas=%+v", foundOpen, foundClose, deltas)
	}
}

func TestNextThinkDelta_TextThenThink(t *testing.T) {
	state := &ThinkStreamState{}
	deltas := NextThinkDelta(state, "before <think>inside</think> after", 0)
	foundOpen, foundClose := false, false
	for _, d := range deltas {
		if d.Kind == ThinkDeltaMarker && d.Value == "<think>" {
			foundOpen = true
		}
		if d.Kind == ThinkDeltaMarker && d.Value == "</think>" {
			foundClose = true
		}
	}
	if !foundOpen || !foundClose {
		t.Errorf("missing markers: open=%v close=%v, deltas=%+v", foundOpen, foundClose, deltas)
	}
}

func TestNextThinkDelta_MultipleChunks(t *testing.T) {
	state := &ThinkStreamState{}
	deltas := NextThinkDelta(state, "hello ", 0)
	_ = deltas
	deltas = NextThinkDelta(state, "<think>", 0)
	_ = deltas
	deltas = NextThinkDelta(state, "reasoning", 0)
	_ = deltas
	deltas = NextThinkDelta(state, "</think>", 0)
	_ = deltas
	deltas = NextThinkDelta(state, " world", 0)
	// After deferral, closing marker and answer text arrive in the last chunk.
	foundClose := false
	for _, d := range deltas {
		if d.Kind == ThinkDeltaMarker && d.Value == "</think>" {
			foundClose = true
		}
	}
	if !foundClose {
		t.Errorf("expected </think> in final chunk, got %+v", deltas)
	}
}

func TestNextThinkDelta_UnclosedThink(t *testing.T) {
	state := &ThinkStreamState{}
	deltas := NextThinkDelta(state, "text <think>unclosed", 0)
	foundOpen := false
	for _, d := range deltas {
		if d.Kind == ThinkDeltaMarker && d.Value == "<think>" {
			foundOpen = true
		}
	}
	if !foundOpen {
		t.Errorf("expected <think> marker in %+v", deltas)
	}
}

func TestNextThinkDelta_EmptyInput(t *testing.T) {
	state := &ThinkStreamState{}
	deltas := NextThinkDelta(state, "", 0)
	if len(deltas) != 0 {
		t.Errorf("expected 0 deltas for empty input, got %d", len(deltas))
	}
}

func TestNextThinkDelta_NilState(t *testing.T) {
	deltas := NextThinkDelta(nil, "test", 0)
	if deltas != nil {
		t.Error("expected nil for nil state")
	}
}

func TestFlushRemaining_FlushesAll(t *testing.T) {
	state := &ThinkStreamState{
		thinkBuffer:       "think-tail",
		closePending:      true,
		answerBuffer:      "answer-tail",
		pendingAfterClose: "pending-tail",
	}
	deltas := FlushRemaining(state)
	if len(deltas) != 4 {
		t.Fatalf("expected 4 deltas, got %d: %+v", len(deltas), deltas)
	}
	if deltas[0].Kind != ThinkDeltaText || deltas[0].Value != "think-tail" {
		t.Errorf("delta[0] = %+v", deltas[0])
	}
	if deltas[1].Kind != ThinkDeltaMarker || deltas[1].Value != "</think>" {
		t.Errorf("delta[1] = %+v", deltas[1])
	}
	if deltas[2].Kind != ThinkDeltaText || deltas[2].Value != "answer-tail" {
		t.Errorf("delta[2] = %+v", deltas[2])
	}
	if deltas[3].Kind != ThinkDeltaText || deltas[3].Value != "pending-tail" {
		t.Errorf("delta[3] = %+v", deltas[3])
	}
	// State should be cleared.
	if state.closePending || state.inThink {
		t.Error("state not fully cleared")
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
		t.Errorf("got %q", got)
	}
}

func TestExtractVisibleAnswer_MultipleThinks(t *testing.T) {
	raw := "<think>first</think>visible1<think>second</think>visible2"
	if got := ExtractVisibleAnswer(raw); got != "visible2" {
		t.Errorf("got %q", got)
	}
}

func TestExtractVisibleAnswer_NestedTags(t *testing.T) {
	raw := "<think><think>nested</think></think>answer"
	if got := ExtractVisibleAnswer(raw); got != "answer" {
		t.Errorf("got %q", got)
	}
}

func TestExtractVisibleAnswer_NoTags(t *testing.T) {
	if got := ExtractVisibleAnswer("plain text"); got != "plain text" {
		t.Errorf("got %q", got)
	}
}

func TestExtractVisibleAnswer_StrayTag(t *testing.T) {
	if got := ExtractVisibleAnswer("<think>text"); got != "text" {
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
	if !strings.Contains(joined, "hello wor") || !strings.Contains(joined, "final") {
		t.Errorf("texts = %q", texts)
	}
}

func TestStreamThinkTagDelta_IncrementalFlush(t *testing.T) {
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

func TestStreamThinkTagDelta_DeferredClose(t *testing.T) {
	// When </think> has no visible text after it, the marker is deferred.
	chunks := []string{"<think>", "hello", "</think>", "world"}
	ch := make(chan string, len(chunks))
	for _, c := range chunks {
		ch <- c
	}
	close(ch)

	var markers []string
	for d := range StreamThinkTagDelta(context.Background(), ch, 1) {
		if d.Kind == ThinkDeltaMarker {
			markers = append(markers, d.Value)
		}
	}
	if len(markers) != 2 {
		t.Fatalf("expected 2 markers, got %d: %v", len(markers), markers)
	}
	if markers[0] != "<think>" {
		t.Errorf("first marker = %q", markers[0])
	}
	if markers[1] != "</think>" {
		t.Errorf("second marker = %q", markers[1])
	}
}
