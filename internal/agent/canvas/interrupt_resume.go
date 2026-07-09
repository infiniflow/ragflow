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

// interrupt_resume.go — eino v0.9.8 interrupt/resume wrappers for the
// canvas layer.
//
// Background (plan §3): the previous "wait for user" mechanism was a
// sentinel chain (`__wait_for_user__` / `_user_input_provided`) that
// never actually connected end-to-end — UserFillUpComponent.Invoke did
// not emit `__wait_for_user__`, so the orchestrator's IsWaitForUser
// branch never fired. This file replaces the sentinel chain with eino's
// native interrupt/resume API:
//
//   - UserFillUpNodeBody — returns a node func that calls
//     compose.Interrupt on first execution and reads the user's input
//     via compose.GetResumeContext on resume.
//   - IsInterruptError / ExtractInterruptContexts — error-side helpers
//     used by the orchestrator Driver to detect a wait-for-user signal
//     and forward it as a `waiting_for_user` SSE event.
//   - BuildInputSpec — extracts the UserFillUp form-field definition
//     from DSL params; this is what we attach to compose.Interrupt's
//     `info` argument so the orchestrator can surface the form schema
//     to the front-end.
//
// v0.9.8 API surface used here (file-level diff against v0.9.5 verified
// identical for these signatures):
//
//	compose.Interrupt(ctx, info) error
//	compose.GetResumeContext[T any](ctx) (isResumeFlow, hasData bool, data T)
//	compose.ResumeWithData(ctx, interruptID, data) context.Context
//	compose.ExtractInterruptInfo(err) (*InterruptInfo, bool)
//	compose.WithCheckPointID(checkPointID) Option
//	compose.WithInterruptBeforeNodes(nodes) GraphCompileOption
package canvas

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/compose"
)

// BuildInputSpec turns the DSL's UserFillUp params into the user-visible
// info payload that travels with the interrupt signal. The orchestrator
// Driver reads this from InterruptCtx.Info on the SSE side and ships it
// to the front-end so the form renderer knows what fields to render.
//
// We deliberately keep the schema tiny: enable_tips + tips + an
// `inputs` map for the field definitions. Anything richer would couple
// the canvas layer to the component package, which is forbidden (the
// component package already knows the UserFillUp shape — it owns the
// form-field schema in userfillup.go; this function only carries the
// minimum the orchestrator needs to round-trip the form schema without
// re-reading the DSL).
func BuildInputSpec(params map[string]any) map[string]any {
	spec := make(map[string]any, 4)
	if params != nil {
		if v, ok := params["inputs"]; ok {
			spec["inputs"] = v
		}
		if v, ok := params["enable_tips"]; ok {
			spec["enable_tips"] = v
		}
		if v, ok := params["tips"]; ok {
			spec["tips"] = v
		}
	}
	spec["kind"] = "user_fill_up" // tag so cancel-vs-wait can be distinguished in Driver
	return spec
}

// UserFillUpNodeBody returns an eino node function implementing
// "wait for user input" semantics.
//
// Flow:
//
//   - First execution (no resume context): build an inputSpec and call
//     compose.Interrupt, returning the resulting error. The engine
//     catches the interrupt signal, persists a checkpoint, and surfaces
//     the error to the orchestrator (which renders it as a
//     `waiting_for_user` SSE event).
//   - Resumed execution: compose.GetResumeContext returns
//     (true, true, userInput). We emit two output keys: `user_input`
//     (the canonical v1 form-fill output name, mirroring the Python
//     fillup.py:66 contract) and the cpnID key (so downstream nodes can
//     reference `{{user_fill_up_1}}`).
//
// Idempotency: the resume branch is the very first thing the node does.
// Anything we did before the Interrupt call on the first run (we did
// nothing — no LLM calls, no file writes) cannot be repeated. The
// "node re-execution from start" risk called out in the plan §5 row 1
// is therefore a non-issue for UserFillUpNodeBody specifically.
func UserFillUpNodeBody(cpnID string, params map[string]any) func(ctx context.Context, input map[string]any) (map[string]any, error) {
	inputSpec := BuildInputSpec(params)
	body := func(ctx context.Context, input map[string]any) (map[string]any, error) {
		// Resume branch: the orchestrator decorated ctx with
		// compose.ResumeWithData(ctx, interruptID, userInput).
		// isResumeFlow is true when THIS node is the explicit target;
		// hasData is true when the caller supplied non-nil resume data.
		if isResume, hasData, data := compose.GetResumeContext[any](ctx); isResume && hasData {
			out := buildUserFillUpResumeOutput(cpnID, inputSpec, data)
			out["__cpn_id__"] = cpnID
			return out, nil
		}

		// Initial-run fast path: match the legacy Python canvas behavior
		// where Begin/UserFillUp consume the current run's inputs
		// directly. We only auto-consume when the node declares form
		// fields; plain wait-for-user nodes (no inputs schema) still
		// interrupt on first execution.
		if data, ok := initialUserFillUpData(ctx, inputSpec); ok {
			out := buildUserFillUpResumeOutput(cpnID, inputSpec, data)
			out["__cpn_id__"] = cpnID
			return out, nil
		}

		// First-call branch: emit the interrupt signal. The returned
		// error implements error; eino's runner catches it, persists a
		// checkpoint, and bubbles it up.
		if err := compose.Interrupt(ctx, inputSpec); err != nil {
			return nil, err
		}

		// Unreachable on a healthy eino runner — Interrupt either
		// returns an interrupt error or panics on engine misuse. Keep
		// the guard so test runs without a runner surface a clear
		// message rather than a panic.
		return nil, fmt.Errorf("canvas: UserFillUp %q: interrupt did not halt execution", cpnID)
	}
	return body
}

func buildUserFillUpResumeOutput(cpnID string, inputSpec map[string]any, data any) map[string]any {
	out := map[string]any{
		"user_input": data,
		cpnID:        data,
	}

	fields, _ := inputSpec["inputs"].(map[string]any)
	if _, hasValue := fields["value"]; hasValue {
		out["value"] = data
	}
	if len(fields) == 1 {
		for name := range fields {
			out[name] = data
		}
		return out
	}

	if values, ok := data.(map[string]any); ok {
		for name := range fields {
			if v, exists := values[name]; exists {
				out[name] = v
			}
		}
	}
	return out
}

func initialUserFillUpData(ctx context.Context, inputSpec map[string]any) (any, bool) {
	fields, _ := inputSpec["inputs"].(map[string]any)
	if len(fields) == 0 {
		return nil, false
	}

	state, _, err := GetStateFromContext[*CanvasState](ctx)
	if err != nil || state == nil {
		return nil, false
	}
	if consumed, _ := state.Sys["__initial_user_input_consumed__"].(bool); consumed {
		return nil, false
	}
	raw, err := state.GetVar("sys.query")
	if err != nil || raw == nil {
		return nil, false
	}
	if values, ok := raw.(map[string]any); ok {
		state.Sys["__initial_user_input_consumed__"] = true
		return values, true
	}
	text, ok := raw.(string)
	if !ok || text == "" {
		return nil, false
	}
	state.Sys["__initial_user_input_consumed__"] = true
	return text, true
}

// IsInterruptError reports whether err carries an eino interrupt signal.
//
// Used by the orchestrator Driver to distinguish wait-for-user from
// genuine run failures. context.Canceled / context.DeadlineExceeded
// are explicitly excluded so cancel-timeout paths don't trigger
// `waiting_for_user` events.
//
// Two detection paths cover the surface:
//   - compose.ExtractInterruptInfo matches wrapped forms
//     (`*interruptError` / `*subGraphInterruptError`) — the shapes
//     the eino runner returns after propagating through the engine.
//   - compose.IsInterruptRerunError matches the raw `*core.InterruptSignal`
//     returned by a direct `compose.Interrupt(...)` call. Useful in
//     unit tests that exercise the helper without spinning up a runner.
func IsInterruptError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if _, ok := compose.ExtractInterruptInfo(err); ok {
		return true
	}
	if _, ok := compose.IsInterruptRerunError(err); ok {
		return true
	}
	return false
}

// ExtractInterruptContexts walks the error chain and returns every
// InterruptCtx the engine surfaced. Returns nil if err is not an
// interrupt error.
//
// This handles two wrapping cases that come up in practice:
//
//  1. workflowx.AddLoopNode wraps sub-workflow interrupts as
//     ErrLoopSubGraphInterrupted (workflowx/loop.go:122-126). The
//     original interrupt error is reachable via errors.As/Is.
//  2. Composite interrupts (ToolsNode, parallel branches) carry a
//     list of nested InterruptCtx — we flatten them so the orchestrator
//     sees a single flat list to pick a target from.
//  3. Raw `*core.InterruptSignal` (the form `compose.Interrupt`
//     returns directly) — handled here so unit tests don't need a
//     full runner. The engine wraps this into `*interruptError` at
//     propagation time, so the wrapped path is the production one.
//
// Single-interrupt vs composite: a plain UserFillUp produces one
// context. The orchestrator currently uses the first; a future phase
// that wants multi-target resume would iterate.
func ExtractInterruptContexts(err error) []*compose.InterruptCtx {
	if err == nil {
		return nil
	}
	if info, ok := extractInterruptInfoDeep(err); ok && info != nil {
		ctxs := collectInterruptContexts(info)
		if len(ctxs) > 0 {
			return ctxs
		}
	}
	// Fallback: raw signal. Use the deprecated IsInterruptRerunError
	// helper which gives us (info, state, ok). We don't have access
	// to InterruptCtx here in the raw form (the engine hasn't wrapped
	// the signal yet), so we return nil — callers that care about
	// the context list rely on the wrapped form, which is what
	// production paths see.
	if _, ok := compose.IsInterruptRerunError(err); ok {
		return nil
	}
	return nil
}

func extractInterruptInfoDeep(err error) (*compose.InterruptInfo, bool) {
	if err == nil {
		return nil, false
	}
	if info, ok := compose.ExtractInterruptInfo(err); ok {
		return info, true
	}
	type multiUnwrapper interface {
		Unwrap() []error
	}
	if mw, ok := err.(multiUnwrapper); ok {
		for _, sub := range mw.Unwrap() {
			if info, ok := extractInterruptInfoDeep(sub); ok {
				return info, true
			}
		}
	}
	if unwrapped := errors.Unwrap(err); unwrapped != nil {
		return extractInterruptInfoDeep(unwrapped)
	}
	return nil, false
}

func collectInterruptContexts(info *compose.InterruptInfo) []*compose.InterruptCtx {
	if info == nil {
		return nil
	}
	var out []*compose.InterruptCtx
	out = append(out, info.InterruptContexts...)
	for _, sub := range info.SubGraphs {
		out = append(out, collectInterruptContexts(sub)...)
	}
	return out
}

// FirstInterruptID is a tiny convenience used by the Driver when it
// picks a single target for the SSE `cpn_id` field. Returns "" when
// no contexts are present. Keeps the Driver code from doing its own
// nil-check dance.
func FirstInterruptID(ctxs []*compose.InterruptCtx) string {
	if ctx := FirstUserFillUpInterrupt(ctxs); ctx != nil {
		return ctx.ID
	}
	if len(ctxs) == 0 {
		return ""
	}
	return ctxs[0].ID
}

// RootInterruptID returns the interrupt id that should be passed to
// compose.ResumeWithData. In composite/subgraph cases this is the
// root-cause context, which is not necessarily the same leaf context we
// want to expose to the front-end as the waiting UserFillUp node.
func RootInterruptID(ctxs []*compose.InterruptCtx) string {
	for _, ctx := range ctxs {
		for cur := ctx; cur != nil; cur = cur.Parent {
			if cur.IsRootCause {
				return cur.ID
			}
		}
	}
	if len(ctxs) == 0 {
		return ""
	}
	return ctxs[0].ID
}

func FirstUserFillUpInterrupt(ctxs []*compose.InterruptCtx) *compose.InterruptCtx {
	for _, ctx := range ctxs {
		for cur := ctx; cur != nil; cur = cur.Parent {
			if info, ok := cur.Info.(map[string]any); ok {
				if kind, _ := info["kind"].(string); kind == "user_fill_up" {
					return cur
				}
			}
		}
	}
	return nil
}

func formatInterruptContexts(ctxs []*compose.InterruptCtx) string {
	if len(ctxs) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(ctxs))
	for _, ctx := range ctxs {
		if ctx == nil {
			parts = append(parts, "<nil>")
			continue
		}
		kind := ""
		if info, ok := ctx.Info.(map[string]any); ok {
			kind, _ = info["kind"].(string)
		}
		addr := ctx.Address.String()
		parentAddr := ""
		if ctx.Parent != nil {
			parentAddr = ctx.Parent.Address.String()
		}
		if kind != "" {
			parts = append(parts, fmt.Sprintf("{id:%q kind:%q addr:%q parent:%q}", ctx.ID, kind, addr, parentAddr))
		} else {
			parts = append(parts, fmt.Sprintf("{id:%q info:%T addr:%q parent:%q}", ctx.ID, ctx.Info, addr, parentAddr))
		}
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// AutoDiscoverUserFillUpIDs returns the cpnIDs of every component whose
// name (case-insensitive) is UserFillUp. The compiler option
// compose.WithInterruptBeforeNodes needs a []string; we compute it
// here so callers don't have to walk the Canvas twice.
//
// Centralised here (rather than inlined in compile.go) so any future
// interrupt-emitting component (e.g. Answer, when ported) can register
// itself by adding to the switch.
func AutoDiscoverUserFillUpIDs(c *Canvas) []string {
	if c == nil {
		return nil
	}
	var ids []string
	for cpnID, comp := range c.Components {
		name := strings.ToLower(comp.Obj.ComponentName)
		switch name {
		case "userfillup":
			ids = append(ids, cpnID)
		}
	}
	return ids
}
