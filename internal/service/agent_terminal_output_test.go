package service

import (
	"reflect"
	"testing"
	"time"

	"ragflow/internal/agent/canvas"
)

func TestTerminalCanvasOutputExcludesLoopBody(t *testing.T) {
	c := &canvas.Canvas{
		Components: map[string]canvas.CanvasComponent{
			"Loop:x":       {Obj: canvas.CanvasComponentObj{ComponentName: "Loop"}, Downstream: []string{"Message:done"}},
			"ExitLoop:x":   {Obj: canvas.CanvasComponentObj{ComponentName: "ExitLoop"}},
			"Message:body": {Obj: canvas.CanvasComponentObj{ComponentName: "Message"}},
			"Message:done": {Obj: canvas.CanvasComponentObj{ComponentName: "Message"}},
		},
		NodeParents: map[string]string{"ExitLoop:x": "Loop:x", "Message:body": "Loop:x"},
	}
	state := canvas.NewCanvasState("run", "session")
	state.SetVar("ExitLoop:x", "_next", []string{"ExitLoop:x"})
	state.SetVar("Message:body", "content", "continue")
	state.SetVar("Message:body", "_created_time", "2026-10-10T00:00:02Z")
	state.SetVar("Message:done", "content", "finished")
	state.SetVar("Message:done", "_created_time", "2026-10-10T00:00:01Z")
	got := terminalCanvasOutput(c, state, nil, time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC))
	if got["content"] != "finished" {
		t.Fatalf("output = %#v, want the outer terminal message", got)
	}
}

func TestTerminalCanvasOutputKeepsCurrentPartialMessage(t *testing.T) {
	c := &canvas.Canvas{Components: map[string]canvas.CanvasComponent{
		"Message:old":     {Obj: canvas.CanvasComponentObj{ComponentName: "Message"}, Downstream: []string{"input"}},
		"Message:current": {Obj: canvas.CanvasComponentObj{ComponentName: "Message"}, Downstream: []string{"input"}},
		"input":           {Obj: canvas.CanvasComponentObj{ComponentName: "UserFillUp"}},
	}}
	state := canvas.NewCanvasState("run", "session")
	state.SetVar("Message:old", "content", "previous prompt")
	state.SetVar("Message:old", "_created_time", "2026-10-09T00:00:00Z")
	state.SetVar("Message:current", "content", "current prompt")
	state.SetVar("Message:current", "_created_time", "2026-10-10T00:00:01Z")
	got := terminalCanvasOutput(c, state, nil, time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC))
	if got["content"] != "current prompt" {
		t.Fatalf("output = %#v, want the current partial message", got)
	}
}

func TestTerminalCanvasOutputUsesCurrentWorkflowBranch(t *testing.T) {
	c := &canvas.Canvas{Components: map[string]canvas.CanvasComponent{
		"Message:a": {Obj: canvas.CanvasComponentObj{ComponentName: "Message"}},
		"Message:b": {Obj: canvas.CanvasComponentObj{ComponentName: "Message"}},
	}}
	state := canvas.NewCanvasState("run", "session")
	state.SetVar("Message:a", "content", "old branch")
	for _, output := range []map[string]any{
		{"content": "current branch"},
		{"content": "", "downloads": []any{map[string]any{"url": "/file"}}},
		{"content": ""},
	} {
		got := terminalCanvasOutput(c, state, map[string]any{"Message:b": output}, time.Now())
		if !reflect.DeepEqual(got, output) {
			t.Fatalf("output = %#v, want %#v", got, output)
		}
	}
}

func TestTerminalCanvasOutputDoesNotReplayPreviousTurn(t *testing.T) {
	c := &canvas.Canvas{Components: map[string]canvas.CanvasComponent{
		"Message:a": {Obj: canvas.CanvasComponentObj{ComponentName: "Message"}},
	}}
	state := canvas.NewCanvasState("run", "session")
	state.SetVar("Message:a", "content", "previous answer")
	state.SetVar("Message:a", "_created_time", "2026-10-09T00:00:00Z")
	got := terminalCanvasOutput(c, state, nil, time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC))
	if len(got) != 0 {
		t.Fatalf("output = %#v, want no output from the previous turn", got)
	}
}
