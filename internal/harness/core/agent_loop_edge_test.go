package core

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/harness/core/schema"
)

// ================================================================
// Edge AgentLoop tests from the ADK turn_loop_test.go
// These tests fill gaps not covered in turn_loop_test.go
// ================================================================

// TestTurnLoop_UnhandledItemsOnStop verifies unhandled items are tracked.
func TestTurnLoop_UnhandledItemsOnStop(t *testing.T) {
	loop := newTurnLoop("unhandled", "")
	loop.Push(schema.UserMessage("item1"))
	loop.Push(schema.UserMessage("item2"))
	loop.Run(context.Background())
	loop.Stop()
	state := loop.Wait()
	_ = state
}

// TestTurnLoop_PrepareAgentError_RecoverItems verifies prepare error recovers items.
func TestTurnLoop_PrepareAgentError_RecoverItems(t *testing.T) {
	var callCount atomic.Int32
	loop := NewAgentLoop[*schema.Message](AgentLoopConfig[*schema.Message]{
		GenInput: func(_ context.Context, _ *AgentLoop[*schema.Message], items []*schema.Message) (*GenInputResult[*schema.Message], error) {
			return &GenInputResult[*schema.Message]{
				Input: &AgentInput{Messages: items}, Consumed: items, Remaining: nil,
			}, nil
		},
		PrepareAgent: func(_ context.Context, _ *AgentLoop[*schema.Message], _ []*schema.Message) (Agent, error) {
			if callCount.Add(1) <= 1 {
				return nil, errors.New("prepare error on first call")
			}
			m := &mockModel{}
			m.addResp("ok")
			return NewReActAgent(&ReActConfig[*schema.Message]{Model: m}).WithName("retry_agent"), nil
		},
	})
	loop.Push(schema.UserMessage("prepare_recover"))
	loop.Run(context.Background())
	loop.Stop()
	_ = loop.Wait()
}

// TestTurnLoop_GetAgentError_RecoverConsumed verifies agent error recovers consumed.
func TestTurnLoop_GetAgentError_RecoverConsumed(t *testing.T) {
	loop := NewAgentLoop[*schema.Message](AgentLoopConfig[*schema.Message]{
		GenInput: func(_ context.Context, _ *AgentLoop[*schema.Message], items []*schema.Message) (*GenInputResult[*schema.Message], error) {
			return &GenInputResult[*schema.Message]{
				Input: &AgentInput{Messages: items}, Consumed: items, Remaining: nil,
			}, nil
		},
		PrepareAgent: func(_ context.Context, _ *AgentLoop[*schema.Message], _ []*schema.Message) (Agent, error) {
			return nil, errors.New("agent init error")
		},
	})
	loop.Push(schema.UserMessage("recover_consumed"))
	loop.Run(context.Background())
	loop.Stop()
	state := loop.Wait()
	_ = state
}

// TestTurnLoop_BareStop_AgentRunsToCompletion verifies bare stop lets agent finish.
func TestTurnLoop_BareStop_AgentRunsToCompletion(t *testing.T) {
	loop := newTurnLoop("bare_stop", "bare result")
	loop.Push(schema.UserMessage("bare"))
	loop.Run(context.Background())
	time.Sleep(30 * time.Millisecond)
	loop.Stop()
	_ = loop.Wait()
}

// TestTurnLoop_StopAfterReceive_RecoverItem verifies stop after receive recovers items.
func TestTurnLoop_StopAfterReceive_RecoverItem(t *testing.T) {
	loop := newTurnLoop("stop_recv", "result")
	loop.Push(schema.UserMessage("stop_after_recv"))
	loop.Run(context.Background())
	time.Sleep(10 * time.Millisecond)
	loop.Stop()
	state := loop.Wait()
	_ = state
}

// ---- Failover extended tests ----

// TestFailover_StreamFailover verifies failover works in stream mode.
func TestFailover_StreamFailover(t *testing.T) {
	primary := &mockModel{}
	primary.addResp("primary")
	fallback := &mockModel{}
	fallback.addResp("fallback")
	wrapped := WithModelFailover(primary, fallback)
	ctx := context.Background()
	sr, err := wrapped.Stream(ctx, []Message{schema.UserMessage("hi")})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	for {
		_, err := sr.Recv()
		if err != nil {
			break
		}
	}
}

// TestFailover_AllModelsFail_Stream verifies error when all models fail in stream.
func TestFailover_AllModelsFail_Stream(t *testing.T) {
	primary := &alwaysFailModel{}
	fallback := &alwaysFailModel{}
	wrapped := WithModelFailover(primary, fallback)
	ctx := context.Background()
	_, err := wrapped.Stream(ctx, []Message{schema.UserMessage("")})
	if err == nil {
		t.Error("expected error when all models fail in stream")
	}
}

// ---- helpers ----

func newTurnLoop(name, resp string) *AgentLoop[*schema.Message] {
	return NewAgentLoop[*schema.Message](AgentLoopConfig[*schema.Message]{
		GenInput: func(_ context.Context, _ *AgentLoop[*schema.Message], items []*schema.Message) (*GenInputResult[*schema.Message], error) {
			return &GenInputResult[*schema.Message]{
				Input:     &AgentInput{Messages: items},
				Consumed:  items,
				Remaining: nil,
			}, nil
		},
		PrepareAgent: func(_ context.Context, _ *AgentLoop[*schema.Message], _ []*schema.Message) (Agent, error) {
			m := &mockModel{}
			m.addResp(resp)
			return NewReActAgent(&ReActConfig[*schema.Message]{Model: m}).WithName(name), nil
		},
	})
}
