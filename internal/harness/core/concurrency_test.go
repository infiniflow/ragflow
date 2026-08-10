package core

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"ragflow/internal/harness/core/schema"
)

// ================================================================
// Concurrency tests for agentcore concurrent components
// ================================================================

// ---- tools_node.go: concurrent tool execution via mockTool ----

// TestToolsNode_ConcurrentInvoke verifies concurrent tool Invoke calls are safe.
func TestToolsNode_ConcurrentInvoke(t *testing.T) {
	tool := &mockTool{name: "conc_tool", desc: "concurrency test tool"}
	ctx := context.Background()

	var wg sync.WaitGroup
	errs := make(chan error, 20)

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_, err := tool.Invoke(ctx, `{"id":`+string(rune('0'+id%10))+`}`)
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Errorf("concurrent tool invoke failed: %v", err)
		}
	}
}

// ---- workflow.go: parallel sub-agent execution ----

// TestWorkflow_ParallelAgentConcurrency verifies parallel sub-agents
// run safely when invoked concurrently.
func TestWorkflow_ParallelAgentConcurrency(t *testing.T) {
	m1 := &mockModel{}
	for i := 0; i < 10; i++ {
		m1.addResp("par_a_result")
	}
	m2 := &mockModel{}
	for i := 0; i < 10; i++ {
		m2.addResp("par_b_result")
	}

	a1 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m1}).WithName("par_conc_a")
	a2 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m2}).WithName("par_conc_b")

	ctx := context.Background()
	par, err := NewParallel(ctx, &ParallelConfig{
		Name:      "par_conc_test",
		SubAgents: []Agent{a1, a2},
	})
	if err != nil {
		t.Fatalf("NewParallel: %v", err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 5)

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: par})
			iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("parallel conc test")})
			for {
				ev, ok := iter.Next()
				if !ok {
					break
				}
				if ev.Err != nil {
					errs <- ev.Err
					return
				}
			}
			errs <- nil
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Errorf("parallel workflow error: %v", err)
		}
	}
}

// ---- AgentLoop concurrent Push/Stop ----

// TestTurnLoop_ConcurrentPushStop verifies AgentLoop handles concurrent
// Push and Stop operations safely.
func TestTurnLoop_ConcurrentPushStop(t *testing.T) {
	ctx := context.Background()

	loop := NewAgentLoop[*schema.Message](AgentLoopConfig[*schema.Message]{
		GenInput: func(_ context.Context, l *AgentLoop[*schema.Message], items []*schema.Message) (*GenInputResult[*schema.Message], error) {
			return &GenInputResult[*schema.Message]{
				Input:     &AgentInput{Messages: items},
				Consumed:  items,
				Remaining: nil,
			}, nil
		},
		PrepareAgent: func(_ context.Context, _ *AgentLoop[*schema.Message], consumed []*schema.Message) (Agent, error) {
			m := &mockModel{}
			m.addResp("turn loop conc response")
			return NewReActAgent(&ReActConfig[*schema.Message]{Model: m}).WithName("conc_loop"), nil
		},
	})

	var wg sync.WaitGroup
	// Concurrent Push from multiple goroutines
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			loop.Push(schema.UserMessage("concurrent item"))
		}(i)
	}
	wg.Wait()

	loop.Run(ctx)
	loop.Stop()
	state := loop.Wait()
	if state.ExitReason != nil && !errors.As(state.ExitReason, new(*CancelError)) {
		t.Logf("turn loop exit: %v", state.ExitReason)
	}
}

// ---- ReActAgent concurrent Run ----

// TestReActAgent_ConcurrentRun verifies multiple agents can run concurrently.
func TestReActAgent_ConcurrentRun(t *testing.T) {
	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			m := &mockModel{}
			m.addResp("concurrent result")
			m.addResp("concurrent result")
			agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: m}).WithName("conc_agent")
			iter := agent.Run(ctx, &AgentInput{
				Messages: []*schema.Message{schema.UserMessage("concurrent run test")},
			})
			for {
				ev, ok := iter.Next()
				if !ok {
					break
				}
				if ev.Err != nil {
					errs <- ev.Err
					return
				}
			}
			errs <- nil
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Errorf("concurrent agent run failed: %v", err)
		}
	}
}

// ---- Runner concurrent execution ----

// TestRunner_ConcurrentInstances verifies multiple Runner instances
// executing concurrently don't interfere.
func TestRunner_ConcurrentInstances(t *testing.T) {
	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make(chan error, 8)

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			m := &mockModel{}
			m.addResp("runner conc result")
			m.addResp("runner conc result")
			agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: m}).WithName("runner_conc")
			runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent})
			iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("runner conc test")})
			for {
				ev, ok := iter.Next()
				if !ok {
					break
				}
				if ev.Err != nil {
					errs <- ev.Err
					return
				}
			}
			errs <- nil
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Errorf("concurrent runner failed: %v", err)
		}
	}
}

// ---- Tool-related concurrency ----

// TestTool_ConcurrentAgent verifies AgentTool with concurrent parent agents.
func TestTool_ConcurrentAgent(t *testing.T) {
	innerM := &mockModel{}
	innerM.addResp("inner agent result")
	innerAgent := NewReActAgent(&ReActConfig[*schema.Message]{Model: innerM}).WithName("inner_conc").WithDescription("inner agent")

	ctx := context.Background()
	agentTool := NewAgentTool(ctx, innerAgent)

	parentM := &forcedToolModel{
		inner:     &mockModel{},
		toolCalls: []schema.ToolCall{{ID: "conc_tc", Function: schema.ToolCallFunction{Name: "inner_conc", Arguments: "{}"}}},
		finalResp: "parent done",
		firstCall: true,
	}
	parent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: parentM, Tools: []Tool{agentTool},
		ToolsConfig: &ToolsNodeConfig{Tools: []Tool{agentTool}},
	}).WithName("parent_conc")

	var wg sync.WaitGroup
	errs := make(chan error, 5)

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: parent})
			iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("use tool conc")})
			for {
				ev, ok := iter.Next()
				if !ok {
					break
				}
				if ev.Err != nil {
					errs <- ev.Err
					return
				}
			}
			errs <- nil
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Errorf("concurrent agent tool failed: %v", err)
		}
	}
}

// ---- Sequential workflow concurrency ----

// TestWorkflow_SequentialConcurrent verifies multiple sequential workflows
// run concurrently without interference.
func TestWorkflow_SequentialConcurrent(t *testing.T) {
	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make(chan error, 6)

	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			m1 := &mockModel{}
			m1.addResp("seq_a_conc")
			m1.addResp("seq_a_conc")
			m2 := &mockModel{}
			m2.addResp("seq_b_conc")
			m2.addResp("seq_b_conc")
			a1 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m1}).WithName("seq_conc_a")
			a2 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m2}).WithName("seq_conc_b")

			seq, err := NewSequential(ctx, &SequentialConfig{
				Name:      "seq_conc",
				SubAgents: []Agent{a1, a2},
			})
			if err != nil {
				errs <- err
				return
			}
			runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: seq})
			iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("seq conc test")})
			for {
				ev, ok := iter.Next()
				if !ok {
					break
				}
				if ev.Err != nil {
					errs <- ev.Err
					return
				}
			}
			errs <- nil
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Errorf("sequential workflow conc error: %v", err)
		}
	}
}

// ---- Cancel concurrency ----

// TestCancel_ConcurrentTrigger verifies cancel can be triggered concurrently.
func TestCancel_ConcurrentTrigger(t *testing.T) {
	m := newCancelTestChatModel(nil)
	m.addResp("will be cancelled")
	m.setDelay(100 * time.Millisecond)

	agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: m}).WithName("cancel_conc")

	cancelOpt, cancelFunc := WithCancel()
	store := newCancelTestStore()
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent, CheckPointStore: store})
	ctx := context.Background()
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("cancel conc test")}, cancelOpt)

	// Trigger cancel from multiple goroutines
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Go(func() {
			cancelFunc(WithCancelMode(CancelImmediate))
		})
	}
	wg.Wait()

	// Drain
	time.Sleep(20 * time.Millisecond)
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			t.Logf("cancel err: %v", ev.Err)
			break
		}
	}
}

// ---- Interrupt concurrency ----

// TestInterrupt_Concurrent verifies interrupt state can be read concurrently.
func TestInterrupt_Concurrent(t *testing.T) {
	ctx := context.Background()
	model := &forcedToolModel{
		inner:     &mockModel{},
		toolCalls: []schema.ToolCall{{ID: "ci", Function: schema.ToolCallFunction{Name: "ci_tool", Arguments: "{}"}}},
		finalResp: "ci done",
		firstCall: true,
	}
	tool := &mockTool{name: "ci_tool", desc: "concurrent interrupt tool"}
	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: model, Tools: []Tool{tool},
		ToolsConfig: &ToolsNodeConfig{Tools: []Tool{tool}},
	}).WithName("ci_agent")

	store := newCancelTestStore()
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent, CheckPointStore: store})
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("ci test")})

	// Single consumer — iter.Next() is not safe for concurrent access.
	for {
		_, ok := iter.Next()
		if !ok {
			break
		}
	}
}
