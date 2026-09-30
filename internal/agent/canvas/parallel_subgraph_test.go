package canvas

import (
	"errors"
	"strings"
	"testing"
)

type failingParallelCloneValue struct{}

func (failingParallelCloneValue) MarshalJSON() ([]byte, error) {
	return nil, errors.New("clone failure")
}

func TestParallelCloneFailureStopsItemBeforeInvocation(t *testing.T) {
	c := &Canvas{Components: map[string]CanvasComponent{
		"Message:body": {Obj: CanvasComponentObj{ComponentName: "Message", Params: map[string]any{"content": []any{"done"}}}},
	}}
	sub, err := buildParallelItemWorkflow(t.Context(), c, "parallel", map[string]bool{"Message:body": true})
	if err != nil {
		t.Fatal(err)
	}
	outer, err := buildParallelOuterWorkflow(t.Context(), "parallel", "sys.items", 1, nil, sub)
	if err != nil {
		t.Fatal(err)
	}
	runnable, err := outer.Compile(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	state := NewCanvasState("run", "session")
	state.Sys["items"] = []any{"one"}
	state.Sys["broken"] = failingParallelCloneValue{}
	_, err = runnable.Invoke(withState(t.Context(), state), map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "clone failure") {
		t.Fatalf("Invoke error = %v; want clone failure", err)
	}
}

func TestCollectGroupedMembers_UsesParentMetadata(t *testing.T) {
	c := &Canvas{
		Components: map[string]CanvasComponent{
			"Parallel:IterateList":    {Obj: CanvasComponentObj{ComponentName: "Parallel"}},
			"IterationItem:IterStart": {Obj: CanvasComponentObj{ComponentName: "IterationItem"}},
			"StringTransform:FmtItem": {Obj: CanvasComponentObj{ComponentName: "StringTransform"}},
			"Message:IterDone":        {Obj: CanvasComponentObj{ComponentName: "Message"}},
		},
		NodeParents: map[string]string{
			"IterationItem:IterStart": "Parallel:IterateList",
			"StringTransform:FmtItem": "Parallel:IterateList",
		},
	}

	got := collectGroupedMembers(c, "Parallel:IterateList")
	if !got["IterationItem:IterStart"] {
		t.Fatalf("IterationItem child missing from grouped members: %v", got)
	}
	if !got["StringTransform:FmtItem"] {
		t.Fatalf("FmtItem child missing from grouped members: %v", got)
	}
	if got["Message:IterDone"] {
		t.Fatalf("outer follower should not be part of grouped members: %v", got)
	}
}

func TestBuildParallelExpansion_PrefersGroupedMembersOverDescendants(t *testing.T) {
	c := &Canvas{
		Components: map[string]CanvasComponent{
			"Parallel:IterateList": {
				Obj: CanvasComponentObj{
					ComponentName: "Parallel",
					Params: map[string]any{
						"items_ref": "sys.items",
						"outputs": map[string]any{
							"lines": map[string]any{
								"ref": "StringTransform:FmtItem@result",
							},
						},
					},
				},
				Downstream: []string{"Message:IterDone"},
			},
			"IterationItem:IterStart": {
				Obj:        CanvasComponentObj{ComponentName: "IterationItem", Params: map[string]any{}},
				Downstream: []string{"StringTransform:FmtItem"},
				Upstream:   []string{"Parallel:IterateList"},
			},
			"StringTransform:FmtItem": {
				Obj: CanvasComponentObj{
					ComponentName: "StringTransform",
					Params: map[string]any{
						"method":     "merge",
						"script":     "{item}",
						"delimiters": []any{"|"},
					},
				},
				Upstream: []string{"IterationItem:IterStart"},
			},
			"Message:IterDone": {
				Obj:      CanvasComponentObj{ComponentName: "Message", Params: map[string]any{"content": []any{"done"}}},
				Upstream: []string{"Parallel:IterateList"},
			},
		},
		NodeParents: map[string]string{
			"IterationItem:IterStart": "Parallel:IterateList",
			"StringTransform:FmtItem": "Parallel:IterateList",
		},
	}

	exp, err := buildParallelExpansion(t.Context(), c, "Parallel:IterateList")
	if err != nil {
		t.Fatalf("buildParallelExpansion: %v", err)
	}
	if !exp.Members["IterationItem:IterStart"] || !exp.Members["StringTransform:FmtItem"] {
		t.Fatalf("expected grouped children in expansion members, got %v", exp.Members)
	}
	if exp.Members["Message:IterDone"] {
		t.Fatalf("outer follower should stay outside the parallel subgraph, got %v", exp.Members)
	}
	if exp.ItemsRef != "sys.items" {
		t.Fatalf("ItemsRef = %q, want sys.items", exp.ItemsRef)
	}
	if exp.OutputRefs["lines"] != "StringTransform:FmtItem@result" {
		t.Fatalf("lines output ref = %q, want StringTransform:FmtItem@result", exp.OutputRefs["lines"])
	}
}
