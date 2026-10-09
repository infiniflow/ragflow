package canvas

import (
	"context"
	"strings"
	"testing"

	"ragflow/internal/agent/component"
	"ragflow/internal/agent/runtime"

	"gorm.io/gorm"
)

// recordingComponent returns a fixed output and keeps the input map it received.
type recordingComponent struct {
	out    map[string]any
	record *map[string]any
}

func (c *recordingComponent) Invoke(_ context.Context, _ *gorm.DB, in map[string]any) (map[string]any, error) {
	if c.record != nil {
		copied := make(map[string]any, len(in))
		for k, v := range in {
			copied[k] = v
		}
		*c.record = copied
	}
	out := make(map[string]any, len(c.out))
	for k, v := range c.out {
		out[k] = v
	}
	return out, nil
}

// TestMessageReadsPredecessorOutput runs Source → Message → Lookup → Message.
// Both messages must resolve a field from the node immediately before them.
// The lookup after the status message must not receive that status text.
func TestMessageReadsPredecessorOutput(t *testing.T) {
	var lookupInputs map[string]any
	factory := func(name string, params map[string]any) (runtime.Component, error) {
		switch strings.ToLower(name) {
		case "message":
			msg, err := component.NewMessageComponent(params)
			if err != nil {
				return nil, err
			}
			return msg, nil
		case "lookup":
			return &recordingComponent{
				out:    map[string]any{"answer": "found-it"},
				record: &lookupInputs,
			}, nil
		default:
			return &recordingComponent{out: map[string]any{"summary": "shipping delayed"}}, nil
		}
	}

	c := &Canvas{
		Components: map[string]CanvasComponent{
			"source_0": {
				Obj:        CanvasComponentObj{ComponentName: "Source", Params: map[string]any{}},
				Downstream: []string{"message_0"},
			},
			"message_0": {
				Obj: CanvasComponentObj{ComponentName: "Message", Params: map[string]any{
					"content": []any{"Checked {source_0@summary}."},
				}},
				Downstream: []string{"lookup_0"},
				Upstream:   []string{"source_0"},
			},
			"lookup_0": {
				Obj:        CanvasComponentObj{ComponentName: "Lookup", Params: map[string]any{}},
				Downstream: []string{"message_1"},
				Upstream:   []string{"message_0"},
			},
			"message_1": {
				Obj: CanvasComponentObj{ComponentName: "Message", Params: map[string]any{
					"content": []any{"Result: {lookup_0@answer}."},
				}},
				Upstream: []string{"lookup_0"},
			},
		},
	}

	ctx := WithComponentFactory(t.Context(), factory)
	runState := NewCanvasState("run-message", "task-message")
	ctx = withState(ctx, runState)
	cc, err := Compile(ctx, c)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if _, err = cc.Workflow.Invoke(ctx, map[string]any{"query": "where is the parcel"}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	status, err := runState.GetVar("message_0@content")
	if err != nil {
		t.Fatalf("status message: %v", err)
	}
	if status != "Checked shipping delayed." {
		t.Fatalf("status message = %#v, want the source summary", status)
	}
	final, err := runState.GetVar("message_1@content")
	if err != nil {
		t.Fatalf("final message: %v", err)
	}
	if final != "Result: found-it." {
		t.Fatalf("final message = %#v, want the lookup answer", final)
	}
	if lookupInputs["query"] != "where is the parcel" {
		t.Fatalf("lookup input = %#v, want the workflow query", lookupInputs)
	}
	if lookupInputs["content"] != nil {
		t.Fatalf("status text was copied into lookup: %#v", lookupInputs)
	}
}
