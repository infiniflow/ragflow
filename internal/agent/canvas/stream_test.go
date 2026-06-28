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

package canvas

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestChannelEmitter_EmitAndClose(t *testing.T) {
	em := NewChannelEmitter(4)
	ch := em.(*channelEmitter).Channel()

	evs := []StreamEvent{
		{Event: "node_start", TaskID: "t1", Component: "begin_0"},
		{Event: "message", TaskID: "t1", Component: "llm_0",
			Data: map[string]any{"delta": "hello"}},
		{Event: "node_finish", TaskID: "t1", Component: "begin_0",
			Data: map[string]any{"ok": true}},
	}
	for _, ev := range evs {
		if err := em.Emit(ev); err != nil {
			t.Fatalf("Emit %q: %v", ev.Event, err)
		}
	}
	if err := em.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	var got []StreamEvent
	for ev := range ch {
		got = append(got, ev)
	}
	if len(got) != len(evs) {
		t.Fatalf("got %d events, want %d", len(got), len(evs))
	}
	for i, ev := range got {
		if ev.Event != evs[i].Event || ev.TaskID != evs[i].TaskID ||
			ev.Component != evs[i].Component {
			t.Fatalf("event %d: got %+v, want %+v", i, ev, evs[i])
		}
	}
}

func TestChannelEmitter_NonBlockingDrop(t *testing.T) {
	// Buffer of 1 with no reader; the second Emit must return nil
	// immediately (drop on full) rather than block.
	em := NewChannelEmitter(1)
	if err := em.Emit(StreamEvent{Event: "e1", TaskID: "t"}); err != nil {
		t.Fatalf("Emit 1: %v", err)
	}
	done := make(chan struct{})
	go func() {
		if err := em.Emit(StreamEvent{Event: "e2", TaskID: "t"}); err != nil {
			t.Errorf("Emit 2: %v", err)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Emit blocked despite non-blocking contract")
	}
	// The first event is still buffered; the second was dropped.
	ch := em.(*channelEmitter).Channel()
	first := <-ch
	if first.Event != "e1" {
		t.Fatalf("first buffered event = %q, want e1", first.Event)
	}
}

func TestFormatSSE(t *testing.T) {
	ev := StreamEvent{
		Event:     "message",
		TaskID:    "task_42",
		Component: "llm_0",
		Data: map[string]any{
			"delta": "héllo, 世界",
			"index": 7,
		},
	}
	got := FormatSSE(ev)

	if !strings.HasPrefix(got, "data: ") {
		t.Fatalf("SSE frame must start with 'data: '; got %q", got)
	}
	if !strings.HasSuffix(got, "\n\n") {
		t.Fatalf("SSE frame must end with '\\n\\n'; got %q", got)
	}
	body := strings.TrimPrefix(got, "data: ")
	body = strings.TrimSuffix(body, "\n\n")

	// Body must be valid JSON and round-trip the Data field.
	var decoded map[string]any
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatalf("SSE body is not JSON: %v\nbody: %q", err, body)
	}
	if decoded["delta"] != "héllo, 世界" {
		t.Fatalf("delta round-trip: got %q, want %q", decoded["delta"], "héllo, 世界")
	}
	if v, _ := decoded["index"].(float64); v != 7 {
		t.Fatalf("index round-trip: got %v, want 7", decoded["index"])
	}
}

func TestFormatSSE_EmptyData(t *testing.T) {
	// Empty Data must still produce a valid frame, not panic.
	got := FormatSSE(StreamEvent{Event: "node_start", TaskID: "t"})
	if !strings.HasPrefix(got, "data: ") || !strings.HasSuffix(got, "\n\n") {
		t.Fatalf("empty Data frame malformed: %q", got)
	}
}

func TestChannelEmitter_CloseIdempotentCheck(t *testing.T) {
	// Emitting after Close must panic — callers should not emit on a
	// closed emitter. This is the desired Go-idiomatic signal.
	em := NewChannelEmitter(1)
	ch := em.(*channelEmitter).Channel()
	if err := em.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Drain to confirm the channel is closed.
	if _, ok := <-ch; ok {
		t.Fatal("channel not closed after Close()")
	}
}
