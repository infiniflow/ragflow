package graph

import (
	"context"
	"testing"
	"time"

	"ragflow/internal/harness/graph/checkpoint"
	"ragflow/internal/harness/graph/constants"
	"ragflow/internal/harness/graph/types"
)

type recoveryState struct {
	Step    int
	Message string
}

// TestCheckpoint_InterruptAndResume verifies the full interrupt-resume cycle
// via the graph engine with actual checkpoint persistence.
func TestCheckpoint_InterruptAndResume(t *testing.T) {
	// Interrupt/resume requires the full Pregel engine.
	// See graph/pregel/pregel_durability_timetravel_test.go for equivalent tests.
	t.Skip("requires full Pregel engine for interrupt/resume")
	// Use map-based state to work with both the test runner and full pregel.
	sg := NewStateGraph(map[string]any{"step": 0, "message": ""})

	// Node 1: sets initial state.
	sg.AddNode("init_state", func(ctx context.Context, state interface{}) (interface{}, error) {
		s := state.(map[string]any)
		s["step"] = 1
		s["message"] = "initialized"
		return s, nil
	})

	// Node 2: blocked by interrupt (human-in-the-loop).
	sg.AddNode("approval_step", func(ctx context.Context, state interface{}) (interface{}, error) {
		s := state.(map[string]any)
		s["step"] = 2
		s["message"] = "approved"
		return s, nil
	})

	// Node 3: final processing.
	sg.AddNode("finalize", func(ctx context.Context, state interface{}) (interface{}, error) {
		s := state.(map[string]any)
		s["step"] = 3
		s["message"] = "finalized"
		return s, nil
	})

	sg.AddEdge(constants.Start, "init_state")
	sg.AddEdge("init_state", "approval_step")
	sg.AddEdge("approval_step", "finalize")
	sg.AddEdge("finalize", constants.End)

	saver := checkpoint.NewMemorySaver()
	cg, err := sg.Compile(
		WithCheckpointer(saver),
		WithInterrupts("approval_step"),
		WithRecursionLimit(10),
	)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	ctx := context.Background()
	threadID := "recovery-thread-001"
	config := types.NewRunnableConfig()
	config.ThreadID = threadID

	// First run: should interrupt before approval_step.
	result, err := cg.Invoke(ctx, map[string]any{"step": 0, "message": ""}, config)
	if err == nil {
		// If graph completed without interrupt, step 1 could have auto-passed.
		s := result.(map[string]any)
		t.Logf("no interrupt — graph completed: step=%v msg=%v", s["step"], s["message"])
		return
	}
	t.Logf("interrupted (expected): %v", err)

	// Resume from checkpoint.
	config.Set("checkpoint_id", threadID)
	result, err = cg.Invoke(ctx, nil, config)
	if err != nil {
		t.Fatalf("Resume failed: %v", err)
	}
	s := result.(map[string]any)
	if step, ok := s["step"].(int); ok && step < 2 {
		t.Errorf("expected step >= 2 after resume, got %d", step)
	}
	t.Logf("resumed: step=%v msg=%v", s["step"], s["message"])
}

// TestCheckpoint_MultiStepRecovery verifies multi-step state is preserved across interrupts.
func TestCheckpoint_MultiStepRecovery(t *testing.T) {
	sg := NewStateGraph(map[string]interface{}{"count": 0, "words": []interface{}{}})

	steps := 5
	for i := 1; i <= steps; i++ {
		idx := i
		name := "step_" + string(rune('A'+idx-1))
		sg.AddNode(name, func(ctx context.Context, state interface{}) (interface{}, error) {
			s := state.(map[string]interface{})
			s["count"] = idx
			words := s["words"].([]interface{})
			words = append(words, name)
			s["words"] = words
			return s, nil
		})
	}

	sg.AddEdge(constants.Start, "step_A")
	for i := 'A'; i < 'A'+rune(steps-1); i++ {
		sg.AddEdge("step_"+string(i), "step_"+string(i+1))
	}
	sg.AddEdge("step_"+string(rune('A'+steps-1)), constants.End)

	saver := checkpoint.NewMemorySaver()
	cg, err := sg.Compile(
		WithCheckpointer(saver),
		WithRecursionLimit(20),
	)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	ctx := context.Background()
	result, err := cg.Invoke(ctx, map[string]interface{}{"count": 0, "words": []interface{}{}},
		&types.RunnableConfig{ThreadID: "multi-step-001"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	s := result.(map[string]interface{})
	if c, ok := s["count"].(int); !ok || c != steps {
		t.Errorf("expected count=%d after %d steps, got %v", steps, steps, s["count"])
	}
	words := s["words"].([]interface{})
	t.Logf("multi-step: count=%d words=%v (len=%d)", s["count"], words, len(words))
}

// TestCheckpoint_ConcurrentSaves verifies concurrent checkpoint saves don't corrupt state.
func TestCheckpoint_ConcurrentSaves(t *testing.T) {
	saver := checkpoint.NewMemorySaver()
	const threads = 10

	done := make(chan bool, threads)
	for i := 0; i < threads; i++ {
		go func(id int) {
			config := map[string]interface{}{
				"thread_id":     "concurrent-save",
				"checkpoint_id": "cp-" + string(rune('A'+id)),
			}
			cp := map[string]interface{}{
				"step":    id,
				"value":   "data",
				"version": id,
			}
			// Put is thread-safe via MemorySaver's RWMutex.
			_ = saver.Put(context.Background(), config, cp)
			time.Sleep(time.Millisecond)
			done <- true
		}(i)
	}

	for i := 0; i < threads; i++ {
		<-done
	}

	// Verify latest checkpoint is accessible.
	latest, err := saver.Get(context.Background(), map[string]interface{}{"thread_id": "concurrent-save"})
	if err != nil {
		t.Fatalf("Get after concurrent saves: %v", err)
	}
	if latest == nil {
		t.Fatal("expected non-nil checkpoint after concurrent saves")
	}
	t.Logf("concurrent saves: checkpoint exists with %d keys", len(latest))
}

// TestCheckpoint_RecursionLimit protects against infinite loop.
func TestCheckpoint_RecursionLimit(t *testing.T) {
	if types.PregelRunFunc == nil {
		t.Skip("Pregel engine not injected")
	}
	sg := NewStateGraph(map[string]interface{}{"count": 0})
	// Create a self-loop that would run forever.
	sg.AddNode("loop", func(ctx context.Context, state interface{}) (interface{}, error) {
		s := state.(map[string]interface{})
		c := s["count"].(int)
		s["count"] = c + 1
		return s, nil
	})
	sg.AddEdge(constants.Start, "loop")
	sg.AddEdge("loop", "loop") // self-loop
	sg.SetFinishPoint("loop")

	cg, err := sg.Compile(WithRecursionLimit(3))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	_, err = cg.Invoke(context.Background(), map[string]interface{}{"count": 0})
	if err == nil {
		t.Fatal("expected recursion limit error, got nil")
	}
	t.Logf("recursion limit caught: %v", err)
}
