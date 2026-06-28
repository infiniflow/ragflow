package canvas

import (
	"context"
	"testing"
)

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

	exp, err := buildParallelExpansion(context.Background(), c, "Parallel:IterateList")
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
