package canvas

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/cloudwego/eino/compose"
)

func TestLoopResumePreservesLargeVariable(t *testing.T) {
	const searchID = "11099848653006015581"
	c := &Canvas{
		Components: map[string]CanvasComponent{
			"loop": {Obj: CanvasComponentObj{ComponentName: "Loop", Params: map[string]any{
				"maximum_loop_count": 1,
				"loop_variables":     []any{map[string]any{"variable": "saved", "input_mode": "variable", "value": "search_0@id", "type": "number"}},
			}}},
			"input": {Obj: CanvasComponentObj{ComponentName: "UserFillUp", Params: map[string]any{
				"inputs": map[string]any{"value": map[string]any{"type": "line"}},
			}}, Upstream: []string{"loop"}, Downstream: []string{"consumer"}},
			"consumer": {Obj: CanvasComponentObj{ComponentName: "StringTransform", Params: map[string]any{
				"method": "merge", "script": "{{loop@saved}}:{{input@value}}", "delimiters": []any{"|"},
			}}, Upstream: []string{"input"}},
		},
		NodeParents: map[string]string{"input": "loop", "consumer": "loop"},
	}
	store, _ := newTestStore(t, time.Minute)
	compiled, err := Compile(t.Context(), c, WithCheckPointStore(store))
	if err != nil {
		t.Fatal(err)
	}
	newContext := func() context.Context { return WithState(t.Context(), NewCanvasState("run-loop", "session-loop")) }
	ctx := newContext()
	state, _ := GetStateFromContext(ctx)
	state.SetVar("search_0", "id", json.Number(searchID))
	const checkpoint = "loop-large-id"
	_, err = compiled.Workflow.Invoke(ctx, map[string]any{}, compose.WithCheckPointID(checkpoint))
	if !IsInterruptError(err) {
		t.Fatalf("first run: want interrupt, got %v", err)
	}
	interruptID := FirstInterruptID(ExtractInterruptContexts(err))
	if interruptID == "" {
		t.Fatal("missing interrupt ID")
	}
	resumed := compose.ResumeWithData(newContext(), interruptID, "answer")
	out, err := compiled.Workflow.Invoke(resumed, map[string]any{}, compose.WithCheckPointID(checkpoint))
	if err != nil {
		t.Fatal(err)
	}
	// The consumer reads the loop snapshot after restore, not just the outer canvas state.
	body, _ := out["loop"].(map[string]any)
	if body["result"] != searchID+":answer" {
		t.Fatalf("loop result = %#v, want %s:answer", out, searchID)
	}
}
