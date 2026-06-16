package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"ragflow/internal/harness/core/schema"
)

func TestDebugParallelCancel(t *testing.T) {
	m1 := &mockModel{}; m1.addResp("P1")
	m2 := &mockModel{}; m2.addResp("P2")
	a1 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m1}); a1.name = "p1"
	a2 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m2}); a2.name = "p2"
	ctx := context.Background()
	wf, err := NewParallel(ctx, &ParallelConfig{Name: "par", Description: "test", SubAgents: []Agent{a1, a2}})
	if err != nil { t.Fatalf("NewParallel: %v", err) }
	opt, cancel := WithCancel()
	iter := wf.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("run")}}, opt)
	time.Sleep(10 * time.Millisecond)
	cancel()
	count := 0
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		count++
		var ce *CancelError
		if ev.Err != nil && errors.As(ev.Err, &ce) {
			t.Logf("event[%d]: CancelError", count)
			continue
		}
		if ev.Action != nil && ev.Action.Interrupted != nil {
			t.Logf("event[%d]: Interrupted", count)
			continue
		}
		t.Logf("event[%d]: Err=%v Action=%+#v", count, ev.Err, ev.Action)
	}
	t.Logf("total events: %d", count)
}
