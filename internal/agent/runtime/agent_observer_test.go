package runtime

import (
	"context"
	"testing"
)

func TestObserveAgentNodePreservesContextWithoutObserver(t *testing.T) {
	ctx := t.Context()
	child, finish := ObserveAgentNode(ctx, "id", "name", "Tool", 1, "input")
	if child != ctx || finish == nil {
		t.Fatal("missing observer changed context or returned no finisher")
	}
	finish("result", nil)
}

func TestObserveAgentNodeDispatchesAndPropagatesChildContext(t *testing.T) {
	type childKey struct{}
	called, finished := false, false
	ctx := WithAgentNodeObserver(t.Context(), func(ctx context.Context, id, name, kind string, attempt uint32, input any) (context.Context, func(any, error)) {
		called = true
		if id != "id" || name != "name" || kind != "Tool" || attempt != 2 || input != "input" {
			t.Fatalf("unexpected observer arguments %q %q %q %d %v", id, name, kind, attempt, input)
		}
		return context.WithValue(ctx, childKey{}, "child"), func(output any, err error) { finished = output == "result" && err == nil }
	})
	child, finish := ObserveAgentNode(ctx, "id", "name", "Tool", 2, "input")
	if child.Value(childKey{}) != "child" || !called {
		t.Fatal("observer did not propagate child context")
	}
	finish("result", nil)
	if !finished {
		t.Fatal("observer did not receive completion")
	}
}
