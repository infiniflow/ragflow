package canvas

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/cloudwego/eino/compose"
)

// Rebuilding a paused workflow must preserve the terminal data mappings even
// when the component map enumerates its mutually exclusive leaves differently.
func TestWorkflowTerminalsResumeAcrossEnumerationOrder(t *testing.T) {
	for _, active := range []string{"a", "b"} {
		t.Run(active, func(t *testing.T) {
			store, _ := newTestStore(t, time.Hour)
			build := func(terminals []string) compose.Runnable[map[string]any, map[string]any] {
				wf := compose.NewWorkflow[map[string]any, map[string]any]()
				wf.AddLambdaNode("route", compose.InvokableLambda(func(_ context.Context, in map[string]any) (map[string]any, error) {
					return in, nil
				})).AddInput(compose.START)
				for _, id := range []string{"a", "b"} {
					wf.AddLambdaNode(id, compose.InvokableLambda(func(_ context.Context, _ map[string]any) (map[string]any, error) {
						return map[string]any{"content": "finished " + id}, nil
					})).AddInput("route")
				}
				wf.AddBranch("route", compose.NewGraphMultiBranch(func(_ context.Context, _ map[string]any) (map[string]bool, error) {
					return map[string]bool{active: true}, nil
				}, map[string]bool{"a": true, "b": true}))
				if err := wireWorkflowTerminals(wf, terminals, "", true); err != nil {
					t.Fatal(err)
				}
				run, err := wf.Compile(t.Context(), compose.WithCheckPointStore(store), compose.WithInterruptBeforeNodes([]string{active}))
				if err != nil {
					t.Fatal(err)
				}
				return run
			}
			first := build([]string{"a", "b"})
			_, err := first.Invoke(t.Context(), map[string]any{}, compose.WithCheckPointID("terminal-order"))
			if !IsInterruptError(err) {
				t.Fatalf("first run: want interrupt, got %v", err)
			}
			resumed := build([]string{"b", "a"})
			out, err := resumed.Invoke(t.Context(), nil, compose.WithCheckPointID("terminal-order"))
			if err != nil {
				t.Fatalf("resume: %v", err)
			}
			want := map[string]any{active: map[string]any{"content": "finished " + active}}
			if !reflect.DeepEqual(out, want) {
				t.Fatalf("output = %#v, want %#v", out, want)
			}
		})
	}
}
