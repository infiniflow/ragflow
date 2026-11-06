// Package pregel provides interrupt tests.
// Now that shouldInterrupt no longer has the trigger-to-nodes bug,
// named-node interrupts work correctly alongside wildcard "*".
package pregel

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"ragflow/internal/harness/graph/channels"
	"ragflow/internal/harness/graph/checkpoint"
	"ragflow/internal/harness/graph/constants"
	"ragflow/internal/harness/graph/graph"
	graphinterrupt "ragflow/internal/harness/graph/interrupt"
	"ragflow/internal/harness/graph/types"
)

// ============================================================
// P0: Named-node interrupt
// ============================================================

// TestInterrupt_NamedNode interrupts at a specific named node.
func TestInterrupt_NamedNode(t *testing.T) {
	engine := NewEngine(newSimpleGraph(t),
		WithRecursionLimit(10),
		WithInterrupts("node_a"),
	)
	_, err := engine.RunSync(t.Context(), map[string]any{"value": "start"})
	if err == nil {
		t.Fatal("expected interrupt at node_a")
	}
}

// TestInterrupt_NamedNode_Second interrupts at the second node.
func TestInterrupt_NamedNode_Second(t *testing.T) {
	engine := NewEngine(newSimpleGraph(t),
		WithRecursionLimit(10),
		WithInterrupts("node_b"),
	)
	_, err := engine.RunSync(t.Context(), map[string]any{"value": "start"})
	if err == nil {
		t.Fatal("expected interrupt at node_b")
	}
}

// TestInterrupt_LastNode interrupts at last node before __end__.
func TestInterrupt_LastNode(t *testing.T) {
	engine := NewEngine(newSimpleGraph(t),
		WithRecursionLimit(10),
		WithInterrupts("node_b"),
	)
	_, err := engine.RunSync(t.Context(), map[string]any{"value": "start"})
	if err == nil {
		t.Fatal("expected interrupt at last node")
	}
}

// ============================================================
// P0: Multiple named nodes
// ============================================================

func TestInterrupt_MultipleNamedNodes(t *testing.T) {
	engine := NewEngine(newSimpleGraph(t),
		WithRecursionLimit(10),
		WithInterrupts("node_a", "node_b"),
	)
	_, err := engine.RunSync(t.Context(), map[string]any{"value": "start"})
	if err == nil {
		t.Fatal("expected interrupt at multiple nodes")
	}
}

// ============================================================
// P0: Wildcard interrupt
// ============================================================

func TestInterrupt_Wildcard(t *testing.T) {
	engine := NewEngine(newSimpleGraph(t),
		WithRecursionLimit(10),
		WithInterrupts("*"),
	)
	_, err := engine.RunSync(t.Context(), map[string]any{"value": "start"})
	if err == nil {
		t.Fatal("expected interrupt on wildcard")
	}
}

// ============================================================
// P0: After-node interrupt
// ============================================================

func TestInterrupt_AfterNamedNode(t *testing.T) {
	engine := NewEngine(newSimpleGraph(t),
		WithRecursionLimit(10),
		WithInterruptsAfter("node_a"),
	)
	_, err := engine.RunSync(t.Context(), map[string]any{"value": "start"})
	if err == nil {
		t.Fatal("expected after-interrupt at node_a")
	}
}

func TestInterrupt_WildcardAfter(t *testing.T) {
	engine := NewEngine(newSimpleGraph(t),
		WithRecursionLimit(10),
		WithInterruptsAfter("*"),
	)
	_, err := engine.RunSync(t.Context(), map[string]any{"value": "start"})
	if err == nil {
		t.Fatal("expected after-interrupt")
	}
}

// ============================================================
// P1: Interrupt with checkpoint
// ============================================================

func TestInterrupt_WithCheckpointer(t *testing.T) {
	ms := checkpoint.NewMemorySaver()
	tid := "int-cp-fix"
	cfg := &types.RunnableConfig{
		Configurable: map[string]interface{}{constants.ConfigKeyThreadID: tid},
	}
	engine := NewEngine(newSimpleGraph(t),
		WithRecursionLimit(10),
		WithCheckpointer(ms),
		WithConfig(cfg),
		WithInterrupts("node_a"),
	)

	_, err := engine.RunSync(t.Context(), map[string]any{"value": "start"})
	if err == nil {
		t.Fatal("expected interrupt")
	}

	cp, _ := ms.Get(t.Context(), map[string]interface{}{
		constants.ConfigKeyThreadID: tid,
	})
	if cp != nil {
		t.Log("checkpoint saved at interrupt")
	}
}

func TestInterrupt_ResumeValueFromConfig(t *testing.T) {
	saver := checkpoint.NewMemorySaver()
	cfg := &types.RunnableConfig{Configurable: map[string]interface{}{
		constants.ConfigKeyThreadID: "dynamic-interrupt-resume",
	}}
	sg := graph.NewStateGraph(map[string]any{"value": ""})
	sg.AddChannel("value", channels.NewLastValue(""))
	sg.AddNode("request_approval", func(ctx context.Context, state any) (any, error) {
		value, err := graphinterrupt.Interrupt(ctx, "approval required")
		if err != nil {
			return nil, err
		}
		state.(map[string]any)["value"] = value
		return state, nil
	})
	if err := sg.AddEdge(constants.Start, "request_approval"); err != nil {
		t.Fatal(err)
	}
	if err := sg.AddEdge("request_approval", constants.End); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(sg,
		WithRecursionLimit(10),
		WithCheckpointer(saver),
		WithConfig(cfg),
	)

	if _, err := engine.RunSync(t.Context(), map[string]any{"value": ""}); err == nil {
		t.Fatal("expected the first execution to interrupt")
	}

	cfg.Configurable[constants.ConfigKeyResumeMap] = map[string]interface{}{"request_approval": "approved"}
	result, err := engine.RunSync(t.Context(), map[string]any{"value": ""})
	if err != nil {
		t.Fatalf("resume execution: %v", err)
	}
	if got := result.(map[string]any)["value"]; got != "approved" {
		t.Fatalf("resumed value = %v, want approved", got)
	}
}

// ============================================================
// P1: No-checkpointer interrupt
// ============================================================

func TestInterrupt_NoCheckpointer(t *testing.T) {
	engine := NewEngine(newSimpleGraph(t),
		WithRecursionLimit(10),
		WithInterrupts("node_a"),
	)
	_, err := engine.RunSync(t.Context(), map[string]any{"value": "x"})
	if err == nil {
		t.Fatal("expected interrupt without checkpointer")
	}
}

// ============================================================
// P2: Concurrent interrupt (named node)
// ============================================================

func TestInterrupt_Concurrent(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Go(func() {
			engine := NewEngine(newSimpleGraph(t),
				WithRecursionLimit(10),
				WithInterrupts("node_a"),
			)
			_, err := engine.RunSync(t.Context(), map[string]any{"value": "conc"})
			if err == nil {
				t.Errorf("expected interrupt")
			}
		})
	}
	wg.Wait()
}

func TestInterrupt_ConcurrentWithCheckpointer(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ms := checkpoint.NewMemorySaver()
			tid := fmt.Sprintf("int-conc-fix-%d", idx)
			cfg := &types.RunnableConfig{
				Configurable: map[string]interface{}{constants.ConfigKeyThreadID: tid},
			}
			engine := NewEngine(newSimpleGraph(t),
				WithRecursionLimit(10),
				WithCheckpointer(ms),
				WithConfig(cfg),
				WithInterrupts("node_a"),
			)
			_, err := engine.RunSync(t.Context(), map[string]any{"value": "x"})
			if err == nil {
				t.Errorf("expected interrupt")
			}
		}(i)
	}
	wg.Wait()
}
