// multibranch_test.go — unit tests for wireMultiBranches and its
// internal condition logic.
package canvas

import (
	"context"
	"testing"

	graphpkg "ragflow/internal/harness/graph/graph"
)

// makeTestSwitchCond returns a Branch condition function that reads
// the "state" map's cpnID entry and extracts "_next".
// This matches the logic inside wireMultiBranches.
func makeTestSwitchCond(cpnID string, endNodes map[string]bool) func(ctx context.Context, state any) (any, error) {
	return func(ctx context.Context, state any) (any, error) {
		st, ok := state.(map[string]any)
		if !ok {
			return "", nil
		}
		stateVal, _ := st["state"].(map[string]map[string]any)
		if stateVal == nil {
			return "", nil
		}
		parentOut, _ := stateVal[cpnID]
		if parentOut == nil {
			return "", nil
		}
		next, ok := parentOut["_next"].(string)
		if !ok || next == "" || !endNodes[next] {
			return "", nil
		}
		return next, nil
	}
}

// TestMakeSwitchBranchCondition_MissingField: when `_next` is absent
// from the parent's output, the condition returns "" so the branch
// sees no chosen end-node and skips routing.
func TestMakeSwitchBranchCondition_MissingField(t *testing.T) {
	cond := makeTestSwitchCond("sw", map[string]bool{"a": true})
	got, err := cond(context.Background(), map[string]any{"other": "x"})
	if err != nil {
		t.Fatalf("cond: %v", err)
	}
	if got != "" {
		t.Errorf("cond on missing _next = %q, want \"\"", got)
	}
}

// TestMakeSwitchBranchCondition_EmptyString: `_next: ""` is treated
// the same as missing.
func TestMakeSwitchBranchCondition_EmptyString(t *testing.T) {
	cond := makeTestSwitchCond("sw", map[string]bool{"a": true})
	got, err := cond(context.Background(), map[string]any{"_next": ""})
	if err != nil {
		t.Fatalf("cond: %v", err)
	}
	if got != "" {
		t.Errorf("cond on empty _next = %q, want \"\"", got)
	}
}

// TestMakeSwitchBranchCondition_WrongType: a non-string `_next`
// value returns "".
func TestMakeSwitchBranchCondition_WrongType(t *testing.T) {
	cond := makeTestSwitchCond("sw", map[string]bool{"a": true})
	got, err := cond(context.Background(), map[string]any{"_next": []string{"a"}})
	if err != nil {
		t.Fatalf("cond: %v", err)
	}
	if got != "" {
		t.Errorf("cond on non-string _next = %q, want \"\"", got)
	}
}

// TestMakeSwitchBranchCondition_UnknownKey: `_next` resolves to a
// cpn_id not in the end-nodes whitelist. We must return "" so the
// harness branch does not emit an unintended end-node.
func TestMakeSwitchBranchCondition_UnknownKey(t *testing.T) {
	cond := makeTestSwitchCond("sw", map[string]bool{"a": true, "b": true})
	got, err := cond(context.Background(), map[string]any{"_next": "ghost"})
	if err != nil {
		t.Fatalf("cond: %v", err)
	}
	if got != "" {
		t.Errorf("cond on unknown _next = %q, want \"\"", got)
	}
}

// TestMakeSwitchBranchCondition_Valid: _next matches an end-node.
func TestMakeSwitchBranchCondition_Valid(t *testing.T) {
	cond := makeTestSwitchCond("sw", map[string]bool{"a": true, "b": true})
	got, err := cond(context.Background(), map[string]any{
		"state": map[string]map[string]any{
			"sw": {"_next": "b"},
		},
	})
	if err != nil {
		t.Fatalf("cond: %v", err)
	}
	if got != "b" {
		t.Errorf("cond = %q, want \"b\"", got)
	}
}

// TestWireMultiBranches_EmptyCanvas does not panic and returns nothing.
func TestWireMultiBranches_EmptyCanvas(t *testing.T) {
	sg := graphpkg.NewStateGraph(map[string]any{})
	wireMultiBranches(sg, nil, nil)
}

// TestWireMultiBranches_SingleChildSkipped: a Switch with only one
// downstream child is degenerate — branch is meaningless. The
// helper should skip it.
func TestWireMultiBranches_SingleChildSkipped(t *testing.T) {
	sg := graphpkg.NewStateGraph(map[string]any{})
	c := &Canvas{
		Components: map[string]CanvasComponent{
			"sw": {
				Obj:        CanvasComponentObj{ComponentName: "Switch"},
				Downstream: []string{"a"},
			},
			"a": {Obj: CanvasComponentObj{ComponentName: "LLM"}},
		},
	}
	wireMultiBranches(sg, c, nil)
}

// TestWireMultiBranches_TwoChildren: a Switch with two downstream
// children should install one branch.
func TestWireMultiBranches_TwoChildren(t *testing.T) {
	sg := graphpkg.NewStateGraph(map[string]any{})
	c := &Canvas{
		Components: map[string]CanvasComponent{
			"sw": {
				Obj:        CanvasComponentObj{ComponentName: "Switch"},
				Downstream: []string{"a", "b"},
			},
			"a": {Obj: CanvasComponentObj{ComponentName: "LLM"}},
			"b": {Obj: CanvasComponentObj{ComponentName: "Message"}},
		},
	}
	wireMultiBranches(sg, c, nil)
}

// TestWireMultiBranches_CategorizeChildren: Categorize with multiple
// children should get a branch just like Switch.
func TestWireMultiBranches_CategorizeChildren(t *testing.T) {
	sg := graphpkg.NewStateGraph(map[string]any{})
	c := &Canvas{
		Components: map[string]CanvasComponent{
			"cat": {
				Obj:        CanvasComponentObj{ComponentName: "Categorize"},
				Downstream: []string{"x", "y", "z"},
			},
			"x": {Obj: CanvasComponentObj{ComponentName: "LLM"}},
			"y": {Obj: CanvasComponentObj{ComponentName: "Message"}},
			"z": {Obj: CanvasComponentObj{ComponentName: "LLM"}},
		},
	}
	wireMultiBranches(sg, c, nil)
}

// TestWireMultiBranches_LoopMembersSkipped: children inside a loop
// subgraph should not get branches in the outer graph.
func TestWireMultiBranches_LoopMembersSkipped(t *testing.T) {
	sg := graphpkg.NewStateGraph(map[string]any{})
	c := &Canvas{
		Components: map[string]CanvasComponent{
			"loop": {
				Obj:        CanvasComponentObj{ComponentName: "Loop"},
				Downstream: []string{"body"},
			},
			"sw": {
				Obj:        CanvasComponentObj{ComponentName: "Switch"},
				Downstream: []string{"body", "other"},
			},
			"body":  {Obj: CanvasComponentObj{ComponentName: "LLM"}},
			"other": {Obj: CanvasComponentObj{ComponentName: "LLM"}},
		},
	}
	loopMembers := map[string]bool{"body": true}
	wireMultiBranches(sg, c, loopMembers)
}
