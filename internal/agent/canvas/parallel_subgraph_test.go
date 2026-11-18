package canvas

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	_ "ragflow/internal/agent/component"

	"github.com/cloudwego/eino/compose"
)

func TestParallelResumePreservesProducerState(t *testing.T) {
	c := &Canvas{
		Components: map[string]CanvasComponent{
			"parallel": {Obj: CanvasComponentObj{ComponentName: "Parallel", Params: map[string]any{
				"items_ref": "sys.items",
				"outputs":   map[string]any{"results": map[string]any{"ref": "consumer@result"}},
			}}},
			"producer": {Obj: CanvasComponentObj{ComponentName: "VariableAssigner", Params: map[string]any{
				"variables": []any{map[string]any{"variable": "env.saved", "operator": "set", "parameter": "prepared"}},
			}}, Upstream: []string{"parallel"}, Downstream: []string{"input"}},
			"input": {Obj: CanvasComponentObj{ComponentName: "UserFillUp", Params: map[string]any{
				"inputs": map[string]any{"value": map[string]any{"type": "line"}},
			}}, Upstream: []string{"producer"}, Downstream: []string{"consumer"}},
			"consumer": {Obj: CanvasComponentObj{ComponentName: "StringTransform", Params: map[string]any{
				"method": "merge", "script": "{{env.saved}}:{{input@value}}", "delimiters": []any{"|"},
			}}, Upstream: []string{"input"}},
		},
		NodeParents: map[string]string{"producer": "parallel", "input": "parallel", "consumer": "parallel"},
	}
	store, _ := newTestStore(t, time.Minute)
	compiled, err := Compile(t.Context(), c, WithCheckPointStore(store))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	const checkpoint = "parallel-state-regression"
	events := make(chan RunEvent, 64)
	newContext := func() context.Context {
		state := NewCanvasState("run-parallel", "session-parallel")
		state.Sys["items"] = []any{"one"}
		return WithRunMeta(WithState(t.Context(), state), &RunMeta{Events: events})
	}
	ctx := newContext()
	_, err = compiled.Workflow.Invoke(ctx, map[string]any{}, compose.WithCheckPointID(checkpoint))
	if !IsInterruptError(err) {
		t.Fatalf("first run: want UserFillUp interrupt, got %v", err)
	}
	interruptID := FirstInterruptID(ExtractInterruptContexts(err))
	if interruptID == "" {
		t.Fatalf("interrupt contexts: %v", ExtractInterruptContexts(err))
	}
	resumedCtx := compose.ResumeWithData(newContext(), interruptID, "answer")
	out, err := compiled.Workflow.Invoke(resumedCtx, map[string]any{}, compose.WithCheckPointID(checkpoint))
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	started, _ := collectLifecycleEvents(t, events)
	if started["producer"] != 1 {
		t.Fatalf("producer invoked %d times, want once", started["producer"])
	}
	parallel, _ := out["parallel"].(map[string]any)
	result, _ := parallel["results"].([]any)
	if len(result) != 1 || result[0] != "prepared:answer" {
		t.Fatalf("results = %#v, want [prepared:answer]", out)
	}
}

func TestParallelResumePreservesStateAcrossTwoInterrupts(t *testing.T) {
	c := &Canvas{
		Components: map[string]CanvasComponent{
			"parallel": {Obj: CanvasComponentObj{ComponentName: "Parallel", Params: map[string]any{
				"items_ref": "sys.items",
				"outputs":   map[string]any{"results": map[string]any{"ref": "consumer@result"}},
			}}},
			"producer": {Obj: CanvasComponentObj{ComponentName: "VariableAssigner", Params: map[string]any{
				"variables": []any{map[string]any{"variable": "env.before", "operator": "set", "parameter": "initial"}},
			}}, Upstream: []string{"parallel"}, Downstream: []string{"first"}},
			"first": {Obj: CanvasComponentObj{ComponentName: "UserFillUp", Params: map[string]any{
				"inputs": map[string]any{"value": map[string]any{"type": "line"}},
			}}, Upstream: []string{"producer"}, Downstream: []string{"between"}},
			"between": {Obj: CanvasComponentObj{ComponentName: "VariableAssigner", Params: map[string]any{
				"variables": []any{map[string]any{"variable": "env.after", "operator": "set", "parameter": "first@value"}},
			}}, Upstream: []string{"first"}, Downstream: []string{"second"}},
			"second": {Obj: CanvasComponentObj{ComponentName: "UserFillUp", Params: map[string]any{
				"inputs": map[string]any{"value": map[string]any{"type": "line"}},
			}}, Upstream: []string{"between"}, Downstream: []string{"consumer"}},
			"consumer": {Obj: CanvasComponentObj{ComponentName: "StringTransform", Params: map[string]any{
				"method": "merge", "script": "{{env.before}}:{{env.after}}:{{second@value}}", "delimiters": []any{"|"},
			}}, Upstream: []string{"second"}},
		},
		NodeParents: map[string]string{
			"producer": "parallel", "first": "parallel", "between": "parallel", "second": "parallel", "consumer": "parallel",
		},
	}
	store, _ := newTestStore(t, time.Minute)
	compiled, err := Compile(t.Context(), c, WithCheckPointStore(store))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	const checkpoint = "parallel-double-interrupt"
	events := make(chan RunEvent, 64)
	newContext := func() context.Context {
		state := NewCanvasState("run-parallel", "session-parallel")
		state.Sys["items"] = []any{"one"}
		return WithRunMeta(WithState(t.Context(), state), &RunMeta{Events: events})
	}
	ctx := newContext()
	_, err = compiled.Workflow.Invoke(ctx, map[string]any{}, compose.WithCheckPointID(checkpoint))
	if !IsInterruptError(err) {
		t.Fatalf("first run: want interrupt, got %v", err)
	}
	firstID := FirstInterruptID(ExtractInterruptContexts(err))
	if firstID == "" {
		t.Fatalf("first interrupt contexts: %v", ExtractInterruptContexts(err))
	}
	ctx = compose.ResumeWithData(newContext(), firstID, "answer-one")
	_, err = compiled.Workflow.Invoke(ctx, map[string]any{}, compose.WithCheckPointID(checkpoint))
	if !IsInterruptError(err) {
		t.Fatalf("first resume: want second interrupt, got %v", err)
	}
	secondID := FirstInterruptID(ExtractInterruptContexts(err))
	if secondID == "" {
		t.Fatalf("second interrupt contexts: %v", ExtractInterruptContexts(err))
	}
	ctx = compose.ResumeWithData(newContext(), secondID, "answer-two")
	out, err := compiled.Workflow.Invoke(ctx, map[string]any{}, compose.WithCheckPointID(checkpoint))
	if err != nil {
		t.Fatalf("second resume: %v", err)
	}
	started, _ := collectLifecycleEvents(t, events)
	if started["producer"] != 1 || started["between"] != 1 {
		t.Fatalf("producer/between runs = %d/%d, want once each", started["producer"], started["between"])
	}
	parallel, _ := out["parallel"].(map[string]any)
	result, _ := parallel["results"].([]any)
	if len(result) != 1 || result[0] != "initial:answer-one:answer-two" {
		t.Fatalf("results = %#v, want [initial:answer-one:answer-two]", out)
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
