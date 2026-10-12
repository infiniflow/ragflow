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

func TestAgentRunThinkingUsesFinalAnswerNode(t *testing.T) {
	start := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	c := &canvas.Canvas{Components: map[string]canvas.CanvasComponent{
		"begin":     {Obj: canvas.CanvasComponentObj{ComponentName: "Begin"}, Downstream: []string{"Agent:A"}},
		"Agent:A":   {Obj: canvas.CanvasComponentObj{ComponentName: "Agent"}, Downstream: []string{"Agent:B"}},
		"Agent:B":   {Obj: canvas.CanvasComponentObj{ComponentName: "Agent"}, Downstream: []string{"Message:0"}},
		"Message:0": {Obj: canvas.CanvasComponentObj{ComponentName: "Message"}},
	}}
	for i := 0; i < 200; i++ {
		state := canvas.NewCanvasState("run", "session")
		state.SetVar("Agent:old", "thinking", "previous turn")
		state.SetVar("Agent:old", "_created_time", "2026-10-09T00:00:00Z")
		state.SetVar("Agent:A", "content", `{"rewrite":"q"}`)
		state.SetVar("Agent:A", "thinking", "A")
		state.SetVar("Agent:A", "_created_time", "2026-10-10T00:00:02Z")
		state.SetVar("Agent:B", "content", "final prose")
		state.SetVar("Agent:B", "thinking", "B")
		state.SetVar("Agent:B", "_created_time", "2026-10-10T00:00:03Z")
		state.SetVar("Message:0", "content", "final prose")
		state.SetVar("Message:0", "_created_time", "2026-10-10T00:00:04Z")
		id, output := selectTerminalCanvasOutput(c, state, nil, start)
		if output["content"] != "final prose" {
			t.Fatalf("run %d: answer = %#v, want final prose", i, output["content"])
		}
		if got := agentRunThinking(c, state, id, output, start); got != "B" {
			t.Fatalf("run %d: thinking = %q, want the final answer node's thinking", i, got)
		}
	}
}

func TestAgentRunThinkingPrefersSelectedOutput(t *testing.T) {
	c := &canvas.Canvas{Components: map[string]canvas.CanvasComponent{
		"Agent:A":   {Obj: canvas.CanvasComponentObj{ComponentName: "Agent"}, Downstream: []string{"Message:0"}},
		"Message:0": {Obj: canvas.CanvasComponentObj{ComponentName: "Message"}},
	}}
	state := canvas.NewCanvasState("run", "session")
	state.SetVar("Agent:A", "thinking", "A")
	state.SetVar("Agent:A", "_created_time", "2026-10-10T00:00:02Z")
	output := map[string]any{"content": "answer", "thinking": "selected"}
	if got := agentRunThinking(c, state, "Message:0", output, time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)); got != "selected" {
		t.Fatalf("thinking = %q, want selected", got)
	}
}

func TestAgentRunThinkingIgnoresParallelBranch(t *testing.T) {
	start := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	// begin fans out to two branches; Message:0 (selected) and Message:1 both finish.
	c := &canvas.Canvas{Components: map[string]canvas.CanvasComponent{
		"begin":     {Obj: canvas.CanvasComponentObj{ComponentName: "Begin"}, Downstream: []string{"Agent:A", "Agent:B"}},
		"Agent:A":   {Obj: canvas.CanvasComponentObj{ComponentName: "Agent"}, Downstream: []string{"Message:0"}},
		"Agent:B":   {Obj: canvas.CanvasComponentObj{ComponentName: "Agent"}, Downstream: []string{"Message:1"}},
		"Message:0": {Obj: canvas.CanvasComponentObj{ComponentName: "Message"}},
		"Message:1": {Obj: canvas.CanvasComponentObj{ComponentName: "Message"}},
	}}
	for i := 0; i < 200; i++ {
		state := canvas.NewCanvasState("run", "session")
		state.SetVar("Agent:A", "thinking", "A")
		state.SetVar("Agent:A", "_created_time", "2026-10-10T00:00:01Z")
		// Agent:B starts later, on the other branch.
		state.SetVar("Agent:B", "thinking", "B")
		state.SetVar("Agent:B", "_created_time", "2026-10-10T00:00:03Z")
		state.SetVar("Message:0", "content", "answer from A")
		state.SetVar("Message:0", "_created_time", "2026-10-10T00:00:02Z")
		state.SetVar("Message:1", "content", "answer from B")
		state.SetVar("Message:1", "_created_time", "2026-10-10T00:00:04Z")
		id, output := selectTerminalCanvasOutput(c, state, nil, start)
		if id != "Message:0" || output["content"] != "answer from A" {
			t.Fatalf("run %d: selected %q %#v, want Message:0", i, id, output["content"])
		}
		if got := agentRunThinking(c, state, id, output, start); got != "A" {
			t.Fatalf("run %d: thinking = %q, want A from the selected branch", i, got)
		}
	}
}

func TestAgentRunThinkingKeepsEmptyFinalAgentThinking(t *testing.T) {
	start := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	c := &canvas.Canvas{Components: map[string]canvas.CanvasComponent{
		"begin":     {Obj: canvas.CanvasComponentObj{ComponentName: "Begin"}, Downstream: []string{"Agent:A"}},
		"Agent:A":   {Obj: canvas.CanvasComponentObj{ComponentName: "Agent"}, Downstream: []string{"Agent:B"}},
		"Agent:B":   {Obj: canvas.CanvasComponentObj{ComponentName: "Agent"}, Downstream: []string{"Message:0"}},
		"Message:0": {Obj: canvas.CanvasComponentObj{ComponentName: "Message"}},
	}}
	state := canvas.NewCanvasState("run", "session")
	state.SetVar("Agent:A", "thinking", "A")
	state.SetVar("Agent:A", "_created_time", "2026-10-10T00:00:01Z")
	state.SetVar("Agent:B", "content", "final prose")
	state.SetVar("Agent:B", "thinking", "")
	state.SetVar("Agent:B", "_created_time", "2026-10-10T00:00:02Z")
	state.SetVar("Message:0", "content", "final prose")
	state.SetVar("Message:0", "_created_time", "2026-10-10T00:00:03Z")
	id, output := selectTerminalCanvasOutput(c, state, nil, start)
	if got := agentRunThinking(c, state, id, output, start); got != "" {
		t.Fatalf("thinking = %q, want empty (final Agent had no thinking)", got)
	}
}
