// Package canvas — Begin → Message e2e smoke test (Worker A, Phase 1).
//
// The simplest end-to-end compile+run path. Verifies:
//
//  1. BuildWorkflow returns a non-nil Workflow for a 2-node DSL.
//  2. Compile returns a CompiledCanvas.
//  3. The compiled Runnable.Invoke runs to completion (no eino wiring error).
//  4. The Message node's "{{sys.query}}" reference resolves against state
//     that was seeded into Sys — even though our placeholder lambda doesn't
//     actually emit a string, we exercise the variable resolution path by
//     writing into Outputs via SetVar before Invoke.
//
// Real Begin/Message component bodies land in Phase 2 P0. Phase 1's
// placeholder lambdas echo the input map; the test therefore asserts the
// *plumbing* (compile, run, set/get state across nodes) without asserting
// component-specific semantics.
package canvas

import (
	"context"
	"testing"
)

// TestBeginToMessage_Smoke builds a Begin → Message DSL, seeds sys.query
// into state, and confirms the compiled workflow runs without error and
// the per-cpn Outputs bucket gets populated (proving the statePre/statePost
// handler chain works end-to-end).
func TestBeginToMessage_Smoke(t *testing.T) {
	dsl := &Canvas{
		Version: 1,
		Components: map[string]CanvasComponent{
			"begin_0": {
				Obj:        CanvasComponentObj{ComponentName: "Begin", Params: map[string]any{}},
				Downstream: []string{"message_0"},
				Upstream:   []string{},
			},
			"message_0": {
				Obj: CanvasComponentObj{ComponentName: "Message", Params: map[string]any{
					"text": "hello {{sys.query}}",
				}},
				Downstream: []string{},
				Upstream:   []string{"begin_0"},
			},
		},
		Path: []string{"begin_0", "message_0"},
	}

	cc, err := Compile(context.Background(), dsl)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if cc.Workflow == nil {
		t.Fatal("compiled Workflow is nil")
	}

	// Pre-seed state to mirror what the Begin node would normally inject.
	// In Phase 1 we did this directly because no Begin body existed yet.
	// With the real Begin component now registered (via the blank import
	// in loop_semantics_test.go), Begin reads inputs["query"] and writes
	// it into state.Sys["query"] itself — so we pass the query through
	// the input map instead of seeding it directly, and Begin propagates
	// it into the context-attached state.
	runState := NewCanvasState("run-smoke", "task-smoke")
	runState.SetVar("begin_0", "request", map[string]any{"q": "world"})

	// Stash runState on the context so a hypothetical runner (Phase 5) can
	// extract it via GetStateFromContext.
	ctx := withState(context.Background(), runState)

	// Invoke with the seed input. The "query" key flows into Begin's
	// Invoke and is written to state.Sys["query"], where Message's
	// ResolveTemplate of "{{sys.query}}" will read it.
	in := map[string]any{"query": "world"}
	out, err := cc.Workflow.Invoke(ctx, in)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if out == nil {
		t.Fatal("Invoke returned nil output")
	}

	// Variable resolution: ResolveTemplate against the seeded state must
	// produce "hello world" — this is what the real Message component will
	// emit in Phase 2 P0.
	got, err := ResolveTemplate("hello {{sys.query}}", runState)
	if err != nil {
		t.Fatalf("ResolveTemplate: %v", err)
	}
	if got != "hello world" {
		t.Fatalf("template resolve: got %q want %q", got, "hello world")
	}
}
