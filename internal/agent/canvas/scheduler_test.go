// Package canvas — scheduler unit tests.
package canvas

import (
	"context"
	"strings"
	"testing"
)

// TestBuildWorkflow_3NodeLinear exercises a trivial Begin → LLM → Message
// chain. Verifies the workflow compiles and the runtime paths exist.
func TestBuildWorkflow_3NodeLinear(t *testing.T) {
	c := &Canvas{
		Components: map[string]CanvasComponent{
			"begin_0": {
				Obj:        CanvasComponentObj{ComponentName: "Begin", Params: map[string]any{}},
				Downstream: []string{"llm_0"},
				Upstream:   []string{},
			},
			"llm_0": {
				Obj:        CanvasComponentObj{ComponentName: "LLM", Params: map[string]any{"prompt": "hi"}},
				Downstream: []string{"message_0"},
				Upstream:   []string{"begin_0"},
			},
			"message_0": {
				Obj:        CanvasComponentObj{ComponentName: "Message", Params: map[string]any{}},
				Downstream: []string{},
				Upstream:   []string{"llm_0"},
			},
		},
		Path: []string{"begin_0", "llm_0", "message_0"},
	}

	wf, err := BuildWorkflow(context.Background(), c)
	if err != nil {
		t.Fatalf("BuildWorkflow: %v", err)
	}
	if wf == nil {
		t.Fatal("nil workflow")
	}

	// Compile to a Runnable to confirm the topology is internally consistent.
	cc, err := Compile(context.Background(), c)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if cc.Workflow == nil {
		t.Fatal("nil compiled workflow")
	}
}

// TestBuildWorkflow_5NodeDiamond exercises a diamond: A → B, A → C,
// B → D, C → D. The two parallel branches converge at D.
func TestBuildWorkflow_5NodeDiamond(t *testing.T) {
	c := &Canvas{
		Components: map[string]CanvasComponent{
			"begin_0": {
				Obj:        CanvasComponentObj{ComponentName: "Begin", Params: map[string]any{}},
				Downstream: []string{"a_0"},
				Upstream:   []string{},
			},
			"a_0": {
				Obj:        CanvasComponentObj{ComponentName: "Categorize", Params: map[string]any{}},
				Downstream: []string{"b_0", "c_0"},
				Upstream:   []string{"begin_0"},
			},
			"b_0": {
				Obj:        CanvasComponentObj{ComponentName: "LLM", Params: map[string]any{}},
				Downstream: []string{"d_0"},
				Upstream:   []string{"a_0"},
			},
			"c_0": {
				Obj:        CanvasComponentObj{ComponentName: "LLM", Params: map[string]any{}},
				Downstream: []string{"d_0"},
				Upstream:   []string{"a_0"},
			},
			"d_0": {
				Obj:        CanvasComponentObj{ComponentName: "Message", Params: map[string]any{}},
				Downstream: []string{},
				Upstream:   []string{"b_0", "c_0"},
			},
		},
		Path: []string{"begin_0", "a_0", "b_0", "c_0", "d_0"},
	}

	cc, err := Compile(context.Background(), c)
	if err != nil {
		t.Fatalf("Compile diamond: %v", err)
	}
	if cc.Workflow == nil {
		t.Fatal("nil compiled diamond workflow")
	}
}

// TestBuildWorkflow_ErrorsOnUnknownUpstream covers the "edge to unknown
// cpn" guard — a DSL bug should fail at compile-time, not silently skip.
func TestBuildWorkflow_ErrorsOnUnknownUpstream(t *testing.T) {
	c := &Canvas{
		Components: map[string]CanvasComponent{
			"begin_0": {
				Obj:        CanvasComponentObj{ComponentName: "Begin", Params: map[string]any{}},
				Downstream: []string{"message_0"},
				Upstream:   []string{},
			},
			"message_0": {
				Obj:        CanvasComponentObj{ComponentName: "Message", Params: map[string]any{}},
				Downstream: []string{},
				Upstream:   []string{"unknown_0"}, // <-- bad
			},
		},
	}
	_, err := BuildWorkflow(context.Background(), c)
	if err == nil {
		t.Fatal("expected error for unknown upstream")
	}
	if !strings.Contains(err.Error(), "unknown upstream") {
		t.Fatalf("expected 'unknown upstream' in error, got: %v", err)
	}
}

// TestBuildWorkflow_ErrorsOnSelfEdge catches the simplest DSL mistake.
func TestBuildWorkflow_ErrorsOnSelfEdge(t *testing.T) {
	c := &Canvas{
		Components: map[string]CanvasComponent{
			"a_0": {
				Obj:        CanvasComponentObj{ComponentName: "LLM", Params: map[string]any{}},
				Downstream: []string{},
				Upstream:   []string{"a_0"}, // <-- self
			},
		},
	}
	_, err := BuildWorkflow(context.Background(), c)
	if err == nil {
		t.Fatal("expected error for self-edge")
	}
	if !strings.Contains(err.Error(), "self-edge") {
		t.Fatalf("expected 'self-edge' in error, got: %v", err)
	}
}
