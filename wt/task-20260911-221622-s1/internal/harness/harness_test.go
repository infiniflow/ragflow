package harness

import (
	"context"
	"sync"
	"testing"
	"time"

	"ragflow/internal/harness/graph/types"
)

func TestNewStateGraph(t *testing.T) {
	sg := NewStateGraph(map[string]interface{}{})
	if sg == nil {
		t.Fatal("expected non-nil StateGraph")
	}
}

func TestNewMemorySaver(t *testing.T) {
	ms := NewMemorySaver()
	if ms == nil {
		t.Fatal("expected non-nil MemorySaver")
	}
}

func TestCompileSimpleGraph(t *testing.T) {
	sg := NewStateGraph(map[string]interface{}{"value": ""})
	sg.AddNode("echo", func(ctx context.Context, state interface{}) (interface{}, error) {
		return state, nil
	})
	sg.AddEdge(Start, "echo")
	sg.AddEdge("echo", End)

	cg, err := sg.Compile()
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	if cg == nil {
		t.Fatal("expected non-nil compiled graph")
	}

	// Verify graph structure
	if cg.GetGraph() == nil {
		t.Fatal("expected non-nil underlying graph")
	}
}

func TestCompileWithRecursionLimit(t *testing.T) {
	sg := NewStateGraph(map[string]interface{}{})
	sg.AddNode("n", func(ctx context.Context, state interface{}) (interface{}, error) {
		return state, nil
	})
	sg.AddEdge(Start, "n")
	sg.AddEdge("n", End)

	cg, err := sg.Compile(WithRecursionLimit(5))
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	if cg == nil {
		t.Fatal("expected non-nil compiled graph")
	}
}

func TestStateGraphNodeOperations(t *testing.T) {
	sg := NewStateGraph(map[string]interface{}{})
	sg.AddNode("agent", func(ctx context.Context, state interface{}) (interface{}, error) {
		return state, nil
	})

	node, ok := sg.GetNode("agent")
	if !ok {
		t.Fatal("expected to find node 'agent'")
	}
	if node.Name != "agent" {
		t.Errorf("expected node name 'agent', got '%s'", node.Name)
	}
}

func TestNewCommand(t *testing.T) {
	cmd := NewCommand()
	if cmd == nil {
		t.Fatal("expected non-nil Command")
	}
}

func TestNewSend(t *testing.T) {
	s := NewSend("target", "arg")
	if s == nil {
		t.Fatal("expected non-nil Send")
	}
	if s.Node != "target" {
		t.Errorf("expected Node='target', got '%s'", s.Node)
	}
}

func TestDefaultRetryPolicy(t *testing.T) {
	rp := DefaultRetryPolicy()
	if rp.MaxAttempts == 0 {
		t.Error("expected non-zero MaxAttempts in default retry policy")
	}
}

// TestGraph_DAG_SlowBranchMerge verifies AllPredecessor mode: merge waits for
// slow branch. Uses pregel engine (via harness.init() which sets PregelRunFunc)
// and channel-based state (pregel-compatible pattern).
func TestGraph_DAG_SlowBranchMerge(t *testing.T) {
	var orderMu sync.Mutex
	var order []string
	slowStarted := make(chan struct{})
	fastDone := make(chan struct{})
	releaseSlow := make(chan struct{})
	mergeRan := make(chan struct{}, 1)
	record := func(name string) {
		orderMu.Lock()
		order = append(order, name)
		orderMu.Unlock()
	}

	sg := NewStateGraph(map[string]any{})

	// Start → dispatch → {fast, slow} → merge
	sg.AddNode("dispatch", func(ctx context.Context, state any) (any, error) {
		record("dispatch")
		return map[string]any{"messages": []string{"dispatch done"}}, nil
	})
	sg.AddNode("fast", func(ctx context.Context, state any) (any, error) {
		record("fast")
		close(fastDone)
		return map[string]any{"messages": []string{"fast done"}}, nil
	})
	sg.AddNode("slow", func(ctx context.Context, state any) (any, error) {
		close(slowStarted)
		<-releaseSlow
		record("slow")
		return map[string]any{"messages": []string{"slow done"}}, nil
	})
	sg.AddNode("merge", func(ctx context.Context, state any) (any, error) {
		select {
		case mergeRan <- struct{}{}:
		default:
		}
		record("merge")
		return map[string]any{"messages": []string{"merge done"}}, nil
	})

	sg.AddEdge(Start, "dispatch")
	sg.AddEdge("dispatch", "fast")
	sg.AddEdge("dispatch", "slow")
	sg.AddEdge("fast", "merge")
	sg.AddEdge("slow", "merge")
	sg.AddEdge("merge", End)

	compiled, err := sg.Compile(
		WithNodeTriggerMode(types.NodeTriggerAllPredecessor),
		WithRecursionLimit(10),
	)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := compiled.Invoke(context.Background(), map[string]any{})
		done <- err
	}()

	select {
	case <-slowStarted:
	case <-time.After(time.Second):
		t.Fatal("slow branch never started")
	}

	select {
	case <-fastDone:
	case <-time.After(time.Second):
		t.Fatal("fast branch never completed")
	}

	select {
	case <-mergeRan:
		t.Fatal("BUG: merge ran before slow branch completed")
	case <-time.After(50 * time.Millisecond):
	}

	close(releaseSlow)

	select {
	case err = <-done:
	case <-time.After(time.Second):
		t.Fatal("compiled graph did not finish after slow branch release")
	}

	if err != nil {
		t.Fatal(err)
	}

	if len(order) != 4 {
		t.Errorf("expected 4 nodes (dispatch+fast+slow+merge), got %d: %v", len(order), order)
	}
	slowIdx, mergeIdx := -1, -1
	for i, name := range order {
		switch name {
		case "slow":
			slowIdx = i
		case "merge":
			mergeIdx = i
		}
	}
	if mergeIdx < slowIdx {
		t.Errorf("BUG: merge before slow branch. merge=%d, slow=%d", mergeIdx, slowIdx)
	} else {
		t.Log("DAG slow-branch merge: merge correctly waited for slow branch")
	}
}
