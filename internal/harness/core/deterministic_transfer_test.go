package core

import (
	"context"
	"testing"

	"ragflow/internal/harness/core/schema"
)

// ---- helpers ----

type dtTestStore struct{ data map[string][]byte }

func newDTTestStore() *dtTestStore { return &dtTestStore{data: make(map[string][]byte)} }
func (s *dtTestStore) Set(_ context.Context, key string, value []byte) error { s.data[key] = value; return nil }
func (s *dtTestStore) Get(_ context.Context, key string) ([]byte, bool, error) { v, ok := s.data[key]; return v, ok, nil }

type dtTestAgent struct {
	name     string
	runFn    func(ctx context.Context, input *AgentInput, options ...RunOption) *AsyncIterator[*AgentEvent]
	resumeFn func(ctx context.Context, info *ResumeInfo, opts ...RunOption) *AsyncIterator[*AgentEvent]
}

func (a *dtTestAgent) Name(_ context.Context) string        { return a.name }
func (a *dtTestAgent) Description(_ context.Context) string { return a.name + " description" }
func (a *dtTestAgent) Run(ctx context.Context, input *AgentInput, options ...RunOption) *AsyncIterator[*AgentEvent] {
	return a.runFn(ctx, input, options...)
}
func (a *dtTestAgent) Resume(ctx context.Context, info *ResumeInfo, opts ...RunOption) *AsyncIterator[*AgentEvent] {
	if a.resumeFn != nil { return a.resumeFn(ctx, info, opts...) }
	return a.runFn(ctx, &AgentInput{}, opts...)
}

// ---- tests ----

func TestDeterministicTransfer_Basic(t *testing.T) {
	ctx := context.Background()
	interruptData := "interrupt_data"
	var runCount int

	innerAgent := &dtTestAgent{
		name: "inner",
		runFn: func(ctx context.Context, input *AgentInput, options ...RunOption) *AsyncIterator[*AgentEvent] {
			runCount++
			iter, gen := NewAsyncIteratorPair[*AgentEvent]()
			go func() {
				defer gen.Close()
				gen.Send(EventFromMessage(&schema.Message{Role: schema.RoleAssistant, Content: "before interrupt"}, nil, schema.RoleAssistant, ""))
				intEvent := Interrupt(ctx, interruptData)
				gen.Send(intEvent)
			}()
			return iter
		},
		resumeFn: func(ctx context.Context, info *ResumeInfo, opts ...RunOption) *AsyncIterator[*AgentEvent] {
			runCount++
			if !info.WasInterrupted { t.Error("should be interrupted") }
			runCtx := getRunCtx(ctx)
			_ = runCtx
			iter, gen := NewAsyncIteratorPair[*AgentEvent]()
			go func() {
				defer gen.Close()
				gen.Send(EventFromMessage(&schema.Message{Role: schema.RoleAssistant, Content: "after resume"}, nil, schema.RoleAssistant, ""))
			}()
			return iter
		},
	}

	agent := AgentWithDeterministicTransfer(ctx, &DeterministicTransferConfig{
		Agent:        innerAgent,
		ToAgentNames: []string{"agent_a", "agent_b"},
	})

	store := newDTTestStore()
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent, CheckPointStore: store})
	iter := runner.Query(ctx, "test")
	drainAgentEvents(t, iter)
	t.Logf("runCount=%d", runCount)
}

func TestDeterministicTransfer_RunPath(t *testing.T) {
	ctx := context.Background()

	innerAgent := &dtTestAgent{
		name: "inner",
		runFn: func(ctx context.Context, input *AgentInput, options ...RunOption) *AsyncIterator[*AgentEvent] {
			runCtx := getRunCtx(ctx)
			_ = runCtx
			iter, gen := NewAsyncIteratorPair[*AgentEvent]()
			go func() {
				defer gen.Close()
				gen.Send(EventFromMessage(&schema.Message{Role: schema.RoleAssistant, Content: "run path test"}, nil, schema.RoleAssistant, ""))
			}()
			return iter
		},
	}

	agent := AgentWithDeterministicTransfer(ctx, &DeterministicTransferConfig{
		Agent:        innerAgent,
		ToAgentNames: []string{"target"},
	})

	iter := agent.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("test")}})
	drainAgentEvents(t, iter)
}

func TestDeterministicTransfer_ExitSkipsTransfer(t *testing.T) {
	ctx := context.Background()

	innerAgent := &dtTestAgent{
		name: "inner",
		runFn: func(ctx context.Context, input *AgentInput, options ...RunOption) *AsyncIterator[*AgentEvent] {
			iter, gen := NewAsyncIteratorPair[*AgentEvent]()
			go func() {
				defer gen.Close()
				gen.Send(EventFromMessage(&schema.Message{Role: schema.RoleAssistant, Content: "normal"}, nil, schema.RoleAssistant, ""))
				gen.Send(&AgentEvent{Action: NewExitAction()})
			}()
			return iter
		},
	}

	agent := AgentWithDeterministicTransfer(ctx, &DeterministicTransferConfig{
		Agent:        innerAgent,
		ToAgentNames: []string{"should_not_transfer"},
	})

	iter := agent.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("exit")}})
	events := drainAgentEvents(t, iter)

	foundTransfer := false
	for _, ev := range events {
		if ev.Action != nil && ev.Action.TransferToAgent != nil {
			foundTransfer = true
		}
	}
	if foundTransfer {
		t.Error("should not transfer after exit action")
	}
}

func TestDeterministicTransfer_NonFlowAgent(t *testing.T) {
	ctx := context.Background()

	innerAgent := &dtTestAgent{
		name: "simple",
		runFn: func(ctx context.Context, input *AgentInput, options ...RunOption) *AsyncIterator[*AgentEvent] {
			iter, gen := NewAsyncIteratorPair[*AgentEvent]()
			go func() {
				defer gen.Close()
				gen.Send(EventFromMessage(&schema.Message{Role: schema.RoleAssistant, Content: "done"}, nil, schema.RoleAssistant, ""))
			}()
			return iter
		},
	}

	agent := AgentWithDeterministicTransfer(ctx, &DeterministicTransferConfig{
		Agent:        innerAgent,
		ToAgentNames: []string{"target_agent"},
	})

	iter := agent.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("run")}})
	events := drainAgentEvents(t, iter)

	foundTransfer := false
	for _, ev := range events {
		if ev.Action != nil && ev.Action.TransferToAgent != nil {
			foundTransfer = true
			if ev.Action.TransferToAgent.DestAgentName != "target_agent" {
				t.Errorf("expected transfer to target_agent, got %s", ev.Action.TransferToAgent.DestAgentName)
			}
		}
	}
	if !foundTransfer {
		t.Log("non-flow-agent: transfer may or may not be appended")
	}
}

func TestDeterministicTransfer_InterruptSkipsTransfer(t *testing.T) {
	ctx := context.Background()

	innerAgent := &dtTestAgent{
		name: "interrupt_test",
		runFn: func(ctx context.Context, input *AgentInput, options ...RunOption) *AsyncIterator[*AgentEvent] {
			iter, gen := NewAsyncIteratorPair[*AgentEvent]()
			go func() {
				defer gen.Close()
				gen.Send(EventFromMessage(&schema.Message{Role: schema.RoleAssistant, Content: "before"}, nil, schema.RoleAssistant, ""))
				gen.Send(Interrupt(ctx, "test_interrupt"))
			}()
			return iter
		},
	}

	agent := AgentWithDeterministicTransfer(ctx, &DeterministicTransferConfig{
		Agent:        innerAgent,
		ToAgentNames: []string{"transfer_after"},
	})

	iter := agent.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("interrupt")}})
	events := drainAgentEvents(t, iter)

	foundTransfer := false
	for _, ev := range events {
		if ev.Action != nil && ev.Action.TransferToAgent != nil {
			foundTransfer = true
		}
	}
	if foundTransfer {
		t.Error("should not transfer after interrupt")
	}
}

func TestDeterministicTransfer_NonResumableAgent(t *testing.T) {
	ctx := context.Background()

	innerAgent := &dtTestAgent{
		name: "non_resumable",
		runFn: func(ctx context.Context, input *AgentInput, options ...RunOption) *AsyncIterator[*AgentEvent] {
			iter, gen := NewAsyncIteratorPair[*AgentEvent]()
			go func() {
				defer gen.Close()
				gen.Send(EventFromMessage(&schema.Message{Role: schema.RoleAssistant, Content: "done"}, nil, schema.RoleAssistant, ""))
			}()
			return iter
		},
	}

	agent := AgentWithDeterministicTransfer(ctx, &DeterministicTransferConfig{
		Agent:        innerAgent,
		ToAgentNames: []string{"next"},
	})

	iter := agent.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("run")}})
	drainAgentEvents(t, iter)
}
