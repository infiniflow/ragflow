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
	"errors"
	"fmt"
	"strings"

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
// - First execution: build an inputSpec and call interrupt.Interrupt.
// The engine catches the GraphInterrupt, saves a checkpoint, and
// surfaces the error to the orchestrator.
// - Resumed execution: interrupt.GetResumeValues returns the user's
// input. Emit user_input and cpnID keys.
func UserFillUpNodeBody(cpnID string, params map[string]any) func(ctx context.Context, input any) (any, error) {
	inputSpec := BuildInputSpec(params)
	body := func(ctx context.Context, input any) (any, error) {
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
func MustExtractInterruptContexts(err error) []*interruptCtx {
	if err == nil {
		return nil
	}
	val, ok := interrupt.GetInterruptValue(err)
	if !ok {
		return nil
	}
	id := fmt.Sprintf("%v", val)
	if m, ok := val.(map[string]any); ok {
		if cpn, ok := m["cpn_id"]; ok {
			id = fmt.Sprintf("%v", cpn)
		}
	}
	return []*interruptCtx{{ID: id}}
}

// interruptCtx is a minimal substitute for InterruptCtx.
type interruptCtx struct {
	ID string
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
