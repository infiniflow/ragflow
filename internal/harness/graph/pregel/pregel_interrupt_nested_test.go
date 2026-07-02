// Package pregel provides interrupt tests.
// Now that shouldInterrupt no longer has the trigger-to-nodes bug,
// named-node interrupts work correctly alongside wildcard "*".
package pregel

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"ragflow/internal/harness/graph/checkpoint"
	"ragflow/internal/harness/graph/constants"
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
	_, err := engine.RunSync(context.Background(), map[string]any{"value": "start"})
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
	_, err := engine.RunSync(context.Background(), map[string]any{"value": "start"})
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
	_, err := engine.RunSync(context.Background(), map[string]any{"value": "start"})
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
	_, err := engine.RunSync(context.Background(), map[string]any{"value": "start"})
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
	_, err := engine.RunSync(context.Background(), map[string]any{"value": "start"})
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
	_, err := engine.RunSync(context.Background(), map[string]any{"value": "start"})
	if err == nil {
		t.Fatal("expected after-interrupt at node_a")
	}
}

func TestInterrupt_WildcardAfter(t *testing.T) {
	engine := NewEngine(newSimpleGraph(t),
		WithRecursionLimit(10),
		WithInterruptsAfter("*"),
	)
	_, err := engine.RunSync(context.Background(), map[string]any{"value": "start"})
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

	_, err := engine.RunSync(context.Background(), map[string]any{"value": "start"})
	if err == nil {
		t.Fatal("expected interrupt")
	}

	cp, _ := ms.Get(context.Background(), map[string]interface{}{
		constants.ConfigKeyThreadID: tid,
	})
	if cp != nil {
		t.Log("checkpoint saved at interrupt")
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
	_, err := engine.RunSync(context.Background(), map[string]any{"value": "x"})
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
			_, err := engine.RunSync(context.Background(), map[string]any{"value": "conc"})
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
			_, err := engine.RunSync(context.Background(), map[string]any{"value": "x"})
			if err == nil {
				t.Errorf("expected interrupt")
			}
		}(i)
	}
	wg.Wait()
}
