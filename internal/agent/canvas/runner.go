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

// runner.go — Canvas execution runtime. Drives a Canvas invocation
// (the caller supplies the RunFunc that does Compile+Invoke), catches
// the four possible outcomes, and surfaces them as RunEvent values on
// a channel that the HTTP layer streams as SSE frames.
//
// Why this file lives in the canvas package: it is the runtime twin
// of scheduler.go (BuildWorkflow = "how to build", Runner = "how to
// drive"). Both concern the Canvas execution lifecycle; nothing
// outside the canvas package needs to know that these concerns are
// split across two files.
//
// Run outcomes — four paths on a single Run() call:
//
//  1. Normal completion (runErr == nil): the buildRunFunc already
//     emitted all workflow events (workflow_started, node_started,
//     node_finished, message, message_end, workflow_finished) during
//     execution. The Runner just sends the `done` terminator.
//  2. Eino interrupt (runErr is an *InterruptSignal or wrapped
//     variant): emit `waiting_for_user` with the first interrupt
//     id. Persist the id so the next call can resume via
//     compose.ResumeWithData (signalled through root:
//     __resume_interrupt_id__ + __resume_data__).
//  3. Cancel / timeout (errors.Is(err, context.Canceled) etc.):
//     silently close. The HTTP handler has already detached.
//  4. Other errors: emit `error` event with the err.Error() string.
//
// SSE wire contract (matches the handler envelope):
//   - RunEvent.Type == "message"          → {data: <string>}
//   - RunEvent.Type == "waiting_for_user" → {cpn_id: <string>}
//   - RunEvent.Type == "error"            → {message: <string>}
//   - RunEvent.Type == "done"             → final terminator frame
package canvas

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"ragflow/internal/utility"
	"runtime/debug"
	"sync"
	"time"

	"go.uber.org/zap"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
)

// RunEvent is the unit the Runner pushes onto its output channel.
// The handler converts each RunEvent into one SSE frame in the
// Python-shaped envelope:
//
//	data:{"event":"<Type>","message_id":"<MessageID>","created_at":<CreatedAt>,"task_id":"<TaskID>","session_id":"<SessionID>","data":<Data>}
//
// Type is the event tag; Data is the JSON payload string (already
// serialised — handler does not re-marshal). The handler wraps Data
// into the "data" field of the outer envelope so the front-end's
// use-send-message.ts parser sees a flat {event, message_id,
// created_at, task_id, session_id, data} object on every frame.
type RunEvent struct {
	Type      string
	Data      string
	MessageID string
	CreatedAt int64
	TaskID    string
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
	Content   string        `json:"content"`
	Reference []interface{} `json:"reference,omitempty"`
}

// MessageEndEvent is the JSON payload for Type=="message_end" frames.
type MessageEndEvent struct {
	Status     *string       `json:"status,omitempty"`
	Attachment []interface{} `json:"attachment,omitempty"`
	Reference  []interface{} `json:"reference,omitempty"`
}

// WaitingForUserEvent is the JSON payload for Type=="waiting_for_user"
// frames. CpnID is the cpn id that emitted the wait sentinel — the
// front-end can use it to surface the prompt or to attach the
// follow-up to the right conversation turn.
type WaitingForUserEvent struct {
	CpnID  string         `json:"cpn_id"`
	Tips   string         `json:"tips,omitempty"`
	Inputs map[string]any `json:"inputs,omitempty"`
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
//   - non-nil error that is an eino interrupt signal: the run paused
//     on a wait-for-user node. The Runner extracts the InterruptCtx
//     list via ExtractInterruptContexts and emits a `waiting_for_user`
//     event. state may be nil in this branch (the engine does not
//     surface a completed state when it halts on an interrupt).
//   - any other non-nil error: run failed; surface as `error` event.
type RunFunc func(ctx context.Context, root map[string]any) (*CanvasState, error)

// Runner is the per-canvas execution runtime. It owns the
// interrupt-id map (V1 in-memory persistence keyed by
// (canvasID, sessionID)) and the goroutine cancellation registry.
//
// Concurrency: Runner methods are safe for concurrent use. The
// output channel is owned by the goroutine that started a run;
// the Cancel method signals the underlying run via the cancel
// channel that the RunFunc is expected to observe.
type Runner struct {
	mu           sync.Mutex
	interruptIDs map[string]string // key = canvasID + "|" + sessionID; value = eino interrupt id
	runCancels   map[string]chan struct{}
}

// NewRunner returns a fresh Runner with the in-memory interrupt-id
// map initialised. The Runner has no background goroutines; it is
// owned by the AgentService.
func NewRunner() *Runner {
	return &Runner{
		interruptIDs: make(map[string]string),
		runCancels:   make(map[string]chan struct{}),
	}
}

// sessionKey is the lookup key for the in-memory interrupt-id map. We
// concatenate with a separator that cannot appear in either id (the
// id format is uuid-hex) so two adjacent ids never collide.
func sessionKey(canvasID, sessionID string) string {
	return canvasID + "|" + sessionID
}

// saveInterruptID stores the eino interrupt id for a (canvasID,
// sessionID) pair. Called when the RunFunc returns an interrupt
// error; the next RunAgent call with the same session id reads it
// back via getInterruptID and forwards it to the RunFunc so the
// RunFunc can target it via compose.ResumeWithData.
func (r *Runner) saveInterruptID(canvasID, sessionID, interruptID string) {
	if interruptID == "" {
		return
	}
	r.mu.Lock()
	r.interruptIDs[sessionKey(canvasID, sessionID)] = interruptID
	r.mu.Unlock()
}

// getInterruptID reads back the interrupt id saved by the previous
// run, then deletes it (the resume consumes it). Returns "" when no
// prior paused run exists for this session.
func (r *Runner) getInterruptID(canvasID, sessionID string) string {
	r.mu.Lock()
	id, ok := r.interruptIDs[sessionKey(canvasID, sessionID)]
	if ok {
		delete(r.interruptIDs, sessionKey(canvasID, sessionID))
	}
	r.mu.Unlock()
	return id
}

// Run drives one canvas invocation. See package docstring for the
// four-outcome flow. The channel is always closed on return so the
// handler's for-range loop terminates.
//
// Metadata injection: the output channel, message_id, task_id, and
// session_id are injected into root so the RunFunc (buildRunFunc in
// service/agent.go) can emit intermediate events (workflow_started,
// node_started, node_finished, workflow_finished) during execution
// rather than only after the invoke completes. The key names follow
// the __<name>__ sentinel convention to avoid collisions with
// runtime DSL keys.
func (r *Runner) Run(
	ctx context.Context,
	run RunFunc,
	canvasID, sessionID string,
	userInput any,
	root map[string]any,
) <-chan RunEvent {
	out := make(chan RunEvent, 8)

	if run == nil {
		pushErr(out, "canvas: nil RunFunc")
		close(out)
		return out
	}

	cancel := make(chan struct{})
	r.mu.Lock()
	if prev, hadPrev := r.runCancels[canvasID]; hadPrev {
		select {
		case <-prev:
		default:
			close(prev)
		}
	}
	r.runCancels[canvasID] = cancel
	r.mu.Unlock()

	// Generate the identifiers the RunFunc and SSE envelope need.
	// message_id is generated per-run so the front-end can correlate
	// all events for a single user turn. task_id is the published
	// version id (if available) or a per-run UUID.
	messageID := utility.GenerateToken()
	taskID := ""
	if v, ok := root["version_id"].(string); ok && v != "" {
		taskID = v
	}
	if taskID == "" {
		taskID = utility.GenerateToken()
	}

	// Inject the output channel + metadata so the RunFunc can emit
	// events during execution (workflow_started, node_started,
	// node_finished, etc.).
	root["__events__"] = out
	root["__message_id__"] = messageID
	root["__task_id__"] = taskID
	root["__session_id__"] = sessionID

	go func() {
		defer close(out)
		defer func() {
			r.mu.Lock()
			if r.runCancels[canvasID] == cancel {
				delete(r.runCancels, canvasID)
			}
			r.mu.Unlock()
		}()
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

		// Resume path: inject the previously-saved interrupt id and
		// the user's follow-up into root. The RunFunc reads these
		// keys and decorates ctx with compose.ResumeWithData before
		// invoking the workflow. The sentinel keys are deleted from
		// root inside the RunFunc — see service/agent.go's
		// buildRunFunc.
		if userInput != nil {
			if id := r.getInterruptID(canvasID, sessionID); id != "" {
				root["__resume_interrupt_id__"] = id
				root["__resume_data__"] = userInput
			}
		}

		_, runErr := safeInvoke(ctx, cancel, run, root)
		if runErr != nil {
			if errors.Is(runErr, context.Canceled) || errors.Is(runErr, errCancelled) {
				return
			}
			if ctxs := ExtractInterruptContexts(runErr); len(ctxs) > 0 {
				// Wait-for-user: persist the real root-cause interrupt id for
				// compose.ResumeWithData, but keep exposing the leaf
				// user_fill_up interrupt id to the front-end so it can attach
				// the prompt to the visible waiting node.
				displayID := FirstInterruptID(ctxs)
				resumeID := RootInterruptID(ctxs)
				common.Info("canvas runner interrupt",
					zap.String("canvas", canvasID),
					zap.String("session", sessionID),
					zap.String("task", taskID),
					zap.String("contexts", formatInterruptContexts(ctxs)),
					zap.String("display", displayID),
					zap.String("resume", resumeID))
				r.saveInterruptID(canvasID, sessionID, resumeID)
				waiting := WaitingForUserEvent{CpnID: displayID}
				if ctx := FirstUserFillUpInterrupt(ctxs); ctx != nil {
					if info, ok := ctx.Info.(map[string]any); ok {
						if tips, _ := info["tips"].(string); tips != "" {
							waiting.Tips = tips
						}
						if inputs, ok := info["inputs"].(map[string]any); ok && len(inputs) > 0 {
							waiting.Inputs = inputs
						}
					}
				}
				push(out, RunEvent{Type: "waiting_for_user", Data: safeEventJSON(waiting), MessageID: messageID, CreatedAt: nowUnix(), TaskID: taskID, SessionID: sessionID})
				// Always close a RunAgent call with the `done`
				// terminator so the front-end can rely on a
				// channel-end sentinel regardless of whether the run
				// completed, errored, or paused for user input.
				push(out, RunEvent{Type: "done", Data: "", MessageID: messageID, CreatedAt: nowUnix(), TaskID: taskID, SessionID: sessionID})
				return
			}
			if IsInterruptError(runErr) {
				// Raw InterruptSignal (no wrapped InterruptCtx list
				// available). Emit a generic waiting_for_user event
				// without a cpn id — the front-end falls back to
				// the first paused session it knows about.
				r.saveInterruptID(canvasID, sessionID, runErr.Error())
				push(out, RunEvent{Type: "waiting_for_user", Data: safeEventJSON(WaitingForUserEvent{CpnID: runErr.Error()}), MessageID: messageID, CreatedAt: nowUnix(), TaskID: taskID, SessionID: sessionID})
				push(out, RunEvent{Type: "done", Data: "", MessageID: messageID, CreatedAt: nowUnix(), TaskID: taskID, SessionID: sessionID})
				return
			}
			pushErr(out, runErr.Error())
			// Close the channel with the `done` terminator so the
			// front-end sees a channel-end sentinel on the error
			// path too — matches the contract for completed and
			// waiting-for-user paths above.
			push(out, RunEvent{Type: "done", Data: "", MessageID: messageID, CreatedAt: nowUnix(), TaskID: taskID, SessionID: sessionID})
			return
		}

		// Normal completion — the buildRunFunc already emitted the
		// workflow events during execution. Runner just sends the
		// terminator.
		push(out, RunEvent{Type: "done", Data: "", MessageID: messageID, CreatedAt: nowUnix(), TaskID: taskID, SessionID: sessionID})
	}()

	return out
}

// Cancel signals an in-flight run for the given canvas to stop.
// Safe to call when no run is active.
func (r *Runner) Cancel(canvasID string) {
	r.mu.Lock()
	cancel, ok := r.runCancels[canvasID]
	r.mu.Unlock()
	if !ok {
		return
	}
	select {
	case <-cancel:
	default:
		close(cancel)
	}
}

// Peek reports whether a paused interrupt id is held for the given
// (canvasID, sessionID). It is intended for tests and diagnostics;
// the real runner does not need it at run time.
func (r *Runner) Peek(canvasID, sessionID string) bool {
	r.mu.Lock()
	_, ok := r.interruptIDs[sessionKey(canvasID, sessionID)]
	r.mu.Unlock()
	return ok
}

// errCancelled is the sentinel safeInvoke returns when the cancel
// channel fires during a run. It is wrapped against context.Canceled
// so callers can `errors.Is` either.
var errCancelled = fmt.Errorf("canvas: run cancelled")

// safeInvoke calls the supplied RunFunc with context-cancel and
// driver-cancel both wired in. The RunFunc is expected to honour
// ctx.Done() — the cancel channel is a secondary signal for the
// V1 in-process driver.
func safeInvoke(ctx context.Context, cancel chan struct{}, run RunFunc, root map[string]any) (*CanvasState, error) {
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
		return state, err
	case <-cancel:
		return nil, errCancelled
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
func pushErr(out chan<- RunEvent, msg string) {
	payload, err := json.Marshal(ErrorEvent{Message: msg})
	if err != nil {
		common.Warn("runner: pushErr json.Marshal failed, falling back",
			zap.Error(err))
		// ErrorEvent only has a string field; this should never fail.
		// Fall back to a hard-coded minimal JSON.
		payload = []byte(`{"message":"event serialization failed"}`)
	}
	push(out, RunEvent{Type: "error", Data: string(payload)})
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

// extractAnswerFromState is kept for reference but is no longer called
// by the Runner — answer extraction now happens in buildRunFunc.
// Remove in a follow-up cleanup pass once all tests pass.
func extractAnswerFromState(state *CanvasState) (string, []interface{}) {
	if state == nil {
		return "", nil
	}
	snap := state.Snapshot()
	var answer string
	var reference []interface{}
	// First pass: look for an "answer" key (preferred).
	for _, bucket := range snap {
		if a, ok := bucket["answer"].(string); ok && a != "" {
			answer = a
			break
		}
	}
	// Second pass: fall back to "result" then "content" if
	// no "answer" was found.
	if answer == "" {
		for _, bucket := range snap {
			if r, ok := bucket["result"].(string); ok && r != "" {
				answer = r
				break
			}
		}
	}
	if answer == "" {
		for _, bucket := range snap {
			if c, ok := bucket["content"].(string); ok && c != "" {
				answer = c
				break
			}
		}
	}
	// Collect references (best-effort, no precedence).
	for _, bucket := range snap {
		if r, ok := bucket["reference"].([]interface{}); ok {
			reference = append(reference, r...)
		}
	}
	if answer == "" {
		answer = "Run completed with no surfaceable answer."
	}
	return answer, reference
}
