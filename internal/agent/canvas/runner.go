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
//  1. Normal completion (runErr == nil): emit `message` + `done`.
//     The answer is extracted from the post-run state via
//     extractAnswerFromState (catches "answer" / "result" / "content"
//     keys — matches Python's v1 surface for legacy SSE consumers).
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
	"sync"
)

// RunEvent is the unit the Runner pushes onto its output channel.
// The handler converts each RunEvent into one SSE frame
// (`data: {...}\n\n`). Type is the event tag; Data is the JSON
// payload (already serialised — handler does not re-marshal).
type RunEvent struct {
	Type string
	Data string
}

// MessageEvent is the JSON payload for Type=="message" frames.
// The Python agent API streams arbitrary strings; we mirror that
// shape so the existing front-end parses the events the same way.
type MessageEvent struct {
	Answer    string        `json:"answer,omitempty"`
	Reference []interface{} `json:"reference,omitempty"`
}

// WaitingForUserEvent is the JSON payload for Type=="waiting_for_user"
// frames. CpnID is the cpn id that emitted the wait sentinel — the
// front-end can use it to surface the prompt or to attach the
// follow-up to the right conversation turn.
type WaitingForUserEvent struct {
	CpnID string `json:"cpn_id"`
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
func (r *Runner) Run(
	ctx context.Context,
	run RunFunc,
	canvasID, sessionID, userInput string,
	root map[string]any,
) <-chan RunEvent {
	out := make(chan RunEvent, 4)

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

	go func() {
		defer close(out)
		defer func() {
			r.mu.Lock()
			if r.runCancels[canvasID] == cancel {
				delete(r.runCancels, canvasID)
			}
			r.mu.Unlock()
		}()

		// Resume path: inject the previously-saved interrupt id and
		// the user's follow-up into root. The RunFunc reads these
		// keys and decorates ctx with compose.ResumeWithData before
		// invoking the workflow. The sentinel keys are deleted from
		// root inside the RunFunc — see service/agent.go's
		// buildRunFunc.
		if userInput != "" {
			if id := r.getInterruptID(canvasID, sessionID); id != "" {
				root["__resume_interrupt_id__"] = id
				root["__resume_data__"] = userInput
			}
		}

		state, runErr := safeInvoke(ctx, cancel, run, root)
		if runErr != nil {
			if errors.Is(runErr, context.Canceled) || errors.Is(runErr, errCancelled) {
				return
			}
			if ctxs := ExtractInterruptContexts(runErr); len(ctxs) > 0 {
				// Wait-for-user: persist the first interrupt id
				// and emit the SSE event.
				cpnID := FirstInterruptID(ctxs)
				r.saveInterruptID(canvasID, sessionID, cpnID)
				payload, _ := json.Marshal(WaitingForUserEvent{CpnID: cpnID})
				push(out, RunEvent{Type: "waiting_for_user", Data: string(payload)})
				return
			}
			if IsInterruptError(runErr) {
				// Raw InterruptSignal (no wrapped InterruptCtx list
				// available). Emit a generic waiting_for_user event
				// without a cpn id — the front-end falls back to
				// the first paused session it knows about.
				r.saveInterruptID(canvasID, sessionID, runErr.Error())
				payload, _ := json.Marshal(WaitingForUserEvent{CpnID: runErr.Error()})
				push(out, RunEvent{Type: "waiting_for_user", Data: string(payload)})
				return
			}
			pushErr(out, runErr.Error())
			return
		}

		// Normal completion. Extract the answer from the post-run
		// state. We walk state.Snapshot() looking for any cpn whose
		// output contains an "answer" / "result" key, and emit a
		// single MessageEvent carrying the value. The first
		// non-empty match wins.
		answer, reference := extractAnswerFromState(state)
		payload, _ := json.Marshal(MessageEvent{Answer: answer, Reference: reference})
		push(out, RunEvent{Type: "message", Data: string(payload)})
		push(out, RunEvent{Type: "done", Data: ""})
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
		state, err = run(ctx, root)
		close(done)
	}()
	select {
	case <-done:
		return state, err
	case <-cancel:
		return nil, errCancelled
	}
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
	payload, _ := json.Marshal(ErrorEvent{Message: msg})
	push(out, RunEvent{Type: "error", Data: string(payload)})
}

// extractAnswerFromState scans the post-run state for the
// surfaceable answer and any reference chunks. The walk is
// cpn-agnostic: it inspects every cpn's output map for an
// "answer", "result", or "content" key with a non-empty value.
//
// Precedence:
//  1. A cpn whose output has an "answer" key — that's the
//     "this cpn is the answer producer" marker Answer
//     components emit.
//  2. A cpn whose output has a "result" key with a string value
//     — the V1 service.RunAgent synthesises this when no full
//     canvas compile/invoke has run yet (see service/agent.go's
//     buildRunFunc).
//  3. The first non-empty "content" key.
//
// Reference is whatever the state carries under "reference" or
// "chunks" — front-ends use this to render citation links. V1
// state has no references yet; an empty slice is fine.
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
