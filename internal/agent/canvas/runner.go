//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
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
//  2. harness interrupt (runErr is an *InterruptSignal or wrapped
//     variant): emit `waiting_for_user` with the first interrupt
//     id. Persist the id so the next call can resume via
//     compose.ResumeWithData (signalled through root:
//     __resume_interrupt_id__ + __resume_data__).
//  3. Cancel / timeout (errors.Is(err, context.Canceled) etc.):
//     silently close. The HTTP handler has already detached.
//  4. Other errors: emit `error` event with the err.Error() string.
//
// SSE wire contract (matches the handler envelope):
//   - RunEvent.Type == "message"     → {data: <string>}
//   - RunEvent.Type == "waiting_for_user" → {cpn_id: <string>}
//   - RunEvent.Type == "error"      → {message: <string>}
//   - RunEvent.Type == "done"       → final terminator frame
package canvas

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"go.uber.org/zap"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
)

// RunEvent is the unit the Runner pushes onto its output channel.
// The handler converts each RunEvent into one SSE frame
// (`data: {...}\n\n`). Type is the event tag; Data is the JSON
// payload (already serialised — handler does not re-marshal).
//
// SessionID / MessageID / TaskID are populated by the runner for
// the chatbot/agentbot completion paths (SSE envelope metadata).
// They are empty on the /agents/{canvas_id}/run path.
type RunEvent struct {
	Type      string
	Data      string
	SessionID string
	MessageID string
	TaskID    string
}

// MessageEvent is the JSON payload for Type=="message" frames.
// The Python agent API streams arbitrary strings; we mirror that
// shape so the existing front-end parses the events the same way.
type MessageEvent struct {
	Content      string        `json:"content,omitempty"`
	Reference    []interface{} `json:"reference,omitempty"`
	StartToThink bool          `json:"start_to_think,omitempty"`
	EndToThink   bool          `json:"end_to_think,omitempty"`
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
// DSL and invokes the harness graph; the Runner is agnostic to
// that machinery.
//
// Return contract:
//
//   - nil error, non-nil state: run completed normally.
//   - non-nil error that is a harness interrupt signal: the run paused
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
	interruptIDs map[string]string // key = canvasID + "|" + sessionID; value = harness interrupt id
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

// saveInterruptID stores the interrupt id for a (canvasID,
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
	canvasID, sessionID string,
	userInput any,
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
		// Panic recovery: a panic anywhere in the run goroutine used to silently
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

		// Create a progress channel so the RunFunc can stream content
		// chunks during the LLM call. The channel is passed through
		// root so the agent component can send to it.
		progCh := make(chan runtime.ProgEvent, 256)
		root[ProgressCh] = progCh

		// Emit workflow_started so the frontend log panel has context.
		push(out, RunEvent{Type: "workflow_started", Data: "{}"})

		// Emit node_started for every known component BEFORE the canvas
		// runs, so the log panel shows a spinning (waiting) indicator
		// while the LLM is generating. Component metadata is injected
		// into root by service.RunAgent under __comp_types__ /
		// __comp_names__; both are optional and safe to be absent.
		emitNodeStarted(out, root)

		// Start the canvas in a goroutine and stream progress chunks
		// while it runs.
		type runResult struct {
			state *CanvasState
			err   error
		}
		resultCh := make(chan runResult, 1)
		go func() {
			state, runErr := safeInvoke(ctx, cancel, run, root)
			resultCh <- runResult{state, runErr}
		}()

		// Simple approach: IsThink=true events open/keep thinking open.
		// IsThink=false events close thinking and go to main output.
		var state *CanvasState
		var runErr error
		var thinkActive bool
	loop:
		for {
			select {
			case pe, ok := <-progCh:
				if !ok || (pe.Text == "" && !pe.IsNodeEvent) {
					continue
				}
				if pe.IsNodeEvent {
					thoughts := ""
					nodeInputs := map[string]any{}
					nodeOutputs := map[string]any{}
					if pe.NodeEventType == "finished" {
						thoughts = pe.Thoughts
						if pe.NodeInputs != nil {
							nodeInputs = pe.NodeInputs
						}
						if pe.NodeOutputs != nil {
							nodeOutputs = pe.NodeOutputs
						}
					}
					eventType := "node_finished"
					if pe.NodeEventType == "started" {
						eventType = "node_started"
					}
					nodeData, _ := json.Marshal(map[string]any{
						"component_id":   pe.NodeCPNID,
						"component_name": pe.NodeDisplayName,
						"component_type": pe.NodeClassName,
						"inputs":         nodeInputs,
						"outputs":        nodeOutputs,
						"error":          nil,
						"elapsed_time":   0,
						"created_at":     time.Now().Unix(),
						"thoughts":       thoughts,
					})
					push(out, RunEvent{Type: eventType, Data: string(nodeData)})
					continue
				}
				msg := MessageEvent{Content: pe.Text}
				if pe.IsThink && !thinkActive {
					thinkActive = true
					msg.StartToThink = true
				} else if !pe.IsThink && thinkActive {
					thinkActive = false
					msg.EndToThink = true
				}
				payload, _ := json.Marshal(msg)
				push(out, RunEvent{Type: "message", Data: string(payload)})
			case res := <-resultCh:
				state, runErr = res.state, res.err
				break loop
			}
		}
	drain:
		for {
			select {
			case pe, ok := <-progCh:
				if !ok || (pe.Text == "" && !pe.IsNodeEvent) {
					continue
				}
				if pe.IsNodeEvent {
					thoughts := ""
					if pe.NodeEventType == "finished" {
						thoughts = pe.Thoughts
					}
					nodeData, _ := json.Marshal(map[string]any{
						"component_id":   pe.NodeCPNID,
						"component_name": pe.NodeDisplayName,
						"component_type": pe.NodeClassName,
						"inputs":         map[string]any{},
						"outputs":        map[string]any{},
						"error":          nil,
						"elapsed_time":   0,
						"created_at":     time.Now().Unix(),
						"thoughts":       thoughts,
					})
					eventType := "node_started"
					if pe.NodeEventType == "finished" {
						eventType = "node_finished"
					}
					push(out, RunEvent{Type: eventType, Data: string(nodeData)})
					continue
				}
				msg := MessageEvent{Content: pe.Text}
				if pe.IsThink && !thinkActive {
					thinkActive = true
					msg.StartToThink = true
				} else if !pe.IsThink && thinkActive {
					thinkActive = false
					msg.EndToThink = true
				}
				payload, _ := json.Marshal(msg)
				push(out, RunEvent{Type: "message", Data: string(payload)})
			default:
				break drain
			}
		}
		delete(root, ProgressCh)
		if runErr != nil {
			if errors.Is(runErr, context.Canceled) || errors.Is(runErr, errCancelled) {
				return
			}
			if ctxs := MustExtractInterruptContexts(runErr); len(ctxs) > 0 {
				cpnID := FirstInterruptID(ctxs)
				r.saveInterruptID(canvasID, sessionID, cpnID)
				common.Info("canvas runner interrupt",
					zap.String("canvas", canvasID),
					zap.String("session", sessionID),
					zap.String("cpn_id", cpnID))
				evt := WaitingForUserEvent{CpnID: cpnID}
				if ctxs[0].Tips != "" {
					evt.Tips = ctxs[0].Tips
				}
				if len(ctxs[0].Inputs) > 0 {
					evt.Inputs = ctxs[0].Inputs
				}
				payload, _ := json.Marshal(evt)
				push(out, RunEvent{Type: "waiting_for_user", Data: string(payload)})
				push(out, RunEvent{Type: "done", Data: ""})
				return
			}
			if IsInterruptError(runErr) {
				r.saveInterruptID(canvasID, sessionID, runErr.Error())
				payload, _ := json.Marshal(WaitingForUserEvent{CpnID: runErr.Error()})
				push(out, RunEvent{Type: "waiting_for_user", Data: string(payload)})
				push(out, RunEvent{Type: "done", Data: ""})
				return
			}
			pushErr(out, runErr.Error())
			return
		}

		if snap := state.Snapshot(); snap != nil {
			ctype, _ := root["__comp_types__"].(map[string]string)
			cname, _ := root["__comp_names__"].(map[string]string)
			now := time.Now().Unix()
			for cpnID, outputs := range snap {
				typeName := ctype[cpnID]
				displayName := cname[cpnID]
				if displayName == "" {
					displayName = cpnID
				}
				if outputs == nil {
					outputs = map[string]any{}
				}
				finishData, _ := json.Marshal(map[string]any{
					"component_id":   cpnID,
					"component_name": displayName,
					"component_type": typeName,
					"inputs":         map[string]any{},
					"outputs":        outputs,
					"error":          nil,
					"elapsed_time":   0,
					"created_at":     now,
					"thoughts":       "",
				})
				push(out, RunEvent{Type: "node_finished", Data: string(finishData)})
			}
		}

		// Redundant EndToThink event removed here. The final answer
		// streaming via streamContentToProgCh already closes the
		// thinking section (IsThink:false when thinkActive is true
		// triggers EndToThink).  An extra EndToThink with empty
		// Content would cause Content to be omitted by omitempty,
		// resulting in data.content===undefined in the front-end
		// and appending the string "undefined" to the answer.
		answer, reference := extractAnswerFromState(state)
		payload, _ := json.Marshal(MessageEvent{Content: answer, Reference: reference})
		push(out, RunEvent{Type: "message", Data: string(payload)})
		push(out, RunEvent{Type: "workflow_finished", Data: "{}"})
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

// push sends an event to the channel, dropping it if the consumer
// has gone away (handler cancelled). Errors on send are intentional
// and ignored — the handler is the only consumer and its
// `for-range` loop exits when the request context is cancelled.
func push(out chan<- RunEvent, ev RunEvent) {
	defer func() { _ = recover() }()
	out <- ev
}

// ProgressCh is the key used in root to pass a streaming-content
// channel from the RunFunc to the runner. The Runner reads from this
// channel during safeInvoke and emits incremental message events.
const ProgressCh = "__progress__"

// emitNodeStarted reads component metadata from root and emits a
// node_started RunEvent for every known component in the order
// specified by __comp_ids__ (a []string, populated by
// service.extractComponentInfo). Map iteration is NOT used because
// Go map iteration order is random.
func emitNodeStarted(out chan<- RunEvent, root map[string]any) {
	ctype, _ := root["__comp_types__"].(map[string]string)
	cname, _ := root["__comp_names__"].(map[string]string)
	ids, _ := root["__comp_ids__"].([]string)
	if len(ids) == 0 {
		// Fallback: if no ordered list, walk types map (unordered).
		for cpnID := range ctype {
			ids = append(ids, cpnID)
		}
	}
	for _, cpnID := range ids {
		displayName := cname[cpnID]
		if displayName == "" {
			displayName = cpnID
		}
		startData, _ := json.Marshal(map[string]any{
			"component_id":   cpnID,
			"component_name": displayName,
			"component_type": ctype[cpnID],
			"inputs":         map[string]any{},
			"outputs":        map[string]any{},
			"error":          nil,
			"elapsed_time":   0,
			"created_at":     time.Now().Unix(),
			"thoughts":       "",
		})
		push(out, RunEvent{Type: "node_started", Data: string(startData)})
	}
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
