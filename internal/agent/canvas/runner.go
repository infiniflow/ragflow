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

// Package canvas runner.go — Canvas execution runtime. Drives a Canvas invocation
// (the caller supplies the RunFunc that does Compile+Invoke), catches
// the possible outcomes, and surfaces them as RunEvent values on
// a channel that the HTTP layer streams as SSE frames.
//
// Why this file lives in the canvas package: it is the runtime twin
// of scheduler.go (BuildWorkflow = "how to build", Runner = "how to
// drive"). Both concern the Canvas execution lifecycle; nothing
// outside the canvas package needs to know that these concerns are
// split across two files.
//
// Run outcomes on a single Run() call:
//
//  1. Normal completion (runErr == nil): the buildRunFunc already
//     emitted all workflow events (workflow_started, node_started,
//     node_finished, message, message_end, workflow_finished) during
//     execution. The Runner just sends the `done` terminator.
//  2. Cancel / timeout (errors.Is(err, context.Canceled) etc.):
//     silently close. The HTTP handler has already detached.
//  3. Other errors: emit `error` event with the err.Error() string.
//
// SSE wire contract (matches the handler envelope):
//   - RunEvent.Type == "message"          → {data: <string>}
//   - RunEvent.Type == "error"            → {message: <string>}
package canvas

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"ragflow/internal/utility"
	"runtime/debug"
	"time"

	"go.uber.org/zap"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
)

// RunEvent is the unit the Runner pushes onto its output channel.
// The handler converts each RunEvent into one SSE frame in the
// Python-shaped envelope:
//
//	data:{"event":"<Type>","message_id":"<MessageID>","created_at":<CreatedAt>,"session_id":"<SessionID>","data":<Data>}
//
// Type is the event tag; Data is the JSON payload string (already
// serialised — handler does not re-marshal). The handler wraps Data
// into the "data" field of the outer envelope so the front-end's
// use-send-message.ts parser sees a flat {event, message_id,
// created_at, session_id, data} object on every frame.
// WriteChatbotRunEvent may additionally expose task_id=session_id as a wire
// alias for existing clients; RunEvent itself has only one run identity.
type RunEvent struct {
	Type      string
	Data      string
	MessageID string
	CreatedAt int64
	SessionID string
}

// NodeStartedData is the "data" payload for "node_started" events.
type NodeStartedData struct {
	Inputs        interface{} `json:"inputs"`
	CreatedAt     float64     `json:"created_at"`
	ComponentID   string      `json:"component_id"`
	ComponentName string      `json:"component_name"`
	ComponentType string      `json:"component_type"`
	Thoughts      string      `json:"thoughts"`
}

// NodeFinishedData is the "data" payload for "node_finished" events.
type NodeFinishedData struct {
	Inputs        interface{} `json:"inputs"`
	Outputs       interface{} `json:"outputs"`
	ComponentID   string      `json:"component_id"`
	ComponentName string      `json:"component_name"`
	ComponentType string      `json:"component_type"`
	Error         interface{} `json:"error"`
	ElapsedTime   float64     `json:"elapsed_time"`
	CreatedAt     float64     `json:"created_at"`
}

// MessageEvent is the JSON payload for Type=="message" frames.
type MessageEvent struct {
	Content      string      `json:"content"`
	Reference    interface{} `json:"reference,omitempty"`
	Thinking     string      `json:"thinking,omitempty"`
	StartToThink bool        `json:"start_to_think,omitempty"`
	EndToThink   bool        `json:"end_to_think,omitempty"`
}

// MessageEndEvent is the JSON payload for Type=="message_end" frames.
type MessageEndEvent struct {
	Status     *string       `json:"status,omitempty"`
	Attachment []interface{} `json:"attachment,omitempty"`
	Reference  interface{}   `json:"reference,omitempty"`
}

// ErrorEvent is the JSON payload for Type=="error" frames.
type ErrorEvent struct {
	Message string `json:"message"`
}

// RunFunc is the canvas execution contract the Runner depends on.
// Service-layer code supplies an implementation that compiles the
// DSL and invokes the eino Workflow; the Runner is agnostic to
// that machinery.
//
// Return contract:
//
//   - nil error, non-nil state: run completed normally.
//   - any non-nil error: run failed; surface as `error` event.
type RunFunc func(ctx context.Context, root map[string]any) (*CanvasState, error)

// Runner is the ordinary-Agent execution runtime. The service owns the run
// context and cancellation.
//
// Concurrency: Runner methods are safe for concurrent use. The
// output channel is owned by the goroutine that started a run.
type Runner struct{}

// NewRunner returns a fresh Runner. The Runner has no background
// goroutines; it is owned by the AgentService.
func NewRunner() *Runner {
	return &Runner{}
}

// Run drives one canvas invocation. See package docstring for the
// outcome flow. The channel is always closed on return so the
// handler's for-range loop terminates.
//
// Metadata injection: the output channel, message_id, and session_id are
// injected into root so the RunFunc (buildRunFunc in
// service/agent.go) can emit intermediate events (workflow_started,
// node_started, node_finished, workflow_finished) during execution
// rather than only after the invoke completes. The key names follow
// the __<name>__ sentinel convention to avoid collisions with
// runtime DSL keys.
func (r *Runner) Run(
	ctx context.Context,
	run RunFunc,
	canvasID, sessionID string,
	root map[string]any,
) <-chan RunEvent {
	out := make(chan RunEvent, 8)

	if run == nil {
		pushErr(out, "canvas: nil RunFunc", sessionID)
		close(out)
		return out
	}

	// Generate the message identifier the RunFunc and SSE envelope need.
	messageID := utility.GenerateToken()

	// Inject the output channel + metadata so the RunFunc can emit
	// events during execution (workflow_started, node_started,
	// node_finished, etc.).
	root["__events__"] = out
	root["__message_id__"] = messageID
	root["__session_id__"] = sessionID

	go func() {
		defer close(out)
		// Panic sentinel (temporary diagnostic — see plan):
		// a panic anywhere in the run goroutine used to silently
		// propagate, leaving the events channel closed-empty so the
		// SSE handler streamed a 200 OK with an empty body. We now
		// log the panic value + stack trace so the next failing run
		// surfaces a clear root cause in the server log.
		defer func() {
			if rec := recover(); rec != nil {
				common.Error("canvas runner PANIC", fmt.Errorf("%v", rec),
					zap.String("canvas", canvasID),
					zap.String("session", sessionID),
					zap.String("stack", string(debug.Stack())))
			}
		}()

		_, runErr := safeInvoke(ctx, run, root)
		if runErr != nil {
			if errors.Is(runErr, context.Canceled) {
				return
			}
			pushErr(out, runErr.Error(), sessionID)
			return
		}
	}()

	return out
}

// safeInvoke calls the supplied RunFunc with the managed child context.
// The RunFunc is expected to honour ctx.Done().
func safeInvoke(ctx context.Context, run RunFunc, root map[string]any) (*CanvasState, error) {
	done := make(chan struct{})
	var (
		state *CanvasState
		err   error
	)
	go func() {
		// Recover here, inside the goroutine that actually invokes
		// `run`. A panic from `run` would otherwise crash the process
		// before any caller could observe it; converting it into a
		// regular error keeps the SSE contract intact and lets the
		// runner emit a terminal `done` event.
		defer func() {
			if rec := recover(); rec != nil {
				common.Error("canvas runner PANIC", fmt.Errorf("%v", rec),
					zap.String("stack", string(debug.Stack())))
				err = fmt.Errorf("canvas runner panic: %v", rec)
			}
			close(done)
		}()
		state, err = run(ctx, root)
	}()
	select {
	case <-done:
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return state, err
	case <-ctx.Done():
		// Do not abandon the workflow goroutine. Eino and context-aware
		// HTTP/tool calls should return promptly once the child context is
		// cancelled; waiting here keeps the run under management.
		<-done
		return nil, ctx.Err()
	}
}

// PushEvent sends an event to the channel, dropping it if the consumer
// has gone away (handler cancelled). Exported so the service layer's
// buildRunFunc can emit intermediate workflow events through the
// same channel during execution.
func PushEvent(ch chan<- RunEvent, ev RunEvent) {
	defer func() { _ = recover() }()
	ch <- ev
}

// push sends an event to the channel, dropping it if the consumer
// has gone away (handler cancelled). Errors on send are intentional
// and ignored — the handler is the only consumer and its
// `for-range` loop exits when the request context is cancelled.
func push(out chan<- RunEvent, ev RunEvent) {
	defer func() { _ = recover() }()
	out <- ev
}

// pushErr serialises an ErrorEvent and pushes it on the channel.
func pushErr(out chan<- RunEvent, msg, sessionID string) {
	payload, err := json.Marshal(ErrorEvent{Message: msg})
	if err != nil {
		common.Warn("runner: pushErr json.Marshal failed, falling back",
			zap.Error(err))
		// ErrorEvent only has a string field; this should never fail.
		// Fall back to a hard-coded minimal JSON.
		payload = []byte(`{"message":"event serialization failed"}`)
	}
	push(out, RunEvent{Type: "error", Data: string(payload), SessionID: sessionID, CreatedAt: nowUnix()})
}

// safeEventJSON marshals v to a JSON string, falling back to
// runtime.SafeJSONMarshal when the value contains non-serializable
// types (funcs, channels). Mirrors the Python PR #14210
// _canvas_json_default fallback for SSE event serialization.
func safeEventJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		common.Warn("runner: json.Marshal event payload failed, trying SafeJSONMarshal",
			zap.Error(err))
		b, err = runtime.SafeJSONMarshal(v)
		if err != nil {
			common.Error("runner: SafeJSONMarshal also failed, using fallback",
				err)
			b = []byte(`{"message":"event serialization failed"}`)
		}
	}
	return string(b)
}

// nowUnix returns the current Unix timestamp in seconds.
func nowUnix() int64 {
	return time.Now().Unix()
}
