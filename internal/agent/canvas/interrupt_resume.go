// interrupt_resume.go — harness interrupt/resume wrappers for the
// canvas layer.
//
// Replaces compose.Interrupt / GetResumeContext with
// harness interrupt.Interrupt / GetResumeValues.
//
// UserFillUpNodeBody — returns a node func that calls
// interrupt.Interrupt on first execution and reads the user input
// via interrupt.GetResumeValues on resume.
//
// IsInterruptError — detection used by the orchestrator Driver to
// distinguish wait-for-user from genuine run failures.
package canvas

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/harness/graph/interrupt"
)

// BuildInputSpec turns the DSL UserFillUp params into the user-visible
// info payload that travels with the interrupt signal.
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
	spec["kind"] = "user_fill_up"
	return spec
}

// UserFillUpNodeBody returns a harness node function implementing
// "wait for user input" semantics.
//
// Flow:
// - If the UserFillUp has an options-type input (menu/dropdown) and the
// initial query is available in state.Sys["query"], consume the query
// directly without interrupting (matching Python's behavior where the
// first interaction node receives the initial message upfront).
// - Otherwise, build an inputSpec and call interrupt.Interrupt.
// The engine catches the GraphInterrupt, saves a checkpoint, and
// surfaces the error to the orchestrator.
// - Resumed execution: interrupt.GetResumeValues returns the user's
// input. Emit user_input and cpnID keys.
func UserFillUpNodeBody(cpnID string, params map[string]any) func(ctx context.Context, input any) (any, error) {
	inputSpec := BuildInputSpec(params)

	// Detect if this is a "dispatch" UserFillUp with an options-type input.
	// These (like UserFillUp:Menu) should consume the initial query without
	// interrupting.  Other UserFillUp nodes (text input, file upload) always
	// interrupt on first call.
	var dispatchKey string
	if rawInputs, ok := inputSpec["inputs"].(map[string]any); ok {
		for k, raw := range rawInputs {
			spec, _ := raw.(map[string]any)
			if spec == nil {
				continue
			}
			typ, _ := spec["type"].(string)
			if typ == "options" || typ == "choice" {
				dispatchKey = k
				break
			}
		}
	}

	body := func(ctx context.Context, input any) (any, error) {
		// Dispatch branch: consume initial query as the options input value.
		if dispatchKey != "" {
			if state, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx); err == nil && state != nil {
				if query, ok := state.Sys["query"].(string); ok && query != "" {
					return map[string]any{
						"user_input": query,
						dispatchKey:  query,
						cpnID:        query,
						"__cpn_id__": cpnID,
					}, nil
				}
			}
		}

		// First-call branch: emit the interrupt signal.
		// On resume, interrupt.Interrupt returns the resume value
		// with a nil error. On first call, it returns a GraphInterrupt error.
		resumeValue, err := interrupt.Interrupt(ctx, map[string]any{
			"cpn_id": cpnID,
			"spec":   inputSpec,
		})
		if err != nil {
			return nil, err
		}

		// Resume branch: resumeValue is the user's input.
		if resumeValue != nil {
			return map[string]any{
				"user_input": resumeValue,
				cpnID:        resumeValue,
				"__cpn_id__": cpnID,
			}, nil
		}

		return nil, fmt.Errorf("canvas: UserFillUp %q: interrupt did not halt execution", cpnID)
	}
	return body
}

// IsInterruptError reports whether err carries a harness interrupt signal.
// context.Canceled / context.DeadlineExceeded are explicitly excluded.
func IsInterruptError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	return interrupt.IsInterrupt(err)
}

// MustExtractInterruptContexts extracts interrupt info from an error
// for the orchestrator. Returns the first non-nil interrupt value as a
// simplified context list so the Driver can emit a `waiting_for_user`
// event.
//
// Handles both direct UserFillUp interrupt values and loop-wrapped
// interrupts (where the loop saves the UserFillUp value under
// "user_fill_up_value" in its JSON-serialised loopInterruptState).
func MustExtractInterruptContexts(err error) []*interruptCtx {
	if err == nil {
		return nil
	}
	val, ok := interrupt.GetInterruptValue(err)
	if !ok {
		return nil
	}

	// Resolve the effective interrupt value, unwrapping loop state
	// when present.
	effectiveVal := val
	if raw, ok := val.([]byte); ok {
		// Try JSON: loopInterruptState or direct map.
		var candidate map[string]any
		if json.Unmarshal(raw, &candidate) == nil {
			if ufv, has := candidate["user_fill_up_value"]; has {
				// Loop-wrapped: extract the original UserFillUp value.
				switch v := ufv.(type) {
				case string:
					if err := json.Unmarshal([]byte(v), &candidate); err != nil {
						// Not JSON — leave candidate as-is (parent loop state).
					}
				case map[string]any:
					candidate = v
				}
			}
			effectiveVal = candidate
		}
	}
	if m, ok := effectiveVal.(map[string]any); ok {
		id, _ := m["cpn_id"].(string)
		tips, inputs := extractSpecDetails(m)
		return []*interruptCtx{{ID: id, Tips: tips, Inputs: inputs}}
	}
	id := fmt.Sprintf("%v", effectiveVal)
	return []*interruptCtx{{ID: id}}
}

// extractSpecDetails reads the "spec" fields from a UserFillUp interrupt value.
func extractSpecDetails(val map[string]any) (tips string, inputs map[string]any) {
	spec, _ := val["spec"].(map[string]any)
	if spec == nil {
		return "", nil
	}
	if t, _ := spec["tips"].(string); t != "" {
		tips = t
	}
	if raw, _ := spec["inputs"].(map[string]any); len(raw) > 0 {
		inputs = raw
	}
	return
}

// interruptCtx is a minimal substitute for InterruptCtx.
type interruptCtx struct {
	ID     string
	Tips   string
	Inputs map[string]any
}

// FirstInterruptID returns the ID of the first interrupt context.
func FirstInterruptID(ctxs []*interruptCtx) string {
	if len(ctxs) == 0 {
		return ""
	}
	return ctxs[0].ID
}

// AutoDiscoverUserFillUpIDs returns the cpnIDs of every component whose
// name (case-insensitive) is UserFillUp.
func AutoDiscoverUserFillUpIDs(c *Canvas) []string {
	if c == nil {
		return nil
	}
	var ids []string
	for cpnID, comp := range c.Components {
		if strings.EqualFold(comp.Obj.ComponentName, "userfillup") {
			ids = append(ids, cpnID)
		}
	}
	return ids
}
