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

// stream.go defines the SSE event channel and the helper that
// formats events in the Python agent_api.py wire format.
package canvas

import (
	"encoding/json"
	"log"
)

// StreamEvent is the unit emitted by canvas components to the SSE writer.
// Field names match the Python "data" payload shape so a single
// frontend SSE parser can consume both runtimes.
type StreamEvent struct {
	// Event is the event name: "node_start" | "node_finish" | "message" | "error" | "cancelled" | ...
	Event string `json:"event"`
	// TaskID identifies the canvas run; required for client correlation.
	TaskID string `json:"task_id"`
	// Component identifies the canvas component that produced the event.
	Component string `json:"component,omitempty"`
	// Data is the free-form event body. SSE wire format is "data: " + json(ev.Data).
	Data map[string]any `json:"data,omitempty"`
}

// StreamEmitter pushes events toward an SSE writer. Emit must be
// non-blocking — a slow consumer must not stall canvas execution. The
// Phase-1 implementation drops events when the buffer is full and
// logs a warning; a Phase-5 SSE handler can swap in a back-pressured
// implementation if needed.
type StreamEmitter interface {
	Emit(ev StreamEvent) error
	Close() error
}

// channelEmitter is the default StreamEmitter: a buffered Go channel
// drained by an HTTP handler running in a separate goroutine.
type channelEmitter struct {
	ch chan StreamEvent
}

// NewChannelEmitter returns a StreamEmitter backed by a buffered channel
// of the given size. Size 0 is valid (unbuffered) but will block Emit
// until a reader is ready — typically not what canvas runs want.
func NewChannelEmitter(buffer int) StreamEmitter {
	return &channelEmitter{ch: make(chan StreamEvent, buffer)}
}

// Emit pushes ev onto the channel. Non-blocking: if the buffer is full
// the event is dropped and a warning is logged. Returning a nil error
// on drop is intentional — the canvas run must keep going even if the
// SSE consumer is slow or absent.
func (e *channelEmitter) Emit(ev StreamEvent) error {
	select {
	case e.ch <- ev:
		return nil
	default:
		log.Printf("canvas stream: dropping event %q for task %q (buffer full)",
			ev.Event, ev.TaskID)
		return nil
	}
}

// Close closes the underlying channel. Safe to call once; further Emits
// will panic (caught by the run goroutine's defer) which is the desired
// signal that the emitter is no longer usable.
func (e *channelEmitter) Close() error {
	close(e.ch)
	return nil
}

// Channel returns the underlying receive-only channel. It is exported
// (lowercase access from same package) only for tests; production code
// should consume via the StreamEmitter interface.
func (e *channelEmitter) Channel() <-chan StreamEvent {
	return e.ch
}

// FormatSSE renders ev into the Python agent_api.py wire format:
// `data: <json>\n\n`. JSON is emitted without HTML escaping so unicode
// stays readable. Errors marshaling Data fall back to a minimal
// `{"error": "..."}` payload so the SSE stream never gets a malformed
// frame.
func FormatSSE(ev StreamEvent) string {
	body, err := json.Marshal(ev.Data)
	if err != nil {
		body, _ = json.Marshal(map[string]string{
			"error":  "stream marshal failed",
			"detail": err.Error(),
		})
	}
	return "data: " + string(body) + "\n\n"
}
