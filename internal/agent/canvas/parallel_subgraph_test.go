package canvas

import (
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
	t.Skip("parallel expansion uses harness graph/graph/parallel.go")
}
